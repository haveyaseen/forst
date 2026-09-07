package typechecker

import (
	"forst/internal/ast"

	logrus "github.com/sirupsen/logrus"
)

func (tc *TypeChecker) isTypeCompatibleImpl(actual ast.TypeNode, expected ast.TypeNode) bool {
	if actual.Ident == expected.Ident {
		if ok, handled := tc.compatSameIdentTypes(actual, expected); handled {
			return ok
		}
	}
	if tc.compatUnionIntersectionRules(actual, expected) {
		return true
	}
	if tc.compatIntFamily(actual, expected) {
		return true
	}
	if tc.compatBuiltinAssignmentRules(actual, expected) {
		return true
	}
	if tc.compatAssertionPeeling(actual, expected) {
		return true
	}
	if tc.compatTypeDefAlias(actual, expected) {
		return true
	}
	if tc.compatStructuralShapes(actual, expected) {
		return true
	}
	if tc.compatSiblingTypeRefs(actual, expected) {
		return true
	}
	tc.debugCompat("Types are not compatible", logrus.Fields{
		"actual":   actual.Ident,
		"expected": expected.Ident,
		"function": "IsTypeCompatible",
	})
	return false
}

func (tc *TypeChecker) compatSameIdentTypes(actual, expected ast.TypeNode) (bool, bool) {
	switch actual.Ident {
	case ast.TypeResult:
		return tc.compatSameIdentResult(actual, expected)
	case ast.TypeTuple:
		return tc.compatSameIdentTuple(actual, expected)
	case ast.TypeArray:
		return tc.compatSameIdentArray(actual, expected)
	case ast.TypeUnion, ast.TypeIntersection:
		return false, false
	case ast.TypeAssertion:
		return assertionTypesCompatible(actual.Assertion, expected.Assertion), true
	case ast.TypeFunc:
		return tc.checkFunctionTypeCompatible(actual, expected), true
	default:
		if tc.log.IsLevelEnabled(logrus.DebugLevel) {
			tc.debugCompat("Direct type match", logrus.Fields{
				"actual": actual.Ident, "expected": expected.Ident, "function": "IsTypeCompatible",
			})
		}
		return true, true
	}
}

func (tc *TypeChecker) compatSameIdentResult(actual, expected ast.TypeNode) (bool, bool) {
	if len(actual.TypeParams) != 2 || len(expected.TypeParams) != 2 {
		return false, true
	}
	return tc.IsTypeCompatible(actual.TypeParams[0], expected.TypeParams[0]) &&
		tc.IsTypeCompatible(actual.TypeParams[1], expected.TypeParams[1]), true
}

func (tc *TypeChecker) compatSameIdentTuple(actual, expected ast.TypeNode) (bool, bool) {
	if len(actual.TypeParams) != len(expected.TypeParams) {
		return false, true
	}
	for i := range actual.TypeParams {
		if !tc.IsTypeCompatible(actual.TypeParams[i], expected.TypeParams[i]) {
			return false, true
		}
	}
	return true, true
}

func (tc *TypeChecker) compatSameIdentArray(actual, expected ast.TypeNode) (bool, bool) {
	if len(actual.TypeParams) != 1 || len(expected.TypeParams) != 1 {
		return false, true
	}
	if !arrayLengthsCompatible(actual, expected) {
		return false, true
	}
	return tc.IsTypeCompatible(actual.TypeParams[0], expected.TypeParams[0]), true
}

func (tc *TypeChecker) compatUnionIntersectionRules(actual, expected ast.TypeNode) bool {
	if actual.Ident == ast.TypeUnion && len(actual.TypeParams) > 0 {
		return tc.compatUnionLeft(actual, expected)
	}
	if expected.Ident == ast.TypeIntersection && len(expected.TypeParams) > 0 {
		return tc.compatIntersectionRight(actual, expected)
	}
	if actual.Ident == ast.TypeIntersection && len(actual.TypeParams) > 0 {
		return tc.compatIntersectionLeft(actual, expected)
	}
	if expected.Ident == ast.TypeUnion && len(expected.TypeParams) > 0 {
		return tc.compatUnionRight(actual, expected)
	}
	return false
}

