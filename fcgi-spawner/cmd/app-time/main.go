package main

import (
	"fmt"
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
	// With a nil listener, fcgi.Serve accepts requests from stdin.
	err := fcgi.Serve(nil, http.HandlerFunc(timeHandler))
	if err != nil {
		fmt.Fprintf(os.Stderr, "fcgi.Serve failed: %v\n", err)
		os.Exit(1)
	}
}

