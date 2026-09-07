package main

import (
	errors "errors"
	fmt "fmt"
	os "os"
	utf8 "unicode/utf8"
)

// Password: TypeDefAssertionExpr(String)
type Password string

func G_Td6yR1SKQP9(password Password) bool {
	if utf8.RuneCountInString(string(password)) < 12 {
		return false
	}
	return true
}

func main() {
	var password Password = "12345abc"
	if !G_Td6yR1SKQP9(password) {
		println("Detected password as too weak, exiting...")
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", errors.New("ensure password is Password.Strong(): want Password.Strong()"))
			os.Exit(1)
		}
	}
	println("We have a strong password, continuing...")
}
