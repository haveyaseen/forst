package typechecker

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"forst/internal/ast"
	"forst/internal/diag"

	"github.com/sirupsen/logrus"
)

// Diagnostic is a type-check error with a source span for editors and LSP.
type Diagnostic struct {
	Span    ast.SourceSpan
	Code    string
	Title   string
	Problem string
	Help    string
	Notes   []string
	Fixes   []diag.Fix
	Related []RelatedDiagnostic
}

// RelatedDiagnostic links a primary diagnostic to another source location (e.g. Providers obligation chain).
type RelatedDiagnostic struct {
	Msg  string
	Span ast.SourceSpan
}

func (d *Diagnostic) Error() string {
	if d == nil {
		return ""
	}
	return diag.FormatReport(diag.Report{
		Code:    d.Code,
		Title:   d.Title,
		Problem: d.Problem,
		Help:    d.Help,
		Notes:   d.Notes,
		Fixes:   d.Fixes,
	})
}

// Report returns the structured report view (for LSP data / fixes).
func (d *Diagnostic) Report() diag.Report {
	if d == nil {
		return diag.Report{}
	}
	return diag.Report{
		Code:    d.Code,
		Title:   d.Title,
		Problem: d.Problem,
		Help:    d.Help,
		Notes:   d.Notes,
		Fixes:   d.Fixes,
	}
}

// spanOfExpression returns the best-known span for an expression (parser may omit for some nodes).
func spanOfExpression(expr ast.ExpressionNode) ast.SourceSpan {
	if expr == nil {
		return ast.SourceSpan{}
	}
	switch e := expr.(type) {
	case ast.FunctionCallNode:
		if e.CallSpan.IsSet() {
			return e.CallSpan
		}
		if e.Function.Span.IsSet() {
			return e.Function.Span
		}
		for _, arg := range e.Arguments {
			if s := spanOfExpression(arg); s.IsSet() {
				return s
			}
		}
	case ast.MethodCallNode:
		if e.CallSpan.IsSet() {
			return e.CallSpan
		}
		if e.Method.Span.IsSet() {
			return e.Method.Span
		}
		return spanOfExpression(e.Receiver)
	case ast.VariableNode:
		if e.Ident.Span.IsSet() {
			return e.Ident.Span
		}
	case ast.FieldAccessNode:
		if e.Field.Span.IsSet() {
			return e.Field.Span
		}
		return spanOfExpression(e.Target)
	case ast.ErrExprNode:
		if e.Span.IsSet() {
			return e.Span
		}
		if e.Value != nil {
			return spanOfExpression(e.Value)
		}
	case ast.OkExprNode:
		if e.Span.IsSet() {
			return e.Span
		}
		if e.Value != nil {
			return spanOfExpression(e.Value)
		}
	case ast.IntLiteralNode:
		return e.Span
	case ast.FloatLiteralNode:
		return e.Span
	case ast.StringLiteralNode:
		return e.Span
	case ast.RuneLiteralNode:
		return e.Span
	case ast.BoolLiteralNode:
		return e.Span
	case ast.ArrayLiteralNode:
		if e.Span.IsSet() {
			return e.Span
		}
		for _, el := range e.Value {
			if s := spanOfExpression(el); s.IsSet() {
				return s
			}
		}
	case ast.MapLiteralNode:
		if e.Span.IsSet() {
			return e.Span
		}
		for _, entry := range e.Entries {
			if key, ok := entry.Key.(ast.ExpressionNode); ok {
				if s := spanOfExpression(key); s.IsSet() {
					return s
				}
			}
			if val, ok := entry.Value.(ast.ExpressionNode); ok {
				if s := spanOfExpression(val); s.IsSet() {
					return s
				}
			}
		}
	case ast.NilLiteralNode:
		return e.Span
	case ast.IotaLiteralNode:
		return e.Span
	case ast.UnaryExpressionNode:
		return spanOfExpression(e.Operand)
	case ast.BinaryExpressionNode:
		return firstSetSpan(spanOfExpression(e.Left), spanOfExpression(e.Right))
	case ast.IndexExpressionNode:
		if e.Span.IsSet() {
			return e.Span
		}
		return firstSetSpan(spanOfExpression(e.Target), spanOfExpression(e.Index))
	case ast.SliceExpressionNode:
		if e.Span.IsSet() {
			return e.Span
		}
		return firstSetSpan(spanOfExpression(e.Target), spanOfExpression(e.Low), spanOfExpression(e.High))
	case ast.SpreadExpressionNode:
		return spanOfExpression(e.Expr)
	case ast.TypeExpressionNode:
		return e.Span
	case ast.ShapeNode:
		return e.Span
	case ast.FunctionLiteralNode:
		return e.Span
	case ast.ReferenceNode:
		if e.Value != nil {
			return spanOfExpression(e.Value)
		}
	case ast.DereferenceNode:
		if e.Value != nil {
			return spanOfExpression(e.Value)
		}
	case *ast.FunctionCallNode:
		if e == nil {
			return ast.SourceSpan{}
		}
		return spanOfExpression(*e)
	case *ast.VariableNode:
		if e == nil {
			return ast.SourceSpan{}
		}
		return spanOfExpression(*e)
	case *ast.ErrExprNode:
		if e == nil {
			return ast.SourceSpan{}
		}
		return spanOfExpression(*e)
	case *ast.OkExprNode:
		if e == nil {
			return ast.SourceSpan{}
		}
		return spanOfExpression(*e)
	}
	return ast.SourceSpan{}
}

