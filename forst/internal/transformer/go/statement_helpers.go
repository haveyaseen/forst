package transformergo

import (
	"fmt"

	"forst/internal/ast"
	"forst/internal/typechecker"
	goast "go/ast"
)

// enclosingReturnTypes returns the Forst return types of the closest function or function literal.
func (t *Transformer) enclosingReturnTypes(fnNode ast.Node) ([]ast.TypeNode, string, error) {
	switch fn := fnNode.(type) {
	case ast.FunctionNode:
		name := string(fn.Ident.ID)
		if retTypes, err := t.TypeChecker.LookupFunctionReturnType(&fn); err == nil && !typechecker.IsVoidReturnTypes(retTypes) {
			return retTypes, name, nil
		}
		if !typechecker.IsVoidReturnTypes(fn.ReturnTypes) {
			return fn.ReturnTypes, name, nil
		}
		return nil, name, nil
	case *ast.FunctionNode:
		if fn == nil {
			return nil, "", fmt.Errorf("enclosing FunctionNode is nil")
		}
		return t.enclosingReturnTypes(*fn)
	case ast.FunctionLiteralNode:
		if typechecker.IsVoidReturnTypes(fn.ReturnTypes) {
			return nil, "_lit", nil
		}
		return fn.ReturnTypes, "_lit", nil
	case *ast.FunctionLiteralNode:
		if fn == nil {
			return nil, "", fmt.Errorf("enclosing FunctionLiteralNode is nil")
		}
		return t.enclosingReturnTypes(*fn)
	default:
		return nil, "", fmt.Errorf("enclosing node is not a FunctionNode or FunctionLiteralNode: %T", fnNode)
	}
}

// transformEnsureErrorFallback lowers `ensure … else Bad("msg")` / `else errVar` / method calls.
func (t *Transformer) transformEnsureErrorFallback(errorNode ast.EnsureErrorNode) (goast.Expr, error) {
	switch e := errorNode.(type) {
	case ast.EnsureErrorCall:
		if len(e.ErrorArgs) == 1 {
			if def, ok := t.TypeChecker.Defs[ast.TypeIdent(e.ErrorType)].(ast.TypeDefNode); ok {
				if _, ok := def.Expr.(ast.TypeDefErrorExpr); ok {
					if shape, ok := e.ErrorArgs[0].(ast.ShapeNode); ok {
						expected := &ast.TypeNode{Ident: def.Ident}
						return t.transformShapeNodeWithExpectedType(&shape, expected, nil)
					}
				}
			}
		}
		args := make([]goast.Expr, len(e.ErrorArgs))
		for i, arg := range e.ErrorArgs {
			ex, err := t.transformExpression(arg)
			if err != nil {
				return nil, fmt.Errorf("ensure error fallback arg %d: %w", i, err)
			}
			args[i] = ex
		}
		return &goast.CallExpr{
			Fun:  goast.NewIdent(e.ErrorType),
			Args: args,
		}, nil
	case ast.EnsureErrorVar:
		return goast.NewIdent(string(e)), nil
	case ast.EnsureErrorExpr:
		return t.transformExpression(e.Expr)
	default:
		return nil, fmt.Errorf("unsupported ensure error fallback: %T", errorNode)
	}
}

func (t *Transformer) defaultAssertionErrorExpr(stmt ast.EnsureNode) goast.Expr {
	t.Output.EnsureImport("errors")
	subjectLabel, assertionLabel, wantHint := t.ensureFailureMessage(stmt)
	msg := fmt.Sprintf("ensure %s is %s: want %s", subjectLabel, assertionLabel, wantHint)
	return &goast.CallExpr{
		Fun: &goast.SelectorExpr{
			X:   goast.NewIdent("errors"),
			Sel: goast.NewIdent("New"),
		},
		Args: []goast.Expr{
			goQuotedStringLit(msg),
		},
	}
}

// ensureFailureErrorExpr returns the Go expression used on ensure failure (custom or generic).
func (t *Transformer) ensureFailureErrorExpr(stmt ast.EnsureNode) (goast.Expr, error) {
	if stmt.Error != nil {
		return t.transformEnsureErrorFallback(*stmt.Error)
	}
	if expr, ok := t.ensurePropagatedFailureExpr(stmt); ok {
		return expr, nil
	}
	return t.defaultAssertionErrorExpr(stmt), nil
}

