package transformergo

import (
	"strconv"
	"strings"

	"forst/internal/ast"
	"forst/internal/astwalk"
	goast "go/ast"
	"go/token"
)

// walkStmtExpressions visits expression-bearing positions in statement trees (conditions,
// range targets, assign RHS/return/call operands, switch tags, defer/go calls).
func walkStmtExpressions(body []ast.Node, visit func(ast.ExpressionNode)) {
	astwalk.WalkStmts(body, stmtExprVisitor(visit, false))
	walkNestedSwitchExpressions(body, visit)
}

func stmtExprVisitor(visit func(ast.ExpressionNode), includeAssignLValues bool) astwalk.StmtVisitor {
	visitNode := func(n ast.Node) {
		if e, ok := n.(ast.ExpressionNode); ok {
			visit(e)
		}
	}
	return astwalk.StmtVisitor{
		OnAssign: func(a ast.AssignmentNode) bool {
			if includeAssignLValues {
				for _, lv := range a.LValues {
					visitNode(lv)
				}
			}
			for _, rv := range a.RValues {
				visit(rv)
			}
			return true
		},
		OnReturn: func(r ast.ReturnNode) bool {
			for _, v := range r.Values {
				visit(v)
			}
			return true
		},
		OnCall: func(c ast.FunctionCallNode) bool {
			for _, arg := range c.Arguments {
				visit(arg)
			}
			return true
		},
		OnIf: func(node ast.IfNode) bool {
			visitNode(node.Init)
			visitNode(node.Condition)
			for _, elif := range node.ElseIfs {
				visitNode(elif.Condition)
			}
			return true
		},
		OnFor: func(node ast.ForNode) bool {
			visitNode(node.Init)
			if node.Cond != nil {
				visit(node.Cond)
			}
			visitNode(node.Post)
			if node.IsRange && node.RangeX != nil {
				visit(node.RangeX)
			}
			return true
		},
		OnDefer: func(node ast.DeferNode) bool {
			if e, ok := node.Call.(ast.ExpressionNode); ok {
				visit(e)
			}
			return true
		},
		OnGo: func(node ast.GoStmtNode) bool {
			if e, ok := node.Call.(ast.ExpressionNode); ok {
				visit(e)
			}
			return true
		},
	}
}

func walkNestedSwitchExpressions(stmts []ast.Node, visit func(ast.ExpressionNode)) {
	for _, n := range stmts {
		walkNodeSwitchExpressions(n, visit)
	}
}

func walkNodeSwitchExpressions(n ast.Node, visit func(ast.ExpressionNode)) {
	switch node := n.(type) {
	case ast.SwitchNode:
		if node.Init != nil {
			if e, ok := node.Init.(ast.ExpressionNode); ok {
				visit(e)
			}
		}
		if node.Tag != nil {
			visit(node.Tag)
		}
		for _, clause := range node.Clauses {
			for _, v := range clause.Values {
				visit(v)
			}
			walkNestedSwitchExpressions(clause.Body, visit)
		}
	case *ast.SwitchNode:
		walkNodeSwitchExpressions(*node, visit)
	case ast.IfNode:
		walkNestedSwitchExpressions(node.Body, visit)
		for _, elif := range node.ElseIfs {
			walkNestedSwitchExpressions(elif.Body, visit)
		}
		if node.Else != nil {
			walkNestedSwitchExpressions(node.Else.Body, visit)
		}
	case *ast.IfNode:
		walkNodeSwitchExpressions(*node, visit)
	case ast.ForNode:
		walkNestedSwitchExpressions(node.Body, visit)
	case *ast.ForNode:
		walkNodeSwitchExpressions(*node, visit)
	case ast.WithNode:
		walkNestedSwitchExpressions(node.Body, visit)
	case ast.FunctionNode:
		walkNestedSwitchExpressions(node.Body, visit)
	case ast.EnsureNode:
		if node.Block != nil {
			walkNestedSwitchExpressions(node.Block.Body, visit)
		}
	}
}

