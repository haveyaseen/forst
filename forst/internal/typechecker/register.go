package typechecker

import (
	"forst/internal/ast"

	logrus "github.com/sirupsen/logrus"
)

// ensureTypeKind ensures that a TypeNode has the correct TypeKind set
func ensureTypeKind(typeNode ast.TypeNode, expectedKind ast.TypeKind) ast.TypeNode {
	if typeNode.TypeKind != expectedKind {
		typeNode.TypeKind = expectedKind
	}
	return typeNode
}

// ensureUserDefinedType ensures that a TypeNode is marked as user-defined
func ensureUserDefinedType(typeNode ast.TypeNode) ast.TypeNode {
	return ensureTypeKind(typeNode, ast.TypeKindUserDefined)
}

func (tc *TypeChecker) storeInferredVariableType(variable ast.VariableNode, typ []ast.TypeNode) {
	tc.log.Tracef("Storing inferred variable type for variable %s: %s", variable.Ident.ID, typ)
	tc.storeSymbol(variable.Ident.ID, typ, SymbolVariable)
	tc.storeInferredType(variable, typ)
	tc.clearStaleIdentGoType(variable.Ident.ID)
	hash, err := tc.Hasher.HashNode(variable)
	if err != nil {
		return
	}
	if stored, ok := tc.Types[hash]; ok {
		tc.VariableTypes[variable.Ident.ID] = stored
	}
}

// Stores a type definition that will be used by code generators
// to create corresponding type definitions in the target language.
// For example, a Forst type definition like `type PhoneNumber = String.Min(3)`
// may be transformed into a TypeScript type with validation decorators.
func (tc *TypeChecker) registerType(node ast.TypeDefNode) {
	if _, exists := tc.Defs[node.Ident]; exists {
		return
	}

	// Store the type definition node
	tc.setDef(node.Ident, node)
	tc.log.WithFields(logrus.Fields{
		"node":     node.String(),
		"function": "registerType",
		"typeKind": "user-defined", // All registered types are user-defined
	}).Trace("Registered type")

	// If this is a shape type, also store the underlying ShapeNode for field access
	if assertionExpr, ok := node.Expr.(ast.TypeDefAssertionExpr); ok {
		if assertionExpr.Assertion != nil {
			// If this is a direct shape alias (e.g. type AppContext = { ... })
			if assertionExpr.Assertion.BaseType != nil && *assertionExpr.Assertion.BaseType == ast.TypeShape {
				// If there are no constraints, check if the assertion has a Match constraint with a shape
				if len(assertionExpr.Assertion.Constraints) == 0 && assertionExpr.Assertion.BaseType != nil {
					// See if the assertion itself has a shape (Match constraint)
					if assertionExpr.Assertion != nil && assertionExpr.Assertion.Constraints != nil {
						for _, constraint := range assertionExpr.Assertion.Constraints {
							for _, arg := range constraint.Args {
								if arg.Shape != nil {
									tc.log.WithFields(logrus.Fields{
										"node":     node.String(),
										"function": "registerType",
										"shape":    arg.Shape,
									}).Trace("Registering shape type from assertion")
									tc.registerShapeType(node.Ident, *arg.Shape)
								}
							}
						}
					}
				}
				// Try to extract from constraints if present (existing logic)
				for _, constraint := range assertionExpr.Assertion.Constraints {
					for _, arg := range constraint.Args {
						if arg.Shape != nil {
							tc.log.WithFields(logrus.Fields{
								"node":     node.String(),
								"function": "registerType",
								"shape":    arg.Shape,
							}).Trace("Registering shape type from constraints")
							tc.registerShapeType(node.Ident, *arg.Shape)
						}
					}
				}
			}
		}
	} else if _, ok := node.Expr.(ast.TypeDefErrorExpr); ok {
		tc.log.WithFields(logrus.Fields{
			"node":     node.String(),
			"function": "registerType",
		}).Trace("Registering nominal error type (wraps payload shape)")
		tc.registerErrorNominalType(node)
	} else if shapeExpr, ok := node.Expr.(ast.TypeDefShapeExpr); ok {
		// If the type definition is directly a shape, store it with a special key
		tc.log.WithFields(logrus.Fields{
			"node":     node.String(),
			"function": "registerType",
			"shape":    shapeExpr.Shape,
		}).Trace("Registering shape type from type definition")

		tc.registerShapeType(node.Ident, shapeExpr.Shape)
	}

	if assertionExpr, ok := typeDefAssertionFromExpr(node.Expr); ok && assertionExpr.Assertion != nil &&
		assertionExpr.Assertion.BaseType != nil && len(assertionExpr.Assertion.Constraints) == 0 {
		tc.registerGoQualifiedTypeAlias(node.Ident, *assertionExpr.Assertion.BaseType)
	}
}

