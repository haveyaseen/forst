package transformergo

import (
	"fmt"
	"strconv"

	"forst/internal/ast"
	goast "go/ast"
	"go/token"
)

func ensureSubjectLabel(v ast.VariableNode) string {
	return string(v.Ident.ID)
}

func ensureNodeSubjectLabel(stmt ast.EnsureNode) string {
	if stmt.IsCallSubject() {
		return stmt.Subject.String()
	}
	return ensureSubjectLabel(stmt.Variable)
}

func constraintArgDisplay(arg ast.ConstraintArgumentNode) string {
	return arg.String()
}

type ensureGotDiagnostic struct {
	suffixFormat string
	gotArgs      []goast.Expr
}

func defaultGotDiagnostic(subjectExpr goast.Expr) ensureGotDiagnostic {
	return ensureGotDiagnostic{
		suffixFormat: "got %v, want %s",
		gotArgs:      []goast.Expr{subjectExpr},
	}
}

func (t *Transformer) ensureSubjectVarType(stmt ast.EnsureNode) (ast.TypeNode, bool) {
	ty, err := t.lookupEnsureSubjectTypeForEmit(stmt)
	if err != nil {
		return ast.TypeNode{}, false
	}
	return ty, true
}

func (t *Transformer) isStringLikeType(tn ast.TypeNode) bool {
	if tn.Ident == ast.TypeString {
		return true
	}
	return t.typeIsStringAlias(tn.Ident)
}

func (t *Transformer) ensureResultGotDiagnostic(variable ast.VariableNode, c ast.ConstraintNode, subjectExpr goast.Expr) ensureGotDiagnostic {
	errExpr, err := t.goResultErrIdentForExpr(variable)
	if err != nil {
		return defaultGotDiagnostic(subjectExpr)
	}
	if c.Name == "Ok" && len(c.Args) == 1 {
		valExpr, err := t.goResultSuccessValueExprForOkDiscriminator(variable)
		if err != nil {
			return ensureGotDiagnostic{
				suffixFormat: "got err=%v, want %s",
				gotArgs:      []goast.Expr{errExpr},
			}
		}
		return ensureGotDiagnostic{
			suffixFormat: "got err=%v, val=%v, want %s",
			gotArgs:      []goast.Expr{errExpr, valExpr},
		}
	}
	return ensureGotDiagnostic{
		suffixFormat: "got err=%v, want %s",
		gotArgs:      []goast.Expr{errExpr},
	}
}

// ensureConstraintGotDiagnostic picks Fatalf got-clause format and expressions for the failed ensure.
func (t *Transformer) ensureConstraintGotDiagnostic(stmt ast.EnsureNode, subjectExpr goast.Expr) ensureGotDiagnostic {
	assertion := stmt.Assertion
	if len(assertion.Constraints) != 1 {
		return defaultGotDiagnostic(subjectExpr)
	}
	c := assertion.Constraints[0]

	switch c.Name {
	case string(TrueConstraint), string(FalseConstraint):
		return ensureGotDiagnostic{
			suffixFormat: "got %t, want %s",
			gotArgs:      []goast.Expr{subjectExpr},
		}
	case "Ok", "Err":
		return t.ensureResultGotDiagnostic(stmt.Variable, c, subjectExpr)
	}

	varType, hasType := t.ensureSubjectVarType(stmt)
	stringLike := hasType && t.isStringLikeType(varType)
	if !stringLike && assertion.BaseType != nil {
		stringLike = t.typeIsStringAlias(*assertion.BaseType)
	}
	if stringLike {
		stringExpr, err := t.assertionTransformer.transformStringBuiltinVariable(stmt.Variable)
		if err != nil {
			return defaultGotDiagnostic(subjectExpr)
		}
		switch c.Name {
		case string(MinConstraint), string(MaxConstraint):
			t.Output.EnsureImport("unicode/utf8")
			return ensureGotDiagnostic{
				suffixFormat: "got runes=%d (%q), want %s",
				gotArgs: []goast.Expr{
					&goast.CallExpr{
						Fun: &goast.SelectorExpr{
							X:   goast.NewIdent("utf8"),
							Sel: goast.NewIdent("RuneCountInString"),
						},
						Args: []goast.Expr{stringExpr},
					},
					stringExpr,
				},
			}
		case string(MinBytesConstraint), string(MaxBytesConstraint):
			return ensureGotDiagnostic{
				suffixFormat: "got len=%d (%q), want %s",
				gotArgs: []goast.Expr{
					&goast.CallExpr{
						Fun:  goast.NewIdent("len"),
						Args: []goast.Expr{stringExpr},
					},
					stringExpr,
				},
			}
		case string(HasPrefixConstraint), string(HasSuffixConstraint), string(ContainsConstraint), string(NotEmptyConstraint):
			return ensureGotDiagnostic{
				suffixFormat: "got %q, want %s",
				gotArgs:      []goast.Expr{stringExpr},
			}
		}
	}

	return defaultGotDiagnostic(subjectExpr)
}

