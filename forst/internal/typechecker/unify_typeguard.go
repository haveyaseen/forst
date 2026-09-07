package typechecker

import (
	"fmt"
	"forst/internal/ast"

	logrus "github.com/sirupsen/logrus"
)

// typeIdentIsNominalError reports whether id names a user `error Name { ... }` type.
func (tc *TypeChecker) typeIdentIsNominalError(id ast.TypeIdent) bool {
	def, ok := tc.Defs[id].(ast.TypeDefNode)
	if !ok {
		return false
	}
	switch def.Expr.(type) {
	case ast.TypeDefErrorExpr, *ast.TypeDefErrorExpr:
		return true
	default:
		return false
	}
}

// rejectNominalErrorAsBareIsGuard rejects `x is ParseError`-style guards: nominal errors must be
// discriminated via Result and `Err()` / `Err(...)` (built-in Result guards), not as the RHS base type.
func (tc *TypeChecker) rejectNominalErrorAsBareIsGuard(assertionNode *ast.AssertionNode, span ast.SourceSpan) error {
	if assertionNode == nil || assertionNode.BaseType == nil {
		return nil
	}
	if tc.typeIdentIsNominalError(*assertionNode.BaseType) {
		return reportf(span, "nominal-error-not-is-guard",
			fmt.Sprintf("%s is not an `is` guard", *assertionNode.BaseType),
			fmt.Sprintf("Nominal error types are not used as `is %s`. Narrow Result failures with Err instead.", *assertionNode.BaseType),
			fmt.Sprintf("write `if r is Err(%s)` or `ensure r is Ok()`", *assertionNode.BaseType))
	}
	return nil
}

// validateTypeDefAssertion validates a TypeDefAssertionExpr against the left-hand side type
func (tc *TypeChecker) validateTypeDefAssertion(assertionNode *ast.AssertionNode, varLeftType ast.TypeNode, span ast.SourceSpan) error {
	if assertionNode == nil {
		return fmt.Errorf("right-hand side of 'is' must be an assertion")
	}
	if err := tc.rejectNominalErrorAsBareIsGuard(assertionNode, span); err != nil {
		return err
	}

	// Check that the assertion's base type matches the left-hand side type or is a subtype
	if assertionNode.BaseType != nil {
		baseType := ast.TypeNode{Ident: *assertionNode.BaseType}
		if !tc.IsTypeCompatible(varLeftType, baseType) {
			return fmt.Errorf("assertion base type %s is not compatible with left-hand side type %s", formatTypeIdentForDiag(baseType.Ident), formatTypeIdentForDiag(varLeftType.Ident))
		}
	}

	// Process type guard constraints
	for _, constraint := range assertionNode.Constraints {
		if guardDef, exists := tc.Defs[ast.TypeIdent(constraint.Name)]; exists {
			if guardNode, ok := guardDef.(ast.TypeGuardNode); ok {
				// Check that the leftmost variable's type matches the guard's subject type
				subjectType := guardNode.Subject.GetType()
				if !tc.IsTypeCompatible(varLeftType, subjectType) {
					return fmt.Errorf("type guard '%s' requires subject type %s, but got %s",
						constraint.Name, subjectType.Ident, varLeftType.Ident)
				}
			}
		}
	}
	return nil
}

// processTypeGuardFields processes type guard constraints and adds their fields to the shape
func (tc *TypeChecker) processTypeGuardFields(shapeNode *ast.ShapeNode, assertionNode *ast.AssertionNode) {
	if assertionNode == nil {
		return
	}
	tc.log.WithFields(logrus.Fields{
		"function":         "processTypeGuardFields",
		"constraintsCount": len(assertionNode.Constraints),
	}).Tracef("Processing type guard application")
	// Add fields from type guards to the right-hand shape
	for _, constraint := range assertionNode.Constraints {
		tc.log.WithFields(logrus.Fields{
			"function":   "processTypeGuardFields",
			"constraint": constraint.Name,
		}).Tracef("Processing type guard constraint")
		// Look up the type guard definition
		if guardDef, exists := tc.Defs[ast.TypeIdent(constraint.Name)]; exists {
			tc.log.WithFields(logrus.Fields{
				"function":   "processTypeGuardFields",
				"constraint": constraint.Name,
			}).Tracef("Found type guard definition")
			if guardNode, ok := guardDef.(ast.TypeGuardNode); ok {
				tc.log.WithFields(logrus.Fields{
					"function":    "processTypeGuardFields",
					"constraint":  constraint.Name,
					"paramsCount": len(guardNode.Params),
				}).Tracef("Found type guard node")

				// Get the parameter name and type from the type guard
				if len(guardNode.Params) > 0 && len(constraint.Args) > 0 {
					param := guardNode.Params[0]
					paramName := param.GetIdent()
					// Use the actual argument type from the constraint application
					argType := constraint.Args[0]
					// Add the new field to the right-hand shape
					if argType.Type != nil {
						shapeNode.Fields[paramName] = ast.ShapeFieldNode{
							Assertion: &ast.AssertionNode{
								BaseType: &argType.Type.Ident,
							},
						}
						tc.log.WithFields(logrus.Fields{
							"function":   "processTypeGuardFields",
							"constraint": constraint.Name,
							"paramName":  paramName,
							"argType":    argType.Type.Ident,
						}).Tracef("Added field to result shape")
					} else {
						tc.log.WithFields(logrus.Fields{
							"function":   "processTypeGuardFields",
							"constraint": constraint.Name,
							"paramName":  paramName,
						}).Errorf("Constraint argument for field has no Type")
					}
				} else {
					tc.log.WithFields(logrus.Fields{
						"function":    "processTypeGuardFields",
						"constraint":  constraint.Name,
						"paramsCount": len(guardNode.Params),
						"argsCount":   len(constraint.Args),
					}).Tracef("Type guard has insufficient params/args")
				}
			} else {
				tc.log.WithFields(logrus.Fields{
					"function":   "processTypeGuardFields",
					"constraint": constraint.Name,
					"guardDef":   guardDef,
				}).Tracef("Definition is not a TypeGuardNode")
			}
		} else {
			tc.log.WithFields(logrus.Fields{
				"function":   "processTypeGuardFields",
				"constraint": constraint.Name,
			}).Tracef("No definition found for type guard")
		}
	}
}

