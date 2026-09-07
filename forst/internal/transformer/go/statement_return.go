package transformergo

import (
	"fmt"
	"forst/internal/ast"
	goast "go/ast"
)

func (t *Transformer) transformReturnStatement(s ast.ReturnNode) (goast.Stmt, error) {
	if _, _, err := t.returnStmtFunctionContext(); err != nil {
		return nil, err
	}
	expectedReturnTypes, functionName, err := t.enclosingReturnTypesForReturn(s)
	if err != nil {
		return nil, err
	}
	expectedReturnTypes = t.expandTupleExpectedReturnTypes(expectedReturnTypes, s)
	if ret, ok, err := t.maybeSingleResultReturnStmt(s, expectedReturnTypes); ok {
		return ret, err
	}
	results, err := t.transformReturnValues(s, expectedReturnTypes, functionName)
	if err != nil {
		return nil, err
	}
	results, err = t.padMissingReturnValues(s, expectedReturnTypes, results)
	if err != nil {
		return nil, err
	}
	return &goast.ReturnStmt{Results: results}, nil
}

func (t *Transformer) returnStmtFunctionContext() (string, bool, error) {
	fnNode, err := t.closestFunction()
	if err != nil {
		return "", false, fmt.Errorf("could not find enclosing function for ReturnNode: %w", err)
	}
	switch fn := fnNode.(type) {
	case ast.FunctionNode:
		name := string(fn.Ident.ID)
		return name, t.functionsWithEnsure[name], nil
	case ast.FunctionLiteralNode:
		return "_lit", false, nil
	case *ast.FunctionNode:
		if fn == nil {
			return "", false, fmt.Errorf("enclosing node is not a FunctionNode or FunctionLiteralNode: %T", fnNode)
		}
		name := string(fn.Ident.ID)
		return name, t.functionsWithEnsure[name], nil
	case *ast.FunctionLiteralNode:
		return "_lit", false, nil
	default:
		return "", false, fmt.Errorf("enclosing node is not a FunctionNode or FunctionLiteralNode: %T", fnNode)
	}
}

func (t *Transformer) enclosingReturnTypesForReturn(s ast.ReturnNode) ([]ast.TypeNode, string, error) {
	fnNode, err := t.closestFunction()
	if err != nil {
		return nil, "", fmt.Errorf("could not find enclosing function for transformReturnStatement: %w", err)
	}
	return t.enclosingReturnTypes(fnNode)
}

func (t *Transformer) expandTupleExpectedReturnTypes(expected []ast.TypeNode, s ast.ReturnNode) []ast.TypeNode {
	if len(expected) != 1 || !expected[0].IsTupleType() || len(s.Values) <= 1 {
		return expected
	}
	tup := expected[0]
	if len(s.Values) == len(tup.TypeParams) {
		return tup.TypeParams
	}
	return expected
}

func (t *Transformer) maybeSingleResultReturnStmt(s ast.ReturnNode, expected []ast.TypeNode) (*goast.ReturnStmt, bool, error) {
	if len(s.Values) != 1 || len(expected) != 1 || !expected[0].IsResultType() {
		return nil, false, nil
	}
	if t.returnValueDelegatesWholeResult(s.Values[0]) {
		return nil, false, nil
	}
	if len(expected[0].TypeParams) < 1 {
		return nil, false, nil
	}
	successType := expected[0].TypeParams[0]
	if successType.Ident == ast.TypeVoid {
		// Result(Void, F) lowers to a single error return; success is bare `return nil`.
		return &goast.ReturnStmt{Results: []goast.Expr{goast.NewIdent("nil")}}, true, nil
	}

	fnName, _, _ := t.returnStmtFunctionContext()
	if expr, ok, err := t.tryWrapReturnValueInNamedStruct(fnName, 0, &successType, s.Values[0]); ok {
		if err != nil {
			return nil, true, err
		}
		return &goast.ReturnStmt{Results: []goast.Expr{expr, goast.NewIdent("nil")}}, true, nil
	}

	succExpr, err := t.transformExpression(s.Values[0])
	if err != nil {
		return nil, true, err
	}
	return &goast.ReturnStmt{Results: []goast.Expr{succExpr, goast.NewIdent("nil")}}, true, nil
}

func (t *Transformer) transformReturnValues(s ast.ReturnNode, expectedReturnTypes []ast.TypeNode, functionName string) ([]goast.Expr, error) {
	results := make([]goast.Expr, len(s.Values))
	for i, value := range s.Values {
		var expectedType *ast.TypeNode
		if i < len(expectedReturnTypes) {
			expectedType = &expectedReturnTypes[i]
		}
		if expr, ok, err := t.tryWrapReturnValueInNamedStruct(functionName, i, expectedType, value); ok {
			if err != nil {
				return nil, err
			}
			results[i] = expr
			continue
		}
		valueExpr, err := t.transformReturnValueExpr(i, value, expectedReturnTypes, results)
		if err != nil {
			return nil, err
		}
		results[i] = valueExpr
	}
	return results, nil
}

