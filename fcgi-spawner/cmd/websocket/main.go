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

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all connections
		return true
	},
}

func websocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Error upgrading to websocket:", err)
		return
	}
	defer conn.Close()

	log.Println("Websocket client connected")

	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			log.Println("Websocket read error:", err)
			return
		}

		log.Printf("Received message: %s", p)

		if err := conn.WriteMessage(messageType, p); err != nil {
			log.Println("Websocket write error:", err)
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
    <title>WebSocket Echo Client</title>
    <style>
        body { font-family: sans-serif; }
        #messages { list-style-type: none; margin: 0; padding: 0; border: 1px solid #ccc; height: 200px; overflow-y: scroll; }
        #messages li { padding: 5px 10px; }
        #messages li:nth-child(odd) { background: #eee; }
    </style>
</head>
<body>
    <h1>WebSocket Echo Client</h1>
    <div>
        <input type="text" id="messageBox" placeholder="Type message here..." />
        <button id="sendButton">Send</button>
    </div>
    <p>Connection Status: <span id="status">Connecting...</span></p>
    <h2>Messages</h2>
    <ul id="messages"></ul>
    <script>
        document.addEventListener("DOMContentLoaded", function() {
            const messageBox = document.getElementById("messageBox");
            const sendButton = document.getElementById("sendButton");
            const messages = document.getElementById("messages");
            const status = document.getElementById("status");

            const wsProtocol = window.location.protocol === "https:" ? "wss:" : "ws:";
            const wsHost = window.location.host;

            // In FCGI mode, the page is at /websocket.fcgi, so we construct the ws path accordingly.
            // In standalone mode, the page is at /, so the path is just /ws.
            const wsPath = window.location.pathname.startsWith("/websocket.fcgi") ? "/websocket.fcgi/ws" : "/ws";
            const socket = new WebSocket(wsProtocol + "//" + wsHost + wsPath);

            socket.onopen = function(event) {
                status.textContent = "Connected";
                addMessage("System", "Connected to WebSocket server.");
            };

            socket.onclose = function(event) {
                status.textContent = "Disconnected";
                if (event.wasClean) {
                    addMessage("System", "Connection closed cleanly, code=" + event.code + " reason=" + event.reason);
                } else {
                    addMessage("System", "Connection died");
                }
            };

            socket.onerror = function(error) {
                status.textContent = "Error";
                addMessage("System", "WebSocket Error: " + error.message);
            };

            socket.onmessage = function(event) {
                addMessage("Server", event.data);
            };

            sendButton.onclick = function() {
                const message = messageBox.value;
                if (message) {
                    socket.send(message);
                    addMessage("Client", message);
                    messageBox.value = "";
                }
            };
            
            messageBox.addEventListener("keypress", function(event) {
                if (event.key === "Enter") {
                    sendButton.click();
                }
            });

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
		const prefix = "/websocket.fcgi"

		if strings.HasPrefix(r.URL.Path, prefix) {
			internalPath = strings.TrimPrefix(r.URL.Path, prefix)
		} else {
			internalPath = r.URL.Path
		}

		if internalPath == "" || internalPath == "/" {
			htmlPageHandler(w, r)
			return
		}

		if internalPath == "/ws" {
			websocketHandler(w, r)
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
