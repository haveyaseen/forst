package typechecker

import (
	"fmt"
	"strings"

	"forst/internal/ast"

	logrus "github.com/sirupsen/logrus"
)

// underlyingBuiltinTypeOfAliasAssertion returns the built-in type ident (e.g. String) when a named
// type is defined as a TypeDefAssertionExpr whose base (directly or via alias chain) is a built-in.
// Inline constraints (String.Min(1), Slug.Premium()) do not change the underlying Go representation.
func (tc *TypeChecker) underlyingBuiltinTypeOfAliasAssertion(alias ast.TypeIdent) ast.TypeIdent {
	def, ok := tc.Defs[alias].(ast.TypeDefNode)
	if !ok {
		return ""
	}
	var ade *ast.TypeDefAssertionExpr
	switch expr := def.Expr.(type) {
	case ast.TypeDefAssertionExpr:
		e := expr
		ade = &e
	case *ast.TypeDefAssertionExpr:
		ade = expr
	default:
		return ""
	}
	if ade == nil || ade.Assertion == nil || ade.Assertion.BaseType == nil {
		return ""
	}
	// Type constructors with params are not scalar underlying builtins for emit.
	if len(ade.Assertion.TypeParams) > 0 || ade.Assertion.ArrayLen != nil {
		return ""
	}
	base := *ade.Assertion.BaseType
	if isBareTypeConstructorIdent(base) {
		return ""
	}
	if tc.isBuiltinType(base) {
		return base
	}
	if len(ade.Assertion.Constraints) != 0 {
		return tc.underlyingBuiltinTypeOfAliasAssertion(ast.TypeIdent(base))
	}
	return tc.underlyingBuiltinTypeOfAliasAssertion(ast.TypeIdent(base))
}

func isBareTypeConstructorIdent(id ast.TypeIdent) bool {
	switch id {
	case ast.TypeArray, ast.TypeMap, ast.TypePointer, ast.TypeChannel, ast.TypeResult, ast.TypeTuple:
		return true
	default:
		return false
	}
}

func (tc *TypeChecker) UnderlyingBuiltinTypeOfAliasAssertion(alias ast.TypeIdent) ast.TypeIdent {
	return tc.underlyingBuiltinTypeOfAliasAssertion(alias)
}

// refinedNarrowingTypeFromAliasAssertion maps `x is MyStr`-style narrowing to the underlying
// built-in when MyStr is an alias defined as an assertion over that built-in. This matches
// refinement semantics (underlying type in the branch) and avoids registering a hash-only T_…
// type as the narrowed binding when InferAssertionType would otherwise fall back to a structural hash.
//
// When the assertion uses only user-defined type guard constraints (e.g. `x is MyGuard()`), we
// refine to the guard's subject parameter type (see refinedNarrowingWhenAssertionUsesOnlyTypeGuards)
// instead of InferAssertionType's merged structural hash, so builtins like println still see String.
func (tc *TypeChecker) refinedNarrowingTypeFromAliasAssertion(assertion *ast.AssertionNode, refined []ast.TypeNode) []ast.TypeNode {
	if assertion == nil {
		return refined
	}
	if len(assertion.Constraints) > 0 {
		if alt := tc.refinedNarrowingWhenAssertionUsesOnlyTypeGuards(assertion); len(alt) > 0 {
			return alt
		}
		return refined
	}
	if assertion.BaseType == nil {
		return refined
	}
	base := *assertion.BaseType
	// `if x is MyGuard` often parses as BaseType = guard name (no constraints). Type guards live in
	// Defs as TypeGuardNode, not TypeDefNode, so alias-chain resolution above does not apply.
	if tc.IsTypeGuardConstraint(string(base)) {
		if t := tc.refinedTypesFromTypeGuardName(ast.TypeIdent(base)); len(t) > 0 {
			return t
		}
	}
	if u := tc.underlyingBuiltinTypeOfAliasAssertion(base); u != "" {
		return []ast.TypeNode{{Ident: u}}
	}
	return refined
}

