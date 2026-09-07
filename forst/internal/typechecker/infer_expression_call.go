package typechecker

import (
	"forst/internal/ast"
	"forst/internal/goload"
	"strings"

	logrus "github.com/sirupsen/logrus"
)

func (tc *TypeChecker) inferExpressionFunctionCall(expr ast.Node) ([]ast.TypeNode, bool, error) {
	e, ok := expr.(ast.FunctionCallNode)
	if !ok {
		return nil, false, nil
	}
	return tc.inferFunctionCallType(e)
}

func (tc *TypeChecker) inferFunctionCallType(e ast.FunctionCallNode) ([]ast.TypeNode, bool, error) {
	if e.Callee != nil {
		ret, err := tc.inferCalleeCall(e.Callee, e.Arguments, e.ArgSpans, e.CallSpan)
		if err != nil {
			return nil, true, err
		}
		tc.storeInferredType(e, ret)
		return ret, true, nil
	}

	tc.log.WithFields(logrus.Fields{
		"function": "inferExpressionType",
		"expr":     e,
	}).Tracef("Checking function call: %s with %d arguments", e.Function.ID, len(e.Arguments))

	argTypes, err := tc.inferFunctionCallArgTypes(e)
	if err != nil {
		return nil, true, err
	}

	if ret, ok, err := tc.inferRegisteredForstFunctionCall(e, argTypes); ok {
		return ret, true, err
	}
	if ret, ok, err := tc.inferUnqualifiedFunctionCall(e, argTypes); ok {
		return ret, true, err
	}
	if ret, ok, err := tc.inferTypeGuardFunctionCall(e); ok {
		return ret, true, err
	}
	if ret, ok, err := tc.inferLocalVariableFunctionCall(e); ok {
		return ret, true, err
	}

	parts := strings.Split(string(e.Function.ID), ".")
	switch len(parts) {
	case 2:
		return tc.inferTwoPartQualifiedFunctionCall(e, parts[0], parts[1], argTypes)
	default:
		if len(parts) >= 3 {
			return tc.inferMultiPartGoMethodCall(e, parts, argTypes)
		}
		return tc.inferUnqualifiedBuiltinOrDotImportCall(e, argTypes)
	}
}

func (tc *TypeChecker) inferFunctionCallArgTypes(e ast.FunctionCallNode) ([][]ast.TypeNode, error) {
	if signature, exists := tc.Functions[e.Function.ID]; exists {
		argTypes := make([][]ast.TypeNode, len(e.Arguments))
		for i, arg := range e.Arguments {
			exp := expectedTypeForCallParam(signature.Parameters, i)
			ts, err := tc.inferExpressionTypeWithExpected(arg, exp)
			if err != nil {
				return nil, err
			}
			argTypes[i] = ts
		}
		return argTypes, nil
	}
	argTypes := make([][]ast.TypeNode, 0, len(e.Arguments))
	for _, arg := range e.Arguments {
		ts, err := tc.inferExpressionType(arg)
		if err != nil {
			return nil, err
		}
		argTypes = append(argTypes, ts)
	}
	return argTypes, nil
}

func (tc *TypeChecker) inferRegisteredForstFunctionCall(e ast.FunctionCallNode, argTypes [][]ast.TypeNode) ([]ast.TypeNode, bool, error) {
	signature, exists := tc.Functions[e.Function.ID]
	if !exists {
		return nil, false, nil
	}
	callSig := signature
	callSpan := e.CallSpan
	if !callSpan.IsSet() {
		callSpan = e.Function.Span
	}
	if len(e.TypeArgs) > 0 && len(signature.TypeParams) == 0 {
		_, err := tc.instantiateGenericCallExplicit(signature, e.TypeArgs, argTypes, callSpan)
		if err != nil {
			return nil, true, err
		}
	}
	if len(signature.TypeParams) > 0 {
		var inst FunctionSignature
		var err error
		if len(e.TypeArgs) > 0 {
			inst, err = tc.instantiateGenericCallExplicit(signature, e.TypeArgs, argTypes, callSpan)
		} else {
			inst, err = tc.instantiateGenericCall(signature, argTypes, callSpan)
		}
		if err != nil {
			return nil, true, err
		}
		callSig = inst
	}
	if err := tc.checkUserFunctionCall(e.Function.ID, callSig, e, argTypes); err != nil {
		return nil, true, err
	}
	tc.recordFunctionCall(e.Function.ID, callSpan)
	retTypes := callSig.ReturnTypes
	tc.storeInferredType(e, retTypes)
	return retTypes, true, nil
}

