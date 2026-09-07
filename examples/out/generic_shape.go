package main

import strconv "strconv"

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
