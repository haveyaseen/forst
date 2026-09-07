package typechecker

import (
	"forst/internal/ast"
	"forst/internal/bridgeinterop"
	"forst/internal/hasher"
	"go/types"

	"github.com/sirupsen/logrus"
)

// compoundNarrowingInfo holds predicate metadata for dotted identifiers (e.g. req.state after ensure).
type compoundNarrowingInfo struct {
	guards []string
	disp   string
}

// TypeChecker performs type inference and type checking on the AST
type TypeChecker struct {
	// Maps structural hashes of AST nodes to their inferred or declared types
	Types map[NodeHash][]ast.TypeNode
	// Maps type identifiers to their definition nodes
	Defs map[ast.TypeIdent]ast.Node
	// Maps type identifiers to nodes where they are referenced
	Uses map[ast.TypeIdent][]ast.Node
	// Maps function identifiers to their parameter and return type signatures
	Functions  map[ast.Identifier]FunctionSignature
	Hasher     *hasher.StructuralHasher
	path       NodePath // Tracks current position while traversing AST
	scopeStack *ScopeStack
	// Map of inferred types for nodes
	InferredTypes map[NodeHash][]ast.TypeNode
	// Map of inferred variable types
	VariableTypes map[ast.Identifier][]ast.TypeNode
	// Per-occurrence inferred types when the parser set Ident.Span (hover / narrowing).
	variableOccurrenceTypes map[variableOccurrenceKey][]ast.TypeNode
	// Per-occurrence type guard names from if-branch / ensure narrowing (when InferAssertionType
	// preserves a named alias without Assertion on the TypeNode).
	variableOccurrenceNarrowingGuards map[variableOccurrenceKey][]string
	// Per-occurrence dotted predicate display from narrowing RHS (e.g. `MyStr().Min(12)`), for LSP hover.
	variableOccurrenceNarrowingPredicateDisplay map[variableOccurrenceKey]string
	// compoundNarrowingByIdentifier stores narrowing for dotted ensure/if subjects (e.g. req.state) by
	// identifier only. Per-span maps miss when the same path appears later with a different span.
	compoundNarrowingByIdentifier map[ast.Identifier]compoundNarrowingInfo
	// Map of inferred function return types
	FunctionReturnTypes map[ast.Identifier][]ast.TypeNode
	// List of imported packages
	imports []ast.ImportNode
	// nodeImports lists opted-in TypeScript imports (stored separately from Go imports).
	nodeImports []ast.ImportNode
	// nodeImportsByLocal maps import local name (e.g. payment) to resolved TS module + index.
	nodeImportsByLocal map[string]nodeImportBinding
	// nodeIndexResolver holds in-memory forst-index-v1 data for JS imports.
	nodeIndexResolver *bridgeinterop.IndexResolver
	// NodeBoundaryRoot is the project root for resolving TS import paths (defaults to GoWorkspaceDir).
	NodeBoundaryRoot string
	// ForstFileDir is the directory containing the Forst source file (for relative TS imports).
	ForstFileDir string
	// NodeImportPolicy is ftconfig node.importPolicy ("explicit" or "implicit").
	NodeImportPolicy string
	// GoWorkspaceDir is the directory used as go/packages Config.Dir for Forst <-> Go boundary checks (optional).
	GoWorkspaceDir string
	// goPkgsByLocal maps Forst import local name (e.g. fmt, bar) to loaded *types.Package for Forst <-> Go boundary checks (optional).
	goPkgsByLocal map[string]*types.Package
	// dotImportPkgs lists packages imported with Go dot-import (import . "path"). Used to resolve unqualified calls like NewReader.
	dotImportPkgs []*types.Package
	// goImportLoadErrors records go/packages load failures keyed by import path.
	goImportLoadErrors map[string]error
	// importPathByLocal maps import local identifier -> Go import path (for hover even when go/packages failed).
	importPathByLocal map[string]string
	// Logger for the type checker
	log *logrus.Logger
	// Whether to report phases
	reportPhases bool
	// loopDepth counts nested for-loop bodies for break/continue validation
	loopDepth int
	// switchDepth counts nested switch statements for fallthrough validation
	switchDepth int
	// loopLabelStack records labels of nested for-loops (innermost last) for labeled break/continue
	loopLabelStack []ast.Identifier
	// LabelScopes holds per-function label bindings after checkFunctionLabels (for LSP).
	LabelScopes   []LabelScope
	labelScopeSeq int
	// ifChainNarrowingStack records per-if-chain narrowing events (`x is …`) for merge/join (narrow_if.go).
	ifChainNarrowingStack [][]narrowingEvent
	// currentFunction is set while inferring a function body (Ok/Err need Result(S,F) from the signature).
	currentFunction *ast.FunctionNode
	// resultErrIfBranchDepth counts nested `if subject is Err(...)` then-bodies (built-in Result
	// narrowing). Failure propagation with `return Err(...)` there is rejected; use `ensure` instead.
	resultErrIfBranchDepth int
	// shapeExpectations maps a missing named parameter type (not in Defs) to the shape inferred for
	// a shape literal that was checked with that contextual type. Enables IsTypeCompatible(hash, T)
	// when T was intentionally absent from Defs but inferShapeType still produced a concrete shape.
	shapeExpectations map[ast.TypeIdent]ast.ShapeNode
	// variableGoTypes maps locals assigned from Go qualified calls to the corresponding go/types result types.
	// Used to type-check method calls against real Go method signatures instead of opaque TYPE_IMPLICIT.
	// Ident-keyed only; last writer wins. Prefer variableGoTypesBySymbol / ByOccurrence for scoped lookup
	// so Test*(t *testing.T) does not steal the name t from other functions.
	variableGoTypes map[ast.Identifier]types.Type
	// variableGoTypesBySymbol maps a scope SymbolID to its Go FFI type (unique across shadowing).
	variableGoTypesBySymbol map[SymbolID]types.Type
	// variableGoTypesByOccurrence maps a source occurrence (ident+span) to its Go FFI type.
	variableGoTypesByOccurrence map[variableOccurrenceKey]types.Type
	// TypeMethods maps receiver type ident -> method name -> signature (Forst receiver methods).
	TypeMethods map[ast.TypeIdent]map[string]FunctionSignature
	// goQualifiedTypeAliases maps Forst type ident -> Go qualified type name (e.g. io.Writer).
	goQualifiedTypeAliases map[ast.TypeIdent]string
	// samePackageGoImportPath is the Go import path for the Forst package directory (mixed .go + .ft).
	samePackageGoImportPath string
	// samePackageGo holds go/types for exported Go symbols in the same directory as this Forst package.
	samePackageGo *types.Package
	// goPackageTypeIdents tracks Forst type idents registered from same-package Go (skip Go emit).
	goPackageTypeIdents map[ast.TypeIdent]struct{}
	// FunctionProviders holds inferred Provider slots per function after fixed-point propagation.
	FunctionProviders map[ast.Identifier][]ProviderSlot
	// providers holds Providers inference state (cleared/rebuilt each CheckTypes pass).
	providers *ProvidersEngine
	// moduleResult is set during module-level check for cross-package Forst call resolution.
	moduleResult ModuleResultView
	// siblingTypeDefCache memoizes resolveForstSiblingTypeDef (hit and miss).
	siblingTypeDefCache map[ast.TypeIdent]cachedSiblingTypeDef
	// siblingImportTypeDefCache memoizes resolveForstSiblingTypeInImports (hit and miss).
	siblingImportTypeDefCache map[string]cachedSiblingTypeDef
	// shapeAliasIndex lazily maps structural shape / assertion hashes to user type names.
	shapeAliasIndex *shapeAliasIndex
	// hashBasedIdents tracks types registered through hash-based provenance (not user T_ names).
	hashBasedIdents map[ast.TypeIdent]struct{}
	// compatMemo caches IsTypeCompatible results for a single CheckTypes pass.
	compatMemo map[compatKey]bool
	// goPackagesPreloaded skips go/packages load in InferTypes when set by InitGoPackagesFromBatch.
	goPackagesPreloaded bool
	Warnings            []Diagnostic
	// scopeOwners maps declaration idents to the ScopeNode registered at collect (for transform restore).
	scopeOwners scopeOwners
	// typecheckNodes is the nodes slice from the last CheckTypes call (scope identity for transform).
	typecheckNodes []ast.Node
	// packageConsts tracks top-level const names (reject reassignment).
	packageConsts map[ast.Identifier]struct{}
	// bridgeRuntime holds compile-time bridge interop facts (needsBridgeRuntime, manifest JSON).
	bridgeRuntime BridgeRuntimeInfo

	// paths interns AccessPath values (phase 2a).
	paths *PathInterner
	// predicates interns canonical Predicate values (phase 2c).
	predicates *PredicateInterner
	// refinementCtx is the active program-point fact context (phase 2d); distinct from Scope.
	refinementCtx *RefinementContext
	// refinementFacts records facts with dependency paths (phase 4a).
	refinementFacts []RefinementFact
	// guardDepsCache caches relative dep steps per named type guard.
	guardDepsCache map[string]AccessPaths
	// droppedFacts records facts removed by writes (phase 4b diagnostics).
	droppedFacts []droppedFact
	// writeCollectorStack collects writes in if/loop bodies for join/backedge invalidation.
	writeCollectorStack [][]collectedWrite
	// functionSummaries maps function id → inferred effect summary (phase 4c).
	functionSummaries map[ast.Identifier]*FunctionSummary
	// currentInferFn / currentInferParams track the function body being inferred for summaries.
	currentInferFn     ast.Identifier
	currentInferParams []ast.Identifier
	// aliasCtx owns may-alias / points-to state (phase 4d).
	aliasCtx *AliasContext
	// closureCaptures maps a local holding a function literal → capture write paths (phase 4g).
	closureCaptures map[ast.Identifier][]*AccessPath
	// capturingClosure, when true, records outer writes into pendingClosureWrites instead of invalidating.
	capturingClosure     bool
	pendingClosureWrites []*AccessPath

	ensureIR        map[string]ensureIRRecord
	guardBodyIR     map[ast.Identifier]Assertion
	ifIsIR          []Assertion
	lastEnsureIR    ensureIRRecord
	lastGuardBodyIR Assertion
	lastIfIsIR      Assertion
}

