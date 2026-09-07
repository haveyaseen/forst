package main

import (
	"fmt"
	os "os"
)

// NotPositive: TypeDefErrorExpr({message: String})
type NotPositive struct {
	message string
}

// T_H4c2uQ34ZJV: TypeDefShapeExpr({})
type T_H4c2uQ34ZJV struct {
}

func (e NotPositive) Error() string {
	return "error"
}

func (e NotPositive) ForstErrorTag() string {
	return "main/NotPositive"
}

func Test() error {
	n := 0
	if n <= 0 {
		return NotPositive{message: "n must be greater than 0"}
	}
	return nil
}

func main() {
	err := Test()
	if err != nil {
		fmt.Println(err)
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", err)
			os.Exit(1)
		}
	}
}
