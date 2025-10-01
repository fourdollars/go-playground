# Go FastCGI Spawner

This project provides a "spawner" service written in Go, designed to manage and proxy requests to a dynamic pool of FastCGI applications. It enables a convenient "drop-in" deployment model: simply place a compiled FastCGI executable into a designated directory, and the spawner makes it immediately accessible.

The spawner is designed for efficiency and flexibility, supporting two types of FastCGI applications:
1.  **Socket-based (`.fcgi`):** Traditional FastCGI applications that are given a Unix socket path via the command line.
2.  **Stdio-based (`.fcgi.stdio`):** Modern applications that expect a listening socket to be provided on their standard input (i.e., socket activation).

In addition to managing applications, the spawner can also serve static files (HTML, CSS, etc.), acting as a simple, lightweight web server.

## ✨ Features

-   **Drop-in Deployment**: Add new FastCGI applications by simply uploading a compiled binary. No need to restart or reload Nginx.
-   **Dual FCGI Modes**: Supports both traditional socket-path and modern stdio-based (socket activation) FastCGI applications.
-   **Persistent Processes**: Manages a pool of running FastCGI applications, reusing processes for multiple requests for high performance. This is **not** a CGI-like model.
-   **Idle Process Management**: Automatically terminates application processes after a configurable idle period to conserve resources.
-   **Hot-Reloading**: Automatically detects changes to `.fcgi` or `.fcgi.stdio` binaries and restarts the corresponding child process.
-   **Static File Serving**: Optionally serve static files from a designated directory.
-   **Child Process Logging**: Captures and logs the `stdout` and `stderr` of each spawned FastCGI application for easy debugging.
-   **Security Conscious**: Includes path safety checks to prevent directory traversal attacks.

## 🏛️ Architecture

The spawner runs as a persistent service that listens for HTTP requests, typically proxied from a web server like Nginx. When a request arrives, the spawner identifies the target FastCGI application, starts it if it's not already running, and proxies the request to it.

```
                  +---------+      +-------------------+      +----------------------+
User Request      |         |      |                   |      |                      |
----------------->|  Nginx  |----->|  Spawner Service  |----->| Your FCGI App Process|
                  |         |      |  (spawner)        |      | (e.g., app-hello.fcgi)|
                  +---------+      +-------------------+      +----------------------+
                                            |
                                            | Serves static file
                                            v
                                     +--------------+
                                     |              |
                                     |  Static Files|
                                     | (e.g. /about)|
                                     +--------------+
```

### Spawner Logic
1.  The spawner receives an HTTP request (e.g., for `/my-app.fcgi`).
2.  It checks if a child process for `my-app.fcgi` is already running.
3.  **If running**, it proxies the request to the existing process.
4.  **If not running**, it starts the `my-app.fcgi` executable, keeps the process running, and then proxies the request.
5.  If the requested path does not match an FCGI application, the spawner attempts to serve it as a static file (if `-staticRoot` is configured).
6.  Running child processes are monitored and terminated if they remain idle for a specified duration (`-idleTimeout`).

## 📂 Project Structure

```
fcgi-spawner/
├── cmd/                # Source code for all executables
│   ├── spawner/        # The core Spawner service
│   ├── app-env/        # Example Application (supports both socket and stdio modes)
│   ├── app-hello/      # Example Application
│   ├── app-time/       # Example Application
│   └── webhook/        # Example Application
├── configs/            # Nginx and systemd/supervisor configuration templates
├── scripts/            # Automation scripts for building and deploying
├── web/                # Directory for compiled .fcgi and .fcgi.stdio files
├── go.mod
├── Dockerfile          # For containerized deployment
└── README.md
```

## 🐳 Docker Deployment

This project includes a `Dockerfile` for easy, containerized deployment. This method bundles Nginx, Supervisor, and the spawner into a single image.

#### Step 1: Build the Docker Image
```bash
docker build -t fcgi-spawner .
```

#### Step 2: Build FastCGI Applications
The build script compiles the example applications. The `app-env` application is compiled into both `.fcgi` (socket) and `.fcgi.stdio` (stdio) versions for demonstration.
```bash
chmod +x scripts/build.sh
./scripts/build.sh
```

