package main

const (
	A = iota
	B
	C
)

const (
	FlagNone = 1 << iota
	FlagRead
	FlagWrite
)

const Greeting = "hello"
const Pi = 3

func main() {
	println(Pi)
	println(len(Greeting))
	println(A)
	println(B)
	println(C)
	println(FlagNone)
	println(FlagRead)
	println(FlagWrite)
}
