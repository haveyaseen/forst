package typechecker

import (
	"fmt"
	"forst/internal/ast"
	"slices"

	logrus "github.com/sirupsen/logrus"
)

// TODO: Improve type inference for complex types
// This should handle:
// 1. Binary type expressions
// 2. Nested shapes
// 3. Type aliases
// 4. Generic types
// Change function signature to accept expectedType
func (tc *TypeChecker) inferShapeType(shape ast.ShapeNode, expectedType *ast.TypeNode) (ast.TypeNode, error) {
	tc.log.WithFields(logrus.Fields{
		"function":     "inferShapeType",
		"shape":        fmt.Sprintf("%+v", shape),
		"expectedType": expectedType,
	}).Debug("Inferring shape type")

	// Typed composite literal TypeName{ … }: use BaseType, validating nominal error payloads.
	if shape.BaseType != nil && *shape.BaseType != ast.TypeShape {
		tc.log.WithFields(logrus.Fields{
			"function": "inferShapeType",
			"baseType": *shape.BaseType,
		}).Debug("Using BaseType for shape literal")
		if err := tc.validateNominalErrorStructLiteral(shape); err != nil {
			return ast.TypeNode{}, err
		}
		return ast.TypeNode{Ident: *shape.BaseType}, nil
	}

	// Try to find a matching type definition for this shape
	shapeHash, err := tc.Hasher.HashNode(shape)
	if err != nil {
		return ast.TypeNode{}, fmt.Errorf("failed to hash shape: %w", err)
	}
	shapeTypeIdent := shapeHash.ToTypeIdent()

	// Check if we already have a type definition for this shape
	if _, exists := tc.Defs[shapeTypeIdent]; exists {
		tc.log.WithFields(logrus.Fields{
			"function":  "inferShapeType",
			"typeIdent": shapeTypeIdent,
		}).Debug("Found existing type definition for shape")
		return ast.TypeNode{Ident: shapeTypeIdent}, nil
	}

	// If expectedType is a named type, try to get its definition for field types
	var expectedFields map[string]*ast.TypeNode
	if expectedType != nil {
		if typeDef, ok := tc.typeDefForIdent(expectedType.Ident); ok {
			if payload, ok := ast.PayloadShape(typeDef.Expr); ok {
				expectedFields = make(map[string]*ast.TypeNode)
				for fname, fdef := range payload.Fields {
					if fdef.Type != nil {
						t := *fdef.Type
						expectedFields[fname] = &t
					}
				}
			}
		}
	}

	// Process each field and infer its type
	processedFields := make(map[string]ast.ShapeFieldNode)

	for name, field := range shape.Fields {
		// Use expectedType for this field if available
		var fieldExpectedType *ast.TypeNode
		if expectedFields != nil {
			if t, ok := expectedFields[name]; ok {
				fieldExpectedType = t
			}
		}
		tc.log.WithFields(logrus.Fields{
			"function":     "inferShapeType",
			"fieldName":    name,
			"expectedType": fieldExpectedType,
		}).Debug("[PINPOINT] Expected type for field (from expectedType param)")

		// If the field has an assertion, infer its type
		if field.Assertion != nil {
			assertionTypes, err := tc.InferAssertionType(field.Assertion, false, name, fieldExpectedType)
			if err != nil {
				return ast.TypeNode{}, fmt.Errorf("failed to infer assertion type for field %s: %w", name, err)
			}
			if len(assertionTypes) == 0 {
				return ast.TypeNode{}, fmt.Errorf("no types inferred for assertion in field %s", name)
			}
			inferredType := assertionTypes[0]
			if !tc.isBuiltinType(inferredType.Ident) {
				tc.registerType(ast.TypeDefNode{
					Ident: inferredType.Ident,
					Expr: ast.TypeDefAssertionExpr{
						Assertion: field.Assertion,
					},
				})
			}
			tc.log.WithFields(logrus.Fields{
				"function":     "inferShapeType",
				"fieldName":    name,
				"inferredType": inferredType.Ident,
			}).Debug("[PINPOINT] Inferred type for field (from assertion)")
			field.Type = &inferredType
		}

		// If the field has a nested shape, infer its type recursively
		if field.Shape != nil {
			nestedType, err := tc.inferShapeType(*field.Shape, fieldExpectedType)
			if err != nil {
				return ast.TypeNode{}, fmt.Errorf("failed to infer nested shape type for field %s: %w", name, err)
			}
			field.Type = &nestedType
		}

		processedFields[name] = field
	}

	// Create a new shape with processed fields
	processedShape := ast.ShapeNode{
		Fields:   processedFields,
		BaseType: shape.BaseType,
	}

	// If the callee names a typedef that is not in Defs (forward ref, or tests that unregister it),
	// record the inferred shape so IsTypeCompatible can still relate the hash type to the param name.
	if expectedType != nil {
		if _, ok := tc.Defs[expectedType.Ident]; !ok {
			if tc.shapeExpectations == nil {
				tc.shapeExpectations = make(map[ast.TypeIdent]ast.ShapeNode)
			}
			tc.shapeExpectations[expectedType.Ident] = processedShape
		}
	}

	// Prefer the contextual expected type when it names a shape that matches. Otherwise map
	// iteration order over tc.Defs can bind a structurally equivalent hash type before a named alias.
	if expectedType != nil {
		if def, ok := tc.Defs[expectedType.Ident]; ok {
			if typeDef, ok := def.(ast.TypeDefNode); ok {
				if payload, ok := ast.PayloadShape(typeDef.Expr); ok {
					if tc.shapesHaveSameStructure(processedShape, *payload) {
						tc.log.WithFields(logrus.Fields{
							"function":     "inferShapeType",
							"matchingType": expectedType.Ident,
						}).Debug("Using contextual expected type for shape literal")
						return ast.TypeNode{Ident: expectedType.Ident}, nil
					}
				}
			}
		}
	}

	// Now look for a matching named type definition that has the same structure.
	// Prefer ordinary shape typedefs over nominal error typedefs when both match (same payload shape).
	matchingTypeIdent := tc.matchingTypeDefForShapeLiteral(processedShape)

	if matchingTypeIdent != "" {
		tc.log.WithFields(logrus.Fields{
			"function":  "inferShapeType",
			"namedType": matchingTypeIdent,
		}).Debug("Using matching named type directly")
		return ast.TypeNode{Ident: matchingTypeIdent}, nil
	}

	finalHash, err := tc.Hasher.HashNode(processedShape)
	if err != nil {
		return ast.TypeNode{}, fmt.Errorf("failed to hash processed shape: %w", err)
	}
	finalTypeIdent := finalHash.ToTypeIdent()
	tc.markHashBasedIdent(finalTypeIdent)
	tc.registerType(ast.TypeDefNode{
		Ident: finalTypeIdent,
		Expr:  ast.TypeDefShapeExpr{Shape: processedShape},
	})

	return ast.TypeNode{Ident: finalTypeIdent}, nil
}

