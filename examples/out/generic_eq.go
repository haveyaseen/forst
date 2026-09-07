package main

import strconv "strconv"

func eq[T comparable](a T, b T) bool {
	return a == b
}

func main() {
	println(strconv.FormatBool(eq(1, 1)))
	println(strconv.FormatBool(eq("a", "a")))
}
