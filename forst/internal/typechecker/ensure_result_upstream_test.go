package typechecker

import (
	"testing"

	"forst/internal/ast"
	"forst/internal/lexer"
	"forst/internal/parser"
	"strings"

	"github.com/sirupsen/logrus"
)

func typecheckUpstream(t *testing.T, src string) *TypeChecker {
	t.Helper()
	log := logrus.New()
	log.SetLevel(logrus.PanicLevel)
	toks := lexer.New([]byte(src), "t.ft", log).Lex()
	nodes, err := parser.New(toks, "t.ft", log).ParseFile()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tc := New(log, false)
	if err := tc.CheckTypes(nodes); err != nil {
		t.Fatalf("CheckTypes: %v", err)
	}
	return tc
}

func TestCheckTypes_voidEnsure_infersResultVoid(t *testing.T) {
	t.Parallel()
	src := `package main

error E { message: String }

func need(ok Bool) {
	ensure ok is True()
		else E({message: "bad"})
}

func main() {}
`
	tc := typecheckUpstream(t, src)
	sig := tc.Functions["need"]
	if len(sig.ReturnTypes) != 1 || !sig.ReturnTypes[0].IsResultType() {
		t.Fatalf("need return = %v, want Result(Void, Error)", formatTypeList(sig.ReturnTypes))
	}
	if sig.ReturnTypes[0].TypeParams[0].Ident != ast.TypeVoid {
		t.Fatalf("success = %s, want Void", sig.ReturnTypes[0].TypeParams[0].Ident)
	}
}

func TestCheckTypes_voidResult_ensureOk(t *testing.T) {
	t.Parallel()
	src := `package main

error E { message: String }

func need(ok Bool) {
	ensure ok is True()
		else E({message: "bad"})
}

func Run(ok Bool) {
	err := need(ok)
	ensure err is Ok()
	return 1
}

func main() {
	r := Run(true)
	ensure r is Ok()
	println(string(r))
}
`
	_ = typecheckUpstream(t, src)
}

func TestCheckTypes_bareEnsureBool(t *testing.T) {
	t.Parallel()
	src := `package main

error E { message: String }

func check(b Bool) {
	ensure b
		else E({message: "no"})
	return 1
}

func main() {
	x := check(true)
	ensure x is Ok()
	println(string(x))
}
`
	_ = typecheckUpstream(t, src)
}

func TestCheckTypes_okUnwrap_propagatesResult(t *testing.T) {
	t.Parallel()
	src := `package main

error E { message: String }

func inner(ok Bool) {
	ensure ok is True()
		else E({message: "bad"})
	return "x"
}

func outerNoPropagate(ok Bool) {
	name := inner(ok)
	ensure name is Ok()
	return 1
}

func main() {
	r := outerNoPropagate(true)
	ensure r is Ok()
	println(r)
}
`
	tc := typecheckUpstream(t, src)
	sig := tc.Functions["outerNoPropagate"]
	if len(sig.ReturnTypes) != 1 || !sig.ReturnTypes[0].IsResultType() {
		t.Fatalf("outerNoPropagate return = %v, want Result(Int, Error)", formatTypeList(sig.ReturnTypes))
	}
	if sig.ReturnTypes[0].TypeParams[0].Ident != ast.TypeInt {
		t.Fatalf("success = %s, want Int", sig.ReturnTypes[0].TypeParams[0].Ident)
	}
}

func TestCheckTypes_mainEnsureOk_staysVoid(t *testing.T) {
	t.Parallel()
	src := `package main

func okInt() {
	n := 42
	ensure n is GreaterThan(0)
	return n
}

func main() {
	x := okInt()
	ensure x is Ok()
	println(x)
}
`
	tc := typecheckUpstream(t, src)
	sig := tc.Functions["main"]
	if !IsVoidReturnTypes(sig.ReturnTypes) {
		t.Fatalf("main return = %v, want void", formatTypeList(sig.ReturnTypes))
	}
}

func TestCheckTypes_sliceAlias_mutualMethods(t *testing.T) {
	t.Parallel()
	src := `package main

type ExprList = []String

type P = { n: Int }

func (p *P) a(): ExprList {
	if p.n == 0 {
		return []String{}
	}
	return p.b()
}

func (p *P) b(): ExprList {
	xs := p.a()
	return append(xs, "y")
}

func main() {
	p := &P{n: 0}
	println(len(p.a()))
}
`
	_ = typecheckUpstream(t, src)
}

func TestCheckTypes_mutualRecursion_string(t *testing.T) {
	t.Parallel()
	src := `package main

type P = { n: Int }

func (p *P) a(): String {
	if p.n == 0 {
		return "x"
	}
	return p.b()
}

func (p *P) b(): String {
	return p.a()
}

func main() {
	p := &P{n: 0}
	println(p.a())
}
`
	_ = typecheckUpstream(t, src)
}

func TestCheckTypes_ensureElseMethod(t *testing.T) {
	t.Parallel()
	src := `package main

error E { message: String }

type P = { n: Int }

func (p *P) errMsg(msg String): E {
	return E({message: msg})
}

func (p *P) badMethod(ok Bool) {
	ensure ok is True()
		else p.errMsg("bad")
}

func (p *P) badConcat(ok Bool, kw String) {
	ensure ok is True()
		else E({message: "bad " + kw})
}

func Run(ok Bool) {
	p := &P{n: 0}
	err := p.badMethod(ok)
	ensure !err
	return 1
}

func main() {
	r := Run(true)
	ensure r is Ok()
	println(string(r))
}
`
	_ = typecheckUpstream(t, src)
}

func TestCheckTypes_bareEnsureResult(t *testing.T) {
	t.Parallel()
	src := `package main

error E { message: String }

func need(ok Bool) {
	ensure ok is True()
		else E({message: "bad"})
}

func Run(ok Bool) {
	err := need(ok)
	ensure err
	return 1
}

func main() {
	r := Run(true)
	ensure r is Ok()
	println(string(r))
}
`
	_ = typecheckUpstream(t, src)
}

func TestCheckTypes_bangEnsure_valuedResult_errors(t *testing.T) {
	t.Parallel()
	src := `package main

func f(x Result(Int, Error)) {
	ensure !x
}

func main() {}
`
	log := logrus.New()
	log.SetLevel(logrus.PanicLevel)
	toks := lexer.New([]byte(src), "t.ft", log).Lex()
	nodes, err := parser.New(toks, "t.ft", log).ParseFile()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tc := New(log, false)
	err = tc.CheckTypes(nodes)
	if err == nil {
		t.Fatal("expected ensure-bang-result for ensure !x on Result(Int, Error)")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ensure-bang-result") {
		t.Fatalf("expected ensure-bang-result, got: %v", err)
	}
	if !strings.Contains(msg, "is Ok()") || !strings.Contains(msg, "is Err()") {
		t.Fatalf("expected is Ok() / is Err() rewrite, got: %v", err)
	}
}

func TestCheckTypes_annotatedError_staysError(t *testing.T) {
	t.Parallel()
	src := `package main

error E { message: String }

func need(ok Bool): Error {
	ensure ok is True()
		else E({message: "bad"})
	return nil
}

func main() {}
`
	tc := typecheckUpstream(t, src)
	sig := tc.Functions["need"]
	if len(sig.ReturnTypes) != 1 || sig.ReturnTypes[0].Ident != ast.TypeError {
		t.Fatalf("need return = %v, want Error", formatTypeList(sig.ReturnTypes))
	}
}
