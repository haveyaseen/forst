package typechecker

import (
	"fmt"
	"forst/internal/ast"
)

func (tc *TypeChecker) inferFunctionReturnType(fn ast.FunctionNode) ([]ast.TypeNode, error) {
	parsedType := fn.ReturnTypes
	inferredType := []ast.TypeNode{}

	if len(fn.Body) == 0 {
		return ensureMatching(tc, fn, inferredType, parsedType, "Empty function is not void")
	}

	hasEnsure := functionBodyHasEnsure(fn.Body)
	returnStmtTypes, err := tc.collectFunctionReturnStmtTypes(fn, parsedType, hasEnsure)
	if err != nil {
		return nil, err
	}

	if err := tc.checkReturnStmtTypeConsistency(fn, parsedType, returnStmtTypes); err != nil {
		return nil, err
	}

	lastStmt := fn.Body[len(fn.Body)-1]
	if expr, ok := lastStmt.(ast.ExpressionNode); ok {
		// Bare `return` established void: still type the trailing expression. Allow only when
		// it is also void (e.g. println); reject a reachable non-void implicit return value.
		if len(returnStmtTypes) > 0 && IsVoidReturnTypes(returnStmtTypes[0]) {
			trailing, err := tc.inferReturnValueTypes(expr)
			if err != nil {
				return nil, err
			}
			if !IsVoidReturnTypes(trailing) {
				return nil, failWithTypeMismatch(fn, trailing, returnStmtTypes[0], "Inconsistent return expression type")
			}
			inferredType := returnStmtTypes[0]
			inferredType, err = tc.applyEnsureReturnInference(fn, parsedType, inferredType, hasEnsure)
			if err != nil {
				return nil, err
			}
			return ensureMatching(tc, fn, inferredType, parsedType, "Invalid return type")
		}
		return tc.inferImplicitReturnExpression(fn, parsedType, returnStmtTypes, expr)
	}

	if len(returnStmtTypes) > 0 {
		if len(parsedType) == 1 && parsedType[0].IsResultType() && len(returnStmtTypes) > 1 {
			inferredType = parsedType
		} else {
			inferredType = returnStmtTypes[0]
		}
	}

	if inferred, err := tc.inferTupleReturnType(fn, parsedType, returnStmtTypes); err != nil {
		return nil, err
	} else if inferred != nil {
		return inferred, nil
	}

	inferredType, err = tc.applyEnsureReturnInference(fn, parsedType, inferredType, hasEnsure)
	if err != nil {
		return nil, err
	}
	if len(inferredType) == 0 {
		inferredType = []ast.TypeNode{{Ident: ast.TypeVoid}}
	}

	return ensureMatching(tc, fn, inferredType, parsedType, "Invalid return type")
}

func functionBodyHasEnsure(body []ast.Node) bool {
	for _, stmt := range body {
		if stmt.Kind() == ast.NodeKindEnsure {
			return true
		}
	}
	return false
}

func (tc *TypeChecker) collectFunctionReturnStmtTypes(fn ast.FunctionNode, parsedType []ast.TypeNode, hasEnsure bool) ([][]ast.TypeNode, error) {
	returnStmtTypes := make([][]ast.TypeNode, 0)
	for _, retStmt := range collectReturnStatements(fn.Body) {
		retTypes, err := tc.inferReturnStatementTypes(fn, parsedType, hasEnsure, returnStmtTypes, retStmt)
		if err != nil {
			return nil, err
		}
		returnStmtTypes = append(returnStmtTypes, retTypes)
	}
	return returnStmtTypes, nil
}

