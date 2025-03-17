package ast

import "fmt"

type LiteralNode interface {
	ValueNode
	isLiteral() // Marker method to identify literal nodes
}

// IntLiteralNode represents an integer literal
type IntLiteralNode struct {
	Value int64
	Type  TypeNode
}

// FloatLiteralNode represents a float literal
type FloatLiteralNode struct {
	Value float64
	Type  TypeNode
}

// StringLiteralNode represents a string literal
type StringLiteralNode struct {
	Value string
	Type  TypeNode
}

// BoolLiteralNode represents a boolean literal
type BoolLiteralNode struct {
	Value bool
	Type  TypeNode
}

func (i IntLiteralNode) NodeType() NodeType {
	return NodeTypeIntLiteral
}
func (f FloatLiteralNode) NodeType() NodeType {
	return NodeTypeFloatLiteral
}
func (s StringLiteralNode) NodeType() NodeType {
	return NodeTypeStringLiteral
}
func (b BoolLiteralNode) NodeType() NodeType {
	return NodeTypeBoolLiteral
}

// Marker methods to satisfy LiteralNode interface
func (i IntLiteralNode) isLiteral()    {}
func (f FloatLiteralNode) isLiteral()  {}
func (s StringLiteralNode) isLiteral() {}
func (b BoolLiteralNode) isLiteral()   {}

// Implement ValueNode interface for all literal nodes
func (i IntLiteralNode) isValue()    {}
func (f FloatLiteralNode) isValue()  {}
func (s StringLiteralNode) isValue() {}
func (b BoolLiteralNode) isValue()   {}

func (i IntLiteralNode) String() string {
	return fmt.Sprintf("%d", i.Value)
}

func (f FloatLiteralNode) String() string {
	return fmt.Sprintf("%f", f.Value)
}

func (s StringLiteralNode) String() string {
	return fmt.Sprintf("\"%s\"", s.Value)
}

func (b BoolLiteralNode) String() string {
	return fmt.Sprintf("%t", b.Value)
}