// refinedTypesFromTypeGuardName returns the subject parameter type for a registered type guard,
// resolving alias-of-builtin to the builtin (e.g. Password → String) for consistent narrowing.
func (tc *TypeChecker) refinedTypesFromTypeGuardName(guardName ast.TypeIdent) []ast.TypeNode {
	def, ok := tc.Defs[guardName]
	if !ok {
		return nil
	}
	var gn ast.TypeGuardNode
	switch d := def.(type) {
	case *ast.TypeGuardNode:
		gn = *d
	case ast.TypeGuardNode:
		gn = d
	default:
		return nil
	}
	sp, ok := gn.Subject.(ast.SimpleParamNode)
	if !ok {
		return nil
	}
	tn := sp.Type
	if u := tc.underlyingBuiltinTypeOfAliasAssertion(tn.Ident); u != "" {
		return []ast.TypeNode{{Ident: u}}
	}
	return []ast.TypeNode{tn}
}

// refinedNarrowingWhenAssertionUsesOnlyTypeGuards applies when every constraint is a user type guard;
// the narrowed static type is then the first guard's subject type (same as the declared subject).
func (tc *TypeChecker) refinedNarrowingWhenAssertionUsesOnlyTypeGuards(assertion *ast.AssertionNode) []ast.TypeNode {
	if assertion == nil || len(assertion.Constraints) == 0 {
		return nil
	}
	for _, c := range assertion.Constraints {
		if !tc.IsTypeGuardConstraint(c.Name) {
			return nil
		}
	}
	return tc.refinedTypesFromTypeGuardName(ast.TypeIdent(assertion.Constraints[0].Name))
}

// applyIfBranchNarrowing registers a shadow variable binding in the current scope when the
// condition is of the form `subject is <assertion|shape>`. Strategy: lexical shadowing in the
// branch scope (see ROADMAP / type narrowing plan) so LookupVariableType sees the refined type.
// The condition must already have been type-checked (unifyIsOperator validation).
func (tc *TypeChecker) applyIfBranchNarrowing(condition ast.Node) {
	if condition == nil {
		return
	}
	tc.recordIfIsIR(condition)
	bin, ok := condition.(ast.BinaryExpressionNode)
	if !ok || bin.Operator != ast.TokenIs {
		return
	}
	refined, err := tc.refinedTypesForIsNarrowing(bin.Left, bin.Right)
	if err != nil {
		tc.log.WithFields(logrus.Fields{
			"function": "applyIfBranchNarrowing",
		}).WithError(err).Debug("skipping if-branch narrowing")
		return
	}
	if len(refined) == 0 {
		return
	}
	lv, err := tc.getLeftmostVariable(bin.Left)
	if err != nil {
		return
	}
	vn, ok := lv.(ast.VariableNode)
	if !ok {
		return
	}
	guards := tc.typeGuardNamesFromIsRHS(bin.Right)
	disp := tc.narrowingPredicateDisplayFromIsRHS(bin.Right)
	if tc.isBuiltinResultOkErrIsNarrowing(bin.Left, bin.Right) {
		// Refined type is already S or F; do not append `.Ok()` / `.Err()` to hover (would read as `Int.Ok()`).
		guards = nil
		disp = ""
	}
	tc.scopeStack.currentScope().RegisterSymbolWithNarrowing(vn.Ident.ID, refined, SymbolVariable, guards, disp)
	tc.recordCompoundNarrowingIdentifier(vn.Ident.ID, guards, disp)
	tc.recordIfChainNarrowingSubject(vn.Ident.ID, refined, guards)
	if a := assertionNodeFromIsRHS(bin.Right); a != nil {
		tc.proveAssertionOnSubject(vn, a)
		tc.exportMustFromAssertion(vn, a)
	}
}

// isBuiltinResultOkErrIsNarrowing is true when `left is <Assertion>` is the built-in Result
// discriminator (`Ok`/`Err` with Result subject), not a user-defined type guard named Ok/Err.
func (tc *TypeChecker) isBuiltinResultOkErrIsNarrowing(left, right ast.Node) bool {
	v, err := tc.getLeftmostVariable(left)
	if err != nil {
		return false
	}
	varLeftTypes, err := tc.inferExpressionType(v)
	if err != nil || len(varLeftTypes) != 1 {
		return false
	}
	varLeftType := varLeftTypes[0]
	a := assertionNodeFromIsRHS(right)
	if a == nil {
		return false
	}
	handled, _, _ := tc.refinedTypesForResultIsNarrowing(varLeftType, a, spanOfNode(v))
	return handled
}

