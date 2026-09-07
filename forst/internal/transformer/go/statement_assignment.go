package transformergo

import (
	"fmt"
	"forst/internal/ast"
	goast "go/ast"
	"go/token"
)

func (t *Transformer) transformFunctionCallStatement(s ast.FunctionCallNode) (goast.Stmt, error) {
	if s.Callee != nil {
		return t.exprStmtFromCalleeCall(s)
	}
	if isPrintLikeBuiltinCall(s.Function) {
		return t.printLikeCallStmt(s)
	}
	if call, ok, err := t.transformNodeQualifiedCall(s); ok {
		if err != nil {
			return nil, err
		}
		return &goast.ExprStmt{X: call}, nil
	}
	return t.namedCallStmt(s)
}

func (t *Transformer) exprStmtFromCalleeCall(s ast.FunctionCallNode) (goast.Stmt, error) {
	funExpr, err := t.transformExpression(s.Callee)
	if err != nil {
		return nil, err
	}
	args, err := t.transformFunctionCallArgs(ast.Identifier("_callee"), s.Arguments)
	if err != nil {
		return nil, err
	}
	return &goast.ExprStmt{X: &goast.CallExpr{Fun: funExpr, Args: args.exprs, Ellipsis: args.ellipsis}}, nil
}

func (t *Transformer) printLikeCallStmt(s ast.FunctionCallNode) (goast.Stmt, error) {
	args, err := t.transformPrintBuiltinCallArgs(s.Arguments)
	if err != nil {
		return nil, err
	}
	return &goast.ExprStmt{X: &goast.CallExpr{Fun: goFunExprFromForstCallIdent(s.Function), Args: args}}, nil
}

func (t *Transformer) namedCallStmt(s ast.FunctionCallNode) (goast.Stmt, error) {
	args, err := t.transformFunctionCallArgs(s.Function.ID, s.Arguments)
	if err != nil {
		return nil, err
	}
	return &goast.ExprStmt{
		X: &goast.CallExpr{Fun: goFunExprFromForstCallIdent(s.Function), Args: args.exprs, Ellipsis: args.ellipsis},
	}, nil
}

func (t *Transformer) transformAssignmentStatement(s ast.AssignmentNode) (goast.Stmt, error) {
	if len(s.ExplicitTypes) > 0 && s.ExplicitTypes[0] != nil {
		return t.transformExplicitTypeAssignment(s)
	}
	if stmt, ok, err := t.tryFoldedTupleOrResultAssignment(s); ok {
		return stmt, err
	}
	return t.transformGeneralAssignment(s)
}

func (t *Transformer) transformExplicitTypeAssignment(s ast.AssignmentNode) (goast.Stmt, error) {
	typeExpr, expectedType := t.explicitAssignmentTypeExpr(s.ExplicitTypes[0])
	vn, vok := s.LValues[0].(ast.VariableNode)
	if !vok {
		return nil, fmt.Errorf("assignment: explicit type requires a simple variable on the left")
	}
	if len(s.RValues) == 0 {
		return varDeclStmt(vn.Ident.String(), typeExpr, nil), nil
	}
	rhs, err := t.explicitAssignmentRHS(s, vn, expectedType)
	if err != nil {
		return nil, err
	}
	return varDeclStmt(vn.Ident.String(), typeExpr, rhs), nil
}

func (t *Transformer) explicitAssignmentRHS(s ast.AssignmentNode, vn ast.VariableNode, expectedType *ast.TypeNode) (goast.Expr, error) {
	if shapeRHS, ok := s.RValues[0].(ast.ShapeNode); ok {
		context := &ShapeContext{ExpectedType: expectedType, VariableName: vn.Ident.String()}
		return t.transformShapeNodeWithExpectedType(&shapeRHS, t.getExpectedTypeForShape(&shapeRHS, context), context)
	}
	return t.transformExpression(s.RValues[0])
}

func (t *Transformer) explicitAssignmentTypeExpr(explicit *ast.TypeNode) (goast.Expr, *ast.TypeNode) {
	if t == nil {
		return goast.NewIdent(string(explicit.Ident)), explicit
	}
	typeIdent, err := t.transformType(*explicit)
	if err != nil {
		return goast.NewIdent(string(explicit.Ident)), explicit
	}
	return typeIdent, explicit
}

