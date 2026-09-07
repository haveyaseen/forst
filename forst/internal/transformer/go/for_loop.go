package transformergo

import (
	"fmt"
	"forst/internal/ast"
	goast "go/ast"
	"go/token"
)

// transformInitPostStmt converts for/if init or post clauses to a single Go statement.
func (t *Transformer) transformInitPostStmt(n ast.Node) (goast.Stmt, error) {
	if u, ok := n.(ast.UnaryExpressionNode); ok &&
		(u.Operator == ast.TokenPlusPlus || u.Operator == ast.TokenMinusMinus) {
		v, ok := u.Operand.(ast.VariableNode)
		if !ok {
			return nil, fmt.Errorf("++/-- only applies to variables")
		}
		tok := token.INC
		if u.Operator == ast.TokenMinusMinus {
			tok = token.DEC
		}
		return &goast.IncDecStmt{
			X:   goast.NewIdent(string(v.Ident.ID)),
			Tok: tok,
		}, nil
	}
	if _, ok := n.(ast.AssignmentNode); ok {
		return t.transformStatement(n)
	}
	if en, ok := n.(ast.ExpressionNode); ok {
		ex, err := t.transformExpression(en)
		if err != nil {
			return nil, err
		}
		return &goast.ExprStmt{X: ex}, nil
	}
	return t.transformStatement(n)
}

func (t *Transformer) transformIfNode(n *ast.IfNode) (goast.Stmt, error) {
	var init goast.Stmt
	if n.Init != nil {
		var err error
		init, err = t.transformInitPostStmt(n.Init)
		if err != nil {
			return nil, err
		}
	}
	ce, ok := n.Condition.(ast.ExpressionNode)
	if !ok {
		return nil, fmt.Errorf("if condition is not an expression")
	}
	condExpr, err := t.transformExpression(ce)
	if err != nil {
		return nil, err
	}
	if err := t.restoreScope(t.resolveIfScopeNode(n)); err != nil {
		return nil, fmt.Errorf("if scope restore: %w", err)
	}
	body := &goast.BlockStmt{}
	if blank := t.blankUnusedResultSuccessForIfCondition(ce); blank != nil {
		body.List = append(body.List, blank)
	}
	for _, st := range n.Body {
		gst, err := t.transformStatement(st)
		if err != nil {
			return nil, err
		}
		if gst != nil {
			body.List = append(body.List, gst)
		}
	}
	main := &goast.IfStmt{Init: init, Cond: condExpr, Body: body}
	cur := main
	for i := range n.ElseIfs {
		ei := &n.ElseIfs[i]
		if err := t.restoreScope(t.resolveElseIfScopeNode(ei)); err != nil {
			return nil, fmt.Errorf("else-if scope restore: %w", err)
		}
		econd, ok := ei.Condition.(ast.ExpressionNode)
		if !ok {
			return nil, fmt.Errorf("else-if condition is not an expression")
		}
		ec, err := t.transformExpression(econd)
		if err != nil {
			return nil, err
		}
		ebody := &goast.BlockStmt{}
		for _, st := range ei.Body {
			gst, err := t.transformStatement(st)
			if err != nil {
				return nil, err
			}
			if gst != nil {
				ebody.List = append(ebody.List, gst)
			}
		}
		next := &goast.IfStmt{Cond: ec, Body: ebody}
		cur.Else = next
		cur = next
	}
	if n.Else != nil {
		if err := t.restoreScope(t.resolveElseBlockScopeNode(n.Else)); err != nil {
			return nil, fmt.Errorf("else scope restore: %w", err)
		}
		eb := &goast.BlockStmt{}
		for _, st := range n.Else.Body {
			gst, err := t.transformStatement(st)
			if err != nil {
				return nil, err
			}
			if gst != nil {
				eb.List = append(eb.List, gst)
			}
		}
		cur.Else = eb
	}
	return main, nil
}

func (t *Transformer) transformForNode(fn *ast.ForNode) (goast.Stmt, error) {
	if fn.IsRange {
		if lowered, ok, err := t.tryTransformNodeIteratorFor(fn); ok {
			return lowered, err
		}
	}
	body := &goast.BlockStmt{}
	for _, st := range fn.Body {
		gst, err := t.transformStatement(st)
		if err != nil {
			return nil, err
		}
		if gst != nil {
			body.List = append(body.List, gst)
		}
	}
	var loop goast.Stmt
	if fn.IsRange {
		rs := &goast.RangeStmt{Body: body}
		if fn.RangeKey != nil {
			rs.Key = goast.NewIdent(string(fn.RangeKey.ID))
		}
		if fn.RangeValue != nil {
			rs.Value = goast.NewIdent(string(fn.RangeValue.ID))
		}
		if fn.RangeKey == nil && fn.RangeValue == nil {
			rs.Tok = token.ILLEGAL
		} else if fn.RangeShort {
			rs.Tok = token.DEFINE
		} else {
			rs.Tok = token.ASSIGN
		}
		xe, err := t.transformExpression(fn.RangeX)
		if err != nil {
			return nil, err
		}
		rs.X = xe
		loop = rs
	} else {
		fs := &goast.ForStmt{Body: body}
		var err error
		if fn.Init != nil {
			fs.Init, err = t.transformInitPostStmt(fn.Init)
			if err != nil {
				return nil, err
			}
		}
		if fn.Cond != nil {
			fs.Cond, err = t.transformExpression(fn.Cond)
			if err != nil {
				return nil, err
			}
		}
		if fn.Post != nil {
			fs.Post, err = t.transformInitPostStmt(fn.Post)
			if err != nil {
				return nil, err
			}
		}
		loop = fs
	}
	if fn.Label != nil {
		return &goast.LabeledStmt{
			Label: goast.NewIdent(string(fn.Label.ID)),
			Stmt:  loop,
		}, nil
	}
	return loop, nil
}