func (tc *TypeChecker) normalizeShapeFieldKinds(shape *ast.ShapeNode) {
	for fieldName, field := range shape.Fields {
		if field.Type != nil {
			if field.Type.TypeKind != ast.TypeKindHashBased && !tc.isBuiltinType(field.Type.Ident) {
				field.Type.TypeKind = ast.TypeKindUserDefined
			}
			shape.Fields[fieldName] = field
		}
	}
}

// registerErrorNominalType stores `error X { ... }`: a TypeDefErrorExpr wrapping the payload shape.
func (tc *TypeChecker) registerErrorNominalType(node ast.TypeDefNode) {
	errEx, ok := node.Expr.(ast.TypeDefErrorExpr)
	if !ok {
		return
	}
	payload := errEx.Payload
	tc.normalizeShapeFieldKinds(&payload)
	tc.setDef(node.Ident, ast.TypeDefNode{
		Ident: node.Ident,
		Expr:  ast.TypeDefErrorExpr{Payload: payload},
	})
	tc.log.WithFields(logrus.Fields{
		"ident":    node.Ident,
		"function": "registerErrorNominalType",
	}).Trace("Registered nominal error type")
}

// registerShapeType registers an ordinary shape-backed type (`type X = { ... }`).
func (tc *TypeChecker) registerShapeType(ident ast.TypeIdent, shape ast.ShapeNode) {
	tc.normalizeShapeFieldKinds(&shape)

	tc.setDef(ident, ast.TypeDefNode{
		Ident: ident,
		Expr:  ast.TypeDefShapeExpr{Shape: shape},
	})

	tc.log.WithFields(logrus.Fields{
		"ident":    ident,
		"shape":    shape,
		"function": "registerShapeType",
		"typeKind": "user-defined",
	}).Trace("Registered shape type")
}

func (tc *TypeChecker) registerFunction(fn ast.FunctionNode) {
	if fn.Receiver != nil {
		recvType := fn.Receiver.Type.Ident
		if fn.Receiver.Type.Ident == ast.TypePointer && len(fn.Receiver.Type.TypeParams) == 1 {
			recvType = fn.Receiver.Type.TypeParams[0].Ident
		}
		tc.registerTypeMethod(recvType, string(fn.Ident.ID), fn)
		return
	}

	sig := normalizeGenericSignature(fn)
	tc.Functions[fn.Ident.ID] = sig

	for i, param := range fn.Params {
		switch p := param.(type) {
		case ast.SimpleParamNode:
			paramType := sig.Parameters[i].Type
			if p.Variadic {
				paramType = ast.NewArrayType(paramType)
			}
			tc.storeSymbol(p.Ident.ID, []ast.TypeNode{paramType}, SymbolParameter)
		case ast.DestructuredParamNode:
			tc.registerDestructuredParamSymbols(p.Fields, sig.Parameters[i].Type, SymbolParameter)
		}
	}
}

func (tc *TypeChecker) registerTypeGuard(guard *ast.TypeGuardNode) {
	// Store type guard in Defs
	if _, exists := tc.Defs[ast.TypeIdent(guard.Ident)]; !exists {
		tc.setDef(ast.TypeIdent(guard.Ident), guard)
		tc.log.WithFields(logrus.Fields{
			"guard":    guard.Ident,
			"function": "registerTypeGuard",
		}).Trace("Registered type guard")
	}

	// Store subject symbol in the current scope
	tc.storeSymbol(
		ast.Identifier(guard.Subject.GetIdent()),
		[]ast.TypeNode{guard.Subject.GetType()},
		SymbolParameter,
	)

	// Store parameter symbols in the current scope
	for _, param := range guard.Params {
		switch p := param.(type) {
		case ast.SimpleParamNode:
			tc.storeSymbol(
				p.Ident.ID,
				[]ast.TypeNode{p.Type},
				SymbolParameter,
			)
		case ast.DestructuredParamNode:
			tc.registerDestructuredParamSymbols(p.Fields, p.Type, SymbolParameter)
		}
	}
}
