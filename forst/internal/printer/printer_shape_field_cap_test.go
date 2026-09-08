package printer

import (
	"strings"
	"testing"

	"forst/internal/ast"
	"github.com/sirupsen/logrus"
)

func TestShapeShouldUseMultiline_twoFieldsUnderWidthStaysOneLine(t *testing.T) {
	t.Parallel()
	p := printer{cfg: DefaultConfig()}
	shape := ast.ShapeNode{
		Fields: map[string]ast.ShapeFieldNode{
			"a": {Type: &ast.TypeNode{Ident: ast.TypeString}},
			"b": {Type: &ast.TypeNode{Ident: ast.TypeInt}},
		},
	}
	if p.shapeShouldUseMultiline(shape) {
		t.Fatal("expected two short fields to stay one-line")
	}
}

func TestShapeShouldUseMultiline_threeFieldsForcesMultiline(t *testing.T) {
	t.Parallel()
	p := printer{cfg: DefaultConfig()}
	shape := ast.ShapeNode{
		Fields: map[string]ast.ShapeFieldNode{
			"cells":      {Type: &ast.TypeNode{Ident: "[]String"}},
			"nextPlayer": {Type: &ast.TypeNode{Ident: ast.TypeString}},
			"status":     {Type: &ast.TypeNode{Ident: ast.TypeString}},
		},
	}
	if !p.shapeShouldUseMultiline(shape) {
		t.Fatal("expected three fields to force multiline even when under width budget")
	}
}

func TestShapeShouldUseMultiline_nestedShapeStillForcesMultiline(t *testing.T) {
	t.Parallel()
	p := printer{cfg: DefaultConfig()}
	nested := ast.ShapeNode{
		Fields: map[string]ast.ShapeFieldNode{
			"n": {Type: &ast.TypeNode{Ident: ast.TypeInt}},
		},
	}
	shape := ast.ShapeNode{
		Fields: map[string]ast.ShapeFieldNode{
			"ctx":   {Shape: &nested},
			"input": {Type: &ast.TypeNode{Ident: ast.TypeString}},
		},
	}
	if !p.shapeShouldUseMultiline(shape) {
		t.Fatal("expected nested shape fields to force multiline")
	}
}

func TestFormatSource_shapeTwoFieldsStaysOneLine(t *testing.T) {
	t.Parallel()
	const src = `package main

type Pair = {a: String, b: Int}
`
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	out, err := FormatSource(src, "pair.ft", log)
	if err != nil {
		t.Fatalf("FormatSource: %v", err)
	}
	if !strings.Contains(out, "type Pair = {a: String, b: Int}") {
		t.Fatalf("expected two-field typedef on one line, got:\n%s", out)
	}
}

func TestFormatSource_shapeThreeFieldsForcesMultiline(t *testing.T) {
	t.Parallel()
	const src = `package main

type GameState = {cells: []String, nextPlayer: String, status: String}
`
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	out, err := FormatSource(src, "gamestate.ft", log)
	if err != nil {
		t.Fatalf("FormatSource: %v", err)
	}
	if strings.Contains(out, "{cells: []String, nextPlayer: String, status: String}") {
		t.Fatalf("expected three-field typedef to break across lines, got:\n%s", out)
	}
	for _, needle := range []string{"cells:", "nextPlayer:", "status:"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("missing field %q in:\n%s", needle, out)
		}
	}
}

func TestPrint_withThreeFieldWiringForcesMultiline(t *testing.T) {
	t.Parallel()
	with := ast.WithNode{
		Wiring: ast.ShapeNode{
			Fields: map[string]ast.ShapeFieldNode{
				"Logger": {Type: &ast.TypeNode{Ident: "NopLogger"}},
				"Clock":  {Type: &ast.TypeNode{Ident: "FakeClock"}},
				"Store":  {Type: &ast.TypeNode{Ident: "MemStore"}},
			},
		},
		Body: []ast.Node{},
	}
	var p printer
	p.cfg = DefaultConfig()
	out, err := p.printWith(with)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "with {\n") {
		t.Fatalf("expected multiline with wiring for three fields, got:\n%s", out)
	}
}
