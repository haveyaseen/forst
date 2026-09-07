package main

import errors "errors"
// T_H4c2uQ34ZJV: TypeDefShapeExpr({})
type T_H4c2uQ34ZJV struct {
}

func main() {
	x, xErr := one()
	if xErr == nil {
		println(x)
	}
}

func one() (int, error) {
	n := 1
	if n <= 0 {
		return 0, errors.New("ensure n is Int.GreaterThan(0): want > 0")
	}
	return n, nil
}
