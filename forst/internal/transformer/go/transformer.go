// Package transformergo converts a Forst AST to a Go AST
package transformergo

import (
	"fmt"
	"forst/internal/ast"
	"forst/internal/modulecheck"
	"forst/internal/typechecker"
	goast "go/ast"
	goasttoken "go/token"

	"sort"
	"strings"

	"github.com/sirupsen/logrus"
)

// Transformer converts a Forst AST to a Go AST
type Transformer struct {
	TypeChecker          *typechecker.TypeChecker
	Output               *TransformerOutput
	assertionTransformer *AssertionTransformer
	log                  *logrus.Logger

	// If true, struct fields for return values will be exported (capitalized)
	ExportReturnStructFields bool

	// Track functions that have ensure statements to prevent normal return statements from overwriting error returns
	functionsWithEnsure map[string]bool

	// resultLocalSplit maps a Forst variable name to the Go identifiers used when lowering
	// "x := pkg.F()" where F returns Result (Go (values..., error)). Scoped per transformFunction.
	resultLocalSplit map[string]resultLocalSplit

	// currentFnBody is the Forst function body being transformed (tuple slot use analysis).
	currentFnBody []ast.Node

	// emittedSealMethods records receiver+method pairs for nominal error union sealing (dedupe on re-emit).
	emittedSealMethods map[string]struct{}

	// mapIndexFuncLitCache deduplicates identical map-read IIFEs within one Forst function (key: expr|succType).
	mapIndexFuncLitCache map[string]*goast.FuncLit
	// mapIndexCacheHits counts cache hits during transform (second+ identical map read in a function).
	mapIndexCacheHits int

	// providersStructByKey maps sorted slot-set key → deduped Providers struct name (ADR-013).
	providersStructByKey map[string]string
	// moduleResult for cross-package Forst call lowering.
	moduleResult *modulecheck.ModuleResult
	// wiringStack holds merged wiring frames during with-block lowering.
	wiringStack []wiringFrame
	// currentFnProvidersName is the Go identifier for the active function's providers param (typically "providers").
	currentFnProvidersName string
	// currentFnProvidersSlots is the slot set for the active function (for pass-through lowering).
	currentFnProvidersSlots []typechecker.ProviderSlot

	// inlineGenericShapeParams records function parameters lowered as inline struct{ ... T } (generic shape params).
	inlineGenericShapeParams map[ast.Identifier]map[int]struct{}

	// OmitPackageTypeDefs skips emitting package types when a lib shim already defines them.
	OmitPackageTypeDefs bool
	// entryNodes is the slice passed to TransformForstFileToGo (for scope-node fallback lookups).
	entryNodes []ast.Node

	// BridgeRuntimeOutput holds generated forst_0_bridge_runtime.gen.go content (bridgert import, wrappers).
	BridgeRuntimeOutput *TransformerOutput
	nodeWrappersEmitted map[string]bool
	nodeSeqTypesEmitted map[string]bool

	// EmbedInvokeServer when true appends ForstInvokeWaitForShutdown() to main for long-lived binaries.
	EmbedInvokeServer bool
	// EmbedBridgeHostMode when true emits ForstBridgeWaitForShutdown for host-mode bridgert binaries.
	EmbedBridgeHostMode bool
	// SandboxModulePath when set rewrites cross-package invoke imports (e.g. forst.run.temp/bcrypt).
	SandboxModulePath string
}

// New creates a new Transformer
func New(tc *typechecker.TypeChecker, log *logrus.Logger, exportReturnStructFields ...bool) *Transformer {
	if log == nil {
		log = logrus.New()
		log.Warnf("No logger provided, using default logger")
	}
	t := &Transformer{
		TypeChecker:          tc,
		Output:               &TransformerOutput{},
		log:                  log,
		functionsWithEnsure:        make(map[string]bool),
		providersStructByKey:       make(map[string]string),
		inlineGenericShapeParams:   make(map[ast.Identifier]map[int]struct{}),
	}
	t.assertionTransformer = NewAssertionTransformer(t)
	if len(exportReturnStructFields) > 0 {
		t.ExportReturnStructFields = exportReturnStructFields[0]
	}
	return t
}

// SetModuleResult attaches cross-package Providers metadata for call lowering.
func (t *Transformer) SetModuleResult(m *modulecheck.ModuleResult) {
	t.moduleResult = m
}

