package parser

import (
	"fmt"
	"forst/internal/ast"
	"strings"
)

// ensureMissingIsReport explains that ensure needs `is` plus a constraint.
func ensureMissingIsReport(subject string, found ast.Token) (code, title, problem, help string) {
	help = fmt.Sprintf("for Bool values write:\n\n    ensure %s is True()\n\nor bare:\n\n    ensure %s", subject, subject)
	switch found.Type {
	case ast.TokenOr:
		return "ensure-missing-is", "ensure needs `is` before `or`",
			fmt.Sprintf("Write `ensure %s is A() or B()`. Use `else` for failure.", subject),
			help
	case ast.TokenGreater, ast.TokenLess, ast.TokenGreaterEqual, ast.TokenLessEqual,
		ast.TokenEquals, ast.TokenNotEquals, ast.TokenLogicalOr, ast.TokenLogicalAnd:
		return "ensure-missing-is", "ensure needs `is` with a constraint",
			fmt.Sprintf("Write `ensure %s is Constraint()`, for example `ensure %s is GreaterThan(0)`.", subject, subject),
			help
	case ast.TokenLParen:
		return "ensure-missing-is", "ensure needs `is` with a constraint",
			fmt.Sprintf("Write `ensure %s is Constraint()`, or check a call with `ensure need(ok)`.", subject),
			help
	default:
		return "ensure-missing-is", "ensure needs `is`",
			fmt.Sprintf("After `%s`, write `is` plus a constraint (found %s).", subject, found.Type),
			help
	}
}

func ensureStatementContinuesWithoutIs(tok ast.Token) bool {
	switch tok.Type {
	case ast.TokenElse, ast.TokenRBrace, ast.TokenEOF,
		ast.TokenEnsure, ast.TokenReturn, ast.TokenIf, ast.TokenFor, ast.TokenSwitch,
		ast.TokenDefer, ast.TokenGo, ast.TokenVar, ast.TokenConst, ast.TokenType,
		ast.TokenFunc, ast.TokenBreak, ast.TokenContinue, ast.TokenFallthrough,
		ast.TokenGoto:
		return true
	case ast.TokenIdentifier:
		// Next statement often starts with an identifier (`x := …`, `println(…)`).
		return true
	default:
		return false
	}
}

func (p *Parser) parseEnsureBlock() *ast.EnsureBlockNode {
	body := []ast.Node{}

	// Ensure block is always optional
	if p.current().Type != ast.TokenLBrace {
		return nil
	}

	body = append(body, p.parseBlock()...)

	return &ast.EnsureBlockNode{Body: body}
}

// parseEnsureError parses the typed failure after `else` as a full expression,
// normalizing Ident / Ident(args) to EnsureErrorVar / EnsureErrorCall.
func (p *Parser) parseEnsureError() *ast.EnsureErrorNode {
	expr := p.parseExpression()
	var err ast.EnsureErrorNode
	switch e := expr.(type) {
	case ast.FunctionCallNode:
		if e.Function.ID != "" && !strings.Contains(string(e.Function.ID), ".") {
			err = ast.EnsureErrorCall{ErrorType: string(e.Function.ID), ErrorArgs: e.Arguments}
		} else {
			err = ast.EnsureErrorExpr{Expr: expr}
		}
	case ast.VariableNode:
		if !strings.Contains(string(e.Ident.ID), ".") {
			err = ast.EnsureErrorVar(string(e.Ident.ID))
		} else {
			err = ast.EnsureErrorExpr{Expr: expr}
		}
	default:
		err = ast.EnsureErrorExpr{Expr: expr}
	}
	return &err
}

