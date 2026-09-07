package transformergo

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"forst/internal/ast"
	goast "go/ast"
	"go/token"
)

func (t *Transformer) transformExpression(expr ast.ExpressionNode) (goast.Expr, error) {
	switch e := expr.(type) {
	case ast.IntLiteralNode:
		lit := e.Source()
		if lit == "" {
			lit = strconv.FormatInt(e.Value, 10)
		}
		return &goast.BasicLit{
			Kind:  token.INT,
			Value: lit,
		}, nil
	case ast.FloatLiteralNode:
		return &goast.BasicLit{
			Kind:  token.FLOAT,
			Value: strconv.FormatFloat(e.Value, 'f', -1, 64),
		}, nil
	case ast.StringLiteralNode:
		return &goast.BasicLit{
			Kind:  token.STRING,
			Value: strconv.Quote(e.Value),
		}, nil
	case ast.RuneLiteralNode:
		return &goast.BasicLit{
			Kind:  token.CHAR,
			Value: strconv.QuoteRune(rune(e.Value)),
		}, nil
	case ast.BoolLiteralNode:
		if e.Value {
			return goast.NewIdent("true"), nil
		}
		return goast.NewIdent("false"), nil
	case ast.NilLiteralNode:
		return goast.NewIdent("nil"), nil
	case ast.IotaLiteralNode:
		return goast.NewIdent("iota"), nil
	case ast.FunctionLiteralNode:
		return t.transformFunctionLiteral(expr, e)
	case ast.TypeExpressionNode:
		return t.transformType(e.Type)
	case ast.ArrayLiteralNode:
		return t.transformArrayLiteral(e, nil)
	case ast.MapLiteralNode:
		if e.Type.Ident != ast.TypeMap || len(e.Type.TypeParams) != 2 {
			return nil, fmt.Errorf("map literal: invalid type %v", e.Type)
		}
		keyGo, err := t.transformType(e.Type.TypeParams[0])
		if err != nil {
			return nil, err
		}
		valGo, err := t.transformType(e.Type.TypeParams[1])
		if err != nil {
			return nil, err
		}
		elts := make([]goast.Expr, 0, len(e.Entries))
		for _, ent := range e.Entries {
			kx, err := t.transformExpression(ent.Key)
			if err != nil {
				return nil, err
			}
			vx, err := t.transformExpression(ent.Value)
			if err != nil {
				return nil, err
			}
			elts = append(elts, &goast.KeyValueExpr{Key: kx, Value: vx})
		}
		return &goast.CompositeLit{
			Type: &goast.MapType{Key: keyGo, Value: valGo},
			Elts: elts,
		}, nil
	case ast.UnaryExpressionNode:
		op, err := t.transformOperator(e.Operator)
		if err != nil {
			return nil, err
		}
		expr, err := t.transformExpression(e.Operand)
		if err != nil {
			return nil, err
		}
		return &goast.UnaryExpr{
			Op: op,
			X:  expr,
		}, nil
	case ast.BinaryExpressionNode:
		if e.Operator == ast.TokenIs {
			var asn *ast.AssertionNode
			switch r := e.Right.(type) {
			case ast.AssertionNode:
				cp := r
				asn = &cp
			case *ast.AssertionNode:
				asn = r
			}
			if asn != nil {
				return t.transformIfIsCondition(e.Left, asn)
			}
		}
		left, err := t.transformExpression(e.Left)
		if err != nil {
			return nil, err
		}
		right, err := t.transformExpression(e.Right)
		if err != nil {
			return nil, err
		}
		op, err := t.transformOperator(e.Operator)
		if err != nil {
			return nil, err
		}
	if e.Operator == ast.TokenPlus {
		left, right = t.coerceGoStringConcatOperands(e.Left, e.Right, left, right)
		left, right = t.coerceGoStringPlusAlias(e.Left, e.Right, left, right)
	}
		return &goast.BinaryExpr{
			X:  left,
			Op: op,
			Y:  right,
		}, nil
	case ast.VariableNode:
		// Support field access: split by '.' and capitalize each field after the first
		parts := strings.Split(e.GetIdent(), ".")
		if len(parts) == 1 {
			if t != nil && t.resultLocalSplit != nil {
				if split, ok := t.resultLocalSplit[parts[0]]; ok && len(split.successGoNames) > 1 {
					return nil, fmt.Errorf(
						"codegen: %q is a Result whose success lowers to multiple Go values %v; use multiple left-hand names (e.g. a, b, err := pkg.F()) instead of a single binding",
						parts[0], split.successGoNames)
				}
				if split, ok := t.resultLocalSplit[parts[0]]; ok {
					if expr, ok2 := t.goExprForNarrowedResultSplitLocal(e, split); ok2 {
						return expr, nil
					}
				}
			}
			return &goast.Ident{Name: parts[0]}, nil
		}
		var sel goast.Expr = goast.NewIdent(parts[0])
		ownerTypes := t.TypeChecker.VariableTypes[ast.Identifier(parts[0])]
		for _, field := range parts[1:] {
			fieldName := t.goSelectorFieldName(ownerTypes, field)
			sel = &goast.SelectorExpr{
				X:   sel,
				Sel: goast.NewIdent(fieldName),
			}
			ownerTypes = t.ownerTypesAfterField(ownerTypes, field)
		}
		sel = t.appendResultStoragePayloadSelector(e, sel)
		return sel, nil
	case ast.IndexExpressionNode:
		ts, err := t.TypeChecker.LookupInferredType(e, false)
		if err == nil && len(ts) == 1 && ts[0].IsResultType() {
			return t.transformMapIndexResultCall(e, ts[0])
		}
		tgt, err := t.transformExpression(e.Target)
		if err != nil {
			return nil, err
		}
		idx, err := t.transformExpression(e.Index)
		if err != nil {
			return nil, err
		}
		return &goast.IndexExpr{
			X:     tgt,
			Index: idx,
		}, nil

	case ast.SliceExpressionNode:
		tgt, err := t.transformExpression(e.Target)
		if err != nil {
			return nil, err
		}
		slice := &goast.SliceExpr{X: tgt}
		if e.Low != nil {
			slice.Low, err = t.transformExpression(e.Low)
			if err != nil {
				return nil, err
			}
		}
		if e.High != nil {
			slice.High, err = t.transformExpression(e.High)
			if err != nil {
				return nil, err
			}
		}
		return slice, nil

	case ast.SpreadExpressionNode:
		return nil, fmt.Errorf("spread expression is only valid in function call arguments")

	case ast.FieldAccessNode:
		if idx, convErr := strconv.Atoi(string(e.Field.ID)); convErr == nil {
			if vn, ok := e.Target.(ast.VariableNode); ok && t.resultLocalSplit != nil {
				if split, ok := t.resultLocalSplit[string(vn.Ident.ID)]; ok && split.errGoName == "" &&
					idx >= 0 && idx < len(split.successGoNames) {
					return goast.NewIdent(split.successGoNames[idx]), nil
				}
			}
		}
		tgt, err := t.transformExpression(e.Target)
		if err != nil {
			return nil, err
		}
		ownerTypes, _ := t.TypeChecker.LookupInferredType(e.Target, false)
		fieldName := t.goSelectorFieldName(ownerTypes, string(e.Field.ID))
		return &goast.SelectorExpr{
			X:   tgt,
			Sel: goast.NewIdent(fieldName),
		}, nil

	case ast.MethodCallNode:
		recv, err := t.transformExpression(e.Receiver)
		if err != nil {
			return nil, err
		}
		args, err := t.transformFunctionCallArgs(e.Method.ID, e.Arguments)
		if err != nil {
			return nil, err
		}
		return &goast.CallExpr{
			Fun: &goast.SelectorExpr{
				X:   recv,
				Sel: goast.NewIdent(string(e.Method.ID)),
			},
			Args:     args.exprs,
			Ellipsis: args.ellipsis,
		}, nil

	case ast.FunctionCallNode:
		if e.Callee != nil {
			funExpr, err := t.transformExpression(e.Callee)
			if err != nil {
				return nil, err
			}
			funExpr, err = t.wrapCallFunWithTypeArgs(funExpr, e.TypeArgs)
			if err != nil {
				return nil, err
			}
			args, err := t.transformFunctionCallArgs(ast.Identifier("_callee"), e.Arguments)
			if err != nil {
				return nil, err
			}
			return &goast.CallExpr{
				Fun:      funExpr,
				Args:     args.exprs,
				Ellipsis: args.ellipsis,
			}, nil
		}
		if isPrintLikeBuiltinCall(e.Function) {
			args, err := t.transformPrintBuiltinCallArgs(e.Arguments)
			if err != nil {
				return nil, err
			}
			return &goast.CallExpr{
				Fun:  goFunExprFromForstCallIdent(e.Function),
				Args: args,
			}, nil
		}
		if e.Function.ID == "string" && len(e.Arguments) == 1 {
			if goExpr, ok, err := t.transformStringBuiltinCall(e.Arguments[0]); ok {
				if err != nil {
					return nil, err
				}
				return goExpr, nil
			}
		}
		if e.Function.ID == "[]byte" && len(e.Arguments) == 1 {
			arg, err := t.transformExpression(e.Arguments[0])
			if err != nil {
				return nil, err
			}
			return &goast.CallExpr{
				Fun:  &goast.ArrayType{Elt: goast.NewIdent("byte")},
				Args: []goast.Expr{arg},
			}, nil
		}
		if call, ok, err := t.transformNodeQualifiedCall(e); ok {
			if err != nil {
				return nil, err
			}
			return call, nil
		}
		if call, ok, err := t.transformGoBoundDottedMethodCall(e); ok {
			if err != nil {
				return nil, err
			}
			return call, nil
		}
		args, err := t.transformFunctionCallArgs(e.Function.ID, e.Arguments)
		if err != nil {
			return nil, err
		}
		funExpr, err := t.wrapCallFunWithTypeArgs(t.goFunExprFromForstCallIdentWithNarrowing(e.Function), e.TypeArgs)
		if err != nil {
			return nil, err
		}
		call := &goast.CallExpr{
			Fun:      funExpr,
			Args:     args.exprs,
			Ellipsis: args.ellipsis,
		}
		return call, nil
	case ast.ReferenceNode:
		expr, err := t.transformExpression(e.Value)
		if err != nil {
			return nil, err
		}
		if composite, ok := expr.(*goast.CompositeLit); ok {
			return &goast.UnaryExpr{Op: token.AND, X: composite}, nil
		}
		return &goast.UnaryExpr{
			Op: token.AND,
			X:  expr,
		}, nil
	case ast.DereferenceNode:
		expr, err := t.transformExpression(e.Value)
		if err != nil {
			return nil, err
		}
		return &goast.UnaryExpr{
			Op: token.MUL,
			X:  expr,
		}, nil
	case ast.ShapeNode:
		// Use the unified helper to determine the expected type
		context := &ShapeContext{}
		expectedType := t.getExpectedTypeForShape(&e, context)
		// Always use the unified aliasing logic for shape literals
		return t.transformShapeNodeWithExpectedType(&e, expectedType, nil)

	case ast.OkExprNode, ast.ErrExprNode:
		return nil, fmt.Errorf("Ok(...) and Err(...) are lowered in return statements only (not as a value expression)")
	}

	return nil, fmt.Errorf("unsupported expression type: %s", reflect.TypeOf(expr).String())
}