// TransformForstFileToGo converts a Forst AST to a Go AST
// The nodes should already have their types inferred/checked
func (t *Transformer) TransformForstFileToGo(nodes []ast.Node) (*goast.File, error) {
	t.entryNodes = nodes
	// First, collect and register shape types from type definitions
	if err := t.defineShapeTypes(); err != nil {
		return nil, err
	}

	// Process all definitions first (sorted for deterministic emission)
	typeNames := make([]ast.TypeIdent, 0, len(t.TypeChecker.Defs))
	for name := range t.TypeChecker.Defs {
		typeNames = append(typeNames, name)
	}
	sort.Slice(typeNames, func(i, j int) bool { return typeNames[i] < typeNames[j] })
	if !t.OmitPackageTypeDefs {
		for _, name := range typeNames {
			if t.shouldOmitGoPackageType(name) {
				continue
			}
			// Hash-based structural types are emitted only when referenced
			// (ensureAllReferencedTypesEmitted / use-site defineShapeType).
			if t.TypeChecker.IsHashBasedIdent(name) {
				t.log.WithFields(logrus.Fields{
					"typeIdent": name,
					"function":  "TransformForstFileToGo",
				}).Debug("Skipping unused hash-based type in Defs dump")
				continue
			}
			def := t.TypeChecker.Defs[name]
			switch def := def.(type) {
			case ast.TypeDefNode:
				if t.shapeTypeDefUsesGenericTypeParams(def) {
					continue
				}
				t.log.WithFields(logrus.Fields{
					"typeDef":  def.GetIdent(),
					"function": "TransformForstFileToGo",
				}).Debug("Processing type definition")
				decl, err := t.transformTypeDef(def)
				if err != nil {
					return nil, fmt.Errorf("failed to transform type def %s: %w", def.GetIdent(), err)
				}
				t.Output.AddType(decl)
				t.log.WithFields(logrus.Fields{
					"typeDef":  def.GetIdent(),
					"function": "TransformForstFileToGo",
				}).Debug("Added type definition to output")
			case ast.TypeGuardNode:
				t.log.WithFields(logrus.Fields{
					"guard":    def.GetIdent(),
					"function": "TransformForstFileToGo",
				}).Debug("Processing type guard definition (value)")
				scopeNode := t.resolveTypeGuardScopeNode(nodes, def)
				decl, err := t.transformTypeGuard(scopeNode, def)
				if err != nil {
					return nil, fmt.Errorf("failed to transform type guard %s: %w", def.GetIdent(), err)
				}
				if decl != nil {
					t.Output.AddFunction(decl)
				}
			case *ast.TypeGuardNode:
				t.log.WithFields(logrus.Fields{
					"guard":    def.GetIdent(),
					"function": "TransformForstFileToGo",
				}).Debug("Processing type guard definition (pointer)")
				scopeNode := t.resolveTypeGuardScopeNode(nodes, *def)
				decl, err := t.transformTypeGuard(scopeNode, *def)
				if err != nil {
					return nil, fmt.Errorf("failed to transform type guard %s: %w", def.GetIdent(), err)
				}
				if decl != nil {
					t.Output.AddFunction(decl)
				}
			case ast.TypeDefShapeExpr:
				decl, err := t.transformShapeType(&def.Shape)
				if err != nil {
					return nil, fmt.Errorf("failed to transform type def shape: %w", err)
				}
				t.Output.AddType(&goast.GenDecl{
					Tok: goasttoken.TYPE,
					Specs: []goast.Spec{
						&goast.TypeSpec{
							Name: goast.NewIdent(string(*def.Shape.BaseType)), // TODO: fix this
							Type: *decl,
						},
					},
				})
			}
		}
	}

	// Emit deduped Providers struct types before functions.
	if !t.OmitPackageTypeDefs {
		if err := t.emitAllProvidersStructs(); err != nil {
			return nil, fmt.Errorf("failed to emit Providers structs: %w", err)
		}
	}

	// Then process the rest of the nodes
	for _, node := range nodes {
		switch n := node.(type) {
		case ast.PackageNode:
			t.Output.SetPackageName(string(n.Ident.ID))
		case ast.ImportNode:
			if n.BridgeOptIn {
				break
			}
			decl := t.transformImport(n)
			t.Output.AddImport(decl)
		case ast.ImportGroupNode:
			decl := t.transformImportGroup(n)
			t.Output.AddImportGroup(decl)
		case ast.FunctionNode:
			scopeNode := t.resolveFunctionScopeNode(nodes, n)
			decl, err := t.transformFunction(scopeNode, n)
			if err != nil {
				return nil, fmt.Errorf("failed to transform function %s: %w", n.GetIdent(), err)
			}
			t.Output.AddFunction(decl)
		case ast.AssignmentNode:
			if !n.IsPackageLevel {
				break
			}
			decl, err := t.transformPackageVarDecl(n)
			if err != nil {
				return nil, fmt.Errorf("failed to transform package var: %w", err)
			}
			t.Output.AddValueDecl(decl)
		case ast.ConstGroupNode:
			decl, err := t.transformConstGroup(n)
			if err != nil {
				return nil, fmt.Errorf("failed to transform const group: %w", err)
			}
			t.Output.AddValueDecl(decl)
		}
	}

	// Log the final output
	t.log.WithFields(logrus.Fields{
		"function": "TransformForstFileToGo",
		"types":    len(t.Output.types),
		"funcs":    len(t.Output.functions),
	}).Debug("Generated Go file")

	// Ensure all referenced types are emitted
	if !t.OmitPackageTypeDefs {
		if err := t.ensureAllReferencedTypesEmitted(); err != nil {
			return nil, fmt.Errorf("failed to ensure all referenced types are emitted: %w", err)
		}
	}

	t.appendNodeBridgeIfNeeded()

	return t.Output.GenerateFile()
}

