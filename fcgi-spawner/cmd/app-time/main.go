package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"time"
)

func timeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	currentTime := time.Now().Format(time.RFC1123)
	fmt.Fprintln(w, "<h1>Current Server Time</h1>")
	fmt.Fprintf(w, "<p>%s</p>", currentTime)
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

	err = fcgi.Serve(l, http.HandlerFunc(timeHandler))
	if err != nil {
		fmt.Fprintf(os.Stderr, "fcgi.Serve failed: %v\n", err)
		os.Exit(1)
	}
}