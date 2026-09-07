package main

import (
	"fmt"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	srv := &http.Server{}
	srv.Handler = mux
	fmt.Println(srv.Handler != nil)
}
