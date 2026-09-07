package main

import strconv "strconv"

func first[T any](xs []T) T {
	return xs[0]
}

func main() {
	n := first([]int{1, 2, 3})
	println(strconv.Itoa(n))
}
