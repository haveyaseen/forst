package transformergo

import (
	"fmt"

	"forst/internal/ast"
	"forst/internal/typechecker"
	goast "go/ast"
)

func (t *Transformer) transformFunctionParamFields(fnIdent ast.Identifier, paramIndex int, paramName string, paramType ast.TypeNode, variadic bool) ([]*goast.Field, error) {
	if ast.IsTestingTParamType(paramType) || typechecker.IsGoTypesTestingT(t.TypeChecker.GoTypeForVariable(ast.Identifier(paramName))) {
		t.Output.EnsureImport("testing")
		return []*goast.Field{{
			Names: []*goast.Ident{goast.NewIdent(paramName)},
			Type:  testingTGoTypeExpr(),
		}}, nil
	}

	var inferredTypes []ast.TypeNode
	var err error

	if paramType.Assertion != nil {
		if paramType.Assertion.BaseType != nil && len(paramType.Assertion.Constraints) == 0 {
			baseType := *paramType.Assertion.BaseType
			baseTypeNode := ast.TypeNode{Ident: baseType}
			if !baseTypeNode.IsHashBased() {
				name, err := t.TypeChecker.GetAliasedTypeName(baseTypeNode, typechecker.GetAliasedTypeNameOptions{AllowStructuralAlias: true})
				if err != nil {
					return nil, fmt.Errorf("failed to get aliased type name for parameter %s: %w", paramName, err)
				}
				return []*goast.Field{{
					Names: []*goast.Ident{goast.NewIdent(paramName)},
					Type:  goast.NewIdent(name),
				}}, nil
			}
		}
		inferredTypes, err = t.TypeChecker.InferAssertionType(paramType.Assertion, false, "", nil)
		if err != nil {
			return nil, fmt.Errorf("failed to infer assertion type for parameter %s: %w", paramName, err)
		}
	} else {
		inferredTypes = []ast.TypeNode{paramType}
	}

	if len(inferredTypes) == 0 {
		return nil, fmt.Errorf("no inferred type found for parameter %s", paramName)
	}

	if len(inferredTypes) == 1 && inferredTypes[0].IsResultType() {
		fields, err := t.transformTypes(inferredTypes)
		if err != nil {
			return nil, fmt.Errorf("failed to transform Result type for parameter %s: %w", paramName, err)
		}
		out := make([]*goast.Field, 0, len(fields.List))
		for i, f := range fields.List {
			field := *f
			if i == 0 && paramName != "" {
				field.Names = []*goast.Ident{goast.NewIdent(paramName)}
			} else if i == 1 {
				field.Names = []*goast.Ident{goast.NewIdent("_")}
			}
			out = append(out, &field)
		}
		return out, nil
	}

	if inlineType, ok, err := t.tryInlineGenericShapeParamType(fnIdent, paramType); err != nil {
		return nil, err
	} else if ok {
		t.markInlineGenericShapeParam(fnIdent, paramIndex)
		return []*goast.Field{{
			Names: []*goast.Ident{goast.NewIdent(paramName)},
			Type:  inlineType,
		}}, nil
	}

	typeExpr, err := t.transformType(inferredTypes[0])
	if err != nil {
		return nil, fmt.Errorf("failed to transform type for parameter %s: %w", paramName, err)
	}
	if variadic {
		typeExpr = &goast.Ellipsis{Elt: typeExpr}
	}

	field := &goast.Field{Type: typeExpr}
	if paramName != "" {
		field.Names = []*goast.Ident{goast.NewIdent(paramName)}
	}
	return []*goast.Field{field}, nil
}

