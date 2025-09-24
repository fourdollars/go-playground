# Stage 1: Build the Go spawner
FROM golang:1.23 AS builder

WORKDIR /app

# Copy go.mod and go.sum to leverage Docker cache
COPY go.mod ./
# go.sum may not exist, so we copy it if it does
RUN touch go.sum
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the spawner
RUN go build -o /app/spawner ./cmd/spawner

# Ensure it is executable for the next stage
RUN chmod +x /app/spawner

# Stage 2: Setup the final image with Nginx and the spawner
FROM ubuntu:22.04

# Install Nginx and supervisor.
# supervisor is used as a process manager since systemd is not available in Docker.
RUN apt-get update && \
    apt-get install -y nginx supervisor && \
    rm -rf /var/lib/apt/lists/*

# Copy compiled files from the builder stage
COPY --from=builder /app/spawner /usr/local/bin/spawner

# The /var/www/html/ directory is expected to be mounted externally,
# containing the FCGI applications.
# Create empty config files for the webhook app to prevent it from crashing.
RUN touch /var/www/html/.webhook.fcgi.url /var/www/html/.webhook.fcgi.secret

# Create a basic Nginx site configuration for the Go FCGI apps.
# This configuration is based on the project's description.
# It listens on port 80 and forwards requests for .fcgi files
# to the spawner service via a Unix socket.
COPY configs/go-fcgi.conf /etc/nginx/sites-available/go-fcgi.conf

# Disable the default Nginx site and enable our new site
RUN rm -f /etc/nginx/sites-enabled/default && \
    ln -s /etc/nginx/sites-available/go-fcgi.conf /etc/nginx/sites-enabled/

# Create a supervisor configuration to run Nginx and the spawner.
# In the original project, systemd is used for socket activation and service management.
# In this Docker environment, supervisord provides a similar capability to manage long-running services.
RUN echo "[supervisord]\n\
nodaemon=true\n\
\n[program:nginx]\n\
command=/usr/sbin/nginx -g 'daemon off;'\n\
autostart=true\n\
autorestart=true\n\
priority=10\n\
stdout_logfile=/dev/stdout\n\
stdout_logfile_maxbytes=0\n\
stderr_logfile=/dev/stderr\n\
stderr_logfile_maxbytes=0\n\
\n[program:spawner]\n\
command=/usr/local/bin/spawner -webRoot /var/www/html -idleTimeout 1m -socketDir /tmp/fcgi-spawner-sockets -listenAddr :9000\n\
autostart=true\n\
autorestart=true\n\
priority=20\n\
stdout_logfile=/dev/stdout\n\
stdout_logfile_maxbytes=0\n\
stderr_logfile=/dev/stderr\n\
stderr_logfile_maxbytes=0\n\n" > /etc/supervisor/conf.d/supervisord.conf

# Expose port 80 for HTTP traffic
EXPOSE 80
EXPOSE 9000

# Set the default command to run supervisord
CMD ["/usr/bin/supervisord"]
