package typechecker

import (
	"testing"

	"forst/internal/ast"
	"forst/internal/parser"
)

func TestErrorSet_directNominalEnsure(t *testing.T) {
	t.Parallel()
	src := `package main

error CellTaken { row: Int, col: Int }

func PlayMove(row Int, col Int) {
	ensure row is GreaterThan(-1) else CellTaken{ row: row, col: col }
}
`
	tc := typecheckErrorSetSource(t, src)
	set := tc.Functions["PlayMove"].ErrorSet
	if len(set.NominalErrors) != 1 || set.NominalErrors[0] != "CellTaken" {
		t.Fatalf("NominalErrors = %v, want [CellTaken]", set.NominalErrors)
	}
	if set.UnknownPossible {
		t.Fatal("expected UnknownPossible false")
	}
}

func TestErrorSet_transitiveCall(t *testing.T) {
	t.Parallel()
	src := `package main

error E1 { msg: String }

func inner() {
	ok := false
	ensure ok is True() else E1{ msg: "a" }
}

func outer() {
	inner()
}
`
	tc := typecheckErrorSetSource(t, src)
	set := tc.Functions["outer"].ErrorSet
	if len(set.NominalErrors) != 1 || set.NominalErrors[0] != "E1" {
		t.Fatalf("outer NominalErrors = %v, want [E1]", set.NominalErrors)
	}
}

func TestErrorSet_unknownGenericHelper(t *testing.T) {
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
`
	tc := typecheckErrorSetSource(t, src)
	set := tc.Functions["f"].ErrorSet
	if !set.UnknownPossible {
		t.Fatal("expected UnknownPossible true for non-error helper used as ensure fallback")
	}
	if len(set.NominalErrors) != 0 {
		t.Fatalf("expected no nominal errors, got %v", set.NominalErrors)
	}
}

func TestErrorSet_unionAlias(t *testing.T) {
	t.Parallel()
	src := `package main

error ParseError { code: Int }
error IoError { path: String }
type ErrKind = ParseError | IoError

func load() {
	ok := false
	ensure ok is True() else ParseError{ code: 1 }
}
`
	tc := typecheckErrorSetSource(t, src)
	set := tc.Functions["load"].ErrorSet
	if len(set.NominalErrors) != 1 || set.NominalErrors[0] != "ParseError" {
		t.Fatalf("NominalErrors = %v, want [ParseError]", set.NominalErrors)
	}
}

func TestErrorSet_ensureWithoutOrMarksUnknown(t *testing.T) {
	t.Parallel()
	src := `package main

func f() {
	ok := false
	ensure ok is True()
}
`
	tc := typecheckErrorSetSource(t, src)
	set := tc.Functions["f"].ErrorSet
	if !set.UnknownPossible {
		t.Fatal("expected UnknownPossible when ensure has no or clause")
	}
}

func typecheckErrorSetSource(t *testing.T, src string) *TypeChecker {
	t.Helper()
	log := ast.SetupTestLogger(nil)
	nodes, err := parser.NewTestParser(src, log).ParseFile()
	if err != nil {
		t.Fatal(err)
	}
	tc := New(log, false)
	if err := tc.CheckTypes(nodes); err != nil {
		t.Fatalf("CheckTypes: %v", err)
	}
	return tc
}
