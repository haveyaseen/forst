package transformergo

import (
	"fmt"
	"forst/internal/ast"
	"forst/internal/typechecker"
	goast "go/ast"
	"go/token"
)

// transformEnsureCondition transforms an ensure node into Go statements.
// Prefers the shared Assertion IR (Any/All/Atom) so fail-closed and assertion `or`
// cannot drift from the typechecker. Type-target membership emit is phase 3.
func (t *Transformer) transformEnsureCondition(ensure *ast.EnsureNode) ([]goast.Stmt, error) {
	t.logAssertionBaseType(ensure)

	varType, err := t.TypeChecker.LookupVariableType(&ensure.Variable, t.currentScope())
	if err != nil {
		return nil, fmt.Errorf("failed to lookup variable type: %w", err)
	}

	result, handled, err := t.handleTypeTargetMembership(ensure, varType)
	if err != nil || handled {
		return result, err
	}

	result, handled, err = t.handleTypeGuardCall(ensure, varType)
	if err != nil {
		return nil, err
	}
	if handled {
		if len(result) == 0 {
			return nil, fmt.Errorf("type guard ensure produced no condition to emit")
		}
		return result, nil
	}

	result, handled, err = t.handleAssertionIR(ensure, varType)
	if err != nil || handled {
		return result, err
	}

	// Fallback: iterate chains, emit join of ORs over chain-constraint meets
	return t.handleMeetChains(ensure, varType)
}

// logAssertionBaseType logs the BaseType for easier reading.
func (t *Transformer) logAssertionBaseType(ensure *ast.EnsureNode) {
	if ensure.Assertion.BaseType == nil {
		t.log.Debugf("[transformEnsureCondition] Assertion.BaseType is nil for assertion: %v", ensure.Assertion)
	} else {
		t.log.Debugf("[transformEnsureCondition] Assertion.BaseType: %v", *ensure.Assertion.BaseType)
	}
}

// handleTypeTargetMembership handles type-target membership: literal-union helpers
// and constrained scalar aliases (expand typedef Meet chain into assertion emit).
func (t *Transformer) handleTypeTargetMembership(ensure *ast.EnsureNode, varType ast.TypeNode) ([]goast.Stmt, bool, error) {
	_, typeTarget := typechecker.LowerRefinementTarget(ensure.Target, ensure.Assertion)
	if typeTarget == nil {
		return nil, false, nil
	}

	if members, ok := t.TypeChecker.LiteralUnionMembers(ast.TypeNode{
		Ident:    typeTarget.Name,
		TypeKind: ast.TypeKindUserDefined,
	}); ok && len(members) > 0 {
		expr, err := t.transformExpression(ensure.Variable)
		if err != nil {
			return nil, true, fmt.Errorf("failed to transform type-target subject: %w", err)
		}
		callExpr := &goast.CallExpr{
			Fun:  goast.NewIdent(literalUnionMembershipFuncName(string(typeTarget.Name))),
			Args: []goast.Expr{expr},
		}
		return []goast.Stmt{&goast.ExprStmt{X: callExpr}}, true, nil
	}

	if assertion, ok := t.TypeChecker.ConstrainedScalarAliasAssertion(typeTarget.Name); ok && assertion != nil {
		t.log.Debugf("[transformEnsureCondition] TypeTarget %s — expanding constrained scalar alias", typeTarget.Name)
		ir := typechecker.LowerAssertionNode(*assertion)
		if ir == nil {
			return nil, true, fmt.Errorf("constrained scalar alias %s lowered to empty assertion", typeTarget.Name)
		}
		expr, err := t.transformEnsureAssertionIR(*ensure, ir, varType)
		if err != nil {
			return nil, true, err
		}
		if expr == nil {
			return nil, true, fmt.Errorf("constrained scalar alias %s produced no ensure condition", typeTarget.Name)
		}
		return []goast.Stmt{&goast.ExprStmt{X: expr}}, true, nil
	}

	t.log.Debugf("[transformEnsureCondition] TypeTarget %s — no literal-union or constrained-alias membership helper", typeTarget.Name)
	return nil, false, nil
}

