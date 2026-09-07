package typechecker

import (
	"fmt"
	"forst/internal/ast"
)

func (tc *TypeChecker) inferEnsureNode(node ast.Node) ([]ast.TypeNode, error) {
	ensureNode, ok := node.(ast.EnsureNode)
	if !ok {
		return nil, fmt.Errorf("inferEnsureNode: unexpected node type %T", node)
	}

	inferredVarType, err := tc.inferEnsureType(ensureNode)
	if err != nil {
		return nil, err
	}

	// Re-specialize for narrowing / Ok discriminators (inferEnsureType specializes a copy).
	variableType, err := tc.LookupVariableType(&ensureNode.Variable, tc.CurrentScope())
	if err != nil {
		return nil, err
	}
	ensureNode, err = tc.SpecializeEnsureSugar(ensureNode, variableType)
	if err != nil {
		return nil, err
	}
	if ensureIsOnlyNilConstraint(ensureNode) && variableType.IsResultType() &&
		len(variableType.TypeParams) >= 1 && variableType.TypeParams[0].Ident == ast.TypeVoid {
		okAssert := ast.ConstraintOnlyAssertion("Ok")
		ensureNode.Assertion = okAssert
		ensureNode.Target = ast.AssertionTarget{Chains: []ast.AssertionNode{okAssert}}
	}

	if ensureNode.Block != nil {
		tc.pushScope(ensureNode.Block)
		if _, err := tc.inferExpressionType(ensureNode.Variable); err != nil {
			return nil, err
		}
		if tc.ensureUsesBuiltinResultOkErrDiscriminator(ensureNode) {
			tc.applyEnsureBlockResultFailureNarrowing(ensureNode)
		} else {
			tc.applyEnsureSuccessorNarrowing(ensureNode)
		}
		_, err = tc.inferNodeTypes(ensureNode.Block.Body, ensureNode.Block)
		tc.popScope()
		if err != nil {
			return nil, err
		}
		if tc.ensureUsesBuiltinResultOkErrDiscriminator(ensureNode) {
			tc.applyEnsureSuccessorNarrowing(ensureNode)
		} else if _, isTT := ensureNode.Target.(ast.TypeTarget); isTT {
			tc.applyEnsureSuccessorNarrowing(ensureNode)
		} else if p, ok := ensureNode.Target.(*ast.TypeTarget); ok && p != nil {
			tc.applyEnsureSuccessorNarrowing(ensureNode)
		}
	} else {
		if _, err := tc.inferExpressionType(ensureNode.Variable); err != nil {
			return nil, err
		}
		tc.applyEnsureSuccessorNarrowing(ensureNode)
	}

	tc.storeInferredType(ensureNode.Assertion, []ast.TypeNode{inferredVarType})
	return nil, nil
}
