package main

func main() {
	n := 10
	inc := func(x int) int {
		return x + 1
	}
	println(inc(n))
	addN := func(x int) int {
		return x + n
	}
	println(addN(5))
	println(twice(inc, 3))
	func() {
		println("iife")
	}()
	go func() {
		println("go")
	}()
	defer func() {
		println("defer")
	}()
}

func twice(fn func(int) int, x int) int {
	return fn(fn(x))
}
