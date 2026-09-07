package typechecker

import (
	"forst/internal/ast"
)

// validateEnsureErrorFallback typechecks the ensure … else failure expression.
// Nominal error types must use struct literals (EnsureErrorExpr ShapeNode), not EnsureErrorCall.
func (tc *TypeChecker) validateEnsureErrorFallback(ensure ast.EnsureNode) error {
	if ensure.Error == nil {
		return nil
	}
	switch e := (*ensure.Error).(type) {
	case ast.EnsureErrorCall:
		if tc.IsNominalErrorType(ast.TypeIdent(e.ErrorType)) {
			def := tc.Defs[ast.TypeIdent(e.ErrorType)].(ast.TypeDefNode)
			errEx := def.Expr.(ast.TypeDefErrorExpr)
			hint := e.ErrorType + "{}"
			if len(errEx.Payload.Fields) > 0 {
				hint = e.ErrorType + "{ field: value }"
			}
			return reportBodyf(ensure.EnsureSubjectSpan(), "error-struct-syntax",
				"%s is an error type — construct it with `%s`, not `%s(...)`",
				e.ErrorType, hint, e.ErrorType)
		}
		// Real function returning Error: typecheck as a call.
		call := ast.FunctionCallNode{
			Function:  ast.Ident{ID: ast.Identifier(e.ErrorType)},
			Arguments: e.ErrorArgs,
			CallSpan:  ensure.EnsureSubjectSpan(),
		}
		_, err := tc.inferExpressionType(call)
		return err
	case ast.EnsureErrorExpr:
		_, err := tc.inferExpressionType(e.Expr)
		return err
	case ast.EnsureErrorVar:
		return nil
	default:
		return nil
	}
}