// isBuiltinType checks if a type identifier is a built-in type
func (tc *TypeChecker) isBuiltinType(typeIdent ast.TypeIdent) bool {
	builtinTypes := []ast.TypeIdent{
		ast.TypeString,
		ast.TypeInt,
		ast.TypeFloat,
		ast.TypeBool,
		ast.TypeError,
		ast.TypeVoid,
		ast.TypePointer,
		ast.TypeShape,
		ast.TypeArray,
		ast.TypeMap,
		ast.TypeObject,
		ast.TypeResult,
		ast.TypeTuple,
		ast.TypeFunc,
	}

	return slices.Contains(builtinTypes, typeIdent)
}

// matchingTypeDefForShapeLiteral picks a named typedef whose payload matches processedShape.
// Prefers TypeDefShapeExpr over TypeDefErrorExpr when both match, so literals do not bind to
// nominal errors when an ordinary shape type with the same fields exists.
func (tc *TypeChecker) matchingTypeDefForShapeLiteral(processedShape ast.ShapeNode) ast.TypeIdent {
	for typeIdent, def := range tc.Defs {
		typeDef, ok := def.(ast.TypeDefNode)
		if !ok {
			continue
		}
		if _, ok := typeDef.Expr.(ast.TypeDefShapeExpr); !ok {
			continue
		}
		payload, ok := ast.PayloadShape(typeDef.Expr)
		if !ok {
			continue
		}
		if tc.shapesHaveSameStructure(processedShape, *payload) {
			tc.log.WithFields(logrus.Fields{
				"function":     "matchingTypeDefForShapeLiteral",
				"matchingType": typeIdent,
				"kind":         "TypeDefShapeExpr",
			}).Debug("Found matching shape typedef for literal")
			return typeIdent
		}
	}
	for typeIdent, def := range tc.Defs {
		typeDef, ok := def.(ast.TypeDefNode)
		if !ok {
			continue
		}
		if _, ok := typeDef.Expr.(ast.TypeDefErrorExpr); !ok {
			continue
		}
		payload, ok := ast.PayloadShape(typeDef.Expr)
		if !ok {
			continue
		}
		if tc.shapesHaveSameStructure(processedShape, *payload) {
			tc.log.WithFields(logrus.Fields{
				"function":     "matchingTypeDefForShapeLiteral",
				"matchingType": typeIdent,
				"kind":         "TypeDefErrorExpr",
			}).Debug("Found matching nominal error typedef for literal")
			return typeIdent
		}
	}
	return ""
}

