package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"strings"
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

	// Send an initial event immediately on connection
	initialTime := time.Now()
	fmt.Fprintf(w, "event: time\n")
	fmt.Fprintf(w, "data: %v\n", initialTime.Format(time.RFC1123))
	fmt.Fprintf(w, "id: %v\n\n", initialTime.UnixNano()/int64(time.Millisecond))
	flusher.Flush()

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

func htmlPageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `
<!DOCTYPE html>
<html>
<head>
    <title>SSE Time Client</title>
    <style>
        body { font-family: sans-serif; }
        #messages { list-style-type: none; margin: 0; padding: 0; border: 1px solid #ccc; height: 200px; overflow-y: scroll; }
        #messages li { padding: 5px 10px; }
        #messages li:nth-child(odd) { background: #eee; }
    </style>
</head>
<body>
    <h1>Server-Sent Events Time Client</h1>
    <p>Connection Status: <span id="status">Connecting...</span></p>
    <h2>Messages</h2>
    <ul id="messages"></ul>
    <script>
        document.addEventListener("DOMContentLoaded", function() {
            const messages = document.getElementById("messages");
            const status = document.getElementById("status");

            // In FCGI mode, the page is at /sse.fcgi, so we construct the events path accordingly.
            // In standalone mode, the page is at /, so the path is just /events.
            const eventsPath = window.location.pathname.startsWith("/sse.fcgi") ? "/sse.fcgi/events" : "/events";
            const eventSource = new EventSource(eventsPath);

            eventSource.onopen = function(event) {
                status.textContent = "Connected";
                addMessage("System", "Connected to SSE server.");
            };

            eventSource.onerror = function(error) {
                status.textContent = "Disconnected";
                addMessage("System", "Connection error. The server may have disconnected.");
                eventSource.close();
            };

            eventSource.addEventListener("time", function(event) {
                addMessage("Server (time event)", event.data);
            });
            
            eventSource.onmessage = function(event) {
                addMessage("Server (generic message)", event.data);
            };

            function addMessage(source, text) {
                const li = document.createElement("li");
                li.textContent = "[" + source + "]: " + text;
                messages.appendChild(li);
                messages.scrollTop = messages.scrollHeight;
            }
        });
    </script>
</body>
</html>
`)
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var internalPath string
		const prefix = "/sse.fcgi"

		if strings.HasPrefix(r.URL.Path, prefix) {
			internalPath = strings.TrimPrefix(r.URL.Path, prefix)
		} else {
			internalPath = r.URL.Path
		}

		if internalPath == "" || internalPath == "/" {
			htmlPageHandler(w, r)
			return
		}

		if internalPath == "/events" {
			sseHandler(w, r)
			return
		}

		http.NotFound(w, r)
	})

	listenAddr := flag.String("listenAddr", "", "address for the standalone server to listen on")
	flag.Parse()

	if *listenAddr != "" {
		log.Printf("Running as a standalone server on %s", *listenAddr)
		log.Fatal(http.ListenAndServe(*listenAddr, nil))
	} else if len(os.Args) == 2 {
		socketPath := os.Args[1]
		l, err := net.Listen("unix", socketPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "net.Listen failed: %v\n", err)
			os.Exit(1)
		}
		log.Print("Running as a FastCGI socket server")
		err = fcgi.Serve(l, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fcgi.Serve failed: %v\n", err)
			os.Exit(1)
		}
	} else {
		log.Print("Running as a FastCGI stdin server")
		if e := fcgi.Serve(nil, nil); e != nil {
			log.Fatal(e)
		}
	}
}
