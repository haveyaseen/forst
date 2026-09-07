package typechecker

import (
	"strings"

	"forst/internal/ast"

	"go/types"
)

// bindVariableGoType records a Go FFI type for a local binding.
// Stores by SymbolID (scoped), optional occurrence span, and the ident map (last-writer convenience).
func (tc *TypeChecker) bindVariableGoType(ident ast.Identifier, vn *ast.VariableNode, gt types.Type) {
	if tc == nil || gt == nil || ident == "" || ident == "_" {
		return
	}
	tc.variableGoTypes[ident] = gt
	if scope := tc.CurrentScope(); scope != nil {
		if sym, ok := scope.LookupVariable(ident); ok && sym.ID != 0 {
			tc.variableGoTypesBySymbol[sym.ID] = gt
		}
	}
	if vn != nil && vn.Ident.Span.IsSet() {
		k := variableOccurrenceKey{ident: vn.Ident.ID, span: vn.Ident.Span}
		tc.variableGoTypesByOccurrence[k] = gt
	}
}

// clearStaleIdentGoType drops a package-level ident Go binding when the current-scope
// symbol has no Go type. Prevents Test*(t *testing.T) from leaking into a later Forst local t.
func (tc *TypeChecker) clearStaleIdentGoType(ident ast.Identifier) {
	if tc == nil || ident == "" || ident == "_" {
		return
	}
	scope := tc.CurrentScope()
	if scope == nil {
		return
	}
	sym, ok := scope.LookupVariable(ident)
	if !ok || sym.ID == 0 {
		return
	}
	if _, hasGo := tc.variableGoTypesBySymbol[sym.ID]; hasGo {
		return
	}
	delete(tc.variableGoTypes, ident)
}

// goTypeForVariableIdent returns the Go type for a simple (non-dotted) local name.
// When the name is bound in the current scope, only that symbol's Go type is used
// (including nil — no fallback to the ident map). Ident map is used only when the
// name is not in scope (post-CheckTypes unique-name lookups).
func (tc *TypeChecker) goTypeForVariableIdent(ident ast.Identifier) types.Type {
	if tc == nil || ident == "" {
		return nil
	}
	base := ident
	if i := strings.IndexByte(string(ident), '.'); i >= 0 {
		base = ast.Identifier(string(ident)[:i])
	}
	if scope := tc.CurrentScope(); scope != nil {
		if sym, ok := scope.LookupVariable(base); ok {
			if gt, has := tc.variableGoTypesBySymbol[sym.ID]; has {
				return gt
			}
			return nil
		}
	}
	return tc.variableGoTypes[base]
}

// goTypeForVariableNode returns the Go type for a variable occurrence.
// Lookup order: occurrence key → current-scope symbol → Forst occurrence without Go → ident map.
func (tc *TypeChecker) goTypeForVariableNode(vn ast.VariableNode) types.Type {
	if tc == nil {
		return nil
	}
	id := vn.Ident.ID
	if id == "" {
		return nil
	}
	if vn.Ident.Span.IsSet() {
		k := variableOccurrenceKey{ident: id, span: vn.Ident.Span}
		if gt, ok := tc.variableGoTypesByOccurrence[k]; ok {
			return gt
		}
		// This occurrence was typechecked as a Forst local with no Go binding —
		// do not fall back to a stolen package-level ident map entry.
		if _, hasForst := tc.variableOccurrenceTypes[k]; hasForst {
			base := id
			if i := strings.IndexByte(string(id), '.'); i >= 0 {
				base = ast.Identifier(string(id)[:i])
			}
			if base != id {
				return tc.goTypeForVariableIdent(base)
			}
			return nil
		}
	}
	return tc.goTypeForVariableIdent(id)
}

// recordOccurrenceGoType copies the scoped Go type onto this variable occurrence (for hover after CheckTypes).
func (tc *TypeChecker) recordOccurrenceGoType(vn ast.VariableNode) {
	if tc == nil || !vn.Ident.Span.IsSet() {
		return
	}
	gt := tc.goTypeForVariableIdent(vn.Ident.ID)
	if gt == nil {
		return
	}
	k := variableOccurrenceKey{ident: vn.Ident.ID, span: vn.Ident.Span}
	tc.variableGoTypesByOccurrence[k] = gt
}