// ensureUsesBuiltinResultOkErrDiscriminator is true when `ensure subject is Ok(...)` / `Err(...)`
// is the built-in Result(S,F) discriminator. In that case `inferExpressionType(AssertionNode)` would
// look for a type guard named Ok/Err and fail; narrowing uses the subject's Result type instead.
func (tc *TypeChecker) ensureUsesBuiltinResultOkErrDiscriminator(n ast.EnsureNode) bool {
	if n.Assertion.BaseType != nil || len(n.Assertion.Constraints) != 1 {
		return false
	}
	c := n.Assertion.Constraints[0].Name
	if c != "Ok" && c != "Err" {
		return false
	}
	vt, err := tc.lookupEnsureSubjectType(n)
	if err != nil {
		return false
	}
	return vt.IsResultType() && len(vt.TypeParams) >= 2
}

func assertionNodeFromIsRHS(right ast.Node) *ast.AssertionNode {
	if right == nil {
		return nil
	}
	switch r := right.(type) {
	case ast.AssertionNode:
		return &r
	case *ast.AssertionNode:
		return r
	case ast.TypeDefAssertionExpr:
		if r.Assertion != nil {
			return r.Assertion
		}
	case *ast.TypeDefAssertionExpr:
		if r != nil && r.Assertion != nil {
			return r.Assertion
		}
	}
	return nil
}

// refinedTypesForIsNarrowing returns the type(s) the subject should have when the `is` condition
// is true. Reuses InferAssertionType / inferShapeType so narrowing stays aligned with assertions.
func (tc *TypeChecker) refinedTypesForIsNarrowing(left, right ast.Node) ([]ast.TypeNode, error) {
	leftmostVar, err := tc.getLeftmostVariable(left)
	if err != nil {
		return nil, err
	}
	varLeftTypes, err := tc.inferExpressionType(leftmostVar)
	if err != nil {
		return nil, err
	}
	subjSpan := firstSetSpan(spanOfNode(leftmostVar), spanOfNode(left))
	if len(varLeftTypes) != 1 {
		return nil, reportf(subjSpan, "narrow-subject-type",
			"`is` subject must have a single type",
			fmt.Sprintf("The subject of `is` has %d types; narrowing needs exactly one.", len(varLeftTypes)),
			"bind the subject to a single-typed name")
	}
	varLeftType := varLeftTypes[0]

	switch r := right.(type) {
	case ast.AssertionNode:
		if handled, refined, err := tc.refinedTypesForResultIsNarrowing(varLeftType, &r, subjSpan); handled {
			if err != nil {
				return nil, err
			}
			return refined, nil
		}
		vn, ok := leftmostVar.(ast.VariableNode)
		if !ok {
			return nil, reportf(subjSpan, "narrow-unsupported-rhs",
				"assertion narrowing needs a variable subject",
				"The left-hand side of `is` must be a variable or field path.",
				"bind the expression to a name first")
		}
		return tc.refinedTypesForAssertionOnVariable(vn, &r)
	case ast.TypeDefAssertionExpr:
		if r.Assertion == nil {
			return nil, reportf(subjSpan, "narrow-missing-assertion",
				"missing assertion on the right of `is`",
				"The right-hand side of `is` must be an assertion or type target.",
				"write `if x is Ok()` or `if x is SomeGuard()`")
		}
		refined, err := tc.InferAssertionType(r.Assertion, false, "", &varLeftType)
		if err != nil {
			return nil, err
		}
		return tc.refinedNarrowingTypeFromAliasAssertion(r.Assertion, refined), nil
	case ast.ShapeNode:
		tn, err := tc.inferShapeType(r, &varLeftType)
		if err != nil {
			return nil, err
		}
		return []ast.TypeNode{tn}, nil
	case ast.FunctionCallNode:
		rhsSpan := firstSetSpan(spanOfExpression(r), spanOfNode(right), subjSpan)
		if tc.IsTypeGuardConstraint(string(r.Function.ID)) {
			if t := tc.refinedTypesFromTypeGuardName(ast.TypeIdent(r.Function.ID)); len(t) > 0 {
				return t, nil
			}
		}
		return nil, reportf(rhsSpan, "narrow-unsupported-rhs",
			"unsupported right-hand side of `is`",
			"This form cannot be used to narrow types.",
			"use an assertion like `Ok()`, a type guard, or a shape type")
	case *ast.FunctionCallNode:
		if r == nil {
			return nil, reportf(subjSpan, "narrow-unsupported-rhs",
				"missing right-hand side of `is`",
				"The right-hand side of `is` is empty.",
				"write `if x is Ok()` or `if x is SomeGuard()`")
		}
		rhsSpan := firstSetSpan(spanOfExpression(*r), spanOfNode(right), subjSpan)
		if tc.IsTypeGuardConstraint(string(r.Function.ID)) {
			if t := tc.refinedTypesFromTypeGuardName(ast.TypeIdent(r.Function.ID)); len(t) > 0 {
				return t, nil
			}
		}
		return nil, reportf(rhsSpan, "narrow-unsupported-rhs",
			"unsupported right-hand side of `is`",
			"This form cannot be used to narrow types.",
			"use an assertion like `Ok()`, a type guard, or a shape type")
	default:
		rightTypes, err := tc.inferExpressionType(right)
		if err != nil {
			return nil, err
		}
		if len(rightTypes) != 1 {
			return nil, reportf(subjSpan, "narrow-rhs-type",
				"right-hand side of `is` must have a single type",
				fmt.Sprintf("The right-hand side of `is` has %d types.", len(rightTypes)),
				"use an assertion, type guard, or shape on the right of `is`")
		}
		if rightTypes[0].Ident == ast.TypeShape {
			return nil, reportf(subjSpan, "narrow-unsupported-rhs",
				"shape narrowing needs a shape literal",
				fmt.Sprintf("Shape narrowing requires a ShapeNode on the right of `is`, got %T.", right),
				"write `if x is Shape { field: T }` with a shape literal")
		}
		return nil, reportf(firstSetSpan(spanOfNode(right), subjSpan), "narrow-unsupported-rhs",
			"unsupported right-hand side of `is`",
			fmt.Sprintf("This form (%T) cannot be used to narrow types.", right),
			"use an assertion like `Ok()`, a type guard, or a shape type")
	}
}