func (tc *TypeChecker) compatUnionLeft(actual, expected ast.TypeNode) bool {
	for _, m := range actual.TypeParams {
		if !tc.IsTypeCompatible(m, expected) {
			return false
		}
	}
	return true
}

func (tc *TypeChecker) compatIntersectionRight(actual, expected ast.TypeNode) bool {
	for _, m := range expected.TypeParams {
		if !tc.IsTypeCompatible(actual, m) {
			return false
		}
	}
	return true
}

func (tc *TypeChecker) compatIntersectionLeft(actual, expected ast.TypeNode) bool {
	for _, m := range actual.TypeParams {
		if !tc.IsTypeCompatible(m, expected) {
			return false
		}
	}
	return true
}

func (tc *TypeChecker) compatUnionRight(actual, expected ast.TypeNode) bool {
	for _, m := range expected.TypeParams {
		if tc.IsTypeCompatible(actual, m) {
			return true
		}
	}
	return false
}

func (tc *TypeChecker) compatIntFamily(actual, expected ast.TypeNode) bool {
	return isIntFamilyIdent(actual.Ident) && isIntFamilyIdent(expected.Ident)
}

func (tc *TypeChecker) compatBuiltinAssignmentRules(actual, expected ast.TypeNode) bool {
	if expected.Ident == ast.TypeError {
		if def, ok := tc.Defs[actual.Ident].(ast.TypeDefNode); ok {
			if _, ok := def.Expr.(ast.TypeDefErrorExpr); ok {
				tc.debugCompat("Nominal error type assignable to built-in Error", logrus.Fields{
					"actual":   actual.Ident,
					"expected": expected.Ident,
					"function": "IsTypeCompatible",
				})
				return true
			}
		}
	}
	if expected.Ident == ast.TypeObject && actual.Ident != ast.TypeVoid {
		tc.debugCompat("Actual type assignable to TypeObject", logrus.Fields{
			"actual":   actual.Ident,
			"expected": expected.Ident,
			"function": "IsTypeCompatible",
		})
		return true
	}
	if expected.Ident == ast.TypePointer && len(expected.TypeParams) == 1 {
		inner := expected.TypeParams[0]
		if !isScalarTypeIdent(inner.Ident) && tc.IsTypeCompatible(actual, inner) {
			tc.debugCompat("Actual type compatible with pointer element type", logrus.Fields{
				"actual":   actual.Ident,
				"expected": expected.Ident,
				"function": "IsTypeCompatible",
			})
			return true
		}
	}
	return false
}

func (tc *TypeChecker) compatAssertionPeeling(actual, expected ast.TypeNode) bool {
	if actual.Ident == ast.TypeAssertion && actual.Assertion != nil && actual.Assertion.BaseType != nil {
		if tc.IsTypeCompatible(ast.TypeNode{Ident: *actual.Assertion.BaseType}, expected) {
			return true
		}
	}
	if expected.Ident == ast.TypeAssertion && expected.Assertion != nil && expected.Assertion.BaseType != nil {
		if tc.IsTypeCompatible(actual, ast.TypeNode{Ident: *expected.Assertion.BaseType}) {
			return true
		}
	}
	if actual.Assertion != nil && actual.Assertion.BaseType != nil {
		if tc.IsTypeCompatible(ast.TypeNode{Ident: *actual.Assertion.BaseType}, expected) {
			return true
		}
	}
	if expected.Assertion != nil && expected.Assertion.BaseType != nil {
		if tc.IsTypeCompatible(actual, ast.TypeNode{Ident: *expected.Assertion.BaseType}) {
			return true
		}
	}
	return false
}

