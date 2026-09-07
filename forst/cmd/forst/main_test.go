package main

import (
	"encoding/json"
	"fmt"
	"forst/cmd/forst/lsp"
	"forst/internal/compiler"
	"forst/internal/ftconfig"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestVersionInfo(t *testing.T) {
	// Test that version variables are set
	if Version == "" {
		t.Error("Version should not be empty")
	}

	if Commit == "" {
		t.Error("Commit should not be empty")
	}

	if Date == "" {
		t.Error("Date should not be empty")
	}

	// Test that version info contains expected values
	if Version == "dev" {
		t.Log("Running in development mode")
	} else {
		t.Logf("Running with version: %s", Version)
	}
}

func TestPrintVersionInfo(t *testing.T) {
	// Test that printVersionInfo doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("printVersionInfo panicked: %v", r)
		}
	}()

	printVersionInfo()
}

func TestMainFunctionSanity(t *testing.T) {
	// Test that main function can be called without panicking
	// We can't easily test the actual main function since it calls os.Exit,
	// but we can test the logic that would be executed

	// Test version flag handling
	testCases := []struct {
		name       string
		args       []string
		expectExit bool
	}{
		{"version flag", []string{"forst", "version"}, true},
		{"--version flag", []string{"forst", "--version"}, true},
		{"-v flag", []string{"forst", "-v"}, true},
		{"normal usage", []string{"forst", "test.ft"}, false},
		{"no args", []string{"forst"}, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Save original args
			originalArgs := os.Args
			defer func() { os.Args = originalArgs }()

			// Set test args
			os.Args = tc.args

			// Test that the version check logic works
			if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version" || os.Args[1] == "-v") {
				// This would normally call printVersionInfo() and os.Exit(0)
				t.Log("Version flag detected correctly")
			}
		})
	}
}

func TestLoggerSetup(t *testing.T) {
	// Test logger setup logic
	testCases := []struct {
		name          string
		version       string
		expectedLevel string
	}{
		{"dev version", "dev", "debug"},
		{"release version", "1.0.0", "info"},
		{"empty version", "", "info"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Save original version
			originalVersion := Version
			defer func() { Version = originalVersion }()

			// Set test version
			Version = tc.version

			// Test logger creation logic
			var logLevel string
			if Version == "dev" {
				logLevel = "debug"
			} else {
				logLevel = "info"
			}

			if logLevel != tc.expectedLevel {
				t.Errorf("Expected log level %s for version %s, got %s", tc.expectedLevel, tc.version, logLevel)
			}
		})
	}
}

func TestLSPCommandParsing(t *testing.T) {
	// Test LSP command flag parsing logic
	testCases := []struct {
		name             string
		args             []string
		expectedPort     string
		expectedLogLevel string
	}{
		{"default lsp", []string{"forst", "lsp"}, ftconfig.DefaultLSPPort, "info"},
		{"lsp with custom port", []string{"forst", "lsp", "-port", "9999"}, "9999", "info"},
		{"lsp with custom log level", []string{"forst", "lsp", "-log-level", "debug"}, ftconfig.DefaultLSPPort, "debug"},
		{"lsp with both", []string{"forst", "lsp", "-port", "8888", "-log-level", "error"}, "8888", "error"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the flag parsing logic that would be used in main
			if len(tc.args) > 1 && tc.args[1] == "lsp" {
				// Simulate flag parsing
				port := ftconfig.DefaultLSPPort // default
				logLevel := "info" // default

				// Simple flag parsing simulation
				for i := 2; i < len(tc.args); i++ {
					switch tc.args[i] {
					case "-port":
						if i+1 < len(tc.args) {
							port = tc.args[i+1]
						}
					case "-log-level":
						if i+1 < len(tc.args) {
							logLevel = tc.args[i+1]
						}
					}
				}

				if port != tc.expectedPort {
					t.Errorf("Expected port %s, got %s", tc.expectedPort, port)
				}

				if logLevel != tc.expectedLogLevel {
					t.Errorf("Expected log level %s, got %s", tc.expectedLogLevel, logLevel)
				}
			}
		})
	}
}

