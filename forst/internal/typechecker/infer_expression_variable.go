package typechecker

import (
	"forst/internal/ast"
	"strings"
)

func (tc *TypeChecker) inferExpressionVariable(expr ast.Node) ([]ast.TypeNode, bool, error) {
	switch e := expr.(type) {
	case ast.VariableNode:

		parts := strings.Split(string(e.Ident.ID), ".")
		if len(parts) == 2 {
			if types, ok := tc.resolveForstSiblingQualifiedVar(parts[0], parts[1]); ok {
				tc.storeInferredType(e, types)
				return types, true, nil
			}
		}
		// Look up the variable's type and store it for this node. Flow-sensitive facts and FFI
		// invalidation (future) belong in FlowTypeFact / a separate layer — not in Meet/Join.
		typ, narrowGuards, predDisplay, err := tc.lookupVariableForExpression(&e, tc.CurrentScope())
		if err != nil {
			return nil, true, err
		}
		tc.log.Tracef("Variable type: %+v, node: %+v, type params: %+v, (original: %+v of type %T)", typ, e, typ.TypeParams, e, e)
		tc.storeInferredType(e, []ast.TypeNode{typ})
		if e.Ident.Span.IsSet() {
			k := variableOccurrenceKey{ident: e.Ident.ID, span: e.Ident.Span}
			if len(narrowGuards) > 0 {
				tc.variableOccurrenceNarrowingGuards[k] = append([]string(nil), narrowGuards...)
			}
			if predDisplay != "" {
				tc.variableOccurrenceNarrowingPredicateDisplay[k] = predDisplay
			}
			tc.recordOccurrenceGoType(e)
		}
		return []ast.TypeNode{typ}, true, nil
	}
	return nil, false, nil
}
