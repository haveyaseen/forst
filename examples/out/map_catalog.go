package main

import (
	errors "errors"
	"fmt"
	os "os"
	strconv "strconv"
)

var errMissingMapKey = errors.New("missing map key")

func main() {
	catalog := map[string]int{"ITEM-1": 10}
	sku := "ITEM-1"
	avail, availErr := func() (int, error) {
		v, ok := catalog[sku]
		if !ok {
			return 0, errMissingMapKey
		}
		return v, nil
	}()
	if availErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", availErr)
			os.Exit(1)
		}
	}
	fmt.Println(strconv.Itoa(avail))
	fmt.Println("ok")
}
