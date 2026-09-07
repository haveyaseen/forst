package ast

import (
	"strings"
	"testing"
)

func TestEnsureNode_String_error_branches(t *testing.T) {
	base := EnsureNode{
		Variable:  VariableNode{Ident: Ident{ID: "x"}},
		Assertion: AssertionNode{},
	}
	if !strings.Contains(base.String(), "Ensure") || strings.Count(base.String(), ",") < 1 {
		t.Fatalf("no error: %q", base.String())
	}
	var call EnsureErrorNode = EnsureErrorCall{ErrorType: "New", ErrorArgs: []ExpressionNode{StringLiteralNode{Value: "m"}}}
	withCall := base
	withCall.Error = &call
	if !strings.Contains(withCall.String(), "New") {
		t.Fatalf("with call: %q", withCall.String())
	}
	var v EnsureErrorNode = EnsureErrorVar("errVar")
	withVar := base
	withVar.Error = &v
	if !strings.Contains(withVar.String(), "errVar") {
		t.Fatalf("with var: %q", withVar.String())
	}
}

func TestEnsureNode_callSubject_helpers(t *testing.T) {
	call := FunctionCallNode{
		Function:  Ident{ID: "need"},
		Arguments: []ExpressionNode{VariableNode{Ident: Ident{ID: "ok"}}},
	}
	e := EnsureNode{
		Subject:  call,
		Implicit: EnsureImplicitBare,
	}
	if !e.IsCallSubject() {
		t.Fatal("expected IsCallSubject")
	}
	if _, ok := e.EnsurePlaceSubject(); ok {
		t.Fatal("call subject must not report a place")
	}
	if e.EnsureSubject() == nil {
		t.Fatal("EnsureSubject nil")
	}
	if !strings.Contains(e.String(), "need") {
		t.Fatalf("String = %q", e.String())
	}
}

func TestEnsureErrorCall_and_EnsureErrorVar_String(t *testing.T) {
	c := EnsureErrorCall{ErrorType: "E", ErrorArgs: []ExpressionNode{IntLiteralNode{Value: 2}}}
	if !strings.Contains(c.String(), "E") || !strings.Contains(c.String(), "2") {
		t.Fatal(c.String())
	}
	if EnsureErrorVar("z").String() != "z" {
		t.Fatal(EnsureErrorVar("z").String())
	}
}

func TestEnsureBlockNode_Kind_and_String(t *testing.T) {
	b := EnsureBlockNode{Body: []Node{IntLiteralNode{Value: 1}}}
	if b.Kind() != NodeKindEnsureBlock || b.String() != "EnsureBlock" {
		t.Fatal(b.Kind(), b.String())
	}
}
