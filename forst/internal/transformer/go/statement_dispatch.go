package transformergo

import (
	"fmt"
	"forst/internal/ast"
	goast "go/ast"
	"go/token"

	logrus "github.com/sirupsen/logrus"
)

func (t *Transformer) transformStatement(stmt ast.Node) (goast.Stmt, error) {
	if t.log != nil {
		t.log.WithFields(logrus.Fields{
			"function": "transformStatement",
			"stmtType": fmt.Sprintf("%T", stmt),
			"stmt":     stmt.String(),
		}).Debug("[PINPOINT] transformStatement called")
	}
	handlers := []func(ast.Node) (goast.Stmt, bool, error){
		t.transformEnsureReturnStmt,
		t.transformCallAssignStmt,
		t.transformControlStmt,
		t.transformJumpStmt,
		t.transformMiscStmt,
	}
	for _, h := range handlers {
		s, ok, err := h(stmt)
		if !ok {
			continue
		}
		if err != nil {
			return nil, err
		}
		// Handlers must not return a nil statement without an error — that
		// puts typed-nil entries into BlockStmt.List and breaks go/ast.Walk.
		if s == nil {
			return &goast.EmptyStmt{}, nil
		}
		return s, nil
	}
	return &goast.EmptyStmt{}, nil
}

func (t *Transformer) transformEnsureReturnStmt(stmt ast.Node) (goast.Stmt, bool, error) {
	switch s := stmt.(type) {
	case ast.EnsureNode:
		out, err := t.transformEnsureStatement(s, stmt)
		return out, true, err
	case ast.ReturnNode:
		out, err := t.transformReturnStatement(s)
		return out, true, err
	default:
		return nil, false, nil
	}
}

func (t *Transformer) transformCallAssignStmt(stmt ast.Node) (goast.Stmt, bool, error) {
	switch s := stmt.(type) {
	case ast.FunctionCallNode:
		out, err := t.transformFunctionCallStatement(s)
		return out, true, err
	case ast.AssignmentNode:
		out, err := t.transformAssignmentStatement(s)
		return out, true, err
	default:
		return nil, false, nil
	}
}

func (t *Transformer) transformControlStmt(stmt ast.Node) (goast.Stmt, bool, error) {
	switch stmt.(type) {
	case *ast.IfNode:
		out, err := t.transformIfNode(stmt.(*ast.IfNode))
		return out, true, err
	case *ast.SwitchNode:
		out, err := t.transformSwitchNode(stmt.(*ast.SwitchNode))
		return out, true, err
	case *ast.ForNode:
		out, err := t.transformForNode(stmt.(*ast.ForNode))
		return out, true, err
	case ast.WithNode:
		out, err := t.transformWithStatement(stmt, stmt.(ast.WithNode))
		return out, true, err
	default:
		return nil, false, nil
	}
}

func (t *Transformer) transformJumpStmt(stmt ast.Node) (goast.Stmt, bool, error) {
	switch s := stmt.(type) {
	case *ast.GotoNode:
		return t.transformGotoStmt(s)
	case *ast.LabeledStmtNode:
		return t.transformLabeledStmt(s)
	case ast.FallthroughNode:
		return &goast.BranchStmt{Tok: token.FALLTHROUGH}, true, nil
	case *ast.BreakNode:
		return t.transformBreakStmt(s)
	case *ast.ContinueNode:
		return t.transformContinueStmt(s)
	default:
		return nil, false, nil
	}
}

func (t *Transformer) transformGotoStmt(s *ast.GotoNode) (goast.Stmt, bool, error) {
	if s.Label == nil {
		return nil, true, fmt.Errorf("goto requires a label")
	}
	t.stmtLog(logrus.Fields{"stmt": "goto", "label": s.Label.ID}, "Emitting goto BranchStmt")
	return &goast.BranchStmt{Tok: token.GOTO, Label: goast.NewIdent(string(s.Label.ID))}, true, nil
}

