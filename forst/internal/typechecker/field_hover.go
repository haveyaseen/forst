package typechecker

import (
	"strings"

	"forst/internal/ast"
	"forst/internal/hoverdoc"
)

// FieldHoverMarkdown returns markdown for a dotted field path rooted at a variable (e.g. move.state
// or move.state.status). Uses lookupFieldPath; ok is false if the path cannot be resolved.
// parentTypeForDoc is the named type whose shape defines the last field (for leading comments), when known.
//
// When dottedSpan is set, it must cover the full dotted expression (recv.field1.…) so the same
// VariableNode identity as in the AST is used; then ensure/if narrowing on the full path (e.g.
// ensure g.cells is Min(9)) appears in hover via FormatVariableOccurrenceTypeForHover.
func (tc *TypeChecker) FieldHoverMarkdown(root ast.Identifier, span ast.SourceSpan, fieldPath []string, dottedSpan ast.SourceSpan) (md string, parentTypeForDoc ast.TypeIdent, ok bool) {
	if tc == nil || len(fieldPath) == 0 {
		return "", "", false
	}
	var baseTypes []ast.TypeNode
	var have bool
	if span.IsSet() {
		vn := ast.VariableNode{Ident: ast.Ident{ID: root, Span: span}}
		baseTypes, have = tc.InferredTypesForVariableNode(vn)
	}
	if !have || len(baseTypes) == 0 {
		baseTypes, have = tc.InferredTypesForVariableIdentifier(root)
	}
	if !have || len(baseTypes) == 0 {
		return "", "", false
	}
	last := fieldPath[len(fieldPath)-1]
	displayPath := string(root) + "." + strings.Join(fieldPath, ".")
	if dottedSpan.IsSet() {
		fullID := ast.Identifier(displayPath)
		vn := ast.VariableNode{Ident: ast.Ident{ID: fullID, Span: dottedSpan}}
		if types, have := tc.InferredTypesForVariableNode(vn); have && len(types) > 0 {
			typeStr := tc.FormatVariableOccurrenceTypeForHover(vn, types)
			if goStr, ok := tc.goTypeDisplayStringForVariablePath(fullID); ok {
				typeStr = goStr
			}
			var b strings.Builder
			b.WriteString("**`")
			b.WriteString(last)
			b.WriteString("`** (field)\n\n")
			b.WriteString(hoverdoc.ForstBlock(displayPath + ": " + typeStr))
			pid := ast.TypeIdent("")
			for _, bt := range baseTypes {
				if p, ok := tc.ParentTypeIdentForFieldPath(bt, fieldPath); ok {
					pid = p
					break
				}
			}
			if pid != "" {
				if f, ok := tc.ShapeFieldFromTypeDef(pid, last); ok && f.Tag != "" {
					b.WriteString("\n\n")
					b.WriteString("**Tag:** `")
					b.WriteString(f.Tag)
					b.WriteString("`")
				}
			}
			return b.String(), pid, true
		}
	}
	for _, bt := range baseTypes {
		var resolved ast.TypeNode
		var err error
		goRoot := tc.goTypeForVariableIdent(root)
		if goRoot != nil {
			resolved, err = tc.lookupFieldPathFromGoType(goRoot, fieldPath)
		}
		if err != nil || goRoot == nil {
			resolved, err = tc.lookupFieldPath(bt, fieldPath)
		}
		if err != nil {
			continue
		}
		typeStr := tc.FormatTypeNodeDisplay(resolved)
		if goStr, ok := tc.goTypeDisplayStringForVariablePath(ast.Identifier(displayPath)); ok {
			typeStr = goStr
		}
		if dottedSpan.IsSet() {
			fullID := ast.Identifier(string(root) + "." + strings.Join(fieldPath, "."))
			vn := ast.VariableNode{Ident: ast.Ident{ID: fullID, Span: dottedSpan}}
			if types, have := tc.InferredTypesForVariableNode(vn); have && len(types) > 0 {
				typeStr = tc.FormatVariableOccurrenceTypeForHover(vn, types)
			}
		}
		pid, _ := tc.ParentTypeIdentForFieldPath(bt, fieldPath)
		var b strings.Builder
		b.WriteString("**`")
		b.WriteString(last)
		b.WriteString("`** (field)\n\n")
		b.WriteString(hoverdoc.ForstBlock(displayPath + ": " + typeStr))
		if pid != "" {
			if f, ok := tc.ShapeFieldFromTypeDef(pid, last); ok && f.Tag != "" {
				b.WriteString("\n\n")
				b.WriteString("**Tag:** `")
				b.WriteString(f.Tag)
				b.WriteString("`")
			}
		}
		return b.String(), pid, true
	}
	return "", "", false
}

// ParentTypeIdentForFieldPath returns the named type whose shape defines the last segment of
// fieldPath (for documentation lookup). For move.state.status with path [state, status], the parent
// of "status" is the type after resolving [state] (e.g. GameState).
func (tc *TypeChecker) ParentTypeIdentForFieldPath(base ast.TypeNode, fieldPath []string) (ast.TypeIdent, bool) {
	if tc == nil || len(fieldPath) == 0 {
		return "", false
	}
	parentPath := fieldPath[:len(fieldPath)-1]
	var parentT ast.TypeNode
	var err error
	if len(parentPath) == 0 {
		parentT = base
	} else {
		parentT, err = tc.lookupFieldPath(base, parentPath)
		if err != nil {
			return "", false
		}
	}
	p := tc.GetMostSpecificNonHashAlias(parentT)
	if p.Ident == "" {
		return "", false
	}
	return ast.TypeIdent(p.Ident), true
}

// ShapeFieldFromTypeDef returns the shape field node for a named type's field, if the definition is a shape.
func (tc *TypeChecker) ShapeFieldFromTypeDef(typeName ast.TypeIdent, fieldName string) (ast.ShapeFieldNode, bool) {
	if tc == nil {
		return ast.ShapeFieldNode{}, false
	}
	def, ok := tc.Defs[typeName]
	if !ok {
		return ast.ShapeFieldNode{}, false
	}
	td, ok := def.(ast.TypeDefNode)
	if !ok {
		return ast.ShapeFieldNode{}, false
	}
	shapePtr, ok := ast.PayloadShape(td.Expr)
	if !ok {
		return ast.ShapeFieldNode{}, false
	}
	f, ok := shapePtr.Fields[fieldName]
	return f, ok
}