// refinedTypesForResultEnsureBlockFailure returns the subject type inside
// `ensure x is Ok()/Err() { ... }` for the block body. The block runs when the assertion fails (same
// as the generated `if !(ok)` branch), so for Ok() the subject is the failure type F; for Err() it is
// the success type S.
func (tc *TypeChecker) refinedTypesForResultEnsureBlockFailure(varLeftType ast.TypeNode, a *ast.AssertionNode, span ast.SourceSpan) ([]ast.TypeNode, error) {
	handled, _, err := tc.refinedTypesForResultIsNarrowing(varLeftType, a, span)
	if !handled || err != nil {
		return nil, err
	}
	if !varLeftType.IsResultType() || len(varLeftType.TypeParams) < 2 || a == nil || len(a.Constraints) != 1 {
		return nil, nil
	}
	c := a.Constraints[0]
	if c.Name == "Ok" {
		return []ast.TypeNode{varLeftType.TypeParams[1]}, nil
	}
	if c.Name == "Err" {
		return []ast.TypeNode{varLeftType.TypeParams[0]}, nil
	}
	return nil, nil
}

// applyEnsureBlockResultFailureNarrowing registers the failure-branch Result(S,F) refinement for the
// ensure subject inside an ensure block body (built-in Ok/Err only).
func (tc *TypeChecker) applyEnsureBlockResultFailureNarrowing(n ast.EnsureNode) {
	if n.IsCallSubject() {
		return
	}
	vn := n.Variable
	vt, err := tc.LookupVariableType(&vn, tc.CurrentScope())
	if err != nil {
		return
	}
	refined, err := tc.refinedTypesForResultEnsureBlockFailure(vt, &n.Assertion, n.Variable.Ident.Span)
	if err != nil || len(refined) == 0 {
		return
	}
	tc.scopeStack.currentScope().RegisterSymbolWithNarrowing(vn.Ident.ID, refined, SymbolVariable, nil, "")
	tc.recordCompoundNarrowingIdentifier(vn.Ident.ID, nil, "")
}

