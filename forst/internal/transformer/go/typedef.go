package transformergo

import (
	"fmt"
	"forst/internal/ast"
	"forst/internal/typechecker"
	goast "go/ast"
	"go/token"

	logrus "github.com/sirupsen/logrus"
)

func (t *Transformer) transformTypeDef(node ast.TypeDefNode) (*goast.GenDecl, error) {
	hash, err := t.TypeChecker.Hasher.HashNode(node)
	if err != nil {
		return nil, fmt.Errorf("failed to hash type node: %w", err)
	}
	hashTypeName := hash.ToTypeIdent()

	// Named homogeneous literal unions: named carrier + constants + membership helper.
	if lit, err := t.tryEmitLiteralUnionNamedType(node); err != nil {
		return nil, err
	} else if lit != nil {
		return lit, nil
	}

	// Closed union of nominal errors: emit a sealed interface + marker methods instead of `type T error`.
	if bin, ok := node.Expr.(ast.TypeDefBinaryExpr); ok && bin.IsDisjunction() {
		if sealed, err := t.tryEmitNominalErrorUnionSealedInterface(node, bin); err != nil {
			return nil, err
		} else if sealed != nil {
			return sealed, nil
		}
	}

	expr, err := t.transformTypeDefExpr(node.Expr)
	if err != nil {
		t.log.WithFields(logrus.Fields{
			"function": "transformTypeDef",
		}).WithError(err).Error("failed to transform type def expr during transformation")
		return nil, err
	}

	// Plain alias chains to a built-in (e.g. SessionId = UserId = String) emit the underlying Go type.
	// Constructor aliases (e.g. type Bytes = []Byte) keep TypeParams and are lowered via transformType.
	if ade, ok := node.Expr.(ast.TypeDefAssertionExpr); ok && ade.Assertion != nil {
		if len(ade.Assertion.TypeParams) > 0 || ade.Assertion.ArrayLen != nil {
			if tn, ok := ade.Assertion.ToTypeNode(); ok {
				goType, err := t.transformType(tn)
				if err != nil {
					return nil, err
				}
				expr = &goType
			}
		} else if bu := t.TypeChecker.UnderlyingBuiltinTypeOfAliasAssertion(node.Ident); bu != "" {
			goIdent, err := transformTypeIdent(bu)
			if err != nil {
				return nil, err
			}
			var asExpr goast.Expr = goIdent
			expr = &asExpr
		}
	}

	// Use original name for user-defined types, hash-based name for structural types
	var typeName ast.TypeIdent
	var typeNode = ast.TypeNode{Ident: node.Ident}
	if typeNode.IsHashBased() {
		// For hash-based types, use the hash-based name
		typeName = hashTypeName
	} else {
		// For user-defined types, use the original name
		typeName = node.Ident
	}

	// For comments, always use the original Forst type name
	commentName := string(node.Ident)

	comments := []*goast.Comment{
		{
			Text: fmt.Sprintf("// %s: %s", commentName, node.Expr.String()),
		},
	}

	if errEx, ok := node.Expr.(ast.TypeDefErrorExpr); ok {
		t.emitNominalErrorErrorMethod(typeName)
		t.emitNominalErrorForstErrorTagMethod(typeName)
		t.emitNominalErrorUnwrapMethod(typeName, errEx.Payload)
	}

	return &goast.GenDecl{
		Tok: token.TYPE,
		Specs: []goast.Spec{
			&goast.TypeSpec{
				Name: &goast.Ident{
				 Name: string(typeName),
				},
				Type: *expr,
			},
		},
		Doc: &goast.CommentGroup{
			List: comments,
		},
	}, nil
}

// emitNominalErrorErrorMethod emits `func (e T) Error() string` for nominal error structs.
func (t *Transformer) emitNominalErrorErrorMethod(typeName ast.TypeIdent) {
	name := string(typeName)
	fn := &goast.FuncDecl{
		Recv: &goast.FieldList{List: []*goast.Field{{
			Names: []*goast.Ident{goast.NewIdent("e")},
			Type:  goast.NewIdent(name),
		}}},
		Name: goast.NewIdent("Error"),
		Type: &goast.FuncType{
			Results: &goast.FieldList{List: []*goast.Field{{Type: goast.NewIdent("string")}}},
		},
		Body: &goast.BlockStmt{List: []goast.Stmt{
			&goast.ReturnStmt{Results: []goast.Expr{goQuotedStringLit(name)}},
		}},
	}
	t.Output.AddFunction(fn)
}