// importNodeSpan returns the best-known span for an import statement.
func importNodeSpan(imp ast.ImportNode) ast.SourceSpan {
	if imp.Span.IsSet() {
		return imp.Span
	}
	if imp.Alias != nil && imp.Alias.Span.IsSet() {
		return imp.Alias.Span
	}
	return ast.SourceSpan{}
}

// constraintSpan returns the span for a constraint name/call.
func constraintSpan(c ast.ConstraintNode) ast.SourceSpan {
	if c.Span.IsSet() {
		return c.Span
	}
	return ast.SourceSpan{}
}

// spanOfShapeField returns a span for a shape field when available.
func spanOfShapeField(field ast.ShapeFieldNode) ast.SourceSpan {
	if field.Node != nil {
		if s := spanOfNode(field.Node); s.IsSet() {
			return s
		}
	}
	if field.TagSpan.IsSet() {
		return field.TagSpan
	}
	return ast.SourceSpan{}
}

// spanOfNode returns a span from an AST node used as an expression subject (is/ensure LHS, etc.).
func spanOfNode(n ast.Node) ast.SourceSpan {
	if n == nil {
		return ast.SourceSpan{}
	}
	if e, ok := n.(ast.ExpressionNode); ok {
		if s := spanOfExpression(e); s.IsSet() {
			return s
		}
	}
	switch v := n.(type) {
	case ast.VariableNode:
		return v.Ident.Span
	case *ast.VariableNode:
		if v != nil {
			return v.Ident.Span
		}
	case ast.ReturnNode:
		for _, val := range v.Values {
			if s := spanOfExpression(val); s.IsSet() {
				return s
			}
		}
	case *ast.ReturnNode:
		if v != nil {
			return spanOfNode(*v)
		}
	case ast.AssignmentNode:
		for _, lv := range v.LValues {
			if s := spanOfExpression(lv); s.IsSet() {
				return s
			}
		}
		for _, rv := range v.RValues {
			if s := spanOfExpression(rv); s.IsSet() {
				return s
			}
		}
	case *ast.AssignmentNode:
		if v != nil {
			return spanOfNode(*v)
		}
	case ast.ForNode:
		if v.Label != nil {
			return v.Label.Span
		}
	case *ast.ForNode:
		if v != nil && v.Label != nil {
			return v.Label.Span
		}
	case ast.IfNode:
		return spanOfNode(v.Condition)
	case *ast.IfNode:
		if v != nil {
			return spanOfNode(v.Condition)
		}
	case ast.EnsureNode:
		return v.Variable.Ident.Span
	case *ast.EnsureNode:
		if v != nil {
			return spanOfNode(*v)
		}
	case ast.GotoNode:
		if v.Label != nil && v.Label.Span.IsSet() {
			return v.Label.Span
		}
		return v.Span
	case *ast.GotoNode:
		if v != nil {
			return spanOfNode(*v)
		}
	case ast.BinaryExpressionNode:
		return firstSetSpan(spanOfNode(v.Left), spanOfNode(v.Right))
	case *ast.BinaryExpressionNode:
		if v != nil {
			return firstSetSpan(spanOfNode(v.Left), spanOfNode(v.Right))
		}
	}
	return ast.SourceSpan{}
}

