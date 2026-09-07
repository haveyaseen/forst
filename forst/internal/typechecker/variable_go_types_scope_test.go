package typechecker

import (
	"strings"
	"testing"

	"forst/internal/ast"
	"forst/internal/lexer"
	"forst/internal/parser"

	"github.com/sirupsen/logrus"
)

func TestCheckTypes_testingTParamDoesNotStealStringLocalNamedT(t *testing.T) {
	t.Parallel()
	dir := moduleRootFromWD(t)

	srcTestFirst := `package main

import "testing"

func TestDemo(t *testing.T) {
	t.Helper()
}

func main() {
	s := "hi"
	t := s
	println(t[1:])
}
`
	srcMainFirst := `package main

import "testing"

func main() {
	s := "hi"
	t := s
	println(t[1:])
}

func TestDemo(t *testing.T) {
	t.Helper()
}
`

	for _, tc := range []struct {
		name string
		src  string
	}{
		{name: "Test_before_main", src: srcTestFirst},
		{name: "main_before_Test", src: srcMainFirst},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			log := logrus.New()
			log.SetLevel(logrus.PanicLevel)
			toks := lexer.New([]byte(tc.src), "t.ft", log).Lex()
			nodes, err := parser.New(toks, "t.ft", log).ParseFile()
			if err != nil {
				t.Fatal(err)
			}
			checker := New(log, false)
			checker.GoWorkspaceDir = dir
			if err := checker.CheckTypes(nodes); err != nil {
				t.Fatalf("CheckTypes: %v", err)
			}

			mainT := findSliceTargetVar(t, nodes, "main", "t")
			types, ok := checker.InferredTypesForVariableNode(mainT)
			if !ok || len(types) != 1 || types[0].Ident != ast.TypeString {
				t.Fatalf("main local t occurrence type: got %v ok=%v, want String", types, ok)
			}
			if gt := checker.GoTypeForVariableNode(mainT); gt != nil {
				t.Fatalf("main local t must have no Go FFI type, got %v", gt)
			}
			hover := checker.FormatVariableOccurrenceTypeForHover(mainT, types)
			if hover != "String" {
				t.Fatalf("main local t hover: got %q, want String", hover)
			}
			if strings.Contains(hover, "testing.T") {
				t.Fatalf("main local t hover must not mention testing.T: %q", hover)
			}

			testT := findTestParamVar(t, nodes, "TestDemo", "t")
			if gt := checker.GoTypeForVariableNode(testT); !IsGoTypesTestingT(gt) {
				t.Fatalf("TestDemo param t Go type: got %v, want *testing.T", gt)
			}
		})
	}
}

func findFunc(t *testing.T, nodes []ast.Node, name string) ast.FunctionNode {
	t.Helper()
	for _, n := range nodes {
		fn, ok := n.(ast.FunctionNode)
		if ok && string(fn.Ident.ID) == name {
			return fn
		}
	}
	t.Fatalf("function %s not found", name)
	return ast.FunctionNode{}
}

func findTestParamVar(t *testing.T, nodes []ast.Node, fnName, ident string) ast.VariableNode {
	t.Helper()
	fn := findFunc(t, nodes, fnName)
	for _, p := range fn.Params {
		sp, ok := p.(ast.SimpleParamNode)
		if !ok || string(sp.Ident.ID) != ident {
			continue
		}
		return ast.VariableNode{Ident: sp.Ident}
	}
	t.Fatalf("param %s not found in %s", ident, fnName)
	return ast.VariableNode{}
}

func findSliceTargetVar(t *testing.T, nodes []ast.Node, fnName, ident string) ast.VariableNode {
	t.Helper()
	fn := findFunc(t, nodes, fnName)
	var found *ast.VariableNode
	var walk func(n ast.Node)
	walk = func(n ast.Node) {
		if found != nil || n == nil {
			return
		}
		switch e := n.(type) {
		case ast.SliceExpressionNode:
			if vn, ok := e.Target.(ast.VariableNode); ok && string(vn.Ident.ID) == ident {
				v := vn
				found = &v
				return
			}
			walk(e.Target)
		case ast.FunctionCallNode:
			for _, a := range e.Arguments {
				walk(a)
			}
		case ast.MethodCallNode:
			walk(e.Receiver)
			for _, a := range e.Arguments {
				walk(a)
			}
		case ast.AssignmentNode:
			for _, lv := range e.LValues {
				walk(lv)
			}
			for _, rv := range e.RValues {
				walk(rv)
			}
		}
	}
	for _, stmt := range fn.Body {
		walk(stmt)
	}
	if found == nil {
		t.Fatalf("slice target %s not found in %s", ident, fnName)
	}
	return *found
}
