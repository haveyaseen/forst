package typechecker

import (
	"testing"

	"forst/internal/testutil"
)

// TestCheckTypes_manyMinimalPrograms exercises common CheckTypes paths (parse → collect → infer).
func TestCheckTypes_manyMinimalPrograms(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		src  string
	}{
		{
			"if_comparison",
			`package main
func main() {
	if 1 < 2 {
		println("a")
	}
}`,
		},
		{
			"else_if_chain",
			`package main
func main() {
	n := 2
	if n > 10 {
		println("a")
	} else if n < 0 {
		println("b")
	} else {
		println("c")
	}
}`,
		},
		{
			"for_while_style",
			`package main
func main() {
	n := 0
	for n < 2 {
		n = n + 1
	}
	println("x")
}`,
		},
		{
			"slice_literal_and_index",
			`package main
func main() {
	xs := [1, 2]
	println(string(xs[0]))
}`,
		},
		{
			"type_alias_shape",
			`package main
type P = { x: Int }
func main() {
	v := { x: 1 }
	println(string(v.x))
}`,
		},
		{
			"result_ok_branch",
			`package main
func f(): Result(Int, Error) {
	return 1
}
func main() {
	x := f()
	if x is Ok() {
		println(x)
	}
}`,
		},
		{
			"type_guard_call",
			`package main
type Password = String
is (p Password) Long() {
	ensure p is Min(3)
}
func main() {
	s: Password = "abc"
	ensure s is Long() else {
		println("no")
	}
	println("done")
}`,
		},
		{
			"string_concat",
			`package main
func main() {
	println("a" + "b")
}`,
		},
		{
			"negation",
			`package main
func main() {
	if !false {
		println("t")
	}
}`,
		},
		{
			"comparison_eq",
			`package main
func main() {
	if 1 == 1 {
		println("eq")
	}
}`,
		},
		{
			"short_decl_and_reassign",
			`package main
func main() {
	x := 1
	x = 2
	println(string(x))
}`,
		},
		{
			"nested_block_scope",
			`package main
func main() {
	if true {
		y := 1
		println(string(y))
	}
	println("out")
}`,
		},
		{
			"logical_or",
			`package main
func main() {
	if false || true {
		println("y")
	}
}`,
		},
		{
			"less_than_compare",
			`package main
func main() {
	a := 1
	b := 2
	if a < b {
		println("lt")
	}
}`,
		},
		{
			"defer_style_not",
			`package main
func work() {}
func main() {
	defer work()
	println("z")
}`,
		},
		{
			"go_stmt",
			`package main
func work() {}
func main() {
	go work()
	println("g")
}`,
		},
		{
			"multiple_returns_void",
			`package main
func early() {
	return
}
func main() {
	early()
	println("m")
}`,
		},
		{
			"string_len_builtin",
			`package main
func main() {
	println(len("hi"))
}`,
		},
		{
			"int_comparison_chain",
			`package main
func main() {
	a := 1
	b := 2
	if a < b && b > 0 {
		println("ok")
	}
}`,
		},
		{
			"shape_field_access",
			`package main
type T = { n: Int }
func main() {
	v := { n: 3 }
	println(string(v.n))
}`,
		},
		{
			"func_two_params",
			`package main
func add(a Int, b Int) {
	println(string(a + b))
}
func main() {
	add(1, 2)
}`,
		},
		{
			"mul_expr",
			`package main
func main() {
	println(string(2 * 3))
}`,
		},
		{
			"nested_if_else",
			`package main
func main() {
	if true {
		if false {
			println("a")
		} else {
			println("b")
		}
	}
}`,
		},
		{
			"void_func_call",
			`package main
func nop() {}
func main() {
	nop()
	println("ok")
}`,
		},
		{
			"sub_int",
			`package main
func main() {
	println(string(5 - 2))
}`,
		},
		{
			"paren_expr",
			`package main
func main() {
	println(string((1 + 2) * 3))
}`,
		},
		{
			"string_eq",
			`package main
func main() {
	if "a" == "a" {
		println("same")
	}
}`,
		},
		{
			"int_neq",
			`package main
func main() {
	if 1 != 2 {
		println("neq")
	}
}`,
		},
		{
			"bool_and_or",
			`package main
func main() {
	if true && false {
		println("no")
	}
	if false || true {
		println("yes")
	}
}`,
		},
		{
			"assign_after_decl",
			`package main
func main() {
	x := 1
	x = x + 1
	println(string(x))
}`,
		},
		{
			"go_append_multi",
			`package main
func main() {
	xs := [1]
	ys := append(xs, 2, 3)
	println(string(len(ys)))
}`,
		},
		{
			"go_copy_slices",
			`package main
func main() {
	a := [1, 2]
	b := [0, 0]
	println(string(copy(b, a)))
}`,
		},
		{
			"go_min_max",
			`package main
func main() {
	println(string(min(3, 2, 1)))
	println(string(max(1, 2)))
}`,
		},
		{
			"go_complex_stmt",
			`package main
func main() {
	complex(1.0, 2.0)
	println("ok")
}`,
		},
		{
			"go_map_delete",
			`package main
func main() {
	m := map[String]Int{ "a": 1 }
	delete(m, "a")
	println("ok")
}`,
		},
		{
			"go_clear_slice",
			`package main
func main() {
	sl := [1, 2]
	clear(sl)
	println("ok")
}`,
		},
		{
			"go_recover",
			`package main
func main() {
	recover()
	println("ok")
}`,
		},
		{
			"go_panic",
			`package main
func main() {
	panic("x")
}`,
		},
		{
			"if_with_short_decl_init",
			`package main
func main() {
	if x := 1; x > 0 {
		println("y")
	}
}`,
		},
		{
			"for_range_two_vars",
			`package main
func main() {
	xs := [1, 2]
	for i, v := range xs {
		println(string(i) + string(v))
	}
}`,
		},
		{
			"else_if_short_decl",
			`package main
func main() {
	n := 2
	if n > 10 {
		println("a")
	} else if n < 5 {
		z := 1
		println(string(z))
	}
}`,
		},
		{
			"nominal_error_union_typedef",
			`package main
error E1 { code: Int }
error E2 { msg: String }
type U = E1 | E2
func main() {
}`,
		},
		{
			"func_if_init_return",
			`package main
func f(): Int {
	if x := 1; x > 0 {
		return x
	}
	return 0
}
func main() {
	println(string(f()))
}`,
		},
		{
			"nested_shape",
			`package main
type Outer = { inner: { x: Int } }
func main() {
	v := { inner: { x: 3 } }
	println(string(v.inner.x))
}`,
		},
		{
			"type_alias_string",
			`package main
type Name = String
func main() {
	n: Name = "a"
	println(n)
}`,
		},
		{
			"pointer_field",
			`package main
type Box = { p: *Int }
func main() {
	n := 1
	b := { p: &n }
	println(string(*b.p))
}`,
		},
		{
			"result_ensure_ok",
			`package main
func one() {
	n := 1
	ensure n is GreaterThan(0)
	return n
}
func main() {
	x := one()
	if x is Ok() {
		println(x)
	}
}`,
		},
		{
			"typeguard_def_and_ensure",
			`package main
type S = String
is (s S) NonEmpty() {
	ensure s is Min(1)
}
func main() {
	v: S = "ok"
	ensure v is NonEmpty() else {
		println("no")
	}
	println("yes")
}`,
		},
		{
			"shape_extra_field_literal",
			`package main
type Row = { id: Int, label: String }
func main() {
	r: Row = { id: 1, label: "a" }
	println(string(r.id))
}`,
		},
		{
			"import_fmt_qualified",
			`package main
import "fmt"
func main() {
	fmt.Println("x")
}`,
		},
		{
			"alias_string_concat",
			`package main
type Slug = String
func label(s Slug): String {
	return "x-" + s
}
func main() {
	println(label("hi"))
}`,
		},
		{
			"alias_guard_concat",
			`package main
type Slug = String.Min(1).Max(64)
func label(s Slug): String {
	return "★ " + s
}
func main() {
	println(label("hi"))
}`,
		},
		{
			"place_order_input_shape",
			`package main
type PlaceOrderInput = {
	stockKeepingUnit: String.Min(1).Max(64),
	quantity:         Int.Min(1).Max(99),
}
func main() {
	x := PlaceOrderInput{ stockKeepingUnit: "ITEM-1", quantity: 2 }
	println(x.stockKeepingUnit)
}`,
		},
		{
			"compound_assign",
			`package main
func main() {
	n := 0
	n += 5
	n -= 2
	println(string(n))
}`,
		},
		{
			"nominal_error_constructor",
			`package main
error E { msg: String }
func main() {
	e := E{ msg: "x" }
	println(e.msg)
}`,
		},
		{
			"pointer_deref",
			`package main
func main() {
	n := 1
	p := &n
	println(string(*p))
}`,
		},
		{
			"star_div_mod_compound",
			`package main
func main() {
	n := 10
	n *= 2
	n /= 4
	n %= 3
	println(string(n))
}`,
		},
		{
			"defer_and_go",
			`package main
func work() {}
func main() {
	defer work()
	go work()
}
`,
		},
		{
			"receiver_method_call",
			`package main
type Counter = { n: Int }
func (Counter) inc(): Counter { return { n: 1 } }
func main() {
	c := Counter{ n: 0 }
	c = c.inc()
	println(string(c.n))
}
`,
		},
		{
			"type_alias_chain",
			`package main
type RawId = String
type UserId = RawId
func main() {
	id: UserId = "u1"
	println(id)
}
`,
		},
		{
			"for_range_index_value",
			`package main
func main() {
	xs := [1, 2]
	for i, v := range xs {
		println(string(i) + string(v))
	}
}
`,
		},
		{
			"postfix_inc_dec",
			`package main
func main() {
	i := 0
	i++
	i--
	println(string(i))
}
`,
		},
		{
			"compound_assign_for_post",
			`package main
func main() {
	for i := 0; i < 3; i += 1 {
		println(string(i))
	}
}
`,
		},
		{
			"labeled_break",
			`package main
func main() {
outer: for i := 0; i < 3; i = i + 1 {
		for j := 0; j < 3; j = j + 1 {
			if j == 1 {
				break outer
			}
		}
	}
}
`,
		},
		{
			"compound_assign_statements",
			`package main
func main() {
	n := 1
	n += 2
	n -= 1
	n *= 2
	println(string(n))
}
`,
		},
		{
			"result_err_narrowing",
			`package main
error ParseError { code: Int }
error IoError { path: String }
type ErrKind = ParseError | IoError
func onlyParse(p ParseError) {}
func mk(): Result(Int, ErrKind) { return 0 }
func main() {
	x := mk()
	if x is Err(ParseError) {
		onlyParse(x)
	}
}
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			MustTypecheck(t, tc.src, testutil.TypecheckOpts{})
		})
	}
}