// handleTypeGuardCall handles the case of a bare type guard (BaseType but no constraints).
func (t *Transformer) handleTypeGuardCall(ensure *ast.EnsureNode, varType ast.TypeNode) ([]goast.Stmt, bool, error) {
	chains := ensure.Assertion.MeetChains()
	if len(chains) == 1 && chains[0].BaseType != nil && len(chains[0].Constraints) == 0 {
		t.log.Debugf("[transformEnsureCondition] Assertion has BaseType but no constraints - treating as type guard call")
		typeGuardName := string(*chains[0].BaseType)
		t.log.Debugf("[transformEnsureCondition] Looking up type guard: %s", typeGuardName)
		typeGuardDef, err := t.lookupTypeGuardNode(typeGuardName)
		if err != nil {
			t.log.Debugf("[transformEnsureCondition] Type guard lookup failed: %v", err)
			return nil, false, nil // fall through to other ensure emit paths
		}
		if typeGuardDef == nil {
			t.log.Debugf("[transformEnsureCondition] Type guard not found: %s", typeGuardName)
			return nil, false, nil
		}

		t.log.Debugf("[transformEnsureCondition] Type guard found: %s", typeGuardName)
		compatible := t.isTypeGuardCompatible(varType, typeGuardDef)
		t.log.Debugf("[transformEnsureCondition] Type guard compatibility check: %v", compatible)
		if compatible {
			hash, err := t.TypeChecker.Hasher.HashNode(*typeGuardDef)
			if err != nil {
				return nil, true, fmt.Errorf("failed to hash type guard node: %w", err)
			}
			guardFuncName := hash.ToGuardIdent()
			expr, err := t.transformExpression(ensure.Variable)
			if err != nil {
				return nil, true, fmt.Errorf("failed to transform expression: %w", err)
			}
			callExpr := &goast.CallExpr{
				Fun:  goast.NewIdent(string(guardFuncName)),
				Args: []goast.Expr{expr},
			}
			return []goast.Stmt{&goast.ExprStmt{X: callExpr}}, true, nil
		}
		// Incompatible with this guard — try other ensure lowering paths.
		return nil, false, nil
	}
	return nil, false, nil
}

// handleAssertionIR emits from Assertion IR using transformEnsureAssertionIR.
func (t *Transformer) handleAssertionIR(ensure *ast.EnsureNode, varType ast.TypeNode) ([]goast.Stmt, bool, error) {
	ir, _ := typechecker.LowerRefinementTarget(ensure.Target, ensure.Assertion)
	if ir != nil {
		expr, err := t.transformEnsureAssertionIR(*ensure, ir, varType)
		if err != nil {
			return nil, true, err
		}
		if expr != nil {
			return []goast.Stmt{&goast.ExprStmt{X: expr}}, true, nil
		}
	}
	return nil, false, nil
}

// handleMeetChains emits OR-join of per-chain meets from transformEnsureMeetChain.
func (t *Transformer) handleMeetChains(ensure *ast.EnsureNode, varType ast.TypeNode) ([]goast.Stmt, error) {
	chains := ensure.Assertion.MeetChains()
	var chainExprs []goast.Expr
	for _, chain := range chains {
		meet, err := t.transformEnsureMeetChain(*ensure, chain, varType)
		if err != nil {
			return nil, err
		}
		if meet != nil {
			chainExprs = append(chainExprs, meet)
		}
	}
	if len(chainExprs) == 0 {
		t.log.Debugf("[transformEnsureCondition] no constraint expressions")
		return nil, nil
	}
	joined := chainExprs[0]
	for i := 1; i < len(chainExprs); i++ {
		joined = &goast.BinaryExpr{X: joined, Op: token.LOR, Y: chainExprs[i]}
	}
	return []goast.Stmt{&goast.ExprStmt{X: joined}}, nil
}

// transformEnsureAssertionIR emits Go bool exprs from shared Assertion IR (Any→||, All→&&).
func (t *Transformer) transformEnsureAssertionIR(ensure ast.EnsureNode, ir typechecker.Assertion, varType ast.TypeNode) (goast.Expr, error) {
	switch v := ir.(type) {
	case typechecker.Atom:
		chain := ast.AssertionNode{
			BaseType:    v.BaseType,
			Constraints: []ast.ConstraintNode{{Name: v.Name, Args: v.Args}},
		}
		return t.transformEnsureMeetChain(ensure, chain, varType)
	case typechecker.Any:
		var parts []goast.Expr
		for _, c := range v.Children {
			e, err := t.transformEnsureAssertionIR(ensure, c, varType)
			if err != nil {
				return nil, err
			}
			if e != nil {
				parts = append(parts, e)
			}
		}
		if len(parts) == 0 {
			return nil, nil
		}
		out := parts[0]
		for i := 1; i < len(parts); i++ {
			out = &goast.BinaryExpr{X: out, Op: token.LOR, Y: parts[i]}
		}
		return out, nil
	case typechecker.All:
		var parts []goast.Expr
		for _, c := range v.Children {
			e, err := t.transformEnsureAssertionIR(ensure, c, varType)
			if err != nil {
				return nil, err
			}
			if e != nil {
				parts = append(parts, e)
			}
		}
		if len(parts) == 0 {
			return nil, nil
		}
		out := parts[0]
		for i := 1; i < len(parts); i++ {
			out = &goast.BinaryExpr{X: out, Op: token.LAND, Y: parts[i]}
		}
		return out, nil
	default:
		return nil, nil
	}
}

// transformEnsureMeetChain lowers one Meet chain to a single Go bool (AND of constraints).
func (t *Transformer) transformEnsureMeetChain(ensure ast.EnsureNode, chain ast.AssertionNode, varType ast.TypeNode) (goast.Expr, error) {
	if len(chain.Constraints) == 0 {
		return nil, nil
	}
	var parts []goast.Expr
	for _, constraint := range chain.Constraints {
		transformed, err := t.transformEnsureConstraint(ensure, constraint, varType)
		if err != nil {
			return nil, fmt.Errorf("failed to transform constraint: %w", err)
		}
		parts = append(parts, transformed)
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out = &goast.BinaryExpr{X: out, Op: token.LAND, Y: parts[i]}
	}
	return out, nil
}