// spanForCallArg prefers ArgSpans[i], then spanOfExpression(args[i]), then callSpan.
func spanForCallArg(argSpans []ast.SourceSpan, i int, args []ast.ExpressionNode, callSpan ast.SourceSpan) ast.SourceSpan {
	if i < len(argSpans) && argSpans[i].IsSet() {
		return argSpans[i]
	}
	if i < len(args) {
		if s := spanOfExpression(args[i]); s.IsSet() {
			return s
		}
	}
	if callSpan.IsSet() {
		return callSpan
	}
	return ast.SourceSpan{}
}

// firstSetSpan returns the first set span among candidates.
func firstSetSpan(spans ...ast.SourceSpan) ast.SourceSpan {
	for _, s := range spans {
		if s.IsSet() {
			return s
		}
	}
	return ast.SourceSpan{}
}

// lastDottedSegmentSpan narrows a full dotted Ident span (e.g. `u.nme`) to the last segment (`nme`)
// when the range is on one line. Falls back to full when unset or multi-line.
func lastDottedSegmentSpan(full ast.SourceSpan, lastSeg string) ast.SourceSpan {
	if !full.IsSet() || lastSeg == "" {
		return full
	}
	if full.StartLine != full.EndLine {
		return full
	}
	w := utf8.RuneCountInString(lastSeg)
	if w < 1 {
		w = 1
	}
	start := full.EndCol - w
	if start < full.StartCol {
		return full
	}
	return ast.SourceSpan{
		StartLine: full.StartLine,
		StartCol:  start,
		EndLine:   full.EndLine,
		EndCol:    full.EndCol,
	}
}

// spanIndexExpr returns the best span for a subscript expression.
func spanIndexExpr(e ast.IndexExpressionNode) ast.SourceSpan {
	if e.Span.IsSet() {
		return e.Span
	}
	return firstSetSpan(spanOfExpression(e.Index), spanOfExpression(e.Target))
}

// spanSliceExpr returns the best span for a slice expression.
func spanSliceExpr(e ast.SliceExpressionNode) ast.SourceSpan {
	if e.Span.IsSet() {
		return e.Span
	}
	spans := []ast.SourceSpan{spanOfExpression(e.Target)}
	if e.Low != nil {
		spans = append(spans, spanOfExpression(e.Low))
	}
	if e.High != nil {
		spans = append(spans, spanOfExpression(e.High))
	}
	return firstSetSpan(spans...)
}

// reportf emits a structured Elm/Rust-style diagnostic.
func reportf(span ast.SourceSpan, code, title, problem, help string, notes ...string) error {
	mustSpan(span, code)
	return &Diagnostic{
		Code:    code,
		Title:   title,
		Problem: problem,
		Help:    help,
		Notes:   notes,
		Span:    span,
	}
}

// reportfRelated is reportf plus related locations (Providers chains, etc.).
func reportfRelated(span ast.SourceSpan, code string, related []RelatedDiagnostic, title, problem, help string, notes ...string) error {
	mustSpan(span, code)
	return &Diagnostic{
		Code:    code,
		Title:   title,
		Problem: problem,
		Help:    help,
		Notes:   notes,
		Span:    span,
		Related: related,
	}
}

// reportWithFixes is reportf plus machine Fixes for LSP quickfixes.
func reportWithFixes(span ast.SourceSpan, code, title, problem, help string, fixes []diag.Fix, notes ...string) error {
	mustSpan(span, code)
	return &Diagnostic{
		Code:    code,
		Title:   title,
		Problem: problem,
		Help:    help,
		Notes:   notes,
		Fixes:   fixes,
		Span:    span,
	}
}

