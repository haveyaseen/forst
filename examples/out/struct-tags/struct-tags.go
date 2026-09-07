package main

// Config: TypeDefShapeExpr({host: String, port: Int, plain: Int})
type Config struct {
	host  string
	plain int
	port  int
}

func main() {
	c := Config{host: "localhost", port: 8080, plain: 1}
	println(c.host, c.port, c.plain)
}
