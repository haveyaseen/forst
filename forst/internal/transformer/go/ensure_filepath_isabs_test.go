package transformergo

import (
	"strings"
	"testing"
)

func TestPipeline_ensureFilepathIsAbs_thenOpen(t *testing.T) {
	t.Parallel()
	src := `package main

import "os"
import "path/filepath"

func openFile(path String) {
	ensure filepath.IsAbs(path)
	file, err := os.Open(path)
	ensure !err
	return file
}

func main() {}
`
	out := compileForstPipelineExt(t, src, pipelineOpts{goWorkspaceDir: moduleRootFromWD(t)})
	if !strings.Contains(out, "filepath.IsAbs") {
		t.Fatalf("expected filepath.IsAbs in generated Go\n----\n%s\n----", out)
	}
	if !strings.Contains(out, "os.Open") {
		t.Fatalf("expected os.Open in generated Go\n----\n%s\n----", out)
	}
}
