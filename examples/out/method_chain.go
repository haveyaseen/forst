package main

import strconv "strconv"
// Q: TypeDefShapeExpr({n: Int})
type Q struct {
	n int
}

func (q Q) Inc() Q {
	return Q{n: 1}
}

func (q Q) Value() int {
	return q.n
}

func main() {
	q := Q{n: 0}
	println(strconv.Itoa(q.Inc().Value()))
}
