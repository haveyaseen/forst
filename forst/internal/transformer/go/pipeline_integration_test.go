package transformergo

import (
	"bytes"
	"go/format"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"forst/internal/ast"
	"forst/internal/goload"
	"forst/internal/parser"
	"forst/internal/testutil"
	"forst/internal/typechecker"
)

// pipelineOpts configures compileForstPipelineExt (Go workspace / optional skip for FFI tests).
type pipelineOpts struct {
	goWorkspaceDir      string
	forstFileDir        string
	samePackageGoImport string
	skipUnlessGoImport  string // if set, t.Skip when go/packages did not load this import local name
	skipUnlessDotImport bool   // if true, t.Skip when go/packages did not populate dot-import packages
}

func pipelineOptsToCompile(opts pipelineOpts) testutil.CompileOpts {
	return testutil.CompileOpts{
		TypecheckOpts: testutil.TypecheckOpts{
			GoWorkspaceDir:      opts.goWorkspaceDir,
			ForstFileDir:        opts.forstFileDir,
			SamePackageGoImport: opts.samePackageGoImport,
			SkipUnlessGoImport:  opts.skipUnlessGoImport,
			SkipUnlessDotImport: opts.skipUnlessDotImport,
		},
	}
}

// compileForstPipeline runs parse → typecheck → transform → generators.GenerateGoCode on Forst source.
func compileForstPipeline(t *testing.T, src string) string {
	t.Helper()
	return compileForstPipelineExt(t, src, pipelineOpts{})
}

// compileForstPipelineExt runs parse → typecheck → transform → generators.GenerateGoCode with optional GoWorkspaceDir.
func compileForstPipelineExt(t *testing.T, src string, opts pipelineOpts) string {
	t.Helper()
	return MustCompileGo(t, src, pipelineOptsToCompile(opts))
}

// compileMergedForstFilesPipeline merges multiple .ft files in one package, then typechecks and transforms.
func compileMergedForstFilesPipeline(t *testing.T, paths []string, opts pipelineOpts) string {
	t.Helper()
	return MustCompileMergedGo(t, paths, pipelineOptsToCompile(opts))
}

func moduleRootFromWD(t *testing.T) string {
	t.Helper()
	return testutil.ModuleRoot(t)
}

// pipelineOptsForExampleFile returns compile options for examples that need go/packages (e.g. os/exec).
func pipelineOptsForExampleFile(t *testing.T, name string) pipelineOpts {
	if name != "go_interop/cli.ft" {
		return pipelineOpts{}
	}
	path := testutil.ExamplePath(t, name)
	dir := filepath.Dir(path)
	return pipelineOpts{
		goWorkspaceDir:      goload.FindModuleRoot(dir),
		forstFileDir:        dir,
		samePackageGoImport: "go_interop",
		skipUnlessGoImport:  "exec",
	}
}

