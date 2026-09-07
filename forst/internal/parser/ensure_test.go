package parser

import (
	"forst/internal/ast"
	"strings"
	"testing"
)

func TestParseEnsure_bareBoolSuggestsIsTrue(t *testing.T) {
	t.Parallel()
	src := `package main

error Fail { msg: String }

func check(ok Bool): Result(String, Error) {
	ensure ok or Fail("no")
	return "ok"
}
`
	err := parseShouldFail(src)
	if err == nil {
		t.Fatal("expected parse error for bare ensure ok or …")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ensure-missing-is") && !strings.Contains(msg, "ensure needs `is`") {
		t.Fatalf("expected ensure-missing-is diagnostic, got: %v", err)
	}
	if !strings.Contains(msg, "ensure ok is True()") {
		t.Fatalf("expected suggestion ensure ok is True(), got: %v", err)
	}
}

func TestParseEnsure_bareBoolElse(t *testing.T) {
	t.Parallel()
	src := `package main

error E { message: String }

func check(b Bool) {
	ensure b
		else E({message: "no"})
	return 1
}
`
	nodes, err := NewTestParser(src, ast.SetupTestLogger(nil)).ParseFile()
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	var fn ast.FunctionNode
	found := false
	for _, n := range nodes {
		if f, ok := n.(ast.FunctionNode); ok {
			fn = f
			found = true
			break
		}
	}
	if !found {
		t.Fatal("missing function")
	}
	ens, ok := fn.Body[0].(ast.EnsureNode)
	if !ok {
		t.Fatalf("expected ensure, got %T", fn.Body[0])
	}
	if ens.Implicit != ast.EnsureImplicitBare {
		t.Fatalf("Implicit = %v, want Bare", ens.Implicit)
	}
}

func TestParseEnsure_bangImplicit(t *testing.T) {
	t.Parallel()
	src := `package main
func f(err Error) {
	ensure !err
}
`
	nodes, err := NewTestParser(src, ast.SetupTestLogger(nil)).ParseFile()
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	var fn ast.FunctionNode
	found := false
	for _, n := range nodes {
		if f, ok := n.(ast.FunctionNode); ok {
			fn = f
			found = true
			break
		}
	}
	if !found {
		t.Fatal("missing function")
	}
	ens := fn.Body[0].(ast.EnsureNode)
	if ens.Implicit != ast.EnsureImplicitBang {
		t.Fatalf("Implicit = %v, want Bang", ens.Implicit)
	}
	if len(ens.Assertion.Constraints) != 0 {
		t.Fatalf("bang sugar must not bake Nil at parse time, got %#v", ens.Assertion)
	}
}

func TestParseEnsure_bangIsRejected(t *testing.T) {
	t.Parallel()
	src := `package main

func f(x Result(Int, Error)) {
	ensure !x is Ok()
}
`
	err := parseShouldFail(src)
	if err == nil {
		t.Fatal("expected parse error for ensure !x is …")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ensure-bang-is") {
		t.Fatalf("expected ensure-bang-is diagnostic, got: %v", err)
	}
	if !strings.Contains(msg, "ensure x is") {
		t.Fatalf("expected rewrite ensure x is …, got: %v", err)
	}
	if !strings.Contains(msg, "ensure !x") {
		t.Fatalf("expected bare ensure !x suggestion, got: %v", err)
	}
}

func TestParseEnsure_elseMethodCall(t *testing.T) {
	t.Parallel()
	src := `package main

error E { message: String }

type P = { n: Int }

func (p *P) errMsg(msg String): E {
	return E({message: msg})
}

func (p *P) bad(ok Bool) {
	ensure ok is True()
		else p.errMsg("bad")
}

func sibling() {}
`
	nodes, err := NewTestParser(src, ast.SetupTestLogger(nil)).ParseFile()
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	foundSibling := false
	for _, n := range nodes {
		if fn, ok := n.(ast.FunctionNode); ok && fn.Ident.ID == "sibling" {
			foundSibling = true
		}
	}
	if !foundSibling {
		t.Fatal("sibling function vanished after ensure-else method parse")
	}
}

func TestParseEnsure_elseConcatInShape(t *testing.T) {
	t.Parallel()
	src := `package main

error E { message: String }

func bad(ok Bool, kw String) {
	ensure ok is True()
		else E({message: "bad " + kw})
}

func sibling() {}
`
	nodes, err := NewTestParser(src, ast.SetupTestLogger(nil)).ParseFile()
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	foundSibling := false
	for _, n := range nodes {
		if fn, ok := n.(ast.FunctionNode); ok && fn.Ident.ID == "sibling" {
			foundSibling = true
		}
	}
	if !foundSibling {
		t.Fatal("sibling vanished after concat-in-shape ensure-else")
	}
}

