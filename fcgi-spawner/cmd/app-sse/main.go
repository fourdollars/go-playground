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
	if len(os.Args) < 2 {
		log.Fatal("Expected socket path as an argument")
	}
	socketPath := os.Args[1]

	// Remove old socket file if it exists
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		log.Fatalf("Could not remove old socket: %v", err)
	}

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("Failed to listen on socket: %v", err)
	}
	defer ln.Close()

	http.HandleFunc("/", sseHandler)

	log.Println("SSE FCGI server starting on socket:", socketPath)
	if err := fcgi.Serve(ln, nil); err != nil {
		log.Fatalf("fcgi.Serve failed: %v", err)
	}
}