// collectTupleIndexUses returns numeric tuple field indices accessed as var.N in body.
func collectTupleIndexUses(body []ast.Node, varName string) map[int]bool {
	used := make(map[int]bool)
	walkStmtExpressions(body, func(expr ast.ExpressionNode) {
		walkExpressionTree(expr, func(e ast.ExpressionNode) {
			fa, ok := e.(ast.FieldAccessNode)
			if !ok {
				return
			}
			vn, ok := fa.Target.(ast.VariableNode)
			if !ok || string(vn.Ident.ID) != varName {
				return
			}
			idx, err := strconv.Atoi(string(fa.Field.ID))
			if err != nil || idx < 0 {
				return
			}
			used[idx] = true
		})
	})
	return used
}

func varNameReferenced(node ast.ExpressionNode, varName string) bool {
	switch e := node.(type) {
	case ast.VariableNode:
		id := string(e.Ident.ID)
		return id == varName || strings.HasPrefix(id, varName+".")
	case ast.FieldAccessNode:
		if vn, ok := e.Target.(ast.VariableNode); ok && string(vn.Ident.ID) == varName {
			return true
		}
	}
	return false
}

func walkExpressionTree(expr ast.ExpressionNode, visit func(ast.ExpressionNode)) {
	if expr == nil {
		return
	}
	visit(expr)
	switch e := expr.(type) {
	case ast.BinaryExpressionNode:
		walkExpressionTree(e.Left, visit)
		walkExpressionTree(e.Right, visit)
	case ast.UnaryExpressionNode:
		walkExpressionTree(e.Operand, visit)
	case ast.IndexExpressionNode:
		walkExpressionTree(e.Target, visit)
		walkExpressionTree(e.Index, visit)
	case ast.SliceExpressionNode:
		walkExpressionTree(e.Target, visit)
		if e.Low != nil {
			walkExpressionTree(e.Low, visit)
		}
		if e.High != nil {
			walkExpressionTree(e.High, visit)
		}
	case ast.FieldAccessNode:
		walkExpressionTree(e.Target, visit)
	case ast.MethodCallNode:
		walkExpressionTree(e.Receiver, visit)
		for _, arg := range e.Arguments {
			walkExpressionTree(arg, visit)
		}
	case ast.FunctionCallNode:
		if e.Callee != nil {
			walkExpressionTree(e.Callee, visit)
		}
		for _, arg := range e.Arguments {
			walkExpressionTree(arg, visit)
		}
	case ast.ReferenceNode:
		if inner, ok := e.Value.(ast.ExpressionNode); ok {
			walkExpressionTree(inner, visit)
		}
	case ast.DereferenceNode:
		if inner, ok := e.Value.(ast.ExpressionNode); ok {
			walkExpressionTree(inner, visit)
		}
	case ast.ShapeNode:
		for _, field := range e.Fields {
			if fv, ok := field.ValueExpression(); ok {
				walkExpressionTree(fv, visit)
			}
		}
	case ast.ArrayLiteralNode:
		for _, el := range e.Value {
			walkExpressionTree(el, visit)
		}
	case ast.MapLiteralNode:
		for _, entry := range e.Entries {
			if kv, ok := entry.Key.(ast.ExpressionNode); ok {
				walkExpressionTree(kv, visit)
			}
			if vv, ok := entry.Value.(ast.ExpressionNode); ok {
				walkExpressionTree(vv, visit)
			}
		}
	case ast.OkExprNode:
		walkExpressionTree(e.Value, visit)
	case ast.ErrExprNode:
		walkExpressionTree(e.Value, visit)
	}
}

// collectResultErrSlotUsed reports whether lowering must bind the error slot for a Result local.
func collectResultErrSlotUsed(body []ast.Node, varName string) bool {
	if collectResultErrSlotUsedStmts(body, varName) {
		return true
	}
	return bodyUsesVarInResultExpandingPrint(body, varName)
}

