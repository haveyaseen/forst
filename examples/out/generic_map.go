package main

import strconv "strconv"

func main() {
	m := map[string]int{"a": 1}
	println(strconv.Itoa(mapLen(m)))
}

func mapLen[K comparable, V any](m map[K]V) int {
	return len(m)
}