// validateAssertionNode validates a direct assertion node
func (tc *TypeChecker) validateAssertionNode(assertionNode ast.AssertionNode, varLeftType ast.TypeNode, span ast.SourceSpan) error {
	if err := tc.rejectNominalErrorAsBareIsGuard(&assertionNode, span); err != nil {
		return err
	}
	if len(assertionNode.OrChains) > 0 {
		if err := tc.validateAssertionOrChains(assertionNode, varLeftType, span); err != nil {
			return err
		}
	}
	if len(assertionNode.Constraints) == 1 && assertionNode.BaseType == nil {
		c := assertionNode.Constraints[0]
		if c.Name == "Ok" || c.Name == "Err" {
			if varLeftType.IsResultType() {
				return tc.validateResultDiscriminatorAssertion(assertionNode, varLeftType, span)
			}
			// Otherwise only valid if a user-defined type guard uses this name (e.g. `is (v N) Ok()`).
			if _, hasGuard := tc.Defs[ast.TypeIdent(c.Name)]; !hasGuard {
				return resultOkSubjectError(c.Name, varLeftType.String(), span)
			}
		}
	}
	for _, constraint := range assertionNode.Constraints {
		if constraint.Name != ConstraintMatch &&
			!isBuiltinAssertionConstraintName(constraint.Name) &&
			!tc.IsTypeGuardConstraint(constraint.Name) {
			return guardUndefinedError(constraint.Name, span)
		}
		if constraint.Name == "Present" {
			if !isPresentableNilable(tc, varLeftType) {
				return fmt.Errorf("present assertion requires a pointer, map, or array type, got %s", formatTypeIdentForDiag(varLeftType.Ident))
			}
		} else if constraint.Name == "Nil" {
			if varLeftType.IsResultType() {
				return reportBodyf(span, "ensure-nil-result",
					"Nil() is for Error and other nilables — write `ensure` / `is Ok()` on a Result (got %s)",
					formatTypeNodeForDiag(varLeftType))
			}
			if !isNilableType(tc, varLeftType) {
				return fmt.Errorf("nil assertion requires a pointer, map, array, or Error type, got %s", formatTypeIdentForDiag(varLeftType.Ident))
			}
		} else {
			// Check type guard subject type for other constraints
			if guardDef, exists := tc.Defs[ast.TypeIdent(constraint.Name)]; exists {
				if guardNode, ok := guardDef.(ast.TypeGuardNode); ok {
					subjectType := guardNode.Subject.GetType()
					if !tc.IsTypeCompatible(varLeftType, subjectType) {
						return fmt.Errorf("type guard '%s' requires subject type %s, but got %s",
							constraint.Name, subjectType.Ident, varLeftType.Ident)
					}
				}
			}
		}
	}
	return nil
}

// validateAssertionOrChains checks Join alternatives; error constructors after `or` need `else`.
func (tc *TypeChecker) validateAssertionOrChains(assertion ast.AssertionNode, varLeftType ast.TypeNode, span ast.SourceSpan) error {
	for _, chain := range assertion.OrChains {
		if tc.assertionChainLooksLikeErrorConstructor(chain) {
			return reportBodyf(span, "refinement-legacy-failure-or",
				"refinement-legacy-failure-or: `or` starts another constraint chain; use `else` for typed failure")
		}
		if err := tc.validateAssertionNode(chain, varLeftType, span); err != nil {
			return err
		}
	}
	return nil
}