func collectResultErrSlotUsedStmts(stmts []ast.Node, varName string) bool {
	for _, n := range stmts {
		switch node := n.(type) {
		case ast.EnsureNode:
			if place, ok := node.EnsurePlaceSubject(); ok && string(place.Ident.ID) == varName {
				return true
			}
		case ast.IfNode:
			if resultDiscriminatorUsesVar(node.Condition, varName) {
				return true
			}
			if isOkDiscriminator(node.Condition, varName) && node.Else != nil &&
				bodyReferencesVariable(node.Else.Body, varName) {
				return true
			}
			if collectResultErrSlotUsedStmts(node.Body, varName) {
				return true
			}
			for _, elif := range node.ElseIfs {
				if resultDiscriminatorUsesVar(elif.Condition, varName) {
					return true
				}
				if collectResultErrSlotUsedStmts(elif.Body, varName) {
					return true
				}
			}
			if node.Else != nil && collectResultErrSlotUsedStmts(node.Else.Body, varName) {
				return true
			}
		case *ast.IfNode:
			if resultDiscriminatorUsesVar(node.Condition, varName) {
				return true
			}
			if isOkDiscriminator(node.Condition, varName) && node.Else != nil &&
				bodyReferencesVariable(node.Else.Body, varName) {
				return true
			}
			if collectResultErrSlotUsedStmts(node.Body, varName) {
				return true
			}
			for _, elif := range node.ElseIfs {
				if resultDiscriminatorUsesVar(elif.Condition, varName) {
					return true
				}
				if collectResultErrSlotUsedStmts(elif.Body, varName) {
					return true
				}
			}
			if node.Else != nil && collectResultErrSlotUsedStmts(node.Else.Body, varName) {
				return true
			}
		case ast.ForNode, *ast.ForNode:
			var forBody []ast.Node
			switch fn := node.(type) {
			case ast.ForNode:
				forBody = fn.Body
			case *ast.ForNode:
				forBody = fn.Body
			}
			if collectResultErrSlotUsedStmts(forBody, varName) {
				return true
			}
		}
	}
	return false
}

func resultDiscriminatorUsesVar(cond ast.Node, varName string) bool {
	expr, ok := cond.(ast.ExpressionNode)
	if !ok {
		return false
	}
	bin, ok := expr.(ast.BinaryExpressionNode)
	if !ok || bin.Operator != ast.TokenIs {
		return false
	}
	vn, ok := bin.Left.(ast.VariableNode)
	if !ok || string(vn.Ident.ID) != varName {
		return false
	}
	asn, ok := bin.Right.(ast.AssertionNode)
	if !ok {
		return false
	}
	for _, c := range asn.Constraints {
		if c.Name == "Ok" || c.Name == "Err" {
			return true
		}
	}
	return false
}

func isOkDiscriminator(cond ast.Node, varName string) bool {
	expr, ok := cond.(ast.ExpressionNode)
	if !ok {
		return false
	}
	bin, ok := expr.(ast.BinaryExpressionNode)
	if !ok || bin.Operator != ast.TokenIs {
		return false
	}
	vn, ok := bin.Left.(ast.VariableNode)
	if !ok || string(vn.Ident.ID) != varName {
		return false
	}
	asn, ok := bin.Right.(ast.AssertionNode)
	if !ok {
		return false
	}
	for _, c := range asn.Constraints {
		if c.Name == "Ok" {
			return true
		}
	}
	return false
}

func bodyUsesVarInResultExpandingPrint(body []ast.Node, varName string) bool {
	found := false
	astwalk.WalkStmts(body, astwalk.StmtVisitor{
		OnCall: func(c ast.FunctionCallNode) bool {
			if !isPrintLikeBuiltinCall(c.Function) {
				return true
			}
			for _, arg := range c.Arguments {
				if vn, ok := arg.(ast.VariableNode); ok && string(vn.Ident.ID) == varName {
					found = true
					return false
				}
			}
			return true
		},
	})
	return found
}

func bodyReferencesVariable(body []ast.Node, varName string) bool {
	found := false
	walkStmtExpressions(body, func(expr ast.ExpressionNode) {
		walkExpressionTree(expr, func(e ast.ExpressionNode) {
			if vn, ok := e.(ast.VariableNode); ok && string(vn.Ident.ID) == varName {
				found = true
			}
		})
	})
	return found
}

