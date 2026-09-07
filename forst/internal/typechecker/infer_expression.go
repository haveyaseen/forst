package typechecker

import (
	"fmt"
	"forst/internal/ast"

	logrus "github.com/sirupsen/logrus"
)

// Checks if a method call is valid for a given type and returns its return type
func (tc *TypeChecker) inferMethodCallType(receiver ast.Identifier, varType []ast.TypeNode, methodName string, e ast.FunctionCallNode, argTypes [][]ast.TypeNode) ([]ast.TypeNode, error) {
	tc.log.WithFields(logrus.Fields{
		"function":   "inferMethodCallType",
		"varType":    varType,
		"methodName": methodName,
		"receiver":   receiver,
	}).Tracef("inferMethodCallType")

	if len(varType) != 1 {
		callSpan := firstSetSpan(e.CallSpan, e.Function.Span)
		tc.log.WithFields(logrus.Fields{
			"function": "inferMethodCallType",
			"varType":  varType,
		}).Tracef("Method calls are only valid on single types, got %s", formatTypeList(varType))
		return nil, reportf(callSpan, "method-receiver-type",
			"method call requires a single receiver type",
			fmt.Sprintf("Method calls require exactly one receiver type, got %s.", formatTypeList(varType)),
			"call the method on a single-typed receiver")
	}

	t := varType[0]
	callSpan := firstSetSpan(e.CallSpan, e.Function.Span)
	if ret, ok, err := tc.inferResultMethodCallType(t, methodName, callSpan); ok {
		if err != nil {
			return nil, err
		}
		return ret, nil
	}

	if ret, ok, err := tc.inferGoVariableMethodCallType(receiver, methodName, e, argTypes); ok {
		if err != nil {
			return nil, err
		}
		return ret, nil
	}

	// *T method calls: lower to element type for built-in / opaque Go receivers.
	if t.Ident == ast.TypePointer && len(t.TypeParams) == 1 {
		t = t.TypeParams[0]
	}

	if ret, err := tc.checkUserTypeMethod(t, methodName, e.Arguments, callSpan); err == nil {
		return ret, nil
	} else if tc.TypeMethods != nil {
		// Only fall through when the type has no method table; otherwise surface the error.
		if methods, ok := tc.TypeMethods[t.Ident]; ok && len(methods) > 0 {
			return nil, err
		}
	}

	if ret, err := tc.checkContractShapeMethod(t, methodName, e.Arguments, callSpan); err == nil {
		return ret, nil
	}

	returnType, err := tc.CheckBuiltinMethod(t, methodName, e.Arguments)
	if err != nil {
		tc.log.WithFields(logrus.Fields{
			"function":   "inferMethodCallType",
			"varType":    varType,
			"methodName": methodName,
		}).Tracef("Error checking built-in method: %v", err)
		return nil, err
	}

	tc.log.WithFields(logrus.Fields{
		"function":   "inferMethodCallType",
		"varType":    varType,
		"methodName": methodName,
	}).Tracef("Successfully inferred method call type: %v", returnType)
	return returnType, nil
}

func (tc *TypeChecker) inferResultMethodCallType(t ast.TypeNode, methodName string, callSpan ast.SourceSpan) ([]ast.TypeNode, bool, error) {
	if !t.IsResultType() || len(t.TypeParams) < 2 {
		return nil, false, nil
	}
	switch methodName {
	case "Ok":
		return []ast.TypeNode{t.TypeParams[0]}, true, nil
	case "Err":
		return []ast.TypeNode{t.TypeParams[1]}, true, nil
	default:
		return nil, true, reportf(callSpan, "method-undefined",
			fmt.Sprintf("method `%s()` not valid on Result", methodName),
			fmt.Sprintf("Result types only support `.Ok()` and `.Err()`; `%s()` is not valid.", methodName),
			"use `.Ok()` or `.Err()`, or call a method on the success/failure payload after narrowing")
	}
}

