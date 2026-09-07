package transformergo

import (
	"fmt"
	"strconv"

	"forst/internal/ast"
	"forst/internal/typechecker"
	goast "go/ast"
	"go/token"
)

func (t *Transformer) transformForstSiblingQualifiedType(typeIdent ast.TypeIdent) (goast.Expr, bool) {
	if t.TypeChecker == nil {
		return nil, false
	}
	if _, ok := t.TypeChecker.ResolveForstSiblingTypeDef(typeIdent); !ok {
		return nil, false
	}
	importLocal, typeName, ok := typechecker.ParseForstSiblingTypeRef(typeIdent)
	if !ok {
		return nil, false
	}
	return &goast.SelectorExpr{
		X:   goast.NewIdent(importLocal),
		Sel: goast.NewIdent(typeName),
	}, true
}

func (t *Transformer) transformGoImportQualifiedType(typeIdent ast.TypeIdent) (goast.Expr, bool) {
	if t.TypeChecker == nil {
		return nil, false
	}
	importLocal, typeName, ok := typechecker.ParseForstSiblingTypeRef(typeIdent)
	if !ok {
		return nil, false
	}
	if _, ok := t.TypeChecker.ResolveForstSiblingTypeDef(typeIdent); ok {
		return nil, false
	}
	if t.TypeChecker.GoPackageForImportLocal(importLocal) == nil {
		return nil, false
	}
	return &goast.SelectorExpr{
		X:   goast.NewIdent(importLocal),
		Sel: goast.NewIdent(typeName),
	}, true
}

// transformType converts a Forst type node to a Go type declaration
func (t *Transformer) transformType(n ast.TypeNode) (goast.Expr, error) {
	if n.Ident == "" {
		return nil, fmt.Errorf("TypeNode is missing an identifier: %+v", n)
	}
	switch n.Ident {
	case ast.TypeAssertion:
		ident, err := t.TypeChecker.LookupAssertionType(n.Assertion)
		if err != nil {
			return nil, fmt.Errorf("failed to lookup assertion type: %s", err)
		}
		return goast.NewIdent(string(ident.Ident)), nil
	case ast.TypeImplicit:
		return nil, fmt.Errorf("TypeImplicit is not a valid Go type")
	case ast.TypeObject:
		return nil, fmt.Errorf("TypeObject should not be used as a Go type")
	case ast.TypePointer:
		if len(n.TypeParams) == 0 {
			return nil, fmt.Errorf("pointer type must have a base type parameter")
		}
		if ast.IsTestingTParamType(n) {
			return &goast.StarExpr{
				X: &goast.SelectorExpr{
					X:   goast.NewIdent("testing"),
					Sel: goast.NewIdent("T"),
				},
			}, nil
		}
		baseType, err := t.transformType(n.TypeParams[0])
		if err != nil {
			return nil, fmt.Errorf("failed to transform pointer base type: %s", err)
		}
		return &goast.StarExpr{X: baseType}, nil
	case ast.TypeArray:
		if len(n.TypeParams) < 1 {
			return nil, fmt.Errorf("array type must have element type parameter")
		}
		elt, err := t.transformType(n.TypeParams[0])
		if err != nil {
			return nil, err
		}
		arr := &goast.ArrayType{Elt: elt}
		if n.ArrayLen != nil {
			arr.Len = &goast.BasicLit{Kind: token.INT, Value: strconv.FormatInt(*n.ArrayLen, 10)}
		}
		return arr, nil
	case ast.TypeMap:
		if len(n.TypeParams) < 2 {
			return nil, fmt.Errorf("map type must have key and value type parameters")
		}
		keyT, err := t.transformType(n.TypeParams[0])
		if err != nil {
			return nil, err
		}
		valT, err := t.transformType(n.TypeParams[1])
		if err != nil {
			return nil, err
		}
		return &goast.MapType{Key: keyT, Value: valT}, nil
	case ast.TypeChannel:
		if len(n.TypeParams) < 1 {
			return nil, fmt.Errorf("channel type must have element type parameter")
		}
		elt, err := t.transformType(n.TypeParams[0])
		if err != nil {
			return nil, err
		}
		return &goast.ChanType{
			Dir:   goast.SEND | goast.RECV,
			Value: elt,
		}, nil
	case ast.TypeIdent("Seq"):
		if len(n.TypeParams) < 1 {
			return nil, fmt.Errorf("seq type must have element type parameter")
		}
		_, seqIdent, _, err := t.ensureForstNodeSeqTypes(ast.NewResultType(
			ast.TypeNode{Ident: "Seq", TypeParams: []ast.TypeNode{n.TypeParams[0]}},
			ast.NewBuiltinType(ast.TypeError),
		))
		if err != nil {
			return nil, err
		}
		return &goast.StarExpr{X: seqIdent}, nil
	case ast.TypeResult:
		return nil, fmt.Errorf("result types are expanded at function boundaries; use transformTypes, not transformType, or transformResultAsStructFieldGoType for struct fields")
	case ast.TypeTuple:
		return nil, fmt.Errorf("tuple types are expanded at function boundaries; use transformTypes, not transformType")
	case ast.TypeUnion:
		if t.TypeChecker.IsErrorKindedType(n) {
			var r goast.Expr = goast.NewIdent("error")
			return r, nil
		}
		var r goast.Expr = goast.NewIdent("any")
		return r, nil
	case ast.TypeIntersection:
		var r goast.Expr = goast.NewIdent("any")
		return r, nil
	case ast.TypeFunc:
		return t.transformFunctionType(n)
	default:
		if n.IsTypeParam() {
			return goast.NewIdent(string(n.Ident)), nil
		}
		if sel, ok := t.transformGoImportQualifiedType(n.Ident); ok {
			return sel, nil
		}
		if sel, ok := t.transformForstSiblingQualifiedType(n.Ident); ok {
			return sel, nil
		}
		// Always use the unified type aliasing function from the typechecker for all non-builtin, non-special types
		name, err := t.TypeChecker.GetAliasedTypeName(n, typechecker.GetAliasedTypeNameOptions{AllowStructuralAlias: false})
		if err != nil {
			return nil, fmt.Errorf("failed to get aliased type name: %s", err)
		}
		return goast.NewIdent(name), nil
	}
}