func TestPipeline_shapeValueMutate_preservesReturnVariable(t *testing.T) {
	t.Parallel()
	src := `package main
type Acc = {
	n: Int
}
func bump(a Acc): Acc {
	a.n = a.n + 1
	return a
}
func main() {
	a := Acc{n: 0}
	a = bump(a)
	println(a.n)
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, "return a") {
		t.Fatalf("expected return of mutated variable, got:\n%s", out)
	}
	if strings.Contains(out, "return Acc{") {
		t.Fatalf("must not rebuild zero Acc literal on return, got:\n%s", out)
	}
}

func TestPipeline_sameShapeDistinctNamedTypes_usesReturnWrap(t *testing.T) {
	t.Parallel()
	src := `package main
type Acc = {
	n: Int
}
type Box = {
	n: Int
}
func asBox(a Acc): Box {
	return a
}
func main() {
	b := asBox(Acc{n: 1})
	println(b.n)
}
`
	out := compileForstPipeline(t, src)
	if strings.Contains(out, "return a") {
		t.Fatalf("distinct same-shaped named types must not bare-return Acc as Box, got:\n%s", out)
	}
	if !strings.Contains(out, "Box{") {
		t.Fatalf("expected wrapping conversion into Box, got:\n%s", out)
	}
}

func TestPipeline_parse_typecheck_transform_goFormat(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		needles []string // substrings that must appear in generated Go (stable signals)
	}{
		{
			name: "basic_function_and_return",
			src: `package main

func greet(): String {
	return "Hello"
}

func main() {
	println(greet())
}
`,
			needles: []string{`package main`, `func greet`, `return "Hello"`, `func main`},
		},
		{
			name: "import_fmt_and_call",
			src: `package main

import "fmt"

func main() {
	fmt.Println("ok")
}
`,
			needles: []string{`package main`, `"fmt"`, `fmt.Println`, `ok`},
		},
		{
			name: "type_def_and_struct_literal_return",
			src: `package main

type Point = { x: Int, y: Int }

func origin(): Point {
	return { x: 0, y: 1 }
}

func main() {
	p := origin()
	println("ok")
}
`,
			// Struct literal return + named shape type; avoid println(int): builtin println expects String.
			needles: []string{`type Point`, `func origin`, `return`, `struct`},
		},
		{
			name: "type_guard_and_ensure_block",
			src: `package main

type Password = String

is (password Password) Strong {
	ensure password is Min(12)
}

func main() {
	password: Password = "1234567890123"
	ensure password is Strong() else {
		println("weak")
	}
	println("done")
}
`,
			// Guard implementations are emitted as func G_<hash>(...) bool; ensure block uses os.Exit on failure.
			needles: []string{`package main`, `func G_`, `Password`, `os.Exit`, `func main`},
		},
		{
			name: "arithmetic_int_return",
			src: `package main

func sum(): Int {
	return 3 + 4
}

func main() {
	println("ok")
}
`,
			// Binary `+` is inferred in sum; main stays string-only for println.
			needles: []string{`func sum`, `+`, `return`, `func main`},
		},
		{
			name: "if_else_branch",
			src: `package main

func main() {
	n := 1
	if n > 0 {
		println("yes")
	} else {
		println("no")
	}
}
`,
			needles: []string{`if `, `else`, `println`, `func main`},
		},
		{
			name: "if_else_if_else_chain",
			src: `package main

func main() {
	n := 2
	if n > 10 {
		println("a")
	} else if n < 0 {
		println("b")
	} else {
		println("c")
	}
}
`,
			// Emitter lowers else-if to nested Go if statements.
			needles: []string{`if `, `else`, `println("c")`, `func main`},
		},
		{
			name: "if_with_short_decl_init",
			src: `package main

func main() {
	if x := 1; x > 0 {
		println("ok")
	}
}
`,
			needles: []string{`if `, `x :=`, `println`, `func main`},
		},
		{
			name: "for_range_over_slice",
			src: `package main

func main() {
	xs := [1, 2]
	for range xs {
		println("r")
	}
}
`,
			needles: []string{`for `, `range`, `println`, `func main`},
		},
		{
			name: "defer_and_go_statements",
			src: `package main

func work() {}

func main() {
	defer work()
	go work()
	println("ok")
}
`,
			needles: []string{`defer work()`, `go work()`, `func main`},
		},
		{
			name: "builtin_len_string",
			src: `package main

func main() {
	println(len("hi"))
}
`,
			needles: []string{`package main`, `len("hi")`, `println`},
		},
		{
			name: "builtin_len_array_literal",
			src: `package main

func main() {
	println(len([1, 2, 3]))
}
`,
			needles: []string{`len(`, `func main`},
		},
		{
			name: "builtin_min_max_literals",
			src: `package main

func main() {
	println(min(1, 2, 3))
	println(max(3, 4))
}
`,
			needles: []string{`min(`, `max(`, `func main`},
		},
		{
			name: "slice_shape_field_and_param_emit_go_slice",
			src: `package main

type Row = { cells: []String }

func getCell(cells []String, idx Int): String {
	return cells[idx]
}

func main() {
	println(getCell(["x"], 0))
}
`,
			// Shape fields and parameters must use Go []string, not a hash-only type name (would break indexing).
			needles: []string{`type Row`, `[]string`, `cells []string`, `cells[idx]`},
		},
		{
			name: "return_user_defined_struct_type_via_call_expr",
			src: `package main

type R = { ok: Bool }

func inner(): R {
	return { ok: true }
}

func outer(): R {
	return inner()
}

func main() {
	x := outer()
	println(x.ok)
}
`,
			needles: []string{`func inner`, `func outer`, `return inner()`, `type R`},
		},
		{
			name: "ensure_greater_than_negative_int_literal_with_or",
			src: `package main

import "errors"

func Bad(msg String): Error {
	return errors.New(msg)
}

func f(row Int): Result(String, Error) {
	ensure row is GreaterThan(-1) else Bad("x")
	return "ok"
}

func main() {
	println("hi")
}
`,
			needles: []string{`-1`, `row`, `errors`},
		},
		{
			name: "return_multi_value_call_single_expr_not_padded_with_nil",
			src: `package main

func inner(): Result(Int, Error) {
	return 1
}

func outer(): Result(Int, Error) {
	return inner()
}

func main() {
	x := outer()
	println(x)
}
`,
			needles: []string{`return inner()`, `func outer`},
		},
		{
			name: "for_three_clause_loop",
			src: `package main

func main() {
	for i := 0; i < 2; i++ {
		println("x")
	}
}
`,
			needles: []string{`for `, `i++`, `func main`},
		},
		{
			name: "pointer_address_and_deref",
			src: `package main

func main() {
	s := "a"
	p := &s
	println(*p)
}
`,
			needles: []string{`&s`, `*p`, `func main`},
		},
		{
			name: "float_literal_arithmetic",
			src: `package main

func main() {
	x := 1.5 + 2.5
	println("ok")
}
`,
			needles: []string{`1.5`, `+`, `func main`},
		},
		{
			name: "map_literal",
			src: `package main

func main() {
	scores := map[String]Int{ "a": 1, "b": 2 }
	println("k")
}
`,
			needles: []string{`map[string]int`, `func main`},
		},
		{
			name: "logical_and_expression",
			src: `package main

func main() {
	if true && true {
		println("y")
	}
}
`,
			needles: []string{`&&`, `func main`},
		},
		{
			name: "ensure_string_contains_builtin",
			src: `package main

func main() {
	s := "hello"
	ensure s is Contains("ell")
	println("ok")
}
`,
			needles: []string{`strings.Contains`, `"strings"`, `func main`},
		},
		{
			name: "ensure_float_greater_than_main",
			src: `package main

func main() {
	x := 1.5
	ensure x is GreaterThan(1.0)
	println("ok")
}
`,
			needles: []string{`1.5`, `os.Exit`, `func main`},
		},
		{
			name: "ensure_array_min_length_main",
			src: `package main

func main() {
	xs := [1, 2]
	ensure xs is Min(1)
	println("ok")
}
`,
			needles: []string{`len(xs)`, `os.Exit`, `func main`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := compileForstPipeline(t, tt.src)
			for _, sub := range tt.needles {
				if !strings.Contains(out, sub) {
					t.Fatalf("generated Go missing %q\n----\n%s\n----", sub, out)
				}
			}
			if tt.name == "ensure_float_greater_than_main" {
				if !strings.Contains(out, `!(x > 1`) && !strings.Contains(out, `x <= 1`) {
					t.Fatalf("expected GreaterThan check\n----\n%s\n----", out)
				}
			}
		})
	}
}

func TestPipeline_dot_import_strings(t *testing.T) {
	dir := moduleRootFromWD(t)
	src := `package main

import . "strings"

func main() {
	println(Contains("a", "b"))
}
`
	out := compileForstPipelineExt(t, src, pipelineOpts{
		goWorkspaceDir:      dir,
		skipUnlessDotImport: true,
	})
	for _, sub := range []string{`package main`, `. "strings"`, `Contains`, `"a"`} {
		if !strings.Contains(out, sub) {
			t.Fatalf("generated Go missing %q\n----\n%s\n----", sub, out)
		}
	}
}

// TestEmitValidation_* cases assert generated Go for built-in constraints and type guards (grep-friendly).
func TestPipeline_return_multi_value_call_not_padded_with_nil(t *testing.T) {
	src := `package main

func inner(): Result(Int, Error) {
	return 1
}

func outer(): Result(Int, Error) {
	return inner()
}

func main() {
	x := outer()
	println(x)
}
`
	out := compileForstPipeline(t, src)
	if strings.Contains(out, "inner(), nil") {
		t.Fatalf("multi-value return must not be padded (invalid Go); got:\n%s", out)
	}
	if !strings.Contains(out, "return inner()") {
		t.Fatalf("expected `return inner()` in:\n%s", out)
	}
}

func TestEmitValidation_builtinMinOnString(t *testing.T) {
	src := `package main

func checkLen(name String) {
	ensure name is Min(1)
}

func main() {
	checkLen("hi")
	println("ok")
}
`
	out := compileForstPipeline(t, src)
	for _, sub := range []string{`func checkLen`, `utf8.RuneCountInString`, `String.Min(1)`, `errors.New`, `package main`} {
		if !strings.Contains(out, sub) {
			t.Fatalf("generated Go missing %q\n----\n%s\n----", sub, out)
		}
	}
}

func TestEmitValidation_ifIsMinOnString(t *testing.T) {
	src := `package main

func checkLen(name String) {
	if name is Min(1) {
		println("ok")
	} else {
		println("short")
	}
}

func main() {
	checkLen("hi")
}
`
	out := compileForstPipeline(t, src)
	for _, sub := range []string{`func checkLen`, `utf8.RuneCountInString`, `package main`} {
		if !strings.Contains(out, sub) {
			t.Fatalf("generated Go missing %q\n----\n%s\n----", sub, out)
		}
	}
}

func TestEmitValidation_builtinLessThanOnInt(t *testing.T) {
	src := `package main

func capSpeed(speed Int) {
	ensure speed is LessThan(100)
}

func main() {
	capSpeed(50)
	println("ok")
}
`
	out := compileForstPipeline(t, src)
	for _, sub := range []string{`func capSpeed`, `100`, `package main`} {
		if !strings.Contains(out, sub) {
			t.Fatalf("generated Go missing %q\n----\n%s\n----", sub, out)
		}
	}
}

func TestEmitValidation_typeGuardStrongPassword(t *testing.T) {
	src := `package main

type Password = String

is (password Password) Strong {
	ensure password is Min(12)
}

func main() {
	password: Password = "1234567890123"
	ensure password is Strong() else {
		println("weak")
	}
	println("ok")
}
`
	out := compileForstPipeline(t, src)
	for _, sub := range []string{`func G_`, `utf8.RuneCountInString`, `Password`, `os.Exit`, `package main`} {
		if !strings.Contains(out, sub) {
			t.Fatalf("generated Go missing %q\n----\n%s\n----", sub, out)
		}
	}
}

func TestEmitValidation_ensureBoolTrue(t *testing.T) {
	src := `package main

func main() {
	ok := true
	ensure ok is True()
	println("ok")
}
`
	out := compileForstPipeline(t, src)
	// Bool True() lowers to a direct boolean check on the subject.
	if !strings.Contains(out, `!ok`) || !strings.Contains(out, `package main`) {
		t.Fatalf("expected negated ok check for True(), got:\n%s", out)
	}
}

func TestEmitValidation_stringHasPrefix(t *testing.T) {
	src := `package main

func main() {
	u := "https://x"
	ensure u is HasPrefix("https://")
	println("ok")
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `strings.HasPrefix`) || !strings.Contains(out, `"strings"`) {
		t.Fatalf("expected strings import and HasPrefix, got:\n%s", out)
	}
}

func TestEmitValidation_stringNotEmpty(t *testing.T) {
	src := `package main

func main() {
	s := "a"
	ensure s is NotEmpty()
	println(s)
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `len(s) != 0`) && !strings.Contains(out, `!(len(s) != 0)`) && !strings.Contains(out, `len(s) == 0`) {
		t.Fatalf("expected NotEmpty length check, got:\n%s", out)
	}
}

func TestPipeline_singleAssignResultCall_emitsTwoValueAssignAndPrintln(t *testing.T) {
	src := `package main

func f(): Result(Int, Error) {
	return 1
}

func main() {
	x := f()
	println(x)
}
`
	out := compileForstPipeline(t, src)
	for _, sub := range []string{
		`x, xErr :=`,
		`println(x, xErr)`,
		`func f()`,
	} {
		if !strings.Contains(out, sub) {
			t.Fatalf("generated Go missing %q\n----\n%s\n----", sub, out)
		}
	}
}

func TestPipeline_discardedResultCallStmt_emitsBareCallExprStmt(t *testing.T) {
	src := `package main

func f(): Result(Int, Error) {
	return 1
}

func main() {
	f()
}
`
	out := compileForstPipeline(t, src)
	if strings.Contains(out, `_, _ = f()`) {
		t.Fatalf("did not expect blank assignment; Go allows discarding multi-return via expression statement, got:\n%s", out)
	}
	if !strings.Contains(out, "main() {\n\tf()\n") {
		t.Fatalf("expected bare f() call statement in main, got:\n%s", out)
	}
}

func TestPipeline_discardedStrconvAtoiStmt_emitsBareCallExprStmt(t *testing.T) {
	dir := moduleRootFromWD(t)
	src := `package main

import "strconv"

func main() {
	strconv.Atoi("42")
}
`
	out := compileForstPipelineExt(t, src, pipelineOpts{
		goWorkspaceDir:     dir,
		skipUnlessGoImport: "strconv",
	})
	if strings.Contains(out, `_, _ = strconv.Atoi`) {
		t.Fatalf("did not expect blank assignment; Go allows discarding multi-return via expression statement, got:\n%s", out)
	}
	if !strings.Contains(out, "main() {\n\tstrconv.Atoi(\"42\")\n") {
		t.Fatalf("expected bare strconv.Atoi call statement in main, got:\n%s", out)
	}
}

func TestPipeline_ifResultIsOk_emitsErrNilCheck(t *testing.T) {
	src := `package main

func f(): Result(Int, Error) {
	return 1
}

func main() {
	x := f()
	if x is Ok() {
		println(x)
	}
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `xErr == nil`) {
		t.Fatalf("expected `if` condition to check success error is nil, got:\n%s", out)
	}
}

func TestPipeline_shapeFieldResult_IntError_emitsStructStorageAndSelectors(t *testing.T) {
	src := `package main

type Wrap = {
	r: Result(Int, Error),
}

func okInt(): Result(Int, Error) {
	return 42
}

func main() {
	x := okInt()
	w := { r: x }
	if w.r is Ok() {
		println(w.r)
	}
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, "Err error") || !strings.Contains(out, "V") {
		t.Fatalf("expected Wrap field to lower Result to struct { V …; Err error }, got:\n%s", out)
	}
	if !strings.Contains(out, "w.r.Err == nil") {
		t.Fatalf("expected if w.r is Ok() to check w.r.Err == nil, got:\n%s", out)
	}
	if !strings.Contains(out, "println(w.r.V)") {
		t.Fatalf("expected println narrowed field to use w.r.V, got:\n%s", out)
	}
}

func TestPipeline_shapeFieldResult_println_unnarrowed_expandsVAndErr(t *testing.T) {
	src := `package main

type Wrap = {
	r: Result(Int, Error),
}

func okInt(): Result(Int, Error) {
	return 42
}

func main() {
	x := okInt()
	w := { r: x }
	println(w.r)
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, "println(w.r.V, w.r.Err)") {
		t.Fatalf("expected println on unnarrowed struct Result field to expand to V and Err, got:\n%s", out)
	}
}

func TestPipeline_shapeFieldResult_ensureOk_emitsCompoundErrCheck(t *testing.T) {
	src := `package main

type Wrap = {
	r: Result(Int, Error),
}

func okInt(): Result(Int, Error) {
	return 42
}

func main() {
	x := okInt()
	w := { r: x }
	ensure w.r is Ok()
	println(w.r)
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `!(w.r.Err == nil)`) && !strings.Contains(out, `w.r.Err != nil`) {
		t.Fatalf("expected ensure w.r is Ok() to branch on w.r.Err, got:\n%s", out)
	}
}

func TestPipeline_testFunction_ensureOnly_emitsTestingT(t *testing.T) {
	src := `package main

import "testing"

func TestEnsureOnly(t:testing.T) {
	s := "hello"
	ensure s is Contains("ell")
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, "func TestEnsureOnly(t *testing.T)") {
		t.Fatalf("expected *testing.T test signature, got:\n%s", out)
	}
	if strings.Contains(out, ") error {") {
		t.Fatalf("test function must not return error, got:\n%s", out)
	}
	if !strings.Contains(out, "t.Helper()") && !strings.Contains(out, "t.Fatalf") {
		t.Fatalf("expected t.Helper/t.Fatalf for ensure in test, got:\n%s", out)
	}
}

func TestPipeline_ensure_string_min_infersBaseType(t *testing.T) {
	src := `package main

func main() {
	s := "ab"
	ensure s is Min(1)
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, "utf8.RuneCountInString") {
		t.Fatalf("expected ensure Min on string to use utf8.RuneCountInString, got:\n%s", out)
	}
}

func TestPipeline_ensure_failure_on_Result_struct_returns_two_values_not_nil(t *testing.T) {
	// Regression: Result(S, Error) is one Forst return type; ensure failure must lower to
	// (zero S, error), not a single nil (invalid for struct S) or one return value.
	src := `package main

type Payload = { n: Int }

func f(): Result(Payload, Error) {
	x := 0
	ensure x is GreaterThan(0)
	return { n: 1 }
}

func main() {}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, "Payload{") {
		t.Fatalf("expected zero composite literal for struct success type on ensure failure, got:\n%s", out)
	}
	if !strings.Contains(out, "errors.New") {
		t.Fatalf("expected errors.New on ensure failure for Result return, got:\n%s", out)
	}
	// Old bug: transformType(Result) failed and produced a lone `return nil`
	if strings.Contains(out, "return nil") {
		t.Fatalf("ensure failure must not emit bare return nil for Result(Payload, Error), got:\n%s", out)
	}
}

func TestPipeline_ensure_contains_on_assertion_narrowed_string(t *testing.T) {
	src := `package main

type Item = { name: String.Min(1) }

func check(item Item) {
	n := item.name
	ensure n is Contains("x")
}

func main() {}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `strings.Contains(`) || !strings.Contains(out, `"x"`) {
		t.Fatalf("expected strings.Contains on assertion-narrowed string, got:\n%s", out)
	}
}

func TestPipeline_mainEnsureFailure_printsStderrBeforeExit(t *testing.T) {
	src := `package main

func f(): Result(Int, Error) {
	return 1
}

func main() {
	x := f()
	ensure x is Ok()
	println(x)
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `fmt.Fprintf(os.Stderr, "ensure failed: %v\n", xErr)`) {
		t.Fatalf("expected main ensure failure to print xErr, got:\n%s", out)
	}
	if !strings.Contains(out, "os.Exit(1)") {
		t.Fatalf("expected os.Exit(1), got:\n%s", out)
	}
}

func TestPipeline_ensureResultIsOk_emitsErrNilCheck(t *testing.T) {
	src := `package main

func f(): Result(Int, Error) {
	return 1
}

func main() {
	x := f()
	ensure x is Ok()
	println(x)
}
`
	out := compileForstPipeline(t, src)
	// Ensure runs the error path when the predicate fails: !(xErr == nil) (equivalent to xErr != nil)
	if !strings.Contains(out, `!(xErr == nil)`) && !strings.Contains(out, `xErr != nil`) {
		t.Fatalf("expected ensure failure branch on non-Ok result (negated xErr == nil), got:\n%s", out)
	}
}

func TestPipeline_ensureOk_propagatesErrorSlot(t *testing.T) {
	src := `package main

error E { message: String }

func inner(ok Bool) {
	ensure ok is True()
		else E{message: "bad"}
	return "x"
}

func outer(ok Bool) {
	name := inner(ok)
	ensure name is Ok()
	return 1
}

func main() {}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, "return 0, nameErr") && !strings.Contains(out, "return 0, nameErr\n") {
		// Accept common formatting variants of returning the Result error slot.
		if !strings.Contains(out, "nameErr") || strings.Contains(out, `errors.New("ensure name is`) {
			t.Fatalf("expected outer ensure Ok() to return nameErr, not synthetic errors.New; got:\n%s", out)
		}
		if !strings.Contains(out, "return") || !strings.Contains(out, "nameErr") {
			t.Fatalf("expected return of nameErr on Ok unwrap failure, got:\n%s", out)
		}
	}
	if strings.Contains(out, `errors.New("ensure name is Ok()`) {
		t.Fatalf("Ok unwrap must not synthesize assertion error, got:\n%s", out)
	}
}

func TestPipeline_resultNamedShapeReturn_usesDeclaredSuccessType(t *testing.T) {
	src := `package main

type A = { id: String }
type B = { id: String }

error E { message: String }

func makeB(ok Bool): Result(B, Error) {
	ensure ok is True()
		else E{message: "bad"}
	return { id: "x" }
}

func main() {}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, "return B{") {
		t.Fatalf("expected success return typed as B, got:\n%s", out)
	}
	if strings.Contains(out, "return A{") {
		t.Fatalf("must not pick structurally identical A over declared B:\n%s", out)
	}
}

func TestPipeline_shapeLiteral_pointerField_fromPointerVar(t *testing.T) {
	src := `package main

type Shard = { id: String }

func build(shardVal: *Shard): { shard: *Shard } {
	return { shard: shardVal }
}

func main() {}
`
	out := compileForstPipeline(t, src)
	if strings.Contains(out, "&&") || strings.Contains(out, "&shardVal") {
		t.Fatalf("pointer field must not double-wrap address, got:\n%s", out)
	}
	if !strings.Contains(out, "shard: shardVal") && !strings.Contains(out, "Shard: shardVal") {
		t.Fatalf("expected pointer var passed through without &, got:\n%s", out)
	}
}

func TestPipeline_fmtPrintln_resultSplitExpandsLikePrintln(t *testing.T) {
	src := `package main

import "fmt"

func f(): Result(Int, Error) {
	return 1
}

func main() {
	x := f()
	fmt.Println(x)
}
`
	out := compileForstPipeline(t, src)
	for _, sub := range []string{
		`x, xErr :=`,
		`fmt.Println(x, xErr)`,
		`import "fmt"`,
	} {
		if !strings.Contains(out, sub) {
			t.Fatalf("generated Go missing %q\n----\n%s\n----", sub, out)
		}
	}
}

func TestPipeline_mapIndex_read_emitsResultSplitAndMissingKeyIIFE(t *testing.T) {
	src := `package main

func main() {
	m := map[String]Int{ "a": 1 }
	x := m["a"]
	ensure x is Ok()
	println(string(x))
}
`
	out := compileForstPipeline(t, src)
	for _, sub := range []string{
		`x, xErr :=`,
		`missing map key`,
		`errMissingMapKey`,
		`errors.New`,
		`v, ok :=`,
		`!ok`,
	} {
		if !strings.Contains(out, sub) {
			t.Fatalf("generated Go missing %q\n----\n%s\n----", sub, out)
		}
	}
}

func TestPipeline_mapIndex_assignTarget_noResultIIFE(t *testing.T) {
	src := `package main

func main() {
	m := map[String]Int{ "a": 1 }
	m["a"] = 3
	println("ok")
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `m["a"] = 3`) {
		t.Fatalf("expected plain indexed assignment, got:\n%s", out)
	}
	if strings.Contains(out, "missing map key") {
		t.Fatalf("did not expect map-read IIFE for assign-only program, got:\n%s", out)
	}
}

func TestPipeline_mapIndex_assignThenRead_sameKey_noResultIIFEOnLHS(t *testing.T) {
	src := `package main

func main() {
	m := map[String]Int{ "a": 1 }
	m["a"] = 99
	v := m["a"]
	ensure v is Ok()
	println(string(v))
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `m["a"] = 99`) {
		t.Fatalf("expected plain indexed assignment, got:\n%s", out)
	}
	if strings.Contains(out, `m["a"] = func()`) {
		t.Fatalf("assign LHS must not be Result read IIFE, got:\n%s", out)
	}
	for _, sub := range []string{`v, vErr :=`, `missing map key`} {
		if !strings.Contains(out, sub) {
			t.Fatalf("expected map read lowering %q in:\n%s", sub, out)
		}
	}
}

func TestPipeline_mapIndex_duplicateReads_usesFuncLitCache(t *testing.T) {
	src := `package main

func main() {
	m := map[String]Int{ "a": 1 }
	a := m["a"]
	b := m["a"]
	ensure a is Ok()
	ensure b is Ok()
	println(string(a))
	println(string(b))
}
`
	log := ast.SetupTestLogger(nil)
	if !testing.Verbose() {
		log.SetOutput(bytes.NewBuffer(nil))
	}
	p := parser.NewTestParser(src, log)
	nodes, err := p.ParseFile()
	if err != nil {
		t.Fatal(err)
	}
	tc := typechecker.New(log, false)
	if err := tc.CheckTypes(nodes); err != nil {
		t.Fatal(err)
	}
	tr := New(tc, log)
	_, err = tr.TransformForstFileToGo(nodes)
	if err != nil {
		t.Fatal(err)
	}
	if tr.mapIndexCacheHits < 1 {
		t.Fatalf("expected at least one map-index FuncLit cache hit for duplicate m[\"a\"], got hits=%d", tr.mapIndexCacheHits)
	}
}

func TestPipeline_mapIndex_return_delegatesWholeResult(t *testing.T) {
	src := `package main

func lookup(): Result(Int, Error) {
	m := map[String]Int{ "a": 1 }
	return m["a"]
}

func main() {
	x := lookup()
	ensure x is Ok()
	println(string(x))
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `return func()`) && !strings.Contains(out, `return func(`) {
		t.Fatalf("expected return of map lookup to lower via func literal / delegate, got:\n%s", out)
	}
	if !strings.Contains(out, `missing map key`) {
		t.Fatalf("expected missing-key path in lowered map read, got:\n%s", out)
	}
}

func TestPipeline_emitted_go_is_gofmt_clean(t *testing.T) {
	src := `package main

func main() {
	println("x")
}
`
	log := ast.SetupTestLogger(nil)
	if !testing.Verbose() {
		log.SetOutput(bytes.NewBuffer(nil))
	}
	p := parser.NewTestParser(src, log)
	nodes, err := p.ParseFile()
	if err != nil {
		t.Fatal(err)
	}
	tc := typechecker.New(log, false)
	if err := tc.CheckTypes(nodes); err != nil {
		t.Fatal(err)
	}
	tr := New(tc, log)
	goFile, err := tr.TransformForstFileToGo(nodes)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), goFile); err != nil {
		t.Fatal(err)
	}
	formatted := buf.Bytes()
	// Second pass through gofmt must be a no-op on valid output.
	again, err := format.Source(formatted)
	if err != nil {
		t.Fatalf("format.Source: %v\n%s", err, formatted)
	}
	if !bytes.Equal(formatted, again) {
		t.Fatalf("emitted Go not stable under gofmt")
	}
}

func TestPipeline_implicitReturn_delegatesToResultCallee(t *testing.T) {
	src := `package main

func g(): Result(Int, Error) {
	return 1
}

func f() {
	g()
}

func main() {}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, "return g()") {
		t.Fatalf("expected implicit return g(), got:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestPipeline_implicitReturn_fmtPrintln_withGoWorkspace(t *testing.T) {
	root := moduleRootFromWD(t)
	src := `package main

import "fmt"

func f() {
	fmt.Println("x")
}

func main() {}
`
	out := compileForstPipelineExt(t, src, pipelineOpts{goWorkspaceDir: root})
	if !strings.Contains(out, "return fmt.Println") {
		t.Fatalf("expected implicit return fmt.Println, got:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestPipeline_providers_noplLoggerPrintln_happyPath(t *testing.T) {
	src := `package main

type Logger = { info(msg String) }
type NopLogger = {}

func (NopLogger) info(msg String) {
	println(msg)
}

func main() {
	with {
		Logger: NopLogger{}
	} {
		println("ok")
	}
}
`
	out := compileForstPipeline(t, src)
	if strings.Contains(out, "info(msg string) (int, error)") {
		t.Fatalf("NopLogger.info should be void, got:\n%s", out)
	}
	assertGoParses(t, out)
}
