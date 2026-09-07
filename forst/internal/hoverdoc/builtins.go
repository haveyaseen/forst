package hoverdoc

import (
	"fmt"
	"strings"

	"forst/internal/ast"
)

// Builtin type names as written in Forst (capitalized keywords and related spellings).
const (
	NameInt     = "Int"
	NameFloat   = "Float"
	NameString  = "String"
	NameBool    = "Bool"
	NameVoid    = "Void"
	NameArray   = "Array"
	NameMap     = "Map"
	NameStruct  = "struct"
	NameError   = "Error"
	NamePointer = "Pointer"
	NameShape   = "Shape"
	NameResult  = "Result"
	NameTuple   = "Tuple"
	NameObject  = "Object"
	NameBytes   = "Bytes"
	NameComplex64  = "Complex64"
	NameComplex128 = "Complex128"
	NameRune    = "Rune"
)

// BuiltinTypeMarkdown returns user-facing markdown for a built-in Forst type, keyed by its
// surface spelling (e.g. "Int", "Result"). Unknown names yield an empty string.
func BuiltinTypeMarkdown(displayName string) string {
	return builtinTypeDocs[displayName]
}

// IsBuiltinTypeSurfaceName reports whether name is a documented built-in type spelling (e.g. "Int",
// "Result"). Used when the name appears as an identifier (e.g. in `Array(Result)`).
func IsBuiltinTypeSurfaceName(name string) bool {
	_, ok := builtinTypeDocs[name]
	return ok
}

