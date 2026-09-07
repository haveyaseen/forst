package typechecker

import (
	"testing"

	"forst/internal/ast"
	"forst/internal/parser"

	"github.com/sirupsen/logrus"
)

// TDD: RFC 02 nominal errors assign to built-in Error; ordinary shape types do not.
func TestIsTypeCompatible_errorNominalToBuiltinError(t *testing.T) {
	tc := New(logrus.New(), false)
	tc.registerType(ast.TypeDefNode{
		Ident: "NotPositive",
		Expr: ast.TypeDefErrorExpr{
			Payload: ast.ShapeNode{
				Fields: map[string]ast.ShapeFieldNode{
					"field": {Type: &ast.TypeNode{Ident: ast.TypeString}},
				},
			},
		},
	})
	if !tc.IsTypeCompatible(ast.TypeNode{Ident: "NotPositive"}, ast.TypeNode{Ident: ast.TypeError}) {
		t.Fatal("want NotPositive <: Error for nominal error type")
	}
}

func TestCheckTypes_errorNominalDeclaration_registersTypeDefErrorExpr(t *testing.T) {
	t.Parallel()
	log := setupTestLogger(nil)
	src := `package main

error NotPositive {
	field: String
}

func main() {
}
`
	p := parser.NewTestParser(src, log)
	nodes, err := p.ParseFile()
	if err != nil {
		t.Fatal(err)
	}
	tc := New(log, false)
	if err := tc.CheckTypes(nodes); err != nil {
		t.Fatal(err)
	}
	def, ok := tc.Defs["NotPositive"].(ast.TypeDefNode)
	if !ok {
		t.Fatalf("Defs[NotPositive]: missing")
	}
	if _, ok := def.Expr.(ast.TypeDefErrorExpr); !ok {
		t.Fatalf("want TypeDefErrorExpr, got %T", def.Expr)
	}
}

func TestIsTypeCompatible_plainShapeTypeNotImplicitError(t *testing.T) {
	tc := New(logrus.New(), false)
	tc.registerType(ast.TypeDefNode{
		Ident: "Person",
		Expr: ast.TypeDefShapeExpr{
			Shape: ast.ShapeNode{
				Fields: map[string]ast.ShapeFieldNode{
					"name": {Type: &ast.TypeNode{Ident: ast.TypeString}},
				},
			},
		},
	})
	if tc.IsTypeCompatible(ast.TypeNode{Ident: "Person"}, ast.TypeNode{Ident: ast.TypeError}) {
		t.Fatal("non-error nominal shape must not be assignable to Error")
	}
}

// End-to-end: same ensure … or Nominal({ … }) shape as examples/in/nominal_error.ft.
func TestCheckTypes_nominalError_ensureOr_typechecks(t *testing.T) {
	t.Parallel()
	log := setupTestLogger(nil)
	src := `package main

error NotPositive {
	message: String
}

func Test() {
	n := 0
	ensure n is GreaterThan(0) else NotPositive{
		message: "n must be greater than 0",
	}
}

func main() {
}
`
	p := parser.NewTestParser(src, log)
	nodes, err := p.ParseFile()
	if err != nil {
		t.Fatal(err)
	}
	tc := New(log, false)
	if err := tc.CheckTypes(nodes); err != nil {
		t.Fatal(err)
	}
}

// Today IsTypeCompatible falls through to structural comparison for typedefs with the same payload.
func TestIsTypeCompatible_distinctNominalErrors_samePayloadStructurallyCompatible(t *testing.T) {
	tc := New(logrus.New(), false)
	tc.registerType(ast.TypeDefNode{
		Ident: "E1",
		Expr: ast.TypeDefErrorExpr{
			Payload: ast.ShapeNode{
				Fields: map[string]ast.ShapeFieldNode{
					"a": {Type: &ast.TypeNode{Ident: ast.TypeInt}},
				},
			},
		},
	})
	tc.registerType(ast.TypeDefNode{
		Ident: "E2",
		Expr: ast.TypeDefErrorExpr{
			Payload: ast.ShapeNode{
				Fields: map[string]ast.ShapeFieldNode{
					"a": {Type: &ast.TypeNode{Ident: ast.TypeInt}},
				},
			},
		},
	})
	if !tc.IsTypeCompatible(ast.TypeNode{Ident: "E1"}, ast.TypeNode{Ident: "E2"}) {
		t.Fatal("same-shaped nominal error typedefs are structurally compatible today")
	}
}
