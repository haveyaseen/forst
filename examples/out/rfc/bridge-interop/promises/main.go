package main

import (
	fmt "fmt"
	os "os"
)

type T_BSiWS9EsB18 struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
	Id       string  `json:"id"`
}

type T_NTbLJjyksQg struct {
	Echo float64 `json:"echo"`
}

func main() {
	result, resultErr := forst_bridge_callasync_legacy_payment_js_create()
	if resultErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", resultErr)
			os.Exit(1)
		}
	}
	println(result.Id)
	echo, echoErr := forst_bridge_callasync_legacy_payment_js_concurrentEcho()
	if echoErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", echoErr)
			os.Exit(1)
		}
	}
	println(echo.Echo)
}