// refinedTypesForResultIsNarrowing handles `x is Ok(...)` / `Err(...)` when x is Result(S,F).
func (tc *TypeChecker) refinedTypesForResultIsNarrowing(varLeftType ast.TypeNode, a *ast.AssertionNode, span ast.SourceSpan) (handled bool, refined []ast.TypeNode, err error) {
	if a == nil || len(a.Constraints) != 1 || a.BaseType != nil {
		return false, nil, nil
	}
	c := a.Constraints[0]
	if c.Name != "Ok" && c.Name != "Err" {
		return false, nil, nil
	}
	if !varLeftType.IsResultType() || len(varLeftType.TypeParams) < 2 {
		// User type guard named Ok/Err (e.g. `is (v N) Ok()`) — not Result discriminators.
		return false, nil, nil
	}
	if err := tc.validateResultDiscriminatorAssertion(*a, varLeftType, span); err != nil {
		return true, nil, err
	}
	if c.Name == "Ok" {
		return true, []ast.TypeNode{varLeftType.TypeParams[0]}, nil
	}
	fail := varLeftType.TypeParams[1]
	if len(c.Args) == 0 {
		return true, []ast.TypeNode{fail}, nil
	}
	if len(c.Args) != 1 {
		return true, nil, reportf(span, "result-err-arity",
			"Err(...) expects at most one argument",
			"`Err(...)` accepts zero or one argument in narrowing.",
			"use `Err()` or `Err(value)`")
	}
	arg := c.Args[0]
	if arg.Type != nil {
		return true, []ast.TypeNode{*arg.Type}, nil
	}
	if arg.Value == nil {
		return true, nil, reportf(span, "result-err-arity",
			"Err(...) requires a value or type argument",
			"`Err(...)` needs a value expression or explicit type when an argument is present.",
			"pass a failure value or write `Err()` with no args")
	}
	vt, err := tc.inferExpressionType(*arg.Value)
	if err != nil {
		return true, nil, err
	}
	if len(vt) != 1 {
		return true, nil, reportf(spanOfExpression(*arg.Value), "result-err-arity",
			"Err(...) argument must have a single type",
			"The argument to `Err(...)` must infer to exactly one type.",
			"pass a single failure value")
	}
	return true, []ast.TypeNode{vt[0]}, nil
}

// refinedTypesForAssertionOnVariable returns the refined type(s) for a variable under the given
// assertion (same rules as the RHS of `x is …`). Shared by if-branch narrowing and ensure-successor narrowing.
func (tc *TypeChecker) refinedTypesForAssertionOnVariable(vn ast.VariableNode, assertion *ast.AssertionNode) ([]ast.TypeNode, error) {
	if assertion == nil {
		return nil, fmt.Errorf("nil assertion")
	}
	varLeftTypes, err := tc.inferExpressionType(vn)
	if err != nil {
		return nil, err
	}
	if len(varLeftTypes) != 1 {
		return nil, reportf(vn.Ident.Span, "narrow-subject-type",
			"assertion subject must have a single type",
			fmt.Sprintf("Variable `%s` has %d types for assertion narrowing.", vn.Ident.ID, len(varLeftTypes)),
			"ensure the subject has a single inferred type")
	}
	varLeftType := varLeftTypes[0]
	// Keep successor scope and hover on the subject's static type (e.g. String) for built-in
	// refinements (Min, Max, …). InferAssertionType still produces hash structural types for codegen.
	if tc.assertionRefinesBuiltinSubjectWithOnlyBuiltinConstraints(assertion) {
		return []ast.TypeNode{varLeftType}, nil
	}
	refined, err := tc.InferAssertionType(assertion, false, "", &varLeftType)
	if err != nil {
		return nil, err
	}
	return tc.refinedNarrowingTypeFromAliasAssertion(assertion, refined), nil
}