// ensurePropagatedFailureExpr returns the subject's error value for bare propagate sugar:
// `ensure !err` / `ensure err is Nil()` on Error, and `ensure x is Ok()` on a Result.
func (t *Transformer) ensurePropagatedFailureExpr(stmt ast.EnsureNode) (goast.Expr, bool) {
	if stmt.Error != nil {
		return nil, false
	}
	// Call-subject Result Ok: Init binds `err` from the call (see transformEnsureResultCallSubject).
	if stmt.IsCallSubject() && ensureIsOnlyOkAssertion(stmt) {
		return goast.NewIdent("err"), true
	}
	variableType, err := t.TypeChecker.LookupVariableType(&stmt.Variable, t.currentScope())
	if err != nil {
		return nil, false
	}

	// Result Ok() → error slot (xErr or the Void-Result binding itself).
	if ensureIsOnlyOkAssertion(stmt) {
		if t.resultLocalSplit != nil {
			if split, ok := t.resultLocalSplit[string(stmt.Variable.Ident.ID)]; ok && split.errGoName != "" {
				return goast.NewIdent(split.errGoName), true
			}
		}
		if variableType.IsResultType() && len(variableType.TypeParams) >= 1 &&
			variableType.TypeParams[0].Ident == ast.TypeVoid {
			// Result(Void) lowers to a single error binding named like the Forst local.
			return goast.NewIdent(string(stmt.Variable.Ident.ID)), true
		}
		return nil, false
	}

	if !ensureIsOnlyNilAssertion(stmt) {
		return nil, false
	}
	if !t.TypeChecker.IsTypeCompatible(variableType, ast.TypeNode{Ident: ast.TypeError}) {
		return nil, false
	}
	expr, err := t.transformExpression(stmt.Variable)
	if err != nil {
		return nil, false
	}
	return expr, true
}

func ensureIsOnlyOkAssertion(stmt ast.EnsureNode) bool {
	if stmt.Assertion.BaseType != nil || len(stmt.Assertion.Constraints) != 1 {
		return false
	}
	c := stmt.Assertion.Constraints[0]
	return c.Name == "Ok" && len(c.Args) == 0
}

func ensureIsOnlyNilAssertion(stmt ast.EnsureNode) bool {
	target, ok := stmt.Target.(ast.AssertionTarget)
	if !ok {
		if ptr, ptrOK := stmt.Target.(*ast.AssertionTarget); ptrOK && ptr != nil {
			target, ok = *ptr, true
		}
	}
	if !ok || len(target.Chains) != 1 {
		// Fall back to Assertion when Target unset (specialized sugar).
		if len(stmt.Assertion.Constraints) == 1 && stmt.Assertion.Constraints[0].Name == "Nil" {
			return len(stmt.Assertion.Constraints[0].Args) == 0
		}
		return false
	}
	chain := target.Chains[0]
	return len(chain.Constraints) == 1 && chain.Constraints[0].Name == "Nil" && len(chain.Constraints[0].Args) == 0
}

// specializeEnsureForEmit resolves bare/bang ensure sugar using the subject type.
func (t *Transformer) specializeEnsureForEmit(ensure ast.EnsureNode) (ast.EnsureNode, error) {
	variableType, err := t.lookupEnsureSubjectTypeForEmit(ensure)
	if err != nil {
		variableType = ast.TypeNode{}
	}
	// After Ok() narrowing, LookupVariableType may be the success payload (e.g. Void).
	// Prefer Result(Void) when this local is a folded void-Result error binding.
	if !ensure.IsCallSubject() && t.isVoidResultErrorBinding(ensure.Variable) {
		variableType = ast.NewResultType(
			ast.TypeNode{Ident: ast.TypeVoid},
			ast.TypeNode{Ident: ast.TypeError},
		)
	}

	if ensure.Implicit == ast.EnsureImplicitNone && len(ensure.Assertion.Constraints) > 0 {
		return ensure, nil
	}
	if ensure.Implicit == ast.EnsureImplicitNone {
		return ensure, nil
	}
	return t.TypeChecker.SpecializeEnsureSugar(ensure, variableType)
}

