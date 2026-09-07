package generators

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"io"
	"strings"
	"testing"
)

func TestGenerateGoCode_blankLineAfterPackageMatchesGofmt(t *testing.T) {
	t.Parallel()
	f := &ast.File{
		Name: ast.NewIdent("main"),
		Decls: []ast.Decl{
			&ast.GenDecl{
				Tok: token.TYPE,
				Doc: &ast.CommentGroup{List: []*ast.Comment{{Text: "// EchoRequest: TypeDefShapeExpr({message: String})"}}},
				Specs: []ast.Spec{
					&ast.TypeSpec{
						Name: ast.NewIdent("EchoRequest"),
						Type: &ast.StructType{Fields: &ast.FieldList{
							List: []*ast.Field{
								{Names: []*ast.Ident{ast.NewIdent("message")}, Type: ast.NewIdent("string")},
							},
						}},
					},
				},
			},
			&ast.FuncDecl{
				Name: ast.NewIdent("Echo"),
				Type: &ast.FuncType{
					Params: &ast.FieldList{List: []*ast.Field{
						{Names: []*ast.Ident{ast.NewIdent("input")}, Type: ast.NewIdent("EchoRequest")},
					}},
					Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("EchoRequest")}}},
				},
				Body: &ast.BlockStmt{},
			},
		},
	}
	out, err := GenerateGoCode(f)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "package main\n\n// EchoRequest:") {
		t.Fatalf("expected blank line after package clause before doc comment, got:\n%s", out)
	}
	formatted, err := format.Source([]byte(out))
	if err != nil {
		t.Fatalf("format.Source: %v", err)
	}
	if string(formatted) != out {
		t.Fatalf("gofmt changed emit:\n--- emit ---\n%s\n--- gofmt ---\n%s", out, formatted)
	}
}

func TestGenerateGoCode_sortsFuncDeclsByName(t *testing.T) {
	f := &ast.File{
		Name: ast.NewIdent("p"),
		Decls: []ast.Decl{
			&ast.FuncDecl{
				Name: ast.NewIdent("Zed"),
				Type: &ast.FuncType{Params: &ast.FieldList{}},
				Body: &ast.BlockStmt{},
			},
			&ast.FuncDecl{
				Name: ast.NewIdent("Auth"),
				Type: &ast.FuncType{Params: &ast.FieldList{}},
				Body: &ast.BlockStmt{},
			},
		},
	}

	out, err := GenerateGoCode(f)
	if err != nil {
		t.Fatal(err)
	}
	iZed := strings.Index(out, "func Zed")
	iAuth := strings.Index(out, "func Auth")
	if iAuth == -1 || iZed == -1 {
		t.Fatalf("expected both funcs in output:\n%s", out)
	}
	if iAuth >= iZed {
		t.Fatalf("expected Auth before Zed, got:\n%s", out)
	}
	if !strings.Contains(out, "}\n\nfunc Zed") && !strings.Contains(out, "}\n\nfunc Auth") {
		// Auth is first; blank line before Zed
		if !strings.Contains(out, "func Auth") || !strings.Contains(out, "\n\nfunc Zed") {
			t.Fatalf("expected blank line between funcs, got:\n%s", out)
		}
	}
}

func TestGenerateGoCode_blankLineBetweenFuncs(t *testing.T) {
	t.Parallel()
	f := &ast.File{
		Name: ast.NewIdent("main"),
		Decls: []ast.Decl{
			&ast.FuncDecl{
				Recv: &ast.FieldList{List: []*ast.Field{
					{Names: []*ast.Ident{ast.NewIdent("e")}, Type: ast.NewIdent("E")},
				}},
				Name: ast.NewIdent("Error"),
				Type: &ast.FuncType{
					Params:  &ast.FieldList{},
					Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("string")}}},
				},
				Body: &ast.BlockStmt{List: []ast.Stmt{
					&ast.ReturnStmt{Results: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"error"`}}},
				}},
			},
			&ast.FuncDecl{
				Recv: &ast.FieldList{List: []*ast.Field{
					{Names: []*ast.Ident{ast.NewIdent("e")}, Type: ast.NewIdent("E")},
				}},
				Name: ast.NewIdent("ForstErrorTag"),
				Type: &ast.FuncType{
					Params:  &ast.FieldList{},
					Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("string")}}},
				},
				Body: &ast.BlockStmt{List: []ast.Stmt{
					&ast.ReturnStmt{Results: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"tag"`}}},
				}},
			},
			&ast.FuncDecl{
				Name: ast.NewIdent("main"),
				Type: &ast.FuncType{Params: &ast.FieldList{}},
				Body: &ast.BlockStmt{},
			},
		},
	}
	out, err := GenerateGoCode(f)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"}\n\nfunc (e E) ForstErrorTag()",
		"}\n\nfunc main()",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing blank line before next func (%q), got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "}\n\n\nfunc") {
		t.Fatalf("unexpected triple newline between funcs:\n%s", out)
	}
}

