package typechecker

import (
	"strings"

	"forst/internal/ast"

	"go/types"
)

func (tc *TypeChecker) resolveAssignmentRValueTypes(assign ast.AssignmentNode) ([][]ast.TypeNode, error) {
	if len(assign.RValues) == 1 && len(assign.LValues) >= 2 {
		if fc, ok := assign.RValues[0].(ast.FunctionCallNode); ok {
			if resolvedTypes, ok, err := tc.tryResolveNValueGoCall(assign, fc); ok {
				return resolvedTypes, err
			} else if err != nil {
				return nil, err
			}
		}
		if mc, ok := assign.RValues[0].(ast.MethodCallNode); ok {
			if resolvedTypes, ok, err := tc.tryResolveNValueGoMethodCall(assign, mc); ok {
				return resolvedTypes, err
			} else if err != nil {
				return nil, err
			}
		}
	}
	return tc.inferAssignmentRValueTypes(assign)
}

func (tc *TypeChecker) tryResolveNValueGoCall(assign ast.AssignmentNode, fc ast.FunctionCallNode) ([][]ast.TypeNode, bool, error) {
	parts := strings.Split(string(fc.Function.ID), ".")
	if len(parts) == 2 {
		if gp := tc.goPackageForImportLocal(parts[0]); gp != nil {
			return tc.tryGoQualifiedNValueCall(assign, fc, gp, parts[0], parts[1])
		}
		if resolvedTypes, ok, err := tc.tryLocalVariableNValueMethodCall(assign, fc, parts[0], parts[1]); ok || err != nil {
			return resolvedTypes, ok, err
		}
	}
	if len(parts) == 1 && len(tc.dotImportPkgs) > 0 {
		sp := fc.Function.Span
		if !sp.IsSet() {
			sp = fc.CallSpan
		}
		gp, err := tc.lookupDotImportFunc(parts[0], sp)
		if err != nil {
			return nil, false, err
		}
		if gp != nil {
			return tc.tryGoQualifiedNValueCall(assign, fc, gp, gp.Path(), parts[0])
		}
	}
	if len(parts) == 1 {
		return tc.trySamePackageNValueCall(assign, fc, parts[0])
	}
	return nil, false, nil
}

func (tc *TypeChecker) tryLocalVariableNValueMethodCall(assign ast.AssignmentNode, fc ast.FunctionCallNode, recvName, methodName string) ([][]ast.TypeNode, bool, error) {
	goRecv := tc.goTypeForVariableIdent(ast.Identifier(recvName))
	if goRecv == nil {
		return nil, false, nil
	}
	argTypes, err := tc.inferGoCallArgTypes(fc)
	if err != nil {
		return nil, false, err
	}
	method := ast.Ident{ID: ast.Identifier(methodName), Span: fc.Function.Span}
	raw, err := tc.checkGoMethodCall(goRecv, method, fc, argTypes, false)
	if err != nil || len(raw) != len(assign.LValues) {
		return nil, false, err
	}
	resolvedTypes := make([][]ast.TypeNode, len(raw))
	for i := range raw {
		resolvedTypes[i] = []ast.TypeNode{raw[i]}
	}
	tc.storeInferredType(fc, raw)
	return resolvedTypes, true, nil
}

func (tc *TypeChecker) inferGoCallArgTypes(fc ast.FunctionCallNode) ([][]ast.TypeNode, error) {
	argTypes := make([][]ast.TypeNode, 0, len(fc.Arguments))
	for _, arg := range fc.Arguments {
		ts, err := tc.inferExpressionType(arg)
		if err != nil {
			return nil, err
		}
		argTypes = append(argTypes, ts)
	}
	return argTypes, nil
}

func (tc *TypeChecker) tryGoQualifiedNValueCall(assign ast.AssignmentNode, fc ast.FunctionCallNode, gp *types.Package, pkgLocal, funcName string) ([][]ast.TypeNode, bool, error) {
	argTypes, err := tc.inferGoCallArgTypes(fc)
	if err != nil {
		return nil, false, err
	}
	raw, err := tc.checkGoQualifiedCall(gp, pkgLocal, funcName, fc, argTypes, false)
	if err != nil || len(raw) != len(assign.LValues) {
		return nil, false, err
	}
	resolvedTypes := make([][]ast.TypeNode, len(raw))
	for i := range raw {
		resolvedTypes[i] = []ast.TypeNode{raw[i]}
	}
	tc.storeInferredType(fc, raw)
	return resolvedTypes, true, nil
}

func (tc *TypeChecker) trySamePackageNValueCall(assign ast.AssignmentNode, fc ast.FunctionCallNode, funcName string) ([][]ast.TypeNode, bool, error) {
	argTypes, err := tc.inferGoCallArgTypes(fc)
	if err != nil {
		return nil, false, err
	}
	raw, found, err := tc.trySamePackageGoCall(funcName, fc, argTypes, false)
	if err != nil || !found || len(raw) != len(assign.LValues) {
		return nil, false, err
	}
	resolvedTypes := make([][]ast.TypeNode, len(raw))
	for i := range raw {
		resolvedTypes[i] = []ast.TypeNode{raw[i]}
	}
	tc.storeInferredType(fc, raw)
	return resolvedTypes, true, nil
}

func (tc *TypeChecker) tryResolveNValueGoMethodCall(assign ast.AssignmentNode, mc ast.MethodCallNode) ([][]ast.TypeNode, bool, error) {
	goRecv, addr := tc.goTypeInfoForExpression(mc.Receiver)
	if goRecv == nil {
		return nil, false, nil
	}
	fc := ast.FunctionCallNode{Arguments: mc.Arguments, CallSpan: mc.CallSpan, ArgSpans: mc.ArgSpans}
	argTypes, err := tc.inferGoCallArgTypes(fc)
	if err != nil {
		return nil, false, err
	}
	raw, err := tc.checkGoMethodCallAddr(goRecv, addr, mc.Method, fc, argTypes, false)
	if err != nil || len(raw) != len(assign.LValues) {
		return nil, false, err
	}
	resolvedTypes := make([][]ast.TypeNode, len(raw))
	for i := range raw {
		resolvedTypes[i] = []ast.TypeNode{raw[i]}
	}
	tc.storeInferredType(mc, raw)
	return resolvedTypes, true, nil
}

func (tc *TypeChecker) inferAssignmentRValueTypes(assign ast.AssignmentNode) ([][]ast.TypeNode, error) {
	resolvedTypes := make([][]ast.TypeNode, 0, len(assign.RValues))
	for i, rvalue := range assign.RValues {
		var expected *ast.TypeNode
		if len(assign.ExplicitTypes) > i && assign.ExplicitTypes[i] != nil {
			expected = assign.ExplicitTypes[i]
		}
		var types []ast.TypeNode
		var err error
		if expected != nil {
			types, err = tc.inferExpressionTypeWithExpected(rvalue, expected)
		} else {
			types, err = tc.inferExpressionType(rvalue)
		}
		if err != nil {
			return nil, err
		}
		resolvedTypes = append(resolvedTypes, types)
	}
	return resolvedTypes, nil
}
