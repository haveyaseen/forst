package main

import (
	"fmt"
	os "os"
)

// InnerFail: TypeDefErrorExpr({reason: String})
type InnerFail struct {
	reason string
}

// T_CQ83zP8NNan: TypeDefShapeExpr({})
type T_CQ83zP8NNan struct {
}

func (e InnerFail) Error() string {
	return "error"
}

func (e InnerFail) ForstErrorTag() string {
	return "main/InnerFail"
}

func inner(ok bool) (string, error) {
	if !ok {
		return "", InnerFail{reason: "inner failed"}
	}
	return "x", nil
}

func main() {
	r, rErr := outer(true)
	if rErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", rErr)
			os.Exit(1)
		}
	}
	fmt.Println(r)
}

func outer(ok bool) (int, error) {
	_, nameErr := inner(ok)
	if nameErr != nil {
		return 0, nameErr
	}
	return 1, nil
}