func (t *Transformer) transformExpressionWithExpected(expr ast.ExpressionNode, expected *ast.TypeNode) (goast.Expr, error) {
	if arr, ok := expr.(ast.ArrayLiteralNode); ok {
		return t.transformArrayLiteral(arr, expected)
	}
	return t.transformExpression(expr)
}

func (t *Transformer) transformArrayLiteral(e ast.ArrayLiteralNode, expectedArrayType *ast.TypeNode) (goast.Expr, error) {
	elemType := ast.TypeNode{Ident: ast.TypeInt}
	if e.Type.Ident != ast.TypeImplicit && e.Type.Ident != "" {
		elemType = e.Type
	} else if expectedArrayType != nil && expectedArrayType.Ident == ast.TypeArray && len(expectedArrayType.TypeParams) > 0 {
		elemType = expectedArrayType.TypeParams[0]
	} else if hash, err := t.TypeChecker.Hasher.HashNode(e); err == nil {
		if types, ok := t.TypeChecker.Types[hash]; ok && len(types) == 1 &&
			types[0].Ident == ast.TypeArray && len(types[0].TypeParams) > 0 {
			elemType = types[0].TypeParams[0]
		}
	}
	eltGo, err := t.transformType(elemType)
	if err != nil {
		return nil, err
	}
	elts := make([]goast.Expr, 0, len(e.Value))
	for _, item := range e.Value {
		var ex goast.Expr
		if shapeNode, ok := item.(ast.ShapeNode); ok {
			ex, err = t.transformShapeNodeWithExpectedType(&shapeNode, &elemType, nil)
		} else {
			ex, err = t.transformExpression(item)
		}
		if err != nil {
			return nil, err
		}
		elts = append(elts, ex)
	}
	litType := &goast.ArrayType{Elt: eltGo}
	if expectedArrayType != nil && expectedArrayType.ArrayLen != nil {
		litType.Len = &goast.BasicLit{Kind: token.INT, Value: strconv.FormatInt(*expectedArrayType.ArrayLen, 10)}
	}
	return &goast.CompositeLit{
		Type: litType,
		Elts: elts,
	}, nil
}