func (t *Transformer) shouldOmitGoPackageType(typeIdent ast.TypeIdent) bool {
	if t == nil || t.TypeChecker == nil {
		return false
	}
	return t.TypeChecker.IsGoPackageType(typeIdent) || t.TypeChecker.SamePackageGoDefinesType(typeIdent)
}

func (t *Transformer) IsMainPackage() bool {
	return t.isMainPackage()
}

func (t *Transformer) isMainPackage() bool {
	return t.Output.PackageName() == "main"
}

// closestFunction returns either the node corresponding to the current scope's function
// or, if the current scope is not a function, the next highest function node in the scope stack
// It returns an error if no function is found
func (t *Transformer) closestFunction() (ast.Node, error) {
	scope := t.currentScope()

	if scope.IsGlobal() {
		t.log.WithFields(map[string]any{
			"scope":    scope,
			"function": "closestFunction",
		}).Debug("Current scope is global")
		return nil, fmt.Errorf("current scope is global, no closest function possible")
	}

	if scope.IsFunction() {
		t.log.WithFields(map[string]any{
			"scope":    scope,
			"function": "closestFunction",
		}).Debug("Current scope is a function")
		return *scope.Node, nil
	}

	t.log.WithFields(map[string]any{
		"scope":    scope,
		"function": "closestFunction",
	}).Debug("Current scope is not a function, searching up the scope stack")

	for scope != nil && scope.Parent != nil {
		scope = scope.Parent
		if scope.Node != nil {
			t.log.WithFields(map[string]any{
				"scope":      scope,
				"isFunction": scope.IsFunction(),
				"function":   "closestFunction",
			}).Debug("Checking parent scope")

			if scope.IsFunction() {
				t.log.WithFields(map[string]any{
					"scope":    scope,
					"function": "closestFunction",
				}).Debug("Found function in scope stack")
				return *scope.Node, nil
			}
		}
	}

	t.log.WithFields(map[string]any{
		"function": "closestFunction",
	}).Debug("No function found in scope stack")
	return ast.FunctionNode{}, fmt.Errorf("no function found")
}

func (t *Transformer) isMainFunction() bool {
	if !t.isMainPackage() {
		return false
	}

	scope := t.currentScope()
	if scope.IsGlobal() {
		t.log.Fatalf("isMainFunction called in global scope")
	}

	function, err := t.closestFunction()
	if err != nil {
		return false
	}
	if function, ok := function.(ast.FunctionNode); ok && function.HasMainFunctionName() {
		return true
	}

	return false
}

func (t *Transformer) isTestFunction() bool {
	function, err := t.closestFunction()
	if err != nil {
		return false
	}
	fn, ok := function.(ast.FunctionNode)
	if !ok {
		return false
	}
	return t.isGoTestFunction(fn)
}

// ensureAllReferencedTypesEmitted ensures hash-based (and other) types that appear
// in already-generated Go are emitted. It does not dump every Defs entry.
func (t *Transformer) ensureAllReferencedTypesEmitted() error {
	t.log.Debug("Starting ensureAllReferencedTypesEmitted")

	processed := make(map[ast.TypeIdent]bool)
	if err := t.scanAndEmitReferencedTypes(processed); err != nil {
		return fmt.Errorf("failed to scan and emit referenced types: %w", err)
	}
	return nil
}

