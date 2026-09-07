package typechecker

import (
	"strings"
	"testing"

	"forst/internal/ast"
	"forst/internal/testutil"
)

func TestUnifyTypeguard_rejectsNominalErrorBareIsGuard(t *testing.T) {
	src := `package main

error ParseError { code: Int }

func main() {
	x := mk()
	if x is ParseError {
		println(x)
	}
}

func mk(): Result(Int, ParseError) {
	return 0
}
`
	_, _, err := Typecheck(t, src, testutil.TypecheckOpts{UseModuleRoot: true})
	if err == nil {
		t.Fatal("expected error for bare nominal error is guard")
	}
	if !strings.Contains(err.Error(), "nominal-error-not-is-guard") && !strings.Contains(err.Error(), "not an `is` guard") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnifyTypeguard_presentRejectsNonNilable(t *testing.T) {
	src := `package main

func main() {
	n := 1
	if n is Present() {
		println(n)
	}
}
`
	_, _, err := Typecheck(t, src, testutil.TypecheckOpts{})
	if err == nil {
		t.Fatal("expected error for Present on non-nilable")
	}
	if !strings.Contains(err.Error(), "present assertion requires a pointer, map, or array") &&
		!strings.Contains(err.Error(), "present assertion requires a pointer") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnifyTypeguard_nilRejectsResult(t *testing.T) {
	src := `package main

func f(x Result(Void, Error)) {
	ensure x is Nil()
}

func main() {}
`
	_, _, err := Typecheck(t, src, testutil.TypecheckOpts{})
	if err == nil {
		t.Fatal("expected error for Nil on Result")
	}
	if !strings.Contains(err.Error(), "ensure-nil-result") && !strings.Contains(err.Error(), "Nil()") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnifyTypeguard_presentAllowsMap(t *testing.T) {
	src := `package main

func main() {
	m := map[String]Int{}
	if m is Present() {
		println(len(m))
	}
}
`
	MustTypecheck(t, src, testutil.TypecheckOpts{})
}

func TestUnifyTypeguard_presentAllowsArray(t *testing.T) {
	src := `package main

func main() {
	xs := []Int{}
	if xs is Present() {
		println(len(xs))
	}
}
`
	MustTypecheck(t, src, testutil.TypecheckOpts{})
}

func TestUnifyTypeguard_okDiscriminatorOnResult(t *testing.T) {
	src := `package main

func main() {
	x := mk()
	if x is Ok() {
		println(x)
	}
}

func mk(): Result(Int, String) {
	return 0
}
`
	MustTypecheck(t, src, testutil.TypecheckOpts{})
}

func TestUnifyTypeguard_typeGuardSubjectMismatch(t *testing.T) {
	tc := New(setupTestLogger(nil), false)
	tc.Defs["Positive"] = ast.TypeGuardNode{
		Ident: ast.Identifier("Positive"),
		Subject: ast.SimpleParamNode{
			Ident: ast.Ident{ID: "n"},
			Type:  ast.TypeNode{Ident: ast.TypeInt},
		},
	}
	err := tc.validateAssertionNode(ast.AssertionNode{
		Constraints: []ast.ConstraintNode{{Name: "Positive"}},
	}, ast.TypeNode{Ident: ast.TypeString}, ast.FakeSpan())
	if err == nil {
		t.Fatal("expected type guard subject mismatch error")
	}
	if !strings.Contains(err.Error(), "type guard 'Positive' requires subject type Int") {
		t.Fatalf("unexpected error: %v", err)
	}
}
