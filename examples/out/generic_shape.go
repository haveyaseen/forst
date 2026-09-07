package main

import strconv "strconv"
// T_Ngzq1ErerT: TypeDefShapeExpr({value: Value(42)})
type T_Ngzq1ErerT struct {
	value int
}

func getValue[T any](b struct {
	value T
}) T {
	return b.value
}

func main() {
	println(strconv.Itoa(getValue(struct {
		value int
	}{value: 42})))
}
