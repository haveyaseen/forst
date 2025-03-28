package ast

// Package Node
type PackageNode struct {
	Ident Ident
}

// NodeType returns the type of this AST node
func (p PackageNode) Kind() NodeKind {
	return NodeKindPackage
}

func (p PackageNode) String() string {
	return p.Ident.String()
}

func (p PackageNode) Id() Identifier {
	return p.Ident.Id
}

func (p PackageNode) IsMainPackage() bool {
	return p.Id() == "main"
}
