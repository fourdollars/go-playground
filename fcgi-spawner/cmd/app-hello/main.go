package main

import (
	"fmt"
	"net/http"
	"net/http/fcgi"
	"os"
)

func helloHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintln(w, "<h1>Hello from Go FastCGI!</h1>")
	fmt.Fprintln(w, "<p>This is the 'app-hello' application.</p>")
}

func main() {
	// With a nil listener, fcgi.Serve accepts requests from stdin.
	// This is the "CGI-like" behavior we want for the spawned apps.
	err := fcgi.Serve(nil, http.HandlerFunc(helloHandler))
	if err != nil {
		// Logging to stderr will be captured by the spawner.
		fmt.Fprintf(os.Stderr, "fcgi.Serve failed: %v\n", err)
		os.Exit(1)
	}
}
