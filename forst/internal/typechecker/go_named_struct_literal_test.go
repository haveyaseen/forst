package typechecker

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"forst/internal/ast"
	"forst/internal/lexer"
	"forst/internal/parser"
	"forst/internal/testmod"

	"github.com/sirupsen/logrus"
)

func TestSamePackageGo_structLiteralAssignable(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	testmod.WriteGoMod(t, root, "example.com/baglit")
	pkgDir := filepath.Join(root, "app")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "helpers.go"), []byte(`package main

type Bag struct {
	Name string
}

func BagName(b Bag) string { return b.Name }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `package main

func main() {
	b := Bag{Name: "x"}
	println(BagName(b))
}
`
	log := logrus.New()
	log.SetLevel(logrus.PanicLevel)
	toks := lexer.New([]byte(src), "app/main.ft", log).Lex()
	nodes, err := parser.New(toks, "app/main.ft", log).ParseFile()
	if err != nil {
		t.Fatal(err)
	}
	tc := New(log, false)
	tc.GoWorkspaceDir = root
	tc.SetSamePackageGoImportPath("example.com/baglit/app")
	if err := tc.CheckTypes(nodes); err != nil {
		t.Fatalf("expected Bag literal assignable to Go Bag: %v", err)
	}
}

func TestSamePackageGo_namedSliceAliasFromGo(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	testmod.WriteGoMod(t, root, "example.com/exprlist")
	pkgDir := filepath.Join(root, "app")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "helpers.go"), []byte(`package main

type Expr struct {
	Kind string
}

type ExprList []Expr

func EmptyExprs() ExprList { return nil }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `package main

func main() {
	xs := EmptyExprs()
	println(len(xs))
}
`
	log := logrus.New()
	log.SetLevel(logrus.PanicLevel)
	toks := lexer.New([]byte(src), "app/main.ft", log).Lex()
	nodes, err := parser.New(toks, "app/main.ft", log).ParseFile()
	if err != nil {
		t.Fatal(err)
	}
	tc := New(log, false)
	tc.GoWorkspaceDir = root
	tc.SetSamePackageGoImportPath("example.com/exprlist/app")
	if err := tc.CheckTypes(nodes); err != nil {
		t.Fatalf("expected Go named slice ExprList usable from Forst: %v", err)
	}
	vt, ok := tc.VariableTypes["xs"]
	if !ok || len(vt) != 1 {
		t.Fatalf("xs types: %+v ok=%v", vt, ok)
	}
	if vt[0].Ident != "ExprList" {
		// Prefer named slice; underlying Array(String) after alias resolve is also acceptable.
		resolved := tc.resolveTypeAliasChain(vt[0])
		if resolved.Ident != ast.TypeArray {
			t.Fatalf("xs type = %v, want ExprList or Array", formatTypeList(vt))
		}
	}
}

func TestSamePackageGo_forstFuncTakesGoNamedType(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	testmod.WriteGoMod(t, root, "example.com/bagsig")
	pkgDir := filepath.Join(root, "app")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "helpers.go"), []byte(`package main

type Bag struct {
	Name string
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `package main

func ForstReturnsBag(): Bag {
	return Bag{Name: "x"}
}

func ForstTakesBag(b Bag): String {
	return b.Name
}

func main() {
	println(ForstTakesBag(ForstReturnsBag()))
}
`
	log := logrus.New()
	log.SetLevel(logrus.PanicLevel)
	toks := lexer.New([]byte(src), "app/main.ft", log).Lex()
	nodes, err := parser.New(toks, "app/main.ft", log).ParseFile()
	if err != nil {
		t.Fatal(err)
	}
	tc := New(log, false)
	tc.GoWorkspaceDir = root
	tc.SetSamePackageGoImportPath("example.com/bagsig/app")
	if err := tc.CheckTypes(nodes); err != nil {
		t.Fatalf("expected Forst funcs with Go Bag in signature: %v", err)
	}
}

func TestSamePackageGo_forstTypeConflictsWithGoNamedType_errors(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	testmod.WriteGoMod(t, root, "example.com/bagconflict")
	pkgDir := filepath.Join(root, "app")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "helpers.go"), []byte(`package main

type Bag struct {
	Name string
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `package main

type Bag = {
	X: Int
}

func main() {
	println("ok")
}
`
	log := logrus.New()
	log.SetLevel(logrus.PanicLevel)
	toks := lexer.New([]byte(src), "app/main.ft", log).Lex()
	nodes, err := parser.New(toks, "app/main.ft", log).ParseFile()
	if err != nil {
		t.Fatal(err)
	}
	tc := New(log, false)
	tc.GoWorkspaceDir = root
	tc.SetSamePackageGoImportPath("example.com/bagconflict/app")
	err = tc.CheckTypes(nodes)
	if err == nil {
		t.Fatal("expected duplicate-type error for conflicting Forst/Go Bag")
	}
	var diag *Diagnostic
	if !errors.As(err, &diag) || diag == nil || diag.Code != "duplicate-type" {
		t.Fatalf("expected duplicate-type diagnostic, got %T: %v", err, err)
	}
}

func TestSamePackageGo_forstTypeCompatibleWithGoNamedType_marksOmit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	testmod.WriteGoMod(t, root, "example.com/bagcompat")
	pkgDir := filepath.Join(root, "app")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "helpers.go"), []byte(`package main

type Bag struct {
	Name string
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `package main

type Bag = {
	Name: String
}

func main() {
	println("ok")
}
`
	log := logrus.New()
	log.SetLevel(logrus.PanicLevel)
	toks := lexer.New([]byte(src), "app/main.ft", log).Lex()
	nodes, err := parser.New(toks, "app/main.ft", log).ParseFile()
	if err != nil {
		t.Fatal(err)
	}
	tc := New(log, false)
	tc.GoWorkspaceDir = root
	tc.SetSamePackageGoImportPath("example.com/bagcompat/app")
	if err := tc.CheckTypes(nodes); err != nil {
		t.Fatalf("expected compatible Forst/Go Bag: %v", err)
	}
	if !tc.IsGoPackageType("Bag") {
		t.Fatal("expected Bag marked as go package type so emit is omitted")
	}
}