func (t *Transformer) transformTypes(types []ast.TypeNode) (*goast.FieldList, error) {
	var fields []*goast.Field
	for _, typ := range types {
		if typ.IsResultType() {
			if len(typ.TypeParams) != 2 {
				return nil, fmt.Errorf("result must have exactly two type parameters")
			}
			// Result(Void, F) lowers to a single error return (idiomatic Go).
			if typ.TypeParams[0].Ident != ast.TypeVoid {
				s, err := t.transformType(typ.TypeParams[0])
				if err != nil {
					return nil, fmt.Errorf("failed to transform Result success type: %w", err)
				}
				fields = append(fields, &goast.Field{Type: s})
			}
			fields = append(fields, &goast.Field{Type: goast.NewIdent("error")})
			continue
		}
		if typ.IsTupleType() {
			for _, elem := range typ.TypeParams {
				expr, err := t.transformType(elem)
				if err != nil {
					return nil, fmt.Errorf("failed to transform Tuple element: %w", err)
				}
				fields = append(fields, &goast.Field{Type: expr})
			}
			continue
		}
		expr, err := t.transformType(typ)
		if err != nil {
			return nil, fmt.Errorf("failed to transform type: %s", err)
		}
		fields = append(fields, &goast.Field{Type: expr})
	}

	return &goast.FieldList{
		List: fields,
	}, nil
}

