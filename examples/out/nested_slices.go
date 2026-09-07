package main

func main() {
	rows := nestedEdges()
	println(len(rows))
	println(len(rows[0]))
	println(rows[0][0])
}

func nestedEdges() [][]string {
	return [][]string{[]string{"a", "b"}, []string{"c", "d"}}
}
