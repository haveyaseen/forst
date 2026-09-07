package ast

import "fmt"

// EnsureImplicitKind records ensure sugar that is specialized after the subject type is known.
type EnsureImplicitKind uint8

const (
	// EnsureImplicitNone means the assertion was written explicitly (`ensure x is …`).
	EnsureImplicitNone EnsureImplicitKind = iota
	// EnsureImplicitBare is `ensure x` with no `is` (Bool→True, Result→Ok).
	EnsureImplicitBare
	// EnsureImplicitBang is `ensure !x` (Bool→False, Error/nilable→Nil).
	// Result subjects are rejected during sugar specialization.
	EnsureImplicitBang
)

// EnsureNode represents an ensure statement in the AST.
// Typed failure is Error (from `else`); FailureBlock is Block (from `{ … }`).
// Error and Block are mutually exclusive (XOR).
type EnsureNode struct {
	// Variable is the place subject (ident or dotted field path). Zero when Subject is set.
	Variable VariableNode
	// Subject is a non-place ensure subject (function or method call). Nil for place subjects.
	// Call subjects are fire-and-forget success checks (especially void Result → Go error);
	// prefer Variable when a narrowed success value is needed afterward.
	Subject ExpressionNode
	// Target is the RHS of `is`: TypeTarget (bare type name) or AssertionTarget
	// (constraint chain(s), possibly Join via `or`). Nil for ImplicitBare / ImplicitBang
	// until the typechecker specializes the sugar.
	Target RefinementTarget
	// Assertion is the primary assertion view for AssertionTarget (first Meet chain,
	// with OrChains for Join). For TypeTarget it holds BaseType only (compat).
	// Empty when Implicit is Bare or Bang until specialized.
	Assertion AssertionNode
	// Implicit records bare / bang sugar that must be specialized from the subject type.
	Implicit EnsureImplicitKind
	/// Is optional if we're in the main function of the main package
	Error *EnsureErrorNode
	// Block is the failure block (alias FailureBlock); runs when the assertion fails.
	Block *EnsureBlockNode
}

// IsCallSubject reports whether the ensure subject is a call (not a place).
func (e EnsureNode) IsCallSubject() bool {
	return e.Subject != nil
}

// EnsureSubject returns the subject expression (call Subject, else place Variable).
func (e EnsureNode) EnsureSubject() ExpressionNode {
	if e.Subject != nil {
		return e.Subject
	}
	return e.Variable
}

// EnsurePlaceSubject returns the place Variable when the subject is a place.
func (e EnsureNode) EnsurePlaceSubject() (VariableNode, bool) {
	if e.Subject != nil {
		return VariableNode{}, false
	}
	return e.Variable, true
}

// EnsureSubjectSpan is the best-known span for diagnostics on the ensure subject.
func (e EnsureNode) EnsureSubjectSpan() SourceSpan {
	if e.Subject != nil {
		return ExpressionSpanStart(e.Subject)
	}
	return e.Variable.Ident.Span
}

// FailureBlock is the ensure failure block (AST name from analyzable-refinements phase 1).
type FailureBlock = EnsureBlockNode

// RefinementTarget is the RHS of `ensure … is` / `if … is`.
type RefinementTarget interface {
	refinementTarget()
	String() string
}

// TypeTarget is a bare type name after `is` (no parentheses).
type TypeTarget struct {
	Name TypeIdent // named type / literal-union domain
}

func (TypeTarget) refinementTarget() {}

func (t TypeTarget) String() string { return string(t.Name) }

// AssertionTarget is one or more constraint chains joined by `or` (Any / Join).
type AssertionTarget struct {
	Chains []AssertionNode // Meet chains; index 0 is primary, rest are Join alts
}

func (AssertionTarget) refinementTarget() {}

func (a AssertionTarget) String() string {
	if len(a.Chains) == 0 {
		return ""
	}
	s := a.Chains[0].String()
	for i := 1; i < len(a.Chains); i++ {
		s += " or " + a.Chains[i].String()
	}
	return s
}

// EnsureBlockNode represents a block of statements for an ensure statement
type EnsureBlockNode struct {
	Body []Node
}

// EnsureErrorNode represents an error node for an ensure statement, can be a call or a variable
type EnsureErrorNode interface {
	String() string
}

// EnsureErrorCall represents an error call for an ensure statement
type EnsureErrorCall struct {
	ErrorType string
	ErrorArgs []ExpressionNode
}

func (e EnsureErrorCall) String() string {
	return fmt.Sprintf("%s(%v)", e.ErrorType, e.ErrorArgs)
}

// EnsureErrorVar represents an error variable for an ensure statement
type EnsureErrorVar string

func (e EnsureErrorVar) String() string {
	return string(e)
}

// EnsureErrorExpr is a general ensure-else failure expression (method call, etc.).
type EnsureErrorExpr struct {
	Expr ExpressionNode
}

func (e EnsureErrorExpr) String() string {
	if e.Expr == nil {
		return "EnsureErrorExpr"
	}
	return e.Expr.String()
}

// Kind returns the node kind for an ensure statement
func (e EnsureNode) Kind() NodeKind {
	return NodeKindEnsure
}

func (e EnsureNode) String() string {
	subj := e.EnsureSubject().String()
	if e.Implicit == EnsureImplicitBang {
		if e.Error == nil {
			return fmt.Sprintf("Ensure(!%s)", subj)
		}
		return fmt.Sprintf("Ensure(!%s, %s)", subj, (*e.Error).String())
	}
	if e.Implicit == EnsureImplicitBare {
		if e.Error == nil {
			return fmt.Sprintf("Ensure(%s)", subj)
		}
		return fmt.Sprintf("Ensure(%s, %s)", subj, (*e.Error).String())
	}
	target := e.Assertion.String()
	if e.Target != nil {
		target = e.Target.String()
	}
	if e.Error == nil {
		return fmt.Sprintf("Ensure(%s, %s)", subj, target)
	}
	return fmt.Sprintf("Ensure(%s, %s, %s)", subj, target, (*e.Error).String())
}

// String returns a string representation of the ensure block
func (e EnsureBlockNode) String() string {
	return "EnsureBlock"
}

// Kind returns the node kind for an ensure block
func (e EnsureBlockNode) Kind() NodeKind {
	return NodeKindEnsureBlock
}

// ConstraintOnlyAssertion builds a bare constraint assertion (True/False/Ok/Nil/…).
func ConstraintOnlyAssertion(name string) AssertionNode {
	return AssertionNode{
		Constraints: []ConstraintNode{{
			Name: name,
			Args: []ConstraintArgumentNode{},
		}},
	}
}
