// Package transformerts converts a Forst AST to TypeScript declaration files
package transformerts

import (
	"fmt"
	"forst/internal/ast"
	"forst/internal/typechecker"
	"regexp"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
)

// tsExportIdentPattern matches generated TypeScript export names ($Foo, $T_abc…).
var tsExportIdentPattern = regexp.MustCompile(`\$[A-Za-z_][A-Za-z0-9_]*`)

// TypeScriptTransformer converts a Forst AST to TypeScript declaration files
type TypeScriptTransformer struct {
	TypeChecker *typechecker.TypeChecker
	Output      *TypeScriptOutput
	log         *logrus.Logger
	typeMapping forstTypeMapper
	// GenerateStreamingClients emits *Stream helpers delegating to invokeStream when row types are known (chan T with typable T).
	GenerateStreamingClients bool
}

// New creates a new TypeScriptTransformer
func New(tc *typechecker.TypeChecker, log *logrus.Logger) *TypeScriptTransformer {
	if log == nil {
		log = logrus.New()
		log.Warnf("No logger provided, using default logger")
	}

	transformer := &TypeScriptTransformer{
		TypeChecker: tc,
		Output:      &TypeScriptOutput{},
		log:         log,
		typeMapping: NewTypeMapping(),
	}

	// Set the typechecker in the type mapping for hash-based type resolution
	transformer.typeMapping.SetTypeChecker(tc)

	return transformer
}

// TransformForstFileToTypeScript converts a Forst AST to TypeScript files.
// sourceFileStem should be the .ft file basename without extension (e.g. "api" for "api.ft");
// pass "" to use PackageName as the TypeScript export name (tests and callers without a file path).
func (t *TypeScriptTransformer) TransformForstFileToTypeScript(nodes []ast.Node, sourceFileStem string) (*TypeScriptOutput, error) {
	t.Output.SourceFileStem = sourceFileStem

	// Build type mapping first
	t.buildTypeMapping()

	// Process user-named definitions first. Hash-based structural types are
	// emitted later only if signatures or named types reference them.
	for _, def := range t.TypeChecker.Defs {
		switch def := def.(type) {
		case ast.TypeDefNode:
			if t.TypeChecker.IsHashBasedIdent(def.Ident) {
				t.log.WithFields(logrus.Fields{
					"typeDef":  def.GetIdent(),
					"function": "TransformForstFileToTypeScript",
				}).Debug("Deferring hash-based type until referenced")
				continue
			}
			t.log.WithFields(logrus.Fields{
				"typeDef":  def.GetIdent(),
				"function": "TransformForstFileToTypeScript",
			}).Debug("Processing type definition")
			if _, ok := def.Expr.(ast.TypeDefErrorExpr); ok {
				typeName := string(def.Ident)
				t.typeMapping.AddUserType(typeName, GeneratedTypeExport(typeName))
				if cls, err := DomainErrorClassFromTypeDef(def, t.TypeChecker); err == nil {
					t.Output.DomainErrors = append(t.Output.DomainErrors, cls)
				}
				continue
			}
			tsType, err := t.transformTypeDef(def)
			if err != nil {
				return nil, fmt.Errorf("failed to transform type def %s: %w", def.GetIdent(), err)
			}
			t.Output.AddType(tsType)
			t.Output.AddExportedTypeName(GeneratedTypeExport(string(def.GetIdent())))
		}
	}

	// Then process the rest of the nodes
	for _, node := range nodes {
		switch n := node.(type) {
		case ast.PackageNode:
			t.Output.SetPackageName(string(n.Ident.ID))
		case ast.FunctionNode:
			reason, emit := ProviderOmissionReason(n, t.TypeChecker)
			if !emit {
				if reason != "" {
					pkg := t.Output.PackageName
					if pkg == "" {
						pkg = sourceFileStem
					}
					omitted := OmittedFunction{
						PackageName:  pkg,
						FunctionName: string(n.Ident.ID),
						Reason:       reason,
					}
					// Capture a declaration for optional omitStubs comments (SPEC §12).
					if result, err := t.transformFunction(n); err == nil && result != nil && result.Signature != nil {
						omitted.ExportDecl = result.Signature.ToString()
					}
					t.Output.OmittedFunctions = append(t.Output.OmittedFunctions, omitted)
				}
				continue
			}
			funcResult, err := t.transformFunction(n)
			if err != nil {
				return nil, fmt.Errorf("failed to transform function %s: %w", n.GetIdent(), err)
			}
			// Add signature to functions list
			t.Output.AddFunction(*funcResult.Signature)
		}
	}

	if err := t.emitReferencedHashTypes(); err != nil {
		return nil, err
	}

	// Generate the new client structure
	t.generateClientStructure()

	if err := StampDomainErrorPackages(t.Output); err != nil {
		return nil, err
	}

	t.log.WithFields(logrus.Fields{
		"function": "TransformForstFileToTypeScript",
		"types":    len(t.Output.Types),
		"funcs":    len(t.Output.Functions),
	}).Debug("Generated TypeScript client structure")

	return t.Output, nil
}

