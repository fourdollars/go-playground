package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"strings"
)

func main() {
	r := http.NewServeMux()
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		for _, item := range os.Environ() {
			k, v := strings.Split(item, "=")[0], strings.Split(item, "=")[1]
			w.Write([]byte(fmt.Sprintf("%s=%s\n", k, v)))
		}
		w.Write([]byte("\n"))
		for k, v := range r.Header {
			w.Write([]byte(fmt.Sprintf("%s=%s\n", k, v)))
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
