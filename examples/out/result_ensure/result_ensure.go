package main

import (
	errors "errors"
	fmt "fmt"
	os "os"
)

// T_H4c2uQ34ZJV: TypeDefShapeExpr({})
type T_H4c2uQ34ZJV struct {
}

func main() {
	x, xErr := okInt()
	if xErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", xErr)
			os.Exit(1)
		}
	}
	println(x)
}

func okInt() (int, error) {
	n := 42
	if n <= 0 {
		return 0, errors.New("ensure n is Int.GreaterThan(0): want > 0")
	}
	return n, nil
}
