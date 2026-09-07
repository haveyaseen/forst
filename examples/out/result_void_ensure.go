package main

import (
	"fmt"
	os "os"
)

// NeedFailed: TypeDefErrorExpr({reason: String})
type NeedFailed struct {
	reason string
}

func (e NeedFailed) Error() string {
	return "error"
}

func (e NeedFailed) ForstErrorTag() string {
	return "main/NeedFailed"
}

func main() {
	r, rErr := run(true)
	if rErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", rErr)
			os.Exit(1)
		}
	}
	fmt.Println(r)
}

func need(ok bool) error {
	if !ok {
		return NeedFailed{reason: "need failed"}
	}
	return nil
}

func run(ok bool) (int, error) {
	if err := need(ok); err != nil {
		return 0, err
	}
	return 1, nil
}
