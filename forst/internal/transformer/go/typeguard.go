package transformergo

import (
	"fmt"
	"forst/internal/ast"
	"forst/internal/typechecker"
	goast "go/ast"

	logrus "github.com/sirupsen/logrus"
)

// transformType transforms a Forst type node into a Go type
// func (t *Transformer) transformType(tn ast.TypeNode) goast.Expr {
// 	// Look up the type alias hash for the type
// 	var typeName string
// 	if t != nil {
// 		name, err := t.getTypeAliasNameForTypeNode(tn)
// 		if err != nil {
// 			typeName = string(tn.Ident)
// 		} else {
// 			typeName = name
// 		}
// 	} else {
// 		typeName = string(tn.Ident)
// 	}
// 	return goast.NewIdent(typeName)
// }

// transformBlock transforms a block of Forst statements into Go statements (type-guard subset).
func (t *Transformer) transformBlock(block []ast.Node) *goast.BlockStmt {
	var stmts []goast.Stmt
	for _, node := range block {
		switch n := node.(type) {
		case ast.CommentNode:
			stmts = append(stmts, &goast.EmptyStmt{})
		case ast.ExpressionNode:
			expr, err := t.transformExpression(n)
			if err != nil {
				t.log.WithError(err).Error("Failed to transform expression")
				continue
			}
			stmts = append(stmts, &goast.ExprStmt{
				X: expr,
			})
		case ast.ReturnNode:
			// Transform all return values
			results := make([]goast.Expr, len(n.Values))
			for i, value := range n.Values {
				expr, err := t.transformExpression(value)
				if err != nil {
					t.log.WithError(err).Error("Failed to transform expression")
					continue
				}
				results[i] = expr
			}
			stmts = append(stmts, &goast.ReturnStmt{
				Results: results,
			})
		}
	}
	return &goast.BlockStmt{
		List: stmts,
	}
}

func (t *Transformer) transformTypeGuardParams(params []ast.ParamNode) (*goast.FieldList, error) {

	fields := &goast.FieldList{
		List: []*goast.Field{},
	}

	for _, param := range params {
		switch p := param.(type) {
		case ast.SimpleParamNode:
			paramName := string(p.Ident.ID)
			paramType := p.Type
			var ident *goast.Ident
			if t != nil {
				name, err := t.TypeChecker.GetAliasedTypeName(paramType, typechecker.GetAliasedTypeNameOptions{AllowStructuralAlias: true})
				if err != nil {
					return nil, fmt.Errorf("failed to get type alias name: %s", err)
				}
				ident = goast.NewIdent(name)
			} else {
				ident = goast.NewIdent(string(paramType.Ident))
			}
			fields.List = append(fields.List, &goast.Field{
				Names: []*goast.Ident{goast.NewIdent(paramName)},
				Type:  ident,
			})
		case ast.DestructuredParamNode:
			shapeFields, ok := t.TypeChecker.ShapeFieldsFromParamType(p.Type)
			if !ok {
				return nil, fmt.Errorf("destructured type guard param has no shape fields in type %s", p.Type.Ident)
			}
			for _, fieldName := range p.Fields {
				sf, ok := shapeFields[fieldName]
				if !ok {
					return nil, fmt.Errorf("destructured field %s not found in type guard param type", fieldName)
				}
				fieldType, ok := typechecker.ShapeFieldTypeNode(sf)
				if !ok {
					return nil, fmt.Errorf("destructured field %s has no type", fieldName)
				}
				typeExpr, err := t.transformType(fieldType)
				if err != nil {
					return nil, fmt.Errorf("failed to transform destructured field %s: %w", fieldName, err)
				}
				fields.List = append(fields.List, &goast.Field{
					Names: []*goast.Ident{goast.NewIdent(fieldName)},
					Type:  typeExpr,
				})
			}
		default:
			return nil, fmt.Errorf("unsupported type guard parameter type %T", param)
		}
	}

	return fields, nil
}