func (tc *TypeChecker) inferUnqualifiedFunctionCall(e ast.FunctionCallNode, argTypes [][]ast.TypeNode) ([]ast.TypeNode, bool, error) {
	if strings.Contains(string(e.Function.ID), ".") {
		return nil, false, nil
	}
	ret, found, err := tc.trySamePackageGoCall(string(e.Function.ID), e, argTypes, true)
	if err != nil {
		return nil, true, err
	}
	if !found {
		return nil, false, nil
	}
	tc.invalidateAfterUntrustedGoCall(e)
	tc.storeInferredType(e, ret)
	return ret, true, nil
}

func (tc *TypeChecker) inferTypeGuardFunctionCall(e ast.FunctionCallNode) ([]ast.TypeNode, bool, error) {
	typeGuard, exists := tc.scopeStack.globalScope().Symbols[e.Function.ID]
	if !exists || typeGuard.Kind != SymbolTypeGuard {
		return nil, false, nil
	}
	tc.log.WithFields(logrus.Fields{
		"function": "inferExpressionType",
		"expr":     e,
	}).Tracef("Found type guard %s with types: %v", e.Function.ID, typeGuard.Types)
	return []ast.TypeNode{{Ident: ast.TypeBool}}, true, nil
}

func (tc *TypeChecker) inferLocalVariableFunctionCall(e ast.FunctionCallNode) ([]ast.TypeNode, bool, error) {
	varType, exists := tc.scopeStack.LookupVariableType(e.Function.ID)
	if !exists {
		return nil, false, nil
	}
	if len(varType) == 1 && varType[0].IsFunctionType() {
		ret, err := tc.inferCalleeCall(ast.VariableNode{Ident: e.Function}, e.Arguments, e.ArgSpans, e.CallSpan)
		if err != nil {
			return nil, true, err
		}
		tc.storeInferredType(e, ret)
		return ret, true, nil
	}
	tc.log.WithFields(logrus.Fields{
		"function": "inferExpressionType",
		"expr":     e,
	}).Tracef("Found local variable %s with type: %v", e.Function.ID, varType)
	return varType, true, nil
}

func (tc *TypeChecker) inferTwoPartQualifiedFunctionCall(e ast.FunctionCallNode, pkgName, funcName string, argTypes [][]ast.TypeNode) ([]ast.TypeNode, bool, error) {
	if ret, ok, err := tc.inferTwoPartLocalVariableMethodCall(e, pkgName, funcName, argTypes); ok {
		return ret, true, err
	}
	if ret, ok, err := tc.inferTwoPartForstSiblingCall(e, pkgName, funcName, argTypes); ok {
		return ret, true, err
	}
	if ret, ok, err := tc.inferTwoPartNodeQualifiedCall(e, pkgName, funcName, argTypes); ok {
		return ret, true, err
	}
	if ret, ok, err := tc.inferTwoPartGoPackageCall(e, pkgName, funcName, argTypes); ok {
		return ret, true, err
	}
	if ret, ok, err := tc.inferTwoPartQualifiedBuiltinCall(e, pkgName, funcName); ok {
		return ret, true, err
	}
	if ret, ok, err := tc.inferTwoPartGoImportNotLoadedError(e, pkgName); ok {
		return ret, true, err
	}
	return tc.inferFunctionCallUnresolved(e, argTypes)
}

