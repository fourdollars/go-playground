# Go FastCGI Spawner

This project provides a "spawner" service written in Go, designed to manage and proxy requests to a dynamic pool of FastCGI applications. It enables a convenient "drop-in" deployment model: simply place a compiled FastCGI executable into a designated directory, and the spawner makes it immediately accessible.

The spawner is designed for efficiency and flexibility, exclusively supporting **Socket-based (`.fcgi`)** applications. These applications are given a Unix socket path via the command line, which the spawner provides when it starts them. The spawner *requires* the `-socketDir` flag to be provided for operation.

In addition to managing applications, the spawner can also serve static files (HTML, CSS, etc.), acting as a simple, lightweight web server.

## ✨ Features

-   **Drop-in Deployment**: Add new FastCGI applications by simply uploading a compiled binary. No need to restart or reload Nginx.
-   **Socket-Based FCGI**: Supports traditional FastCGI applications that communicate over Unix sockets. The spawner *requires* the `-socketDir` flag.
-   **Persistent Processes**: Manages a pool of running FastCGI applications, reusing processes for multiple requests for high performance. This is **not** a CGI-like model.
-   **Idle Process Management**: Automatically terminates application processes after a configurable idle period (`-idleTimeout`) to conserve resources.
-   **Hot-Reloading**: Automatically detects changes (file writes) to `.fcgi` binaries in the `webRoot` and restarts the corresponding child process.
-   **Static File Serving**: Optionally serve static files from a designated directory (`-staticRoot`). Hidden files (starting with `.`) are not served.
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
2.  It checks if a child process for `my-app.fcgi` is already running and if its binary hasn't been modified.
3.  **If running and up-to-date**, it proxies the request to the existing process.
4.  **If not running, or if the binary has changed**, it starts the `my-app.fcgi` executable, passing a Unix socket path as an argument (since `-socketDir` is required).
    It then keeps the process running and proxies the request.
5.  If the requested path does not match an executable FCGI application, the spawner attempts to serve it as a static file (if `-staticRoot` is configured).
6.  Running child processes are monitored and terminated if they remain idle for a specified duration (`-idleTimeout`).
7.  Changes to `.fcgi` binaries in the `webRoot` directory trigger a restart of the corresponding child process.

## 📂 Project Structure

```
fcgi-spawner/
├── cmd/                # Source code for all executables
│   ├── spawner/        # The core Spawner service
│   ├── app-env/        # Example Application
│   ├── app-hello/      # Example Application
│   ├── app-time/       # Example Application
│   └── webhook/        # Example Application
├── configs/            # Nginx and systemd/supervisor configuration templates
├── scripts/            # Automation scripts for building and deploying
├── web/                # Directory for compiled .fcgi files
├── go.mod
├── Dockerfile          # For containerized deployment
└── README.md
```

## 🐳 Docker Deployment

This project includes a `Dockerfile` for easy, containerized deployment. This method bundles Nginx, Supervisor, and the spawner into a single image. The spawner in the Docker image is configured to use socket-based FCGI applications (by providing the `-socketDir` flag).

#### Step 1: Build the Docker Image
```bash
docker build -t fcgi-spawner .
```

#### Step 2: Build FastCGI Applications
The build script compiles the example applications.
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
You can now test the endpoints for your `.fcgi` applications.
```bash
# Test a socket-based app
curl http://localhost:8080/app-env.fcgi
```

## 🚀 Manual Deployment Guide (systemd)

This guide is for deploying the service directly on a Linux server with `systemd`. The spawner in this setup will use socket-based FCGI applications (by providing the `-socketDir` flag).

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
You can now test the endpoints for your `.fcgi` applications.
```bash
# Test a socket-based app
curl http://<your_server_ip>/app-env.fcgi
```

## 💡 How to Add Your Own Application

The spawner *requires* the `-socketDir` flag to be provided, meaning it exclusively supports socket-based applications. Your application's code needs to be written to handle a Unix socket path provided as a command-line argument.

### Socket-Based Applications (File Name Convention: `my-app.fcgi`)

These applications receive a Unix socket path as a command-line argument and create a `net.Listener` on that path.

```go
// cmd/my-socket-app/main.go
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

	// Remove old socket file if it exists (important for clean restarts)
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		log.Fatalf("Could not remove old socket: %v", err)
	}

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("Failed to listen on socket: %v", err)
	}
	defer ln.Close() // Ensure the listener is closed when the app exits

	// Example handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello from socket app!"))
	})

	log.Println("Socket-based FCGI server starting...")
	if err := fcgi.Serve(ln, nil); err != nil {
		log.Fatalf("fcgi.Serve failed: %v", err)
	}
}
```

### Deployment Steps

1.  **Build**: Run `./scripts/build.sh`. It will automatically find and compile your new application.
2.  **Deploy**: Copy the new binary from the `web/` directory to `/var/www/fcgi` (on server) or ensure your `web` directory is mounted (in Docker).
3.  **Done!** Access your app at `http://<host>/my-app.fcgi`.

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
-   **502 Bad Gateway**: The child process is likely crashing or not responding. Check the spawner logs for errors from your application.
-   **Connection Errors**: Ensure your application is a socket-based `.fcgi` application and that the spawner is configured with `-socketDir`.

## 📄 License

This project is licensed under the [MIT License](LICENSE).