func varDeclStmt(name string, typeExpr goast.Expr, value goast.Expr) goast.Stmt {
	spec := &goast.ValueSpec{Names: []*goast.Ident{goast.NewIdent(name)}, Type: typeExpr}
	if value != nil {
		spec.Values = []goast.Expr{value}
	}
	return &goast.DeclStmt{Decl: &goast.GenDecl{Tok: token.VAR, Specs: []goast.Spec{spec}}}
}

func (t *Transformer) tryFoldedTupleOrResultAssignment(s ast.AssignmentNode) (goast.Stmt, bool, error) {
	if len(s.LValues) != 1 || len(s.RValues) != 1 {
		return nil, false, nil
	}
	vn, ok := s.LValues[0].(ast.VariableNode)
	if !ok {
		return nil, false, nil
	}
	if t.rhsExprIsFoldedTuple(s.RValues[0]) {
		stmt, err := t.transformFoldedTupleAssignment(s, vn, s.RValues[0])
		return stmt, true, err
	}
	if t.rhsExprIsFoldedResult(s.RValues[0]) {
		stmt, err := t.transformFoldedResultAssignment(s, vn, s.RValues[0])
		return stmt, true, err
	}
	return nil, false, nil
}

func (t *Transformer) transformFoldedTupleAssignment(s ast.AssignmentNode, vn ast.VariableNode, rhs ast.ExpressionNode) (goast.Stmt, error) {
	ts, err := t.TypeChecker.LookupInferredType(rhs, false)
	if err != nil || len(ts) != 1 || !ts[0].IsTupleType() {
		return nil, fmt.Errorf("assignment: expected Tuple from RHS")
	}
	k := len(ts[0].TypeParams)
	used := collectTupleIndexUses(t.currentFnBody, string(vn.Ident.ID))
	slotNames := make([]string, k)
	for i := range k {
		if used[i] {
			slotNames[i] = fmt.Sprintf("%s%d", string(vn.Ident.ID), i)
		}
	}
	rhsExpr, err := t.transformExpression(rhs)
	if err != nil {
		return nil, err
	}
	if t.resultLocalSplit == nil {
		t.resultLocalSplit = make(map[string]resultLocalSplit)
	}
	t.resultLocalSplit[string(vn.Ident.ID)] = resultLocalSplit{successGoNames: slotNames}
	lhs := make([]goast.Expr, k)
	for i := range k {
		if used[i] {
			lhs[i] = goast.NewIdent(slotNames[i])
		} else {
			lhs[i] = goast.NewIdent("_")
		}
	}
	return &goast.AssignStmt{Lhs: lhs, Tok: assignOpForMultiValueLHS(s.IsShort, lhs), Rhs: []goast.Expr{rhsExpr}}, nil
}

func (t *Transformer) transformFoldedResultAssignment(s ast.AssignmentNode, vn ast.VariableNode, rhs ast.ExpressionNode) (goast.Stmt, error) {
	ts, err := t.TypeChecker.LookupInferredType(rhs, false)
	if err != nil || len(ts) != 1 || !ts[0].IsResultType() {
		return nil, fmt.Errorf("assignment: expected Result from RHS")
	}
	varName := string(vn.Ident.ID)
	rhsExpr, err := t.transformExpression(rhs)
	if err != nil {
		return nil, err
	}
	if t.resultLocalSplit == nil {
		t.resultLocalSplit = make(map[string]resultLocalSplit)
	}

	// Result(Void, F) lowers to a single error value; the Forst name is the error binding.
	if len(ts[0].TypeParams) >= 1 && ts[0].TypeParams[0].Ident == ast.TypeVoid {
		t.resultLocalSplit[varName] = resultLocalSplit{errGoName: varName}
		return &goast.AssignStmt{
			Lhs: []goast.Expr{goast.NewIdent(varName)},
			Tok: assignOpForMultiValueLHS(s.IsShort, []goast.Expr{goast.NewIdent(varName)}),
			Rhs: []goast.Expr{rhsExpr},
		}, nil
	}

	successNames := t.resultSuccessGoNames(ts[0].TypeParams[0], varName)
	errName := varName + "Err"
	errUsed := collectResultErrSlotUsed(t.currentFnBody, varName)
	split := resultLocalSplit{successGoNames: successNames}
	if errUsed {
		split.errGoName = errName
	}
	t.resultLocalSplit[varName] = split
	lhs := t.resultAssignmentLHS(successNames, errUsed, errName)
	return &goast.AssignStmt{Lhs: lhs, Tok: assignOpForMultiValueLHS(s.IsShort, lhs), Rhs: []goast.Expr{rhsExpr}}, nil
}

