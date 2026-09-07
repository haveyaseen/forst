package transformergo

import (
	"errors"
	"fmt"
	"strconv"

	"forst/internal/ast"
	goast "go/ast"
	gotoken "go/token"
)

var (
	errConstraintArgNeedsKind = errors.New("constraint argument must be a type, shape, or value")
	errConstraintArgValueKind = errors.New("unsupported value kind in constraint argument")
)

// transformEnsureConstraints transforms an ensure node based on its type
func (at *AssertionTransformer) transformEnsureConstraints(ensure ast.EnsureNode) (goast.Expr, error) {
	baseType, err := at.transformer.getEnsureBaseType(ensure)
	if err != nil {
		return nil, err
	}
	transformed, err := at.TransformBuiltinConstraint(baseType.Ident, ensure.EnsureSubject(), ensure.Assertion.Constraints[0])
	if err != nil {
		return nil, fmt.Errorf("failed to transform ensure conditions constraints: %w", err)
	}
	return transformed, nil
}

// transformEnsureConstraint transforms a single constraint in an ensure statement
func (t *Transformer) transformEnsureConstraint(ensure ast.EnsureNode, constraint ast.ConstraintNode, varType ast.TypeNode) (goast.Expr, error) {
	builtinSubject := unwrapAssertionTypeForBuiltin(varType)

	// Result(S,F) discriminators: same lowering as `if x is Ok() / Err()` (success/err + error split).
	// Ensure-successor narrowing may replace the subject's static type with S; use resultLocalSplit
	// (from `x := f()` with Result) when present, else varType.IsResultType().
	if ensure.Assertion.BaseType == nil && len(ensure.Assertion.Constraints) == 1 {
		c := ensure.Assertion.Constraints[0]
		if c.Name == "Ok" || c.Name == "Err" {
			if ensure.IsCallSubject() {
				return nil, fmt.Errorf("Result Ok/Err on call subject must use transformEnsureResultCallSubject")
			}
			if t.hasResultLocalSplitForSimpleVariable(ensure.Variable) || varType.IsResultType() || t.compoundVarDeclaresResultField(ensure.Variable) {
				return t.transformResultIsDiscriminator(ensure.Variable, c)
			}
		}
	}

	// Type-level shape constraints (`ensure m is { field }` → Match) are not lowered to runtime checks yet.
	// Return true (success) so type-guard lowering `if !cond { return false }` is a no-op.
	if constraint.Name == "is" || constraint.Name == "Match" {
		return goast.NewIdent("true"), nil
	}

	subjectExpr := ensure.EnsureSubject()

	// Try built-in constraints first
	if transformed, err := t.assertionTransformer.TransformBuiltinConstraint(builtinSubject.Ident, subjectExpr, constraint); err == nil {
		return transformed, nil
	}

	// Try type guards
	typeGuardDef, err := t.lookupTypeGuardNode(constraint.Name)
	if err == nil && typeGuardDef != nil && t.isTypeGuardCompatible(varType, typeGuardDef) {
		hash, err := t.TypeChecker.Hasher.HashNode(*typeGuardDef)
		if err != nil {
			return nil, fmt.Errorf("failed to hash type guard node: %w", err)
		}
		guardFuncName := hash.ToGuardIdent()
		expr, err := t.transformExpression(subjectExpr)
		if err != nil {
			return nil, fmt.Errorf("failed to transform expression: %w", err)
		}
		args := []goast.Expr{expr}
		for _, arg := range constraint.Args {
			argExpr, err := transformConstraintArg(arg)
			if err != nil {
				return nil, err
			}
			args = append(args, argExpr)
		}
		callExpr := &goast.CallExpr{
			Fun:  goast.NewIdent(string(guardFuncName)),
			Args: args,
		}
		return callExpr, nil
	}

	// Try all base types in the alias chain
	aliasChain := t.TypeChecker.GetTypeAliasChain(varType)
	t.log.WithFields(map[string]any{
		"aliasChain": aliasChain,
		"function":   "transformEnsureConstraint",
	}).Debug("Alias chain for type alias")
	for _, baseType := range aliasChain[1:] {
		t.log.WithFields(map[string]any{
			"baseType":   baseType.Ident,
			"constraint": constraint.Name,
			"function":   "transformEnsureConstraint",
		}).Debug("Trying type guard lookup for base type in alias chain")
		guard, err := t.lookupTypeGuardNode(constraint.Name)
		if err == nil && guard != nil && t.isTypeGuardCompatible(baseType, guard) {
			expr, err := t.transformExpression(subjectExpr)
			if err != nil {
				return nil, fmt.Errorf("failed to transform expression: %w", err)
			}
			hash, err := t.TypeChecker.Hasher.HashNode(*guard)
			if err != nil {
				return nil, fmt.Errorf("failed to hash type guard node: %w", err)
			}
			guardFuncName := hash.ToGuardIdent()
			callExpr := &goast.CallExpr{
				Fun:  goast.NewIdent(string(guardFuncName)),
				Args: []goast.Expr{expr},
			}
			return callExpr, nil
		}
		// Retry built-in constraint for base type
		t.log.WithFields(map[string]any{
			"baseType":   baseType.Ident,
			"constraint": constraint.Name,
			"function":   "transformEnsureConstraint",
		}).Debug("Retrying built-in constraint for base type in alias chain")
		typeIdent := ast.TypeIdent(baseType.Ident)
		if result, err := t.assertionTransformer.TransformBuiltinConstraint(typeIdent, subjectExpr, constraint); err == nil {
			return result, nil
		}
	}

	return nil, fmt.Errorf("no valid transformation found for constraint: %s", constraint.Name)
}

