package transformergo

import (
	"strings"
	"testing"

	"forst/internal/ast"
)

func TestCollectResultErrSlotUsed_ensureAndIfDiscriminators(t *testing.T) {
	t.Parallel()
	varName := "x"

	t.Run("ensure", func(t *testing.T) {
		t.Parallel()
		body := []ast.Node{
			ast.EnsureNode{Variable: ast.VariableNode{Ident: ast.Ident{ID: "x"}}},
		}
		if !collectResultErrSlotUsed(body, varName) {
			t.Fatal("expected ensure on x to use error slot")
		}
	})

	t.Run("ifOkDiscriminator", func(t *testing.T) {
		t.Parallel()
		body := []ast.Node{
			ast.IfNode{
				Condition: ast.BinaryExpressionNode{
					Left:     ast.VariableNode{Ident: ast.Ident{ID: "x"}},
					Operator: ast.TokenIs,
					Right: ast.AssertionNode{
						Constraints: []ast.ConstraintNode{{Name: "Ok"}},
					},
				},
			},
		}
		if !collectResultErrSlotUsed(body, varName) {
			t.Fatal("expected if x is Ok() to use error slot")
		}
	})

	t.Run("unused", func(t *testing.T) {
		t.Parallel()
		body := []ast.Node{
			ast.FunctionCallNode{
				Function:  ast.Ident{ID: "println"},
				Arguments: []ast.ExpressionNode{ast.IntLiteralNode{Value: 1}},
			},
		}
		if collectResultErrSlotUsed(body, varName) {
			t.Fatal("expected no error slot use when x is never referenced")
		}
	})
}

func TestTransformResultSplitAssignment_unusedErrAndSuccess_goBuilds(t *testing.T) {
	t.Parallel()
	src := `package main

func f(): Result(Int, Error) {
	return 1
}

func main() {
	x := f()
	println(1)
}
`
	out := compileForstPipelineExt(t, src, pipelineOpts{goWorkspaceDir: moduleRootFromWD(t)})
	if !strings.Contains(out, "_, _ = f()") && !strings.Contains(out, "_, _ := f()") {
		t.Fatalf("expected blank bindings when Result local is unused, got:\n%s", out)
	}
	assertGoBuildsInTempModule(t, out)
}

func TestTransformResultSplitAssignment_fieldAccessAfterEnsure_goBuilds(t *testing.T) {
	t.Parallel()
	src := `package main

type Payload = { id: String }

func create(): Result(Payload, Error) {
	return Payload{id: "x"}
}

func checkout() {
	result := create()
	ensure result is Ok()
	return result.id
}

func main() {
	s := checkout()
	ensure s is Ok()
	println(s)
}
`
	out := compileForstPipelineExt(t, src, pipelineOpts{goWorkspaceDir: moduleRootFromWD(t)})
	if strings.Contains(out, "_, resultErr := create()") {
		t.Fatalf("expected result bound when result.id is returned, got:\n%s", out)
	}
	if !strings.Contains(out, "result.Id") && !strings.Contains(out, "result.id") {
		t.Fatalf("expected field access on result, got:\n%s", out)
	}
	assertGoBuildsInTempModule(t, out)
}

func TestTransformResultSplitAssignment_ifIsOk_keepsErrSlot(t *testing.T) {
	t.Parallel()
	src := `package main

func f(): Result(Int, Error) {
	return 1
}

func main() {
	x := f()
	if x is Ok() {
		println(x)
	}
}
`
	out := compileForstPipelineExt(t, src, pipelineOpts{goWorkspaceDir: moduleRootFromWD(t)})
	if !strings.Contains(out, "xErr") {
		t.Fatalf("expected xErr when if x is Ok() checks error slot, got:\n%s", out)
	}
	if strings.Contains(out, "_, xErr := f()") {
		t.Fatalf("expected success slot bound when x used in if body, got:\n%s", out)
	}
	assertGoBuildsInTempModule(t, out)
}

func TestTransformResultSplitAssignment_successUsedInIfPredicate_goBuilds(t *testing.T) {
	t.Parallel()
	src := `package main

func f(): Result(Int, Error) {
	return 1
}

func main() {
	x := f()
	if x is Ok() {
		println(0)
	}
}
`
	out := compileForstPipelineExt(t, src, pipelineOpts{goWorkspaceDir: moduleRootFromWD(t)})
	if !strings.Contains(out, "xErr") {
		t.Fatalf("expected xErr binding for if x is Ok(), got:\n%s", out)
	}
	assertGoBuildsInTempModule(t, out)
}