// transformGoBoundDottedMethodCall lowers cmd.ProcessState.ExitCode() when the parser folded
// the selector chain into a single FunctionCall identifier.
func (t *Transformer) transformGoBoundDottedMethodCall(e ast.FunctionCallNode) (goast.Expr, bool, error) {
	parts := strings.Split(string(e.Function.ID), ".")
	if len(parts) < 3 || t == nil || t.TypeChecker == nil {
		return nil, false, nil
	}
	if t.TypeChecker.GoTypeForVariable(ast.Identifier(parts[0])) == nil {
		return nil, false, nil
	}
	recv := goast.Expr(goast.NewIdent(parts[0]))
	for _, field := range parts[1 : len(parts)-1] {
		recv = &goast.SelectorExpr{X: recv, Sel: goast.NewIdent(field)}
	}
	method := parts[len(parts)-1]
	args, err := t.transformFunctionCallArgs(e.Function.ID, e.Arguments)
	if err != nil {
		return nil, true, err
	}
	return &goast.CallExpr{
		Fun:      &goast.SelectorExpr{X: recv, Sel: goast.NewIdent(method)},
		Args:     args.exprs,
		Ellipsis: args.ellipsis,
	}, true, nil
}

func (t *Transformer) wrapCallFunWithTypeArgs(fun goast.Expr, typeArgs []ast.TypeNode) (goast.Expr, error) {
	if len(typeArgs) == 0 {
		return fun, nil
	}
	indices := make([]goast.Expr, len(typeArgs))
	for i, ta := range typeArgs {
		gt, err := t.transformType(ta)
		if err != nil {
			return nil, err
		}
		indices[i] = gt
	}
	return &goast.IndexListExpr{X: fun, Indices: indices}, nil
}