func (tc *TypeChecker) inferGoVariableMethodCallType(receiver ast.Identifier, methodName string, e ast.FunctionCallNode, argTypes [][]ast.TypeNode) ([]ast.TypeNode, bool, error) {
	goRecv := tc.goTypeForVariableIdent(receiver)
	if goRecv == nil {
		return nil, false, nil
	}
	method := e.Function
	if method.ID == "" {
		method = ast.Ident{ID: ast.Identifier(methodName)}
	}
	ret, err := tc.checkGoMethodCall(goRecv, method, e, argTypes, true)
	if err != nil {
		return nil, true, err
	}
	tc.log.WithFields(logrus.Fields{
		"function":   "inferMethodCallType",
		"receiver":   receiver,
		"methodName": methodName,
	}).Tracef("Go method call: %v", ret)
	return ret, true, nil
}

// inferIndexExpressionAsAssignTarget types an index expression as an assignment target (m[k] = x or xs[i] = x).
// Map cells use element type V; rvalue map reads elsewhere use Result(V, Error) via inferExpressionType.
func (tc *TypeChecker) inferIndexExpressionAsAssignTarget(e ast.IndexExpressionNode) ([]ast.TypeNode, error) {
	targetTypes, err := tc.inferExpressionType(e.Target)
	if err != nil {
		return nil, err
	}
	if len(targetTypes) != 1 {
		return nil, reportf(spanIndexExpr(e), "index-target-type",
			"index target must have a single type",
			fmt.Sprintf("The indexed value has %d types; subscript needs exactly one.", len(targetTypes)),
			"bind the container to a single-typed name")
	}
	t := targetTypes[0]
	indexTypes, err := tc.inferExpressionType(e.Index)
	if err != nil {
		return nil, err
	}
	if len(indexTypes) != 1 {
		return nil, reportf(spanIndexExpr(e), "index-type",
			"index expression must have a single index type",
			"The index expression must infer to exactly one type.",
			"use an Int index for slices/arrays/strings or the map key type")
	}
	if t.Ident == ast.TypeMap && len(t.TypeParams) >= 2 {
		wantK, wantV := t.TypeParams[0], t.TypeParams[1]
		if !tc.IsTypeCompatible(indexTypes[0], wantK) {
			return nil, reportf(spanIndexExpr(e), "index-type",
				"map index key type mismatch",
				fmt.Sprintf("Map key type must be `%s`, got `%s`.", formatTypeIdentForDiag(wantK.Ident), formatTypeIdentForDiag(indexTypes[0].Ident)),
				"convert the key or change the map's key type")
		}
		tc.storeInferredType(e, []ast.TypeNode{wantV})
		return []ast.TypeNode{wantV}, nil
	}
	if t.Ident == ast.TypeString {
		if indexTypes[0].Ident != ast.TypeInt {
			return nil, reportf(spanIndexExpr(e), "index-type",
				"string index must be Int",
				fmt.Sprintf("String indexing requires Int, got `%s`.", indexTypes[0].Ident),
				"use an integer index")
		}
		elem := ast.TypeNode{Ident: ast.TypeInt}
		tc.storeInferredType(e, []ast.TypeNode{elem})
		return []ast.TypeNode{elem}, nil
	}
	if t.Ident == ast.TypeBytes {
		if indexTypes[0].Ident != ast.TypeInt {
			return nil, reportf(spanIndexExpr(e), "index-type",
				"[]byte index must be Int",
				fmt.Sprintf("`[]byte` indexing requires Int, got `%s`.", indexTypes[0].Ident),
				"use an integer index")
		}
		elem := ast.TypeNode{Ident: ast.TypeIdent("byte")}
		tc.storeInferredType(e, []ast.TypeNode{elem})
		return []ast.TypeNode{elem}, nil
	}
	if t.Ident != ast.TypeArray || len(t.TypeParams) < 1 {
		return nil, reportf(spanIndexExpr(e), "index-target-type",
			"index target must be map, slice, or array",
			fmt.Sprintf("Cannot index type `%s`; expected map, slice, array, string, or []byte.", formatTypeIdentForDiag(t.Ident)),
			"index a supported container type")
	}
	if indexTypes[0].Ident != ast.TypeInt {
		return nil, reportf(spanIndexExpr(e), "index-type",
			"slice/array index must be Int",
			fmt.Sprintf("Slice and array indexes must be Int, got `%s`.", indexTypes[0].Ident),
			"use an integer index")
	}
	elem := t.TypeParams[0]
	tc.storeInferredType(e, []ast.TypeNode{elem})
	return []ast.TypeNode{elem}, nil
}

