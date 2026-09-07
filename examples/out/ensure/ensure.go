package main

import (
	"fmt"
	os "os"
	utf8 "unicode/utf8"
)

// T_EzZKiw9FNu2: TypeDefShapeExpr({})
type T_EzZKiw9FNu2 struct {
}

// T_Saiim6wHvrR: TypeDefShapeExpr({})
type T_Saiim6wHvrR struct {
}

func checkConditions() (int, error) {
	err := mustBeARealName("John")
	if err != nil {
		return 0, err
	}
	speed := 80
	err = mustNotExceedSpeedLimit(speed)
	if err != nil {
		return 0, err
	}
	return 20, nil
}

func main() {
	result, resultErr := checkConditions()
	if resultErr != nil {
		fmt.Printf("Conditions not met: %s", resultErr.Error())
		fmt.Println()
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", resultErr)
			os.Exit(1)
		}
	}
	fmt.Printf("Conditions met (value %d), program exiting successfully", result)
	fmt.Println()
}

func mustBeARealName(name string) error {
	if utf8.RuneCountInString(name) < 1 {
		return TooShort("Name must be at least 1 character long")
	}
	return nil
}

func mustNotExceedSpeedLimit(speed int) error {
	if speed >= 100 {
		return TooFast("Speed must not exceed 100 km/h")
	}
	return nil
}
