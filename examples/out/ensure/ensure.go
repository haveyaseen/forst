package main

import (
	"fmt"
	os "os"
	utf8 "unicode/utf8"
)

// TooFast: TypeDefErrorExpr({message: String})
type TooFast struct {
	message string
}

// TooShort: TypeDefErrorExpr({message: String})
type TooShort struct {
	message string
}

func (e TooFast) Error() string {
	return "TooFast"
}

func (e TooShort) Error() string {
	return "TooShort"
}

func (e TooFast) ForstErrorTag() string {
	return "main/TooFast"
}

func (e TooShort) ForstErrorTag() string {
	return "main/TooShort"
}

func checkConditions() (int, error) {
	if err := mustBeARealName("John"); err != nil {
		return 0, err
	}
	speed := 80
	if err := mustNotExceedSpeedLimit(speed); err != nil {
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
		return TooShort{message: "Name must be at least 1 character long"}
	}
	return nil
}

func mustNotExceedSpeedLimit(speed int) error {
	if speed >= 100 {
		return TooFast{message: "Speed must not exceed 100 km/h"}
	}
	return nil
}
