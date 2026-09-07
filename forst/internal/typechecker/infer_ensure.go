package typechecker

import (
	"forst/internal/ast"

	"github.com/sirupsen/logrus"
)

// lookupEnsureSubjectType returns the static type of an ensure subject's place or call.
func (tc *TypeChecker) lookupEnsureSubjectType(ensure ast.EnsureNode) (ast.TypeNode, error) {
	if ensure.Subject != nil {
		types, err := tc.inferExpressionType(ensure.Subject)
		if err != nil {
			return ast.TypeNode{}, err
		}
		if len(types) != 1 {
			return ast.TypeNode{}, reportBodyf(ensure.EnsureSubjectSpan(), "ensure-call-subject-type",
				"ensure call subject must have a single type (got %d)", len(types))
		}
		tc.log.WithFields(logrus.Fields{
			"function": "lookupEnsureSubjectType",
			"subject":  ensure.Subject.String(),
			"type":     formatTypeNodeForDiag(types[0]),
		}).Debug("ensure call subject type")
		return types[0], nil
	}
	return tc.LookupVariableType(&ensure.Variable, tc.CurrentScope())
}

func (tc *TypeChecker) inferEnsureType(ensure ast.EnsureNode) (ast.TypeNode, error) {
	variableType, err := tc.lookupEnsureSubjectType(ensure)
	if err != nil {
		return ast.TypeNode{}, err
	}

	ensure, err = tc.SpecializeEnsureSugar(ensure, variableType)
	if err != nil {
		return ast.TypeNode{}, err
	}

	// Phase 2: lower ensure RHS to Assertion IR (TypeTarget stays separate).
	tc.recordEnsureIR(ensure)

	subjSpan := ensure.EnsureSubjectSpan()

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
					return ast.TypeNode{}, reportBodyf(subjSpan, "refinement-bare-guard-needs-parens",
						"refinement-bare-guard-needs-parens: guard %s requires parentheses — use `%s()` for an assertion, or a type name for a type target",
						*name, *name)
				}
				if _, isGuard := def.(*ast.TypeGuardNode); isGuard {
					return ast.TypeNode{}, reportBodyf(subjSpan, "refinement-bare-guard-needs-parens",
						"refinement-bare-guard-needs-parens: guard %s requires parentheses — use `%s()` for an assertion, or a type name for a type target",
						*name, *name)
				}
			}
		}
	}

	if err := tc.rejectTypeNameAsAssertionCall(ensure.Assertion, subjSpan); err != nil {
		return ast.TypeNode{}, err
	}

	if err := tc.validateAssertionNode(ensure.Assertion, variableType, subjSpan); err != nil {
		return ast.TypeNode{}, err
	}

	// Runtime-only atoms (e.g. Min(n)): prove via IR only; do not build a static dependent type.
	// Call subjects have no place to narrow — skip AccessPath prove.
	if ir := tc.lastEnsureIR.Assertion; HasRuntimeOnlyAtom(ir) {
		if !ensure.IsCallSubject() {
			path := tc.AccessPathForVariable(&ensure.Variable)
			if tc.predicates != nil {
				pred := tc.predicates.FromAssertion(ir)
				tc.CurrentRefinementContext().Prove(path, pred)
			}
		}
		return variableType, nil
	}

	// Assertion hover type is stored in infer.go after successor narrowing so `tc.Types` matches the
	// same inference order as `if x is <assertion>` (see applyEnsureSuccessorNarrowing).
	return variableType, nil
}