func (p *Parser) parseEnsureStatement() ast.EnsureNode {
	p.advance() // Move past `ensure`

	var variable ast.VariableNode
	var callSubject ast.ExpressionNode
	var assertion ast.AssertionNode
	var target ast.RefinementTarget
	implicit := ast.EnsureImplicitNone

	// Handle special case for negated variable check: `ensure !x` (place-only; no call subjects).
	if p.current().Type == ast.TokenLogicalNot && p.peek().Type == ast.TokenIdentifier {
		p.advance() // Move past !
		if p.peek().Type == ast.TokenLParen {
			p.FailWithReport(p.current(), "ensure-negation-subject", "ensure ! needs a variable",
				"After `ensure !`, write a variable name such as `ensure !flag` or `ensure !err`.",
				"write `ensure !flag` (Bool) or `ensure !err` (Error). For a Result, write `ensure result`")
		}
		tok := p.current()
		variable = ast.VariableNode{
			Ident: ast.Ident{
				ID:   ast.Identifier(tok.Value),
				Span: ast.SpanFromToken(tok),
			},
		}
		p.advance() // Move past variable
		if p.current().Type == ast.TokenIs {
			subj := string(variable.Ident.ID)
			p.FailWithReport(p.current(), "ensure-bang-is", "`!` and `is` cannot combine",
				"After `ensure !"+subj+"`, end the statement or write `else`.",
				fmt.Sprintf("write `ensure %s is …` (put the constraint on the subject) or bare `ensure !%s`", subj, subj))
		}
		// Constraint is specialized from the subject type during typechecking.
		implicit = ast.EnsureImplicitBang
	} else {
		// Reject literals / paren groups early (calls are allowed as fire-and-forget subjects).
		switch p.current().Type {
		case ast.TokenStringLiteral, ast.TokenIntLiteral, ast.TokenFloatLiteral,
			ast.TokenRuneLiteral, ast.TokenTrue, ast.TokenFalse, ast.TokenNil:
			p.FailWithReport(p.current(), "refinement-non-place-subject", "ensure needs a variable or call",
				"Write `ensure x`, `ensure x.field`, or `ensure need(ok)`.",
				"bind the value to a variable, then ensure on that name")
		case ast.TokenLParen:
			p.FailWithReport(p.current(), "refinement-non-place-subject", "ensure needs a variable or call",
				"Write `ensure x`, `ensure x.field`, or `ensure need(ok)`.",
				"bind the expression to a variable, then ensure on that name")
		}

		if p.current().Type != ast.TokenIdentifier {
			code, title, problem, help := ensureMissingIsReport("?", p.current())
			p.FailWithReport(p.current(), code, title, problem, help)
		}

		variable, callSubject = p.parseEnsureSubject()
		subjLabel := ensureSubjectLabel(variable, callSubject)

		// Reject arithmetic / comparison subjects: `a + b is …` never starts with two idents.
		// `ensure a + b` hits `+` before `is`.
		if p.current().Type != ast.TokenIs {
			tok := p.current()
			if tok.Type.IsArithmeticBinaryOperator() || tok.Type.IsComparisonBinaryOperator() ||
				tok.Type == ast.TokenLogicalOr || tok.Type == ast.TokenLogicalAnd {
				p.FailWithReport(tok, "refinement-non-place-subject", "ensure needs a variable or call",
					fmt.Sprintf("Write `ensure x` or `ensure need(ok)`. Found operator %s after the subject.", tok.Type),
					"bind the expression to a variable, then ensure on that name")
			}
			if tok.Type == ast.TokenOr {
				code, title, problem, help := ensureMissingIsReport(subjLabel, tok)
				p.FailWithReport(tok, code, title, problem, help)
			}
			// Bare `ensure x` / `ensure need(ok)` / `ensure x else …` — specialize later.
			if ensureStatementContinuesWithoutIs(tok) {
				implicit = ast.EnsureImplicitBare
			} else {
				code, title, problem, help := ensureMissingIsReport(subjLabel, tok)
				p.FailWithReport(tok, code, title, problem, help)
			}
		} else {
			p.expect(ast.TokenIs)
			if tok := p.current(); tok.Type == ast.TokenTrue || tok.Type == ast.TokenFalse {
				want := "True()"
				if tok.Type == ast.TokenFalse {
					want = "False()"
				}
				p.FailWithReport(tok, "ensure-boolean-literal", "ensure predicate must be a constraint",
					fmt.Sprintf("Write `ensure %s is %s`.", subjLabel, want),
					fmt.Sprintf("use `ensure %s is %s`", subjLabel, want))
			}

			target, assertion = p.parseRefinementTarget()

			// Try to set the base type from the current scope if not set (simple place subject only).
			if callSubject == nil && assertion.BaseType == nil && p.context != nil && p.context.ScopeStack != nil {
				scope := p.context.ScopeStack.CurrentScope()
				if scope != nil {
					parts := strings.Split(string(variable.Ident.ID), ".")
					baseIdent := parts[0]
					if typeNode, ok := scope.Variables[baseIdent]; ok && len(parts) == 1 {
						baseType := typeNode.Ident
						assertion.BaseType = &baseType
					}
				}
			}
		}
	}

	inGuard := p.context != nil && p.context.IsTypeGuard()
	inMain := p.context != nil && p.context.IsMainFunction()

	var errNode *ast.EnsureErrorNode
	var block *ast.EnsureBlockNode

	// Failure handling: `else <Error()|errVar>` or `else { … }`
	if p.current().Type == ast.TokenElse {
		elseTok := p.current()
		// `else if` belongs to surrounding control flow
		if p.peek().Type != ast.TokenIf {
			p.advance() // consume else
			if p.current().Type == ast.TokenLBrace {
				if inGuard {
					p.FailWithReport(elseTok, "refinement-failure-block-in-guard", "failure blocks are not allowed inside type guards",
						"Typed failure blocks (`else { … }`) cannot appear inside type guards.",
						"use a typed `else Error{}` or move the ensure outside the guard")
				}
				block = p.parseEnsureBlock()
			} else {
				if inGuard {
					p.FailWithReport(elseTok, "refinement-else-in-guard", "typed `else` is not allowed inside type guards",
						"Typed failure (`else Error{}`) cannot appear inside type guards.",
						"move the ensure outside the guard or use a failure block only in ordinary functions")
				}
				if inMain {
					p.FailWithReport(elseTok, "refinement-else-in-main", "typed failure is not allowed in main",
						`"else" typed failure in ensure statements is not allowed in main function.`,
						"use a failure block `else { … }` in main, or move typed failure to another function")
				}
				errNode = p.parseEnsureError()
				if p.current().Type == ast.TokenLBrace {
					p.FailWithReport(p.current(), "refinement-else-and-block", "cannot combine typed `else` and failure block",
						"use either `else <error>` or a failure block `else { … }`, not both.",
						"pick one failure form per ensure statement")
				}
			}
		}
	} else if p.current().Type == ast.TokenLBrace {
		if inGuard {
			p.FailWithReport(p.current(), "refinement-failure-block-in-guard", "failure blocks are not allowed inside type guards",
				"Typed failure blocks (`else { … }`) cannot appear inside type guards.",
				"use a typed `else Error{}` or move the ensure outside the guard")
		}
		p.FailWithReport(p.current(), "refinement-bare-ensure-block", "ensure failure block requires `else`",
			"ensure failure block requires 'else'; write: ensure … else { … }",
			"prefix the block with `else`")
	}

	// `else` after block
	if p.current().Type == ast.TokenElse && block != nil {
		p.FailWithReport(p.current(), "refinement-else-and-block", "cannot combine typed `else` and failure block",
			"use either `else <error>` or a failure block `else { … }`, not both.",
			"pick one failure form per ensure statement")
	}

	// Legacy `or` as typed failure: if somehow still present after a complete target
	// without having been consumed as Join (e.g. orphaned), suggest else.
	if p.current().Type == ast.TokenOr && errNode == nil && block == nil {
		p.FailWithReport(p.current(), "refinement-legacy-failure-or", "typed failure uses `else`, not `or`",
			"typed failure uses `else`, not `or`; `or` joins assertion alternatives.",
			"write `ensure x is Foo() else MyError{}` instead of `… or MyError{}`")
	}

	return ast.EnsureNode{
		Variable:  variable,
		Subject:   callSubject,
		Target:    target,
		Assertion: assertion,
		Implicit:  implicit,
		Block:     block,
		Error:     errNode,
	}
}