func (tc *TypeChecker) inferReturnStatementTypes(fn ast.FunctionNode, parsedType []ast.TypeNode, hasEnsure bool, priorReturns [][]ast.TypeNode, retStmt ast.ReturnNode) ([]ast.TypeNode, error) {
	if len(retStmt.Values) == 0 {
		return []ast.TypeNode{{Ident: ast.TypeVoid}}, nil
	}
	retTypes := make([]ast.TypeNode, 0)
	for i, value := range retStmt.Values {
		if value.Kind() == ast.NodeKindNilLiteral {
			expectedType := expectedNilReturnType(parsedType, fn.ReturnTypes, priorReturns, hasEnsure, i)
			if isNilableType(tc, expectedType) {
				retTypes = append(retTypes, expectedType)
			} else {
				return nil, fmt.Errorf("'nil' used as return value but expected type is not nilable (got %s)", formatTypeIdentForDiag(expectedType.Ident))
			}
			continue
		}
		retType, err := tc.inferReturnValueTypes(value)
		if err != nil {
			return nil, err
		}
		if len(retType) == 1 {
			tc.logReturnTypeInference(fn, i, value, retType[0])
			retTypes = append(retTypes, retType[0])
		} else if len(retType) > 1 && len(retStmt.Values) == 1 && len(retType) == len(parsedType) {
			tc.logMultiValueReturn(fn, value, retType)
			retTypes = append(retTypes, retType...)
		} else {
			return nil, fmt.Errorf("return value expression must return exactly one type, got %d", len(retType))
		}
	}
	return retTypes, nil
}

func expectedNilReturnType(parsedType, fnReturnTypes []ast.TypeNode, priorReturns [][]ast.TypeNode, hasEnsure bool, i int) ast.TypeNode {
	var expectedType ast.TypeNode
	if len(parsedType) > i {
		expectedType = parsedType[i]
	} else if len(fnReturnTypes) > i {
		expectedType = fnReturnTypes[i]
	} else if len(priorReturns) > 0 && len(priorReturns[0]) > i {
		expectedType = priorReturns[0][i]
	} else if hasEnsure && i == 1 {
		expectedType = ast.TypeNode{Ident: ast.TypeError}
	}
	return expectedType
}

func (tc *TypeChecker) logReturnTypeInference(fn ast.FunctionNode, i int, value ast.ExpressionNode, typ ast.TypeNode) {
	if tc.log == nil {
		return
	}
	tc.log.WithFields(map[string]any{
		"function":     fn.Ident.ID,
		"returnIndex":  i,
		"returnAST":    fmt.Sprintf("%T", value),
		"inferredType": typ.Ident,
	}).Debug("[PINPOINT] Inferred return type for function")
}

func (tc *TypeChecker) logMultiValueReturn(fn ast.FunctionNode, value ast.ExpressionNode, retType []ast.TypeNode) {
	if tc.log == nil {
		return
	}
	tc.log.WithFields(map[string]any{
		"function":      fn.Ident.ID,
		"returnAST":     fmt.Sprintf("%T", value),
		"inferredTypes": formatTypeList(retType),
	}).Debug("[PINPOINT] Multi-value return from single expression")
}

func (tc *TypeChecker) checkReturnStmtTypeConsistency(fn ast.FunctionNode, parsedType []ast.TypeNode, returnStmtTypes [][]ast.TypeNode) error {
	if len(returnStmtTypes) <= 1 {
		return nil
	}
	if len(parsedType) == 1 && parsedType[0].IsResultType() {
		for _, retTypes := range returnStmtTypes {
			if len(retTypes) != 1 {
				return fmt.Errorf("result-returning function expects single-value returns, got %d values", len(retTypes))
			}
			if !tc.isCompatibleResultReturnArm(retTypes[0], parsedType[0]) {
				return failWithTypeMismatch(fn, retTypes, parsedType, "Inconsistent type of return statements")
			}
		}
		return nil
	}
	firstType := returnStmtTypes[0]
	for _, retTypes := range returnStmtTypes[1:] {
		for i, retType := range retTypes {
			if i >= len(firstType) {
				return failWithTypeMismatch(fn, nil, firstType, "Inconsistent type of return statements")
			}
			if !tc.IsTypeCompatible(retType, firstType[i]) {
				return failWithTypeMismatch(fn, nil, firstType, "Inconsistent type of return statements")
			}
		}
	}
	return nil
}