#### Step 3: Run the Container
Run the container, mapping a host port to port 80 in the container and mounting the `web/` directory.
```bash
docker run -d -p 8080:80 -v "$(pwd)/web:/var/www/fcgi" --name fcgi-container fcgi-spawner
```

#### Step 4: Test
You can now test the endpoints for both the socket and stdio application types.
```bash
# Test the socket-based app
curl http://localhost:8080/app-env.fcgi

# Test the stdio-based app
curl http://localhost:8080/app-env.fcgi.stdio
```

## 🚀 Manual Deployment Guide (systemd)

This guide is for deploying the service directly on a Linux server with `systemd`.

### Step 1: Build Binaries
```bash
chmod +x scripts/build.sh
./scripts/build.sh
```

### Step 2: Deploy Files
The deployment script copies the configuration files, spawner program, and example applications to their final destinations. **Note**: This script uses `sudo`. Please review `scripts/deploy.sh`.
```bash
chmod +x scripts/deploy.sh
./scripts/deploy.sh
```

### Step 3: Enable the Services
1.  **Reload systemd and start the Spawner Service**
    ```bash
    sudo systemctl daemon-reload
    sudo systemctl enable --now fcgi-spawner.service
    ```

2.  **Check the Service Status**
    ```bash
    sudo systemctl status fcgi-spawner.service
    ```

3.  **Enable the Nginx Configuration and Reload**
    ```bash
    sudo ln -s /etc/nginx/sites-available/go-fcgi.conf /etc/nginx/sites-enabled/
    sudo nginx -t
    sudo systemctl reload nginx
    ```

### Step 4: Test
```bash
# Test the socket-based app
curl http://<your_server_ip>/app-env.fcgi

# Test the stdio-based app
curl http://<your_server_ip>/app-env.fcgi.stdio
```

## 💡 How to Add Your Own Application

You can write your FastCGI application in one of two ways.

### Method 1: Socket-Based (Recommended)

Your application receives a Unix socket path as a command-line argument and creates a `net.Listener` on that path.

**File Name:** `my-app.fcgi`

```go
// cmd/my-app/main.go
package main

import (
	"log"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
)

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

	// Example handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello from socket app!"))
	})

	log.Println("Socket-based FCGI server starting...")
	fcgi.Serve(ln, nil)
}
```

### Method 2: Stdio-Based (Socket Activation)

Your application does not parse command-line arguments. It calls `fcgi.Serve` with a `nil` listener, expecting the parent process (the spawner) to provide a listening socket on its standard input.

**File Name:** `my-stdio-app.fcgi.stdio`

```go
// cmd/my-stdio-app/main.go
package main

import (
	"log"
	"net/http"
	"net/http/fcgi"
)

func main() {
	// Example handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello from stdio app!"))
	})

	log.Println("Stdio-based FCGI server starting...")
	// Serve will accept connections from the listener provided on stdin
	fcgi.Serve(nil, nil)
}
```

### Deployment Steps

1.  **Build**: Run `./scripts/build.sh`. It will automatically find and compile your new application.
2.  **Deploy**: Copy the new binary from the `web/` directory to `/var/www/fcgi` (on server) or ensure your `web` directory is mounted (in Docker).
3.  **Done!** Access your app at `http://<host>/my-app.fcgi` or `http://<host>/my-stdio-app.fcgi.stdio`.

## 🛠️ Troubleshooting

The spawner captures the `stdout` and `stderr` of child processes. Check the spawner's logs for output from your application.

**With Docker:**
```bash
docker logs fcgi-container
```

**On a Linux Server (Manual):**
```bash
sudo journalctl -u fcgi-spawner.service -f
```

Common issues:
-   **502 Bad Gateway**: The child process is likely crashing. Check the logs for panic messages from your app.
-   **Connection Errors**: Ensure your application type (socket vs. stdio) matches the implementation and the file extension (`.fcgi` vs. `.fcgi.stdio`).

## 📄 License

This project is licensed under the [MIT License](LICENSE).