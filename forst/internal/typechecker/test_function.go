package typechecker

import (
	"strings"

	"forst/internal/ast"

	"go/types"
)

// IsGoTestFunction reports Test* functions with a single *testing.T parameter.
func (tc *TypeChecker) IsGoTestFunction(fn ast.FunctionNode) bool {
	if !strings.HasPrefix(string(fn.Ident.ID), "Test") {
		return false
	}
	paramTypes := ast.ParamTypesFromFunction(fn)
	if len(paramTypes) != 1 {
		return false
	}
	if sig, ok := tc.Functions[fn.Ident.ID]; ok && len(sig.Parameters) == 1 {
		if ast.IsTestingTParamType(sig.Parameters[0].Type) {
			return true
		}
	}
	return ast.IsTestingTParamType(paramTypes[0])
}

// GoTypeForVariable returns the go/types binding for a local when known.
// Prefer GoTypeForVariableNode when an occurrence span is available so shadowed
// names (e.g. Test* param t vs a string local t) resolve correctly.
func (tc *TypeChecker) GoTypeForVariable(ident ast.Identifier) types.Type {
	return tc.goTypeForVariableIdent(ident)
}

// GoTypeForVariableNode returns the go/types binding for a specific variable occurrence.
func (tc *TypeChecker) GoTypeForVariableNode(vn ast.VariableNode) types.Type {
	return tc.goTypeForVariableNode(vn)
}

// IsGoTypesTestingT reports whether gt is *testing.T.
func IsGoTypesTestingT(gt types.Type) bool {
	if gt == nil {
		return false
	}
	ptr, ok := gt.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok {
		return false
	}
	pkg := named.Obj().Pkg()
	return pkg != nil && pkg.Path() == "testing" && named.Obj().Name() == "T"
}