func TestGenerateGoCode_mergedImportsAndBlankLinesBetweenTypes(t *testing.T) {
	t.Parallel()
	f := &ast.File{
		Name: ast.NewIdent("main"),
		Decls: []ast.Decl{
			&ast.GenDecl{
				Tok: token.IMPORT,
				Specs: []ast.Spec{
					&ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: `"fmt"`}},
				},
			},
			&ast.GenDecl{
				Tok: token.IMPORT,
				Specs: []ast.Spec{
					&ast.ImportSpec{
						Name: ast.NewIdent("os"),
						Path: &ast.BasicLit{Kind: token.STRING, Value: `"os"`},
					},
				},
			},
			&ast.GenDecl{
				Tok: token.TYPE,
				Doc: &ast.CommentGroup{List: []*ast.Comment{{Text: "// NeedFailed: err"}}},
				Specs: []ast.Spec{
					&ast.TypeSpec{
						Name: ast.NewIdent("NeedFailed"),
						Type: &ast.StructType{Fields: &ast.FieldList{
							List: []*ast.Field{
								{Names: []*ast.Ident{ast.NewIdent("reason")}, Type: ast.NewIdent("string")},
							},
						}},
					},
				},
			},
			&ast.GenDecl{
				Tok: token.TYPE,
				Doc: &ast.CommentGroup{List: []*ast.Comment{{Text: "// T_hash: shape"}}},
				Specs: []ast.Spec{
					&ast.TypeSpec{
						Name: ast.NewIdent("T_hash"),
						Type: &ast.StructType{Fields: &ast.FieldList{}},
					},
				},
			},
			&ast.FuncDecl{
				Name: ast.NewIdent("main"),
				Type: &ast.FuncType{Params: &ast.FieldList{}},
				Body: &ast.BlockStmt{},
			},
		},
	}
	out, err := GenerateGoCode(f)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "import (\n") {
		t.Fatalf("expected parenthesized import group, got:\n%s", out)
	}
	if strings.Contains(out, "import \"fmt\"\nimport ") {
		t.Fatalf("imports must be merged into one group, got:\n%s", out)
	}
	if !strings.Contains(out, "\"fmt\"") || !strings.Contains(out, `"os"`) {
		t.Fatalf("missing import paths, got:\n%s", out)
	}
	if !strings.Contains(out, "}\n\n// T_hash:") && !strings.Contains(out, "}\n\n// T_hash") {
		t.Fatalf("expected blank line between types, got:\n%s", out)
	}
	if !strings.Contains(out, ")\n\n// NeedFailed") && !strings.Contains(out, ")\n\n// NeedFailed:") {
		t.Fatalf("expected blank line after import group before first type, got:\n%s", out)
	}

	// gofmt must be a no-op on emitted source.
	formatted, err := format.Source([]byte(out))
	if err != nil {
		t.Fatalf("format.Source: %v\n%s", err, out)
	}
	if string(formatted) != out {
		t.Fatalf("gofmt changed emit:\n--- emit ---\n%s\n--- gofmt ---\n%s", out, formatted)
	}
}

func TestEnsureBlankLinesBetweenTopLevelDecls_idempotent(t *testing.T) {
	t.Parallel()
	in := "package p\n\nfunc a() {\n}\n\nfunc b() {\n}\n"
	once := ensureBlankLinesBetweenTopLevelDecls(in)
	twice := ensureBlankLinesBetweenTopLevelDecls(once)
	if once != twice {
		t.Fatalf("not idempotent:\n once=%q\n twice=%q", once, twice)
	}
	if once != in {
		t.Fatalf("should leave already-spaced funcs alone, got:\n%s", once)
	}
}