// emitReferencedHashTypes emits hash-based Defs entries that appear in
// already-emitted named types or function signatures ($T_…).
func (t *TypeScriptTransformer) emitReferencedHashTypes() error {
	emitted := make(map[string]bool, len(t.Output.ExportedTypeNames))
	for _, name := range t.Output.ExportedTypeNames {
		emitted[name] = true
	}

	for {
		needed := collectReferencedTSExports(t.Output)
		var toEmit []ast.TypeIdent
		for ident, def := range t.TypeChecker.Defs {
			if !t.TypeChecker.IsHashBasedIdent(ident) {
				continue
			}
			if _, ok := def.(ast.TypeDefNode); !ok {
				continue
			}
			exportName := GeneratedTypeExport(string(ident))
			if emitted[exportName] {
				continue
			}
			if !needed[exportName] {
				continue
			}
			toEmit = append(toEmit, ident)
		}
		if len(toEmit) == 0 {
			return nil
		}
		sort.Slice(toEmit, func(i, j int) bool { return toEmit[i] < toEmit[j] })
		for _, ident := range toEmit {
			def := t.TypeChecker.Defs[ident].(ast.TypeDefNode)
			tsType, err := t.transformTypeDef(def)
			if err != nil {
				return fmt.Errorf("failed to transform referenced hash type %s: %w", ident, err)
			}
			t.Output.AddType(tsType)
			exportName := GeneratedTypeExport(string(ident))
			t.Output.AddExportedTypeName(exportName)
			emitted[exportName] = true
			t.log.WithFields(logrus.Fields{
				"typeDef":  ident,
				"function": "emitReferencedHashTypes",
			}).Debug("Emitted referenced hash-based type")
		}
	}
}

// collectReferencedTSExports gathers $ExportName identifiers from type bodies
// and function signatures so hash types can be emitted on demand.
func collectReferencedTSExports(out *TypeScriptOutput) map[string]bool {
	needed := make(map[string]bool)
	if out == nil {
		return needed
	}
	var buf strings.Builder
	for _, typ := range out.Types {
		buf.WriteString(typ)
		buf.WriteByte('\n')
	}
	for _, fn := range out.Functions {
		buf.WriteString(fn.ReturnType)
		buf.WriteByte('\n')
		buf.WriteString(fn.StreamingRowType)
		buf.WriteByte('\n')
		buf.WriteString(fn.FailureType)
		buf.WriteByte('\n')
		for _, p := range fn.Parameters {
			buf.WriteString(p.Type)
			buf.WriteByte('\n')
		}
	}
	for _, name := range out.ExportedTypeNames {
		buf.WriteString(name)
		buf.WriteByte('\n')
	}
	for _, match := range tsExportIdentPattern.FindAllString(buf.String(), -1) {
		needed[match] = true
	}
	return needed
}

// buildTypeMapping creates a mapping from Forst types to TypeScript types
func (t *TypeScriptTransformer) buildTypeMapping() {
	// User types will be added as we process type definitions
	t.log.WithFields(logrus.Fields{
		"function": "buildTypeMapping",
	}).Debug("Built type mapping")
}