// assertionRefinesBuiltinSubjectWithOnlyBuiltinConstraints matches assertions like
// `String.Min(12)`, `ensure x is Min(1)` (no BaseType), or `ensure x is String.Min(1)` where every
// constraint is a built-in refinement, not a user TypeGuardNode.
func (tc *TypeChecker) assertionRefinesBuiltinSubjectWithOnlyBuiltinConstraints(a *ast.AssertionNode) bool {
	if a == nil || len(a.Constraints) == 0 {
		return false
	}
	for _, c := range a.Constraints {
		if c.Name == ConstraintMatch {
			return false
		}
		if def, ok := tc.Defs[ast.TypeIdent(c.Name)]; ok {
			if _, ok := def.(ast.TypeGuardNode); ok {
				return false
			}
		}
		if !isBuiltinAssertionConstraintName(c.Name) {
			return false
		}
	}
	if a.BaseType != nil && !tc.isBuiltinType(*a.BaseType) {
		return false
	}
	return true
}

// applyEnsureSuccessorNarrowing registers a refined binding for the ensure subject so that
// following statements (or the ensure block body) see the same types as after `x is …`.
// Field paths such as `g.cells` register under the full identifier so lookup + hover match
// simple variables (Min/Max chain, etc.).
func (tc *TypeChecker) applyEnsureSuccessorNarrowing(n ast.EnsureNode) {
	// Call subjects are fire-and-forget — there is no place binding to refine.
	if n.IsCallSubject() {
		tc.log.WithFields(logrus.Fields{
			"function": "applyEnsureSuccessorNarrowing",
			"subject":  n.Subject.String(),
		}).Debug("skipping ensure successor narrowing for call subject")
		return
	}
	vn := n.Variable
	// TypeTarget: narrow subject to the named type (literal union / nominal domain).
	if tt, ok := n.Target.(ast.TypeTarget); ok {
		refined := []ast.TypeNode{{Ident: tt.Name, TypeKind: ast.TypeKindUserDefined}}
		tc.scopeStack.currentScope().RegisterSymbolWithNarrowing(vn.Ident.ID, refined, SymbolVariable, nil, string(tt.Name))
		tc.recordCompoundNarrowingIdentifier(vn.Ident.ID, nil, string(tt.Name))
		tc.recordEnsureRefinementFact(n)
		return
	}
	if p, ok := n.Target.(*ast.TypeTarget); ok && p != nil {
		refined := []ast.TypeNode{{Ident: p.Name, TypeKind: ast.TypeKindUserDefined}}
		tc.scopeStack.currentScope().RegisterSymbolWithNarrowing(vn.Ident.ID, refined, SymbolVariable, nil, string(p.Name))
		tc.recordCompoundNarrowingIdentifier(vn.Ident.ID, nil, string(p.Name))
		tc.recordEnsureRefinementFact(n)
		return
	}
	// Runtime-only atoms (Min(n)): keep carrier type; do not invent a static dependent type.
	if a, _ := LowerRefinementTarget(n.Target, n.Assertion); HasRuntimeOnlyAtom(a) {
		path := tc.AccessPathForVariable(&vn)
		if tc.predicates != nil {
			tc.CurrentRefinementContext().Prove(path, tc.predicates.FromAssertion(a))
		}
		tc.recordEnsureRefinementFact(n)
		return
	}
	// Best-effort assertion expression inference (registers tc.Types for the assertion subtree).
	// Do not abort narrowing on failure: `inferExpressionType(AssertionNode)` often lacks the
	// subject context that `refinedTypesForIsNarrowing` supplies via InferAssertionType, so it can
	// error while validation + narrowing still succeed (e.g. `ensure x is String.Min(1)`).
	// Skip for built-in Result Ok/Err — those are not type guards; infer would error ("Ok not found").
	if !tc.ensureUsesBuiltinResultOkErrDiscriminator(n) {
		if _, err := tc.inferExpressionType(n.Assertion); err != nil {
			tc.log.WithFields(logrus.Fields{
				"function": "applyEnsureSuccessorNarrowing",
			}).WithError(err).Debug("ensure assertion expr inference failed (continuing with narrowing)")
		}
	}
	refined, err := tc.refinedTypesForIsNarrowing(n.Variable, n.Assertion)
	if err != nil {
		tc.log.WithFields(logrus.Fields{
			"function": "applyEnsureSuccessorNarrowing",
		}).WithError(err).Debug("skipping ensure successor narrowing")
		return
	}
	if len(refined) == 0 {
		return
	}
	guards := tc.typeGuardNamesFromAssertionNode(&n.Assertion)
	disp := tc.narrowingPredicateDisplayFromIsRHS(n.Assertion)
	if tc.isBuiltinResultOkErrIsNarrowing(n.Variable, n.Assertion) {
		guards = nil
		disp = ""
	}
	tc.scopeStack.currentScope().RegisterSymbolWithNarrowing(vn.Ident.ID, refined, SymbolVariable, guards, disp)
	tc.recordCompoundNarrowingIdentifier(vn.Ident.ID, guards, disp)
	tc.proveAssertionOnSubject(vn, &n.Assertion)
	tc.exportMustFromAssertion(vn, &n.Assertion)
	tc.recordEnsureRefinementFact(n)
}

