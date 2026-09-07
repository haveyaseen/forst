package printer

import (
	"strings"
	"testing"

	"forst/internal/ast"
	"forst/internal/lexer"
	"forst/internal/parser"

	"github.com/sirupsen/logrus"
)

func TestPrintParam_simple(t *testing.T) {
	t.Parallel()
	var p printer
	p.cfg = DefaultConfig()
	simple, err := p.printParam(ast.SimpleParamNode{
		Ident: ast.Ident{ID: ast.Identifier("x")},
		Type:  ast.NewBuiltinType(ast.TypeInt),
	})
	if err != nil || simple != "x Int" {
		t.Fatalf("simple: %q err=%v", simple, err)
	}
}

func TestPrintParam_destructured(t *testing.T) {
	t.Parallel()
	var p printer
	p.cfg = DefaultConfig()
	destr, err := p.printParam(ast.DestructuredParamNode{
		Fields: []string{"a", "b"},
		Type:   ast.NewBuiltinType(ast.TypeString),
	})
	if err != nil || destr != "{a, b} String" {
		t.Fatalf("destructured: %q err=%v", destr, err)
	}
}

func TestPrintParam_variadic(t *testing.T) {
	t.Parallel()
	var p printer
	p.cfg = DefaultConfig()
	got, err := p.printParam(ast.SimpleParamNode{
		Ident:    ast.Ident{ID: ast.Identifier("xs")},
		Type:     ast.NewTypeParamType("T"),
		Variadic: true,
	})
	if err != nil || got != "xs ...T" {
		t.Fatalf("variadic: %q err=%v", got, err)
	}
}

func TestFormatSource_genericVariadicParam_idempotent(t *testing.T) {
	t.Parallel()
	const src = `package main

func ignore[T any](xs ...T): Bool {
	return true
}

func main() {
	println(ignore(1, 2, 3))
}
`
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	out, err := FormatSource(src, "generic_variadic.ft", log)
	if err != nil {
		t.Fatalf("FormatSource: %v", err)
	}
	if !strings.Contains(out, "xs ...T") {
		t.Fatalf("expected variadic param preserved, got:\n%s", out)
	}
	if strings.Contains(out, "xs T") && !strings.Contains(out, "xs ...T") {
		t.Fatalf("fmt stripped variadic ellipsis:\n%s", out)
	}
	l := lexer.New([]byte(out), "generic_variadic.ft", log)
	tokens := l.Lex()
	p := parser.New(tokens, "generic_variadic.ft", log)
	if _, err := p.ParseFile(); err != nil {
		t.Fatalf("re-parse pretty output: %v\n--- out ---\n%s", err, out)
	}
}

func TestFormatSource_functionParams_goStyleNoColonBeforeType(t *testing.T) {
	t.Parallel()
	const src = `package main

func add(a Int, b Int): Int {
	return a + b
}
`
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	out, err := FormatSource(src, "params.ft", log)
	if err != nil {
		t.Fatalf("FormatSource: %v", err)
	}
	if strings.Contains(out, "a: Int") || strings.Contains(out, "b: Int") {
		t.Fatalf("expected Go-style params (name Type), got:\n%s", out)
	}
	if !strings.Contains(out, "a Int") || !strings.Contains(out, "b Int") {
		t.Fatalf("expected Go-style params, got:\n%s", out)
	}
	l := lexer.New([]byte(out), "params.ft", log)
	tokens := l.Lex()
	p := parser.New(tokens, "params.ft", log)
	if _, err := p.ParseFile(); err != nil {
		t.Fatalf("re-parse pretty output: %v\n--- out ---\n%s", err, out)
	}
}

