package main

// Item: TypeDefShapeExpr({name: String})
type Item struct {
	name string
}

func main() {
	xs := []Item{Item{name: "a"}}
	println(xs[0].name)
}