func (tc *TypeChecker) assertionChainLooksLikeErrorConstructor(chain ast.AssertionNode) bool {
	if chain.BaseType != nil || len(chain.Constraints) != 1 {
		return false
	}
	name := chain.Constraints[0].Name
	if name == "Error" {
		return true
	}
	def, ok := tc.Defs[ast.TypeIdent(name)]
	if !ok {
		return false
	}
	td, ok := def.(ast.TypeDefNode)
	if !ok {
		return false
	}
	_, isErr := td.Expr.(ast.TypeDefErrorExpr)
	return isErr
}

// validateResultDiscriminatorAssertion validates `x is Ok(...)` / `Err(...)` when the subject is Result(S,F).
func (tc *TypeChecker) validateResultDiscriminatorAssertion(a ast.AssertionNode, varLeftType ast.TypeNode, span ast.SourceSpan) error {
	if !varLeftType.IsResultType() || len(varLeftType.TypeParams) < 2 {
		c := "Ok"
		if len(a.Constraints) == 1 && a.Constraints[0].Name == "Err" {
			c = "Err"
		}
		return resultOkSubjectError(c, varLeftType.String(), span)
	}
	if a.BaseType != nil {
		return reportf(span, "result-ok-no-base", "Ok/Err cannot follow a base type",
			"Write the discriminator alone (with optional payload type), not as a chain on another type.",
			"use `is Ok()` or `is Ok(User)`, not `is Something.Ok()`")
	}
	if len(a.Constraints) != 1 {
		return reportf(span, "result-ok-arity", "Ok/Err takes at most one type argument",
			"Result discriminators are a single constraint.",
			"write `is Ok()`, `is Ok(T)`, `is Err()`, or `is Err(E)`")
	}
	c := a.Constraints[0]
	succ := varLeftType.TypeParams[0]
	fail := varLeftType.TypeParams[1]
	switch c.Name {
	case "Ok":
		switch len(c.Args) {
		case 0:
			return nil
		case 1:
			if c.Args[0].Value == nil {
				return reportf(span, "result-ok-arity", "Ok(...) needs a value argument",
					"`Ok(...)` with parentheses expects a value (or use bare `Ok()`).",
					"write `is Ok()` or `is Ok(value)`")
			}
			vt, err := tc.inferExpressionType(*c.Args[0].Value)
			if err != nil {
				return err
			}
			if len(vt) != 1 {
				return reportf(span, "result-ok-arity", "Ok(...) needs a single value",
					"The Ok payload argument must have exactly one type.",
					"pass one value: `is Ok(value)`")
			}
			if !tc.IsTypeCompatible(vt[0], succ) {
				return reportf(span, "result-ok-payload", "Ok(...) value does not match success type",
					fmt.Sprintf("Expected success type `%s`, got `%s`.", succ.String(), vt[0].String()),
					"pass a value compatible with the Result success type")
			}
			return nil
		default:
			return reportf(span, "result-ok-arity", "Ok(...) expects at most one argument",
				"`Ok` takes zero or one argument.",
				"write `is Ok()` or `is Ok(value)`")
		}
	case "Err":
		switch len(c.Args) {
		case 0:
			return nil
		case 1:
			arg := c.Args[0]
			if arg.Type != nil {
				if !tc.IsTypeCompatible(*arg.Type, fail) {
					return reportf(span, "result-err-payload", "Err(...) type does not match failure type",
						fmt.Sprintf("Expected failure type `%s`, got `%s`.", fail.String(), arg.Type.String()),
						"pass a type or value compatible with the Result failure type")
				}
				return nil
			}
			if arg.Value == nil {
				return reportf(span, "result-ok-arity", "Err(...) needs a value or type argument",
					"`Err(...)` with parentheses expects a failure value or type (or use bare `Err()`).",
					"write `is Err()` or `is Err(ParseError)`")
			}
			vt, err := tc.inferExpressionType(*arg.Value)
			if err != nil {
				return err
			}
			if len(vt) != 1 {
				return reportf(span, "result-ok-arity", "Err(...) needs a single value",
					"The Err payload argument must have exactly one type.",
					"pass one value: `is Err(err)`")
			}
			if !tc.IsTypeCompatible(vt[0], fail) {
				return reportf(span, "result-err-payload", "Err(...) value does not match failure type",
					fmt.Sprintf("Expected failure type `%s`, got `%s`.", fail.String(), vt[0].String()),
					"pass a value compatible with the Result failure type")
			}
			return nil
		default:
			return reportf(span, "result-ok-arity", "Err(...) expects at most one argument",
				"`Err` takes zero or one argument.",
				"write `is Err()` or `is Err(E)`")
		}
	default:
		return reportf(span, "result-ok-arity", "not an Ok/Err discriminator",
			"This path expected a Result Ok/Err constraint.",
			"write `is Ok()` or `is Err()`")
	}
}
