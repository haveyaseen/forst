package transformergo

import (
	"bytes"
	goast "go/ast"
	"go/format"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forst/internal/generators"
	"forst/internal/testutil"
	"forst/internal/typechecker"
)

func formatTransformerGoFiles(t *testing.T, tr *Transformer, mainFile *goast.File) (mainCode, runtimeCode string) {
	t.Helper()
	var mainBuf bytes.Buffer
	if err := format.Node(&mainBuf, token.NewFileSet(), mainFile); err != nil {
		t.Fatalf("format main: %v", err)
	}
	mainCode = mainBuf.String()
	runtimeAST, err := tr.BridgeRuntimeFile()
	if err != nil {
		t.Fatalf("BridgeRuntimeFile: %v", err)
	}
	if runtimeAST == nil {
		return mainCode, ""
	}
	runtimeCode, err = generators.GenerateGoCode(runtimeAST)
	if err != nil {
		t.Fatalf("generate runtime: %v", err)
	}
	return mainCode, runtimeCode
}

func TestTransformNodeQualifiedCall_emitsBridgeCall(t *testing.T) {
	root := t.TempDir()
	writeNodeBridgeFixture(t, root)

	src := `package main
import "./legacy/payment" js

func main() {
	result := payment.create(10.0, "usd")
	ensure result is Ok()
	println(result.id)
}
`
	tc, nodes := typechecker.MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
	})
	tr := New(tc, nil)
	file, err := tr.TransformForstFileToGo(nodes)
	if err != nil {
		t.Fatalf("TransformForstFileToGo: %v", err)
	}
	mainCode, runtimeCode := formatTransformerGoFiles(t, tr, file)
	if strings.Contains(mainCode, "bridgert.") {
		t.Fatalf("main must not reference bridgert:\n%s", mainCode)
	}
	if !strings.Contains(mainCode, "forst_bridge_callsync_") {
		t.Fatalf("missing wrapper call in main:\n%s", mainCode)
	}
	if !strings.Contains(runtimeCode, "bridgert.CallSync") {
		t.Fatalf("missing bridgert.CallSync in runtime:\n%s", runtimeCode)
	}
	if !strings.Contains(runtimeCode, "forstBridgeManifestJSON") {
		t.Fatalf("missing manifest in runtime:\n%s", runtimeCode)
	}
	if !strings.Contains(mainCode, "resultErr") {
		t.Fatalf("missing resultErr split assignment in:\n%s", mainCode)
	}
}

func TestCodegen_asyncExportUsesCallAsync(t *testing.T) {
	root := t.TempDir()
	writeNodeBridgeAsyncFixture(t, root)

	src := `package main
import "./legacy/payment" js

func main() {
	result := payment.create(10.0, "usd")
	ensure result is Ok()
	println(result.id)
}
`
	tc, nodes := typechecker.MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
	})
	tr := New(tc, nil)
	file, err := tr.TransformForstFileToGo(nodes)
	if err != nil {
		t.Fatalf("TransformForstFileToGo: %v", err)
	}
	mainCode, runtimeCode := formatTransformerGoFiles(t, tr, file)
	if !strings.Contains(runtimeCode, "bridgert.CallAsync") {
		t.Fatalf("missing bridgert.CallAsync in runtime:\n%s", runtimeCode)
	}
	if strings.Contains(runtimeCode, "bridgert.MustCallAsync[") {
		t.Fatalf("must not use MustCallAsync in:\n%s", runtimeCode)
	}
	if strings.Contains(runtimeCode, "bridgert.CallSync[") && !strings.Contains(runtimeCode, "bridgert.CallSyncArgs") {
		t.Fatalf("async export must not use CallSync in:\n%s", runtimeCode)
	}
	_ = mainCode
}

