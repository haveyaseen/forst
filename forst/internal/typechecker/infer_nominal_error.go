package typechecker

import (
	"forst/internal/ast"
)

// inferNominalErrorConstructorCall rejects call-shaped constructors on error types.
// Nominal errors must use Go-style struct literals: N{} or N{ field: value }.
func (tc *TypeChecker) inferNominalErrorConstructorCall(e ast.FunctionCallNode, argTypes [][]ast.TypeNode) ([]ast.TypeNode, bool, error) {
	_ = argTypes
	def, ok := tc.Defs[ast.TypeIdent(e.Function.ID)].(ast.TypeDefNode)
	if !ok {
		return nil, false, nil
	}
	errEx, ok := def.Expr.(ast.TypeDefErrorExpr)
	if !ok {
		return nil, false, nil
	}
	sp := e.CallSpan
	if !sp.IsSet() {
		sp = e.Function.Span
	}
	hint := string(e.Function.ID) + "{}"
	if len(errEx.Payload.Fields) > 0 {
		hint = string(e.Function.ID) + "{ field: value }"
	}
	return nil, true, reportBodyf(sp, "error-struct-syntax",
		"%s is an error type — construct it with `%s`, not `%s(...)`",
		e.Function.ID, hint, e.Function.ID)
}

// validateNominalErrorStructLiteral checks that a typed composite literal matches the
// declared payload of a nominal error type (E{message: "bad"} / TooFast{}).
func (tc *TypeChecker) validateNominalErrorStructLiteral(shape ast.ShapeNode) error {
	if shape.BaseType == nil {
		return nil
	}
	def, ok := tc.Defs[*shape.BaseType].(ast.TypeDefNode)
	if !ok {
		return nil
	}
	errEx, ok := def.Expr.(ast.TypeDefErrorExpr)
	if !ok {
		return nil
	}
	payload := errEx.Payload
	if len(shape.Fields) != len(payload.Fields) {
		return reportBodyf(shape.Span, "error-payload",
			"%s payload does not match %s", formatTypeIdentForDiag(def.Ident), formatTypeIdentForDiag(def.Ident))
	}
	for name, pf := range payload.Fields {
		sf, ok := shape.Fields[name]
		if !ok {
			return reportBodyf(shape.Span, "error-payload",
				"%s missing field %s", formatTypeIdentForDiag(def.Ident), name)
		}
		var want *ast.TypeNode
		if pf.Type != nil {
			t := *pf.Type
			want = &t
		}
		got, err := tc.inferErrorLiteralFieldType(sf, want)
		if err != nil {
			return err
		}
		if want != nil && !tc.IsTypeCompatible(got, *want) {
			return reportBodyf(shape.Span, "error-payload",
				"%s field %s: got %s, want %s",
				formatTypeIdentForDiag(def.Ident), name,
				formatTypeNodeForDiag(got), formatTypeNodeForDiag(*want))
		}
	}
	return nil
}

func (tc *TypeChecker) inferErrorLiteralFieldType(field ast.ShapeFieldNode, expected *ast.TypeNode) (ast.TypeNode, error) {
	// Prefer expression typing. The parser sets Type=variable-name for VariableNode fields
	// (legacy), which must not win over looking up the variable's actual type.
	if expr, ok := shapeFieldValueExpr(field); ok {
		if expected != nil {
			types, err := tc.inferExpressionTypeWithExpected(expr, expected)
			if err == nil && len(types) > 0 {
				return types[0], nil
			}
		}
		types, err := tc.inferExpressionType(expr)
		if err != nil {
			return ast.TypeNode{}, err
		}
		if len(types) == 0 {
			return ast.TypeNode{}, reportBodyf(ast.FakeSpan(), "error-payload", "no type for error field expression")
		}
		return types[0], nil
	}
	if field.Shape != nil {
		return tc.inferShapeType(*field.Shape, expected)
	}
	if field.Type != nil {
		return *field.Type, nil
	}
	if field.Assertion != nil {
		types, err := tc.InferAssertionType(field.Assertion, false, "", expected)
		if err != nil {
			return ast.TypeNode{}, err
		}
		if len(types) == 0 {
			return ast.TypeNode{}, reportBodyf(ast.FakeSpan(), "error-payload", "no type for error field")
		}
		return types[0], nil
	}
	return ast.TypeNode{}, reportBodyf(ast.FakeSpan(), "error-payload", "error field has no value")
}

// shapeFieldValueExpr extracts the RHS expression from a shape literal field.
func shapeFieldValueExpr(field ast.ShapeFieldNode) (ast.ExpressionNode, bool) {
	if field.Node != nil {
		if expr, ok := field.Node.(ast.ExpressionNode); ok {
			return expr, true
		}
	}
	a := field.Assertion
	if a == nil || a.BaseType != nil || len(a.Constraints) != 1 {
		return nil, false
	}
	c := a.Constraints[0]
	if c.Name != ast.ValueConstraint || len(c.Args) != 1 || c.Args[0].Value == nil {
		return nil, false
	}
	expr, ok := (*c.Args[0].Value).(ast.ExpressionNode)
	return expr, ok
}
