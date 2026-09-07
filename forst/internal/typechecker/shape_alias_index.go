package typechecker

import (
	"forst/internal/ast"
	"forst/internal/hasher"
	"sort"
)

type shapeAliasIndex struct {
	byShapeHash          map[hasher.NodeHash]ast.TypeIdent
	byAssertionTypeIdent map[ast.TypeIdent]ast.TypeIdent
}

func (tc *TypeChecker) invalidateShapeAliasIndex() {
	tc.shapeAliasIndex = nil
}

func (tc *TypeChecker) markHashBasedIdent(ident ast.TypeIdent) {
	if tc.hashBasedIdents == nil {
		tc.hashBasedIdents = make(map[ast.TypeIdent]struct{})
	}
	tc.hashBasedIdents[ident] = struct{}{}
}

// IsHashBasedIdent reports whether ident was registered as a compiler-invented
// structural/hash type (via RegisterHashBasedType / inferShapeType), not a
// user-named typedef that happens to start with "T_".
func (tc *TypeChecker) IsHashBasedIdent(ident ast.TypeIdent) bool {
	_, ok := tc.hashBasedIdents[ident]
	return ok
}

func (tc *TypeChecker) isHashBasedIdent(ident ast.TypeIdent) bool {
	return tc.IsHashBasedIdent(ident)
}

func (tc *TypeChecker) setDef(ident ast.TypeIdent, def ast.Node) {
	tc.Defs[ident] = def
	// Hash-based entries are skipped when building the index; invalidating
	// on them only forces repeated full rebuilds during inference.
	if !tc.isHashBasedIdent(ident) {
		tc.invalidateShapeAliasIndex()
	}
}

func (tc *TypeChecker) shapeAliasIndexOrBuild() *shapeAliasIndex {
	if tc.shapeAliasIndex != nil {
		return tc.shapeAliasIndex
	}
	idx := &shapeAliasIndex{
		byShapeHash:          make(map[hasher.NodeHash]ast.TypeIdent),
		byAssertionTypeIdent: make(map[ast.TypeIdent]ast.TypeIdent),
	}
	shapeCandidates := make(map[hasher.NodeHash][]ast.TypeIdent)
	assertionCandidates := make(map[ast.TypeIdent][]ast.TypeIdent)
	for _, def := range tc.Defs {
		userDef, ok := def.(ast.TypeDefNode)
		if !ok || userDef.Ident == "" || tc.isHashBasedIdent(userDef.Ident) {
			continue
		}
		if payload, ok := ast.PayloadShape(userDef.Expr); ok {
			h, err := tc.Hasher.HashNode(*payload)
			if err != nil {
				continue
			}
			shapeCandidates[h] = append(shapeCandidates[h], userDef.Ident)
		}
		if _, ok := typeDefAssertionFromExpr(userDef.Expr); ok {
			bt := userDef.Ident
			a := ast.AssertionNode{BaseType: &bt}
			h, err := tc.Hasher.HashNode(a)
			if err != nil {
				continue
			}
			key := h.ToTypeIdent()
			assertionCandidates[key] = append(assertionCandidates[key], userDef.Ident)
		}
	}
	for h, idents := range shapeCandidates {
		idx.byShapeHash[h] = stableTypeIdentWinner(idents)
	}
	for key, idents := range assertionCandidates {
		idx.byAssertionTypeIdent[key] = stableTypeIdentWinner(idents)
	}
	tc.shapeAliasIndex = idx
	return idx
}

func stableTypeIdentWinner(idents []ast.TypeIdent) ast.TypeIdent {
	if len(idents) == 0 {
		return ""
	}
	sort.Slice(idents, func(i, j int) bool {
		return idents[i] < idents[j]
	})
	return idents[0]
}

func (tc *TypeChecker) lookupShapeAliasForHashType(typeNode ast.TypeNode) (ast.TypeIdent, bool) {
	hashDef, ok := tc.Defs[typeNode.Ident]
	if !ok {
		return "", false
	}
	hashTypeDef, ok := hashDef.(ast.TypeDefNode)
	if !ok {
		return "", false
	}
	hashShapeExpr, ok := hashTypeDef.Expr.(ast.TypeDefShapeExpr)
	if !ok {
		return "", false
	}
	h, err := tc.Hasher.HashNode(hashShapeExpr.Shape)
	if err != nil {
		return "", false
	}
	alias, ok := tc.shapeAliasIndexOrBuild().byShapeHash[h]
	if !ok || alias == typeNode.Ident {
		return "", false
	}
	return alias, true
}

func (tc *TypeChecker) lookupAssertionAliasForHashIdent(ident ast.TypeIdent) (ast.TypeIdent, bool) {
	alias, ok := tc.shapeAliasIndexOrBuild().byAssertionTypeIdent[ident]
	return alias, ok
}
