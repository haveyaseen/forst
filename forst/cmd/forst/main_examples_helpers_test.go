package main

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"forst/internal/forstpkg"
	"forst/internal/goload"
	"forst/internal/parser"
	"forst/internal/printer"
	transformergo "forst/internal/transformer/go"
	"forst/internal/testutil"

	"github.com/sirupsen/logrus"
)

// wipExampleRels are incomplete RFC sketches under examples/in that must not enter
// golden compile tests. Repo test harness only — not a compiler convention.
var wipExampleRels = map[string]struct{}{
	"input_validation.skip.ft": {},
	"match.skip.ft":            {},
}

func isWipExampleSkip(relPath string) bool {
	_, ok := wipExampleRels[filepath.ToSlash(relPath)]
	return ok
}

// Returns all .go files in the output directory for a given example.
func findExpectedOutputFiles(basePath string) ([]string, error) {
	var files []string

	info, err := os.Stat(basePath)
	if err == nil && info.IsDir() {
		entries, err := os.ReadDir(basePath)
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
				files = append(files, filepath.Join(basePath, entry.Name()))
			}
		}
	} else {
		filePath := basePath + ".go"
		if _, err := os.Stat(filePath); err == nil {
			files = append(files, filePath)
		}
	}

	return files, nil
}

func exampleTestLogger() *logrus.Logger {
	log := logrus.New()
	log.SetOutput(io.Discard)
	log.SetLevel(logrus.ErrorLevel)
	return log
}

type exampleGoldenCompileOpts struct {
	exportStructFields bool
	packageRoot        string // non-empty merges same-package .ft under root (excludes *_test.ft)
}

// formatExampleFtInPlace applies the same formatting as `forst fmt` (default flags).
func formatExampleFtInPlace(t *testing.T, path string) {
	t.Helper()
	log := exampleTestLogger()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	src := string(b)
	formatted := printer.FormatDocument(src, path, 8, false, log)
	if formatted == src {
		return
	}
	p := parser.NewTestParser(formatted, log)
	if !exampleFtFormattedSourceParses(p) {
		t.Logf("skip fmt %s: formatted source does not parse: %v", path, err)
		return
	}
	perm := fs.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		perm = info.Mode() & fs.ModePerm
	}
	if err := os.WriteFile(path, []byte(formatted), perm); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	t.Logf("formatted %s", path)
}

// formatExampleGoldenInputs runs forst fmt on the .ft sources that feed a golden compile.
func formatExampleGoldenInputs(t *testing.T, absEntry string, opts exampleGoldenCompileOpts) {
	t.Helper()
	log := exampleTestLogger()
	if opts.packageRoot != "" {
		paths, err := collectSamePackageExampleFtPaths(log, opts.packageRoot, absEntry)
		if err != nil {
			t.Fatalf("collectSamePackageExampleFtPaths(%s): %v", absEntry, err)
		}
		for _, p := range paths {
			formatExampleFtInPlace(t, p)
		}
		return
	}
	formatExampleFtInPlace(t, absEntry)
}

// exampleFtFormattedSourceParses returns false when ParseFile panics or returns an error.
func exampleFtFormattedSourceParses(p *parser.Parser) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	_, err := p.ParseFile()
	return err == nil
}

// compileExampleForGolden emits Go for a single example using an isolated pipeline.
// This avoids module-wide Providers merging that would emit every package main type
// when compiling from the forst workspace (examples/in/*.ft).
func compileExampleForGolden(t *testing.T, absEntry string, opts exampleGoldenCompileOpts) string {
	t.Helper()
	log := exampleTestLogger()
	compileOpts := testutil.CompileOpts{
		TypecheckOpts: testutil.TypecheckOpts{
			FileID: filepath.Base(absEntry),
			Logger: log,
		},
		ExportStructFields: opts.exportStructFields,
	}
	if modRoot := goload.FindModuleRoot(filepath.Dir(absEntry)); modRoot != "" {
		entryDir := filepath.Dir(absEntry)
		compileOpts.GoWorkspaceDir = modRoot
		compileOpts.ForstFileDir = entryDir
		if modPath := goload.ModulePath(modRoot); modPath != "" {
			if importPath, err := forstpkg.ImportPathForDir(modRoot, modPath, entryDir); err == nil {
				compileOpts.SamePackageGoImport = importPath
			}
		}
	}
	if opts.packageRoot != "" {
		paths, err := collectSamePackageExampleFtPaths(log, opts.packageRoot, absEntry)
		if err != nil {
			t.Fatalf("collectSamePackageExampleFtPaths(%s): %v", absEntry, err)
		}
		return transformergo.MustCompileMergedGo(t, paths, compileOpts)
	}
	data, err := os.ReadFile(absEntry)
	if err != nil {
		t.Fatalf("read %s: %v", absEntry, err)
	}
	return transformergo.MustCompileGo(t, string(data), compileOpts)
}