func (tc *TypeChecker) recordEnsureRefinementFact(n ast.EnsureNode) {
	if tc == nil {
		return
	}
	if tc.paths == nil {
		tc.paths = NewPathInterner()
	}
	subject := tc.AccessPathForVariable(&n.Variable)
	var pred *Predicate
	if tt, ok := n.Target.(ast.TypeTarget); ok {
		if tc.predicates != nil {
			pred = tc.predicates.InternAtom(Operand{Name: string(tt.Name)})
		}
	} else if p, ok := n.Target.(*ast.TypeTarget); ok && p != nil {
		if tc.predicates != nil {
			pred = tc.predicates.InternAtom(Operand{Name: string(p.Name)})
		}
	} else if tc.predicates != nil {
		ir, _ := LowerRefinementTarget(n.Target, n.Assertion)
		pred = tc.predicates.FromAssertion(ir)
	}
	reads := tc.ExtractDepsForEnsure(n)
	predName := ""
	if names := tc.typeGuardNamesFromAssertionNode(&n.Assertion); len(names) > 0 {
		predName = names[0]
		if tc.predicates != nil {
			pred = tc.predicates.InternAtom(Operand{Name: predName})
		}
	}
	tc.recordRefinementFact(RefinementFact{
		Subject:       subject,
		Predicate:     pred,
		Reads:         reads,
		EstablishedAt: n.Variable.Ident.Span,
	})
	_ = predName
	// Also record must()-exported Present facts with their own deps.
	for _, c := range n.Assertion.Constraints {
		if !tc.IsTypeGuardConstraint(c.Name) {
			continue
		}
		tc.recordExportedMustFacts(n.Variable, c.Name)
	}
}

func (tc *TypeChecker) recordExportedMustFacts(subject ast.VariableNode, guardName string) {
	def, ok := tc.Defs[ast.TypeIdent(guardName)]
	if !ok {
		return
	}
	var gn ast.TypeGuardNode
	switch d := def.(type) {
	case *ast.TypeGuardNode:
		gn = *d
	case ast.TypeGuardNode:
		gn = d
	default:
		return
	}
	subjIdent := string(gn.Subject.GetIdent())
	callRoot := string(subject.Ident.ID)
	for _, node := range gn.Body {
		ens, ok := ensureStmt(node)
		if !ok || len(ens.Assertion.OrChains) > 0 {
			continue
		}
		pathID := rebaseGuardEnsurePath(subjIdent, callRoot, string(ens.Variable.Ident.ID))
		if pathID == "" {
			continue
		}
		vn := ast.VariableNode{Ident: ast.Ident{ID: ast.Identifier(pathID), Span: subject.Ident.Span}}
		fake := ast.EnsureNode{Variable: vn, Target: ens.Target, Assertion: ens.Assertion}
		tc.recordRefinementFact(RefinementFact{
			Subject:       tc.AccessPathForVariable(&vn),
			Predicate:     tc.predicates.FromAssertion(LowerAssertionNode(ens.Assertion)),
			Reads:         tc.ExtractDepsForEnsure(fake),
			EstablishedAt: subject.Ident.Span,
		})
	}
}