// MarkdownForKeywordToken returns markdown for lexer keyword tokens that correspond to built-in
// types or language keywords documented for hovers. Unknown tokens yield "".
func MarkdownForKeywordToken(t ast.TokenIdent) string {
	switch t {
	case ast.TokenInt:
		return BuiltinTypeMarkdown(NameInt)
	case ast.TokenFloat:
		return BuiltinTypeMarkdown(NameFloat)
	case ast.TokenString:
		return BuiltinTypeMarkdown(NameString)
	case ast.TokenBool:
		return BuiltinTypeMarkdown(NameBool)
	case ast.TokenVoid:
		return BuiltinTypeMarkdown(NameVoid)
	case ast.TokenArray:
		return BuiltinTypeMarkdown(NameArray)
	case ast.TokenMap:
		return BuiltinTypeMarkdown(NameMap)
	case ast.TokenStruct:
		return BuiltinTypeMarkdown(NameStruct)
	case ast.TokenFunc:
		return keywordDoc("func", strings.Join([]string{
			"Declares a function. Parameters look like `name: Type`; the return type comes after the closing `)` before `{`.",
			"",
			"**Example**",
			"",
			forstBlock(
				"func greet(name: String): String {",
				"  return \"Hello, \" + name",
				"}",
			),
		}, "\n"))
	case ast.TokenType:
		return keywordDoc("type", strings.Join([]string{
			"Introduces a name for a shape, alias, nominal error, or type guard. Shapes use `{ field: Type }` syntax.",
			"",
			"**Examples**",
			"",
			forstBlock(
				"type User = {",
				"  id: Int",
				"  name: String",
				"}",
			),
		}, "\n"))
	case ast.TokenReturn:
		return keywordDoc("return", strings.Join([]string{
			"Exits the function and sends a value (or values) back to the caller. The expression must match the declared return type.",
			"",
			"**Example**",
			"",
			forstBlock(
				"func double(x: Int): Int {",
				"  return x * 2",
				"}",
			),
		}, "\n"))
	case ast.TokenImport:
		return keywordDoc("import", strings.Join([]string{
			"Loads a Go package so you can call it with `localName.Symbol` (for example `fmt.Println`).",
			"",
			"**Example**",
			"",
			forstBlock(
				`import "fmt"`,
				"",
				"func main(): Void {",
				"  fmt.Println(\"hi\")",
				"}",
			),
		}, "\n"))
	case ast.TokenPackage:
		return keywordDoc("package", strings.Join([]string{
			"First line of a file: declares which Go package this file belongs to (usually `main` for programs).",
			"",
			"**Example**",
			"",
			forstBlock(
				"package main",
				"",
				"func main(): Void { }",
			),
		}, "\n"))
	case ast.TokenEnsure:
		return keywordDoc("ensure", strings.Join([]string{
			"Runtime check: if the assertion fails, execution takes the failure path (`else <error>` or a failure block). When the check passes, types can narrow for the rest of the function.",
			"",
			"`or` joins assertion alternatives on one place (`ensure x is A() or B() else E{}`). Typed failure uses `else`, not `or`.",
			"",
			"**Example**",
			"",
			forstBlock(
				"func use(s: String): Result(Void, Error) {",
				"  ensure s is Min(1) else TooShort{message: \"empty\"}",
				"  // here `s` is known non-empty for the compiler",
				"}",
			),
		}, "\n"))
	case ast.TokenIs:
		return keywordDoc("is", strings.Join([]string{
			"Introduces a guard or refinement target on the left-hand side. Used in `if x is …` (branch narrowing) and `ensure … is …` (must hold here).",
			"",
			"A bare type name is a type target; `Name()` is an assertion. Assertion alternatives join with `or`.",
			"",
			"**Examples**",
			"",
			forstBlock(
				"if v is Ok() {",
				"  // success branch",
				"}",
				"",
				"ensure n is Max(100) else TooBig{}",
			),
		}, "\n"))
	case ast.TokenOr:
		return keywordDoc("or", strings.Join([]string{
			"Joins assertion constraint chains on one place: `ensure x is A() or B() else E{}`. Not typed failure and not boolean `||`.",
			"",
			"For typed failure after `ensure`, use `else`. For boolean OR in expressions, use `||`.",
		}, "\n"))
	case ast.TokenIf:
		return keywordDoc("if", strings.Join([]string{
			"Runs a block when the condition is true. With `subject is …`, the typechecker narrows types inside the then-branch.",
			"",
			"**Example**",
			"",
			forstBlock(
				"if x is Err() {",
				"  // handle failure",
				"}",
			),
		}, "\n"))
	case ast.TokenElseIf:
		return keywordDoc("else if", strings.Join([]string{
			"Tries another condition after a failed `if`. Only one branch runs—the first that matches.",
			"",
			"**Example**",
			"",
			forstBlock(
				"if x is Ok() { }",
				"else if x is Err() { }",
			),
		}, "\n"))
	case ast.TokenElse:
		return keywordDoc("else", strings.Join([]string{
			"After `ensure … is …`, `else <error>` is the typed failure path. Also the optional final branch of `if` when no prior arm matched.",
			"",
			"**Examples**",
			"",
			forstBlock(
				"ensure password is Strong() or Passkey() else InvalidCredential{}",
				"",
				"if cond { }",
				"else { }",
			),
		}, "\n"))
	case ast.TokenFor:
		return keywordDoc("for", strings.Join([]string{
			"C-style loops, `while`-style loops, or paired with `range` to iterate.",
			"",
			"**Examples**",
			"",
			forstBlock(
				"for i := 0; i < n; i++ { }",
				"for cond { }",
				"for i, v := range items { }",
			),
		}, "\n"))
	case ast.TokenRange:
		return keywordDoc("range", strings.Join([]string{
			"Used with `for` to walk a slice, string, or map—similar to Go.",
			"",
			"**Example**",
			"",
			forstBlock(
				"for i, ch := range s {",
				"  _ = i",
				"  _ = ch",
				"}",
			),
		}, "\n"))
	case ast.TokenBreak:
		return keywordDoc("break", strings.Join([]string{
			"Leaves the innermost `for` or `switch` immediately.",
		}, "\n"))
	case ast.TokenContinue:
		return keywordDoc("continue", strings.Join([]string{
			"Skips to the next iteration of the innermost `for` loop.",
		}, "\n"))
	case ast.TokenSwitch:
		return keywordDoc("switch", strings.Join([]string{
			"Chooses one arm by comparing a value against `case` labels; optional `default` if nothing matched.",
		}, "\n"))
	case ast.TokenCase:
		return keywordDoc("case", strings.Join([]string{
			"Labels one arm of a `switch`. Compared in order until a match.",
		}, "\n"))
	case ast.TokenDefault:
		return keywordDoc("default", strings.Join([]string{
			"Runs when no `case` in this `switch` matched.",
		}, "\n"))
	case ast.TokenFallthrough:
		return keywordDoc("fallthrough", strings.Join([]string{
			"Continues into the *next* `case` body (Go semantics). Rare; use only when you intend fall-through.",
		}, "\n"))
	case ast.TokenVar:
		return keywordDoc("var", strings.Join([]string{
			"Go-style variable declaration for interop. In idiomatic Forst, prefer `name := expr` or typed bindings where the language supports them.",
		}, "\n"))
	case ast.TokenConst:
		return keywordDoc("const", strings.Join([]string{
			"Declares a compile-time constant (Go interop).",
		}, "\n"))
	case ast.TokenChan:
		return keywordDoc("chan", strings.Join([]string{
			"Channel types for concurrent code that lowers to Go `chan`.",
			"",
			"**Example**",
			"",
			forstBlock("ch: chan Int"),
		}, "\n"))
	case ast.TokenInterface:
		return keywordDoc("interface", strings.Join([]string{
			"Declares a Go `interface{ ... }` type for FFI.",
		}, "\n"))
	case ast.TokenGo:
		return keywordDoc("go", strings.Join([]string{
			"Starts a goroutine: `go someCall()` → Go `go` statement.",
		}, "\n"))
	case ast.TokenDefer:
		return keywordDoc("defer", strings.Join([]string{
			"Schedules a call to run when the surrounding function returns—same idea as Go `defer`.",
		}, "\n"))
	case ast.TokenGoto:
		return keywordDoc("goto", strings.Join([]string{
			"Jumps to a label (Go `goto`). Uncommon; prefer structured control flow.",
		}, "\n"))
	case ast.TokenIota:
		return keywordDoc("iota", strings.Join([]string{
			"Predeclared identifier in `const` groups. Starts at `0` and increments by one for each successive spec.",
			"",
			"**Example**",
			"",
			forstBlock(
				"const (",
				"  A = iota // 0",
				"  B        // 1",
				")",
			),
		}, "\n"))
	case ast.TokenEllipsis:
		return keywordDoc("...", strings.Join([]string{
			"Variadic parameter marker (`args ...T`) or unpack/spread where the grammar allows.",
			"",
			"**Example**",
			"",
			forstBlock("func sum(xs ...Int): Int { }"),
		}, "\n"))
	case ast.TokenBitwiseAnd:
		return BitwiseAndAmpersandMarkdown()
	case ast.TokenBitwiseOr:
		return keywordDoc("|", "Bitwise OR on integers.")
	case ast.TokenXor:
		return keywordDoc("^", "Bitwise XOR on integers (also unary bitwise complement in Go-shaped expressions).")
	case ast.TokenLShift:
		return keywordDoc("<<", "Left shift.")
	case ast.TokenRShift:
		return keywordDoc(">>", "Right shift.")
	case ast.TokenAndNot:
		return keywordDoc("&^", "Bitwise AND-NOT (`x &^ y` clears bits of `y` in `x`).")
	case ast.TokenBitwiseAndEq:
		return keywordDoc("&=", "Compound assign: bitwise AND.")
	case ast.TokenBitwiseOrEq:
		return keywordDoc("|=", "Compound assign: bitwise OR.")
	case ast.TokenXorEq:
		return keywordDoc("^=", "Compound assign: bitwise XOR.")
	case ast.TokenLShiftEq:
		return keywordDoc("<<=", "Compound assign: left shift.")
	case ast.TokenRShiftEq:
		return keywordDoc(">>=", "Compound assign: right shift.")
	case ast.TokenAndNotEq:
		return keywordDoc("&^=", "Compound assign: bitwise AND-NOT.")
	case ast.TokenArrow:
		return keywordDoc("<-", strings.Join([]string{
			"Channel send/receive operator in Go-shaped code (`ch <- v`, `v := <-ch`).",
		}, "\n"))
	case ast.TokenError:
		return keywordDoc("error", strings.Join([]string{
			"Starts a **nominal error** type: a named error with a payload shape. Values implement the built-in `Error` interface.",
			"",
			"**Example**",
			"",
			forstBlock(
				"error NotFound {",
				"  path: String",
				"}",
			),
		}, "\n"))
	case ast.TokenLogicalAnd:
		return keywordDoc("&&", strings.Join([]string{
			"Logical AND. If the left side is false, the right side is **not** evaluated.",
			"",
			"**Example:** `ok && ok.doWork()`",
		}, "\n"))
	case ast.TokenLogicalOr:
		return keywordDoc("||", strings.Join([]string{
			"Logical OR. If the left side is true, the right side is **not** evaluated.",
			"",
			"**Example:** `x == nil || x.f()`",
		}, "\n"))
	case ast.TokenLogicalNot:
		return keywordDoc("!", strings.Join([]string{
			"Inverts a boolean. Often used with `ensure`: `ensure !err` checks that an `Error` is nil. For a `Result`, prefer `ensure result`.",
		}, "\n"))
	case ast.TokenNil:
		return keywordDoc("nil", strings.Join([]string{
			"The **zero value** for pointers, slices, maps, channels, functions, and interface-shaped values such as `Error`.",
			"",
			"**Example**",
			"",
			forstBlock(
				"var p: Pointer(String)",
				"p = nil",
			),
		}, "\n"))
	case ast.TokenTrue:
		return keywordDoc("true", strings.Join([]string{
			"Boolean literal **true**.",
		}, "\n"))
	case ast.TokenFalse:
		return keywordDoc("false", strings.Join([]string{
			"Boolean literal **false**.",
		}, "\n"))
	default:
		return ""
	}
}

