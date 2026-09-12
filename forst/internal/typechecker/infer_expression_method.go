package typechecker

import (
	"strings"

	"forst/internal/ast"
)

func (tc *TypeChecker) inferExpressionMethodCall(expr ast.Node) ([]ast.TypeNode, bool, error) {
	switch e := expr.(type) {
	case ast.MethodCallNode:

		argTypes := make([][]ast.TypeNode, 0, len(e.Arguments))
		for _, arg := range e.Arguments {
			ts, err := tc.inferExpressionType(arg)
			if err != nil {
				return nil, true, err
			}
			argTypes = append(argTypes, ts)
		}
		if ret, ok, err := tc.inferImportLocalFunctionAsMethodCall(e, argTypes); ok {
			if err != nil {
				return nil, true, err
			}
			tc.storeInferredType(e, ret)
			return ret, true, nil
		}
		if goRecv, addr := tc.goTypeInfoForExpression(e.Receiver); goRecv != nil {
			fc := ast.FunctionCallNode{Arguments: e.Arguments, CallSpan: e.CallSpan, ArgSpans: e.ArgSpans}
			ret, err := tc.checkGoMethodCallAddr(goRecv, addr, e.Method, fc, argTypes, true)
			if err != nil {
				return nil, true, err
			}
			span := e.CallSpan
			if !span.IsSet() {
				span = e.Method.Span
			}
			tc.invalidateReachableMutableArg(e.Receiver, span, dropByForeign)
			tc.storeInferredType(e, ret)
			return ret, true, nil
		}
		recvTypes, err := tc.inferExpressionType(e.Receiver)
		if err != nil {
			return nil, true, err
		}
		recvID := ast.Identifier("")
		fnID := ast.Identifier(string(e.Method.ID))
		if vn, ok := e.Receiver.(ast.VariableNode); ok {
			recvID = vn.Ident.ID
			fnID = ast.Identifier(string(vn.Ident.ID) + "." + string(e.Method.ID))
		}
		fc := ast.FunctionCallNode{
			Function:  ast.Ident{ID: fnID, Span: e.Method.Span},
			Arguments: e.Arguments,
			CallSpan:  e.CallSpan,
			ArgSpans:  e.ArgSpans,
		}
		ret, err := tc.inferMethodCallType(recvID, recvTypes, string(e.Method.ID), fc, argTypes)
		if err != nil {
			return nil, true, err
		}
		tc.storeInferredType(e, ret)
		return ret, true, nil
	}
	return nil, false, nil
}

// inferImportLocalFunctionAsMethodCall types `filepath.IsAbs(path)` when the parser
// emits a MethodCallNode (ensure subjects) instead of a dotted FunctionCallNode.
// Lexical locals and parameters shadow imported package identifiers.
func (tc *TypeChecker) inferImportLocalFunctionAsMethodCall(e ast.MethodCallNode, argTypes [][]ast.TypeNode) ([]ast.TypeNode, bool, error) {
	vn, ok := e.Receiver.(ast.VariableNode)
	if !ok {
		return nil, false, nil
	}
	pkgName := string(vn.Ident.ID)
	if pkgName == "" || strings.Contains(pkgName, ".") {
		return nil, false, nil
	}
	if _, exists := tc.scopeStack.LookupVariableType(ast.Identifier(pkgName)); exists {
		return nil, false, nil
	}
	if !tc.IsImportedLocalName(pkgName) && tc.goPackageForImportLocal(pkgName) == nil {
		return nil, false, nil
	}
	fc := ast.FunctionCallNode{
		Function: ast.Ident{
			ID:   ast.Identifier(pkgName + "." + string(e.Method.ID)),
			Span: e.Method.Span,
		},
		Arguments: e.Arguments,
		CallSpan:  e.CallSpan,
		ArgSpans:  e.ArgSpans,
	}
	if ret, ok, err := tc.inferTwoPartGoPackageCall(fc, pkgName, string(e.Method.ID), argTypes); ok {
		return ret, true, err
	}
	if ret, ok, err := tc.inferTwoPartQualifiedBuiltinCall(fc, pkgName, string(e.Method.ID)); ok {
		return ret, true, err
	}
	return tc.inferTwoPartGoImportNotLoadedError(fc, pkgName)
}