func (tc *TypeChecker) recordCompoundNarrowingIdentifier(id ast.Identifier, guards []string, disp string) {
	if tc == nil || id == "" || !strings.Contains(string(id), ".") {
		return
	}
	prev := tc.compoundNarrowingByIdentifier[id]
	mergedGuards := mergeNarrowingGuardNamesDedupe(prev.guards, guards)
	mergedDisp := mergeNarrowingPredicateDisplaySegments(prev.disp, disp)
	tc.compoundNarrowingByIdentifier[id] = compoundNarrowingInfo{
		guards: mergedGuards,
		disp:   mergedDisp,
	}
}

// --- Control-flow join at the merge point after a completed if / else-if / else chain (plan §3.2).

// narrowingEvent records one successful `x is …` branch narrowing in the current if-chain.
type narrowingEvent struct {
	ident               ast.Identifier
	refined             []ast.TypeNode
	narrowingTypeGuards []string
}

func (tc *TypeChecker) beginIfChainForStatement() {
	tc.ifChainNarrowingStack = append(tc.ifChainNarrowingStack, nil)
}

func (tc *TypeChecker) recordIfChainNarrowingSubject(id ast.Identifier, refined []ast.TypeNode, guards []string) {
	if len(tc.ifChainNarrowingStack) == 0 || len(refined) == 0 {
		return
	}
	top := len(tc.ifChainNarrowingStack) - 1
	tc.ifChainNarrowingStack[top] = append(tc.ifChainNarrowingStack[top], narrowingEvent{
		ident:               id,
		refined:             append([]ast.TypeNode(nil), refined...),
		narrowingTypeGuards: append([]string(nil), guards...),
	})
}

// endIfChainApplyJoin runs at the syntactic merge point after an IfNode: for each binding that was
// narrowed in any branch, the static type after the whole chain is JoinAfterIfMerge → outer
// (pre-if) type. LookupVariable already resolves to the outer binding; we also run
// MergeFlowFactsAtIfJoin with actual branch refinements so JoinAfterIfMerge receives real inputs
// (today still widens to outer; future union/LUB can use branchRefinements).
func (tc *TypeChecker) endIfChainApplyJoin() {
	if len(tc.ifChainNarrowingStack) == 0 {
		return
	}
	top := len(tc.ifChainNarrowingStack) - 1
	frame := tc.ifChainNarrowingStack[top]
	tc.ifChainNarrowingStack = tc.ifChainNarrowingStack[:top]
	if len(frame) == 0 {
		return
	}

	seenID := make(map[ast.Identifier]struct{}, len(frame))
	var branchFacts []FlowTypeFact
	for _, ev := range frame {
		if _, ok := seenID[ev.ident]; !ok {
			seenID[ev.ident] = struct{}{}
		}
		for _, rt := range ev.refined {
			branchFacts = append(branchFacts, FlowTypeFact{
				Ident:               ev.ident,
				RefinedType:         rt,
				NarrowingTypeGuards: append([]string(nil), ev.narrowingTypeGuards...),
			})
		}
	}

	refinementCount := make(map[ast.Identifier]int, len(seenID))
	for _, f := range branchFacts {
		refinementCount[f.Ident]++
	}

	outerByIdent := make(map[ast.Identifier]ast.TypeNode, len(seenID))
	for id := range seenID {
		sym, ok := tc.CurrentScope().LookupVariable(id)
		if !ok || len(sym.Types) != 1 {
			continue
		}
		outerByIdent[id] = sym.Types[0]
	}

	mergedByIdent := MergeFlowFactsAtIfJoin(tc, outerByIdent, branchFacts)
	for id, merged := range mergedByIdent {
		outer := outerByIdent[id]
		tc.log.WithFields(logrus.Fields{
			"function":          "endIfChainApplyJoin",
			"identifier":        id,
			"outer":             outer.Ident,
			"merged":            merged.Ident,
			"branchRefinements": refinementCount[id],
		}).Trace("if-chain merge point: MergeFlowFactsAtIfJoin + JoinAfterIfMerge (widen to outer / pre-if binding)")
	}
}