func (tc *TypeChecker) compatTypeDefAlias(actual, expected ast.TypeNode) bool {
	actualDef, actualExists := tc.Defs[actual.Ident]
	if actualExists {
		if typeDef, ok := actualDef.(ast.TypeDefNode); ok {
			if typeDefExpr, ok := typeDefAssertionFromExpr(typeDef.Expr); ok {
				if typeDefExpr.Assertion != nil {
					if baseType, ok := typeDefExpr.Assertion.ToTypeNode(); ok {
						if tc.IsTypeCompatible(baseType, expected) {
							tc.debugCompat("Actual type is alias of expected type", logrus.Fields{
								"actual":   actual.Ident,
								"expected": expected.Ident,
								"function": "IsTypeCompatible",
							})
							return true
						}
					}
				}
			}
		}
	}
	expectedDef, expectedExists := tc.Defs[expected.Ident]
	if expectedExists {
		if typeDef, ok := expectedDef.(ast.TypeDefNode); ok {
			if typeDefExpr, ok := typeDefAssertionFromExpr(typeDef.Expr); ok {
				if typeDefExpr.Assertion != nil {
					if baseType, ok := typeDefExpr.Assertion.ToTypeNode(); ok {
						if tc.IsTypeCompatible(actual, baseType) {
							tc.debugCompat("Expected type is alias of actual type", logrus.Fields{
								"actual":   actual.Ident,
								"expected": expected.Ident,
								"function": "IsTypeCompatible",
							})
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func (tc *TypeChecker) compatStructuralShapes(actual, expected ast.TypeNode) bool {
	if tc.shapeExpectationMatches(actual, expected) {
		return true
	}
	actualDef := tc.Defs[actual.Ident]
	expectedDef := tc.Defs[expected.Ident]
	if actualDef == nil || expectedDef == nil {
		tc.debugCompat("Skipping structural compatibility - missing type definitions", logrus.Fields{
			"actual":      actual.Ident,
			"expected":    expected.Ident,
			"actualDef":   actualDef != nil,
			"expectedDef": expectedDef != nil,
			"function":    "IsTypeCompatible",
		})
		return false
	}
	tc.debugCompat("Checking structural compatibility", logrus.Fields{
		"actual":   actual.Ident,
		"expected": expected.Ident,
		"function": "IsTypeCompatible",
	})
	actualShape, actualShapeOk := tc.getShapeFromTypeDef(actualDef)
	expectedShape, expectedShapeOk := tc.getShapeFromTypeDef(expectedDef)
	if actualShapeOk && expectedShapeOk {
		identical := tc.shapesHaveSameStructure(*actualShape, *expectedShape)
		if identical {
			return true
		}
		if (*expectedShape).IsMethodOnlyContract() && tc.typeMethodsSatisfyContract(actual.Ident, *expectedShape) {
			return true
		}
		return false
	}
	if expectedShapeOk && (*expectedShape).IsMethodOnlyContract() {
		return tc.typeMethodsSatisfyContract(actual.Ident, *expectedShape)
	}
	return false
}

func (tc *TypeChecker) compatSiblingTypeRefs(actual, expected ast.TypeNode) bool {
	if _, _, ok := parseForstSiblingTypeRef(expected.Ident); ok {
		if tc.siblingShapeTypeMatches(actual, expected.Ident) {
			return true
		}
	}
	if _, _, ok := parseForstSiblingTypeRef(actual.Ident); ok {
		if tc.siblingShapeTypeMatches(expected, actual.Ident) {
			return true
		}
	}
	if expected.Assertion != nil && expected.Assertion.BaseType != nil {
		if _, _, ok := parseForstSiblingTypeRef(*expected.Assertion.BaseType); ok {
			if tc.siblingShapeTypeMatches(actual, *expected.Assertion.BaseType) {
				return true
			}
		}
	}
	return false
}