// scanAndEmitReferencedTypes walks generated types and functions (including
// bodies and receivers) and emits any Forst hash types they reference.
func (t *Transformer) scanAndEmitReferencedTypes(processed map[ast.TypeIdent]bool) error {
	t.log.Debug("Scanning generated code for referenced types")

	for _, typeDecl := range t.Output.types {
		if err := t.scanGoNodeForReferencedTypes(typeDecl, processed); err != nil {
			return err
		}
	}
	for _, funcDecl := range t.Output.functions {
		if err := t.scanGoNodeForReferencedTypes(funcDecl, processed); err != nil {
			return err
		}
	}
	return nil
}

// scanGoNodeForReferencedTypes walks a Go AST fragment and emits hash-based
// Forst types named by identifiers (including composite-literal type names).
func (t *Transformer) scanGoNodeForReferencedTypes(node goast.Node, processed map[ast.TypeIdent]bool) error {
	if node == nil {
		return nil
	}
	var walkErr error
	goast.Inspect(node, func(n goast.Node) bool {
		if walkErr != nil || n == nil {
			return false
		}
		ident, ok := n.(*goast.Ident)
		if !ok {
			return true
		}
		if err := t.ensureTypeEmittedFromGoType(ident, processed); err != nil {
			walkErr = err
			return false
		}
		return true
	})
	return walkErr
}

// ensureTypeEmittedFromGoType ensures that a Go type is properly emitted if it represents a Forst type
func (t *Transformer) ensureTypeEmittedFromGoType(goType goast.Expr, processed map[ast.TypeIdent]bool) error {
	if goType == nil {
		return nil
	}
	t.log.WithFields(logrus.Fields{
		"function": "ensureTypeEmittedFromGoType",
		"goType":   fmt.Sprintf("%#v", goType),
	}).Debug("[DEBUG] Checking type emission for goType")

	switch expr := goType.(type) {
	case *goast.Ident:
		typeIdent := ast.TypeIdent(expr.Name)
		// Prefer IsHashBasedIdent over a raw T_ prefix (users may name type T_Foo).
		isHash := t.TypeChecker != nil && t.TypeChecker.IsHashBasedIdent(typeIdent)
		looksLikeHash := strings.HasPrefix(expr.Name, "T_")
		if !isHash && !looksLikeHash {
			return nil
		}
		if processed[typeIdent] {
			return nil
		}
		t.log.WithFields(logrus.Fields{
			"function": "ensureTypeEmittedFromGoType",
			"type":     expr.Name,
		}).Debug("[DEBUG] Found hash-based type in generated code that needs emission")
		if def, exists := t.TypeChecker.Defs[typeIdent]; exists {
			if err := t.emitTypeAndReferencedTypes(typeIdent, def, processed); err != nil {
				return fmt.Errorf("failed to emit referenced type %s: %w", typeIdent, err)
			}
			return nil
		}
		if !looksLikeHash {
			return nil
		}
		t.log.WithFields(logrus.Fields{
			"function": "ensureTypeEmittedFromGoType",
			"type":     expr.Name,
		}).Debug("[DEBUG] Hash-based type found in generated code but not in Defs, creating minimal definition")
		minimalDef := ast.TypeDefNode{
			Ident: typeIdent,
			Expr: ast.TypeDefAssertionExpr{
				Assertion: &ast.AssertionNode{
					BaseType: func() *ast.TypeIdent { t := ast.TypeString; return &t }(),
					Constraints: []ast.ConstraintNode{{
						Name: "Value",
						Args: []ast.ConstraintArgumentNode{{
							Value: func() *ast.ValueNode {
								v := ast.ValueNode(ast.StringLiteralNode{Value: "placeholder"})
								return &v
							}(),
						}},
					}},
				},
			},
		}
		if err := t.emitTypeAndReferencedTypes(typeIdent, minimalDef, processed); err != nil {
			return fmt.Errorf("failed to emit minimal type definition for %s: %w", typeIdent, err)
		}
	case *goast.StarExpr:
		return t.ensureTypeEmittedFromGoType(expr.X, processed)
	case *goast.ArrayType:
		return t.ensureTypeEmittedFromGoType(expr.Elt, processed)
	case *goast.MapType:
		if err := t.ensureTypeEmittedFromGoType(expr.Key, processed); err != nil {
			return err
		}
		return t.ensureTypeEmittedFromGoType(expr.Value, processed)
	}
	return nil
}