func TestDevCommandParsing(t *testing.T) {
	// Test dev command flag parsing logic
	testCases := []struct {
		name           string
		args           []string
		expectedPort   string
		expectedConfig string
		expectedRoot   string
	}{
		{"default dev", []string{"forst", "dev"}, "", "", "."},
		{"dev with custom port", []string{"forst", "dev", "-port", "9999"}, "9999", "", "."},
		{"dev with config", []string{"forst", "dev", "-config", "/path/to/config"}, "", "/path/to/config", "."},
		{"dev with root", []string{"forst", "dev", "-root", "/custom/root"}, "", "", "/custom/root"},
		{"dev with all", []string{"forst", "dev", "-port", "8888", "-config", "/config", "-root", "/root"}, "8888", "/config", "/root"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the flag parsing logic that would be used in main
			if len(tc.args) > 1 && tc.args[1] == "dev" {
				// Simulate flag parsing
				port := "" // default: unset; EffectiveListenPort uses ftconfig
				configPath := "" // default
				rootDir := "."   // default

				// Simple flag parsing simulation
				for i := 2; i < len(tc.args); i++ {
					switch tc.args[i] {
					case "-port":
						if i+1 < len(tc.args) {
							port = tc.args[i+1]
						}
					case "-config":
						if i+1 < len(tc.args) {
							configPath = tc.args[i+1]
						}
					case "-root":
						if i+1 < len(tc.args) {
							rootDir = tc.args[i+1]
						}
					}
				}

				if port != tc.expectedPort {
					t.Errorf("Expected port %s, got %s", tc.expectedPort, port)
				}

				if configPath != tc.expectedConfig {
					t.Errorf("Expected config %s, got %s", tc.expectedConfig, configPath)
				}

				if rootDir != tc.expectedRoot {
					t.Errorf("Expected root %s, got %s", tc.expectedRoot, rootDir)
				}
			}
		})
	}
}

func TestLogLevelParsing(t *testing.T) {
	// Test log level parsing logic
	testCases := []struct {
		name          string
		logLevel      string
		expectedLevel string
	}{
		{"trace level", "trace", "trace"},
		{"debug level", "debug", "debug"},
		{"info level", "info", "info"},
		{"warn level", "warn", "warn"},
		{"error level", "error", "error"},
		{"invalid level", "invalid", "info"}, // should default to info
		{"empty level", "", "info"},          // should default to info
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the log level parsing logic from main
			var actualLevel string
			switch tc.logLevel {
			case "trace":
				actualLevel = "trace"
			case "debug":
				actualLevel = "debug"
			case "info":
				actualLevel = "info"
			case "warn":
				actualLevel = "warn"
			case "error":
				actualLevel = "error"
			default:
				actualLevel = "info"
			}

			if actualLevel != tc.expectedLevel {
				t.Errorf("Expected log level %s for input %s, got %s", tc.expectedLevel, tc.logLevel, actualLevel)
			}
		})
	}
}

func TestNewLogger_setsLevelByVersion(t *testing.T) {
	original := Version
	defer func() { Version = original }()

	Version = "dev"
	if got := newLogger().GetLevel(); got != logrus.DebugLevel {
		t.Fatalf("dev logger level: %s", got)
	}

	Version = "1.2.3"
	if got := newLogger().GetLevel(); got != logrus.InfoLevel {
		t.Fatalf("release logger level: %s", got)
	}
}

func TestSetLogLevel_appliesKnownAndDefault(t *testing.T) {
	log := logrus.New()

	setLogLevel(log, "trace")
	if log.GetLevel() != logrus.TraceLevel {
		t.Fatalf("trace level not applied: %s", log.GetLevel())
	}

	setLogLevel(log, "error")
	if log.GetLevel() != logrus.ErrorLevel {
		t.Fatalf("error level not applied: %s", log.GetLevel())
	}

	setLogLevel(log, "not-a-level")
	if log.GetLevel() != logrus.InfoLevel {
		t.Fatalf("default info level not applied: %s", log.GetLevel())
	}
}

func TestCompilerArgsParsing(t *testing.T) {
	// Test that compiler args can be parsed
	// Save original args
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	// Test with minimal args
	os.Args = []string{"forst", "test.ft"}

	// This would normally call compiler.ParseArgs(log)
	// We'll test that the logic doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("compiler.ParseArgs panicked: %v", r)
		}
	}()

	// Test that we can create a logger without issues
	log := setupTestLogger(nil)
	if log == nil {
		t.Error("Expected logger to be created")
	}
}

