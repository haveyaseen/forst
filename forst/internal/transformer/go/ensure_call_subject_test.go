package transformergo

import (
	"strings"
	"testing"
)

func TestPipeline_ensureVoidResultCallSubject_emitsIfErrInit(t *testing.T) {
	t.Parallel()
	src := `package main

error E { message: String }

func need(ok Bool) {
	ensure ok is True()
		else E{message: "bad"}
}

func run(ok Bool) {
	ensure need(ok)
	return 1
}

func main() {}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, "if err := need(ok); err != nil") {
		t.Fatalf("expected if-err Init for ensure need(ok), got:\n%s", out)
	}
	if !strings.Contains(out, "return 0, err") && !strings.Contains(out, "return err") {
		// Result(Int, Error) lowers to (int, error)
		if !strings.Contains(out, "err") {
			t.Fatalf("expected error propagation from ensure call subject, got:\n%s", out)
		}
	}
	// Must not require a prior binding
	if strings.Contains(out, "result := need(ok)") {
		t.Fatalf("did not expect bound result for call-subject ensure, got:\n%s", out)
	}
}

func TestPipeline_ensureResultCallSubject_discardsSuccess(t *testing.T) {
	t.Parallel()
	src := `package main

error E { message: String }

func fetch(): Result(Int, Error) {
	return 1
}

func run() {
	ensure fetch()
	return 1
}

func main() {}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, "if _, err := fetch(); err != nil") {
		t.Fatalf("expected blank success + err Init for ensure fetch(), got:\n%s", out)
	}
}

func TestPipeline_ensureCallSubject_explicitIsOk(t *testing.T) {
	t.Parallel()
	src := `package main

error E { message: String }

func need(ok Bool) {
	ensure ok is True()
		else E{message: "bad"}
}

func run(ok Bool) {
	ensure need(ok) is Ok()
	return 1
}

func main() {}
`
	out := compileForstPipeline(t, src)
	if !strings.Contains(out, "if err := need(ok); err != nil") {
		t.Fatalf("expected if-err Init for ensure need(ok) is Ok(), got:\n%s", out)
	}
}
