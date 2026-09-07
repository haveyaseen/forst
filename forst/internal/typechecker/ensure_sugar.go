package typechecker

import (
	"forst/internal/ast"
)

// SpecializeEnsureSugar fills Assertion/Target for bare and bang ensure sugar from the subject type.
// Returns a copy of ensure with Implicit cleared when specialization succeeds.
func (tc *TypeChecker) SpecializeEnsureSugar(ensure ast.EnsureNode, subjectType ast.TypeNode) (ast.EnsureNode, error) {
	if ensure.Implicit == ast.EnsureImplicitNone {
		return ensure, nil
	}

	base := subjectType
	if chain := tc.GetTypeAliasChain(subjectType); len(chain) > 0 {
		base = chain[len(chain)-1]
	}

	var name string
	switch ensure.Implicit {
	case ast.EnsureImplicitBare:
		switch {
		case base.Ident == ast.TypeBool:
			name = "True"
		case base.IsResultType():
			name = "Ok"
		default:
			return ensure, reportBodyf(ensure.Variable.Ident.Span, "ensure-bare-subject",
				"ensure without `is` needs a Bool or Result subject (got %s)",
				formatTypeNodeForDiag(subjectType))
		}
	case ast.EnsureImplicitBang:
		switch {
		case base.Ident == ast.TypeBool:
			name = "False"
		case base.IsResultType():
			if len(base.TypeParams) >= 1 && base.TypeParams[0].Ident == ast.TypeVoid {
				name = "Ok"
			} else {
				return ensure, reportBodyf(ensure.Variable.Ident.Span, "ensure-bang-result",
					"ensure ! on Result(%s, …) is ambiguous — write `ensure %s is Ok()` or `ensure %s is Err()`",
					formatTypeNodeForDiag(base.TypeParams[0]), ensure.Variable.Ident.ID, ensure.Variable.Ident.ID)
			}
		case isNilableType(tc, subjectType):
			name = "Nil"
		default:
			return ensure, reportBodyf(ensure.Variable.Ident.Span, "ensure-bang-subject",
				"ensure ! needs a Bool, Error/nilable, or Result(Void, …) subject (got %s)",
				formatTypeNodeForDiag(subjectType))
		}
	default:
		return ensure, nil
	}

	assertion := ast.ConstraintOnlyAssertion(name)
	if name == "Nil" {
		errType := ast.TypeError
		assertion.BaseType = &errType
	}
	ensure.Assertion = assertion
	ensure.Target = ast.AssertionTarget{Chains: []ast.AssertionNode{assertion}}
	ensure.Implicit = ast.EnsureImplicitNone
	return ensure, nil
}

// ensureIsOkDiscriminator reports `ensure x is Ok()` (after sugar specialization).
func ensureIsOkDiscriminator(n ast.EnsureNode) bool {
	if n.Assertion.BaseType != nil || len(n.Assertion.Constraints) != 1 {
		return false
	}
	return n.Assertion.Constraints[0].Name == "Ok"
}

// ensureIsErrDiscriminator reports `ensure x is Err()`.
func ensureIsErrDiscriminator(n ast.EnsureNode) bool {
	if n.Assertion.BaseType != nil || len(n.Assertion.Constraints) != 1 {
		return false
	}
	return n.Assertion.Constraints[0].Name == "Err"
}

func isMainFunctionNode(fn ast.FunctionNode) bool {
	return fn.Ident.ID == "main"
}
