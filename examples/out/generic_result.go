package main

import errors "errors"

func accept[T any](r T, _ error) bool {
	return true
}

func main() {
	println(accept(one()))
}

func one() (int, error) {
	n := 1
	if n <= 0 {
		return 0, errors.New("ensure n is Int.GreaterThan(0): want > 0")
	}
	return n, nil
}
