package typechecker

import (
	"fmt"
	"strings"

	"forst/internal/ast"

	"github.com/sirupsen/logrus"
)

// LookupVariableType finds a variable's type in the current scope chain.
func (tc *TypeChecker) LookupVariableType(variable *ast.VariableNode, scope *Scope) (ast.TypeNode, error) {
	typ, _, _, err := tc.lookupVariableForExpression(variable, scope)
	return typ, err
}

// lookupVariableForExpression returns the inferred type, optional narrowing type guard names,
// optional dotted predicate display from the narrowing RHS, and an error. For field paths
// (e.g. g.cells), ensure-successor narrowing may register the full identifier; that symbol is
// preferred so hover shows Min(9).Max(9) like for a simple name.
func (tc *TypeChecker) lookupVariableForExpression(variable *ast.VariableNode, scope *Scope) (ast.TypeNode, []string, string, error) {
	tc.log.WithFields(logrus.Fields{
		"function": "LookupVariableType",
		"variable": variable.Ident.ID,
		"scope":    scope.String(),
	}).Debugf("Looking up variable type")

	parts := strings.Split(string(variable.Ident.ID), ".")
	if len(parts) > 1 {
		if symbol, exists := scope.LookupVariable(variable.Ident.ID); exists && len(symbol.Types) > 0 {
			tc.log.WithFields(logrus.Fields{
				"function":  "LookupVariableType",
				"variable":  variable.Ident.ID,
				"narrowing": symbol.NarrowingPredicateDisplay,
			}).Debugf("Using full-path symbol from ensure successor narrowing")
			return symbol.Types[0], symbol.NarrowingTypeGuards, symbol.NarrowingPredicateDisplay, nil
		}
	}
	baseIdent := ast.Identifier(parts[0])

	tc.log.WithFields(logrus.Fields{
		"function":  "LookupVariableType",
		"variable":  variable.Ident.ID,
		"baseIdent": baseIdent,
		"parts":     parts,
	}).Debugf("Split variable into parts")

	symbol, exists := scope.LookupVariable(baseIdent)
	if !exists {
		if len(parts) > 1 && tc.IsImportedLocalName(string(baseIdent)) {
			if t, err := tc.lookupGoImportedPackageSelector(baseIdent, parts[1:], variable.Ident.Span); err == nil {
				return t, nil, "", nil
			}
		}
		tc.log.WithFields(logrus.Fields{
			"function":  "LookupVariableType",
			"variable":  variable.Ident.ID,
			"baseIdent": baseIdent,
			"scope":     scope.String(),
		}).Debugf("Variable not found in scope")
		return ast.TypeNode{}, nil, "", reportf(variable.Ident.Span, "undefined-symbol",
			fmt.Sprintf("undefined symbol `%s`", parts[0]),
			fmt.Sprintf("No variable, function, or import named `%s` is in scope.", parts[0]),
			"declare the name, import it, or fix the spelling")
	}

	tc.log.WithFields(logrus.Fields{
		"function":  "LookupVariableType",
		"variable":  variable.Ident.ID,
		"baseIdent": baseIdent,
		"symbol":    fmt.Sprintf("%+v", symbol),
		"types":     symbol.Types,
	}).Debugf("Found symbol in scope")

	if len(symbol.Types) != 1 {
		tc.log.WithFields(logrus.Fields{
			"function":  "LookupVariableType",
			"variable":  variable.Ident.ID,
			"baseIdent": baseIdent,
			"typeCount": len(symbol.Types),
		}).Debugf("Expected single type but got multiple")
		return ast.TypeNode{}, nil, "", fmt.Errorf("expected single type for variable %s but got %d types", parts[0], len(symbol.Types))
	}

	if len(parts) == 1 {
		tc.log.WithFields(logrus.Fields{
			"function": "LookupVariableType",
			"variable": variable.Ident.ID,
			"type":     symbol.Types[0].Ident,
		}).Debugf("Returning single variable type")
		return symbol.Types[0], symbol.NarrowingTypeGuards, symbol.NarrowingPredicateDisplay, nil
	}

	tc.log.WithFields(logrus.Fields{
		"function":  "LookupVariableType",
		"variable":  variable.Ident.ID,
		"baseType":  symbol.Types[0].Ident,
		"fieldPath": parts[1:],
	}).Debugf("Looking up field path on base type")

	// Prefer go/types field resolution when this local was bound from a Go call (variableGoTypes):
	// e.g. *enmime.Envelope maps to Pointer((implicit)) in Forst, but Attachments is a real slice type.
	if goBase := tc.goTypeForVariableIdent(baseIdent); goBase != nil {
		if t, err := tc.lookupFieldPathFromGoType(goBase, parts[1:]); err == nil {
			return t, nil, "", nil
		}
	}

	// Use lookupFieldPath for multi-segment field access (Forst typedefs / shapes).
	t, err := tc.lookupFieldPath(symbol.Types[0], parts[1:], variable.Ident.Span)
	return t, nil, "", err
}

// LookupEnsureBaseType looks up the base type of an ensure node in a given scope.
func (tc *TypeChecker) LookupEnsureBaseType(ensure *ast.EnsureNode, scope *Scope) (*ast.TypeNode, error) {
	if ensure.IsCallSubject() {
		baseType, err := tc.lookupEnsureSubjectType(*ensure)
		if err != nil {
			return nil, err
		}
		return &baseType, nil
	}
	baseType, err := tc.LookupVariableType(&ensure.Variable, scope)
	if err != nil {
		return nil, err
	}
	return &baseType, nil
}
