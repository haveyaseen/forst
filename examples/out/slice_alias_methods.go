package main

// ExprList: TypeDefAssertionExpr(Array(String))
type ExprList []string

// P: TypeDefShapeExpr({n: Int})
type P struct {
	n int
}

func (p *P) a() ExprList {
	if p.n == 0 {
		return []string{}
	}
	return p.b()
}

func (p *P) b() ExprList {
	xs := p.a()
	return append(xs, "y")
}

func main() {
	p := &P{n: 0}
	println(len(p.a()))
}