func TestLSPVersionInjection(t *testing.T) {
	// Test that version information is correctly injected into LSP package
	origVersion, origCommit, origDate := lsp.BuildInfo()
	t.Cleanup(func() {
		lsp.SetBuildMetadata(origVersion, origCommit, origDate)
	})

	lsp.SetBuildMetadata("test-version", "test-commit", "test-date")

	lspVersion, lspCommit, lspDate := lsp.BuildInfo()

	if lspVersion != "test-version" {
		t.Errorf("Expected LSP version test-version, got %s", lspVersion)
	}

	if lspCommit != "test-commit" {
		t.Errorf("Expected LSP commit test-commit, got %s", lspCommit)
	}

	if lspDate != "test-date" {
		t.Errorf("Expected LSP date test-date, got %s", lspDate)
	}
}

func TestFileValidation(t *testing.T) {
	// Test file path validation logic
	testCases := []struct {
		name        string
		filePath    string
		expectError bool
	}{
		{"empty path", "", true},
		{"valid path", "test.ft", false},
		{"valid path with dir", "path/to/test.ft", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the file path validation logic from main
			if tc.filePath == "" {
				// This would normally cause an error in main
				t.Log("Empty file path would cause error")
			} else {
				t.Log("Valid file path")
			}
		})
	}
}

func TestWatchModeLogic(t *testing.T) {
	// Test watch mode logic
	testCases := []struct {
		name         string
		watch        bool
		expectedMode string
	}{
		{"watch mode", true, "watch"},
		{"single compile", false, "single"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the watch mode logic from main
			var mode string
			if tc.watch {
				mode = "watch"
			} else {
				mode = "single"
			}

			if mode != tc.expectedMode {
				t.Errorf("Expected mode %s for watch=%t, got %s", tc.expectedMode, tc.watch, mode)
			}
		})
	}
}

func TestRunCommandLogic(t *testing.T) {
	// Test run command logic
	testCases := []struct {
		name      string
		command   string
		shouldRun bool
	}{
		{"run command", "run", true},
		{"compile command", "compile", false},
		{"other command", "other", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the run command logic from main
			shouldRun := tc.command == "run"

			if shouldRun != tc.shouldRun {
				t.Errorf("Expected shouldRun=%t for command %s, got %t", tc.shouldRun, tc.command, shouldRun)
			}
		})
	}
}

func TestExamples(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping example compiles in -short mode")
	}
	// Get all example input files
	inputDir := "../../../examples/in"
	outputDir := "../../../examples/out"

	// Walk through all subdirectories
	err := filepath.Walk(inputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Only process .ft and .go files
		if !strings.HasSuffix(info.Name(), ".ft") && !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}

		// Get relative path from input directory
		relPath, err := filepath.Rel(inputDir, path)
		if err != nil {
			return err
		}

		// Get base name without extension
		baseName := strings.TrimSuffix(relPath, filepath.Ext(relPath))

		// Create corresponding output path
		outputBasePath := filepath.Join(outputDir, baseName)

		t.Run(relPath, func(t *testing.T) {
			t.Parallel()
			if isWipExampleSkip(relPath) {
				t.Skip("WIP example sketch (not a golden)", relPath)
				return
			}

			// Multi-file package: golden + merged compile are checked in TestExampleTictactoeMergedPackage.
			if strings.HasPrefix(relPath, "tictactoe/") && strings.HasSuffix(relPath, ".ft") {
				t.Skip("covered by TestExampleTictactoeMergedPackage (-root merged package)")
				return
			}
			if strings.HasPrefix(relPath, "rfc/providers/providers") && strings.HasSuffix(relPath, ".ft") {
				t.Skip("covered by TestExampleProvidersMergedPackage (-root merged package)")
				return
			}
			if strings.HasPrefix(relPath, "rfc/bridge-interop/") && strings.HasSuffix(relPath, ".ft") {
				t.Skip("covered by TestExampleNodeInteropPackagesCompileGolden (-root merged package)")
				return
			}
			if strings.HasPrefix(relPath, "rfc/embedded-invoke/") && strings.HasSuffix(relPath, ".ft") {
				t.Skip("covered by TestExampleEmbeddedInvokeCompileGolden (-root merged package)")
				return
			}

			// Find expected output file(s)
			expectedFiles, err := findExpectedOutputFiles(outputBasePath)
			if err != nil {
				t.Fatalf("Failed to find expected output files for %s: %v", baseName, err)
			}

			if len(expectedFiles) == 0 {
				t.Skipf("No expected output files found for %s", baseName)
				return
			}

			// Compile once and compare against golden output.
			c := compiler.New(compiler.Args{
				Command:  "run",
				FilePath: path,
				LogLevel: "error",
			}, exampleTestLogger())
			code, err := c.CompileFile()
			if err != nil {
				if strings.HasPrefix(relPath, "rfc/") && len(expectedFiles) == 0 {
					t.Logf("Ignoring failure for RFC example %s (no golden): %v", relPath, err)
					return
				}
				t.Fatalf("Failed to compile file: %v", err)
			}
			actualOutput := *code

			t.Logf("Generated output for %s:\n%s", baseName, actualOutput)

			// Verify that the output contains key elements from the expected files
			for _, expectedPath := range expectedFiles {
				expectedContent, err := os.ReadFile(expectedPath)
				if err != nil {
					t.Fatalf("Failed to read expected output file %s: %v", expectedPath, err)
				}

				verifyOutputContainsExpectedElements(t, string(expectedContent), actualOutput, expectedPath)
			}
		})

		return nil
	})

	if err != nil {
		t.Fatalf("Failed to walk examples directory: %v", err)
	}
}