func TestCodegen_nodeWrapperUsesIndexParamTypesForFloatArg(t *testing.T) {
	root := t.TempDir()
	legacyDir := filepath.Join(root, "legacy")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tsFile := filepath.Join(legacyDir, "payment.ts")
	if err := os.WriteFile(tsFile, []byte("export async function concurrentEcho(n: number): Promise<{ echo: number }> { return { echo: n }; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	src := `package main
import "./legacy/payment" js
import "strconv"

func echo(n Float): Result(String, Error) {
	res := payment.concurrentEcho(n)
	ensure res is Ok()
	return strconv.FormatFloat(res.echo, 'f', 0, 64)
}
`
	tc, nodes := typechecker.MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
	})
	tr := New(tc, nil)
	file, err := tr.TransformForstFileToGo(nodes)
	if err != nil {
		t.Fatalf("TransformForstFileToGo: %v", err)
	}
	_, runtimeCode := formatTransformerGoFiles(t, tr, file)
	if !strings.Contains(runtimeCode, "concurrentEcho(arg0 float64)") {
		t.Fatalf("wrapper should use float64 from TS index, got:\n%s", runtimeCode)
	}
}

func TestCodegen_blockingForInOverAsyncIterator(t *testing.T) {
	root := t.TempDir()
	writeNodeBridgeAsyncGenFixture(t, root)

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
	tc, nodes := typechecker.MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
	})
	tr := New(tc, nil)
	file, err := tr.TransformForstFileToGo(nodes)
	if err != nil {
		t.Fatalf("TransformForstFileToGo: %v", err)
	}
	mainCode, runtimeCode := formatTransformerGoFiles(t, tr, file)
	for _, want := range []string{
		"forst_bridge_open_seq_",
		"seqErr",
		".NextBatch(",
		".Close()",
		"forst_bridge_callasync_",
	} {
		if !strings.Contains(mainCode, want) {
			t.Fatalf("missing %q in main:\n%s", want, mainCode)
		}
	}
	if !strings.Contains(runtimeCode, "bridgert.OpenSeq") {
		t.Fatalf("missing bridgert.OpenSeq in runtime:\n%s", runtimeCode)
	}
	if strings.Contains(mainCode, "async.MustAwait[") {
		t.Fatalf("blocking sync model must not emit async.MustAwait in:\n%s", mainCode)
	}
}

func TestCodegen_generatorExportUsesOpenGenAndRangeLowering(t *testing.T) {
	root := t.TempDir()
	writeNodeBridgeGeneratorFixture(t, root)

	src := `package main
import "./legacy/generators" js

func main() {
	seq := generators.syncNumbers(2.0)
	ensure seq is Ok()
	for range seq {
	}
}
`
	tc, nodes := typechecker.MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
	})
	tr := New(tc, nil)
	file, err := tr.TransformForstFileToGo(nodes)
	if err != nil {
		t.Fatalf("TransformForstFileToGo: %v", err)
	}
	mainCode, runtimeCode := formatTransformerGoFiles(t, tr, file)
	for _, want := range []string{
		"forst_bridge_open_seq_",
		".NextBatch(",
		".Close()",
		"forstBridgeGenStepDone",
	} {
		if !strings.Contains(mainCode, want) {
			t.Fatalf("missing %q in main:\n%s", want, mainCode)
		}
	}
	if !strings.Contains(runtimeCode, "bridgert.OpenSeq") {
		t.Fatalf("missing bridgert.OpenSeq in runtime:\n%s", runtimeCode)
	}
}

func TestCodegen_nodeIteratorFor_bindsIndexAndValue(t *testing.T) {
	root := t.TempDir()
	writeNodeBridgeGeneratorFixture(t, root)

	src := `package main
import "./legacy/generators" js

func main() {
	seq := generators.syncNumbers(2.0)
	ensure seq is Ok()
	for i, n := range seq {
		println(i)
		println(n)
	}
}
`
	tc, nodes := typechecker.MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
	})
	tr := New(tc, nil)
	file, err := tr.TransformForstFileToGo(nodes)
	if err != nil {
		t.Fatalf("TransformForstFileToGo: %v", err)
	}
	mainCode, _ := formatTransformerGoFiles(t, tr, file)
	for _, want := range []string{
		"_nodeIdx",
		"_nodeStep",
		"_nodeStep.Value",
	} {
		if !strings.Contains(mainCode, want) {
			t.Fatalf("missing %q in main:\n%s", want, mainCode)
		}
	}
}

func TestCodegen_nodeIteratorFor_emitsErrorKindPanic(t *testing.T) {
	root := t.TempDir()
	writeNodeBridgeGeneratorFixture(t, root)

	src := `package main
import "./legacy/generators" js

func main() {
	seq := generators.syncNumbers(2.0)
	ensure seq is Ok()
	for range seq {
	}
}
`
	tc, nodes := typechecker.MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
	})
	tr := New(tc, nil)
	file, err := tr.TransformForstFileToGo(nodes)
	if err != nil {
		t.Fatalf("TransformForstFileToGo: %v", err)
	}
	mainCode, _ := formatTransformerGoFiles(t, tr, file)
	if !strings.Contains(mainCode, forstBridgeGenStepErrorIdent) {
		t.Fatalf("missing error-kind branch in main:\n%s", mainCode)
	}
	if !strings.Contains(mainCode, "panic") {
		t.Fatalf("missing panic on generator error step in main:\n%s", mainCode)
	}
}

func writeNodeBridgeAsyncGenFixture(t *testing.T, root string) {
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

func writeNodeBridgeGeneratorFixture(t *testing.T, root string) {
	t.Helper()
	legacyDir := filepath.Join(root, "legacy")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tsFile := filepath.Join(legacyDir, "generators.ts")
	if err := os.WriteFile(tsFile, []byte("export function* syncNumbers(limit: number): Generator<number> { for (let i = 0; i < limit; i++) yield i; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeNodeBridgeAsyncFixture(t *testing.T, root string) {
	t.Helper()
	legacyDir := filepath.Join(root, "legacy")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tsFile := filepath.Join(legacyDir, "payment.ts")
	if err := os.WriteFile(tsFile, []byte("export async function create(amount: number, currency: string): Promise<{ id: string }> { return { id: \"x\" } }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeNodeBridgeFixture(t *testing.T, root string) {
	t.Helper()
	legacyDir := filepath.Join(root, "legacy")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tsFile := filepath.Join(legacyDir, "payment.ts")
	if err := os.WriteFile(tsFile, []byte("export function create(amount: number, currency: string): { id: string } { return { id: \"x\" } }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