func (t *Transformer) transformFunctionParams(fnIdent ast.Identifier, params []ast.ParamNode) (*goast.FieldList, error) {
	t.log.Debugf("transformFunctionParams: processing %d parameters", len(params))

	var sig *typechecker.FunctionSignature
	if s, ok := t.TypeChecker.Functions[fnIdent]; ok {
		sig = &s
	}

	fields := &goast.FieldList{
		List: []*goast.Field{},
	}

	for i, param := range params {
		switch p := param.(type) {
		case ast.SimpleParamNode:
			paramType := p.Type
			if sig != nil && i < len(sig.Parameters) {
				paramType = sig.Parameters[i].Type
			}
			t.log.Debugf("transformFunctionParams: param %d '%s' has type %q", i, p.Ident.ID, paramType.Ident)
			paramFields, err := t.transformFunctionParamFields(fnIdent, i, string(p.Ident.ID), paramType, p.Variadic)
			if err != nil {
				return nil, err
			}
			fields.List = append(fields.List, paramFields...)
		case ast.DestructuredParamNode:
			shapeFields, ok := t.TypeChecker.ShapeFieldsFromParamType(p.Type)
			if !ok {
				return nil, fmt.Errorf("destructured parameter has no shape fields in type %s", p.Type.Ident)
			}
			for _, fieldName := range p.Fields {
				sf, ok := shapeFields[fieldName]
				if !ok {
					return nil, fmt.Errorf("destructured field %s not found in parameter type", fieldName)
				}
				fieldType, ok := typechecker.ShapeFieldTypeNode(sf)
				if !ok {
					return nil, fmt.Errorf("destructured field %s has no type", fieldName)
				}
				t.log.Debugf("transformFunctionParams: destructured field %s has type %q", fieldName, fieldType.Ident)
				paramFields, err := t.transformFunctionParamFields(fnIdent, i, fieldName, fieldType, false)
				if err != nil {
					return nil, err
				}
				fields.List = append(fields.List, paramFields...)
			}
		default:
			return nil, fmt.Errorf("unsupported parameter type %T", param)
		}
	}

	return fields, nil
}