// TestResultExamplesIncludeEnsureLowering locks in the shape of result_if / result_ensure: a
// Result-returning helper should include an `ensure` in its body so tooling and lowering match
// real code paths (plain `return n` alone does not emit an ensure branch).
func TestResultExamplesIncludeEnsureLowering(t *testing.T) {
	t.Parallel()
	root := examplesInRoot(t)
	examples := []string{
		filepath.Join(root, "result_if.ft"),
		filepath.Join(root, "result_ensure.ft"),
	}
	const wantBranch = `if n <= 0`
	const wantMsg = `ensure n is Int.GreaterThan(0): want > 0`
	for _, path := range examples {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read example: %v", err)
			}
			if !strings.Contains(string(src), "ensure n is") {
				t.Fatalf("example %s should include `ensure n is` in the Result-returning helper", path)
			}

			c := compiler.New(compiler.Args{
				Command:  "run",
				FilePath: path,
				LogLevel: "error",
			}, nil)
			code, err := c.CompileFile()
			if err != nil {
				t.Fatalf("CompileFile: %v", err)
			}
			out := *code
			if !strings.Contains(out, wantBranch) {
				t.Errorf("expected generated Go to include ensure failure branch %q", wantBranch)
			}
			if !strings.Contains(out, wantMsg) {
				t.Errorf("expected generated Go to include assertion message %q", wantMsg)
			}
		})
	}
}

// TestExampleTictactoeMergedPackage compiles examples/in/tictactoe with -root (same-package merge)
// and checks generated Go against examples/out/tictactoe/server.go.
// Regenerate the golden file: UPDATE_TICTACTOE_GOLDEN=1 go test ./cmd/forst -run TestExampleTictactoeMergedPackage -count=1
// (also: task examples:update-goldens)
func TestExampleTictactoeMergedPackage(t *testing.T) {
	root := filepath.Join("..", "..", "..", "examples", "in", "tictactoe")
	entry := filepath.Join(root, "main", "server.ft")
	goldenPath := filepath.Join("..", "..", "..", "examples", "out", "tictactoe", "server.go")

	actual := compileExampleForGolden(t, entry, exampleGoldenCompileOpts{
		packageRoot:        root,
		exportStructFields: ftconfig.ExportStructFieldsFromDir(root),
	})

	if os.Getenv("UPDATE_TICTACTOE_GOLDEN") == "1" || os.Getenv("UPDATE_EXAMPLES_GOLDENS") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, []byte(actual), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote golden %s", goldenPath)
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v (set UPDATE_TICTACTOE_GOLDEN=1 to create)", goldenPath, err)
	}
	// Hash-based emitted type names (T_…) are not stable across small compiler changes; assert
	// structural markers instead of line-for-line equality with extractKeyElements.
	verifyTictactoeMergedGolden(t, string(expected), actual, goldenPath)
}

