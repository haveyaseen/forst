package main

import (
	fmt "fmt"
	os "os"
	"strconv"
)

func main() {
	first, firstErr := forst_bridge_callsync_legacy_counter_js_inc()
	if firstErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", firstErr)
			os.Exit(1)
		}
	}
	println(strconv.FormatFloat(first, 'f', 0, 64))
	second, secondErr := forst_bridge_callsync_legacy_counter_js_inc()
	if secondErr != nil {
		{
			fmt.Fprintf(os.Stderr, "ensure failed: %v\n", secondErr)
			os.Exit(1)
		}
	}
	println(strconv.FormatFloat(second, 'f', 0, 64))
	ForstBridgeWaitForShutdown()
}
