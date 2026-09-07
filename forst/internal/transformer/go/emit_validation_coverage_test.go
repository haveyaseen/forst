package transformergo

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func assertGoParses(t *testing.T, out string) {
	t.Helper()
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "gen.go", out, parser.AllErrors); err != nil {
		t.Fatalf("generated Go does not parse: %v\n%s", err, out)
	}
}

func TestEmitValidation_mapLiteralAndIndex(t *testing.T) {
	src := `package main

func main() {
	m := map[String]Int{ "a": 1, "b": 2 }
	println(m["a"])
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `map[`) || !strings.Contains(out, `[`) {
		t.Fatalf("expected map literal emit:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_arrayLiteral(t *testing.T) {
	src := `package main

func main() {
	a := [1, 2, 3]
	println(a[0])
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `[]`) {
		t.Fatalf("expected slice literal:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_breakContinueInFor(t *testing.T) {
	src := `package main

func main() {
	for i := 0; i < 10; i = i + 1 {
		if i == 3 {
			continue
		}
		if i == 7 {
			break
		}
	}
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `break`) || !strings.Contains(out, `continue`) {
		t.Fatalf("expected break/continue:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_labeledBreak(t *testing.T) {
	src := `package main

func main() {
outer: for i := 0; i < 10; i = i + 1 {
		for j := 0; j < 10; j = j + 1 {
			if j == 5 {
				break outer
			}
		}
	}
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `outer:`) || !strings.Contains(out, `break outer`) {
		t.Fatalf("expected labeled break:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_pointerAndReference(t *testing.T) {
	src := `package main

func main() {
	x := 1
	p := &x
	y := *p
	println(y)
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `&x`) || !strings.Contains(out, `*p`) {
		t.Fatalf("expected pointer ops:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_ensurePresentAndNil(t *testing.T) {
	src := `package main

func main() {
	var p *Int = nil
	ensure p is Nil()
	x := 1
	px := &x
	ensure px is Present()
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `== nil`) {
		t.Fatalf("expected nil check emit:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_shapeLiteralWithBaseType(t *testing.T) {
	src := `package main

type Point = { x: Int, y: Int }

func main() {
	p: Point = { x: 1, y: 2 }
	println(p.x)
}
`
	out := compileForstPipeline(t, src)
	for _, sub := range []string{`type Point`, `x:`, `y:`} {
		if !strings.Contains(out, sub) {
			t.Fatalf("missing %q in:\n%s", sub, out)
		}
	}
	assertGoParses(t, out)
}

func TestEmitValidation_deferStatement(t *testing.T) {
	src := `package main

func cleanup() {
	println("done")
}

func main() {
	defer cleanup()
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `defer cleanup()`) {
		t.Fatalf("expected defer emit:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_withBlockEmitsProvidersStruct(t *testing.T) {
	src := `package main

type Logger = { info(msg String) }
type NopLogger = {}

func (NopLogger) info(msg String) {}

func main() {
	with {
		Logger: NopLogger{}
	} {
		println("ok")
	}
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `Logger`) || !strings.Contains(out, `NopLogger`) {
		t.Fatalf("expected with/providers emit:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_typedefUnionAndAlias(t *testing.T) {
	src := `package main

type ParseError = { message: String }
type IoError = { code: Int }
type AppError = ParseError | IoError

func main() {
	var e: AppError = ParseError{ message: "x" }
	println("ok")
}
`
	out := compileForstPipeline(t, src)
	for _, sub := range []string{`type ParseError`, `type IoError`, `message`} {
		if !strings.Contains(out, sub) {
			t.Fatalf("missing %q in:\n%s", sub, out)
		}
	}
	assertGoParses(t, out)
}

func TestEmitValidation_useSiteInFunctionBody(t *testing.T) {
	src := `package main

type Logger = { info(msg String) }
type NopLogger = {}

func (NopLogger) info(msg String) {}

func work() {
	use logger: Logger
	logger.info("hi")
}

func main() {
	with { Logger: NopLogger{} } {
		work()
	}
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `Providers_`) || !strings.Contains(out, `work(providers`) {
		t.Fatalf("expected use/with lowering:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_goImportAndDefer(t *testing.T) {
	src := `package main

import "os"

func main() {
	defer os.Exit(0)
}
`
	out := compileForstPipelineExt(t, src, pipelineOpts{goWorkspaceDir: moduleRootFromWD(t)})
	if !strings.Contains(out, `"os"`) || !strings.Contains(out, `defer os.Exit`) {
		t.Fatalf("expected os import emit:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_nominalErrorConstructor(t *testing.T) {
	src := `package main

error NotFound {
	id: String
}

func main() {
	e := NotFound{ id: "x" }
	println(e.id)
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `type NotFound`) || !strings.Contains(out, `NotFound{`) {
		t.Fatalf("expected nominal error emit:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_compoundAssignPlusEq(t *testing.T) {
	src := `package main

func main() {
	n := 1
	n += 2
	println(n)
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `+=`) {
		t.Fatalf("expected += emit:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_ensureTypeGuardCall(t *testing.T) {
	src := `package main

type Score = Int

is (n Score) Positive {
	ensure n is GreaterThan(0)
}

func main() {
	x := 5
	ensure x is Positive()
	println(x)
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `func G_`) || !strings.Contains(out, `os.Exit`) {
		t.Fatalf("expected type guard ensure emit:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_stringBuiltinBool(t *testing.T) {
	src := `package main

func main() {
	b := true
	println(string(b))
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `strconv.FormatBool`) {
		t.Fatalf("expected FormatBool for string(bool):\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_stringBuiltinInt(t *testing.T) {
	src := `package main

func main() {
	println(string(42))
	xs := [10, 20]
	for i, v := range xs {
		println(string(i) + ":" + string(v))
	}
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `strconv.Itoa`) {
		t.Fatalf("expected Itoa for string(int):\n%s", out)
	}
	if strings.Contains(out, `string(42)`) || strings.Contains(out, `string(i)`) || strings.Contains(out, `string(string(`) {
		t.Fatalf("expected no Go string(int) or double-wrap:\n%s", out)
	}
	if !strings.Contains(out, `strconv.Itoa(i) + ":" + strconv.Itoa(v)`) {
		t.Fatalf("expected Itoa concat for string(i) + \":\" + string(v):\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_stringBuiltinByteSlice(t *testing.T) {
	src := `package main

func main() {
	b := []byte("hi")
	println(string(b))
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `string(b)`) {
		t.Fatalf("expected Go string(b) for string([]byte):\n%s", out)
	}
	if strings.Contains(out, `strconv.Itoa`) {
		t.Fatalf("expected no Itoa for string([]byte):\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_providersCrossFunctionCall(t *testing.T) {
	src := `package main

type Logger = { info(msg String) }
type NopLogger = {}

func (NopLogger) info(msg String) {}

func callee() {
	use logger: Logger
	logger.info("callee")
}

func main() {
	with { Logger: NopLogger{} } {
		callee()
	}
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `callee(providers`) {
		t.Fatalf("expected providers param on callee:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_typedefAssertionAlias(t *testing.T) {
	src := `package main

type Slug = String.Min(1).Max(64)

func main() {
	s: Slug = "hello"
	println(s)
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `type Slug`) {
		t.Fatalf("expected Slug typedef emit:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_receiverMethod(t *testing.T) {
	src := `package main

type Counter = { n: Int }

func (Counter) bump(): Counter {
	return { n: 1 }
}

func main() {
	c := Counter{ n: 0 }
	c = c.bump()
	println(string(c.n))
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `func (Counter) bump`) {
		t.Fatalf("expected receiver method emit:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_resultAssign(t *testing.T) {
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
	if !strings.Contains(out, `x, xErr :=`) {
		t.Fatalf("expected Result assign emit:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_typeAliasWithConstraints(t *testing.T) {
	src := `package main

type Slug = String.Min(1).Max(64)
type Label = Slug

func main() {
	s: Label = "hello"
	println(s)
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `type Slug`) || !strings.Contains(out, `type Label`) {
		t.Fatalf("expected alias emit:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_deferAndGoStmt(t *testing.T) {
	src := `package main

func cleanup() {}

func main() {
	defer cleanup()
	go cleanup()
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `defer cleanup()`) || !strings.Contains(out, `go cleanup()`) {
		t.Fatalf("expected defer/go emit:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_postfixIncDec(t *testing.T) {
	src := `package main

func main() {
	i := 0
	i++
	i--
	println(string(i))
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `i++`) || !strings.Contains(out, `i--`) {
		t.Fatalf("expected inc/dec emit:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_compoundAssignForPost(t *testing.T) {
	src := `package main

func main() {
	for i := 0; i < 3; i += 1 {
		println(string(i))
	}
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `i += 1`) {
		t.Fatalf("expected compound assign in for post:\n%s", out)
	}
	assertGoParses(t, out)
}

func TestEmitValidation_rangeTwoVars(t *testing.T) {
	src := `package main

func main() {
	xs := [10, 20]
	for i, v := range xs {
		println(string(i) + string(v))
	}
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `range xs`) {
		t.Fatalf("expected range emit:\n%s", out)
	}
	assertGoParses(t, out)
}