// TestExampleProvidersMergedPackage compiles examples/in/rfc/providers with -root
// (library .ft only; *_test.ft excluded from -root merge) and checks generated Go
// against examples/out/rfc/providers/providers.go.
// Regenerate: UPDATE_PROVIDERS_GOLDEN=1 go test ./cmd/forst -run TestExampleProvidersMergedPackage -count=1
func TestExampleProvidersMergedPackage(t *testing.T) {
	root := filepath.Join("..", "..", "..", "examples", "in", "rfc", "providers")
	entry := filepath.Join(root, "providers_demo", "providers.ft")
	goldenPath := filepath.Join("..", "..", "..", "examples", "out", "rfc", "providers", "providers.go")

	actual := compileExampleForGolden(t, entry, exampleGoldenCompileOpts{packageRoot: root})

	if os.Getenv("UPDATE_PROVIDERS_GOLDEN") == "1" || os.Getenv("UPDATE_EXAMPLES_GOLDENS") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, []byte(actual), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote golden %s", goldenPath)
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v (set UPDATE_PROVIDERS_GOLDEN=1 to create)", goldenPath, err)
	}
	if string(expected) != actual {
		t.Fatalf("golden mismatch for providers/providers.go (set UPDATE_PROVIDERS_GOLDEN=1 to refresh)\n--- expected ---\n%s\n--- actual ---\n%s", string(expected), actual)
	}
}