// emitTypeAndReferencedTypes recursively emits a type and all types it references
func (t *Transformer) emitTypeAndReferencedTypes(typeIdent ast.TypeIdent, def any, processed map[ast.TypeIdent]bool) error {
	// Add debug log for type emission
	t.log.WithFields(logrus.Fields{
		"function":  "emitTypeAndReferencedTypes",
		"typeIdent": typeIdent,
		"defType":   fmt.Sprintf("%T", def),
	}).Debug("[DEBUG] Emitting type and referenced types")
	// Skip if already processed
	if processed[typeIdent] {
		return nil
	}
	processed[typeIdent] = true

	// Same-package Go named types already exist in sibling .go; do not re-emit.
	if t.shouldOmitGoPackageType(typeIdent) {
		t.log.WithFields(logrus.Fields{
			"function":  "emitTypeAndReferencedTypes",
			"typeIdent": typeIdent,
		}).Debug("[DEBUG] Skipping emit for same-package Go type")
		return nil
	}

	// Check if already emitted
	alreadyEmitted := false
	for _, typeDecl := range t.Output.types {
		if len(typeDecl.Specs) > 0 {
			if spec, ok := typeDecl.Specs[0].(*goast.TypeSpec); ok {
				if spec.Name.Name == string(typeIdent) {
					alreadyEmitted = true
					break
				}
			}
		}
	}

	if alreadyEmitted {
		t.log.WithFields(logrus.Fields{
			"function":  "emitTypeAndReferencedTypes",
			"typeIdent": typeIdent,
		}).Debug("[DEBUG] Type already emitted, skipping")
		return nil
	}

	if typeDef, ok := def.(ast.TypeDefNode); ok && t.shapeTypeDefUsesGenericTypeParams(typeDef) {
		t.log.WithFields(logrus.Fields{
			"function":  "emitTypeAndReferencedTypes",
			"typeIdent": typeIdent,
		}).Debug("[DEBUG] Skipping generic shape type def emission (lowered inline at use sites)")
		return nil
	}

	// Special case: emit type alias for hash-based types that are value constraints or primitive aliases
	if typeDef, ok := def.(ast.TypeDefNode); ok {
		if assertionExpr, ok := typeDef.Expr.(ast.TypeDefAssertionExpr); ok && assertionExpr.Assertion != nil {
			// If the assertion is a value constraint or base type is a primitive, emit alias
			if assertionExpr.Assertion.BaseType != nil {
				base := *assertionExpr.Assertion.BaseType
				t.log.WithFields(logrus.Fields{
					"function":  "emitTypeAndReferencedTypes",
					"typeIdent": typeIdent,
					"baseType":  base,
				}).Debug("[DEBUG] Emitting type alias for hash-based or primitive type")
				typeNode := ast.TypeNode{Ident: base}
				if typeNode.IsGoBuiltin() || base == ast.TypeString || base == ast.TypeInt || base == ast.TypeFloat || base == ast.TypeBool {
					goType, err := transformTypeIdent(base)
					if err == nil && goType != nil {
						t.Output.AddType(&goast.GenDecl{
							Tok: goasttoken.TYPE,
							Specs: []goast.Spec{
								&goast.TypeSpec{
									Name: goast.NewIdent(string(typeIdent)),
									Type: goType,
								},
							},
						})
						return nil
					}
				}
			}
		}
	}

	// Determine the type name to use for emission
	// For user-defined types (not hash-based), use the original name
	// For hash-based types, use the hash-based name
	typeNameToEmit := string(typeIdent)
	// The typeIdent is already the correct name to use - no need to change it
	// User-defined types will have their original names, hash-based types will have hash-based names

	// Transform and emit the type definition
	switch def := def.(type) {
	case ast.TypeDefNode:
		// First, emit all types referenced by this type definition
		if err := t.emitReferencedTypes(def, processed); err != nil {
			return fmt.Errorf("failed to emit referenced types for %s: %w", typeIdent, err)
		}

		// Then emit this type using the determined type name
		decl, err := t.transformTypeDef(def)
		if err != nil {
			t.log.WithFields(logrus.Fields{
				"function": "emitTypeAndReferencedTypes",
				"type":     string(typeIdent),
				"error":    err,
			}).Warn("Failed to transform type definition")
			return nil
		}
		if decl != nil {
			// Extract the type expression from the GenDecl
			var typeExpr goast.Expr
			if len(decl.Specs) > 0 {
				if spec, ok := decl.Specs[0].(*goast.TypeSpec); ok {
					typeExpr = spec.Type
				}
			}

			// Use the determined type name
			t.Output.AddType(&goast.GenDecl{
				Tok: goasttoken.TYPE,
				Specs: []goast.Spec{
					&goast.TypeSpec{
						Name: goast.NewIdent(typeNameToEmit),
						Type: typeExpr,
					},
				},
			})
			t.log.WithFields(logrus.Fields{
				"function":    "emitTypeAndReferencedTypes",
				"type":        string(typeIdent),
				"emittedName": typeNameToEmit,
			}).Debug("[DEBUG] Emitted type definition")
		}

	case ast.TypeDefShapeExpr:
		// First, emit all types referenced by this shape
		if err := t.emitReferencedTypesFromShape(&def.Shape, processed); err != nil {
			return fmt.Errorf("failed to emit referenced types for shape %s: %w", typeIdent, err)
		}

		// Then emit this shape type using the determined type name
		decl, err := t.transformShapeType(&def.Shape)
		if err != nil {
			t.log.WithFields(logrus.Fields{
				"function": "emitTypeAndReferencedTypes",
				"type":     string(typeIdent),
				"error":    err,
			}).Warn("Failed to transform shape type")
			return nil
		}
		if decl != nil {
			t.Output.AddType(&goast.GenDecl{
				Tok: goasttoken.TYPE,
				Specs: []goast.Spec{
					&goast.TypeSpec{
						Name: goast.NewIdent(typeNameToEmit),
						Type: *decl,
					},
				},
			})
			t.log.WithFields(logrus.Fields{
				"function":    "emitTypeAndReferencedTypes",
				"type":        string(typeIdent),
				"emittedName": typeNameToEmit,
			}).Debug("[DEBUG] Emitted shape type definition")
		}
	}

	return nil
}

