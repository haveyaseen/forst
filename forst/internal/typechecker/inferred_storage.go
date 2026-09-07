// Inferred type storage: hashes on AST nodes and function return signatures.
package typechecker

import (
	"fmt"
	"forst/internal/ast"

	"github.com/sirupsen/logrus"
)

// storeInferredType associates inferred types with an AST node using its structural hash.
func (tc *TypeChecker) storeInferredType(node ast.Node, types []ast.TypeNode) {
	processedTypes := tc.normalizeTypesForStorage(types)

	if vn, ok := node.(ast.VariableNode); ok && vn.Ident.Span.IsSet() {
		k := variableOccurrenceKey{ident: vn.Ident.ID, span: vn.Ident.Span}
		tc.variableOccurrenceTypes[k] = processedTypes
	}
	hash, err := tc.Hasher.HashNode(node)
	if err != nil {
		if tc.log.IsLevelEnabled(logrus.ErrorLevel) {
			tc.log.WithFields(logrus.Fields{
				"node":     node.String(),
				"function": "storeInferredType",
			}).WithError(err).Error("failed to hash node during storeInferredType")
		}
		return
	}
	tc.Types[hash] = processedTypes
	if tc.log.IsLevelEnabled(logrus.TraceLevel) {
		tc.log.WithFields(logrus.Fields{
			"node":     node.String(),
			"key":      hash.ToTypeIdent(),
			"types":    processedTypes,
			"function": "storeInferredType",
			"hash":     fmt.Sprintf("%x", uint64(hash)),
		}).Trace("Stored inferred type for node")
	}
}

// storeInferredFunctionReturnType stores the return types for a function in its signature.
func (tc *TypeChecker) storeInferredFunctionReturnType(fn *ast.FunctionNode, returnTypes []ast.TypeNode) {
	// Constructor-free Result returns: the body infers plain S or F but the function type stays Result(S,F).
	if len(fn.ReturnTypes) == 1 && len(returnTypes) == 1 &&
		fn.ReturnTypes[0].IsResultType() && !returnTypes[0].IsResultType() {
		if tc.isPlainSuccessCompatibleWithDeclaredResult(returnTypes[0], fn.ReturnTypes[0]) ||
			tc.isPlainFailureCompatibleWithDeclaredResult(returnTypes[0], fn.ReturnTypes[0]) {
			returnTypes = []ast.TypeNode{fn.ReturnTypes[0]}
		}
	}
	// Prefer the declared named shape return when inference collapsed to a different
	// same-shaped named type (e.g. Acc value returned as Box). Also prefer declared
	// slice typedef aliases (ExprList = []String) over bare Array(String).
	if len(fn.ReturnTypes) == 1 && len(returnTypes) == 1 {
		declared, inferred := fn.ReturnTypes[0], returnTypes[0]
		if declared.Ident != "" && inferred.Ident != "" &&
			declared.Ident != inferred.Ident &&
			!declared.IsTypeParam() && !inferred.IsTypeParam() &&
			!declared.IsResultType() && !inferred.IsResultType() &&
			tc.IsTypeCompatible(inferred, declared) {
			if _, ok := tc.getShapeFromTypeDef(tc.Defs[declared.Ident]); ok {
				if _, ok := tc.getShapeFromTypeDef(tc.Defs[inferred.Ident]); ok {
					returnTypes = []ast.TypeNode{declared}
				}
			} else if _, ok := tc.Defs[declared.Ident].(ast.TypeDefNode); ok {
				returnTypes = []ast.TypeNode{declared}
			}
		}
	}
	// Resolve aliased types for return types
	resolvedReturnTypes := make([]ast.TypeNode, len(returnTypes))
	for i, returnType := range returnTypes {
		resolvedType := tc.resolveAliasedType(returnType)
		resolvedReturnTypes[i] = tc.normalizeTypeForStorage(resolvedType)
	}
	if IsVoidReturnTypes(resolvedReturnTypes) {
		resolvedReturnTypes = nil
	}

	if fn.Receiver != nil {
		recvType := receiverTypeIdentFromFn(fn)
		methodName := string(fn.Ident.ID)
		if tc.TypeMethods == nil {
			tc.TypeMethods = make(map[ast.TypeIdent]map[string]FunctionSignature)
		}
		if tc.TypeMethods[recvType] == nil {
			tc.TypeMethods[recvType] = make(map[string]FunctionSignature)
		}
		sig, ok := tc.TypeMethods[recvType][methodName]
		if !ok {
			sig = FunctionSignature{Ident: fn.Ident}
		}
		sig.ReturnTypes = resolvedReturnTypes
		tc.TypeMethods[recvType][methodName] = sig
		tc.log.WithFields(logrus.Fields{
			"fn":          fn.Ident.ID,
			"recvType":    recvType,
			"returnTypes": resolvedReturnTypes,
			"function":    "storeInferredFunctionReturnType",
		}).Trace("Stored inferred receiver method return type")
		return
	}

	sig := tc.Functions[fn.Ident.ID]
	sig.ReturnTypes = resolvedReturnTypes
	tc.Functions[fn.Ident.ID] = sig
	tc.log.WithFields(logrus.Fields{
		"fn":          fn.Ident.ID,
		"returnTypes": resolvedReturnTypes,
		"sig":         sig,
		"function":    "storeInferredFunctionReturnType",
	}).Trace("Stored inferred function return type")
}
