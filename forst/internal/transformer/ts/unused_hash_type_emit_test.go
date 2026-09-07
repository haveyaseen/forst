package transformerts

import (
	"strings"
	"testing"

	"forst/internal/ast"
	"forst/internal/parser"
	"forst/internal/typechecker"
)

func TestTransformForstFileToTypeScript_omitsUnusedEmptyHashType(t *testing.T) {
	t.Parallel()
	const src = `package main

func okInt() {
	n := 42
	ensure n is GreaterThan(0)
	return n
}

func main() {
	x := okInt()
	ensure x is Ok()
	println(x)
}
`
	logger := ast.SetupTestLogger(nil)
	p := parser.NewTestParser(src, logger)
	nodes, err := p.ParseFile()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tc := typechecker.New(logger, false)
	if err := tc.CheckTypes(nodes); err != nil {
		t.Fatalf("typecheck: %v", err)
	}
	tr := New(tc, logger)
	out, err := tr.TransformForstFileToTypeScript(nodes, "")
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	typesFile := out.GenerateTypesFile()
	if strings.Contains(typesFile, "$T_H4c2uQ34ZJV") || strings.Contains(typesFile, "export interface $T_") {
		t.Fatalf("unused empty hash type must not appear in types.d.ts:\n%s", typesFile)
	}
	for _, name := range out.ExportedTypeNames {
		if strings.HasPrefix(name, "$T_") {
			t.Fatalf("ExportedTypeNames must not include unused hash type %q", name)
		}
	}
}

func TestTransformForstFileToTypeScript_namedShapeStillEmitted(t *testing.T) {
	t.Parallel()
	const src = `package main

type EchoRequest = {
	message: String
}

func Echo(input EchoRequest) {
	return {
		echo: input.message,
	}
}
`
	logger := ast.SetupTestLogger(nil)
	p := parser.NewTestParser(src, logger)
	nodes, err := p.ParseFile()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tc := typechecker.New(logger, false)
	if err := tc.CheckTypes(nodes); err != nil {
		t.Fatalf("typecheck: %v", err)
	}
	tr := New(tc, logger)
	out, err := tr.TransformForstFileToTypeScript(nodes, "")
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	typesFile := out.GenerateTypesFile()
	if !strings.Contains(typesFile, "export interface $EchoRequest") {
		t.Fatalf("named shape must still be emitted:\n%s", typesFile)
	}
}

func TestTransformForstFileToTypeScript_referencedHashTypeEmitted(t *testing.T) {
	t.Parallel()
	const src = `package main

func MakePoint() {
	return { x: 1, y: 2 }
}
`
	logger := ast.SetupTestLogger(nil)
	p := parser.NewTestParser(src, logger)
	nodes, err := p.ParseFile()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tc := typechecker.New(logger, false)
	if err := tc.CheckTypes(nodes); err != nil {
		t.Fatalf("typecheck: %v", err)
	}
	tr := New(tc, logger)
	out, err := tr.TransformForstFileToTypeScript(nodes, "")
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	if len(out.Functions) != 1 {
		t.Fatalf("expected one function, got %d (omitted=%v)", len(out.Functions), out.OmittedFunctions)
	}
	ret := out.Functions[0].ReturnType
	if !strings.HasPrefix(ret, "$T_") {
		t.Logf("return type is %q (not a bare hash export); skip emit assertion", ret)
		return
	}
	typesFile := out.GenerateTypesFile()
	if !strings.Contains(typesFile, "export interface "+ret) {
		t.Fatalf("hash return type %s must be emitted in types file:\n%s", ret, typesFile)
	}
}
