package typechecker

import (
	"go/types"
	"sort"
	"strings"

	"forst/internal/ast"
)

// VisibleVariableLikeSymbols returns variable and parameter identifiers visible from the
// current scope after RestoreScope, inner scopes shadowing outer names (first occurrence wins).
func (tc *TypeChecker) VisibleVariableLikeSymbols() []ast.Identifier {
	seen := make(map[string]bool)
	var out []ast.Identifier

	scope := tc.CurrentScope()
	for scope != nil {
		for name, sym := range scope.Symbols {
			key := string(name)
			if seen[key] {
				continue
			}
			switch sym.Kind {
			case SymbolVariable:
				seen[key] = true
				out = append(out, name)
			case SymbolParameter:
				if scope.Parent != nil && (scope.IsFunction() || scope.IsTypeGuard()) {
					seen[key] = true
					out = append(out, name)
				}
			default:
				// skip types, functions at file scope, etc.
			}
		}
		scope = scope.Parent
	}

	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	return out
}

// ListFieldNamesForType returns field and method names suitable for member completion after `.`
// for the given type (may be a pointer or assertion-backed type).
func (tc *TypeChecker) ListFieldNamesForType(baseType ast.TypeNode) []string {
	seen := make(map[string]bool)
	var names []string

	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		names = append(names, s)
	}

	t := baseType
	if t.Ident == ast.TypePointer && len(t.TypeParams) > 0 {
		t = t.TypeParams[0]
	}

	t = tc.resolveAliasedType(t)

	if t.IsResultType() && len(t.TypeParams) >= 2 {
		add("Ok")
		add("Err")
		sort.Strings(names)
		return names
	}

	if bt, ok := BuiltinTypes[t.Ident]; ok {
		for m := range bt.Methods {
			add(m)
		}
		sort.Strings(names)
		return names
	}

	def, ok := tc.Defs[t.Ident]
	if !ok {
		sort.Strings(names)
		return names
	}

	switch d := def.(type) {
	case ast.TypeDefNode:
		if payload, ok := ast.PayloadShape(d.Expr); ok {
			for k := range payload.Fields {
				add(k)
			}
			tc.appendPromotedEmbeddedFieldNames(payload, add, seen)
		}
		if assertionExpr, ok := d.Expr.(ast.TypeDefAssertionExpr); ok && assertionExpr.Assertion != nil {
			merged := tc.resolveShapeFieldsFromAssertion(assertionExpr.Assertion)
			for k := range merged {
				add(k)
			}
		}
	case ast.TypeGuardNode:
		for _, param := range d.Parameters() {
			add(param.GetIdent())
		}
	case *ast.TypeGuardNode:
		for _, param := range d.Parameters() {
			add(param.GetIdent())
		}
	}

	if baseType.Assertion != nil {
		merged := tc.resolveShapeFieldsFromAssertion(baseType.Assertion)
		for k := range merged {
			add(k)
		}
	}

	sort.Strings(names)
	return names
}

// appendPromotedEmbeddedFieldNames adds names reachable via Embedded fields, skipping
// names already present (direct fields win) and names promoted from more than one embed
// (ambiguous selectors).
func (tc *TypeChecker) appendPromotedEmbeddedFieldNames(payload *ast.ShapeNode, add func(string), seen map[string]bool) {
	if payload == nil {
		return
	}
	counts := make(map[string]int)
	for _, embedName := range ast.ShapeFieldNamesInOrder(payload.Fields, payload.FieldOrder) {
		field := payload.Fields[embedName]
		if !field.Embedded {
			continue
		}
		innerShape, innerType, err := tc.shapePayloadForEmbeddedField(field)
		if err != nil {
			continue
		}
		var nested []string
		if innerShape != nil {
			nested = tc.listShapeFieldNamesIncludingPromotion(innerShape)
		} else {
			nested = tc.ListFieldNamesForType(innerType)
		}
		for _, n := range nested {
			if seen[n] {
				continue
			}
			counts[n]++
		}
	}
	for n, c := range counts {
		if c == 1 {
			add(n)
		}
	}
}

func (tc *TypeChecker) listShapeFieldNamesIncludingPromotion(payload *ast.ShapeNode) []string {
	seen := make(map[string]bool)
	var names []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		names = append(names, s)
	}
	if payload == nil {
		return names
	}
	for k := range payload.Fields {
		add(k)
	}
	tc.appendPromotedEmbeddedFieldNames(payload, add, seen)
	return names
}

// InferExpressionTypeForCompletion runs expression type inference using the current scope chain.
// Used by the LSP after RestoreScope to resolve member completion types.
func (tc *TypeChecker) InferExpressionTypeForCompletion(expr ast.ExpressionNode) ([]ast.TypeNode, error) {
	return tc.inferExpressionType(expr)
}

// TopLevelPackageVariables returns package-level var names registered in the global scope.
func (tc *TypeChecker) TopLevelPackageVariables() []ast.Identifier {
	if tc == nil {
		return nil
	}
	gs := tc.globalScope()
	if gs == nil {
		return nil
	}
	var out []ast.Identifier
	for name, sym := range gs.Symbols {
		if sym.Kind == SymbolVariable {
			out = append(out, name)
		}
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	return out
}

// IsTopLevelPackageVariable reports whether id is a package-level var in this file.
func (tc *TypeChecker) IsTopLevelPackageVariable(id ast.Identifier) bool {
	for _, v := range tc.TopLevelPackageVariables() {
		if v == id {
			return true
		}
	}
	return false
}

func goMemberNamesForType(t types.Type) []string {
	if t == nil {
		return nil
	}
	seen := make(map[string]bool)
	var names []string
	add := func(s string) {
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		names = append(names, s)
	}
	mset := types.NewMethodSet(t)
	for i := 0; i < mset.Len(); i++ {
		add(mset.At(i).Obj().Name())
	}
	elem := t
	if ptr, ok := t.(*types.Pointer); ok {
		elem = ptr.Elem()
	}
	if st, ok := elem.Underlying().(*types.Struct); ok {
		for i := 0; i < st.NumFields(); i++ {
			f := st.Field(i)
			if f.Exported() {
				add(f.Name())
			}
		}
	}
	sort.Strings(names)
	return names
}

// ListMembersForExpression returns field and method names for member completion after `.`.
func (tc *TypeChecker) ListMembersForExpression(expr ast.ExpressionNode) []string {
	if tc == nil || expr == nil {
		return nil
	}
	if gt := tc.goTypeForExpression(expr); gt != nil {
		return goMemberNamesForType(gt)
	}
	if vn, ok := expr.(ast.VariableNode); ok {
		parts := strings.Split(string(vn.Ident.ID), ".")
		if len(parts) >= 2 {
			if gt := tc.goTypeForVariableIdent(ast.Identifier(parts[0])); gt != nil {
				if goT, err := goTypeAtFieldPath(gt, parts[1:]); err == nil {
					return goMemberNamesForType(goT)
				}
			}
		}
	}
	types, err := tc.inferExpressionType(expr)
	if err != nil || len(types) == 0 {
		return nil
	}
	return tc.ListFieldNamesForType(types[0])
}