// emitReferencedTypes emits all types referenced by a TypeDefNode
func (t *Transformer) emitReferencedTypes(def ast.TypeDefNode, processed map[ast.TypeIdent]bool) error {
	switch expr := def.Expr.(type) {
	case ast.TypeDefAssertionExpr:
		if expr.Assertion != nil {
			return t.emitReferencedTypesFromAssertion(expr.Assertion, processed)
		}
	default:
		if payload, ok := ast.PayloadShape(def.Expr); ok {
			return t.emitReferencedTypesFromShape(payload, processed)
		}
	}
	return nil
}

// emitReferencedTypesFromShape emits all types referenced by a shape
func (t *Transformer) emitReferencedTypesFromShape(shape *ast.ShapeNode, processed map[ast.TypeIdent]bool) error {
	for _, field := range shape.Fields {
		if field.Type != nil {
			// Emit the field type if it's a user-defined or hash-based type
			if !field.Type.IsGoBuiltin() {
				// Look up the type definition in TypeChecker.Defs
				if def, exists := t.TypeChecker.Defs[field.Type.Ident]; exists {
					if err := t.emitTypeAndReferencedTypes(field.Type.Ident, def, processed); err != nil {
						return fmt.Errorf("failed to emit field type %s: %w", field.Type.Ident, err)
					}
				}
			}
		}
		if field.Shape != nil {
			// Recursively emit nested shapes
			if err := t.emitReferencedTypesFromShape(field.Shape, processed); err != nil {
				return fmt.Errorf("failed to emit nested shape: %w", err)
			}
		}
		if field.Assertion != nil {
			// Emit types referenced by assertions
			if err := t.emitReferencedTypesFromAssertion(field.Assertion, processed); err != nil {
				return fmt.Errorf("failed to emit assertion type: %w", err)
			}
		}
	}
	return nil
}

// emitReferencedTypesFromAssertion emits all types referenced by an assertion
func (t *Transformer) emitReferencedTypesFromAssertion(assertion *ast.AssertionNode, processed map[ast.TypeIdent]bool) error {
	// Emit base type if it's user-defined
	if assertion.BaseType != nil {
		baseType := ast.TypeNode{Ident: *assertion.BaseType}
		if !baseType.IsGoBuiltin() {
			// Look up the type definition in TypeChecker.Defs
			if def, exists := t.TypeChecker.Defs[*assertion.BaseType]; exists {
				if err := t.emitTypeAndReferencedTypes(*assertion.BaseType, def, processed); err != nil {
					return fmt.Errorf("failed to emit assertion base type %s: %w", *assertion.BaseType, err)
				}
			}
		}
	}

	// Emit types referenced in constraints
	for _, constraint := range assertion.Constraints {
		for _, arg := range constraint.Args {
			if arg.Shape != nil {
				if err := t.emitReferencedTypesFromShape(arg.Shape, processed); err != nil {
					return fmt.Errorf("failed to emit constraint shape: %w", err)
				}
			}
		}
	}
	return nil
}