func TestParseEnsure_isTrueLiteralSuggestsTrueConstraint(t *testing.T) {
	t.Parallel()
	src := `package main

error Fail { msg: String }

func check(flag Bool): Result(String, Error) {
	ensure flag is true or Fail("no")
	return "ok"
}
`
	err := parseShouldFail(src)
	if err == nil {
		t.Fatal("expected parse error for ensure … is true")
	}
	msg := err.Error()
	if !strings.Contains(msg, "True()") {
		t.Fatalf("expected True() suggestion, got: %v", err)
	}
	if !strings.Contains(msg, "ensure-boolean-literal") && !strings.Contains(msg, "must be a constraint") {
		t.Fatalf("expected boolean-literal diagnostic, got: %v", err)
	}
}

func TestParseEnsure_okIsTrueOrNamedError(t *testing.T) {
	t.Parallel()
	src := `package main

error Fail { msg: String }

func check(ok Bool): Result(String, Error) {
	ensure ok is True() else Fail("no")
	return "ok"
}
`
	nodes, err := NewTestParser(src, ast.SetupTestLogger(nil)).ParseFile()
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(nodes) < 2 {
		t.Fatalf("expected error typedef + function, got %d nodes", len(nodes))
	}
}

func TestParseEnsure_ElseFailureBlockInMain(t *testing.T) {
	t.Parallel()

	// 1. main accepts ensure !err else { ... }
	srcValid := `package main
func main() {
	err := false
	ensure !err else {
		println("failed")
	}
}
`
	_, err := NewTestParser(srcValid, ast.SetupTestLogger(nil)).ParseFile()
	if err != nil {
		t.Fatalf("expected valid parse for ensure !err else { ... } in main, got: %v", err)
	}

	// 2. main rejects bare block ensure !err { ... }
	srcBareBlock := `package main
func main() {
	err := false
	ensure !err {
		println("failed")
	}
}
`
	errBare := parseShouldFail(srcBareBlock)
	if errBare == nil {
		t.Fatal("expected parse error for bare block in main")
	}
	if !strings.Contains(errBare.Error(), "ensure failure block requires 'else'") {
		t.Fatalf("expected 'ensure failure block requires else' message, got: %v", errBare)
	}

	// 3. main rejects typed else
	srcTypedElse := `package main
error Fail { msg: String }
func main() {
	err := false
	ensure !err else Fail("no")
}
`
	errTyped := parseShouldFail(srcTypedElse)
	if errTyped == nil {
		t.Fatal("expected parse error for typed else in main")
	}
	if !strings.Contains(errTyped.Error(), "typed failure in ensure statements is not allowed in main function") {
		t.Fatalf("expected typed failure in main error message, got: %v", errTyped)
	}
}

func TestParseEnsure_callSubjectAllowed(t *testing.T) {
	t.Parallel()
	src := `package main

error Fail { msg: String }

func need(ok Bool) {
	ensure ok is True() else Fail("no")
}

func check(ok Bool) {
	ensure need(ok)
	return 1
}
`
	nodes, err := NewTestParser(src, ast.SetupTestLogger(nil)).ParseFile()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	fn := findFunction(t, nodes, "check")
	ens, ok := fn.Body[0].(ast.EnsureNode)
	if !ok {
		t.Fatalf("want EnsureNode, got %T", fn.Body[0])
	}
	if !ens.IsCallSubject() {
		t.Fatal("expected call subject on ensure need(ok)")
	}
	call, ok := ens.Subject.(ast.FunctionCallNode)
	if !ok {
		t.Fatalf("want FunctionCallNode subject, got %T", ens.Subject)
	}
	if call.Function.ID != "need" {
		t.Fatalf("callee = %s, want need", call.Function.ID)
	}
	if ens.Implicit != ast.EnsureImplicitBare {
		t.Fatalf("Implicit = %v, want Bare", ens.Implicit)
	}
}

