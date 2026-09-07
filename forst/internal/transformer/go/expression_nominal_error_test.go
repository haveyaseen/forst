package transformergo

import (
	"strings"
	"testing"
)

func TestTransform_nominalErrorStructLiteral(t *testing.T) {
	src := `package main

error NotFound {
	id: String
}

error TooFast {}

func main() {
	e := NotFound{id: "x"}
	println(e.id)
	_ = TooFast{}
}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, "NotFound{") {
		t.Fatalf("expected NotFound composite, got:\n%s", out)
	}
	if !strings.Contains(out, "TooFast{}") {
		t.Fatalf("expected TooFast{}, got:\n%s", out)
	}
	if strings.Contains(out, "NotFound(") {
		t.Fatalf("did not expect call-shaped constructor:\n%s", out)
	}
}
