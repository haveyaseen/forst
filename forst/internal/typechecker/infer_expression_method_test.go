package typechecker

import (
	"testing"
)

// Parameter named like an imported Go package must use the local method signature
// when ensure parses the call as a MethodCallNode (not a dotted FunctionCallNode).
func TestRegression_ensureMethodCall_localShadowsImportPackage(t *testing.T) {
	t.Parallel()
	typecheckMustOK(t, `package main

import "path/filepath"

type Box = {}

func (b Box) Own(): Bool {
	return true
}

func demo(filepath Box) {
	ensure filepath.Own()
}

func main() {}
`)
}

func TestRegression_ensureMethodCall_importPackageUnshadowed(t *testing.T) {
	t.Parallel()
	typecheckMustOK(t, `package main

import "path/filepath"

func openFile(path String) {
	ensure filepath.IsAbs(path)
}

func main() {}
`)
}
