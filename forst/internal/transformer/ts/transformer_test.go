package transformerts

import (
	"errors"
	"strings"
	"testing"

	"forst/internal/ast"
	"forst/internal/parser"
	"forst/internal/typechecker"

	"github.com/sirupsen/logrus"
)

func TestTransformForstFileToTypeScript_echoShapeAndFunction(t *testing.T) {
	const src = `package main

type EchoRequest = {
	message: String
}

func Echo(input EchoRequest) {
	return {
		echo: input.message,
		timestamp: 1234567890,
	}
}
`
	logger := ast.SetupTestLogger(nil)
	p := parser.NewTestParser(src, logger)
	nodes, err := p.ParseFile()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	tc := typechecker.New(nil, false)
	if err := tc.CheckTypes(nodes); err != nil {
		t.Fatalf("typecheck: %v", err)
	}

	tr := New(tc, logger)
	out, err := tr.TransformForstFileToTypeScript(nodes, "")
	if err != nil {
		t.Fatalf("transform: %v", err)
	}

	typesFile := out.GenerateTypesFile()
	for _, fragment := range []string{
		"export interface $EchoRequest",
	} {
		if !strings.Contains(typesFile, fragment) {
			t.Fatalf("types file missing %q:\n%s", fragment, typesFile)
		}
	}
	if strings.Contains(typesFile, "export function Echo(") {
		t.Fatalf("types must be shapes only; function signatures live on package modules, not types.d.ts:\n%s", typesFile)
	}

	clientFile := out.GenerateClientFile()
	for _, fragment := range []string{
		"invokeFunction",
		"'main'",
		"Echo",
		"import type {",
		"$EchoRequest",
		"from '../types.js'",
	} {
		if !strings.Contains(clientFile, fragment) {
			t.Fatalf("client file missing %q:\n%s", fragment, clientFile)
		}
	}
	if strings.Contains(clientFile, "export interface $EchoRequest") {
		t.Fatalf("client file should not duplicate types.d.ts interfaces; got:\n%s", clientFile)
	}
}

func TestTransformForstFileToTypeScript_omitsNominalErrorsFromTypesFile(t *testing.T) {
	const src = `package main

error EmptyMessage { message: String }

type EchoRequest = { message: String }

func Echo(input EchoRequest): Result(EchoResponse, Error) {
	ensure input.message is Min(1) else EmptyMessage{ message: "empty" }
	return { echo: input.message, timestamp: 0 }
}

type EchoResponse = { echo: String, timestamp: Int }
`
	logger := ast.SetupTestLogger(nil)
	p := parser.NewTestParser(src, logger)
	nodes, err := p.ParseFile()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tc := typechecker.New(nil, false)
	if err := tc.CheckTypes(nodes); err != nil {
		t.Fatalf("typecheck: %v", err)
	}
	tr := New(tc, logger)
	out, err := tr.TransformForstFileToTypeScript(nodes, "")
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	typesFile := out.GenerateTypesFile()
	if strings.Contains(typesFile, "export interface EmptyMessage") {
		t.Fatalf("nominal errors belong in errors.js, not types.d.ts:\n%s", typesFile)
	}
	found := false
	for _, c := range out.DomainErrors {
		if c.Name == "EmptyMessage" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected EmptyMessage in DomainErrors")
	}
	for _, name := range out.ExportedTypeNames {
		if name == "EmptyMessage" {
			t.Fatalf("EmptyMessage must not be in ExportedTypeNames: %v", out.ExportedTypeNames)
		}
	}
}

func TestTransformForstFileToTypeScript_setsPackageName(t *testing.T) {
	const src = `package sidecartest

func Ping() {
	return 1
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
	if out.PackageName != "sidecartest" {
		t.Fatalf("PackageName: got %q, want sidecartest", out.PackageName)
	}
	if !strings.Contains(out.GenerateClientFile(), "'sidecartest'") {
		t.Fatalf("expected client to use package sidecartest:\n%s", out.GenerateClientFile())
	}
}

func TestTransformForstFileToTypeScript_namespaceExportUsesPackageName(t *testing.T) {
	const src = `package main

func F() { return 1 }
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
	out, err := tr.TransformForstFileToTypeScript(nodes, "api")
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	client := out.GenerateClientFile()
	if !strings.Contains(client, "export const $main") {
		t.Fatalf("want $-prefixed package namespace export, got:\n%s", client)
	}
	if !strings.Contains(client, "'main'") {
		t.Fatalf("invokeFunction should still use Forst package main, got:\n%s", client)
	}
}

func TestNew_nilLoggerUsesDefault(t *testing.T) {
	tc := typechecker.New(nil, false)
	tr := New(tc, nil)
	if tr.log == nil {
		t.Fatal("expected non-nil logger")
	}
	// Exercise one call path that uses the logger without failing.
	tr.log.SetLevel(logrus.ErrorLevel)
}

func TestTransformForstFileToTypeScript_arrayMapAndPointerInShape(t *testing.T) {
	const src = `package main

type Row = {
	names: []String
	scores: map[String]Int
	next: *String
}

func Count() {
	return 0
}
`
	logger := ast.SetupTestLogger(nil)
	p := parser.NewTestParser(src, logger)
	nodes, err := p.ParseFile()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	tc := typechecker.New(nil, false)
	if err := tc.CheckTypes(nodes); err != nil {
		t.Fatalf("typecheck: %v", err)
	}

	tr := New(tc, logger)
	out, err := tr.TransformForstFileToTypeScript(nodes, "")
	if err != nil {
		t.Fatalf("transform: %v", err)
	}

	typesFile := out.GenerateTypesFile()
	for _, frag := range []string{
		"string[]",
		"Record<string, number>",
		"(string) | null",
	} {
		if !strings.Contains(typesFile, frag) {
			t.Fatalf("types file missing %q:\n%s", frag, typesFile)
		}
	}
}