func (tc *TypeChecker) inferTwoPartLocalVariableMethodCall(e ast.FunctionCallNode, pkgName, funcName string, argTypes [][]ast.TypeNode) ([]ast.TypeNode, bool, error) {
	varType, exists := tc.scopeStack.LookupVariableType(ast.Identifier(pkgName))
	if !exists {
		return nil, false, nil
	}
	tc.log.WithFields(logrus.Fields{
		"function": "inferExpressionType",
		"expr":     e,
	}).Tracef("Found local variable %s with type: %v", pkgName, varType)
	returnType, err := tc.inferMethodCallType(ast.Identifier(pkgName), varType, funcName, e, argTypes)
	if err != nil {
		return nil, true, err
	}
	tc.storeInferredType(e, returnType)
	return returnType, true, nil
}

func (tc *TypeChecker) inferTwoPartForstSiblingCall(e ast.FunctionCallNode, pkgName, funcName string, argTypes [][]ast.TypeNode) ([]ast.TypeNode, bool, error) {
	ret, err := tc.resolveForstSiblingCall(pkgName, funcName, e, argTypes)
	if err != nil {
		return nil, true, err
	}
	if ret == nil {
		return nil, false, nil
	}
	tc.storeInferredType(e, ret)
	return ret, true, nil
}

func (tc *TypeChecker) inferTwoPartNodeQualifiedCall(e ast.FunctionCallNode, pkgName, funcName string, argTypes [][]ast.TypeNode) ([]ast.TypeNode, bool, error) {
	ret, found, err := tc.tryNodeQualifiedCall(pkgName, funcName, e, argTypes)
	if err != nil {
		return nil, true, err
	}
	if !found {
		return nil, false, nil
	}
	tc.storeInferredType(e, ret)
	return ret, true, nil
}

func (tc *TypeChecker) inferTwoPartGoPackageCall(e ast.FunctionCallNode, pkgName, funcName string, argTypes [][]ast.TypeNode) ([]ast.TypeNode, bool, error) {
	gp := tc.goPackageForImportLocal(pkgName)
	if gp == nil {
		return nil, false, nil
	}
	ret, err := tc.checkGoQualifiedCall(gp, pkgName, funcName, e, argTypes, true)
	if err != nil {
		return nil, true, err
	}
	callSpan := e.CallSpan
	if !callSpan.IsSet() {
		callSpan = e.Function.Span
	}
	tc.recordCrossPackageCall(pkgName, ast.Identifier(funcName), callSpan)
	tc.invalidateAfterUntrustedGoCall(e)
	tc.storeInferredType(e, ret)
	return ret, true, nil
}

func (tc *TypeChecker) inferTwoPartQualifiedBuiltinCall(e ast.FunctionCallNode, pkgName, funcName string) ([]ast.TypeNode, bool, error) {
	qualifiedName := pkgName + "." + funcName
	builtin, exists := BuiltinFunctions[qualifiedName]
	if !exists {
		return nil, false, nil
	}
	tc.log.WithFields(logrus.Fields{
		"function": "inferExpressionType",
		"expr":     e,
	}).Tracef("Found built-in function %s", qualifiedName)
	returnType, err := tc.checkBuiltinFunctionCall(builtin, e.Arguments, e.ArgSpans, e.CallSpan)
	if err != nil {
		return nil, true, err
	}
	tc.storeInferredType(e, returnType)
	return returnType, true, nil
}

func (tc *TypeChecker) inferTwoPartGoImportNotLoadedError(e ast.FunctionCallNode, pkgName string) ([]ast.TypeNode, bool, error) {
	if !tc.IsImportedLocalName(pkgName) || tc.isNodeImportLocal(pkgName) {
		return nil, false, nil
	}
	callSpan := e.CallSpan
	if !callSpan.IsSet() {
		callSpan = e.Function.Span
	}
	importPath := pkgName
	if tc.importPathByLocal != nil {
		if p, ok := tc.importPathByLocal[pkgName]; ok && p != "" {
			importPath = p
		}
	}
	if loadErr := tc.goImportLoadErrorForPath(importPath); loadErr != nil {
		return nil, true, reportBodyf(callSpan, "go-import", "failed to load Go package %q: %v", importPath, loadErr)
	}
	tc.log.WithFields(logrus.Fields{
		"function":            "inferExpressionType",
		"pkgLocal":            pkgName,
		"importPath":          importPath,
		"goWorkspaceDir":      tc.GoWorkspaceDir,
		"goPackagesPreloaded": tc.goPackagesPreloaded,
		"missingGoImports":    tc.missingGoImportPaths(),
	}).Debug("Go import package types not loaded")
	return nil, true, reportBodyf(callSpan, "go-import", "%s", goload.GoImportTypesNotLoadedMsg(pkgName, importPath, tc.GoWorkspaceDir, tc.NodeBoundaryRoot))
}

