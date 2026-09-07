package parser

import (
	"forst/internal/ast"
	"strings"
	"testing"
)

// TDD: nominal error declarations (RFC 02) — `error X { ... }` without `type`/`=`.
func TestParseErrorTypeDef_withPayloadFields(t *testing.T) {
	tokens := []ast.Token{
		{Type: ast.TokenError, Value: "error", Line: 1, Column: 1},
		{Type: ast.TokenIdentifier, Value: "NotPositive", Line: 1, Column: 7},
		{Type: ast.TokenLBrace, Value: "{", Line: 1, Column: 19},
		{Type: ast.TokenIdentifier, Value: "field", Line: 1, Column: 21},
		{Type: ast.TokenColon, Value: ":", Line: 1, Column: 26},
		{Type: ast.TokenString, Value: "String", Line: 1, Column: 28},
		{Type: ast.TokenRBrace, Value: "}", Line: 1, Column: 35},
		{Type: ast.TokenEOF, Value: "", Line: 1, Column: 36},
	}
	logger := ast.SetupTestLogger(nil)
	p := setupParser(tokens, logger)
	nodes, err := p.ParseFile()
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(nodes))
	}
	td := assertNodeType[ast.TypeDefNode](t, nodes[0], "ast.TypeDefNode")
	if td.Ident != "NotPositive" {
		t.Fatalf("Ident: got %q", td.Ident)
	}
	ee, ok := td.Expr.(ast.TypeDefErrorExpr)
	if !ok {
		t.Fatalf("Expr: want TypeDefErrorExpr, got %T", td.Expr)
	}
	if len(ee.Payload.Fields) != 1 {
		t.Fatalf("payload fields: want 1, got %d", len(ee.Payload.Fields))
	}
	f := ee.Payload.Fields["field"]
	if f.Type == nil || f.Type.Ident != ast.TypeString {
		t.Fatalf("field type: got %+v", f.Type)
	}
}

func TestParseErrorTypeDef_emptyPayload(t *testing.T) {
	tokens := []ast.Token{
		{Type: ast.TokenError, Value: "error", Line: 1, Column: 1},
		{Type: ast.TokenIdentifier, Value: "RateLimited", Line: 1, Column: 7},
		{Type: ast.TokenLBrace, Value: "{", Line: 1, Column: 19},
		{Type: ast.TokenRBrace, Value: "}", Line: 1, Column: 20},
		{Type: ast.TokenEOF, Value: "", Line: 1, Column: 21},
	}
	logger := ast.SetupTestLogger(nil)
	p := setupParser(tokens, logger)
	nodes, err := p.ParseFile()
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	td := assertNodeType[ast.TypeDefNode](t, nodes[0], "ast.TypeDefNode")
	if td.Ident != "RateLimited" {
		t.Fatalf("Ident: got %q", td.Ident)
	}
	ee := td.Expr.(ast.TypeDefErrorExpr)
	if len(ee.Payload.Fields) != 0 {
		t.Fatalf("empty payload: want 0 fields, got %d", len(ee.Payload.Fields))
	}
}

// Full-file parse (same surface as examples/in/nominal_error.ft type + ensure arm).
func TestParseFile_errorNominal_ensureOrPayload(t *testing.T) {
	src := `package main

error NotPositive {
	message: String
}

func F() {
	n := 0
	ensure n is GreaterThan(0) else NotPositive{
		message: "n must be greater than 0",
	}
}
`
	logger := ast.SetupTestLogger(nil)
	p := NewTestParser(src, logger)
	nodes, err := p.ParseFile()
	if err != nil {
		t.Fatal(err)
	}
	var td *ast.TypeDefNode
	for _, n := range nodes {
		if d, ok := n.(ast.TypeDefNode); ok {
			td = &d
			break
		}
	}
	if td == nil {
		t.Fatal("expected TypeDefNode")
	}
	if td.Ident != "NotPositive" {
		t.Fatalf("Ident: %q", td.Ident)
	}
	if _, ok := td.Expr.(ast.TypeDefErrorExpr); !ok {
		t.Fatalf("Expr: want TypeDefErrorExpr, got %T", td.Expr)
	}
}

func TestParseErrorTypeDef_rejectsNameError(t *testing.T) {
	tokens := []ast.Token{
		{Type: ast.TokenError, Value: "error", Line: 1, Column: 1},
		{Type: ast.TokenIdentifier, Value: "Error", Line: 1, Column: 7},
		{Type: ast.TokenLBrace, Value: "{", Line: 1, Column: 13},
		{Type: ast.TokenIdentifier, Value: "x", Line: 1, Column: 15},
		{Type: ast.TokenColon, Value: ":", Line: 1, Column: 16},
		{Type: ast.TokenString, Value: "String", Line: 1, Column: 18},
		{Type: ast.TokenRBrace, Value: "}", Line: 1, Column: 25},
		{Type: ast.TokenEOF, Value: "", Line: 1, Column: 26},
	}
	logger := ast.SetupTestLogger(nil)
	p := setupParser(tokens, logger)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic from FailWithParseError for `error Error { ... }`")
		}
		pe, ok := r.(*ParseError)
		if !ok {
			t.Fatalf("recover: got %T", r)
		}
		if !strings.Contains(pe.Msg, "error Error") {
			t.Fatalf("message: %q", pe.Msg)
		}
	}()
	_, _ = p.ParseFile()
}
