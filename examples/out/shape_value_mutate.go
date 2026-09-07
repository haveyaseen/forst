package main

// Acc: TypeDefShapeExpr({n: Int})
type Acc struct {
	n int
}

func bump(a Acc) Acc {
	a.n = a.n + 1
	return a
}

func main() {
	a := Acc{n: 0}
	a = bump(a)
	println(a.n)
}