// transformFunction converts a Forst function node to a Go function declaration
func (t *Transformer) transformFunction(scopeNode ast.Node, n ast.FunctionNode) (*goast.FuncDecl, error) {
	if n.Receiver != nil && len(n.TypeParams) > 0 {
		return nil, fmt.Errorf("generic methods are not supported")
	}
	if err := t.restoreScope(scopeNode); err != nil {
		return nil, fmt.Errorf("failed to restore function scope: %s", err)
	}

	prevResultSplit := t.resultLocalSplit
	t.resultLocalSplit = make(map[string]resultLocalSplit)
	defer func() { t.resultLocalSplit = prevResultSplit }()

	prevFnBody := t.currentFnBody
	t.currentFnBody = n.Body
	defer func() { t.currentFnBody = prevFnBody }()

	prevMapIndexCache := t.mapIndexFuncLitCache
	t.mapIndexFuncLitCache = make(map[string]*goast.FuncLit)
	defer func() { t.mapIndexFuncLitCache = prevMapIndexCache }()

	// Create function parameters
	params, err := t.transformFunctionParams(n.Ident.ID, n.Params)
	if err != nil {
		return nil, fmt.Errorf("failed to transform function parameters: %s", err)
	}
	var providersParamName string
	params, providersParamName, err = t.prependProvidersParam(params, n)
	if err != nil {
		return nil, fmt.Errorf("failed to prepend providers parameter: %w", err)
	}

	prevProvidersName := t.currentFnProvidersName
	prevProvidersSlots := t.currentFnProvidersSlots
	if t.functionNeedsProvidersParam(n) {
		t.currentFnProvidersName = providersParamName
		t.currentFnProvidersSlots = t.TypeChecker.FunctionProviders[n.Ident.ID]
	} else {
		t.currentFnProvidersName = ""
		t.currentFnProvidersSlots = nil
	}
	defer func() {
		t.currentFnProvidersName = prevProvidersName
		t.currentFnProvidersSlots = prevProvidersSlots
	}()

	// Create function return type
	returnType, err := t.TypeChecker.LookupFunctionReturnType(&n)
	if err != nil {
		return nil, err
	}
	var results *goast.FieldList
	isMainFunc := t.isMainPackage() && n.HasMainFunctionName()
	if !isMainFunc && !typechecker.IsVoidReturnTypes(returnType) {
		results, err = t.transformTypes(returnType)
		if err != nil {
			return nil, fmt.Errorf("failed to transform types: %s", err)
		}
	}

	implicitIdx, hasTrailingExpr := lastImplicitReturnIndex(n.Body)
	emitImplicitReturn := hasTrailingExpr && !isMainFunc && !typechecker.IsVoidReturnTypes(returnType)

	// Create function body statements
	stmts := []goast.Stmt{}

	for i, stmt := range n.Body {
		if emitImplicitReturn && i == implicitIdx {
			continue
		}
		if err := t.restoreScope(scopeNode); err != nil {
			return nil, fmt.Errorf("failed to restore function scope in body: %s", err)
		}

		switch s := stmt.(type) {
		case ast.UseNode:
			goStmt, err := t.transformUseStatement(s)
			if err != nil {
				return nil, fmt.Errorf("failed to transform use statement: %w", err)
			}
			if _, ok := goStmt.(*goast.EmptyStmt); ok && s.Ident == nil {
				continue
			}
			stmts = append(stmts, goStmt)
		case ast.WithNode:
			withStmts, err := t.transformWithStatements(stmt, s)
			if err != nil {
				return nil, fmt.Errorf("failed to transform with block: %w", err)
			}
			stmts = append(stmts, withStmts...)
		default:
			goStmt, err := t.transformStatement(stmt)
			if err != nil {
				return nil, fmt.Errorf("failed to transform statement: %s", err)
			}
			stmts = append(stmts, goStmt)
		}
	}

	if emitImplicitReturn {
		implicitExpr, ok := n.Body[implicitIdx].(ast.ExpressionNode)
		if !ok {
			return nil, fmt.Errorf("internal error: implicit return index is not an expression")
		}
		retStmt, err := t.transformStatement(ast.ReturnNode{
			Values: []ast.ExpressionNode{implicitExpr},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to transform implicit return: %w", err)
		}
		stmts = append(stmts, retStmt)
	}

	var recv *goast.FieldList
	if n.Receiver != nil {
		recvType, err := t.transformType(n.Receiver.Type)
		if err != nil {
			return nil, fmt.Errorf("failed to transform receiver type: %w", err)
		}
		var names []*goast.Ident
		if n.Receiver.Ident.ID != "" {
			names = []*goast.Ident{goast.NewIdent(string(n.Receiver.Ident.ID))}
		}
		recv = &goast.FieldList{
			List: []*goast.Field{{Names: names, Type: recvType}},
		}
	}

	// Make sure that functions return nil if they return an error
	if !isMainFunc && !typechecker.IsVoidReturnTypes(returnType) && len(returnType) > 0 {
		lastReturnType := returnType[len(returnType)-1]
		needsTrailingNil := lastReturnType.IsError() ||
			(lastReturnType.IsResultType() && len(lastReturnType.TypeParams) >= 1 &&
				lastReturnType.TypeParams[0].Ident == ast.TypeVoid)
		if needsTrailingNil {
			var lastStmt ast.Node
			for i := len(n.Body) - 1; i >= 0; i-- {
				if _, ok := n.Body[i].(ast.CommentNode); ok {
					continue
				}
				lastStmt = n.Body[i]
				break
			}
			if lastStmt == nil || lastStmt.Kind() != ast.NodeKindReturn {
				stmts = append(stmts, &goast.ReturnStmt{
					Results: []goast.Expr{
						goast.NewIdent("nil"),
					},
				})
			}
		}
	}

	// Create the function declaration
	typeParams, err := t.transformTypeParams(n.TypeParams)
	if err != nil {
		return nil, fmt.Errorf("failed to transform type parameters: %w", err)
	}
	return &goast.FuncDecl{
		Recv: recv,
		Name: goast.NewIdent(n.Ident.String()),
		Type: &goast.FuncType{
			TypeParams: typeParams,
			Params:     params,
			Results:    results,
		},
		Body: &goast.BlockStmt{
			List: stmts,
		},
	}, nil
}

func (t *Transformer) transformTypeParams(params []ast.TypeParamDecl) (*goast.FieldList, error) {
	if len(params) == 0 {
		return nil, nil
	}
	list := make([]*goast.Field, 0, len(params))
	for _, tp := range params {
		var constraint goast.Expr
		if tp.Constraint != nil {
			switch tp.Constraint.Ident {
			case ast.TypeIdent("any"), ast.TypeIdent("comparable"):
				constraint = goast.NewIdent(string(tp.Constraint.Ident))
			default:
				c, err := t.transformType(*tp.Constraint)
				if err != nil {
					return nil, err
				}
				constraint = c
			}
		} else {
			constraint = goast.NewIdent("any")
		}
		list = append(list, &goast.Field{
			Names: []*goast.Ident{goast.NewIdent(string(tp.Name))},
			Type:  constraint,
		})
	}
	return &goast.FieldList{List: list}, nil
}
