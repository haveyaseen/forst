package typechecker

import (
	"fmt"
	"strings"

	"forst/internal/ast"
)

// FunctionSignature represents a function's type information
type FunctionSignature struct {
	Ident          ast.Ident
	TypeParams     []ast.TypeParamDecl
	TypeParamNames typeParamSet // names declared on this generic function
	Parameters     []ParameterSignature
	ReturnTypes    []ast.TypeNode
	ErrorSet       FunctionErrorSet
}

// FunctionErrorSet is the inferred set of nominal errors a function may raise at runtime.
type FunctionErrorSet struct {
	NominalErrors   []ast.TypeIdent
	UnknownPossible bool
}

// ParameterSignature represents a function parameter's type information
type ParameterSignature struct {
	Ident    ast.Ident
	Type     ast.TypeNode
	Variadic bool // true for trailing ...T parameters (Type is the element type)
}

// String returns a string representation of the parameter signature
func (p ParameterSignature) String() string {
	if p.Variadic {
		return fmt.Sprintf("%s ...%s", p.Ident.ID, p.Type.String())
	}
	return fmt.Sprintf("%s %s", p.Ident.ID, p.Type.String())
}

// GetIdent returns the parameter identifier as a string
func (p ParameterSignature) GetIdent() string {
	return string(p.Ident.ID)
}

func (f FunctionSignature) String() string {
	paramStrings := make([]string, len(f.Parameters))
	for i, param := range f.Parameters {
		paramStrings[i] = param.String()
	}
	returnStrings := make([]string, len(f.ReturnTypes))
	for i, ret := range f.ReturnTypes {
		returnStrings[i] = ret.String()
	}
	return fmt.Sprintf("%s(%s) -> %s", f.Ident.ID, strings.Join(paramStrings, ", "), strings.Join(returnStrings, ", "))
}

// GetIdent returns the function identifier as a string
func (f FunctionSignature) GetIdent() string {
	return string(f.Ident.ID)
}

// FormatTypeNodeDisplay formats a type for human-facing UI (e.g. LSP hover). It resolves hash-based
// structural types to a named alias in the alias chain when one exists, so hover matches what the
// user wrote when possible.
func (tc *TypeChecker) FormatTypeNodeDisplay(t ast.TypeNode) string {
	if tc == nil {
		return t.String()
	}
	d := tc.GetMostSpecificNonHashAlias(t)
	d = tc.resolveAliasedType(d)
	return d.String()
}