// collectResultSuccessValueUsed reports whether the Result success payload must be bound
// (excludes uses that are only Ok/Err discriminators in if conditions).
func collectResultSuccessValueUsed(body []ast.Node, varName string) bool {
	found := false
	visit := func(expr ast.ExpressionNode) {
		walkExpressionTree(expr, func(e ast.ExpressionNode) {
			if varNameReferenced(e, varName) {
				found = true
			}
		})
	}
	visitNode := func(n ast.Node) {
		if e, ok := n.(ast.ExpressionNode); ok {
			visit(e)
		}
	}
	astwalk.WalkStmts(body, astwalk.StmtVisitor{
		OnAssign: func(a ast.AssignmentNode) bool {
			for _, rv := range a.RValues {
				visit(rv)
			}
			return true
		},
		OnReturn: func(r ast.ReturnNode) bool {
			for _, v := range r.Values {
				visit(v)
			}
			return true
		},
		OnCall: func(c ast.FunctionCallNode) bool {
			for _, arg := range c.Arguments {
				visit(arg)
			}
			return true
		},
		OnEnsure: func(ens ast.EnsureNode) bool {
			// `ensure x is Ok(42)` compares the success payload — keep it bound.
			if place, ok := ens.EnsurePlaceSubject(); ok && string(place.Ident.ID) == varName {
				if ensureAssertionNeedsResultSuccessValue(ens) {
					found = true
				}
			}
			if ens.Error != nil {
				switch err := (*ens.Error).(type) {
				case ast.EnsureErrorCall:
					for _, a := range err.ErrorArgs {
						visit(a)
					}
				case ast.EnsureErrorExpr:
					visit(err.Expr)
				}
			}
			return true
		},
		OnIf: func(node ast.IfNode) bool {
			visitNode(node.Init)
			if !resultDiscriminatorUsesVar(node.Condition, varName) {
				visitNode(node.Condition)
			} else if isResultOkWithValueCompare(node.Condition, varName) {
				found = true
			}
			for _, elif := range node.ElseIfs {
				if !resultDiscriminatorUsesVar(elif.Condition, varName) {
					visitNode(elif.Condition)
				} else if isResultOkWithValueCompare(elif.Condition, varName) {
					found = true
				}
			}
			return true
		},
		OnFor: func(node ast.ForNode) bool {
			visitNode(node.Init)
			if node.Cond != nil {
				visit(node.Cond)
			}
			visitNode(node.Post)
			if node.IsRange && node.RangeX != nil {
				visit(node.RangeX)
			}
			return true
		},
		OnDefer: func(node ast.DeferNode) bool {
			if e, ok := node.Call.(ast.ExpressionNode); ok {
				visit(e)
			}
			return true
		},
		OnGo: func(node ast.GoStmtNode) bool {
			if e, ok := node.Call.(ast.ExpressionNode); ok {
				visit(e)
			}
			return true
		},
	})
	walkNestedSwitchExpressions(body, visit)
	return found
}

// ensureAssertionNeedsResultSuccessValue is true when ensure compares Ok's success payload.
func ensureAssertionNeedsResultSuccessValue(ens ast.EnsureNode) bool {
	if constraintListNeedsOkValue(ens.Assertion.Constraints) {
		return true
	}
	for _, alt := range ens.Assertion.OrChains {
		if constraintListNeedsOkValue(alt.Constraints) {
			return true
		}
	}
	if at, ok := ens.Target.(ast.AssertionTarget); ok {
		for _, chain := range at.Chains {
			if constraintListNeedsOkValue(chain.Constraints) {
				return true
			}
		}
	}
	return false
}

func constraintListNeedsOkValue(cs []ast.ConstraintNode) bool {
	for _, c := range cs {
		if c.Name == "Ok" && len(c.Args) > 0 {
			return true
		}
	}
	return false
}

// isResultOkWithValueCompare reports `x is Ok(value)` style conditions that need the success slot.
func isResultOkWithValueCompare(cond ast.Node, varName string) bool {
	bin, ok := cond.(ast.BinaryExpressionNode)
	if !ok || bin.Operator != ast.TokenIs {
		return false
	}
	left, ok := bin.Left.(ast.VariableNode)
	if !ok || string(left.Ident.ID) != varName {
		return false
	}
	switch r := bin.Right.(type) {
	case ast.AssertionNode:
		return constraintListNeedsOkValue(r.Constraints)
	case *ast.AssertionNode:
		return r != nil && constraintListNeedsOkValue(r.Constraints)
	default:
		return false
	}
}

// collectVariableAnyUse reports whether varName appears in any expression in body.
func collectVariableAnyUse(body []ast.Node, varName string) bool {
	return bodyReferencesVariable(body, varName)
}

// assignOpForMultiValueLHS picks := vs = for multi-value assignment; all-blank LHS must use =.
func assignOpForMultiValueLHS(isShort bool, lhs []goast.Expr) token.Token {
	if !isShort {
		return token.ASSIGN
	}
	for _, l := range lhs {
		if ident, ok := l.(*goast.Ident); !ok || ident.Name == "_" {
			continue
		}
		return token.DEFINE
	}
	return token.ASSIGN
}