// New creates a new TypeChecker.
func New(log *logrus.Logger, reportPhases bool) *TypeChecker {
	if log == nil {
		log = logrus.New()
		log.Warnf("No logger provided, using default logger")
	}
	h := hasher.New()
	tc := &TypeChecker{
		Types:                             make(map[NodeHash][]ast.TypeNode),
		Defs:                              make(map[ast.TypeIdent]ast.Node),
		Uses:                              make(map[ast.TypeIdent][]ast.Node),
		Functions:                         make(map[ast.Identifier]FunctionSignature),
		Hasher:                            h,
		path:                              make(NodePath, 0),
		scopeStack:                        NewScopeStack(h, log),
		InferredTypes:                     make(map[NodeHash][]ast.TypeNode),
		VariableTypes:                     make(map[ast.Identifier][]ast.TypeNode),
		variableOccurrenceTypes:           make(map[variableOccurrenceKey][]ast.TypeNode),
		variableOccurrenceNarrowingGuards: make(map[variableOccurrenceKey][]string),
		variableOccurrenceNarrowingPredicateDisplay: make(map[variableOccurrenceKey]string),
		compoundNarrowingByIdentifier:               make(map[ast.Identifier]compoundNarrowingInfo),
		FunctionReturnTypes:                         make(map[ast.Identifier][]ast.TypeNode),
		variableGoTypes:                             make(map[ast.Identifier]types.Type),
		variableGoTypesBySymbol:                     make(map[SymbolID]types.Type),
		variableGoTypesByOccurrence:                 make(map[variableOccurrenceKey]types.Type),
		log:                                         log,
		reportPhases:                                reportPhases,
		scopeOwners:                                 newScopeOwners(),
		hashBasedIdents:                             make(map[ast.TypeIdent]struct{}),
		paths:                                       NewPathInterner(),
		predicates:                                  NewPredicateInterner(),
		refinementCtx:                               NewRefinementContext(),
		aliasCtx:                                    newAliasContext(),
		closureCaptures:                             make(map[ast.Identifier][]*AccessPath),
	}

	return tc
}

