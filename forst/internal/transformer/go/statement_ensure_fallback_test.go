package transformergo

import (
	"strings"
	"testing"

	"forst/internal/ast"
	goast "go/ast"
)

func TestTransformEnsureErrorFallback_callAndVar(t *testing.T) {
	t.Parallel()
	log := setupTestLogger(nil)
	tc := setupTypeChecker(log)
	tr := setupTransformer(tc, log)

	call, err := tr.transformEnsureErrorFallback(ast.EnsureErrorCall{
		ErrorType: "Bad",
		ErrorArgs: []ast.ExpressionNode{ast.StringLiteralNode{Value: "nope"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	callS := goExprString(t, call)
	if !strings.Contains(callS, `Bad("nope")`) {
		t.Fatalf("call: %s", callS)
	}

	v, err := tr.transformEnsureErrorFallback(ast.EnsureErrorVar("errVar"))
	if err != nil {
		t.Fatal(err)
	}
	if id, ok := v.(*goast.Ident); !ok || id.Name != "errVar" {
		t.Fatalf("var: %#v", v)
	}
}

func TestEnsureFailureErrorExpr_customOrGeneric(t *testing.T) {
	t.Parallel()
	log := setupTestLogger(nil)
	tr := setupTransformer(setupTypeChecker(log), log)

	var errNode ast.EnsureErrorNode = ast.EnsureErrorCall{
		ErrorType: "Bad",
		ErrorArgs: []ast.ExpressionNode{ast.StringLiteralNode{Value: "x"}},
	}
	custom, err := tr.ensureFailureErrorExpr(ast.EnsureNode{
		Assertion: ast.AssertionNode{BaseType: typeIdentPtr(string(ast.TypeInt))},
		Error:     &errNode,
	})
	if err != nil {
		t.Fatal(err)
	}
	if s := goExprString(t, custom); !strings.Contains(s, `Bad("x")`) {
		t.Fatalf("custom: %s", s)
	}

	generic, err := tr.ensureFailureErrorExpr(ast.EnsureNode{
		Variable: ast.VariableNode{Ident: ast.Ident{ID: "n"}},
		Assertion: ast.AssertionNode{
			BaseType: typeIdentPtr(string(ast.TypeInt)),
			Constraints: []ast.ConstraintNode{{
				Name: "GreaterThan",
				Args: []ast.ConstraintArgumentNode{{Value: intLiteralNodePtr(1)}},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if s := goExprString(t, generic); !strings.Contains(s, `errors.New`) || !strings.Contains(s, `ensure n is Int.GreaterThan(1): want > 1`) {
		t.Fatalf("generic: %s", s)
	}
}

func TestTransformEnsureErrorFallback_nominalErrorStructLiteral(t *testing.T) {
	t.Parallel()
	log := setupTestLogger(nil)
	tc := setupTypeChecker(log)
	tc.Defs["NotOk"] = ast.TypeDefNode{
		Ident: "NotOk",
		Expr: ast.TypeDefErrorExpr{
			Payload: ast.ShapeNode{
				Fields: map[string]ast.ShapeFieldNode{
					"msg": {Type: &ast.TypeNode{Ident: ast.TypeString}},
				},
			},
		},
	}
	tr := setupTransformer(tc, log)
	bt := ast.TypeIdent("NotOk")
	got, err := tr.transformEnsureErrorFallback(ast.EnsureErrorExpr{
		Expr: ast.ShapeNode{
			BaseType: &bt,
			Fields: map[string]ast.ShapeFieldNode{
				"msg": {Node: ast.StringLiteralNode{Value: "bad"}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := goExprString(t, got)
	if !strings.Contains(s, `NotOk{`) || !strings.Contains(s, `"bad"`) {
		t.Fatalf("nominal fallback: %s", s)
	}
}

func intLiteralNodePtr(v int64) *ast.ValueNode {
	var n ast.ValueNode = ast.IntLiteralNode{Value: v}
	return &n
}
