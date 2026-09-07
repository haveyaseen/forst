// Package generators provides code generation for Go.
package generators

import (
	"bytes"
	"fmt"
	goast "go/ast"
	"go/format"
	"go/token"
	"sort"
	"strings"
)

var formatGoNode = format.Node

// GenerateGoCode generates Go code from a Go AST with consistent ordering
func GenerateGoCode(goFile *goast.File) (string, error) {
	var buf bytes.Buffer
	fset := token.NewFileSet()

	// Sort while imports are still separate one-spec decls (SortImports needs
	// real positions for parenthesized blocks). Then merge into one import ().
	goast.SortImports(fset, goFile)
	sortDeclarations(goFile)
	sortStructFields(goFile)
	sortMergedImportSpecs(goFile)

	if err := formatGoNode(&buf, fset, goFile); err != nil {
		return "", fmt.Errorf("failed to format Go code: %w", err)
	}
	return ensureBlankLinesBetweenTopLevelDecls(buf.String()), nil
}

// ensureBlankLinesBetweenTopLevelDecls inserts blank lines between consecutive
// top-level declarations (types, funcs, doc comments). go/format omits them when
// FileSet positions have no source gaps (typical for synthetic ASTs).
func ensureBlankLinesBetweenTopLevelDecls(src string) string {
	lines := strings.Split(src, "\n")
	if len(lines) < 2 {
		return src
	}
	out := make([]string, 0, len(lines)+8)
	for i, line := range lines {
		out = append(out, line)
		if i+1 >= len(lines) {
			break
		}
		next := lines[i+1]
		if next == "" {
			continue
		}
		if isTopLevelCloseLine(line) && isTopLevelStartLine(next) {
			out = append(out, "")
		}
	}
	return strings.Join(out, "\n")
}

func isTopLevelCloseLine(line string) bool {
	return line == "}" || line == ")"
}

func isTopLevelStartLine(line string) bool {
	switch {
	case strings.HasPrefix(line, "func "),
		strings.HasPrefix(line, "type "),
		strings.HasPrefix(line, "const "),
		strings.HasPrefix(line, "var "),
		strings.HasPrefix(line, "//"):
		return true
	default:
		return false
	}
}

// sortDeclarations sorts declarations in a Go file for consistent ordering
func sortDeclarations(file *goast.File) {
	// Group declarations by type
	var imports, types, funcs, vars, consts []goast.Decl

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *goast.GenDecl:
			switch d.Tok {
			case token.IMPORT:
				imports = append(imports, d)
			case token.TYPE:
				types = append(types, d)
			case token.VAR:
				vars = append(vars, d)
			case token.CONST:
				consts = append(consts, d)
			}
		case *goast.FuncDecl:
			funcs = append(funcs, d)
		}
	}

	imports = mergeImportDecls(imports)

	// Sort each group by name
	sortDeclsByName := func(decls []goast.Decl) {
		sort.Slice(decls, func(i, j int) bool {
			return getDeclName(decls[i]) < getDeclName(decls[j])
		})
	}

	sortDeclsByName(types)
	sortDeclsByName(funcs)
	sortDeclsByName(vars)
	sortDeclsByName(consts)

	// Reassemble declarations in consistent order
	file.Decls = make([]goast.Decl, 0, len(file.Decls))
	file.Decls = append(file.Decls, imports...)
	file.Decls = append(file.Decls, consts...)
	file.Decls = append(file.Decls, vars...)
	file.Decls = append(file.Decls, types...)
	file.Decls = append(file.Decls, funcs...)

	// Keep File.Imports in sync with merged import specs for SortImports/tools.
	file.Imports = nil
	for _, decl := range imports {
		gd, ok := decl.(*goast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		for _, spec := range gd.Specs {
			if is, ok := spec.(*goast.ImportSpec); ok {
				file.Imports = append(file.Imports, is)
			}
		}
	}
}