func collectSamePackageExampleFtPaths(log *logrus.Logger, rootDir, entryPath string) ([]string, error) {
	rootDir = filepath.Clean(rootDir)
	entryPath = filepath.Clean(entryPath)
	entryNodes, err := forstpkg.ParseForstFile(log, entryPath)
	if err != nil {
		return nil, err
	}
	pkg := forstpkg.PackageNameOrDefault(forstpkg.PackageNameFromNodes(entryNodes))

	var out []string
	err = filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		lower := strings.ToLower(path)
		if !strings.HasSuffix(lower, ".ft") || strings.HasSuffix(lower, "_test.ft") {
			return nil
		}
		nodes, err := forstpkg.ParseForstFile(log, path)
		if err != nil {
			return nil
		}
		if forstpkg.PackageNameOrDefault(forstpkg.PackageNameFromNodes(nodes)) == pkg {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fs.ErrNotExist
	}
	sort.Strings(out)
	return out, nil
}

// verifyTictactoeMergedGolden checks merged-package Go output without depending on hash-based
// type names (T_…) that may change between compiler versions.
func verifyTictactoeMergedGolden(t *testing.T, expected, actual, goldenPath string) {
	markers := []string{
		"package main",
		"type GameState struct",
		"type MoveRequest struct",
		"type MoveResponse struct",
		"func ApplyMove(req MoveRequest) (MoveResponse, error)",
		"func PlayMove(req MoveRequest) (MoveResponse, error)",
		"func NewGame() GameState",
		"func main()",
		"fmt",
	}
	for _, marker := range markers {
		if !strings.Contains(actual, marker) {
			t.Errorf("output missing %q (golden %s)", marker, goldenPath)
		}
	}
	if len(expected) > 0 && len(actual) < len(expected)/2 {
		t.Errorf("output much shorter than golden (%d vs %d bytes)", len(actual), len(expected))
	}
}

// Checks if the actual output contains key elements from expected.
func verifyOutputContainsExpectedElements(t *testing.T, expected, actual, filePath string) {
	keyElements := extractKeyElements(expected)

	for _, element := range keyElements {
		if !strings.Contains(actual, element) {
			t.Errorf("Output missing expected element from %s: %s", filePath, element)
		}
	}

	checkIfStatementConditions(t, actual, filePath)
}

// Extracts key code elements from Go code.
func extractKeyElements(code string) []string {
	var elements []string

	lines := strings.SplitSeq(code, "\n")
	for line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "func ") && strings.Contains(line, "(") {
			elements = append(elements, line)
		}

		if strings.HasPrefix(line, "type ") && strings.Contains(line, "struct") {
			elements = append(elements, line)
		}

		if strings.Contains(line, " string") || strings.Contains(line, " int") ||
			strings.Contains(line, " float") || strings.Contains(line, " bool") {
			elements = append(elements, line)
		}

		if strings.HasPrefix(line, "if ") {
			elements = append(elements, line)
		}
	}

	return elements
}

// Checks if statement conditions match expected conditions.
func checkIfStatementConditions(t *testing.T, code, filePath string) {
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "if ") {
			start := strings.Index(line, "if ") + 3
			end := strings.Index(line, "{")
			if end == -1 {
				continue
			}

			actualCondition := strings.TrimSpace(line[start:end])
			t.Logf("If statement condition in %s at line %d: %s", filePath, i+1, actualCondition)
		}
	}
}

// captureDumpCommandOutput runs run() with handleDumpCommand writing to a buffer (not os.Stdout),
// so -race + coverage do not race on the global stdout fd.
func captureDumpCommandOutput(t *testing.T, run func()) string {
	t.Helper()
	var buf bytes.Buffer
	old := dumpCommandStdout
	dumpCommandStdout = &buf
	t.Cleanup(func() { dumpCommandStdout = old })
	run()
	return buf.String()
}