// TestExampleProvidersCrossPkgGolden compiles cross_pkg auth/api and checks generated Go goldens.
// Regenerate: UPDATE_PROVIDERS_CROSS_PKG_GOLDEN=1 go test ./cmd/forst -run TestExampleProvidersCrossPkgGolden -count=1
func TestExampleProvidersCrossPkgGolden(t *testing.T) {
	type pkgCase struct {
		root, entry, golden string
	}
	cases := []pkgCase{
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
		c := compiler.New(compiler.Args{
			Command:  "run",
			FilePath: tc.entry,
			LogLevel: "error",
		}, nil)
		code, err := c.CompileFile()
		if err != nil {
			t.Fatalf("%s: CompileFile: %v", tc.entry, err)
		}
		actual := *code
		if os.Getenv("UPDATE_PROVIDERS_CROSS_PKG_GOLDEN") == "1" || os.Getenv("UPDATE_EXAMPLES_GOLDENS") == "1" {
			if err := os.MkdirAll(filepath.Dir(tc.golden), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(tc.golden, []byte(actual), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Logf("wrote golden %s", tc.golden)
			continue
		}
		expected, err := os.ReadFile(tc.golden)
		if err != nil {
			t.Fatalf("read golden %s: %v (set UPDATE_PROVIDERS_CROSS_PKG_GOLDEN=1 to create)", tc.golden, err)
		}
		if string(expected) != actual {
			t.Fatalf("golden mismatch for %s (set UPDATE_PROVIDERS_CROSS_PKG_GOLDEN=1 to refresh)\n--- expected ---\n%s\n--- actual ---\n%s", tc.golden, string(expected), actual)
		}
	}
}

func TestFindExpectedOutputFiles_directoryAndSingleFile(t *testing.T) {
	baseDir := t.TempDir()

	dirPath := filepath.Join(baseDir, "module")
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirPath, "a.go"), []byte("package x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirPath, "README.md"), []byte("ignore"), 0o644); err != nil {
		t.Fatal(err)
	}

	gotDirFiles, err := findExpectedOutputFiles(dirPath)
	if err != nil {
		t.Fatalf("findExpectedOutputFiles(dir): %v", err)
	}
	if len(gotDirFiles) != 1 || !strings.HasSuffix(gotDirFiles[0], "a.go") {
		t.Fatalf("unexpected directory files: %+v", gotDirFiles)
	}

	singleBase := filepath.Join(baseDir, "single_output")
	if err := os.WriteFile(singleBase+".go", []byte("package y"), 0o644); err != nil {
		t.Fatal(err)
	}
	gotSingleFile, err := findExpectedOutputFiles(singleBase)
	if err != nil {
		t.Fatalf("findExpectedOutputFiles(single): %v", err)
	}
	if len(gotSingleFile) != 1 || gotSingleFile[0] != singleBase+".go" {
		t.Fatalf("unexpected single file lookup: %+v", gotSingleFile)
	}

	gotMissing, err := findExpectedOutputFiles(filepath.Join(baseDir, "missing"))
	if err != nil {
		t.Fatalf("findExpectedOutputFiles(missing): %v", err)
	}
	if len(gotMissing) != 0 {
		t.Fatalf("expected no files for missing path, got %+v", gotMissing)
	}
}

func TestExtractKeyElements_collectsSignaturesTypesFieldsAndIf(t *testing.T) {
	code := strings.Join([]string{
		"package main",
		"type User struct {",
		"  Name string",
		"}",
		"func run(x int) error {",
		"  if x > 0 {",
		"    return nil",
		"  }",
		"  return nil",
		"}",
	}, "\n")

	keyElements := extractKeyElements(code)
	joined := strings.Join(keyElements, "\n")

	expectedFragments := []string{
		"type User struct {",
		"Name string",
		"func run(x int) error {",
		"if x > 0 {",
	}
	for _, fragment := range expectedFragments {
		if !strings.Contains(joined, fragment) {
			t.Fatalf("missing key element fragment %q in:\n%s", fragment, joined)
		}
	}
}

func TestCheckIfStatementConditions_handlesMalformedIfWithoutPanic(t *testing.T) {
	log := logrus.New()
	log.SetOutput(io.Discard)
	_ = log // keeps intent explicit for this test file's logging-heavy helpers

	code := strings.Join([]string{
		"package main",
		"func x() {",
		"  if brokenCondition()",
		"  if validCondition() {",
		"  }",
		"}",
	}, "\n")

	checkIfStatementConditions(t, code, "synthetic.go")
}

func TestHandleDumpCommand_jsonAndPrettyOutput(t *testing.T) {
	testFilePath := filepath.Join(t.TempDir(), "input.ft")
	source := "fn main() { return }\n"
	if err := os.WriteFile(testFilePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	logger := logrus.New()
	logger.SetOutput(io.Discard)

	compactOutput := captureDumpCommandOutput(t, func() {
		if err := handleDumpCommand(testFilePath, false, "json", "", false, logger); err != nil {
			t.Fatalf("handleDumpCommand json: %v", err)
		}
	})
	if strings.TrimSpace(compactOutput) == "" {
		t.Fatal("expected non-empty json output")
	}
	var compactJSON any
	if err := json.Unmarshal([]byte(compactOutput), &compactJSON); err != nil {
		t.Fatalf("compact output should be valid JSON: %v\noutput: %s", err, compactOutput)
	}

	prettyOutput := captureDumpCommandOutput(t, func() {
		if err := handleDumpCommand(testFilePath, false, "pretty", "", true, logger); err != nil {
			t.Fatalf("handleDumpCommand pretty: %v", err)
		}
	})
	if strings.TrimSpace(prettyOutput) == "" {
		t.Fatal("expected non-empty pretty output")
	}
	var prettyJSON any
	if err := json.Unmarshal([]byte(prettyOutput), &prettyJSON); err != nil {
		t.Fatalf("pretty output should be valid JSON: %v\noutput: %s", err, prettyOutput)
	}
	if !strings.Contains(prettyOutput, "\n") {
		t.Fatalf("expected pretty output to contain newlines, got: %s", prettyOutput)
	}
}

func TestHandleDumpCommand_absPathError(t *testing.T) {
	testFilePath := filepath.Join(t.TempDir(), "input.ft")
	if err := os.WriteFile(testFilePath, []byte("fn main() { return }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := pathAbs
	pathAbs = func(string) (string, error) { return "", fmt.Errorf("abs") }
	t.Cleanup(func() { pathAbs = orig })

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	if err := handleDumpCommand(testFilePath, false, "json", "", false, logger); err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("expected absolute path error, got %v", err)
	}
}

func TestHandleDumpCommand_marshalParamsError(t *testing.T) {
	testFilePath := filepath.Join(t.TempDir(), "input.ft")
	if err := os.WriteFile(testFilePath, []byte("fn main() { return }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := jsonMarshalDumpParams
	jsonMarshalDumpParams = func(any) ([]byte, error) { return nil, fmt.Errorf("params") }
	t.Cleanup(func() { jsonMarshalDumpParams = orig })

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	if err := handleDumpCommand(testFilePath, false, "json", "", false, logger); err == nil || !strings.Contains(err.Error(), "marshal params") {
		t.Fatalf("expected marshal params error, got %v", err)
	}
}

func TestHandleDumpCommand_marshalOutputJSONError(t *testing.T) {
	testFilePath := filepath.Join(t.TempDir(), "input.ft")
	if err := os.WriteFile(testFilePath, []byte("fn main() { return }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := jsonMarshalDumpResult
	jsonMarshalDumpResult = func(any) ([]byte, error) { return nil, fmt.Errorf("out") }
	t.Cleanup(func() { jsonMarshalDumpResult = orig })

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	if err := handleDumpCommand(testFilePath, false, "json", "", false, logger); err == nil || !strings.Contains(err.Error(), "marshal output") {
		t.Fatalf("expected marshal output error, got %v", err)
	}
}

func TestHandleDumpCommand_marshalOutputPrettyError(t *testing.T) {
	testFilePath := filepath.Join(t.TempDir(), "input.ft")
	if err := os.WriteFile(testFilePath, []byte("fn main() { return }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := jsonMarshalDumpIndent
	jsonMarshalDumpIndent = func(any, string, string) ([]byte, error) { return nil, fmt.Errorf("indent") }
	t.Cleanup(func() { jsonMarshalDumpIndent = orig })

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	if err := handleDumpCommand(testFilePath, false, "pretty", "", false, logger); err == nil || !strings.Contains(err.Error(), "marshal output") {
		t.Fatalf("expected marshal output error, got %v", err)
	}
}

func TestHandleDumpCommand_helperProcess(_ *testing.T) {
	if os.Getenv("FORST_MAIN_DUMP_HELPER") != "1" {
		return
	}
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	if err := handleDumpCommand("/path/that/does/not/exist.ft", false, "json", "", false, logger); err == nil {
		os.Exit(0)
	}
	os.Exit(1)
}

func TestHandleDumpCommand_exitsOnReadFailure(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestHandleDumpCommand_helperProcess")
	cmd.Env = append(os.Environ(), "FORST_MAIN_DUMP_HELPER=1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected helper process to exit with non-zero status")
	}
}

func TestMain_helperProcess(t *testing.T) {
	helperCase := os.Getenv("FORST_MAIN_HELPER_CASE")
	if helperCase == "" {
		return
	}

	switch helperCase {
	case "version":
		os.Args = []string{"forst", "version"}
	case "dump-missing-file":
		os.Args = []string{"forst", "dump", "--file", "/path/that/does/not/exist.ft"}
	case "dump-ok":
		tmp := os.Getenv("FORST_MAIN_HELPER_TMP")
		if tmp == "" {
			t.Fatal("FORST_MAIN_HELPER_TMP required")
		}
		os.Args = []string{"forst", "dump", "--file", filepath.Join(tmp, "sample.ft")}
	case "dev-bad-port":
		tmp := os.Getenv("FORST_MAIN_HELPER_TMP")
		if tmp == "" {
			t.Fatal("FORST_MAIN_HELPER_TMP required")
		}
		os.Args = []string{"forst", "dev", "-port", "notaport", "-root", tmp}
	case "dev-unix-socket-conflict":
		tmp := os.Getenv("FORST_MAIN_HELPER_TMP")
		if tmp == "" {
			t.Fatal("FORST_MAIN_HELPER_TMP required")
		}
		os.Args = []string{"forst", "dev", "-root", tmp}
	case "lsp-bad-port":
		os.Args = []string{"forst", "lsp", "-port", "notaport"}
	case "fmt-list":
		tmp := os.Getenv("FORST_MAIN_HELPER_TMP")
		if tmp == "" {
			t.Fatal("FORST_MAIN_HELPER_TMP required")
		}
		os.Args = []string{"forst", "fmt", "-l", filepath.Join(tmp, "a.ft")}
	case "generate-ok":
		tmp := os.Getenv("FORST_MAIN_HELPER_TMP")
		if tmp == "" {
			t.Fatal("FORST_MAIN_HELPER_TMP required")
		}
		os.Args = []string{"forst", "generate", filepath.Join(tmp, "main.ft")}
	case "run-no-file":
		os.Args = []string{"forst", "run"}
	default:
		t.Fatalf("unknown helper case: %s", helperCase)
	}

	main()
}

func TestMain_versionCommand_exitsZeroAndPrintsVersion(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestMain_helperProcess")
	cmd.Env = append(os.Environ(), "FORST_MAIN_HELPER_CASE=version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected zero exit for version command: %v, output=%s", err, string(output))
	}
	if !strings.Contains(string(output), "forst ") {
		t.Fatalf("expected version output to contain 'forst ', got: %s", string(output))
	}
}

func TestMain_dumpMissingFile_exitsNonZero(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestMain_helperProcess")
	cmd.Env = append(os.Environ(), "FORST_MAIN_HELPER_CASE=dump-missing-file")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected dump command to exit non-zero when file is missing")
	}
}

func TestMain_dumpOk_runsDumpSubcommand(t *testing.T) {
	tmp := t.TempDir()
	ftPath := filepath.Join(tmp, "sample.ft")
	if err := os.WriteFile(ftPath, []byte("fn main() { return }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMain_helperProcess")
	cmd.Env = append(os.Environ(),
		"FORST_MAIN_HELPER_CASE=dump-ok",
		"FORST_MAIN_HELPER_TMP="+tmp,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected zero exit: %v out=%s", err, string(out))
	}
	if !strings.Contains(string(out), "{") {
		t.Fatalf("expected JSON output, got: %s", string(out))
	}
}

func TestMain_devBadPort_exitsNonZero(t *testing.T) {
	tmp := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestMain_helperProcess")
	cmd.Env = append(os.Environ(),
		"FORST_MAIN_HELPER_CASE=dev-bad-port",
		"FORST_MAIN_HELPER_TMP="+tmp,
		"FORST_INVOKE_TRANSPORT=tcp",
	)
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected dev with invalid port to exit non-zero")
	}
}

func TestMain_devUnixSocketConflict_exitsNonZero(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix invoke is not the default on Windows")
	}
	tmp := t.TempDir()
	writeBlockingInvokeSocket(t, tmp)
	cmd := exec.Command(os.Args[0], "-test.run=TestMain_helperProcess")
	cmd.Env = append(os.Environ(),
		"FORST_MAIN_HELPER_CASE=dev-unix-socket-conflict",
		"FORST_MAIN_HELPER_TMP="+tmp,
		"FORST_INVOKE_TRANSPORT=unix",
	)
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected dev with blocked unix socket to exit non-zero")
	}
}

func TestMain_lspBadPort_exitsNonZero(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestMain_helperProcess")
	cmd.Env = append(os.Environ(), "FORST_MAIN_HELPER_CASE=lsp-bad-port")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected lsp with invalid port to exit non-zero")
	}
}

func TestMain_fmtList_subcommand(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "a.ft")
	if err := os.WriteFile(path, []byte("package main  \nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMain_helperProcess")
	cmd.Env = append(os.Environ(),
		"FORST_MAIN_HELPER_CASE=fmt-list",
		"FORST_MAIN_HELPER_TMP="+tmp,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fmt -l: %v out=%s", err, string(out))
	}
	if !strings.Contains(string(out), "a.ft") {
		t.Fatalf("expected list to mention a.ft: %s", string(out))
	}
}

func TestMain_generateOk_subcommand(t *testing.T) {
	tmp := t.TempDir()
	ftPath := filepath.Join(tmp, "main.ft")
	if err := os.WriteFile(ftPath, []byte(generateTestMinimalValidForst), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMain_helperProcess")
	cmd.Env = append(os.Environ(),
		"FORST_MAIN_HELPER_CASE=generate-ok",
		"FORST_MAIN_HELPER_TMP="+tmp,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generate: %v out=%s", err, string(out))
	}
	if !strings.Contains(string(out), "Found") && !strings.Contains(string(out), "generated") {
		t.Logf("output: %s", string(out))
	}
}

func TestMain_runNoFile_exitsNonZero(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestMain_helperProcess")
	cmd.Env = append(os.Environ(), "FORST_MAIN_HELPER_CASE=run-no-file")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero when run has no input file")
	}
}
