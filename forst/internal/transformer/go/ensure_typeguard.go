package transformergo

import (
	"fmt"
	"forst/internal/ast"
	goast "go/ast"
	"go/token"
	"strings"

	"github.com/sirupsen/logrus"
)

// transformTypeGuardEnsure transforms a type guard ensure
func (t *Transformer) transformTypeGuardEnsure(ensure *ast.EnsureNode) ([]goast.Stmt, error) {
	// Get the variable type from the symbol table
	varType, err := t.lookupEnsureSubjectTypeForEmit(*ensure)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup variable type: %w", err)
	}

	chains := ensure.Assertion.MeetChains()
	var chainExprs []goast.Expr
	for _, chain := range chains {
		if len(chain.Constraints) == 0 {
			continue
		}
		var parts []goast.Expr
		for _, constraint := range chain.Constraints {
			transformed, err := t.transformEnsureConstraint(*ensure, constraint, varType)
			if err != nil {
				return nil, fmt.Errorf("failed to transform constraint: %w", err)
			}
			parts = append(parts, transformed)
		}
		meet := parts[0]
		for i := 1; i < len(parts); i++ {
			meet = &goast.BinaryExpr{X: meet, Op: token.LAND, Y: parts[i]}
		}
		chainExprs = append(chainExprs, meet)
	}
	if len(chainExprs) == 0 {
		// Type-level / empty: success so `if !cond { return false }` is a no-op.
		return []goast.Stmt{&goast.ExprStmt{X: goast.NewIdent("true")}}, nil
	}
	joined := chainExprs[0]
	for i := 1; i < len(chainExprs); i++ {
		joined = &goast.BinaryExpr{X: joined, Op: token.LOR, Y: chainExprs[i]}
	}
	return []goast.Stmt{&goast.ExprStmt{X: joined}}, nil
}

// Helper to look up a TypeGuardNode by name
func (t *Transformer) lookupTypeGuardNode(name string) (*ast.TypeGuardNode, error) {
	t.log.WithFields(logrus.Fields{
		"requested": name,
		"function":  "lookupTypeGuardNode",
	}).Debug("Starting lookup for type guard")

	def, ok := t.TypeChecker.Defs[ast.TypeIdent(name)]
	if !ok {
		t.log.WithFields(logrus.Fields{
			"requested": name,
			"found":     false,
			"function":  "lookupTypeGuardNode",
		}).Debug("not found")
		return nil, fmt.Errorf("type guard not found: %s", name)
	}
	switch tg := def.(type) {
	case ast.TypeGuardNode:
		t.log.WithFields(logrus.Fields{
			"requested": name,
			"found":     true,
		}).Debug("lookupTypeGuardNode: found match (value)")
		return &tg, nil
	case *ast.TypeGuardNode:
		if tg == nil {
			break
		}
		t.log.WithFields(logrus.Fields{
			"requested": name,
			"found":     true,
			"function":  "lookupTypeGuardNode",
		}).Debug("found match (pointer)")
		return tg, nil
	}

	t.log.WithFields(logrus.Fields{
		"requested": name,
		"found":     false,
		"function":  "lookupTypeGuardNode",
	}).Debug("not found")
	return nil, fmt.Errorf("type guard not found: %s", name)
}

func (t *Transformer) isTypeGuardCompatible(varType ast.TypeNode, typeGuard *ast.TypeGuardNode) bool {
	t.log.WithFields(logrus.Fields{
		"varType":   varType.Ident,
		"typeGuard": typeGuard.GetIdent(),
		"function":  "isTypeGuardCompatible",
	}).Debug("Checking type guard compatibility")

	// Use varType directly as the base type
	baseType := varType
	t.log.WithFields(logrus.Fields{
		"baseType": baseType.Ident,
		"function": "isTypeGuardCompatible",
	}).Debug("Expected base type of type guard identified based on variable type")

	// Check if the type guard is defined for the base type
	for _, param := range typeGuard.Parameters() {
		paramType := param.GetType()
		t.log.WithFields(logrus.Fields{
			"paramType": paramType.Ident,
			"baseType":  baseType.Ident,
			"function":  "isTypeGuardCompatible",
		}).Trace("Checking parameter type")

		// Use the type checker's IsTypeCompatible function to handle type aliases and structural compatibility
		compatible := t.TypeChecker.IsTypeCompatible(baseType, paramType)
		t.log.WithFields(logrus.Fields{
			"typeGuard":  typeGuard.GetIdent(),
			"baseType":   baseType.Ident,
			"paramType":  paramType.Ident,
			"compatible": compatible,
			"function":   "isTypeGuardCompatible",
		}).Trace("Type compatibility check result")

		if compatible {
			t.log.WithFields(logrus.Fields{
				"typeGuard": typeGuard.GetIdent(),
				"baseType":  baseType.Ident,
				"paramType": paramType.Ident,
				"function":  "isTypeGuardCompatible",
			}).Debug("Found compatible type guard")
			return true
		}

		// If the base type is a hash-based type (T_*) and the param type is a named type,
		// check if they are structurally compatible by looking up their definitions
		if strings.HasPrefix(string(baseType.Ident), "T_") && !strings.HasPrefix(string(paramType.Ident), "T_") {
			// Get the base type definition
			if baseDef, exists := t.TypeChecker.Defs[baseType.Ident]; exists {
				if baseTypeDef, ok := baseDef.(ast.TypeDefNode); ok {
					if basePayload, ok := ast.PayloadShape(baseTypeDef.Expr); ok {
						// Get the param type definition
						if paramDef, exists := t.TypeChecker.Defs[paramType.Ident]; exists {
							if paramTypeDef, ok := paramDef.(ast.TypeDefNode); ok {
								if paramPayload, ok := ast.PayloadShape(paramTypeDef.Expr); ok {
									// Check if the shapes are structurally compatible
									if t.shapesCompatibleForExpectedType(basePayload, paramPayload) {
										t.log.WithFields(logrus.Fields{
											"typeGuard": typeGuard.GetIdent(),
											"baseType":  baseType.Ident,
											"paramType": paramType.Ident,
											"function":  "isTypeGuardCompatible",
										}).Debug("Found structurally compatible type guard")
										return true
									}
								}
							}
						}
					}
				}
			}
		}
	}

	t.log.WithFields(logrus.Fields{
		"typeGuard": typeGuard.GetIdent(),
		"baseType":  baseType.Ident,
		"function":  "isTypeGuardCompatible",
	}).Debug("No compatible type guard found")
	return false
}
