package typechecker

import (
	"sort"

	"forst/internal/ast"
	"forst/internal/astwalk"
)

type errorSetAcc struct {
	nominal map[ast.TypeIdent]struct{}
	unknown bool
}

func newErrorSetAcc() *errorSetAcc {
	return &errorSetAcc{nominal: make(map[ast.TypeIdent]struct{})}
}

func (a *errorSetAcc) addNominal(id ast.TypeIdent) {
	if id == "" {
		return
	}
	a.nominal[id] = struct{}{}
}

func (a *errorSetAcc) addNominals(ids []ast.TypeIdent) {
	for _, id := range ids {
		a.addNominal(id)
	}
}

func (a *errorSetAcc) markUnknown() {
	a.unknown = true
}

func (a *errorSetAcc) merge(other FunctionErrorSet) {
	a.addNominals(other.NominalErrors)
	if other.UnknownPossible {
		a.unknown = true
	}
}

func (a *errorSetAcc) finish() FunctionErrorSet {
	out := FunctionErrorSet{UnknownPossible: a.unknown}
	if len(a.nominal) == 0 {
		return out
	}
	out.NominalErrors = make([]ast.TypeIdent, 0, len(a.nominal))
	for id := range a.nominal {
		out.NominalErrors = append(out.NominalErrors, id)
	}
	sort.Slice(out.NominalErrors, func(i, j int) bool {
		return out.NominalErrors[i] < out.NominalErrors[j]
	})
	return out
}

// IsNominalErrorType reports whether ident names a user declared `error X { ... }` type.
func (tc *TypeChecker) IsNominalErrorType(id ast.TypeIdent) bool {
	if id == "" {
		return false
	}
	def, ok := tc.Defs[id].(ast.TypeDefNode)
	if !ok {
		return false
	}
	_, ok = def.Expr.(ast.TypeDefErrorExpr)
	return ok
}

// nominalErrorsFromTypeNode expands a type node to nominal error idents when possible.
func (tc *TypeChecker) nominalErrorsFromTypeNode(t ast.TypeNode) ([]ast.TypeIdent, bool) {
	if tc.IsNominalErrorType(t.Ident) {
		return []ast.TypeIdent{t.Ident}, false
	}
	if t.Ident == ast.TypeUnion && len(t.TypeParams) > 0 {
		var out []ast.TypeIdent
		for _, m := range t.TypeParams {
			noms, unk := tc.nominalErrorsFromTypeNode(m)
			if unk {
				return nil, true
			}
			out = append(out, noms...)
		}
		if len(out) == 0 {
			return nil, false
		}
		seen := make(map[ast.TypeIdent]struct{}, len(out))
		var deduped []ast.TypeIdent
		for _, id := range out {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			deduped = append(deduped, id)
		}
		sort.Slice(deduped, func(i, j int) bool { return deduped[i] < deduped[j] })
		return deduped, false
	}
	if t.Ident == ast.TypeError {
		return nil, true
	}
	if alias, ok := tc.Defs[t.Ident].(ast.TypeDefNode); ok {
		if canon, err := tc.TypeDefExprToTypeNode(alias.Expr); err == nil {
			return tc.nominalErrorsFromTypeNode(canon)
		}
	}
	return nil, false
}

func (tc *TypeChecker) errorSetFromEnsure(stmt ast.EnsureNode) FunctionErrorSet {
	acc := newErrorSetAcc()
	if stmt.Error == nil {
		acc.markUnknown()
		return acc.finish()
	}
	switch e := (*stmt.Error).(type) {
	case ast.EnsureErrorCall:
		if tc.IsNominalErrorType(ast.TypeIdent(e.ErrorType)) {
			acc.addNominal(ast.TypeIdent(e.ErrorType))
			return acc.finish()
		}
		if set, ok := tc.errorSetFromEnsureHelperCall(e); ok {
			acc.merge(set)
			return acc.finish()
		}
		acc.markUnknown()
	case ast.EnsureErrorVar:
		types, ok := tc.InferredTypesForVariableIdentifier(ast.Identifier(e))
		if !ok || len(types) == 0 {
			acc.markUnknown()
			return acc.finish()
		}
		for _, ty := range types {
			noms, unk := tc.nominalErrorsFromTypeNode(ty)
			if unk {
				acc.markUnknown()
			}
			acc.addNominals(noms)
		}
	case ast.EnsureErrorExpr:
		acc.markUnknown()
	}
	return acc.finish()
}

