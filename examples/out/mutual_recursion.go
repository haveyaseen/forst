package main
// P: TypeDefShapeExpr({n: Int})
type P struct {
	n int
}

func (p *P) a() string {
	if p.n == 0 {
		return "x"
	}
	return p.b()
}

func (p *P) b() string {
	return p.a()
}

func main() {
	p := &P{n: 0}
	println(p.a())
}