func TestTransformForstFileToTypeScript_nestedShapeFields(t *testing.T) {
	const src = `package main

type Outer = {
	meta: { version: Int, label: String }
}

func Touch() {
	return 0
}
`
	logger := ast.SetupTestLogger(nil)
	p := parser.NewTestParser(src, logger)
	nodes, err := p.ParseFile()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	tc := typechecker.New(nil, false)
	if err := tc.CheckTypes(nodes); err != nil {
		t.Fatalf("typecheck: %v", err)
	}

	tr := New(tc, logger)
	out, err := tr.TransformForstFileToTypeScript(nodes, "")
	if err != nil {
		t.Fatalf("transform: %v", err)
	}

	typesFile := out.GenerateTypesFile()
	if !strings.Contains(typesFile, "export interface $Outer") {
		t.Fatalf("missing Outer interface:\n%s", typesFile)
	}
	if !strings.Contains(typesFile, "label: string") || !strings.Contains(typesFile, "version: number") {
		t.Fatalf("expected nested meta fields in types file:\n%s", typesFile)
	}
}

func TestGenerateClientStructure_skipsClientWhenPackageNameEmpty(t *testing.T) {
	logger := ast.SetupTestLogger(nil)
	tc := typechecker.New(logger, false)
	tr := New(tc, logger)
	tr.Output.Types = []string{"export interface X { x: number }"}
	tr.Output.Functions = []FunctionSignature{{Name: "F", ReturnType: "number"}}
	tr.Output.PackageName = ""
	tr.generateClientStructure()
	if tr.Output.GenerateClientFile() != "" {
		t.Fatalf("expected no per-package client, got %q", tr.Output.GenerateClientFile())
	}
	if tr.Output.GenerateMainClient() != "" {
		t.Fatal("expected no main client")
	}
	if tr.Output.GenerateTypesFile() == "" {
		t.Fatal("types file should still be generated")
	}
}

func TestTransformForstFileToTypeScript_clientInvoke_zeroAndTwoParams(t *testing.T) {
	const src = `package main

func Zero() {
	return 0
}

func Two(a Int, b Int) {
	return a + b
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
	client := out.GenerateClientFile()
	if !strings.Contains(client, "Zero") || !strings.Contains(client, "invokeFunction") {
		t.Fatalf("client:\n%s", client)
	}
	if !strings.Contains(client, "'main', 'Zero', []") {
		t.Fatalf("want zero-arg invoke with empty array, got:\n%s", client)
	}
	if !strings.Contains(client, "[a, b]") {
		t.Fatalf("want two-arg invoke with [a, b], got:\n%s", client)
	}
}

func TestTransformForstFileToTypeScript_debugLogsFireAtDebugLevel(t *testing.T) {
	logger := ast.SetupTestLogger(&ast.TestLoggerOptions{ForceLevel: logrus.DebugLevel})
	const src = `package main

type EchoRequest = { message: String }

func Echo(input EchoRequest) {
	return { echo: input.message, timestamp: 0 }
}
`
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
	if _, err := tr.TransformForstFileToTypeScript(nodes, ""); err != nil {
		t.Fatalf("transform: %v", err)
	}
}

// oneShotErrMapper wraps *TypeMapping; first GetTypeScriptType returns errOnce then clears.
type oneShotErrMapper struct {
	*TypeMapping
	errOnce error
}

func (m *oneShotErrMapper) GetTypeScriptType(forstType *ast.TypeNode) (string, error) {
	if m.errOnce != nil {
		err := m.errOnce
		m.errOnce = nil
		return "", err
	}
	return m.TypeMapping.GetTypeScriptType(forstType)
}

func TestTransformForstFileToTypeScript_propagatesTransformFunctionMappingError(t *testing.T) {
	// No type defs so Defs loop does not call GetTypeScriptType before transformFunction.
	const src = `package main

func F(x Int) {
	return 0
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
	base, ok := tr.typeMapping.(*TypeMapping)
	if !ok {
		t.Fatal("expected *TypeMapping from New")
	}
	injected := errors.New("ts mapping error")
	tr.typeMapping = &oneShotErrMapper{TypeMapping: base, errOnce: injected}
	_, err = tr.TransformForstFileToTypeScript(nodes, "")
	if err == nil {
		t.Fatal("expected error from GetTypeScriptType")
	}
	if !strings.Contains(err.Error(), "F") {
		t.Fatalf("want function name in error: %v", err)
	}
	if !strings.Contains(err.Error(), "ts mapping error") {
		t.Fatalf("want wrapped mapping error: %v", err)
	}
}

func TestTransformForstFileToTypeScript_typeDefTransformError(t *testing.T) {
	const src = `package main

func Ok() {
	return 0
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
	tc.Defs[ast.TypeIdent("Bad")] = ast.TypeDefNode{
		Ident: ast.TypeIdent("Bad"),
		Expr:  ast.TypeDefAssertionExpr{Assertion: nil},
	}
	tr := New(tc, logger)
	_, err = tr.TransformForstFileToTypeScript(nodes, "")
	if err == nil || !strings.Contains(err.Error(), "Bad") {
		t.Fatalf("expected error from bad typedef, got %v", err)
	}
}