// inferDerefExpressionAsAssignTarget types *p = x (including **pp = x).
func (tc *TypeChecker) inferDerefExpressionAsAssignTarget(e ast.DereferenceNode) ([]ast.TypeNode, error) {
	ptrTypes, err := tc.inferExpressionType(e.Value)
	if err != nil {
		return nil, err
	}
	if len(ptrTypes) != 1 || ptrTypes[0].Ident != ast.TypePointer || len(ptrTypes[0].TypeParams) != 1 {
		return nil, reportf(spanOfExpression(e.Value), "deref-assign-type",
			"dereference assignment requires a pointer on the left",
			fmt.Sprintf("Left-hand side of `*p = v` must be a pointer, got %s.", formatTypeList(ptrTypes)),
			"dereference a pointer variable or field")
	}
	elem := ptrTypes[0].TypeParams[0]
	tc.storeInferredType(e, []ast.TypeNode{elem})
	return []ast.TypeNode{elem}, nil
}

// inferExpressionTypeWithExpected infers an expression's type. For shape literals it passes the
// callee parameter type into inferShapeType so fields match the formal parameter (e.g. *String for
// `sessionId` when the parameter is AppMutation.Input(...)).
func (tc *TypeChecker) inferExpressionTypeWithExpected(expr ast.Node, expected *ast.TypeNode) ([]ast.TypeNode, error) {
	if expected == nil {
		return tc.inferExpressionType(expr)
	}
	if ret, ok, err := tc.inferShapeExpressionWithExpected(expr, expected); ok {
		return ret, err
	}
	if ret, ok, err := tc.inferArrayLiteralWithExpected(expr, expected); ok {
		return ret, err
	}
	if _, ok := expr.(ast.IndexExpressionNode); ok {
		return tc.inferExpressionType(expr)
	}
	return tc.inferExpressionType(expr)
}

func (tc *TypeChecker) inferShapeExpressionWithExpected(expr ast.Node, expected *ast.TypeNode) ([]ast.TypeNode, bool, error) {
	switch x := expr.(type) {
	case ast.ShapeNode:
		inferredType, err := tc.inferShapeType(x, expected)
		if err != nil {
			return nil, true, err
		}
		tc.storeInferredType(expr, []ast.TypeNode{inferredType})
		return []ast.TypeNode{inferredType}, true, nil
	case *ast.ShapeNode:
		inferredType, err := tc.inferShapeType(*x, expected)
		if err != nil {
			return nil, true, err
		}
		tc.storeInferredType(expr, []ast.TypeNode{inferredType})
		return []ast.TypeNode{inferredType}, true, nil
	default:
		return nil, false, nil
	}
}

func (tc *TypeChecker) inferArrayLiteralWithExpected(expr ast.Node, expected *ast.TypeNode) ([]ast.TypeNode, bool, error) {
	x, ok := expr.(ast.ArrayLiteralNode)
	if !ok {
		return nil, false, nil
	}
	if len(x.Value) == 0 && expected.Ident == ast.TypeArray && len(expected.TypeParams) == 1 {
		arr := ast.TypeNode{Ident: ast.TypeArray, TypeParams: []ast.TypeNode{expected.TypeParams[0]}}
		if expected.ArrayLen != nil {
			arr.ArrayLen = expected.ArrayLen
		}
		tc.storeInferredType(expr, []ast.TypeNode{arr})
		return []ast.TypeNode{arr}, true, nil
	}
	if expected.IsFixedArray() && len(expected.TypeParams) == 1 {
		arrSpan := ast.SourceSpan{}
		if len(x.Value) > 0 {
			arrSpan = spanOfExpression(x.Value[0])
		}
		if err := checkFixedArrayLiteralLength(*expected, len(x.Value), arrSpan); err != nil {
			return nil, true, err
		}
	}
	types, err := tc.inferExpressionType(x)
	return types, true, err
}
