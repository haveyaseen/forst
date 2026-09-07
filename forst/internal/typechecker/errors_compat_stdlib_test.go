package typechecker

import (
	"io"
	"testing"

	"forst/internal/ast"
	"forst/internal/lexer"
	"forst/internal/parser"

	"github.com/sirupsen/logrus"
)

func TestGoInterop_multiReturnAlwaysTuple(t *testing.T) {
	t.Parallel()
	dir := moduleRootFromWD(t)
	src := `package main
import "strconv"
func main() {
  x := strconv.Atoi("1")
  println(x)
}
`
	log := logrus.New()
	log.SetLevel(logrus.PanicLevel)
	toks := lexer.New([]byte(src), "t.ft", log).Lex()
	nodes, err := parser.New(toks, "t.ft", log).ParseFile()
	if err != nil {
		t.Fatal(err)
	}
	tc := New(log, false)
	tc.GoWorkspaceDir = dir
	if err := tc.CheckTypes(nodes); err != nil {
		t.Fatalf("typecheck: %v", err)
	}
	for _, n := range nodes {
		fn, ok := n.(ast.FunctionNode)
		if !ok || fn.Ident.ID != "main" {
			continue
		}
		asg := fn.Body[0].(ast.AssignmentNode)
		x := asg.LValues[0].(ast.VariableNode)
		types, _ := tc.LookupInferredType(x, true)
		if len(types) != 1 || !types[0].IsTupleType() {
			t.Fatalf("want Tuple(Int, Error), got %v", types)
		}
		if len(types[0].TypeParams) != 2 {
			t.Fatalf("want 2-tuple, got %v", types[0])
		}
		return
	}
	t.Fatal("main not found")
}

func TestErrorsCompat_sentinelIsEOF(t *testing.T) {
	t.Parallel()
	dir := moduleRootFromWD(t)
	src := `package main
import "errors"
import "io"
func main() {
  err := io.EOF
  ok := errors.Is(err, io.EOF)
  println(ok)
}
`
	typecheckSource(t, dir, src)
}

func TestErrorsCompat_wrappedChainFromGoSurvives(t *testing.T) {
	t.Parallel()
	dir := moduleRootFromWD(t)
	src := `package main
import "errors"
import "fmt"
func main() {
  err := fmt.Errorf("wrap: %w", fmt.Errorf("root"))
  ok := errors.Is(err, fmt.Errorf("root"))
  println(ok)
}
`
	typecheckSource(t, dir, src)
}

func TestErrorsCompat_forstNominalErrorUnwraps_typechecks(t *testing.T) {
	t.Parallel()
	dir := moduleRootFromWD(t)
	src := `package main
import "errors"
import "io"
error WrapErr { cause: Error, msg: String }
func main() {
  e := WrapErr{ cause: io.EOF, msg: "eof" }
  ok := errors.Is(e, io.EOF)
  println(ok)
}
`
	typecheckSource(t, dir, src)
}

func TestErrorsCompat_sentinelIsEOF_goTypes(t *testing.T) {
	t.Parallel()
	if io.EOF == nil {
		t.Fatal("sanity: io.EOF must be non-nil")
	}
}

func typecheckSource(t *testing.T, dir, src string) *TypeChecker {
	t.Helper()
	log := logrus.New()
	log.SetLevel(logrus.PanicLevel)
	toks := lexer.New([]byte(src), "t.ft", log).Lex()
	nodes, err := parser.New(toks, "t.ft", log).ParseFile()
	if err != nil {
		t.Fatal(err)
	}
	tc := New(log, false)
	tc.GoWorkspaceDir = dir
	if err := tc.CheckTypes(nodes); err != nil {
		t.Fatalf("typecheck: %v", err)
	}
	return tc
}