// parseEnsureSubject parses a place (ident / field path) or call / method-call subject.
// Caller must be positioned on TokenIdentifier.
func (p *Parser) parseEnsureSubject() (ast.VariableNode, ast.ExpressionNode) {
	firstTok := p.expect(ast.TokenIdentifier)

	// Function call: need(ok)
	if p.current().Type == ast.TokenLParen {
		lparen := p.current()
		p.advance()
		args, argSpans := p.parseCallArguments()
		rparen := p.expect(ast.TokenRParen)
		return ast.VariableNode{}, ast.FunctionCallNode{
			Function:  ast.Ident{ID: ast.Identifier(firstTok.Value), Span: ast.SpanFromToken(firstTok)},
			Arguments: args,
			CallSpan:  ast.SpanBetweenTokens(lparen, rparen),
			ArgSpans:  argSpans,
		}
	}

	curIdent := ast.Identifier(firstTok.Value)
	lastTok := firstTok
	recvSpanStart := firstTok

	for p.current().Type == ast.TokenDot {
		p.advance()
		nextTok := p.expect(ast.TokenIdentifier)
		// Method call: obj.doThing() / a.b.c()
		if p.current().Type == ast.TokenLParen {
			lparen := p.current()
			p.advance()
			args, argSpans := p.parseCallArguments()
			rparen := p.expect(ast.TokenRParen)
			recvSpan := ast.SpanBetweenTokens(recvSpanStart, lastTok)
			return ast.VariableNode{}, ast.MethodCallNode{
				Receiver: ast.VariableNode{
					Ident: ast.Ident{ID: curIdent, Span: recvSpan},
				},
				Method:    ast.Ident{ID: ast.Identifier(nextTok.Value), Span: ast.SpanFromToken(nextTok)},
				Arguments: args,
				CallSpan:  ast.SpanBetweenTokens(lparen, rparen),
				ArgSpans:  argSpans,
			}
		}
		curIdent = ast.Identifier(string(curIdent) + "." + nextTok.Value)
		lastTok = nextTok
	}

	subjectSpan := ast.SpanBetweenTokens(firstTok, lastTok)
	return ast.VariableNode{
		Ident: ast.Ident{ID: curIdent, Span: subjectSpan},
	}, nil
}

func ensureSubjectLabel(variable ast.VariableNode, callSubject ast.ExpressionNode) string {
	if callSubject != nil {
		return callSubject.String()
	}
	return string(variable.Ident.ID)
}