// transformTypeGuard transforms a type guard into a Go function
func (t *Transformer) transformTypeGuard(scopeNode ast.Node, guard ast.TypeGuardNode) (*goast.FuncDecl, error) {
	// Create function name with G_ prefix
	guardHash, err := t.TypeChecker.Hasher.HashNode(guard)
	if err != nil {
		return nil, fmt.Errorf("failed to hash guard: %s", err)
	}
	guardIdent := guardHash.ToGuardIdent()

	t.log.WithFields(logrus.Fields{
		"guard":      guard.Ident,
		"function":   "transformTypeGuard",
		"guardIdent": guardIdent,
	}).Debug("Transforming type guard")

	// Check if this is a type-level type guard (only contains type-level assertions)
	if t.isTypeLevelTypeGuard(guard) {
		t.log.WithFields(logrus.Fields{
			"guard":    guard.Ident,
			"function": "transformTypeGuard",
		}).Debug("Emitting stub for type-level type guard")

		subjectParam, err := t.transformTypeGuardParams([]ast.ParamNode{guard.Subject})
		if err != nil {
			return nil, fmt.Errorf("failed to transform subject parameter: %s", err)
		}
		additionalParams, err := t.transformTypeGuardParams(guard.Params)
		if err != nil {
			return nil, fmt.Errorf("failed to transform type guard parameters: %s", err)
		}
		params := append(subjectParam.List, additionalParams.List...)

		return &goast.FuncDecl{
			Name: goast.NewIdent(string(guardIdent)),
			Type: &goast.FuncType{
				Params: &goast.FieldList{List: params},
				Results: &goast.FieldList{
					List: []*goast.Field{{Type: goast.NewIdent("bool")}},
				},
			},
			Body: &goast.BlockStmt{
				List: []goast.Stmt{
					&goast.ReturnStmt{Results: []goast.Expr{goast.NewIdent("true")}},
				},
			},
		}, nil
	}

	if !t.TypeChecker.HasScopeForNode(scopeNode) {
		return nil, fmt.Errorf("type guard %s: no registered scope for transform node %s", guard.Ident, scopeNode)
	}

	// Transform subject parameter
	subjectParam, err := t.transformTypeGuardParams([]ast.ParamNode{guard.Subject})
	if err != nil {
		return nil, fmt.Errorf("failed to transform subject parameter: %s", err)
	}

	// Create parameter list
	additionalParams, err := t.transformTypeGuardParams(guard.Params)
	if err != nil {
		return nil, fmt.Errorf("failed to transform type guard parameters: %s", err)
	}

	params := append(subjectParam.List, additionalParams.List...)

	// Transform the body into a series of if-else blocks
	var bodyStmts []goast.Stmt
	hasMissableIf := false
	for _, node := range guard.Body {
		// Ensure the type guard parameter scope is active
		if err := t.restoreScope(scopeNode); err != nil {
			return nil, fmt.Errorf("failed to restore type guard parameter scope: %s", err)
		}
		switch n := node.(type) {
		case ast.CommentNode:
			bodyStmts = append(bodyStmts, &goast.EmptyStmt{})
		case *ast.IfNode:
			hasMissableIf = true
			if err := t.restoreScope(t.resolveIfScopeNode(n)); err != nil {
				return nil, fmt.Errorf("failed to restore if scope in type guard: %s", err)
			}

			// Transform if condition (must be an is assertion)
			cond, ok := n.Condition.(ast.ExpressionNode)
			if !ok {
				return nil, fmt.Errorf("if condition must be an expression")
			}

			// Transform if body — succeed when the branch matches and body completes.
			ifBody := t.transformBlock(n.Body)
			ifBody.List = append(ifBody.List, &goast.ReturnStmt{
				Results: []goast.Expr{goast.NewIdent("true")},
			})

			// Transform else-if blocks
			var elseIfs []goast.Stmt
			for i := range n.ElseIfs {
				elseIf := &n.ElseIfs[i]
				if err := t.restoreScope(t.resolveElseIfScopeNode(elseIf)); err != nil {
					return nil, fmt.Errorf("failed to restore else-if scope in type guard: %s", err)
				}

				elseIfCond, ok := elseIf.Condition.(ast.ExpressionNode)
				if !ok {
					return nil, fmt.Errorf("else-if condition must be an expression")
				}
				elseIfCondExpr, err := t.transformExpression(elseIfCond)
				if err != nil {
					return nil, fmt.Errorf("failed to transform else-if condition: %s", err)
				}
				elseIfBody := t.transformBlock(elseIf.Body)
				elseIfBody.List = append(elseIfBody.List, &goast.ReturnStmt{
					Results: []goast.Expr{goast.NewIdent("true")},
				})
				elseIfs = append(elseIfs, &goast.IfStmt{
					Cond: elseIfCondExpr,
					Body: elseIfBody,
				})
			}

			// Transform else block
			var elseBody *goast.BlockStmt
			if n.Else != nil {
				if err := t.restoreScope(t.resolveElseBlockScopeNode(n.Else)); err != nil {
					return nil, fmt.Errorf("failed to restore else scope in type guard: %s", err)
				}

				elseBody = t.transformBlock(n.Else.Body)
				elseBody.List = append(elseBody.List, &goast.ReturnStmt{
					Results: []goast.Expr{goast.NewIdent("true")},
				})
			}

			// Add if statement to body. Do not append a typed-nil elseBody into
			// a BlockStmt list — go/ast.Walk panics on nil list elements.
			elseParts := append([]goast.Stmt{}, elseIfs...)
			if elseBody != nil {
				elseParts = append(elseParts, elseBody)
			}
			var elseBranch goast.Stmt
			switch len(elseParts) {
			case 0:
				// Unmatched if falls through (fail-closed via trailing return false).
			case 1:
				elseBranch = elseParts[0]
			default:
				elseBranch = &goast.BlockStmt{List: elseParts}
			}
			condExpr, err := t.transformExpression(cond)
			if err != nil {
				return nil, fmt.Errorf("failed to transform if condition: %s", err)
			}
			bodyStmts = append(bodyStmts, &goast.IfStmt{
				Cond: condExpr,
				Body: ifBody,
				Else: elseBranch,
			})

		case ast.EnsureNode:
			if err := t.restoreScope(node); err != nil {
				return nil, fmt.Errorf("failed to restore ensure statement scope in type guard: %s", err)
			}

			// Transform ensure statement into a boolean expression
			// For type guards, we want to return true if the condition is met
			condStmts, err := t.transformTypeGuardEnsure(&n)
			if err != nil {
				return nil, fmt.Errorf("failed to transform ensure condition in type guard: %s", err)
			}
			t.log.WithFields(logrus.Fields{
				"ensure":   n,
				"stmts":    condStmts,
				"function": "transformTypeGuard",
			}).Trace("Transformed ensure condition")

			// If the condition is not met, return false
			// Use the first statement's expression as the condition
			if len(condStmts) == 0 {
				return nil, fmt.Errorf("no statements generated from ensure condition")
			}
			exprStmt, ok := condStmts[0].(*goast.ExprStmt)
			if !ok {
				return nil, fmt.Errorf("first statement is not an expression statement")
			}

			// Constraints emit success polarity; fail the guard when success is false.
			bodyStmts = append(bodyStmts, &goast.IfStmt{
				Cond: negateCondition(exprStmt.X),
				Body: &goast.BlockStmt{
					List: []goast.Stmt{
						&goast.ReturnStmt{
							Results: []goast.Expr{
								goast.NewIdent("false"),
							},
						},
					},
				},
			})
		}
	}

	// Fail closed when an `if` can miss; ensure-only guards fall through to true.
	trailing := "true"
	if hasMissableIf {
		trailing = "false"
	}
	bodyStmts = append(bodyStmts, &goast.ReturnStmt{
		Results: []goast.Expr{
			goast.NewIdent(trailing),
		},
	})

	// Create the function declaration
	decl := &goast.FuncDecl{
		Recv: nil,
		Name: goast.NewIdent(string(guardIdent)),
		Type: &goast.FuncType{
			Params: &goast.FieldList{
				List: params,
			},
			Results: &goast.FieldList{
				List: []*goast.Field{
					{
						Type: goast.NewIdent("bool"),
					},
				},
			},
		},
		Body: &goast.BlockStmt{
			List: bodyStmts,
		},
	}

	t.log.WithFields(logrus.Fields{
		"guard":      guard.Ident,
		"function":   "transformTypeGuard",
		"guardIdent": guardIdent,
		"declName":   decl.Name.Name,
		"params":     len(params),
		"bodyStmts":  len(bodyStmts),
	}).Debug("Created type guard function declaration")

	return decl, nil
}

// isTypeLevelTypeGuard checks if a type guard contains only type-level assertions
func (t *Transformer) isTypeLevelTypeGuard(guard ast.TypeGuardNode) bool {
	for _, node := range guard.Body {
		switch n := node.(type) {
		case ast.CommentNode:
			// comments do not affect type-level classification
		case ast.EnsureNode:
			// Check if all constraints in the ensure statement are type-level
			for _, chain := range n.Assertion.MeetChains() {
				for _, constraint := range chain.Constraints {
					if !t.isTypeLevelConstraint(constraint) {
						return false
					}
				}
			}
		default:
			// If there are any non-ensure nodes, it's not purely type-level
			return false
		}
	}
	return true
}

// isTypeLevelConstraint checks if a constraint is type-level (like "is" / Match shape)
func (t *Transformer) isTypeLevelConstraint(constraint ast.ConstraintNode) bool {
	// Type-level constraints are those that can't be transformed into runtime code
	return constraint.Name == "is" || constraint.Name == "Match"
}