func TestGenerateGoCode_sortsStructFieldsByName(t *testing.T) {
	f := &ast.File{
		Name: ast.NewIdent("p"),
		Decls: []ast.Decl{
			&ast.GenDecl{
				Tok: token.TYPE,
				Specs: []ast.Spec{
					&ast.TypeSpec{
						Name: ast.NewIdent("S"),
						Type: &ast.StructType{
							Fields: &ast.FieldList{
								List: []*ast.Field{
									{Names: []*ast.Ident{ast.NewIdent("zebra")}, Type: ast.NewIdent("int")},
									{Names: []*ast.Ident{ast.NewIdent("apple")}, Type: ast.NewIdent("string")},
								},
							},
						},
					},
				},
			},
		},
	}

	out, err := GenerateGoCode(f)
	if err != nil {
		t.Fatal(err)
	}
	iApple := strings.Index(out, "apple")
	iZebra := strings.Index(out, "zebra")
	if iApple == -1 || iZebra == -1 {
		t.Fatalf("expected both fields in output:\n%s", out)
	}
	if iApple >= iZebra {
		t.Fatalf("expected apple before zebra in struct, got:\n%s", out)
	}
}

func TestGenerateGoCode_declarationOrderGroups(t *testing.T) {
	// Unsorted input: type, func, const — output order should be const, var, type, func.
	f := &ast.File{
		Name: ast.NewIdent("p"),
		Decls: []ast.Decl{
			&ast.GenDecl{
				Tok: token.TYPE,
				Specs: []ast.Spec{
					&ast.TypeSpec{Name: ast.NewIdent("T"), Type: ast.NewIdent("int")},
				},
			},
			&ast.FuncDecl{
				Name: ast.NewIdent("F"),
				Type: &ast.FuncType{Params: &ast.FieldList{}},
				Body: &ast.BlockStmt{},
			},
			&ast.GenDecl{
				Tok: token.CONST,
				Specs: []ast.Spec{
					&ast.ValueSpec{
						Names:  []*ast.Ident{ast.NewIdent("C")},
						Values: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "1"}},
					},
				},
			},
		},
	}

	out, err := GenerateGoCode(f)
	if err != nil {
		t.Fatal(err)
	}
	iConst := strings.Index(out, "const")
	iType := strings.Index(out, "type T")
	iFunc := strings.Index(out, "func F")
	if iConst == -1 || iType == -1 || iFunc == -1 {
		t.Fatalf("missing expected decls:\n%s", out)
	}
	if iConst >= iType || iType >= iFunc {
		t.Fatalf("expected const then type then func, got:\n%s", out)
	}
}

func TestGenerateGoCode_sortsVarDeclsAndImports(t *testing.T) {
	t.Parallel()
	f := &ast.File{
		Name: ast.NewIdent("p"),
		Decls: []ast.Decl{
			&ast.GenDecl{
				Tok: token.VAR,
				Specs: []ast.Spec{
					&ast.ValueSpec{
						Names:  []*ast.Ident{ast.NewIdent("zebra")},
						Values: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "1"}},
					},
				},
			},
			&ast.GenDecl{
				Tok: token.VAR,
				Specs: []ast.Spec{
					&ast.ValueSpec{
						Names:  []*ast.Ident{ast.NewIdent("apple")},
						Values: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "2"}},
					},
				},
			},
			&ast.GenDecl{
				Tok: token.IMPORT,
				Specs: []ast.Spec{
					&ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: `"fmt"`}},
				},
			},
		},
	}
	out, err := GenerateGoCode(f)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `import "fmt"`) {
		t.Fatalf("missing import:\n%s", out)
	}
	iApple := strings.Index(out, "apple")
	iZebra := strings.Index(out, "zebra")
	if iApple == -1 || iZebra == -1 || iApple >= iZebra {
		t.Fatalf("expected sorted var decl groups:\n%s", out)
	}
}

