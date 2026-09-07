package main

import "fmt"

func main() {
	go printHi()
	fmt.Println("done")
}

func printHi() (int, error) {
	return fmt.Println("hi")
}