// emitNominalErrorForstErrorTagMethod emits `func (e T) ForstErrorTag() string` for wire encoding.
func (t *Transformer) emitNominalErrorForstErrorTagMethod(typeName ast.TypeIdent) {
	name := string(typeName)
	tag := name
	if pkg := t.Output.PackageName(); pkg != "" {
		tag = pkg + "/" + name
	}
	fn := &goast.FuncDecl{
		Recv: &goast.FieldList{List: []*goast.Field{{
			Names: []*goast.Ident{goast.NewIdent("e")},
			Type:  goast.NewIdent(name),
		}}},
		Name: goast.NewIdent("ForstErrorTag"),
		Type: &goast.FuncType{
			Results: &goast.FieldList{List: []*goast.Field{{Type: goast.NewIdent("string")}}},
		},
		Body: &goast.BlockStmt{List: []goast.Stmt{
			&goast.ReturnStmt{Results: []goast.Expr{goQuotedStringLit(tag)}},
		}},
	}
	t.Output.AddFunction(fn)
}

// emitNominalErrorUnwrapMethod emits `func (e T) Unwrap() error` when the payload declares `cause: Error`.
func (t *Transformer) emitNominalErrorUnwrapMethod(typeName ast.TypeIdent, payload ast.ShapeNode) {
	field, ok := payload.Fields["cause"]
	if !ok || field.Type == nil || field.Type.Ident != ast.TypeError {
		return
	}
	name := string(typeName)
	goField := "cause"
	if t.ExportReturnStructFields {
		goField = capitalizeFirst("cause")
	}
	fn := &goast.FuncDecl{
		Recv: &goast.FieldList{List: []*goast.Field{{
			Names: []*goast.Ident{goast.NewIdent("e")},
			Type:  goast.NewIdent(name),
		}}},
		Name: goast.NewIdent("Unwrap"),
		Type: &goast.FuncType{
			Results: &goast.FieldList{List: []*goast.Field{{Type: goast.NewIdent("error")}}},
		},
		Body: &goast.BlockStmt{List: []goast.Stmt{
			&goast.ReturnStmt{Results: []goast.Expr{&goast.SelectorExpr{
				X:   goast.NewIdent("e"),
				Sel: goast.NewIdent(goField),
			}}},
		}},
	}
	t.Output.AddFunction(fn)
}

func (t *Transformer) getAssertionBaseTypeIdent(assertion *ast.AssertionNode) (*goast.Ident, error) {
	if assertion.BaseType != nil {
		ident, err := transformTypeIdent(*assertion.BaseType)
		if err != nil {
			err = fmt.Errorf("failed to transform type ident during getAssertionBaseTypeIdent: %w", err)
			t.log.WithFields(logrus.Fields{
				"function": "getAssertionBaseTypeIdent",
			}).WithError(err).Error("transforming assertion base type ident failed")
			return nil, err
		}
		return ident, nil
	}

	typeNode, err := t.TypeChecker.LookupAssertionType(assertion)
	if err != nil {
		err = fmt.Errorf("failed to lookup assertion type during getAssertionBaseTypeIdent: %w", err)
		t.log.WithFields(logrus.Fields{
			"function": "getAssertionBaseTypeIdent",
		}).WithError(err).Error("transforming assertion base type ident failed")
		return nil, err
	}

	ident, err := transformTypeIdent(typeNode.Ident)
	if err != nil {
		err = fmt.Errorf("failed to transform type ident during getAssertionBaseTypeIdent: %w", err)
		t.log.WithFields(logrus.Fields{
			"function": "getAssertionBaseTypeIdent",
		}).WithError(err).Error("transforming assertion base type ident failed")
		return nil, err
	}
	return ident, nil
}