// transformResultAsStructFieldGoType lowers Result(S, F) stored in a Forst shape field to a single
// Go struct type { V S; Err F }. Call sites must use the same layout for literals and field access
// (.V success payload, .Err failure / error slot). Only failure type Error is supported for now
// (matches Go error checks on .Err).
func (t *Transformer) transformResultAsStructFieldGoType(rt ast.TypeNode) (*goast.StructType, error) {
	if !rt.IsResultType() || len(rt.TypeParams) < 2 {
		return nil, fmt.Errorf("expected result(S, F)")
	}
	fail := rt.TypeParams[1]
	if fail.Ident != ast.TypeError {
		return nil, fmt.Errorf("result in struct fields: failure type must be Error for Go codegen (got %s)", fail.String())
	}
	s, err := t.transformType(rt.TypeParams[0])
	if err != nil {
		return nil, fmt.Errorf("result field success type: %w", err)
	}
	e, err := t.transformType(fail)
	if err != nil {
		return nil, fmt.Errorf("result field failure type: %w", err)
	}
	return &goast.StructType{
		Fields: &goast.FieldList{
			List: []*goast.Field{
				{Names: []*goast.Ident{goast.NewIdent(loweredResultValueFieldName)}, Type: s},
				{Names: []*goast.Ident{goast.NewIdent(loweredResultErrFieldName)}, Type: e},
			},
		},
	}, nil
}

func transformTypeIdent(ident ast.TypeIdent) (*goast.Ident, error) {
	switch ident {
	case ast.TypeString:
		return &goast.Ident{Name: "string"}, nil
	case ast.TypeInt:
		return &goast.Ident{Name: "int"}, nil
	case ast.TypeFloat:
		return &goast.Ident{Name: "float64"}, nil
	case ast.TypeComplex64:
		return &goast.Ident{Name: "complex64"}, nil
	case ast.TypeComplex128:
		return &goast.Ident{Name: "complex128"}, nil
	case ast.TypeIdent("byte"), ast.TypeIdent("Byte"):
		return &goast.Ident{Name: "byte"}, nil
	case ast.TypeIdent("float32"):
		return &goast.Ident{Name: "float32"}, nil
	case ast.TypeIdent("int8"):
		return &goast.Ident{Name: "int8"}, nil
	case ast.TypeIdent("int16"):
		return &goast.Ident{Name: "int16"}, nil
	case ast.TypeIdent("int32"):
		return &goast.Ident{Name: "int32"}, nil
	case ast.TypeIdent("int64"):
		return &goast.Ident{Name: "int64"}, nil
	case ast.TypeIdent("uint"):
		return &goast.Ident{Name: "uint"}, nil
	case ast.TypeIdent("uint8"):
		return &goast.Ident{Name: "uint8"}, nil
	case ast.TypeIdent("uint16"):
		return &goast.Ident{Name: "uint16"}, nil
	case ast.TypeIdent("uint32"):
		return &goast.Ident{Name: "uint32"}, nil
	case ast.TypeIdent("uint64"):
		return &goast.Ident{Name: "uint64"}, nil
	case ast.TypeIdent("uintptr"):
		return &goast.Ident{Name: "uintptr"}, nil
	case ast.TypeIdent("rune"):
		return &goast.Ident{Name: "rune"}, nil
	case ast.TypeBool:
		return &goast.Ident{Name: "bool"}, nil
	case ast.TypeVoid:
		return &goast.Ident{Name: "void"}, nil
	case ast.TypeError:
		return &goast.Ident{Name: "error"}, nil
	case ast.TypeBytes:
		return &goast.Ident{Name: "[]byte"}, nil
	case ast.TypeObject:
		return nil, fmt.Errorf("TypeObject should not be used as a Go type")
	case ast.TypeAssertion:
		return nil, fmt.Errorf("TypeAssertion should not be used as a Go type")
	case ast.TypeImplicit:
		return nil, fmt.Errorf("TypeImplicit should not be used as a Go type")
	default:
		// For user-defined types (aliases, shapes, etc.), just use the type name
		return goast.NewIdent(string(ident)), nil
	}
}
