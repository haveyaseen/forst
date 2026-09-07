package typechecker

import (
	"strings"
	"testing"

	"forst/internal/ast"
	"forst/internal/parser"

	"github.com/sirupsen/logrus"
)

func nominalErrorTC(t *testing.T) *TypeChecker {
	t.Helper()
	tc := New(logrus.New(), false)
	tc.registerType(ast.TypeDefNode{
		Ident: "NotFound",
		Expr: ast.TypeDefErrorExpr{
			Payload: ast.ShapeNode{
				Fields: map[string]ast.ShapeFieldNode{
					"id": {Type: &ast.TypeNode{Ident: ast.TypeString}},
				},
			},
		},
	})
	return tc
}

func TestInferNominalErrorConstructorCall_rejectsShapeArgCall(t *testing.T) {
	tc := nominalErrorTC(t)
	call := ast.FunctionCallNode{
		Function: ast.Ident{ID: "NotFound", Span: ast.FakeSpan()},
		Arguments: []ast.ExpressionNode{
			ast.ShapeNode{
				Fields: map[string]ast.ShapeFieldNode{
					"id": {Type: &ast.TypeNode{Ident: ast.TypeString}},
				},
			},
		},
		CallSpan: ast.FakeSpan(),
	}
	_, handled, err := tc.inferNominalErrorConstructorCall(call, nil)
	if err == nil || !handled {
		t.Fatalf("expected rejection, handled=%v err=%v", handled, err)
	}
	diag, ok := err.(*Diagnostic)
	if !ok || diag.Code != "error-struct-syntax" {
		t.Fatalf("got %T %v", err, err)
	}
	if !strings.Contains(err.Error(), "NotFound{ field: value }") {
		t.Fatalf("err = %v", err)
	}
}

func TestInferNominalErrorConstructorCall_rejectsEmptyCall(t *testing.T) {
	tc := New(logrus.New(), false)
	tc.registerType(ast.TypeDefNode{
		Ident: "TooFast",
		Expr:  ast.TypeDefErrorExpr{Payload: ast.ShapeNode{Fields: map[string]ast.ShapeFieldNode{}}},
	})
	call := ast.FunctionCallNode{
		Function:  ast.Ident{ID: "TooFast", Span: ast.FakeSpan()},
		Arguments: nil,
		CallSpan:  ast.FakeSpan(),
	}
	_, handled, err := tc.inferNominalErrorConstructorCall(call, nil)
	if err == nil || !handled {
		t.Fatalf("expected rejection, handled=%v err=%v", handled, err)
	}
	if !strings.Contains(err.Error(), "TooFast{}") {
		t.Fatalf("err = %v", err)
	}
}

func TestInferNominalErrorConstructorCall_rejectsStringArgCall(t *testing.T) {
	tc := nominalErrorTC(t)
	call := ast.FunctionCallNode{
		Function:  ast.Ident{ID: "NotFound", Span: ast.FakeSpan()},
		Arguments: []ast.ExpressionNode{ast.StringLiteralNode{Value: "x", Span: ast.FakeSpan()}},
		CallSpan:  ast.FakeSpan(),
	}
	_, handled, err := tc.inferNominalErrorConstructorCall(call, nil)
	if err == nil || !handled {
		t.Fatalf("expected rejection, handled=%v err=%v", handled, err)
	}
	diag, ok := err.(*Diagnostic)
	if !ok || diag.Code != "error-struct-syntax" {
		t.Fatalf("got %T %v", err, err)
	}
}

func TestInferNominalErrorConstructorCall_notErrorTypedef(t *testing.T) {
	tc := New(logrus.New(), false)
	tc.registerType(ast.TypeDefNode{
		Ident: "Point",
		Expr: ast.TypeDefShapeExpr{
			Shape: ast.ShapeNode{
				Fields: map[string]ast.ShapeFieldNode{
					"x": {Type: &ast.TypeNode{Ident: ast.TypeInt}},
				},
			},
		},
	})
	call := ast.FunctionCallNode{
		Function: ast.Ident{ID: "Point"},
		Arguments: []ast.ExpressionNode{
			ast.ShapeNode{Fields: map[string]ast.ShapeFieldNode{
				"x": {Type: &ast.TypeNode{Ident: ast.TypeInt}},
			}},
		},
	}
	_, handled, err := tc.inferNominalErrorConstructorCall(call, nil)
	if err != nil || handled {
		t.Fatalf("expected not handled for shape typedef, handled=%v err=%v", handled, err)
	}
}

func TestInferNominalErrorConstructorCall_unknownFunction(t *testing.T) {
	tc := New(logrus.New(), false)
	call := ast.FunctionCallNode{
		Function:  ast.Ident{ID: "Missing"},
		Arguments: []ast.ExpressionNode{ast.ShapeNode{}},
	}
	_, handled, err := tc.inferNominalErrorConstructorCall(call, nil)
	if err != nil || handled {
		t.Fatalf("expected not handled for unknown fn, handled=%v err=%v", handled, err)
	}
}

func TestCheckTypes_nominalError_structLiteral_ok(t *testing.T) {
	t.Parallel()
	src := `package main

error NotFound { id: String }
error TooFast {}

func f() {
	e := NotFound{id: "x"}
	_ = e
	ok := false
	ensure ok is True() else TooFast{}
}

func main() {}
`
	log := logrus.New()
	log.SetLevel(logrus.PanicLevel)
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

func TestCheckTypes_nominalError_callConstructor_rejected(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		src  string
	}{
		{
			"shape_arg_call",
			`package main
error E { message: String }
func f() {
	ok := false
	ensure ok is True() else E({message: "bad"})
}
func main() {}
`,
		},
		{
			"empty_call",
			`package main
error TooFast {}
func f() {
	ok := false
	ensure ok is True() else TooFast()
}
func main() {}
`,
		},
		{
			"string_arg_call",
			`package main
error TooShort { message: String }
func f() {
	ok := false
	ensure ok is True() else TooShort("empty")
}
func main() {}
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			log := logrus.New()
			log.SetLevel(logrus.PanicLevel)
			p := parser.NewTestParser(tc.src, log)
			nodes, err := p.ParseFile()
			if err != nil {
				t.Fatal(err)
			}
			checker := New(log, false)
			err = checker.CheckTypes(nodes)
			if err == nil {
				t.Fatal("expected error-struct-syntax")
			}
			if !strings.Contains(err.Error(), "error-struct-syntax") {
				t.Fatalf("got: %v", err)
			}
		})
	}
}

func TestCheckTypes_ensureElse_helperFunctionCall_stillOk(t *testing.T) {
	t.Parallel()
	src := `package main

import "errors"

func bad(msg String): Error {
	return errors.New(msg)
}

func f() {
	ok := false
	ensure ok is True() else bad("nope")
}

func main() {}
`
	log := logrus.New()
	log.SetLevel(logrus.PanicLevel)
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

func TestCheckTypes_nominalError_payloadMismatch_rejected(t *testing.T) {
	t.Parallel()
	src := `package main

error E { message: String }

func f() {
	_ = E{wrong: 1}
}

func main() {}
`
	log := logrus.New()
	log.SetLevel(logrus.PanicLevel)
	p := parser.NewTestParser(src, log)
	nodes, err := p.ParseFile()
	if err != nil {
		t.Fatal(err)
	}
	tc := New(log, false)
	err = tc.CheckTypes(nodes)
	if err == nil {
		t.Fatal("expected payload mismatch")
	}
	if !strings.Contains(err.Error(), "payload") && !strings.Contains(err.Error(), "wrong") {
		t.Fatalf("got: %v", err)
	}
}
