package ast

import (
	"fmt"
	"strings"
)

// ExpressionNode represents any expression in the AST
type ExpressionNode interface {
	Node

	isExpression()
	String() string
}

// UnaryExpressionNode represents a unary expression in the AST
type UnaryExpressionNode struct {
	Operator TokenIdent
	Operand  ExpressionNode
}

// BinaryExpressionNode represents a binary expression in the AST
type BinaryExpressionNode struct {
	Left     ExpressionNode
	Operator TokenIdent
	Right    ExpressionNode
}

type FunctionCallNode struct {
	Function  Ident
	Arguments []ExpressionNode
}

// Ensure LiteralNode implements ExpressionNode
func (i IntLiteralNode) isExpression()       {}
func (f FloatLiteralNode) isExpression()     {}
func (s StringLiteralNode) isExpression()    {}
func (b BoolLiteralNode) isExpression()      {}
func (u UnaryExpressionNode) isExpression()  {}
func (b BinaryExpressionNode) isExpression() {}
func (f FunctionCallNode) isExpression()     {}

func (u UnaryExpressionNode) Kind() NodeKind {
	return NodeKindUnaryExpression
}
func (b BinaryExpressionNode) Kind() NodeKind {
	return NodeKindBinaryExpression
}
func (f FunctionCallNode) Kind() NodeKind {
	return NodeKindFunctionCall
}

func (u UnaryExpressionNode) String() string {
	return fmt.Sprintf("%s %s", u.Operator, u.Operand.String())
}

func (b BinaryExpressionNode) String() string {
	return fmt.Sprintf("%s %s %s", b.Left.String(), b.Operator, b.Right.String())
}

func (f FunctionCallNode) String() string {
	args := make([]string, len(f.Arguments))
	for i, arg := range f.Arguments {
		args[i] = arg.String()
	}
	return fmt.Sprintf("%s(%s)", f.Function, strings.Join(args, ", "))
}