func (tc *TypeChecker) errorSetFromEnsureHelperCall(e ast.EnsureErrorCall) (FunctionErrorSet, bool) {
	sig, ok := tc.Functions[ast.Identifier(e.ErrorType)]
	if !ok {
		return FunctionErrorSet{}, false
	}
	if len(sig.ReturnTypes) == 1 && sig.ReturnTypes[0].IsResultType() && len(sig.ReturnTypes[0].TypeParams) >= 2 {
		fail := sig.ReturnTypes[0].TypeParams[1]
		noms, unk := tc.nominalErrorsFromTypeNode(fail)
		return FunctionErrorSet{NominalErrors: noms, UnknownPossible: unk}, true
	}
	for _, rt := range sig.ReturnTypes {
		if rt.IsError() {
			return FunctionErrorSet{UnknownPossible: true}, true
		}
		if noms, unk := tc.nominalErrorsFromTypeNode(rt); len(noms) > 0 || unk {
			return FunctionErrorSet{NominalErrors: noms, UnknownPossible: unk}, true
		}
	}
	return FunctionErrorSet{UnknownPossible: true}, true
}

func (tc *TypeChecker) mergeErrorSetFromCall(call ast.FunctionCallNode, calleeSets map[ast.Identifier]FunctionErrorSet, acc *errorSetAcc) {
	if tc.IsNominalErrorType(ast.TypeIdent(call.Function.ID)) {
		acc.addNominal(ast.TypeIdent(call.Function.ID))
		return
	}
	if calleeSets != nil {
		if set, ok := calleeSets[call.Function.ID]; ok {
			acc.merge(set)
			return
		}
	}
	if _, ok := tc.Functions[call.Function.ID]; ok {
		return
	}
	acc.markUnknown()
}

func (tc *TypeChecker) collectLocalFunctionErrorSet(fn ast.FunctionNode, calleeSets map[ast.Identifier]FunctionErrorSet) FunctionErrorSet {
	acc := newErrorSetAcc()
	astwalk.WalkStmts(fn.Body, astwalk.StmtVisitor{
		OnEnsure: func(stmt ast.EnsureNode) bool {
			acc.merge(tc.errorSetFromEnsure(stmt))
			return true
		},
		OnCall: func(call ast.FunctionCallNode) bool {
			tc.mergeErrorSetFromCall(call, calleeSets, acc)
			return true
		},
		OnReturn: func(ret ast.ReturnNode) bool {
			for _, val := range ret.Values {
				if call, ok := val.(ast.FunctionCallNode); ok {
					if tc.IsNominalErrorType(ast.TypeIdent(call.Function.ID)) {
						acc.addNominal(ast.TypeIdent(call.Function.ID))
					}
				}
			}
			return true
		},
	})
	return acc.finish()
}

func errorSetsEqual(a, b FunctionErrorSet) bool {
	if a.UnknownPossible != b.UnknownPossible {
		return false
	}
	if len(a.NominalErrors) != len(b.NominalErrors) {
		return false
	}
	for i := range a.NominalErrors {
		if a.NominalErrors[i] != b.NominalErrors[i] {
			return false
		}
	}
	return true
}

// inferAllFunctionErrorSets computes transitive error sets for every registered function.
func (tc *TypeChecker) inferAllFunctionErrorSets(nodes []ast.Node) {
	fnBodies := make(map[ast.Identifier]ast.FunctionNode)
	for _, node := range nodes {
		fn, ok := node.(ast.FunctionNode)
		if !ok {
			continue
		}
		if _, exists := tc.Functions[fn.Ident.ID]; !exists {
			continue
		}
		fnBodies[fn.Ident.ID] = fn
	}

	sets := make(map[ast.Identifier]FunctionErrorSet, len(fnBodies))
	for id, fn := range fnBodies {
		sets[id] = tc.collectLocalFunctionErrorSet(fn, nil)
	}

	changed := true
	for changed {
		changed = false
		for id, fn := range fnBodies {
			next := tc.collectLocalFunctionErrorSet(fn, sets)
			if !errorSetsEqual(next, sets[id]) {
				sets[id] = next
				changed = true
			}
		}
	}

	for id, set := range sets {
		sig := tc.Functions[id]
		sig.ErrorSet = set
		tc.Functions[id] = sig
	}
}