// unwrapAssertionTypeForBuiltin returns the underlying builtin type for constraint lowering.
func unwrapAssertionTypeForBuiltin(varType ast.TypeNode) ast.TypeNode {
	if varType.Ident == ast.TypeAssertion && varType.Assertion != nil && varType.Assertion.BaseType != nil {
		return ast.TypeNode{Ident: *varType.Assertion.BaseType}
	}
	return varType
}

// transformConstraintArg transforms a constraint argument node into a Go expression
func transformConstraintArg(arg ast.ConstraintArgumentNode) (goast.Expr, error) {
	if arg.Type != nil {
		return goast.NewIdent(string(arg.Type.Ident)), nil
	}
	if arg.Shape != nil {
		return goast.NewIdent("struct{}"), nil
	}
	if arg.Value != nil {
		switch v := (*arg.Value).(type) {
		case ast.IntLiteralNode:
			return &goast.BasicLit{Kind: gotoken.INT, Value: v.Source()}, nil
		case ast.StringLiteralNode:
			return &goast.BasicLit{Kind: gotoken.STRING, Value: strconv.Quote(v.Value)}, nil
		case ast.BoolLiteralNode:
			return goast.NewIdent(strconv.FormatBool(v.Value)), nil
		case ast.FloatLiteralNode:
			return &goast.BasicLit{Kind: gotoken.FLOAT, Value: strconv.FormatFloat(v.Value, 'f', -1, 64)}, nil
		case ast.VariableNode:
			return &goast.Ident{Name: string(v.Ident.ID)}, nil
		default:
			return nil, fmt.Errorf("%w: %T", errConstraintArgValueKind, v)
		}
	}
	return nil, errConstraintArgNeedsKind
}

// validateConstraintArgs validates the number of arguments for a constraint
func (at *AssertionTransformer) validateConstraintArgs(constraint ast.ConstraintNode, expectedArgs int) error {
	if len(constraint.Args) != expectedArgs {
		return fmt.Errorf("%s constraint requires %d argument(s)", constraint.Name, expectedArgs)
	}
	return nil
}
