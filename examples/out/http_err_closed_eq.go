package main

import "net/http"

func checkClosed(err error) {
	if err == http.ErrServerClosed {
		return
	}
}

func main() {
	checkClosed(http.ErrServerClosed)
}
