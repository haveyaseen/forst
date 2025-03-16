package typechecker

import "forst/pkg/ast"

type Scope struct {
	Parent   *Scope
	Node     ast.Node                  // The AST node that created this scope (function, block, if, etc)
	Symbols  map[ast.Identifier]Symbol // All symbols defined in this scope
	Children []*Scope                  // Child scopes (blocks, branches)
}

type Symbol struct {
	Identifier ast.Identifier
	Type       ast.TypeNode
	Kind       SymbolKind // Variable, Function, Type, etc
	Scope      *Scope     // Where this symbol is defined
	Position   NodePath   // Precise location in AST where symbol is valid
}

type NodePath []ast.Node // Path from root to current node

type SymbolKind int

const (
	SymbolVariable SymbolKind = iota
	SymbolFunction
	SymbolType
	SymbolParameter
)

// Add these methods to manage symbols
func (tc *TypeChecker) storeSymbol(ident ast.Identifier, typ ast.TypeNode, kind SymbolKind) {
	symbol := Symbol{
		Identifier: ident,
		Type:       typ,
		Kind:       kind,
		Scope:      tc.currentScope,
		Position:   append(NodePath(nil), tc.path...), // Copy current path
	}
	tc.currentScope.Symbols[ident] = symbol
}

func (tc *TypeChecker) pushScope(node ast.Node) {
	newScope := &Scope{
		Parent:   tc.currentScope,
		Node:     node,
		Symbols:  make(map[ast.Identifier]Symbol),
		Children: make([]*Scope, 0),
	}

	if tc.currentScope != nil {
		tc.currentScope.Children = append(tc.currentScope.Children, newScope)
	}
	tc.Scopes[node] = newScope
	tc.currentScope = newScope
}

func (tc *TypeChecker) popScope() {
	if tc.currentScope.Parent != nil {
		tc.currentScope = tc.currentScope.Parent
	}
}