// ensureConstraintWantHint returns a human-readable expected-value description from ensure constraints.
func (t *Transformer) ensureConstraintWantHint(assertion *ast.AssertionNode) string {
	if assertion.BaseType != nil && len(assertion.Constraints) == 0 {
		return fmt.Sprintf("%s()", assertion.BaseType.String())
	}
	if len(assertion.Constraints) == 0 {
		return t.getAssertionStringForError(assertion)
	}
	if len(assertion.Constraints) > 1 {
		return t.getAssertionStringForError(assertion)
	}
	c := assertion.Constraints[0]
	switch c.Name {
	case string(TrueConstraint):
		return "true"
	case string(FalseConstraint):
		return "false"
	case string(NilConstraint):
		return "nil"
	case string(PresentConstraint):
		return "non-nil"
	case string(NotEmptyConstraint):
		return "non-empty"
	case "Ok":
		return "Ok()"
	case "Err":
		return "Err()"
	case string(GreaterThanConstraint):
		if len(c.Args) >= 1 {
			return "> " + constraintArgDisplay(c.Args[0])
		}
	case string(LessThanConstraint):
		if len(c.Args) >= 1 {
			return "< " + constraintArgDisplay(c.Args[0])
		}
	case string(MinConstraint):
		if len(c.Args) >= 1 {
			return ">= " + constraintArgDisplay(c.Args[0])
		}
	case string(MaxConstraint):
		if len(c.Args) >= 1 {
			return "<= " + constraintArgDisplay(c.Args[0])
		}
	case string(HasPrefixConstraint):
		if len(c.Args) >= 1 {
			return fmt.Sprintf("prefix %s", constraintArgDisplay(c.Args[0]))
		}
	case string(HasSuffixConstraint):
		if len(c.Args) >= 1 {
			return fmt.Sprintf("suffix %s", constraintArgDisplay(c.Args[0]))
		}
	case string(ContainsConstraint):
		if len(c.Args) >= 1 {
			return fmt.Sprintf("contains %s", constraintArgDisplay(c.Args[0]))
		}
	case string(MinBytesConstraint):
		if len(c.Args) >= 1 {
			return ">= " + constraintArgDisplay(c.Args[0]) + " bytes"
		}
	case string(MaxBytesConstraint):
		if len(c.Args) >= 1 {
			return "<= " + constraintArgDisplay(c.Args[0]) + " bytes"
		}
	case string(FiniteConstraint):
		return "finite"
	}
	return t.getAssertionStringForError(assertion)
}

func (t *Transformer) ensureFailureMessage(stmt ast.EnsureNode) (subjectLabel, assertionLabel, wantHint string) {
	subjectLabel = ensureNodeSubjectLabel(stmt)
	assertionLabel = t.getAssertionStringForError(&stmt.Assertion)
	wantHint = t.ensureConstraintWantHint(&stmt.Assertion)
	return subjectLabel, assertionLabel, wantHint
}

func goQuotedStringLit(s string) *goast.BasicLit {
	return &goast.BasicLit{Kind: token.STRING, Value: strconv.Quote(s)}
}

func testFatalfCall(testIdent *goast.Ident, format string, args ...goast.Expr) *goast.ExprStmt {
	callArgs := make([]goast.Expr, 0, 1+len(args))
	callArgs = append(callArgs, goQuotedStringLit(format))
	callArgs = append(callArgs, args...)
	return &goast.ExprStmt{
		X: &goast.CallExpr{
			Fun: &goast.SelectorExpr{
				X:   testIdent,
				Sel: goast.NewIdent("Fatalf"),
			},
			Args: callArgs,
		},
	}
}

// ensureTestFatalfCall emits t.Fatalf with got/want diagnostics for a failed ensure in a test function.
func (t *Transformer) ensureTestFatalfCall(testIdent *goast.Ident, stmt ast.EnsureNode) goast.Stmt {
	subjectLabel, assertionLabel, wantHint := t.ensureFailureMessage(stmt)
	subjectExpr, err := t.transformExpression(stmt.EnsureSubject())
	if err != nil {
		return testFatalfCall(testIdent, "ensure %s is %s",
			goQuotedStringLit(subjectLabel),
			goQuotedStringLit(assertionLabel),
		)
	}
	got := t.ensureConstraintGotDiagnostic(stmt, subjectExpr)
	format := "ensure %s is %s: " + got.suffixFormat
	args := []goast.Expr{
		goQuotedStringLit(subjectLabel),
		goQuotedStringLit(assertionLabel),
	}
	args = append(args, got.gotArgs...)
	args = append(args, goQuotedStringLit(wantHint))
	return testFatalfCall(testIdent, format, args...)
}
