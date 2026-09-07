package forstdep

import (
	"os"
	"path/filepath"
	"strings"
)

// ForstFilesInDir lists production .ft files in a single package directory.
// Skips *_test.ft. Does not walk subdirectories.
func ForstFilesInDir(dir string) ([]string, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".ft") {
			continue
		}
		if strings.HasSuffix(name, "_test.ft") {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	return out, nil
}
