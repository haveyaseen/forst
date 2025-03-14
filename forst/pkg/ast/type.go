package ast

// TypeNode represents a type in the Forst language
type TypeNode struct {
	Name string
}

// Built-in types
const (
	TypeInt    = "INT"
	TypeString = "STRING" 
	TypeBool   = "BOOL"
	TypeVoid   = "VOID"
	TypeImplicit = "IMPLICIT"
)

// IsImplicit returns true if the type has not been specified explicitly
func (t TypeNode) IsImplicit() bool {
	return t.Name == TypeImplicit
}