// mergeImportDecls collapses one-or-many IMPORT GenDecls into a single decl.
// Two or more specs use a parenthesized import block (gofmt-stable).
func mergeImportDecls(imports []goast.Decl) []goast.Decl {
	if len(imports) == 0 {
		return imports
	}
	var specs []goast.Spec
	seen := map[string]bool{}
	for _, decl := range imports {
		gd, ok := decl.(*goast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		for _, spec := range gd.Specs {
			is, ok := spec.(*goast.ImportSpec)
			if !ok || is.Path == nil {
				continue
			}
			key := is.Path.Value
			if is.Name != nil {
				key = is.Name.Name + "\x00" + key
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			specs = append(specs, is)
		}
	}
	if len(specs) == 0 {
		return nil
	}
	gd := &goast.GenDecl{Tok: token.IMPORT, Specs: specs}
	if len(specs) > 1 {
		gd.Lparen = 1
		gd.Rparen = 1
	}
	return []goast.Decl{gd}
}

// sortMergedImportSpecs orders specs inside the merged import block by path
// (same primary key as go/ast.SortImports), without needing FileSet positions.
func sortMergedImportSpecs(file *goast.File) {
	for _, decl := range file.Decls {
		gd, ok := decl.(*goast.GenDecl)
		if !ok || gd.Tok != token.IMPORT || len(gd.Specs) < 2 {
			continue
		}
		sort.SliceStable(gd.Specs, func(i, j int) bool {
			return importSpecSortKey(gd.Specs[i]) < importSpecSortKey(gd.Specs[j])
		})
		file.Imports = nil
		for _, spec := range gd.Specs {
			if is, ok := spec.(*goast.ImportSpec); ok {
				file.Imports = append(file.Imports, is)
			}
		}
		return
	}
}

func importSpecSortKey(spec goast.Spec) string {
	is, ok := spec.(*goast.ImportSpec)
	if !ok || is.Path == nil {
		return ""
	}
	return is.Path.Value
}

// getDeclName gets the name of a declaration for sorting
func getDeclName(decl goast.Decl) string {
	switch d := decl.(type) {
	case *goast.GenDecl:
		if len(d.Specs) > 0 {
			switch s := d.Specs[0].(type) {
			case *goast.TypeSpec:
				return s.Name.Name
			case *goast.ValueSpec:
				if len(s.Names) > 0 {
					return s.Names[0].Name
				}
			case *goast.ImportSpec:
				if s.Name != nil {
					return s.Name.Name
				}
				return s.Path.Value
			}
		}
	case *goast.FuncDecl:
		return d.Name.Name
	}
	return ""
}

// sortStructFields recursively sorts fields in all struct types
func sortStructFields(file *goast.File) {
	for _, decl := range file.Decls {
		if genDecl, ok := decl.(*goast.GenDecl); ok && genDecl.Tok == token.TYPE {
			for _, spec := range genDecl.Specs {
				if typeSpec, ok := spec.(*goast.TypeSpec); ok {
					if structType, ok := typeSpec.Type.(*goast.StructType); ok {
						sortStructTypeFields(structType)
					}
				}
			}
		}
	}
}

// sortStructTypeFields sorts fields in a struct type and recursively sorts nested structs
func sortStructTypeFields(structType *goast.StructType) {
	if structType.Fields == nil {
		return
	}

	// Sort fields by name
	sort.Slice(structType.Fields.List, func(i, j int) bool {
		// If either field has no name, sort by type
		if len(structType.Fields.List[i].Names) == 0 || len(structType.Fields.List[j].Names) == 0 {
			return fmt.Sprint(structType.Fields.List[i].Type) < fmt.Sprint(structType.Fields.List[j].Type)
		}
		return structType.Fields.List[i].Names[0].Name < structType.Fields.List[j].Names[0].Name
	})

	// Recursively sort nested structs
	for _, field := range structType.Fields.List {
		if field.Type != nil {
			switch t := field.Type.(type) {
			case *goast.StructType:
				sortStructTypeFields(t)
			case *goast.ArrayType:
				if structType, ok := t.Elt.(*goast.StructType); ok {
					sortStructTypeFields(structType)
				}
			case *goast.MapType:
				if structType, ok := t.Value.(*goast.StructType); ok {
					sortStructTypeFields(structType)
				}
			}
		}
	}
}