// BitwiseAndAmpersandMarkdown is hover text for binary `&` (integer bitwise AND).
func BitwiseAndAmpersandMarkdown() string {
	return keywordDoc("&", "Bitwise AND on integers.")
}

// AddressOfAmpersandMarkdown is hover text for unary `&` (address-of / take pointer).
func AddressOfAmpersandMarkdown() string {
	return keywordDoc("&", strings.Join([]string{
		"Address-of: takes a pointer to a value (Go `&expr`).",
		"",
		"**Example**",
		"",
		forstBlock(
			"n := 1",
			"p := &n",
			"srv := &http.Server{}",
		),
	}, "\n"))
}

func keywordDoc(title, body string) string {
	var b strings.Builder
	b.WriteString("**`")
	b.WriteString(title)
	b.WriteString("`**\n\n")
	b.WriteString(body)
	return b.String()
}

func typeDoc(title, body string) string {
	return fmt.Sprintf("**`%s`** (built-in type)\n\n%s", title, body)
}

// builtinTypeDocs holds LSP hover text for built-in Forst types. Keys match Forst spellings.
var builtinTypeDocs = map[string]string{
	NameInt: typeDoc(NameInt, strings.Join([]string{
		"Whole numbers. Generated Go code uses `int`.",
		"",
		"**Example**",
		"",
		forstBlock(
			"func inc(x: Int): Int {",
			"  return x + 1",
			"}",
		),
		"",
		"**Guards:** `Min`, `Max`, `LessThan`, `GreaterThan` in `ensure` / `if … is …`.",
	}, "\n")),
	NameFloat: typeDoc(NameFloat, strings.Join([]string{
		"Floating-point numbers. Lowers to Go `float64`.",
		"",
		"**Example**",
		"",
		forstBlock(
			"func half(x: Float): Float {",
			"  return x / 2",
			"}",
		),
	}, "\n")),
	NameString: typeDoc(NameString, strings.Join([]string{
		"UTF-8 text. Lowers to Go `string`.",
		"",
		"**Example**",
		"",
		forstBlock(`s: String = "hello"`),
		"",
		"**Common guards:** `Min` / `Max` (length), `HasPrefix`, `Contains`, `NotEmpty`.",
	}, "\n")),
	NameBool: typeDoc(NameBool, strings.Join([]string{
		"Boolean flag. Lowers to Go `bool`.",
		"",
		"**Example**",
		"",
		forstBlock("ok: Bool = true"),
		"",
		"**Guards:** `True`, `False`.",
	}, "\n")),
	NameVoid: typeDoc(NameVoid, strings.Join([]string{
		"Means “no return value” (like Go functions with no results).",
		"",
		"**Example**",
		"",
		forstBlock(
			"func logMsg(s: String): Void {",
			"  // side effects only",
			"}",
		),
	}, "\n")),
	NameArray: typeDoc(NameArray, strings.Join([]string{
		"Ordered sequence `Array(T)`—lowers to a Go slice `[]T`.",
		"",
		"**Example**",
		"",
		forstBlock("xs: Array(Int) = [1, 2, 3]"),
		"",
		"**Guards:** length checks behave like `String` (`Min`, `Max`, `NotEmpty`).",
	}, "\n")),
	NameMap: typeDoc(NameMap, strings.Join([]string{
		"Key/value map `Map(Key, Value)` → Go `map[Key]Value`.",
		"",
		"**Example**",
		"",
		forstBlock("scores: Map(String, Int)"),
	}, "\n")),
	NameStruct: typeDoc(NameStruct, strings.Join([]string{
		"Anonymous struct in Go interop. For app types, prefer a named shape: `type Row = { … }`.",
	}, "\n")),
	NameError: typeDoc(NameError, strings.Join([]string{
		"The built-in error interface (Go `error`). Call `.Error()` for the message when hovering works through the Go bridge.",
		"",
		"**Example**",
		"",
		forstBlock(
			"func mayFail(): Error {",
			"  return nil",
			"}",
		),
	}, "\n")),
	NamePointer: typeDoc(NamePointer, strings.Join([]string{
		"Optional reference `Pointer(T)` → Go `*T`.",
		"",
		"**Example**",
		"",
		forstBlock("p: Pointer(String)"),
		"",
		"**Guards:** `Nil` (must be nil) or `Present` (must be non-nil).",
	}, "\n")),
	NameShape: typeDoc(NameShape, strings.Join([]string{
		"Record / object type with named fields. Often written as `type Name = { field: Type }`.",
		"",
		"**Example**",
		"",
		forstBlock(
			"type Point = {",
			"  x: Int",
			"  y: Int",
			"}",
		),
	}, "\n")),
	NameResult: typeDoc(NameResult, strings.Join([]string{
		"Success-or-failure value `Result(Ok, Err)` with failure error-kinded. Narrow with `Ok` / `Err` in `if` or `ensure`.",
		"",
		"**Example**",
		"",
		forstBlock(
			"func parse(): Result(Int, Error) { }",
			"",
			"if r is Ok() {",
			"  // use success value",
			"}",
		),
	}, "\n")),
	NameTuple: typeDoc(NameTuple, strings.Join([]string{
		"Fixed product of types, mainly for FFI and multi-value interop.",
		"",
		"**Example**",
		"",
		forstBlock("Tuple(Int, String)"),
	}, "\n")),
	NameObject: typeDoc(NameObject, strings.Join([]string{
		"Loose object interop slot. Prefer `Shape` or `Map` when you want precise static typing.",
	}, "\n")),
	NameBytes: typeDoc(NameBytes, strings.Join([]string{
		"Byte slice type. Lowers to Go `[]byte`.",
		"",
		"**Example**",
		"",
		forstBlock("buf: Bytes"),
	}, "\n")),
	NameComplex64: typeDoc(NameComplex64, strings.Join([]string{
		"Complex number with `float32` components. Lowers to Go `complex64`.",
	}, "\n")),
	NameComplex128: typeDoc(NameComplex128, strings.Join([]string{
		"Complex number with `float64` components. Lowers to Go `complex128`.",
	}, "\n")),
	NameRune: typeDoc(NameRune, strings.Join([]string{
		"Unicode code point. Often written with a rune literal (`'a'`). Lowers to Go `rune` (`int32`).",
	}, "\n")),
}
