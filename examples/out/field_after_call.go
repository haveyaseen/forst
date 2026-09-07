package main

import strconv "strconv"

// Q: TypeDefShapeExpr({n: Int})
type Q struct {
	n int
}

func main() {
	println(strconv.Itoa(makeQ().n))
}

func makeQ() Q {
	return Q{n: 7}
}
