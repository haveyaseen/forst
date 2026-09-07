package typechecker

import (
	"forst/internal/ast"
)

func (tc *TypeChecker) inferEnsureType(ensure ast.EnsureNode) (ast.TypeNode, error) {
	variableType, err := tc.LookupVariableType(&ensure.Variable, tc.CurrentScope())
	if err != nil {
		return ast.TypeNode{}, err
	}

	ensure, err = tc.SpecializeEnsureSugar(ensure, variableType)
	if err != nil {
		return ast.TypeNode{}, err
	}

	// Phase 2: lower ensure RHS to Assertion IR (TypeTarget stays separate).
	tc.recordEnsureIR(ensure)

	if _, isType := ensure.Target.(ast.TypeTarget); isType {
		if err := tc.validateEnsureTypeTarget(ensure, variableType); err != nil {
			return ast.TypeNode{}, err
		}
		return variableType, nil
	}
	if p, ok := ensure.Target.(*ast.TypeTarget); ok && p != nil {
		if err := tc.validateEnsureTypeTarget(ensure, variableType); err != nil {
			return ast.TypeNode{}, err
		}
		return variableType, nil
	}

	// TypeTarget that names a type guard must use assertion parens: Strong() not Strong.
	if ensure.Assertion.IsBareTypeShape() {
		name := ensure.Assertion.BaseType
		if name != nil {
			if def, ok := tc.Defs[*name]; ok {
				if _, isGuard := def.(ast.TypeGuardNode); isGuard {
					return ast.TypeNode{}, reportBodyf(ensure.Variable.Ident.Span, "refinement-bare-guard-needs-parens",
						"refinement-bare-guard-needs-parens: guard %s requires parentheses — use `%s()` for an assertion, or a type name for a type target",
						*name, *name)
				}
				if _, isGuard := def.(*ast.TypeGuardNode); isGuard {
					return ast.TypeNode{}, reportBodyf(ensure.Variable.Ident.Span, "refinement-bare-guard-needs-parens",
						"refinement-bare-guard-needs-parens: guard %s requires parentheses — use `%s()` for an assertion, or a type name for a type target",
						*name, *name)
				}
			}
		}
	}

	if err := tc.rejectTypeNameAsAssertionCall(ensure.Assertion, ensure.Variable.Ident.Span); err != nil {
		return ast.TypeNode{}, err
	}

	// Result(Void) may use Nil() as Error-shaped sugar (same as ensure !err → Ok).
	if ensureIsOnlyNilConstraint(ensure) && variableType.IsResultType() &&
		len(variableType.TypeParams) >= 1 && variableType.TypeParams[0].Ident == ast.TypeVoid {
		okAssert := ast.ConstraintOnlyAssertion("Ok")
		ensure.Assertion = okAssert
		ensure.Target = ast.AssertionTarget{Chains: []ast.AssertionNode{okAssert}}
		tc.recordEnsureIR(ensure)
	}

	if err := tc.validateAssertionNode(ensure.Assertion, variableType, ensure.Variable.Ident.Span); err != nil {
		return ast.TypeNode{}, err
	}

	// Runtime-only atoms (e.g. Min(n)): prove via IR only; do not build a static dependent type.
	if ir := tc.lastEnsureIR.Assertion; HasRuntimeOnlyAtom(ir) {
		path := tc.AccessPathForVariable(&ensure.Variable)
		if tc.predicates != nil {
			pred := tc.predicates.FromAssertion(ir)
			tc.CurrentRefinementContext().Prove(path, pred)
		}
		return variableType, nil
	}

	// Assertion hover type is stored in infer.go after successor narrowing so `tc.Types` matches the
	// same inference order as `if x is <assertion>` (see applyEnsureSuccessorNarrowing).
	return variableType, nil
}

func ensureIsOnlyNilConstraint(n ast.EnsureNode) bool {
	if len(n.Assertion.Constraints) != 1 {
		return false
	}
	c := n.Assertion.Constraints[0]
	return c.Name == "Nil" && len(c.Args) == 0
}