// getExpectedTypeForShape determines the expected type for a shape literal based on context.
// This function provides a unified way to determine the best type to use for struct literal emission.
// It prioritizes named types when available, falling back to hash-based types only when necessary.
func (t *Transformer) getExpectedTypeForShape(shape *ast.ShapeNode, context *ShapeContext) *ast.TypeNode {
	t.log.WithFields(logrus.Fields{
		"function": "getExpectedTypeForShape",
		"context":  fmt.Sprintf("%+v", context),
		"shape":    fmt.Sprintf("%+v", shape),
	}).Debug("[DEBUG] Determining expected type for shape literal")

	// If the shape has an explicit BaseType, use it
	if shape.BaseType != nil {
		t.log.WithFields(logrus.Fields{
			"function": "getExpectedTypeForShape",
			"baseType": *shape.BaseType,
		}).Debug("[DEBUG] Using explicit BaseType")
		return &ast.TypeNode{Ident: *shape.BaseType}
	}

	// If context provides an expected type, validate and use it
	if context != nil && context.ExpectedType != nil {
		expectedType := context.ExpectedType
		t.log.WithFields(logrus.Fields{
			"function":     "getExpectedTypeForShape",
			"expectedType": expectedType.Ident,
		}).Debug("[DEBUG] Context provided expected type")

		// If the expected type is a hash-based type, try to find a structurally compatible named type
		if strings.HasPrefix(string(expectedType.Ident), "T_") {
			// Use robust type selection to find the best named type
			bestType := t.findBestNamedTypeForStructLiteral(*expectedType, nil)
			if !strings.HasPrefix(string(bestType.Ident), "T_") {
				t.log.WithFields(logrus.Fields{
					"function": "getExpectedTypeForShape",
					"hashType": expectedType.Ident,
					"bestType": bestType.Ident,
					"note":     "Resolved hash-based type to named type",
				}).Debug("[PINPOINT] getExpectedTypeForShape: Resolved hash-based type to named type")
				return &bestType
			}
		}

		// Check if the expected type is compatible with the shape
		if def, exists := t.TypeChecker.Defs[expectedType.Ident]; exists {
			if typeDef, ok := def.(ast.TypeDefNode); ok {
				if payload, ok := ast.PayloadShape(typeDef.Expr); ok {
					// Use typechecker to validate compatibility
					err := t.TypeChecker.ValidateShapeFields(*payload, shape.Fields, expectedType.Ident)
					if err == nil {
						t.log.WithFields(logrus.Fields{
							"function":     "getExpectedTypeForShape",
							"expectedType": expectedType.Ident,
						}).Debug("[DEBUG] Expected type is compatible")
						return expectedType
					}
					t.log.WithFields(logrus.Fields{
						"function":     "getExpectedTypeForShape",
						"expectedType": expectedType.Ident,
						"error":        err.Error(),
					}).Debug("[DEBUG] Expected type is not compatible, will fall back to structural matching")
				}
			}
		}
	}

	// Always try to find a matching named type through structural matching first
	// This is the most reliable way to find the correct type for shape literals
	typeIdent, found := t.findExistingTypeForShape(shape, nil)
	if found {
		t.log.WithFields(logrus.Fields{
			"function":  "getExpectedTypeForShape",
			"typeIdent": typeIdent,
		}).Debug("[DEBUG] Found matching named type through structural matching")
		return &ast.TypeNode{Ident: typeIdent}
	}

	// If context provides variable name, try to infer from variable type
	if context != nil && context.VariableName != "" {
		if types, ok := t.TypeChecker.VariableTypes[ast.Identifier(context.VariableName)]; ok && len(types) > 0 {
			expectedType := &types[0]
			t.log.WithFields(logrus.Fields{
				"function":     "getExpectedTypeForShape",
				"variableName": context.VariableName,
				"variableType": expectedType.Ident,
			}).Debug("[DEBUG] Found variable type for assignment")

			// Check if the variable type is compatible
			if def, exists := t.TypeChecker.Defs[expectedType.Ident]; exists {
				if typeDef, ok := def.(ast.TypeDefNode); ok {
					if payload, ok := ast.PayloadShape(typeDef.Expr); ok {
						err := t.TypeChecker.ValidateShapeFields(*payload, shape.Fields, expectedType.Ident)
						if err == nil {
							t.log.WithFields(logrus.Fields{
								"function":     "getExpectedTypeForShape",
								"variableType": expectedType.Ident,
							}).Debug("[DEBUG] Variable type is compatible")
							return expectedType
						}
					}
				}
			}
		}
	}

	// If context provides function name and parameter index, try to infer from function signature
	if context != nil && context.FunctionName != "" && context.ParameterIndex >= 0 {
		if sig, ok := t.TypeChecker.Functions[ast.Identifier(context.FunctionName)]; ok && context.ParameterIndex < len(sig.Parameters) {
			param := sig.Parameters[context.ParameterIndex]
			expectedType := &param.Type
			t.log.WithFields(logrus.Fields{
				"function":       "getExpectedTypeForShape",
				"functionName":   context.FunctionName,
				"parameterIndex": context.ParameterIndex,
				"parameterType":  expectedType.Ident,
			}).Debug("[DEBUG] Found function parameter type")

			// For assertion types, try to infer the concrete type
			if expectedType.Ident == ast.TypeAssertion && expectedType.Assertion != nil {
				inferredTypes, err := t.TypeChecker.InferAssertionType(expectedType.Assertion, false, "", nil)
				if err == nil && len(inferredTypes) > 0 {
					inferredType := &inferredTypes[0]
					t.log.WithFields(logrus.Fields{
						"function":     "getExpectedTypeForShape",
						"inferredType": inferredType.Ident,
					}).Debug("[DEBUG] Inferred concrete type from assertion")

					// Check if the inferred type is compatible
					if def, exists := t.TypeChecker.Defs[inferredType.Ident]; exists {
						if typeDef, ok := def.(ast.TypeDefNode); ok {
							if payload, ok := ast.PayloadShape(typeDef.Expr); ok {
								err := t.TypeChecker.ValidateShapeFields(*payload, shape.Fields, inferredType.Ident)
								if err == nil {
									t.log.WithFields(logrus.Fields{
										"function":     "getExpectedTypeForShape",
										"inferredType": inferredType.Ident,
									}).Debug("[DEBUG] Inferred type is compatible")
									return inferredType
								}
							}
						}
					}
				}
			} else {
				// For non-assertion types, check compatibility directly
				if def, exists := t.TypeChecker.Defs[expectedType.Ident]; exists {
					if typeDef, ok := def.(ast.TypeDefNode); ok {
						if payload, ok := ast.PayloadShape(typeDef.Expr); ok {
							err := t.TypeChecker.ValidateShapeFields(*payload, shape.Fields, expectedType.Ident)
							if err == nil {
								t.log.WithFields(logrus.Fields{
									"function":      "getExpectedTypeForShape",
									"parameterType": expectedType.Ident,
								}).Debug("[DEBUG] Parameter type is compatible")
								return expectedType
							}
						}
					}
				}
			}
		}
	}

	// If context provides return index, try to infer from function return type
	if context != nil && context.FunctionName != "" && context.ReturnIndex >= 0 {
		if sig, ok := t.TypeChecker.Functions[ast.Identifier(context.FunctionName)]; ok && context.ReturnIndex < len(sig.ReturnTypes) {
			returnType := &sig.ReturnTypes[context.ReturnIndex]
			t.log.WithFields(logrus.Fields{
				"function":     "getExpectedTypeForShape",
				"functionName": context.FunctionName,
				"returnIndex":  context.ReturnIndex,
				"returnType":   returnType.Ident,
			}).Debug("[DEBUG] Found function return type")

			// Check if the return type is compatible
			if def, exists := t.TypeChecker.Defs[returnType.Ident]; exists {
				if typeDef, ok := def.(ast.TypeDefNode); ok {
					if payload, ok := ast.PayloadShape(typeDef.Expr); ok {
						err := t.TypeChecker.ValidateShapeFields(*payload, shape.Fields, returnType.Ident)
						if err == nil {
							t.log.WithFields(logrus.Fields{
								"function":   "getExpectedTypeForShape",
								"returnType": returnType.Ident,
							}).Debug("[DEBUG] Return type is compatible")
							return returnType
						}
					}
				}
			}
		}
	}

	// No compatible named type found, return nil to indicate hash-based type should be used
	t.log.WithFields(logrus.Fields{
		"function": "getExpectedTypeForShape",
	}).Debug("[DEBUG] No compatible named type found, will use hash-based type")
	return nil
}

// ShapeContext provides context information for determining the expected type of a shape literal
type ShapeContext struct {
	// ExpectedType is the explicitly provided expected type
	ExpectedType *ast.TypeNode
	// VariableName is the name of the variable being assigned (for assignment context)
	VariableName string
	// FunctionName is the name of the function (for function call or return context)
	FunctionName string
	// ParameterIndex is the index of the parameter (for function call context)
	ParameterIndex int
	// ReturnIndex is the index of the return value (for return context)
	ReturnIndex int
}
