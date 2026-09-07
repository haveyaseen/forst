package main

import (
	fmt "fmt"
	os "os"
)

type T_S47SAU5d2zT struct {
	id string
}

func main() {
	result, resultErr := forst_bridge_callsync_legacy_payment_js_create()
	if resultErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", resultErr)
			os.Exit(1)
		}
	}
	println(result.id)
}