func (tc *TypeChecker) inferMultiPartGoMethodCall(e ast.FunctionCallNode, parts []string, argTypes [][]ast.TypeNode) ([]ast.TypeNode, bool, error) {
	base := ast.Identifier(parts[0])
	gt := tc.goTypeForVariableIdent(base)
	if gt == nil {
		return tc.inferUnqualifiedBuiltinOrDotImportCall(e, argTypes)
	}
	fieldPath := parts[1 : len(parts)-1]
	recvGo, err := goTypeAtFieldPath(gt, fieldPath)
	if err != nil {
		return tc.inferUnqualifiedBuiltinOrDotImportCall(e, argTypes)
	}
	fc := ast.FunctionCallNode{
		Function:  e.Function,
		Arguments: e.Arguments,
		CallSpan:  e.CallSpan,
		ArgSpans:  e.ArgSpans,
	}
	ret, err := tc.checkGoMethodCall(recvGo, e.Function, fc, argTypes, true)
	if err != nil {
		return nil, true, err
	}
	tc.storeInferredType(e, ret)
	return ret, true, nil
}

func (tc *TypeChecker) inferUnqualifiedBuiltinOrDotImportCall(e ast.FunctionCallNode, argTypes [][]ast.TypeNode) ([]ast.TypeNode, bool, error) {
	if builtin, exists := BuiltinFunctions[string(e.Function.ID)]; exists {
		tc.log.WithFields(logrus.Fields{
			"function": "inferExpressionType",
			"expr":     e,
		}).Tracef("Found built-in function %s", e.Function.ID)
		returnType, err := tc.checkBuiltinFunctionCall(builtin, e.Arguments, e.ArgSpans, e.CallSpan)
		if err != nil {
			return nil, true, err
		}
		tc.storeInferredType(e, returnType)
		return returnType, true, nil
	}
	spDot := e.Function.Span
	if !spDot.IsSet() {
		spDot = e.CallSpan
	}
	gp, err := tc.lookupDotImportFunc(string(e.Function.ID), spDot)
	if err != nil {
		return nil, true, err
	}
	if gp != nil {
		ret, err := tc.checkGoQualifiedCall(gp, gp.Path(), string(e.Function.ID), e, argTypes, true)
		if err != nil {
			return nil, true, err
		}
		tc.storeInferredType(e, ret)
		return ret, true, nil
	}
	return tc.inferFunctionCallUnresolved(e, argTypes)
}

func (tc *TypeChecker) inferFunctionCallUnresolved(e ast.FunctionCallNode, argTypes [][]ast.TypeNode) ([]ast.TypeNode, bool, error) {
	tc.log.WithFields(logrus.Fields{
		"function": "inferExpressionType",
		"expr":     e,
	}).Tracef("No function found for %s", e.Function.ID)

	if ret, ok, err := tc.inferNominalErrorConstructorCall(e, argTypes); ok {
		if err != nil {
			return nil, true, err
		}
		tc.storeInferredType(e, ret)
		return ret, true, nil
	}
	if ret, ok := tc.treatUnresolvedQualifiedCallAsForeign(e); ok {
		tc.storeInferredType(e, ret)
		return ret, true, nil
	}
	sp := e.Function.Span
	if !sp.IsSet() {
		sp = e.CallSpan
	}
	return nil, true, reportBodyf(sp, "undefined-identifier", "unknown identifier: %s", e.Function.ID)
}