func (t *Transformer) transformLabeledStmt(s *ast.LabeledStmtNode) (goast.Stmt, bool, error) {
	if s.Label == nil || s.Stmt == nil {
		return nil, true, fmt.Errorf("labeled statement requires label and body")
	}
	inner, err := t.transformStatement(s.Stmt)
	if err != nil {
		return nil, true, err
	}
	return &goast.LabeledStmt{Label: goast.NewIdent(string(s.Label.ID)), Stmt: inner}, true, nil
}

func (t *Transformer) transformBreakStmt(s *ast.BreakNode) (goast.Stmt, bool, error) {
	bs := &goast.BranchStmt{Tok: token.BREAK}
	if s.Label != nil {
		bs.Label = goast.NewIdent(string(s.Label.ID))
	}
	return bs, true, nil
}

func (t *Transformer) transformContinueStmt(s *ast.ContinueNode) (goast.Stmt, bool, error) {
	cs := &goast.BranchStmt{Tok: token.CONTINUE}
	if s.Label != nil {
		cs.Label = goast.NewIdent(string(s.Label.ID))
	}
	return cs, true, nil
}

func (t *Transformer) transformMiscStmt(stmt ast.Node) (goast.Stmt, bool, error) {
	switch s := stmt.(type) {
	case *ast.DeferNode:
		return t.transformDeferStmt(s)
	case *ast.GoStmtNode:
		return t.transformGoStmt(s)
	case ast.UnaryExpressionNode:
		return t.transformUnaryExprStmt(s)
	case ast.VariableNode:
		return nil, true, fmt.Errorf("transformStatement: VariableNode should not be directly transformed here")
	case ast.CommentNode:
		return &goast.EmptyStmt{}, true, nil
	case ast.UseNode:
		out, err := t.transformUseStatement(s)
		return out, true, err
	default:
		return nil, false, nil
	}
}

func (t *Transformer) transformDeferStmt(s *ast.DeferNode) (goast.Stmt, bool, error) {
	ex, err := t.transformExpression(s.Call)
	if err != nil {
		return nil, true, err
	}
	call, ok := ex.(*goast.CallExpr)
	if !ok {
		return nil, true, fmt.Errorf("defer: internal error, expected call expression, got %T", ex)
	}
	return &goast.DeferStmt{Call: call}, true, nil
}

func (t *Transformer) transformGoStmt(s *ast.GoStmtNode) (goast.Stmt, bool, error) {
	ex, err := t.transformExpression(s.Call)
	if err != nil {
		return nil, true, err
	}
	call, ok := ex.(*goast.CallExpr)
	if !ok {
		return nil, true, fmt.Errorf("go: internal error, expected call expression, got %T", ex)
	}
	return &goast.GoStmt{Call: call}, true, nil
}

func (t *Transformer) transformUnaryExprStmt(s ast.UnaryExpressionNode) (goast.Stmt, bool, error) {
	if s.Operator == ast.TokenPlusPlus || s.Operator == ast.TokenMinusMinus {
		v, ok := s.Operand.(ast.VariableNode)
		if !ok {
			return nil, true, fmt.Errorf("++/-- only applies to variables")
		}
		tok := token.INC
		if s.Operator == ast.TokenMinusMinus {
			tok = token.DEC
		}
		return &goast.IncDecStmt{X: goast.NewIdent(string(v.Ident.ID)), Tok: tok}, true, nil
	}
	ex, err := t.transformExpression(s)
	if err != nil {
		return nil, true, err
	}
	return &goast.ExprStmt{X: ex}, true, nil
}

func (t *Transformer) transformWithStatement(original ast.Node, s ast.WithNode) (goast.Stmt, error) {
	withStmts, err := t.transformWithStatements(original, s)
	if err != nil {
		return nil, err
	}
	if len(withStmts) == 1 {
		return withStmts[0], nil
	}
	return &goast.BlockStmt{List: withStmts}, nil
}