// TODO: Improve shape type registration
// This should handle nested shapes better and generate appropriate validation code
func (t *Transformer) defineShapeType(shape *ast.ShapeNode) error {
	// First register all nested shape fields
	if err := t.defineShapeFields(shape); err != nil {
		return err
	}

	// Then register the shape itself
	hash, err := t.TypeChecker.Hasher.HashNode(shape)
	if err != nil {
		return fmt.Errorf("failed to hash shape during defineShapeType: %w", err)
	}
	typeIdent := hash.ToTypeIdent()

	// Create struct type for the shape
	structType, err := t.transformShapeType(shape)
	if err != nil {
		return fmt.Errorf("failed to transform shape type %s: %w", typeIdent, err)
	}

	// Use the unified aliasing logic for the type name
	aliasName, err := t.TypeChecker.GetAliasedTypeName(ast.TypeNode{Ident: typeIdent}, typechecker.GetAliasedTypeNameOptions{AllowStructuralAlias: true})
	if err != nil || aliasName == "" {
		aliasName = string(typeIdent)
	}

	decl, err := t.transformTypeDef(ast.TypeDefNode{
		Ident: ast.TypeIdent(aliasName),
		Expr: ast.TypeDefAssertionExpr{
			Assertion: &ast.AssertionNode{
				BaseType: nil,
				Constraints: []ast.ConstraintNode{{
					Name: "Shape",
					Args: []ast.ConstraintArgumentNode{{
						Shape: shape,
					}},
				}},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to transform shape type %s: %w", typeIdent, err)
	}

	// Override the type with our struct type
	decl.Specs[0].(*goast.TypeSpec).Type = *structType
	t.Output.AddType(decl)
	return nil
}

// defineShapeFields recursively registers type definitions for all shape fields
func (t *Transformer) defineShapeFields(shape *ast.ShapeNode) error {
	for _, field := range shape.Fields {
		if field.Shape != nil {
			// Recursively register nested shapes
			if err := t.defineShapeFields(field.Shape); err != nil {
				return err
			}

			// Register the shape field type
			hash, err := t.TypeChecker.Hasher.HashNode(field.Shape)
			if err != nil {
				return fmt.Errorf("failed to hash shape field type %s: %w", field.Shape.String(), err)
			}
			typeIdent := hash.ToTypeIdent()

			// Create struct type for the shape field
			structType, err := t.transformShapeType(field.Shape)
			if err != nil {
				return fmt.Errorf("failed to transform shape field type %s: %w", typeIdent, err)
			}

			// Use the unified aliasing logic for the type name
			aliasName, err := t.TypeChecker.GetAliasedTypeName(ast.TypeNode{Ident: typeIdent}, typechecker.GetAliasedTypeNameOptions{AllowStructuralAlias: true})
			if err != nil || aliasName == "" {
				aliasName = string(typeIdent)
			}

			decl, err := t.transformTypeDef(ast.TypeDefNode{
				Ident: ast.TypeIdent(aliasName),
				Expr: ast.TypeDefAssertionExpr{
					Assertion: &ast.AssertionNode{
						BaseType: nil,
						Constraints: []ast.ConstraintNode{{
							Name: "Shape",
							Args: []ast.ConstraintArgumentNode{{
								Shape: field.Shape,
							}},
						}},
					},
				},
			})
			if err != nil {
				return fmt.Errorf("failed to transform shape field type %s: %w", typeIdent, err)
			}

			// Override the type with our struct type
			decl.Specs[0].(*goast.TypeSpec).Type = *structType
			t.Output.AddType(decl)
		}
	}
	return nil
}

// defineShapeTypes finds all shapes in type definitions and registers them
func (t *Transformer) defineShapeTypes() error {
	for ident, def := range t.TypeChecker.Defs {
		if t.TypeChecker.IsHashBasedIdent(ident) {
			continue
		}
		if typeDef, ok := def.(ast.TypeDefNode); ok {
			if assertionExpr, ok := typeDef.Expr.(ast.TypeDefAssertionExpr); ok {
				if assertionExpr.Assertion != nil {
					for _, constraint := range assertionExpr.Assertion.Constraints {
						if len(constraint.Args) > 0 && constraint.Args[0].Shape != nil {
							if err := t.defineShapeType(constraint.Args[0].Shape); err != nil {
								return err
							}
						}
					}
				}
			}
		}
	}
	return nil
}