func (t *Transformer) tryWrapReturnValueInNamedStruct(functionName string, i int, expectedType *ast.TypeNode, value ast.ExpressionNode) (goast.Expr, bool, error) {
	if expectedType == nil || !expectedType.IsUserDefined() || expectedType.IsTypeParam() {
		return nil, false, nil
	}
	switch v := value.(type) {
	case ast.VariableNode:
		// If the variable is already the expected named type, emit it directly.
		// Rebuilding a zero composite (legacy wrap) drops prior field mutations.
		if t.variableAlreadyNamedReturnType(v, expectedType) {
			expr, err := t.transformExpression(v)
			if err != nil {
				return nil, true, err
			}
			return expr, true, nil
		}
		expr, err := t.wrapVariableInNamedStruct(expectedType, v)
		if err != nil {
			return nil, true, fmt.Errorf("transformReturnStatement: %w", err)
		}
		return expr, true, nil
	case ast.ShapeNode, *ast.ShapeNode:
		shapeNode, ok := getShapeNode(value)
		if !ok {
			return nil, false, nil
		}
		expr, err := t.transformShapeNodeWithExpectedType(shapeNode, expectedType, nil)
		if err != nil {
			return nil, true, fmt.Errorf("transformReturnStatement: failed to transform shape node: %w", err)
		}
		return expr, true, nil
	default:
		return nil, false, nil
	}
}

func (t *Transformer) variableAlreadyNamedReturnType(v ast.VariableNode, expectedType *ast.TypeNode) bool {
	if t == nil || t.TypeChecker == nil || expectedType == nil {
		return false
	}
	if occ, ok := t.TypeChecker.InferredTypesForVariableNode(v); ok && len(occ) == 1 {
		return namedReturnTypesIdentical(occ[0], *expectedType)
	}
	if ty, err := t.TypeChecker.LookupVariableType(&v, t.currentScope()); err == nil && ty.Ident != "" {
		return namedReturnTypesIdentical(ty, *expectedType)
	}
	return false
}

// namedReturnTypesIdentical requires the same type ident so distinct same-shaped
// named types do not skip conversion wrapping.
func namedReturnTypesIdentical(actual, expected ast.TypeNode) bool {
	return actual.Ident != "" && actual.Ident == expected.Ident
}

func (t *Transformer) transformReturnValueExpr(i int, value ast.ExpressionNode, expectedReturnTypes []ast.TypeNode, results []goast.Expr) (goast.Expr, error) {
	if i >= len(expectedReturnTypes) {
		return t.transformExpression(value)
	}
	expectedType := &expectedReturnTypes[i]
	if expectedType.TypeKind == ast.TypeKindUserDefined {
		return t.transformUserDefinedReturnValue(i, value, expectedType, results)
	}
	return t.transformNonUserDefinedReturnValue(i, value, expectedType)
}

func (t *Transformer) transformUserDefinedReturnValue(i int, value ast.ExpressionNode, expectedType *ast.TypeNode, results []goast.Expr) (goast.Expr, error) {
	for j, ret := range results {
		if ident, ok := ret.(*goast.Ident); ok {
			results[j] = t.buildCompositeLiteralForReturn(expectedType, ident.Name)
		}
	}
	if results[i] != nil {
		return results[i], nil
	}
	return t.transformExpression(value)
}

func (t *Transformer) transformNonUserDefinedReturnValue(i int, value ast.ExpressionNode, expectedType *ast.TypeNode) (goast.Expr, error) {
	shapeValue, ok := value.(ast.ShapeNode)
	if !ok {
		return t.transformExpression(value)
	}
	context := &ShapeContext{ExpectedType: expectedType, ReturnIndex: i}
	fnNode, err := t.closestFunction()
	if err == nil {
		if fn, ok := fnNode.(ast.FunctionNode); ok {
			context.FunctionName = string(fn.Ident.ID)
		}
	}
	expectedTypeForShape := t.getExpectedTypeForShape(&shapeValue, context)
	useType := t.findBestNamedTypeForReturnStructLiteral(shapeValue, expectedTypeForShape)
	return t.transformShapeNodeWithExpectedType(&shapeValue, useType, nil)
}

func (t *Transformer) padMissingReturnValues(s ast.ReturnNode, expectedReturnTypes []ast.TypeNode, results []goast.Expr) ([]goast.Expr, error) {
	if len(expectedReturnTypes) <= len(results) || t.shouldSkipNilReturnPadding(s, expectedReturnTypes) {
		return results, nil
	}
	for i := len(results); i < len(expectedReturnTypes); i++ {
		expectedType := expectedReturnTypes[i]
		if expectedType.IsError() {
			results = append(results, goast.NewIdent("nil"))
			continue
		}
		zv, err := t.zeroValueExprForASTType(expectedType)
		if err != nil {
			return nil, fmt.Errorf("transformReturnStatement: zero value for %s: %w", expectedType.Ident, err)
		}
		results = append(results, zv)
	}
	return results, nil
}

func (t *Transformer) shouldSkipNilReturnPadding(s ast.ReturnNode, expectedReturnTypes []ast.TypeNode) bool {
	if len(s.Values) != 1 {
		return false
	}
	fc, ok := s.Values[0].(ast.FunctionCallNode)
	if !ok {
		return false
	}
	calleeSig, ok := t.TypeChecker.Functions[fc.Function.ID]
	return ok && len(calleeSig.ReturnTypes) == len(expectedReturnTypes)
}
