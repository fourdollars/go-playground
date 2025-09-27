# Go FastCGI Spawner

This project provides a "spawner" service written in Go, designed to enable a convenient "drop-in" deployment model for FastCGI applications. Simply place a compiled Go FastCGI executable into a designated directory, and it becomes immediately accessible via Nginx. In addition to spawning FastCGI applications, the spawner can also serve static files (HTML, CSS, images, etc.) from a specified directory, acting as a simple, lightweight web server.

The system leverages **systemd Socket Activation** to ensure that the core spawner service itself consumes no resources when idle, achieving **zero resource usage when inactive**.

> **⚠️ Performance Warning:** This architecture prioritizes convenience at the cost of performance. For FastCGI applications, the system spawns a new Go process for **every single HTTP request**, similar to the classic CGI model. This is suitable for internal tools, admin panels, or low-traffic services, but it is **not recommended** for high-performance, public-facing APIs. Static file serving is generally performant.

## ✨ Features

-   **Drop-in Deployment**: Add new FastCGI applications by simply uploading a compiled binary. No need to restart or reload Nginx or systemd.
-   **Static File Serving**: Optionally serve static files like HTML, CSS, and JavaScript from a designated directory, just like a standard web server.
-   **Zero Idle Resource Usage**: Thanks to systemd socket activation, no Go processes are running when there are no requests. *(Note: This applies to the manual `systemd` deployment, not the Docker deployment.)*
-   **Centralized Configuration**: A single, one-time setup for Nginx and systemd manages an unlimited number of FastCGI applications and static content.
-   **Security Conscious**: Includes built-in path safety checks to prevent directory traversal attacks.

## 🏛️ Architecture

The request lifecycle is as follows:

```
                  +---------+      +-------------------+      +-------------------+
User Request      |         |      |                   |      |                   |
----------------->|  Nginx  |----->| systemd Socket    |----->|  Spawner Service  |
                  |         |      |(fcgi-spawner.sock)|      |  (spawner)        |
                  +---------+      +-------------------+      +---------+---------+
                                                                        |
                                                                        | Spawns FCGI app
                                                                        | or serves static file
                                                                        v
                                                            +-----------+-----------+
                                                            |                       |
                                                            |  Your Application     |
                                                            | (e.g., app-hello.fcgi)|
                                                            | or static/index.html  |
                                                            +-----------------------+
```
> **Note on Spawner Logic**: The spawner service first checks if a request matches a compiled `.fcgi` application. If it does, it spawns that application. If not, and if a static file directory is configured (`-staticRoot`), it attempts to serve the request as a static file.

> **Note on Docker**: In the Docker environment, `systemd` is replaced by `supervisor`, but the core request flow from Nginx to the spawner remains the same.

## 📂 Project Structure

```
fcgi-spawner/
├── cmd/                # Source code for all executables
│   ├── spawner/        # The core Spawner service
│   ├── app-env/        # Example Application 1 (Environment variable and request inspector)
│   ├── app-hello/      # Example Application 2
│   ├── app-time/       # Example Application 3
│   └── webhook/        # Example Application 4 (Webhook handler)
├── configs/            # Nginx and systemd configuration templates
├── scripts/            # Automation scripts for building and deploying
├── web/                # Directory for compiled .fcgi files (to be mounted into the Docker container)
├── go.mod
├── Dockerfile          # For containerized deployment
└── README.md
```

## 🐳 Docker Deployment

This project includes a `Dockerfile` for easy, containerized deployment. This method bundles Nginx and the spawner into a single image. The FastCGI applications are expected to be mounted into the container from the host.

#### Step 1: Build the Docker Image

From the root of the project, run the following command:

```bash
docker build -t fcgi-spawner .
```

#### Step 2: Build FastCGI Applications

Build your FastCGI applications on the host. The compiled binaries will be placed in the `web/` directory.

```bash
# Make the script executable
chmod +x scripts/build.sh

# Run the build script
./scripts/build.sh
```

#### Step 3: Run the Container

Run the container, mapping port 8080 on your host to port 80 in the container, and mounting the `web/` directory containing your compiled FCGI applications to `/var/www/fcgi/` inside the container:

```bash
docker run -d -p 8080:80 -v "$(pwd)/web:/var/www/fcgi" --name fcgi-container fcgi-spawner
```
> To pass additional arguments to the spawner, like `-staticRoot`, append them to the `docker run` command. See the 'Serving Static Files' section for an example.

#### Step 4: Test

You can now test the endpoints.

```bash
# Test the hello app
curl http://localhost:8080/app-hello.fcgi

# Test the time app
curl http://localhost:8080/app-time.fcgi

# Test the environment app
curl http://localhost:8080/app-env.fcgi

# Test the webhook app (POST request)
curl -X POST -H "Content-Type: application/json" -d '{"key": "value"}' http://localhost:8080/webhook.fcgi?id=<Mattermost Channel ID>
```

## 🚀 Manual Deployment Guide

This guide is for deploying the service directly on a Linux server with `systemd`.

### Prerequisites

-   A Linux server (Ubuntu/Debian recommended)
-   `sudo` access
-   Go 1.23.0+ build environment
-   Nginx installed

### Step 1: Build All Binaries

A convenient build script is provided to compile the spawner and all example applications.

```bash
# Make the script executable
chmod +x scripts/build.sh

# Run the build script
./scripts/build.sh
```

After running, you will find the compiled `app-hello.fcgi`, `app-time.fcgi`, and `webhook.fcgi` in the `web/` directory, and the `spawner` executable in the project root.

### Step 2: Deploy Files to the System

The deployment script copies the configuration files, spawner program, and example applications to their final destinations on the server.

