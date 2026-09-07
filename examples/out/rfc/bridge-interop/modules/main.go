package main

import (
	fmt "fmt"
	os "os"
	"strconv"
)

type T_Zn4FXrBCht3 struct {
	Id         string  `json:"id"`
	Total      string  `json:"total"`
	TotalCents float64 `json:"totalCents"`
}

func main() {
	order, orderErr := forst_bridge_callsync_legacy_api_checkout_js_createOrder()
	if orderErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", orderErr)
			os.Exit(1)
		}
	}
	println(order.Id)
	println(order.Total)
	println(strconv.FormatFloat(order.TotalCents, 'f', 0, 64))
}