// GoImportPackageLoaded reports whether go/packages successfully loaded the given import local name
// (e.g. "strconv", "fmt") for Forst↔Go boundary typing. When false, qualified calls may fall back to builtins only.
func (tc *TypeChecker) GoImportPackageLoaded(local string) bool {
	return tc.goPkgsByLocal != nil && tc.goPkgsByLocal[local] != nil
}

// HasDotImportPackages reports whether go/packages loaded at least one dot-imported package (import . "path").
func (tc *TypeChecker) HasDotImportPackages() bool {
	for _, pkg := range tc.dotImportPkgs {
		if pkg != nil {
			return true
		}
	}
	return false
}

// SamePackageGoLoaded reports whether same-package Go interop loaded via SetSamePackageGoImportPath.
func (tc *TypeChecker) SamePackageGoLoaded() bool {
	return tc.samePackageGo != nil
}

// CheckTypes performs type inference in two passes:
// 1. Collects explicit type declarations and function signatures
// 2. Infers types for expressions and statements
func (tc *TypeChecker) CheckTypes(nodes []ast.Node) error {
	tc.typecheckNodes = nodes
	tc.shapeAliasIndex = nil
	tc.compatMemo = nil
	tc.imports = nil
	tc.nodeImports = nil
	tc.LabelScopes = nil
	tc.labelScopeSeq = 0
	if tc.nodeImportsByLocal != nil {
		for k := range tc.nodeImportsByLocal {
			delete(tc.nodeImportsByLocal, k)
		}
	}
	if err := tc.CollectTypes(nodes); err != nil {
		return err
	}
	if err := tc.resolveNodeImports(); err != nil {
		return err
	}
	if err := tc.preloadGoImportPackages(); err != nil {
		return err
	}
	return tc.InferTypes(nodes)
}