// shapesHaveSameStructure compares two shapes to see if they have the same field structure
func (tc *TypeChecker) shapesHaveSameStructure(shape1, shape2 ast.ShapeNode) bool {
	tc.log.Debug("shapesHaveSameStructure called")

	tc.log.WithFields(logrus.Fields{
		"function": "shapesHaveSameStructure",
		"shape1":   fmt.Sprintf("%+v", shape1),
		"shape2":   fmt.Sprintf("%+v", shape2),
	}).Debug("Comparing shapes for structural compatibility")

	if len(shape1.Fields) != len(shape2.Fields) {
		tc.log.WithFields(logrus.Fields{
			"function": "shapesHaveSameStructure",
			"reason":   "field count mismatch",
			"fields1":  len(shape1.Fields),
			"fields2":  len(shape2.Fields),
		}).Debug("Shape field count mismatch")
		return false
	}

	for fieldName, field1 := range shape1.Fields {
		field2, exists := shape2.Fields[fieldName]
		if !exists {
			tc.log.WithFields(logrus.Fields{
				"function":  "shapesHaveSameStructure",
				"reason":    "field missing",
				"fieldName": fieldName,
			}).Debug("Field missing in compared shape")
			return false
		}

		// Get the resolved type for field1 (handling assertions)
		var field1Type *ast.TypeNode
		if field1.Type != nil {
			field1Type = field1.Type
		} else if field1.Assertion != nil {
			assertionTypes, err := tc.InferAssertionType(field1.Assertion, false, fieldName, nil)
			if err == nil && len(assertionTypes) > 0 {
				field1Type = &assertionTypes[0]
			}
		}

		// Get the resolved type for field2 (handling assertions)
		var field2Type *ast.TypeNode
		if field2.Type != nil {
			field2Type = field2.Type
		} else if field2.Assertion != nil {
			assertionTypes, err := tc.InferAssertionType(field2.Assertion, false, fieldName, nil)
			if err == nil && len(assertionTypes) > 0 {
				field2Type = &assertionTypes[0]
			}
		}

		tc.log.WithFields(logrus.Fields{
			"fieldName":  fieldName,
			"field1Type": field1Type,
			"field2Type": field2Type,
			"field1":     field1,
			"field2":     field2,
		}).Debug("Comparing field types")

		if field1.IsMethod && field2.IsMethod {
			if len(field1.MethodParams) != len(field2.MethodParams) {
				return false
			}
			for i := range field1.MethodParams {
				p1, ok1 := field1.MethodParams[i].(ast.SimpleParamNode)
				p2, ok2 := field2.MethodParams[i].(ast.SimpleParamNode)
				if !ok1 || !ok2 {
					return false
				}
				if !tc.IsTypeCompatible(p1.Type, p2.Type) && !tc.IsTypeCompatible(p2.Type, p1.Type) {
					return false
				}
			}
			if len(field1.MethodReturnTypes) != len(field2.MethodReturnTypes) {
				return false
			}
			for i := range field1.MethodReturnTypes {
				if !tc.IsTypeCompatible(field1.MethodReturnTypes[i], field2.MethodReturnTypes[i]) &&
					!tc.IsTypeCompatible(field2.MethodReturnTypes[i], field1.MethodReturnTypes[i]) {
					return false
				}
			}
			continue
		} else if field1.IsMethod != field2.IsMethod {
			return false
		}

		if field1Type != nil && field2Type != nil {
			sameIdent := field1Type.Ident == field2Type.Ident
			compatible := tc.IsTypeCompatible(*field1Type, *field2Type)
			if !sameIdent && !compatible {
				tc.log.WithFields(logrus.Fields{
					"function":   "shapesHaveSameStructure",
					"fieldName":  fieldName,
					"field1Type": field1Type.Ident,
					"field2Type": field2Type.Ident,
				}).Debug("Field type mismatch")
				return false
			}
			// Same head identifier or compatible types; continue to nested shape check below if any.
		} else if field1Type != nil || field2Type != nil {
			tc.log.WithFields(logrus.Fields{
				"function":   "shapesHaveSameStructure",
				"fieldName":  fieldName,
				"field1Type": field1Type,
				"field2Type": field2Type,
			}).Debug("One field has type, the other does not")
			return false
		}

		if field1.Shape != nil && field2.Shape != nil {
			if !tc.shapesHaveSameStructure(*field1.Shape, *field2.Shape) {
				tc.log.WithFields(logrus.Fields{
					"function":  "shapesHaveSameStructure",
					"fieldName": fieldName,
				}).Debug("Nested shape mismatch")
				return false
			}
		}
	}

	return true
}
