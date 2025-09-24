package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"sort"
)

func main() {
	r := http.NewServeMux()
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "--- Request Details ---\n")
		fmt.Fprintf(w, "Method: %s\n", r.Method)
		fmt.Fprintf(w, "URL Path: %s\n", r.URL.Path)
		fmt.Fprintf(w, "Query String: %s\n", r.URL.RawQuery)
		fmt.Fprintf(w, "Remote Address: %s\n", r.RemoteAddr)
		fmt.Fprintf(w, "Protocol: %s\n", r.Proto)
		fmt.Fprintf(w, "\n--- HTTP Headers (from Request) ---\n")
		// Sort headers
		var headerKeys []string
		for name := range r.Header {
			headerKeys = append(headerKeys, name)
		}
		sort.Strings(headerKeys)
		for _, name := range headerKeys {
			for _, h := range r.Header[name] {
				fmt.Fprintf(w, "%s: %s\n", name, h)
			}
		}
		fmt.Fprintf(w, "\n--- Process Environment Variables (os.Environ()) ---\n")
		// Sort environment variables
		envVars := os.Environ()
		sort.Strings(envVars)
		for _, env := range envVars {
			fmt.Fprintf(w, "%s\n", env)
		}
	})
	if os.Getenv("_") == "./app-env.fcgi" {
		log.Print("Running as a standalone server")
		http.ListenAndServe(":8080", r)
	} else if len(os.Args) == 2 {
		socketPath := os.Args[1]
		l, err := net.Listen("unix", socketPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "net.Listen failed: %v\n", err)
			os.Exit(1)
		}
		log.Print("Running as a FastCGI socket server")
		err = fcgi.Serve(l, r)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fcgi.Serve failed: %v\n", err)
			os.Exit(1)
		}
	} else {
		log.Print("Running as a FastCGI stdin server")
		if e := fcgi.Serve(nil, r); e != nil {
			log.Fatal(e)
		}
	}
}
