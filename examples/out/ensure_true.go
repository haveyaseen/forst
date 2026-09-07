package main

import (
	"fmt"
	os "os"
)

// FlagFail: TypeDefErrorExpr({reason: String})
type FlagFail struct {
	reason string
}

// T_CQ83zP8NNan: TypeDefShapeExpr({})
type T_CQ83zP8NNan struct {
}

func (e FlagFail) Error() string {
	return "error"
}

func (e FlagFail) ForstErrorTag() string {
	return "main/FlagFail"
}

func checkFlag(ok bool) (string, error) {
	if !ok {
		return "", FlagFail{reason: "flag was false"}
	}
	return "ok", nil
}

func main() {
	r, rErr := checkFlag(true)
	if rErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", rErr)
			os.Exit(1)
		}
	}
	fmt.Println(r)
}