func TestParseEnsure_methodCallSubjectAllowed(t *testing.T) {
	t.Parallel()
	src := `package main

type S = {}

func (s S) ping() {}

func check(s S) {
	ensure s.ping()
}
`
	nodes, err := NewTestParser(src, ast.SetupTestLogger(nil)).ParseFile()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	fn := findFunction(t, nodes, "check")
	ens := fn.Body[0].(ast.EnsureNode)
	mc, ok := ens.Subject.(ast.MethodCallNode)
	if !ok {
		t.Fatalf("want MethodCallNode, got %T", ens.Subject)
	}
	if mc.Method.ID != "ping" {
		t.Fatalf("method = %s, want ping", mc.Method.ID)
	}
}

func TestParseEnsure_bangCallSubjectRejected(t *testing.T) {
	t.Parallel()
	src := `package main

func f() {
	ensure !need(true)
}
`
	err := parseShouldFail(src)
	if err == nil {
		t.Fatal("expected parse error for ensure !call()")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ensure-negation-subject") && !strings.Contains(msg, "ensure ! needs a variable") {
		t.Fatalf("expected bang-call diagnostic, got: %v", err)
	}
}

func TestParseEnsure_callSubjectWithIsOk(t *testing.T) {
	t.Parallel()
	src := `package main

func need(ok Bool) {}

func check(ok Bool) {
	ensure need(ok) is Ok()
}
`
	nodes, err := NewTestParser(src, ast.SetupTestLogger(nil)).ParseFile()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	fn := findFunction(t, nodes, "check")
	ens := fn.Body[0].(ast.EnsureNode)
	if !ens.IsCallSubject() {
		t.Fatal("expected call subject")
	}
	if ens.Implicit != ast.EnsureImplicitNone {
		t.Fatalf("Implicit = %v, want None", ens.Implicit)
	}
	if len(ens.Assertion.Constraints) != 1 || ens.Assertion.Constraints[0].Name != "Ok" {
		t.Fatalf("assertion = %+v, want Ok()", ens.Assertion)
	}
}

func TestParseEnsure(t *testing.T) {
	tests := []struct {
		name     string
		tokens   []ast.Token
		validate func(t *testing.T, nodes []ast.Node)
	}{
		{
			name: "ensure statement with type guard",
			tokens: []ast.Token{
				{Type: ast.TokenFunc, Value: "func", Line: 1, Column: 1},
				{Type: ast.TokenIdentifier, Value: "main", Line: 1, Column: 6},
				{Type: ast.TokenLParen, Value: "(", Line: 1, Column: 9},
				{Type: ast.TokenRParen, Value: ")", Line: 1, Column: 10},
				{Type: ast.TokenLBrace, Value: "{", Line: 1, Column: 12},
				{Type: ast.TokenEnsure, Value: "ensure", Line: 2, Column: 4},
				{Type: ast.TokenIdentifier, Value: "x", Line: 2, Column: 11},
				{Type: ast.TokenIs, Value: "is", Line: 2, Column: 13},
				{Type: ast.TokenIdentifier, Value: "String", Line: 2, Column: 16},
				{Type: ast.TokenRBrace, Value: "}", Line: 3, Column: 1},
				{Type: ast.TokenEOF, Value: "", Line: 3, Column: 2},
			},
			validate: func(t *testing.T, nodes []ast.Node) {
				if len(nodes) != 1 {
					t.Fatalf("Expected 1 node, got %d", len(nodes))
				}
				functionNode := assertNodeType[ast.FunctionNode](t, nodes[0], "ast.FunctionNode")
				if len(functionNode.Body) != 1 {
					t.Fatalf("Expected 1 statement in function body, got %d", len(functionNode.Body))
				}
				ensureNode := assertNodeType[ast.EnsureNode](t, functionNode.Body[0], "ast.EnsureNode")
				if ensureNode.Variable.Ident.ID == "" {
					t.Fatal("Expected ensure variable, got empty")
				}
				if !ensureNode.Variable.Ident.Span.IsSet() {
					t.Fatal("ensure subject Ident.Span must be set for per-occurrence inference and LSP hover")
				}
				wantSpan := ast.SpanFromToken(ast.Token{Line: 2, Column: 11, Value: "x"})
				if ensureNode.Variable.Ident.Span != wantSpan {
					t.Fatalf("ensure subject span: got %+v want %+v", ensureNode.Variable.Ident.Span, wantSpan)
				}
				if ensureNode.Assertion.BaseType == nil {
					t.Fatal("Expected ensure assertion with BaseType, got nil")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := ast.SetupTestLogger(nil)
			p := setupParser(tt.tokens, logger)
			nodes, err := p.ParseFile()
			if err != nil {
				t.Fatalf("ParseFile failed: %v", err)
			}
			tt.validate(t, nodes)
		})
	}
}
