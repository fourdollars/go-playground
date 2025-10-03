package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
)

func helloHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintln(w, "<h1>Hello from Go FastCGI!</h1>")
	fmt.Fprintln(w, "<p>This is the 'hello' application.</p>")
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <socket-path>\n", os.Args[0])
		os.Exit(1)
	}
	socketPath := os.Args[1]

	l, err := net.Listen("unix", socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "net.Listen failed: %v\n", err)
		os.Exit(1)
	}

	err = fcgi.Serve(l, http.HandlerFunc(helloHandler))
	if err != nil {
		fmt.Fprintf(os.Stderr, "fcgi.Serve failed: %v\n", err)
		os.Exit(1)
	}
}
