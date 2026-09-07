package typechecker

import (
	"os"
	"path/filepath"
	"testing"

	"forst/internal/ast"
	"forst/internal/testutil"
)

func writeNodeFixture(t *testing.T, root string) {
	t.Helper()
	legacyDir := filepath.Join(root, "legacy")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tsFile := filepath.Join(legacyDir, "payment.ts")
	if err := os.WriteFile(tsFile, []byte(`export async function create(amount: number, currency: string): Promise<{ id: string }> {
  return { id: "x" };
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	// TypeScript source mode avoids esbuild precompile in unit tests (compiled is the production default).
	if err := os.WriteFile(filepath.Join(root, "ftconfig.json"), []byte(`{"bridge":{"legacyModules":{"format":"typescript"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNodeImports_registersLocalAndTypechecksCall(t *testing.T) {
	root := t.TempDir()
	writeNodeFixture(t, root)

	src := `package main
import "./legacy/payment" js

func main() {
	payment.create(10.0, "usd")
}
`
	tc, _ := MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
	})
	if !tc.NeedsBridgeRuntime() {
		t.Fatal("expected NeedsBridgeRuntime")
	}
	state := tc.BridgeRuntimeState()
	if !state.NeedsBridgeRuntime {
		t.Fatal("BridgeRuntimeState.NeedsBridgeRuntime = false")
	}
	if len(state.Manifest.Exports) != 1 {
		t.Fatalf("manifest exports: %+v", state.Manifest.Exports)
	}
	if _, ok := tc.nodeModuleForLocal("payment"); !ok {
		t.Fatal("payment node module not registered")
	}
	if tc.IsImportedLocalName("payment") {
		t.Fatal("JS import local should not be registered as Go import")
	}
}

func TestNodeImports_rejectsTSWithoutOptIn(t *testing.T) {
	root := t.TempDir()
	writeNodeFixture(t, root)
	if err := os.WriteFile(filepath.Join(root, "ftconfig.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	src := `package main
import "./legacy/payment"

func main() {}
`
	MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
		ExpectError:      "cannot import TypeScript module without import \"./path\" js or import alias \"./path\" js (payment.ts found)",
	})
}

func TestNodeImports_implicitPolicyAllowsWithoutOptIn(t *testing.T) {
	root := t.TempDir()
	writeNodeFixture(t, root)
	if err := os.WriteFile(filepath.Join(root, "ftconfig.json"), []byte(`{"bridge":{"importPolicy":"implicit"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	src := `package main
import "./legacy/payment"

func main() {
	payment.create(10.0, "usd")
}
`
	tc, _ := MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
	})
	if !tc.NeedsBridgeRuntime() {
		t.Fatal("expected NeedsBridgeRuntime under implicit importPolicy")
	}
	if tc.NodeImportPolicy != "implicit" {
		t.Fatalf("NodeImportPolicy = %q", tc.NodeImportPolicy)
	}
}

func TestNodeImports_rejectsMissingModule(t *testing.T) {
	root := t.TempDir()
	src := `package main
import "./legacy/payment" js

func main() {}
`
	MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
		ExpectError:      "TypeScript module not found",
	})
}

func TestNodeImports_rejectsWrongArity(t *testing.T) {
	root := t.TempDir()
	writeNodeFixture(t, root)

	src := `package main
import "./legacy/payment" js

func main() {
	payment.create(10.0)
}
`
	MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
		ExpectError:      "expects 2 arguments",
	})
}

func TestNodeImports_rejectsWrongArgumentType(t *testing.T) {
	root := t.TempDir()
	writeNodeFixture(t, root)

	src := `package main
import "./legacy/payment" js

func main() {
	payment.create("ten", "usd")
}
`
	MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
		ExpectError:      "argument 1",
	})
}

func TestNodeImports_rejectsUnknownExport(t *testing.T) {
	root := t.TempDir()
	writeNodeFixture(t, root)

	src := `package main
import "./legacy/payment" js

func main() {
	payment.refund()
}
`
	MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
		ExpectError:      "not found",
	})
}