// mustSpan logs when a user-facing diagnostic is missing a source span (IDE falls back to package line).
func mustSpan(span ast.SourceSpan, code string) {
	if span.IsSet() {
		return
	}
	logrus.WithField("code", code).Error("diagnostic emitted without SourceSpan; LSP may squiggle package line — fix span plumbing at emit site")
}

// reportBody builds a diagnostic from a free-form body string (title; help / hint lines).
// Prefer explicit reportf at call sites; this exists for dense emit tables (builtins, go-call).
func reportBody(span ast.SourceSpan, code, body string) error {
	title, problem, help, notes := partitionBody(code, body)
	if help == "" {
		help = defaultHelp(code)
	}
	return reportf(span, code, title, problem, help, notes...)
}

func reportBodyRelated(span ast.SourceSpan, code string, related []RelatedDiagnostic, body string) error {
	title, problem, help, notes := partitionBody(code, body)
	if help == "" {
		help = defaultHelp(code)
	}
	return reportfRelated(span, code, related, title, problem, help, notes...)
}

func reportBodyf(span ast.SourceSpan, code, format string, a ...any) error {
	return reportBody(span, code, fmt.Sprintf(format, a...))
}

func reportBodyfRelated(span ast.SourceSpan, code string, related []RelatedDiagnostic, format string, a ...any) error {
	return reportBodyRelated(span, code, related, fmt.Sprintf(format, a...))
}

func partitionBody(code, body string) (title, problem, help string, notes []string) {
	body = strings.TrimSpace(body)
	if code != "" {
		for _, p := range []string{code + ": ", code + ":", code + " "} {
			if strings.HasPrefix(body, p) {
				body = strings.TrimSpace(body[len(p):])
				break
			}
		}
	}
	if i := strings.Index(body, "\n  hint:"); i >= 0 {
		help = strings.TrimSpace(body[i+len("\n  hint:"):])
		body = strings.TrimSpace(body[:i])
		if j := strings.Index(help, "\n  note:"); j >= 0 {
			notes = append(notes, strings.TrimSpace(help[j+len("\n  note:"):]))
			help = strings.TrimSpace(help[:j])
		}
	}
	for {
		i := strings.Index(body, "\n  note:")
		if i < 0 {
			break
		}
		rest := strings.TrimSpace(body[i+len("\n  note:"):])
		body = strings.TrimSpace(body[:i])
		if k := strings.Index(rest, "\n  note:"); k >= 0 {
			notes = append(notes, strings.TrimSpace(rest[:k]))
			body = body + "\n  note:" + rest[k:]
			continue
		}
		notes = append(notes, rest)
		break
	}
	body = strings.TrimSpace(body)
	if i := strings.IndexByte(body, '\n'); i >= 0 {
		title = strings.TrimSpace(body[:i])
		problem = strings.TrimSpace(body[i+1:])
	} else if i := strings.Index(body, "; "); i >= 0 {
		title = strings.TrimSpace(body[:i])
		tail := strings.TrimSpace(body[i+2:])
		if help == "" {
			help = tail
		} else {
			problem = tail
		}
	} else {
		title = body
	}
	return title, problem, help, notes
}

func defaultHelp(code string) string {
	switch {
	case strings.HasPrefix(code, "providers-"):
		return "add the missing provider to the with { … } block or ciProviders bundle"
	case strings.HasPrefix(code, "refinement-"):
		return "adjust the assertion, ensure, or type guard to match Forst refinement rules"
	case strings.HasPrefix(code, "go-"):
		return "check the Go import path, exported name, and argument types"
	case strings.HasPrefix(code, "js-"):
		return "check the JS/TS import path and that the bridge index is up to date"
	case strings.HasPrefix(code, "call-") || strings.HasPrefix(code, "generic-"):
		return "fix the call arity or supply explicit type arguments"
	case code == "error-struct-syntax":
		return "use a Go-style struct literal: `E{}` or `E{ field: value }`"
	case code == "error-payload":
		return "match the fields declared on the error type"
	default:
		return "fix the types or expression at this location and try again"
	}
}
