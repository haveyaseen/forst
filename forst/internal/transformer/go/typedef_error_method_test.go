package transformergo

import (
	"strings"
	"testing"
)

func TestPipeline_nominalError_ErrorReturnsTypeName(t *testing.T) {
	t.Parallel()
	src := `package main

error SlotAlreadyTaken {}

func main() {}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, `func (e SlotAlreadyTaken) Error() string`) {
		t.Fatalf("missing Error method\n----\n%s\n----", out)
	}
	if !strings.Contains(out, `return "SlotAlreadyTaken"`) {
		t.Fatalf("Error() should return type name\n----\n%s\n----", out)
	}
	if strings.Contains(out, `func (e SlotAlreadyTaken) Error() string {
	return "error"
}`) || strings.Contains(out, `Error() string { return "error" }`) {
		t.Fatalf("Error() must not return generic \"error\"\n----\n%s\n----", out)
	}
	if !strings.Contains(out, `return "main/SlotAlreadyTaken"`) {
		t.Fatalf("ForstErrorTag should be package-qualified\n----\n%s\n----", out)
	}
}
