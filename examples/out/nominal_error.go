package main

import (
	"fmt"
	os "os"
)

// NotPositive: TypeDefErrorExpr({message: String})
type NotPositive struct {
	message string
}

func (e NotPositive) Error() string {
	return "NotPositive"
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
	result := Test()
	if result != nil {
		fmt.Println(result)
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", result)
			os.Exit(1)
		}
	}
}