// PredicateChainForVariableHover returns the ordered list of narrowing / assertion predicate names
// for hover: flow-narrowing guards first, then extra user type-guard names from the inferred type
// node (deduplicated, order preserved).
func (tc *TypeChecker) PredicateChainForVariableHover(vn ast.VariableNode, types []ast.TypeNode) []string {
	if tc == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	add := func(name string) {
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	for _, g := range tc.NarrowingTypeGuardsForVariableOccurrence(vn) {
		add(g)
	}
	if len(types) == 1 {
		for _, g := range tc.TypeGuardConstraintNamesForInferredType(types[0]) {
			add(g)
		}
	}
	return out
}

// FormatVariableOccurrenceTypeForHover formats a variable occurrence for LSP hover. The static
// type uses FormatTypeNodeDisplay; assertion predicates (flow + inferred assertion) are appended
// as a dotted call chain, e.g. `String.MyStr().Contains("bla")`.
func (tc *TypeChecker) FormatVariableOccurrenceTypeForHover(vn ast.VariableNode, types []ast.TypeNode) string {
	if tc == nil || len(types) == 0 {
		return ""
	}
	if len(types) > 1 {
		parts := make([]string, len(types))
		for i := range types {
			parts[i] = tc.FormatTypeNodeDisplay(types[i])
		}
		return strings.Join(parts, ", ")
	}
	// When the variable comes from a Go FFI binding, show the real go/types string (e.g. *enmime.Envelope)
	// instead of lossy Forst mapping like Pointer((implicit)).
	if goStr, ok := tc.goTypeDisplayStringForVariable(vn.Ident.ID, vn.Ident.Span); ok {
		chain := tc.PredicateChainForVariableHover(vn, types)
		astSuffix := tc.NarrowingPredicateDisplayForVariableOccurrence(vn)
		if len(chain) == 0 && astSuffix == "" {
			return goStr
		}
	}
	base := tc.FormatTypeNodeDisplay(types[0])
	chain := tc.PredicateChainForVariableHover(vn, types)
	if len(chain) == 0 {
		return base
	}
	flowNames := tc.NarrowingTypeGuardsForVariableOccurrence(vn)
	flowSet := make(map[string]struct{}, len(flowNames))
	for _, g := range flowNames {
		flowSet[g] = struct{}{}
	}
	astSuffix := tc.NarrowingPredicateDisplayForVariableOccurrence(vn)

	var b strings.Builder
	b.WriteString(base)
	if astSuffix != "" {
		b.WriteString(".")
		b.WriteString(astSuffix)
		for _, g := range chain {
			if _, ok := flowSet[g]; ok {
				continue
			}
			b.WriteString(".")
			b.WriteString(g)
			b.WriteString("()")
		}
		return b.String()
	}
	b.WriteString(".")
	for i, g := range chain {
		if i > 0 {
			b.WriteString(".")
		}
		b.WriteString(g)
		b.WriteString("()")
	}
	return b.String()
}

// FormatFunctionSignatureDisplay formats a function signature using FormatTypeNodeDisplay for every
// parameter and return type. Multiple return values (including value + Error) are shown as a
// parenthesized tuple, e.g. (String, Error).
func (tc *TypeChecker) FormatFunctionSignatureDisplay(sig FunctionSignature) string {
	paramStrings := make([]string, len(sig.Parameters))
	for i, param := range sig.Parameters {
		paramStrings[i] = fmt.Sprintf("%s %s", param.Ident.ID, tc.FormatTypeNodeDisplay(param.Type))
	}
	retParts := make([]string, len(sig.ReturnTypes))
	for i, ret := range sig.ReturnTypes {
		retParts[i] = tc.FormatTypeNodeDisplay(ret)
	}
	var retStr string
	switch len(retParts) {
	case 0:
		retStr = "Void"
	case 1:
		retStr = retParts[0]
	default:
		retStr = "(" + strings.Join(retParts, ", ") + ")"
	}
	return fmt.Sprintf("%s(%s) -> %s", sig.Ident.ID, strings.Join(paramStrings, ", "), retStr)
}

// InferredTypesForVariableNode returns inferred types for this variable occurrence. The structural
// hash includes the identifier and, when set, its source span—so distinct occurrences of the same
// name (e.g. under `if x is …` narrowing) can have different entries in tc.Types. Callers that only
// have a lexer token should build VariableNode{Ident: {ID, Span: SpanFromToken(tok)}} to match the
// parser (e.g. `ensure` subject identifiers in internal/parser/ensure.go). Falls back to
// InferredTypesForVariableIdentifier when the node has no span.
func (tc *TypeChecker) InferredTypesForVariableNode(vn ast.VariableNode) ([]ast.TypeNode, bool) {
	if tc == nil {
		return nil, false
	}
	if vn.Ident.Span.IsSet() {
		k := variableOccurrenceKey{ident: vn.Ident.ID, span: vn.Ident.Span}
		if t, ok := tc.variableOccurrenceTypes[k]; ok && len(t) > 0 {
			return t, true
		}
	}
	hash, err := tc.Hasher.HashNode(vn)
	if err == nil {
		if t, ok := tc.Types[hash]; ok && len(t) > 0 {
			return t, true
		}
	}
	if !vn.Ident.Span.IsSet() {
		return tc.InferredTypesForVariableIdentifier(vn.Ident.ID)
	}
	return tc.InferredTypesForVariableIdentifier(vn.Ident.ID)
}

// InferredTypesForVariableIdentifier returns inferred types for a bare identifier. Types come from
// tc.Types (VariableNode keys include the identifier and, when present, source span) with a fallback
// to VariableTypes after declarations. Identifiers that are also function or type names should be
// handled by the caller first (e.g. prefer function signature hover for func f() { ... }'s name).
func (tc *TypeChecker) InferredTypesForVariableIdentifier(id ast.Identifier) ([]ast.TypeNode, bool) {
	if tc == nil {
		return nil, false
	}
	vn := ast.VariableNode{Ident: ast.Ident{ID: id}}
	hash, err := tc.Hasher.HashNode(vn)
	if err == nil {
		if t, ok := tc.Types[hash]; ok && len(t) > 0 {
			return t, true
		}
	}
	if t, ok := tc.VariableTypes[id]; ok && len(t) > 0 {
		return t, true
	}
	return nil, false
}
