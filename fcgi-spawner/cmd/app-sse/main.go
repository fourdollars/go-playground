package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"time"
)

func sseHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	log.Println("Client connected for SSE.")

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case t := <-ticker.C:
			// Send the current time to the client
			fmt.Fprintf(w, "event: time\n")
			fmt.Fprintf(w, "data: %v\n", t.Format(time.RFC1123))
			fmt.Fprintf(w, "id: %v\n\n", t.UnixNano()/int64(time.Millisecond))
			flusher.Flush()
		case <-r.Context().Done():
			// Client has disconnected
			log.Println("Client disconnected.")
			return
		}
	}
}

func main() {
	r := http.NewServeMux()
	r.HandleFunc("/", sseHandler)
	if len(os.Args) == 2 {
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
