package main

import (
	"strconv"
	"strings"
)

func demoAtoi(s string) {
	pair0, _ := strconv.Atoi(s)
	n := pair0
	println(n)
}

func demoCut(s string, sep string) {
	t0, t1, t2 := strings.Cut(s, sep)
	before := t0
	after := t1
	found := t2
	println(before)
	println(after)
	println(found)
}

func main() {
	demoAtoi("42")
	demoCut("a,b", ",")
}
