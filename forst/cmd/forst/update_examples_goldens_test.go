package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forst/internal/compiler"
	"forst/internal/ftconfig"
)

// TestUpdateExamplesGoldens regenerates committed Go goldens under examples/out/ and
// Effect-mode generate snapshots under testdata/effect/golden/.
// Formats example .ft inputs (forst fmt) and emits Go via generators.GenerateGoCode (same as the compiler).
// Run from forst/: UPDATE_EXAMPLES_GOLDENS=1 go test ./cmd/forst -run TestUpdateExamplesGoldens -count=1
// Or: task examples:update-goldens
func TestUpdateExamplesGoldens(t *testing.T) {
	if os.Getenv("UPDATE_EXAMPLES_GOLDENS") != "1" {
		t.Skip("set UPDATE_EXAMPLES_GOLDENS=1 to regenerate all example goldens")
	}

	inputDir := filepath.Join("..", "..", "..", "examples", "in")
	outputDir := filepath.Join("..", "..", "..", "examples", "out")

	err := filepath.Walk(inputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".ft") {
			return nil
		}

		relPath, err := filepath.Rel(inputDir, path)
		if err != nil {
			return err
		}
		if shouldSkipExampleGoldenUpdate(relPath) {
			return nil
		}

		baseName := strings.TrimSuffix(relPath, filepath.Ext(relPath))
		outputBasePath := filepath.Join(outputDir, baseName)

		expectedFiles, err := findExpectedOutputFiles(outputBasePath)
		if err != nil {
			return err
		}
		if len(expectedFiles) == 0 {
			return nil
		}

		t.Run(relPath, func(t *testing.T) {
			writeExampleGolden(t, path, outputBasePath, exampleGoldenCompileOpts{})
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("tictactoe/main/server.ft", func(t *testing.T) {
		updateTictactoeGolden(t)
	})
	t.Run("rfc/providers/providers_demo/providers.ft", func(t *testing.T) {
		updateProvidersMergedGolden(t)
	})
	t.Run("rfc/providers/cross_pkg", func(t *testing.T) {
		updateProvidersCrossPkgGolden(t)
	})
	t.Run("map_catalog.ft", func(t *testing.T) {
		updateMapCatalogGolden(t)
	})
	for _, tc := range nodeInteropGoldenCases() {
		tc := tc
		t.Run(tc.entryRel, func(t *testing.T) {
			writeNodeInteropPackageGolden(t, tc)
		})
	}
	for _, tc := range embeddedInvokeGoldenCases() {
		tc := tc
		t.Run(tc.entryRel, func(t *testing.T) {
			writeEmbeddedInvokePackageGolden(t, tc)
		})
	}
	t.Run("testdata/effect/golden", func(t *testing.T) {
		updateEffectGenerateGoldens(t)
	})
}

func shouldSkipExampleGoldenUpdate(relPath string) bool {
	if isWipExampleSkip(relPath) {
		return true
	}
	if strings.HasPrefix(relPath, "tictactoe/") {
		return true
	}
	if strings.HasPrefix(relPath, "rfc/providers/") {
		return true
	}
	if strings.HasPrefix(relPath, "rfc/bridge-interop/") {
		return true
	}
	if strings.HasPrefix(relPath, "rfc/embedded-invoke/") {
		return true
	}
	return false
}

func writeExampleGolden(t *testing.T, inPath, outputBasePath string, opts exampleGoldenCompileOpts) {
	t.Helper()

	absIn, err := filepath.Abs(inPath)
	if err != nil {
		t.Fatal(err)
	}
	formatExampleGoldenInputs(t, absIn, opts)
	code := compileExampleForGolden(t, absIn, opts)

	dest, err := goldenDestForOutputBase(outputBasePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", dest)
}

func goldenDestForOutputBase(outputBasePath string) (string, error) {
	expectedFiles, err := findExpectedOutputFiles(outputBasePath)
	if err != nil {
		return "", err
	}
	switch len(expectedFiles) {
	case 0:
		return outputBasePath + ".go", nil
	case 1:
		return expectedFiles[0], nil
	default:
		base := filepath.Base(outputBasePath) + ".go"
		for _, f := range expectedFiles {
			if filepath.Base(f) == base {
				return f, nil
			}
		}
		return expectedFiles[0], nil
	}
}

func updateTictactoeGolden(t *testing.T) {
	t.Helper()
	root := filepath.Join("..", "..", "..", "examples", "in", "tictactoe")
	entry := filepath.Join(root, "main", "server.ft")
	goldenPath := filepath.Join("..", "..", "..", "examples", "out", "tictactoe", "server.go")

	opts := exampleGoldenCompileOpts{
		packageRoot:        root,
		exportStructFields: ftconfig.ExportStructFieldsFromDir(root),
	}
	formatExampleGoldenInputs(t, entry, opts)
	code := compileExampleForGolden(t, entry, opts)
	if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goldenPath, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", goldenPath)
}

func updateProvidersMergedGolden(t *testing.T) {
	t.Helper()
	root := filepath.Join("..", "..", "..", "examples", "in", "rfc", "providers")
	entry := filepath.Join(root, "providers_demo", "providers.ft")
	goldenPath := filepath.Join("..", "..", "..", "examples", "out", "rfc", "providers", "providers.go")

	opts := exampleGoldenCompileOpts{packageRoot: root}
	formatExampleGoldenInputs(t, entry, opts)
	code := compileExampleForGolden(t, entry, opts)
	if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goldenPath, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", goldenPath)
}

func updateProvidersCrossPkgGolden(t *testing.T) {
	t.Helper()
	cases := []struct {
		root, entry, golden string
	}{
		{
			root:   filepath.Join("..", "..", "..", "examples", "in", "rfc", "providers", "cross_pkg", "auth"),
			entry:  filepath.Join("..", "..", "..", "examples", "in", "rfc", "providers", "cross_pkg", "auth", "log.ft"),
			golden: filepath.Join("..", "..", "..", "examples", "out", "rfc", "providers", "cross_pkg", "auth", "log.go"),
		},
		{
			root:   filepath.Join("..", "..", "..", "examples", "in", "rfc", "providers", "cross_pkg", "api"),
			entry:  filepath.Join("..", "..", "..", "examples", "in", "rfc", "providers", "cross_pkg", "api", "handle.ft"),
			golden: filepath.Join("..", "..", "..", "examples", "out", "rfc", "providers", "cross_pkg", "api", "handle.go"),
		},
	}
	for _, tc := range cases {
		formatExampleFtInPlace(t, tc.entry)
		c := compiler.New(compiler.Args{
			Command:  "run",
			FilePath: tc.entry,
			LogLevel: "error",
		}, nil)
		code, err := c.CompileFile()
		if err != nil {
			t.Fatalf("%s: CompileFile: %v", tc.entry, err)
		}
		if err := os.MkdirAll(filepath.Dir(tc.golden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(tc.golden, []byte(*code), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", tc.golden)
	}
}

func updateMapCatalogGolden(t *testing.T) {
	t.Helper()
	inPath := filepath.Join("..", "..", "..", "examples", "in", "map_catalog.ft")
	goldenPath := filepath.Join("..", "..", "..", "examples", "out", "map_catalog.go")

	formatExampleFtInPlace(t, inPath)
	code := compileExampleForGolden(t, inPath, exampleGoldenCompileOpts{})
	if err := os.WriteFile(goldenPath, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", goldenPath)
}