func TestNodeImports_qualifiedCallReturnTypeIsResultOfPromiseElement(t *testing.T) {
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
	if len(vt[0].TypeParams) != 2 || vt[0].TypeParams[1].Ident != ast.TypeError {
		t.Fatalf("expected Result(_, Error), got %+v", vt[0])
	}
}

func TestNodeImports_checkoutHelperWithEnsureOkReturnString(t *testing.T) {
	root := t.TempDir()
	writeNodeFixture(t, root)

	src := `package main
import "./legacy/payment" js

func checkout(): Result(String, Error) {
	result := payment.create(1.0, "usd")
	ensure result is Ok()
	return result.id
}
`
	MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
	})
}

func TestNodeImports_goImportStillSeparate(t *testing.T) {
	root := t.TempDir()
	writeNodeFixture(t, root)

	src := `package main
import "fmt"
import "./legacy/payment" js

func main() {
	fmt.Println("ok")
	payment.create(1.0, "usd")
}
`
	tc, _ := MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
	})
	if !tc.IsImportedLocalName("fmt") {
		t.Fatal("fmt should remain a Go import local")
	}
	if len(tc.imports) != 1 {
		t.Fatalf("expected 1 Go import, got %d", len(tc.imports))
	}
	if len(tc.nodeImports) != 1 {
		t.Fatalf("expected 1 JS import, got %d", len(tc.nodeImports))
	}
}

func TestNodeImports_goAliasNode_notBridgeOptIn(t *testing.T) {
	root := t.TempDir()

	src := `package main
import node "fmt"

func main() {
	node.Println("ok")
}
`
	tc, _ := MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
	})
	if tc.NeedsBridgeRuntime() {
		t.Fatal("expected NeedsBridgeRuntime false for Go alias import node \"fmt\"")
	}
	if !tc.IsImportedLocalName("node") {
		t.Fatal("expected local name node to be a Go import")
	}
	if len(tc.nodeImports) != 0 {
		t.Fatalf("expected 0 JS imports, got %d", len(tc.nodeImports))
	}
	if len(tc.imports) != 1 {
		t.Fatalf("expected 1 Go import, got %d", len(tc.imports))
	}
}

func writeNodeTypeFixture(t *testing.T, root string) {
	t.Helper()
	legacyDir := filepath.Join(root, "legacy")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tsFile := filepath.Join(legacyDir, "type.ts")
	if err := os.WriteFile(tsFile, []byte(`export function create(): { id: string } {
  return { id: "x" };
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNodeImports_reservedLocalKeyword_requiresAlias(t *testing.T) {
	root := t.TempDir()
	writeNodeTypeFixture(t, root)

	src := `package main
import "./legacy/type.ts" js

func main() {}
`
	MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
		ExpectError:      `JS import local name "type" is a Forst keyword`,
	})
}

func TestNodeImports_reservedLocalKeyword_withSuggestedAlias_ok(t *testing.T) {
	root := t.TempDir()
	writeNodeTypeFixture(t, root)

	src := `package main
import typePkg "./legacy/type.ts" js

func main() {
	typePkg.create()
}
`
	MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
	})
}

func TestNodeImports_reservedLocalBareSpecifier_requiresAlias(t *testing.T) {
	root := t.TempDir()

	src := `package main
import "map" js

func main() {}
`
	MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
		ExpectError:      `JS import local name "map" is a Forst keyword`,
	})
}

func TestNodeImports_invalidScopedLocal_requiresAlias(t *testing.T) {
	root := t.TempDir()

	src := `package main
import "@scope/my-package" js

func main() {}
`
	MustTypecheck(t, src, testutil.TypecheckOpts{
		NodeBoundaryRoot: root,
		ForstFileDir:     root,
		ExpectError:      `JS import local name "my-package" is not a valid identifier`,
	})
}