// ResolveNodeImportsAfterCollect resolves opted-in TypeScript imports after CollectTypes.
func (tc *TypeChecker) ResolveNodeImportsAfterCollect() error {
	return tc.resolveNodeImports()
}

// preloadGoImportPackages batch-loads Go packages for import lines collected in CollectTypes.
// LSP and single-file CheckTypes use the same path as module-wide typechecking so qualified
// calls like exec.Command resolve when go/packages is available.
func (tc *TypeChecker) preloadGoImportPackages() error {
	loaded, err := BatchLoadGoPackagesForModule(tc.goPackagesLoadDir(), []*TypeChecker{tc})
	if err != nil {
		tc.log.WithFields(logrus.Fields{
			"function": "preloadGoImportPackages",
			"dir":      tc.goPackagesLoadDir(),
		}).WithError(err).Debug("go/packages batch load failed; Forst↔Go boundary checks use lazy load")
	}
	tc.RecordUnloadedGoImportPaths(loaded, err)
	if err := tc.InitGoPackagesFromBatch(loaded); err != nil {
		return err
	}
	return tc.validateGoImportLocalsAfterLoad(loaded)
}

// TypecheckNodes returns the nodes slice from the last CheckTypes call.
func (tc *TypeChecker) TypecheckNodes() []ast.Node {
	return tc.typecheckNodes
}

// CollectTypes runs the first pass: explicit types, imports, and function signatures.
func (tc *TypeChecker) CollectTypes(nodes []ast.Node) error {
	tc.resetScopeOwners()
	tc.packageConsts = nil
	if tc.reportPhases {
		tc.log.WithFields(logrus.Fields{
			"function": "CollectTypes",
		}).Info("First pass: collecting explicit types and function signatures")
	}

	collectOrder := partitionTopLevelForCollect(nodes)
	for _, node := range collectOrder {
		tc.path = append(tc.path, node)
		if err := tc.collectExplicitTypes(node); err != nil {
			return err
		}
		tc.path = tc.path[:len(tc.path)-1]
	}
	return nil
}

// InferTypes runs the second pass after CollectTypes (local or module-wide).
func (tc *TypeChecker) InferTypes(nodes []ast.Node) error {
	tc.initGoImportPackages()
	if err := tc.initSamePackageGoExports(); err != nil {
		return err
	}

	if err := tc.validateReferencedTypesAfterCollect(); err != nil {
		return err
	}

	tc.log.WithFields(logrus.Fields{
		"imports":   len(tc.imports),
		"typeDefs":  len(tc.Defs),
		"functions": len(tc.Functions),
		"uses":      len(tc.Uses),
		"function":  "InferTypes",
	}).Debug("Collected types and function signatures")

	if tc.reportPhases {
		tc.log.WithFields(logrus.Fields{
			"function": "InferTypes",
		}).Info("Starting second pass: inferring types")
	}

	tc.initProvidersInference()
	tc.seedKnownProviderRootsFromTypes()

	for _, node := range nodes {
		tc.path = append(tc.path, node)
		if _, err := tc.inferNodeType(node); err != nil {
			return err
		}
		tc.path = tc.path[:len(tc.path)-1]
	}

	if err := tc.finishProvidersChecking(nodes); err != nil {
		return err
	}

	tc.checkShapeStructTags()

	tc.inferAllFunctionErrorSets(nodes)

	return nil
}
