package main

import "fmt"

func Boom() (string, error) {
	return "", fmt.Errorf("x")
}

func main() {
	s, err := Boom()
	if err != nil {
		println(err.Error())
		return
	}
	println(s)
}