// lookupEnsureSubjectTypeForEmit resolves the ensure subject type for codegen.
func (t *Transformer) lookupEnsureSubjectTypeForEmit(ensure ast.EnsureNode) (ast.TypeNode, error) {
	if ensure.IsCallSubject() {
		ts, err := t.TypeChecker.LookupInferredType(ensure.Subject, false)
		if err != nil {
			return ast.TypeNode{}, err
		}
		if len(ts) != 1 {
			return ast.TypeNode{}, fmt.Errorf("ensure call subject: expected 1 type, got %d", len(ts))
		}
		return ts[0], nil
	}
	return t.TypeChecker.LookupVariableType(&ensure.Variable, t.currentScope())
}

func (t *Transformer) isVoidResultErrorBinding(vn ast.VariableNode) bool {
	if isDotQualifiedVariable(vn) || t.resultLocalSplit == nil {
		return false
	}
	split, ok := t.resultLocalSplit[string(vn.Ident.ID)]
	if !ok || split.errGoName == "" {
		return false
	}
	// Void Result assign uses errGoName == Forst name and no success slots.
	return split.errGoName == string(vn.Ident.ID) && len(split.successGoNames) == 0
}

// getAssertionStringForError returns a properly qualified assertion string for error messages
func (t *Transformer) getAssertionStringForError(assertion *ast.AssertionNode) string {
	// If BaseType is set, use it with the constraint name
	if assertion.BaseType != nil {
		return assertion.ToString(assertion.BaseType)
	}

	// Otherwise, try to get the inferred type from the typechecker
	if inferredType, err := t.TypeChecker.LookupInferredType(assertion, false); err == nil && len(inferredType) > 0 {
		// Use the most specific non-hash-based alias for error messages
		nonHash := t.TypeChecker.GetMostSpecificNonHashAlias(inferredType[0])
		return assertion.ToString(&nonHash.Ident)
	}

	// Fallback to the original string representation
	return assertion.String()
}

// findBestNamedTypeForReturnType tries to find a named type that matches the given hash-based type
func (t *Transformer) findBestNamedTypeForReturnType(hashType ast.TypeNode) string {
	// If it's already a named type (not hash-based), return it
	if !hashType.IsHashBased() {
		return string(hashType.Ident)
	}

	// Look through all named types to find one that's compatible
	for typeIdent, def := range t.TypeChecker.Defs {
		if _, ok := def.(ast.TypeDefNode); ok {
			// Skip hash-based types
			if string(typeIdent)[:2] == "T_" {
				continue
			}

			// Check if this named type is compatible with the hash-based type
			if t.TypeChecker.IsTypeCompatible(hashType, ast.TypeNode{Ident: typeIdent}) {
				return string(typeIdent)
			}
		}
	}

	return ""
}

// findBestNamedTypeForReturnStructLiteral finds the best named type for a struct literal in a return statement.
// When expectedType is a user-named type whose shape matches the literal, that name wins over other structural matches.
func (t *Transformer) findBestNamedTypeForReturnStructLiteral(shapeNode ast.ShapeNode, expectedType *ast.TypeNode) *ast.TypeNode {
	// Prefer the expected name whenever it resolves to a matching Defs entry.
	// Do not require TypeKindUserDefined: callers sometimes pass Ident-only TypeNodes.
	if expectedType != nil && !expectedType.IsHashBased() && !expectedType.IsGoBuiltin() {
		if t.TypeChecker.UserNamedTypeMatchesShape(expectedType.Ident, shapeNode) {
			return &ast.TypeNode{Ident: expectedType.Ident, TypeKind: ast.TypeKindUserDefined}
		}
	}

	if namedType := t.TypeChecker.FindAnyStructurallyIdenticalNamedType(shapeNode); namedType != "" {
		return &ast.TypeNode{Ident: namedType}
	}

	// If no structurally identical named type found, use the expected type
	if expectedType != nil {
		return expectedType
	}

	return nil
}

// getShapeNode extracts a *ast.ShapeNode from an ast.Node, handling both value and pointer types
func getShapeNode(value ast.Node) (*ast.ShapeNode, bool) {
	if sn, ok := value.(*ast.ShapeNode); ok {
		return sn, true
	}
	if snVal, ok := value.(ast.ShapeNode); ok {
		return &snVal, true
	}
	return nil, false
}