func TestGenerateGoCode_sortsNestedStructInStructField(t *testing.T) {
	t.Parallel()
	f := &ast.File{
		Name: ast.NewIdent("p"),
		Decls: []ast.Decl{
			&ast.GenDecl{
				Tok: token.TYPE,
				Specs: []ast.Spec{
					&ast.TypeSpec{
						Name: ast.NewIdent("Outer"),
						Type: &ast.StructType{
							Fields: &ast.FieldList{
								List: []*ast.Field{
									{
										Names: []*ast.Ident{ast.NewIdent("items")},
										Type: &ast.ArrayType{
											Elt: &ast.StructType{
												Fields: &ast.FieldList{
													List: []*ast.Field{
														{Names: []*ast.Ident{ast.NewIdent("z")}, Type: ast.NewIdent("int")},
														{Names: []*ast.Ident{ast.NewIdent("a")}, Type: ast.NewIdent("int")},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	out, err := GenerateGoCode(f)
	if err != nil {
		t.Fatal(err)
	}
	iA := strings.Index(out, "a int")
	iZ := strings.Index(out, "z int")
	if iA == -1 || iZ == -1 || iA >= iZ {
		t.Fatalf("expected nested struct fields sorted:\n%s", out)
	}
}

func TestSortStructTypeFields_nilFieldsNoPanic(t *testing.T) {
	t.Parallel()
	sortStructTypeFields(&ast.StructType{})
}

func TestSortStructTypeFields_mapValueNestedStruct(t *testing.T) {
	t.Parallel()
	outer := &ast.StructType{
		Fields: &ast.FieldList{
			List: []*ast.Field{
				{
					Names: []*ast.Ident{ast.NewIdent("m")},
					Type: &ast.MapType{
						Key: ast.NewIdent("string"),
						Value: &ast.StructType{
							Fields: &ast.FieldList{
								List: []*ast.Field{
									{Names: []*ast.Ident{ast.NewIdent("z")}, Type: ast.NewIdent("int")},
									{Names: []*ast.Ident{ast.NewIdent("a")}, Type: ast.NewIdent("int")},
								},
							},
						},
					},
				},
			},
		},
	}
	sortStructTypeFields(outer)
	names := []string{
		outer.Fields.List[0].Type.(*ast.MapType).Value.(*ast.StructType).Fields.List[0].Names[0].Name,
		outer.Fields.List[0].Type.(*ast.MapType).Value.(*ast.StructType).Fields.List[1].Names[0].Name,
	}
	if names[0] != "a" || names[1] != "z" {
		t.Fatalf("got field order %#v", names)
	}
}

func TestSortStructTypeFields_anonymousFieldsSortByType(t *testing.T) {
	t.Parallel()
	st := &ast.StructType{
		Fields: &ast.FieldList{
			List: []*ast.Field{
				{Type: ast.NewIdent("string")},
				{Type: ast.NewIdent("int")},
			},
		},
	}
	sortStructTypeFields(st)
	if fmt.Sprint(st.Fields.List[0].Type) > fmt.Sprint(st.Fields.List[1].Type) {
		t.Fatalf("expected anonymous fields sorted by type string")
	}
}

func TestSortStructTypeFields_arrayElementNestedStruct(t *testing.T) {
	t.Parallel()
	outer := &ast.StructType{
		Fields: &ast.FieldList{
			List: []*ast.Field{
				{
					Names: []*ast.Ident{ast.NewIdent("xs")},
					Type: &ast.ArrayType{
						Elt: &ast.StructType{
							Fields: &ast.FieldList{
								List: []*ast.Field{
									{Names: []*ast.Ident{ast.NewIdent("z")}, Type: ast.NewIdent("int")},
									{Names: []*ast.Ident{ast.NewIdent("a")}, Type: ast.NewIdent("int")},
								},
							},
						},
					},
				},
			},
		},
	}
	sortStructTypeFields(outer)
	inner := outer.Fields.List[0].Type.(*ast.ArrayType).Elt.(*ast.StructType)
	if inner.Fields.List[0].Names[0].Name != "a" || inner.Fields.List[1].Names[0].Name != "z" {
		t.Fatalf("got %#v", inner.Fields.List)
	}
}

func TestSortStructTypeFields_directNestedStructField(t *testing.T) {
	t.Parallel()
	outer := &ast.StructType{
		Fields: &ast.FieldList{
			List: []*ast.Field{
				{
					Names: []*ast.Ident{ast.NewIdent("inner")},
					Type: &ast.StructType{
						Fields: &ast.FieldList{
							List: []*ast.Field{
								{Names: []*ast.Ident{ast.NewIdent("z")}, Type: ast.NewIdent("int")},
								{Names: []*ast.Ident{ast.NewIdent("a")}, Type: ast.NewIdent("int")},
							},
						},
					},
				},
			},
		},
	}
	sortStructTypeFields(outer)
	inner := outer.Fields.List[0].Type.(*ast.StructType)
	if inner.Fields.List[0].Names[0].Name != "a" {
		t.Fatalf("got %#v", inner.Fields.List)
	}
}

func TestGenerateGoCode_formatError(t *testing.T) {
	// Serial: swaps package-level formatGoNode; must finish before t.Parallel() tests run.
	orig := formatGoNode
	formatGoNode = func(io.Writer, *token.FileSet, interface{}) error {
		return fmt.Errorf("format failed")
	}
	t.Cleanup(func() { formatGoNode = orig })
	f := &ast.File{Name: ast.NewIdent("main")}
	if _, err := GenerateGoCode(f); err == nil {
		t.Fatal("expected format error")
	}
}
