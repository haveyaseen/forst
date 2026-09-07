package typechecker

import (
	"os"
	"path/filepath"
	"testing"

	"forst/internal/testutil"
)

func TestAsync_blockingCallFromSyncMainReturnsResult(t *testing.T) {
	root := t.TempDir()
	writeNodeFixture(t, root)

	src := `package main
import "./legacy/payment" js

func main() {
	x := payment.create(1.0, "usd")
	ensure x is Ok()
	println(x.id)
}
`
	tc, _ := MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
	})
	vt, ok := tc.VariableTypes["x"]
	if !ok || len(vt) != 1 {
		t.Fatalf("variable x types: %+v ok=%v", vt, ok)
	}
	if !vt[0].IsResultType() {
		t.Fatalf("expected Result type, got %+v", vt[0])
	}
}

func TestAsync_forInOverAsyncIteratorWithBlockingDispatch(t *testing.T) {
	root := t.TempDir()
	writeNodeEventsFixture(t, root)

	src := `package main
import "./legacy/events" js

func drain(userId String): Result(Void, Error) {
	seq := events.subscribe(userId)
	ensure seq is Ok()
	for _, evt := range seq {
		events.dispatch(evt)
	}
}
`
	tc, _ := MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
	})
	if _, ok := tc.Functions["drain"]; !ok {
		t.Fatal("drain should be registered")
	}
}

func writeNodeEventsFixture(t *testing.T, root string) {
	t.Helper()
	legacyDir := filepath.Join(root, "legacy")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tsFile := filepath.Join(legacyDir, "events.ts")
	if err := os.WriteFile(tsFile, []byte(
		"export async function* subscribe(userId: string): AsyncGenerator<{ type: string }> { yield { type: userId }; }\n"+
			"export async function dispatch(evt: { type: string }): Promise<void> {}\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
}
