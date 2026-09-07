package transformergo

import (
	"strings"
	"testing"
)

func TestPipeline_resultEnsure_omitsUnusedEmptyHashType(t *testing.T) {
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
	out := compileForstPipeline(t, src)
	if strings.Contains(out, "TypeDefShapeExpr({})") {
		t.Fatalf("unused empty hash type must not be emitted:\n%s", out)
	}
	if strings.Contains(out, "type T_") {
		t.Fatalf("result/ensure lowering must not dump unused hash types:\n%s", out)
	}
	if !strings.Contains(out, "func okInt()") {
		t.Fatalf("expected okInt in output:\n%s", out)
	}
}

func TestPipeline_usedAnonymousShape_stillEmitsHashType(t *testing.T) {
	t.Parallel()
	src := `package main

func makePoint() {
	return { x: 1, y: 2 }
}

func main() {
	p := makePoint()
	println(p.x)
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, "type T_") {
		t.Fatalf("anonymous shape used as return must emit a hash struct:\n%s", out)
	}
	if !strings.Contains(out, "x") || !strings.Contains(out, "y") {
		t.Fatalf("expected shape fields in emitted type:\n%s", out)
	}
}

func TestPipeline_namedType_alwaysEmittedEvenIfUnused(t *testing.T) {
	t.Parallel()
	src := `package main

type Unused = {
	label: String
}

func main() {
	println(1)
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, "type Unused struct") {
		t.Fatalf("user-named typedef must always be emitted:\n%s", out)
	}
}
