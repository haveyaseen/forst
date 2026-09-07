package main
// Inner: TypeDefShapeExpr({Value: Int})
type Inner struct {
	Value int
}

// Outer: TypeDefShapeExpr({Inner: Inner})
type Outer struct {
	Inner
}

func main() {
	o := Outer{Inner: Inner{Value: 10}}
	println(o.Value)
}