func (tc *TypeChecker) inferImplicitReturnExpression(fn ast.FunctionNode, parsedType []ast.TypeNode, returnStmtTypes [][]ast.TypeNode, expr ast.ExpressionNode) ([]ast.TypeNode, error) {
	exprTypes, err := tc.inferReturnValueTypes(expr)
	if err != nil {
		return nil, err
	}
	if len(returnStmtTypes) > 0 {
		for i, exprType := range exprTypes {
			if i >= len(returnStmtTypes[0]) {
				return nil, failWithTypeMismatch(fn, exprTypes, exprTypes, "Inconsistent return expression type")
			}
			if !tc.IsTypeCompatible(exprType, returnStmtTypes[0][i]) {
				return nil, failWithTypeMismatch(fn, exprTypes, exprTypes, "Inconsistent return expression type")
			}
		}
	}
	return ensureMatching(tc, fn, exprTypes, parsedType, "Invalid return expression type")
}

func (tc *TypeChecker) inferTupleReturnType(fn ast.FunctionNode, parsedType []ast.TypeNode, returnStmtTypes [][]ast.TypeNode) ([]ast.TypeNode, error) {
	if len(parsedType) != 1 || !parsedType[0].IsTupleType() || len(returnStmtTypes) == 0 {
		return nil, nil
	}
	retVals := returnStmtTypes[0]
	tup := parsedType[0]
	if len(retVals) != len(tup.TypeParams) {
		return nil, nil
	}
	for i, elem := range retVals {
		if !tc.IsTypeCompatible(elem, tup.TypeParams[i]) {
			return nil, failWithTypeMismatch(fn, retVals, tup.TypeParams, "Tuple return element mismatch")
		}
	}
	return parsedType, nil
}

func (tc *TypeChecker) applyEnsureReturnInference(fn ast.FunctionNode, parsedType, inferredType []ast.TypeNode, hasEnsure bool) ([]ast.TypeNode, error) {
	if !hasEnsure {
		return inferredType, nil
	}
	if tc.IsGoTestFunction(fn) || isMainFunctionNode(fn) {
		return []ast.TypeNode{{Ident: ast.TypeVoid}}, nil
	}
	if len(inferredType) == 0 {
		if len(parsedType) == 1 && parsedType[0].IsResultType() {
			return parsedType, nil
		}
		if !tc.functionEnsureImpliesResultReturn(fn) {
			return []ast.TypeNode{{Ident: ast.TypeVoid}}, nil
		}
		// Void success + failure path → Result(Void, Error), not bare Error.
		return []ast.TypeNode{ast.NewResultType(
			ast.TypeNode{Ident: ast.TypeVoid},
			ast.TypeNode{Ident: ast.TypeError},
		)}, nil
	}
	if tc.functionEnsureImpliesResultReturn(fn) &&
		(len(parsedType) != 1 || !parsedType[0].IsResultType()) &&
		(len(inferredType) != 1 || !inferredType[0].IsResultType()) {
		return tc.promoteEnsureInferredReturn(inferredType)
	}
	return inferredType, nil
}

func (tc *TypeChecker) promoteEnsureInferredReturn(inferredType []ast.TypeNode) ([]ast.TypeNode, error) {
	if len(inferredType) < 1 || len(inferredType) > 2 {
		return nil, fmt.Errorf("ensure statements require the function to return an error or a tuple with an error as the second type, got %s", formatTypeList(inferredType))
	}
	if len(inferredType) == 1 && inferredType[0].Ident != ast.TypeError {
		return []ast.TypeNode{ast.NewResultType(inferredType[0], ast.TypeNode{Ident: ast.TypeError})}, nil
	}
	if len(inferredType) == 2 && inferredType[len(inferredType)-1].Ident != ast.TypeError {
		inferredType[len(inferredType)-1] = ast.TypeNode{Ident: ast.TypeError}
	}
	return inferredType, nil
}
