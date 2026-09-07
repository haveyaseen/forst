package forstdep

import (
	"os"
	"path/filepath"
	"testing"

	"forst/internal/goload"
	"forst/internal/testmod"
)

func TestForstFilesInDir_skipsTestsIncludesSkipFt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"ok.ft", "x_test.ft", "draft.skip.ft", "helpers.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("package lib\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ForstFilesInDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	bases := make(map[string]bool, len(got))
	for _, p := range got {
		bases[filepath.Base(p)] = true
	}
	if !bases["ok.ft"] || !bases["draft.skip.ft"] {
		t.Fatalf("got %v, want ok.ft and draft.skip.ft", got)
	}
	if bases["x_test.ft"] {
		t.Fatalf("got %v, must still skip *_test.ft", got)
	}
}

func TestLocatePackageDirs_ftOnlyHasDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	consumer := filepath.Join(root, "consumer")
	lib := filepath.Join(root, "acme-lib")
	if err := os.MkdirAll(consumer, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	testmod.WriteGoMod(t, consumer, "testmod")
	goMod := "module github.com/acme/lib\n\ngo " + testmod.GoVersion + "\n"
	if err := os.WriteFile(filepath.Join(lib, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "add.ft"), []byte("package lib\n\nfunc Add(a Int, b Int) {\n\treturn a + b\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	consumerMod := testmod.GoModContent("testmod") + "\nrequire github.com/acme/lib v0.0.0\n\nreplace github.com/acme/lib => ../acme-lib\n"
	if err := os.WriteFile(filepath.Join(consumer, "go.mod"), []byte(consumerMod), 0o644); err != nil {
		t.Fatal(err)
	}
	goload.ClearLoadCacheForTest()
	locs, err := goload.LocatePackageDirs(consumer, []string{"github.com/acme/lib"})
	if err != nil {
		t.Fatal(err)
	}
	loc, ok := locs["github.com/acme/lib"]
	if !ok {
		t.Fatalf("missing loc, got %#v", locs)
	}
	if loc.Dir == "" {
		t.Fatal("expected Dir set for Forst-only package")
	}
	if filepath.Clean(loc.Dir) != filepath.Clean(lib) {
		got, _ := filepath.EvalSymlinks(loc.Dir)
		want, _ := filepath.EvalSymlinks(lib)
		if filepath.Clean(got) != filepath.Clean(want) {
			t.Fatalf("Dir = %q, want %q", loc.Dir, lib)
		}
	}
}

func TestDiscoverForstDep_ftOnlyPackage(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	consumer := filepath.Join(root, "consumer")
	lib := filepath.Join(root, "acme-lib")
	mustMkdir(t, consumer)
	mustMkdir(t, lib)
	write(t, filepath.Join(lib, "go.mod"), "module github.com/acme/lib\n\ngo "+testmod.GoVersion+"\n")
	write(t, filepath.Join(lib, "add.ft"), "package lib\n\nfunc Add(a Int, b Int) {\n\treturn a + b\n}\n")
	write(t, filepath.Join(consumer, "go.mod"), testmod.GoModContent("testmod")+"\nrequire github.com/acme/lib v0.0.0\n\nreplace github.com/acme/lib => ../acme-lib\n")
	write(t, filepath.Join(consumer, "main.ft"), "package main\n\nimport \"github.com/acme/lib\"\n\nfunc main() {\n\t_ = lib.Add(1, 2)\n}\n")
	goload.ClearLoadCacheForTest()
	got, err := Discover(nil, consumer, nil, []string{"github.com/acme/lib"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d packages, want 1", len(got))
	}
	if got[0].Loc.ImportPath != "github.com/acme/lib" {
		t.Fatalf("import path = %q", got[0].Loc.ImportPath)
	}
	if got[0].ForstPkg != "lib" {
		t.Fatalf("ForstPkg = %q", got[0].ForstPkg)
	}
}

func TestDiscoverForstDep_goOnlyUnchanged(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	consumer := filepath.Join(root, "consumer")
	lib := filepath.Join(root, "acme-lib")
	mustMkdir(t, consumer)
	mustMkdir(t, lib)
	write(t, filepath.Join(lib, "go.mod"), "module github.com/acme/lib\n\ngo "+testmod.GoVersion+"\n")
	write(t, filepath.Join(lib, "add.go"), "package lib\n\nfunc Add(a, b int) int { return a + b }\n")
	write(t, filepath.Join(consumer, "go.mod"), testmod.GoModContent("testmod")+"\nrequire github.com/acme/lib v0.0.0\n\nreplace github.com/acme/lib => ../acme-lib\n")
	goload.ClearLoadCacheForTest()
	got, err := Discover(nil, consumer, nil, []string{"github.com/acme/lib"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("go-only package should not be discovered as Forst, got %#v", got)
	}
}

func TestDiscoverForstDep_unimportedDepPackageSkipped(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	consumer := filepath.Join(root, "consumer")
	lib := filepath.Join(root, "acme-lib")
	unused := filepath.Join(lib, "unused")
	mustMkdir(t, consumer)
	mustMkdir(t, lib)
	mustMkdir(t, unused)
	write(t, filepath.Join(lib, "go.mod"), "module github.com/acme/lib\n\ngo "+testmod.GoVersion+"\n")
	write(t, filepath.Join(lib, "add.ft"), "package lib\n\nfunc Add(a Int, b Int) {\n\treturn a + b\n}\n")
	write(t, filepath.Join(unused, "dead.ft"), "package unused\n\nthis is not valid {{{{\n")
	write(t, filepath.Join(consumer, "go.mod"), testmod.GoModContent("testmod")+"\nrequire github.com/acme/lib v0.0.0\n\nreplace github.com/acme/lib => ../acme-lib\n")
	goload.ClearLoadCacheForTest()
	got, err := Discover(nil, consumer, nil, []string{"github.com/acme/lib"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Loc.ImportPath != "github.com/acme/lib" {
		t.Fatalf("got %#v", got)
	}
}

func TestDiscoverForstDep_transitiveFtImport(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	consumer := filepath.Join(root, "consumer")
	libA := filepath.Join(root, "acme-a")
	libB := filepath.Join(root, "acme-b")
	mustMkdir(t, consumer)
	mustMkdir(t, libA)
	mustMkdir(t, libB)
	write(t, filepath.Join(libB, "go.mod"), "module github.com/acme/b\n\ngo "+testmod.GoVersion+"\n")
	write(t, filepath.Join(libB, "val.ft"), "package b\n\nfunc Val() {\n\treturn 7\n}\n")
	write(t, filepath.Join(libA, "go.mod"), "module github.com/acme/a\n\ngo "+testmod.GoVersion+"\n\nrequire github.com/acme/b v0.0.0\n\nreplace github.com/acme/b => ../acme-b\n")
	write(t, filepath.Join(libA, "wrap.ft"), "package a\n\nimport \"github.com/acme/b\"\n\nfunc Wrap() {\n\treturn b.Val()\n}\n")
	write(t, filepath.Join(consumer, "go.mod"), testmod.GoModContent("testmod")+"\nrequire github.com/acme/a v0.0.0\n\nreplace github.com/acme/a => ../acme-a\n\nreplace github.com/acme/b => ../acme-b\n")
	goload.ClearLoadCacheForTest()
	got, err := Discover(nil, consumer, nil, []string{"github.com/acme/a"})
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, p := range got {
		paths[p.Loc.ImportPath] = true
	}
	if !paths["github.com/acme/a"] || !paths["github.com/acme/b"] {
		t.Fatalf("want both a and b, got %#v", got)
	}
}

func TestCopyModuleForOverlay_skipsFtAndGenGo(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	dst := t.TempDir()
	write(t, filepath.Join(src, "go.mod"), "module github.com/acme/lib\n\ngo "+testmod.GoVersion+"\n")
	write(t, filepath.Join(src, "add.ft"), "package lib\n")
	write(t, filepath.Join(src, "lib.gen.go"), "package lib\n")
	write(t, filepath.Join(src, "helpers.go"), "package lib\n\nfunc Help() {}\n")
	if err := CopyModuleForOverlay(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "helpers.go")); err != nil {
		t.Fatalf("helpers.go missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "add.ft")); !os.IsNotExist(err) {
		t.Fatal("add.ft should not be copied")
	}
	if _, err := os.Stat(filepath.Join(dst, "lib.gen.go")); !os.IsNotExist(err) {
		t.Fatal("lib.gen.go should not be copied")
	}
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