func (t *Transformer) resultSuccessGoNames(succ ast.TypeNode, varName string) []string {
	if succ.IsTupleType() {
		k := len(succ.TypeParams)
		used := collectTupleIndexUses(t.currentFnBody, varName)
		successNames := make([]string, k)
		for i := range k {
			if used[i] {
				successNames[i] = fmt.Sprintf("%s%d", varName, i)
			}
		}
		return successNames
	}
	if collectResultSuccessValueUsed(t.currentFnBody, varName) {
		return []string{varName}
	}
	return []string{""}
}

func (t *Transformer) resultAssignmentLHS(successNames []string, errUsed bool, errName string) []goast.Expr {
	lhs := make([]goast.Expr, 0, len(successNames)+1)
	for _, n := range successNames {
		if n != "" {
			lhs = append(lhs, goast.NewIdent(n))
		} else {
			lhs = append(lhs, goast.NewIdent("_"))
		}
	}
	if errUsed {
		lhs = append(lhs, goast.NewIdent(errName))
	} else {
		lhs = append(lhs, goast.NewIdent("_"))
	}
	return lhs
}

func (t *Transformer) transformGeneralAssignment(s ast.AssignmentNode) (goast.Stmt, error) {
	lhs, err := t.transformAssignmentLHS(s.LValues)
	if err != nil {
		return nil, err
	}
	rhs, err := t.transformAssignmentRHS(s)
	if err != nil {
		return nil, err
	}
	return &goast.AssignStmt{Lhs: lhs, Tok: t.assignmentOperator(s), Rhs: rhs}, nil
}

func (t *Transformer) transformAssignmentLHS(lvals []ast.ExpressionNode) ([]goast.Expr, error) {
	lhs := make([]goast.Expr, len(lvals))
	for i, lval := range lvals {
		var lhsExpr goast.Expr
		var err error
		if idx, ok := lval.(ast.IndexExpressionNode); ok {
			lhsExpr, err = t.transformIndexAssignTarget(idx)
		} else {
			lhsExpr, err = t.transformExpression(lval)
		}
		if err != nil {
			return nil, err
		}
		lhs[i] = lhsExpr
	}
	return lhs, nil
}

func (t *Transformer) transformAssignmentRHS(s ast.AssignmentNode) ([]goast.Expr, error) {
	rhs := make([]goast.Expr, len(s.RValues))
	for i, rval := range s.RValues {
		if shapeRHS, ok := rval.(ast.ShapeNode); ok && len(s.LValues) == 1 {
			varName := ""
			if vn, vok := s.LValues[0].(ast.VariableNode); vok {
				varName = vn.Ident.String()
			}
			rhsExpr, err := t.transformShapeNodeWithExpectedType(
				&shapeRHS,
				t.getExpectedTypeForShape(&shapeRHS, &ShapeContext{VariableName: varName}),
				nil,
			)
			if err != nil {
				return nil, err
			}
			rhs[i] = rhsExpr
			continue
		}
		rhsExpr, err := t.transformExpression(rval)
		if err != nil {
			return nil, err
		}
		rhs[i] = rhsExpr
	}
	return rhs, nil
}

func (t *Transformer) assignmentOperator(s ast.AssignmentNode) token.Token {
	if s.CompoundOp != "" {
		op, err := assignmentGoToken(s)
		if err == nil {
			return op
		}
	}
	if s.IsShort {
		return token.DEFINE
	}
	return token.ASSIGN
}