> **Note**: This script uses `sudo`. Please review the contents of `scripts/deploy.sh` to understand the actions it will perform.

```bash
# Make the script executable
chmod +x scripts/deploy.sh

# Run the deployment script
./scripts/deploy.sh
```
> To configure the spawner with command-line flags like `-staticRoot`, you will need to edit the systemd service file at `/etc/systemd/system/fcgi-spawner.service` after deployment. See the 'Serving Static Files' section for details.

### Step 3: Enable the Services

1.  **Reload systemd and start the Spawner Socket**
    ```bash
    sudo systemctl daemon-reload
    sudo systemctl enable --now fcgi-spawner.socket
    ```

2.  **Check the Socket Status**
    ```bash
    sudo systemctl status fcgi-spawner.socket
    ```
    The status should be `active (listening)`.

3.  **Enable the Nginx Configuration and Reload**
    ```bash
    sudo ln -s /etc/nginx/sites-available/go-fcgi.conf /etc/nginx/sites-enabled/
    sudo nginx -t
    sudo systemctl reload nginx
    ```

### Step 4: Test

The deployment is complete. You can now test the endpoints using a browser or `curl`.

```bash
# Test the hello app
curl http://<your_server_ip>/app-hello.fcgi

# Test the time app
curl http://<your_server_ip>/app-time.fcgi

# Test the environment app
curl http://<your_server_ip>/app-env.fcgi

# Test the webhook app (POST request)
curl -X POST -H "Content-Type: application/json" -d '{"key": "value"}' http://<your_server_ip>/webhook.fcgi
```

## 🌐 Serving Static Files

The spawner can serve static files from a local directory by using the `-staticRoot` command-line flag.

### With Docker

1.  **Place Static Files**: Create a directory on your host machine (e.g., `my-static-files`) and place your static content inside (e.g., `index.html`, `styles.css`).

2.  **Run Container with Volume and Flag**: When running the container, mount your static files directory as a volume and pass the `-staticRoot` flag with the container path to the spawner command.

    ```bash
    docker run -d -p 8080:80 \
      -v "$(pwd)/web:/var/www/fcgi" \
      -v "$(pwd)/my-static-files:/var/www/html" \
      --name fcgi-container \
      fcgi-spawner
    ```

3.  **Test**: You can now access your static files. For example, if you have `my-static-files/about.html`, it will be available at `http://localhost:8080/about.html`.

### On a Linux Server (Manual)

1.  **Place Static Files**: Copy your static files to a directory on the server, for example, `/var/www/html`.

2.  **Configure the Service**: Edit the systemd service file to add the `-staticRoot` flag. The recommended way to do this is to create an override file.

    ```bash
    # Create and open an override file for the service
    sudo systemctl edit fcgi-spawner.service
    ```

    This will open an empty text editor. Add the following content, replacing the `ExecStart` line to include your new flag. Make sure to provide the full path to your static directory.

    ```ini
    [Service]
    ExecStart=/usr/local/bin/spawner -webRoot /var/www/fcgi -staticRoot /var/www/html
    ```

3.  **Reload and Restart**: Reload the systemd daemon to apply the changes and restart the socket.

    ```bash
    sudo systemctl daemon-reload
    sudo systemctl restart fcgi-spawner.socket
    ```

4.  **Test**: Your static files are now available at `http://<your_server_ip>/<your_file>`.


## 💡 How to Add Your Own Application

### With Docker

1.  **Create the Source Code**
    Create a new directory under `cmd/`, for example, `cmd/my-app`, and place your `main.go` file inside. Your application must accept a unix socket path as a command-line argument and use `fcgi.Serve` with a listener on that path.

2.  **Build the Application**
    Run the build script to compile your new application. The binary will be placed in the `web/` directory.
    ```bash
    ./scripts/build.sh
    ```

3.  **Run the Container**
    If you have an old container running, stop and remove it first. Then run the container, ensuring you mount the `web/` directory.
    ```bash
    docker stop fcgi-container && docker rm fcgi-container
    docker run -d -p 8080:80 -v "$(pwd)/web:/var/www/fcgi" --name fcgi-container fcgi-spawner
    ```

4.  **Done!**
    You can now access your application at `http://localhost:8080/my-app.fcgi`.

### On a Linux Server (Manual)

1.  **Create the Source Code**
    Create a new directory under `cmd/`, for example, `cmd/my-app`, and place your `main.go` file inside. Your application must accept a unix socket path as a command-line argument and use `fcgi.Serve` with a listener on that path.

2.  **Build**
    Run the build script again: `./scripts/build.sh`. It will automatically find and compile your new application.

3.  **Copy and Set Permissions**
    Copy the newly generated binary from the `web/` directory to your server's FCGI root (`/var/www/fcgi`).
    ```bash
    sudo cp web/my-app.fcgi /var/www/fcgi/
    sudo chmod +x /var/www/fcgi/my-app.fcgi
    sudo chown www-data:www-data /var/www/fcgi/my-app.fcgi
    ```

4.  **Done!**
    You can now access your application at `http://<your_server_ip>/my-app.fcgi` with no further configuration required.

## 🛠️ Troubleshooting

### With Docker
If you encounter a `502 Bad Gateway` or other errors, check the container's logs. `supervisor` directs the output of both Nginx and the spawner service to the container's stdout/stderr.
```bash
docker logs fcgi-container
docker exec -it fcgi-container /bin/bash
```

### On a Linux Server (Manual)
If you encounter a `502 Bad Gateway` error, check the spawner service logs for clues:
```bash
sudo journalctl -u fcgi-spawner.service -f
```

## 📄 License

This project is licensed under the [MIT License](LICENSE).
