package main

import strconv "strconv"

func main() {
	n := pick(1, "x")
	println(strconv.Itoa(n))
}

func pick[T any, U any](a T, b U) T {
	return a
}
