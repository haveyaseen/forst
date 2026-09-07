package hasher

import (
	"forst/internal/ast"
	"hash/fnv"
	"sort"
)

// HashScopeKey returns a scope-map key for node. It is cheaper than HashNode for
// scope-owning nodes whose bodies are not part of scope identity (functions,
// type guards, ensure blocks). Full HashNode semantics are unchanged for type identity.
func (h *StructuralHasher) HashScopeKey(node ast.Node) (NodeHash, error) {
	if node == nil || isNilPointer(node) {
		return NodeHash(NilHash), nil
	}
	w := newHashWalk(h)
	switch n := node.(type) {
	case ast.FunctionNode:
		return w.hashScopeFunction(n)
	case *ast.FunctionNode:
		if n == nil {
			return NodeHash(NilHash), nil
		}
		return w.hashScopeFunction(*n)
	case ast.EnsureNode:
		return w.hashScopeEnsure(n)
	case *ast.EnsureNode:
		if n == nil {
			return NodeHash(NilHash), nil
		}
		return w.hashScopeEnsure(*n)
	case ast.EnsureBlockNode:
		return w.hashScopeEnsureBlock(node)
	case *ast.EnsureBlockNode:
		if n == nil {
			return NodeHash(NilHash), nil
		}
		return w.hashScopeEnsureBlock(node)
	case ast.TypeGuardNode:
		return w.hashScopeTypeGuard(n)
	case *ast.TypeGuardNode:
		if n == nil {
			return NodeHash(NilHash), nil
		}
		return w.hashScopeTypeGuard(*n)
	case ast.FunctionLiteralNode:
		return w.hashScopeFunctionLiteral(n)
	case *ast.FunctionLiteralNode:
		if n == nil {
			return NodeHash(NilHash), nil
		}
		return w.hashScopeFunctionLiteral(*n)
	default:
		return h.HashNode(node)
	}
}

// HashScopeKeyDisambiguated mixes a base scope key with node identity when structural
// scope keys collide for distinct AST nodes.
func (h *StructuralHasher) HashScopeKeyDisambiguated(node ast.Node, base NodeHash) (NodeHash, error) {
	hasher := fnv.New64a()
	if err := h.writeHashes(hasher, uint64(base)); err != nil {
		return 0, err
	}
	if key, ok := NodeIdentityKey(node); ok {
		if err := h.writeHashes(hasher, uint64(key.Typ), uint64(key.Data)); err != nil {
			return 0, err
		}
	}
	return NodeHash(hasher.Sum64()), nil
}

func (w *hashWalk) hashScopeFunction(n ast.FunctionNode) (NodeHash, error) {
	hasher := fnv.New64a()
	if err := w.h.writeHashes(hasher, NodeKind["Function"], []byte(n.Ident.ID)); err != nil {
		return 0, err
	}
	if n.Receiver != nil {
		hash, err := w.hash(*n.Receiver)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, hash); err != nil {
			return 0, err
		}
	}
	params := make([]ast.Node, len(n.Params))
	for i, p := range n.Params {
		params[i] = p
	}
	paramHash, err := w.hashNodes(params)
	if err != nil {
		return 0, err
	}
	if err := w.h.writeHashes(hasher, paramHash); err != nil {
		return 0, err
	}
	for _, rt := range n.ReturnTypes {
		hash, err := w.hash(rt)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, hash); err != nil {
			return 0, err
		}
	}
	return NodeHash(hasher.Sum64()), nil
}

func (w *hashWalk) hashScopeEnsure(n ast.EnsureNode) (NodeHash, error) {
	hasher := fnv.New64a()
	if err := w.h.writeHashes(hasher, NodeKind["Ensure"]); err != nil {
		return 0, err
	}
	if n.Subject != nil {
		sh, err := w.hash(n.Subject)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, sh); err != nil {
			return 0, err
		}
	} else {
		vh, err := w.hash(n.Variable)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, vh); err != nil {
			return 0, err
		}
	}
	if err := w.h.writeHashes(hasher, uint8(n.Implicit)); err != nil {
		return 0, err
	}
	ah, err := w.hash(n.Assertion)
	if err != nil {
		return 0, err
	}
	if err := w.h.writeHashes(hasher, ah); err != nil {
		return 0, err
	}
	if n.Error != nil {
		if err := w.h.writeHashes(hasher, []byte((*n.Error).String())); err != nil {
			return 0, err
		}
	}
	return NodeHash(hasher.Sum64()), nil
}

func (w *hashWalk) hashScopeEnsureBlock(node ast.Node) (NodeHash, error) {
	hasher := fnv.New64a()
	if err := w.h.writeHashes(hasher, NodeKind["EnsureBlock"]); err != nil {
		return 0, err
	}
	if key, ok := NodeIdentityKey(node); ok {
		if err := w.h.writeHashes(hasher, uint64(key.Typ), uint64(key.Data)); err != nil {
			return 0, err
		}
	}
	return NodeHash(hasher.Sum64()), nil
}

func (w *hashWalk) hashScopeTypeGuard(n ast.TypeGuardNode) (NodeHash, error) {
	hasher := fnv.New64a()
	if err := w.h.writeHashes(hasher, NodeKind["TypeGuard"], []byte(n.Ident)); err != nil {
		return 0, err
	}
	subjHash, err := w.hash(n.Subject)
	if err != nil {
		return 0, err
	}
	if err := w.h.writeHashes(hasher, subjHash); err != nil {
		return 0, err
	}
	params := make([]ast.ParamNode, len(n.Parameters()))
	copy(params, n.Parameters())
	sort.Slice(params, func(i, j int) bool {
		var iName, jName string
		switch p := params[i].(type) {
		case ast.SimpleParamNode:
			iName = string(p.Ident.ID)
		case ast.DestructuredParamNode:
			if len(p.Fields) > 0 {
				iName = p.Fields[0]
			}
		}
		switch p := params[j].(type) {
		case ast.SimpleParamNode:
			jName = string(p.Ident.ID)
		case ast.DestructuredParamNode:
			if len(p.Fields) > 0 {
				jName = p.Fields[0]
			}
		}
		return iName < jName
	})
	nodes := make([]ast.Node, len(params))
	for i, p := range params {
		nodes[i] = p
	}
	paramHash, err := w.hashNodes(nodes)
	if err != nil {
		return 0, err
	}
	if err := w.h.writeHashes(hasher, paramHash); err != nil {
		return 0, err
	}
	return NodeHash(hasher.Sum64()), nil
}

func (w *hashWalk) hashScopeFunctionLiteral(n ast.FunctionLiteralNode) (NodeHash, error) {
	hasher := fnv.New64a()
	if err := w.h.writeHashes(hasher, NodeKind["FunctionLiteral"]); err != nil {
		return 0, err
	}
	params := make([]ast.Node, len(n.Params))
	for i, p := range n.Params {
		params[i] = p
	}
	paramHash, err := w.hashNodes(params)
	if err != nil {
		return 0, err
	}
	if err := w.h.writeHashes(hasher, paramHash); err != nil {
		return 0, err
	}
	for _, rt := range n.ReturnTypes {
		rtHash, err := w.hash(rt)
		if err != nil {
			return 0, err
		}
		if err := w.h.writeHashes(hasher, rtHash); err != nil {
			return 0, err
		}
	}
	bodyHash, err := w.hashNodes(n.Body)
	if err != nil {
		return 0, err
	}
	if err := w.h.writeHashes(hasher, bodyHash); err != nil {
		return 0, err
	}
	return NodeHash(hasher.Sum64()), nil
}
