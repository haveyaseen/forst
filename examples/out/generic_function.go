package main

import strconv "strconv"

func identity[T any](x T) T {
	return x
}

func main() {
	n := identity(42)
	explicit := identity[int](42)
	println(strconv.Itoa(n))
	println(strconv.Itoa(explicit))
}
