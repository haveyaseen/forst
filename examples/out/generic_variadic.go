package main

func ignore[T any](xs ...T) bool {
	return true
}

func main() {
	println(ignore(1, 2, 3))
}
