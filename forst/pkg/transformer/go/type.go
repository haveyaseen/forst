package transformer_go

import (
	"fmt"
	"forst/pkg/ast"
	goast "go/ast"
)

// transformType converts a Forst type node to a Go type declaration
func transformType(n ast.TypeNode) *goast.Ident {
	switch n.Name {
	case ast.TypeInt:
		return goast.NewIdent("int")
	case ast.TypeFloat:
		return goast.NewIdent("float64")
	case ast.TypeString:
		return goast.NewIdent("string")
	case ast.TypeBool:
		return goast.NewIdent("bool")
	case ast.TypeVoid:
		return goast.NewIdent("void")
	case ast.TypeError:
		return goast.NewIdent("error")
	case ast.TypeAssertion:
		// TODO: Look up the type assertion in the type registry
		return goast.NewIdent(string(n.Name))
	}
	panic(fmt.Sprintf("Unknown type: %s", n.Name))
}

func transformTypes(types []ast.TypeNode) *goast.FieldList {
	fields := make([]*goast.Field, len(types))
	for i, typ := range types {
		fields[i] = &goast.Field{
			Type: transformType(typ),
		}
	}
	return &goast.FieldList{
		List: fields,
	}
}