func TestFormatSource_basicFt_roundTripsParse(t *testing.T) {
	t.Parallel()
	const src = `package main

func greet(): String {
	return "Hello, World!"
}

func main() {
	println(greet())
}
`
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	out, err := FormatSource(src, "basic.ft", log)
	if err != nil {
		t.Fatalf("FormatSource: %v", err)
	}
	// Second parse must succeed
	l := lexer.New([]byte(out), "basic.ft", log)
	tokens := l.Lex()
	p := parser.New(tokens, "basic.ft", log)
	if _, err := p.ParseFile(); err != nil {
		t.Fatalf("re-parse pretty output: %v\n--- out ---\n%s", err, out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Fatal("output should end with newline")
	}
}

func TestFormatSource_trimsMeaninglessWhitespace(t *testing.T) {
	t.Parallel()
	const messy = `package   main   

func   main (  )   {   return   1
}
`
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	out, err := FormatSource(messy, "x.ft", log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "package   main") {
		t.Fatalf("expected normalized package line, got %q", out)
	}
}

func TestFormatSource_preservesCommentsAndBlankLinesBetweenDecls(t *testing.T) {
	t.Parallel()
	const src = `package main
// Doc for a
func a(): Int {
	// inside
	return 1
}

func b(): Int {
	return 2
}
`
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	out, err := FormatSource(src, "x.ft", log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "// Doc for a") {
		t.Fatalf("missing doc comment: %q", out)
	}
	if !strings.Contains(out, "// inside") {
		t.Fatalf("missing inner comment: %q", out)
	}
	if !strings.Contains(out, "\n\nfunc a()") && !strings.Contains(out, "\nfunc a()") {
		t.Fatalf("unexpected layout around func a: %q", out)
	}
	// Two top-level funcs separated by a blank line (gofmt-style)
	if !strings.Contains(out, "}\n\nfunc b()") {
		t.Fatalf("expected blank line between top-level funcs, got:\n%s", out)
	}
}

func TestFormatSource_commentOnlyFile(t *testing.T) {
	t.Parallel()
	const src = `// SPDX
// license
`
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	out, err := FormatSource(src, "x.ft", log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "// SPDX") {
		t.Fatalf("got %q", out)
	}
}

func TestPrint_unsupportedTopLevelErrors(t *testing.T) {
	t.Parallel()
	// nil slice only — empty file
	out, err := Print(DefaultConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Fatalf("got %q", out)
	}
}

func TestFormatSource_multilineShapesAndIndentedBlockLines(t *testing.T) {
	t.Parallel()
	const src = `package main

func main() {
  println({
    ctx: {
      n: 1,
    },
    input: {
      name: "x",
    },
  })
}
`
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	out, err := FormatSource(src, "shape.ft", log)
	if err != nil {
		t.Fatalf("FormatSource: %v", err)
	}
	if !strings.Contains(out, "println({\n") {
		t.Fatalf("expected multiline call + shape opening, got:\n%s", out)
	}
	if !strings.Contains(out, "\tctx:") {
		t.Fatalf("expected ctx field indented inside outer shape, got:\n%s", out)
	}
	if !strings.Contains(out, "ctx: {n: 1}") {
		t.Fatalf("expected compact nested shape when short, got:\n%s", out)
	}
}

func TestFormatSource_shapeLiteral_nilPrintsAsNilNotValueNil(t *testing.T) {
	t.Parallel()
	const src = `package main

func main() {
  println({
    ctx: {
      sessionId: nil,
    },
    input: {
      name: "Go to the gym",
    },
  })
}
`
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	out, err := FormatSource(src, "nilshape.ft", log)
	if err != nil {
		t.Fatalf("FormatSource: %v", err)
	}
	if strings.Contains(out, "Value(nil)") {
		t.Fatalf("shape literal must print nil, not Value(nil):\n%s", out)
	}
	if !strings.Contains(out, "sessionId: nil") {
		t.Fatalf("expected sessionId: nil, got:\n%s", out)
	}
}

func TestFormatSource_importRoundTrip(t *testing.T) {
	t.Parallel()
	const src = `package main

import "fmt"

func main() {
	fmt.Println("done")
}
`
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	out, err := FormatSource(src, "bundle.ft", log)
	if err != nil {
		t.Fatalf("FormatSource: %v", err)
	}
	for _, needle := range []string{`import "fmt"`, `fmt.Println`} {
		if !strings.Contains(out, needle) {
			t.Fatalf("missing %q in:\n%s", needle, out)
		}
	}
	l := lexer.New([]byte(out), "bundle.ft", log)
	tokens := l.Lex()
	p := parser.New(tokens, "bundle.ft", log)
	if _, err := p.ParseFile(); err != nil {
		t.Fatalf("re-parse pretty output: %v\n--- out ---\n%s", err, out)
	}
}

func TestFormatSource_nominalError_roundTrips(t *testing.T) {
	t.Parallel()
	const src = `package main

error NotPositive {
	message: String
}

func main() {
}
`
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	out, err := FormatSource(src, "err.ft", log)
	if err != nil {
		t.Fatalf("FormatSource: %v", err)
	}
	if strings.Contains(out, "type NotPositive") {
		t.Fatalf("must not emit type alias for nominal error, got:\n%s", out)
	}
	if !strings.Contains(out, "error NotPositive") {
		t.Fatalf("expected error NotPositive { ... }, got:\n%s", out)
	}
	l := lexer.New([]byte(out), "err.ft", log)
	tokens := l.Lex()
	p := parser.New(tokens, "err.ft", log)
	if _, err := p.ParseFile(); err != nil {
		t.Fatalf("re-parse pretty output: %v\n--- out ---\n%s", err, out)
	}
}

func TestFormatSource_multiAssertionEnsure_wrapsEachLine(t *testing.T) {
	t.Parallel()
	const src = `package main

func f(password String, status String) {
	ensure password is Strong() or Passkey() else InvalidCredential{}
	ensure status is Pending() or Processing() or Retrying() else InvalidCredential{}
	ensure status is Pending() or Processing()
}
`
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	out, err := FormatSource(src, "ensure-multi.ft", log)
	if err != nil {
		t.Fatalf("FormatSource: %v", err)
	}

	expected := `package main

func f(password String, status String) {
	ensure password
	    is Strong()
	    or Passkey()
	    else InvalidCredential{}
	ensure status
	    is Pending()
	    or Processing()
	    or Retrying()
	    else InvalidCredential{}
	ensure status
	    is Pending()
	    or Processing()
}
`
	if out != expected {
		t.Fatalf("unexpected formatted multi-assertion ensure output:\n--- got ---\n%s\n--- expected ---\n%s", out, expected)
	}

	l := lexer.New([]byte(out), "ensure-multi.ft", log)
	tokens := l.Lex()
	p := parser.New(tokens, "ensure-multi.ft", log)
	if _, err := p.ParseFile(); err != nil {
		t.Fatalf("re-parse pretty output: %v\n--- out ---\n%s", err, out)
	}
}

func TestFormatSource_ensureOr_putsOrOnNextLineIndentedFour(t *testing.T) {
	t.Parallel()
	const src = `package main

func f(name String) {
	ensure name is Min(1) else TooShort{message: "msg"}
}
`
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	out, err := FormatSource(src, "ensure-or.ft", log)
	if err != nil {
		t.Fatalf("FormatSource: %v", err)
	}
	if !strings.Contains(out, "\n\t    else TooShort") {
		t.Fatalf("expected `else` on the line after `ensure` with +4 column indent, got:\n%s", out)
	}
	if strings.Contains(out, "Min(1) else TooShort") && !strings.Contains(out, "\n\t    else TooShort") {
		t.Fatalf("did not expect `ensure` and `else` jammed on one line without indent break, got:\n%s", out)
	}
	l := lexer.New([]byte(out), "ensure-or.ft", log)
	tokens := l.Lex()
	p := parser.New(tokens, "ensure-or.ft", log)
	if _, err := p.ParseFile(); err != nil {
		t.Fatalf("re-parse pretty output: %v\n--- out ---\n%s", err, out)
	}
}

func TestFormatSource_ensureNegatedBlock_noDoubleIndent(t *testing.T) {
	t.Parallel()
	const src = `package main

import "fmt"

func main() {
	ensure !err else {
		fmt.Printf("x")
	}
}
`
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	out, err := FormatSource(src, "ensure-block.ft", log)
	if err != nil {
		t.Fatalf("FormatSource: %v", err)
	}
	if strings.Contains(out, "\t\t\tfmt.Printf") {
		t.Fatalf("ensure block body must not be triple-indented:\n%s", out)
	}
	if strings.Contains(out, "\t\t}\n}") {
		t.Fatalf("ensure closing brace must not be double-indented:\n%s", out)
	}
	l := lexer.New([]byte(out), "ensure-block.ft", log)
	tokens := l.Lex()
	p := parser.New(tokens, "ensure-block.ft", log)
	if _, err := p.ParseFile(); err != nil {
		t.Fatalf("re-parse pretty output: %v\n--- out ---\n%s", err, out)
	}
}

func TestPrintExpr_mapLiteral(t *testing.T) {
	t.Parallel()
	var p printer
	p.cfg = DefaultConfig()
	got, err := p.printExpr(ast.MapLiteralNode{
		Type: ast.TypeNode{
			Ident:      ast.TypeMap,
			TypeParams: []ast.TypeNode{{Ident: ast.TypeString}, {Ident: ast.TypeInt}},
		},
		Entries: []ast.MapEntryNode{
			{Key: ast.StringLiteralNode{Value: "a"}, Value: ast.IntLiteralNode{Value: 1}},
			{Key: ast.StringLiteralNode{Value: "b"}, Value: ast.IntLiteralNode{Value: 2}},
		},
	})
	if err != nil {
		t.Fatalf("printExpr map literal: %v", err)
	}
	want := `map[String]Int{"a": 1, "b": 2}`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
