package main

import strconv "strconv"

func deref[T any](p *T) T {
	return *p
}

func main() {
	n := 7
	x := deref(&n)
	println(strconv.Itoa(x))
}
