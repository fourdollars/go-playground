#!/bin/bash
set -e

echo "--- Deploying FCGI Spawner System ---"
echo "This script requires sudo access to copy files and set permissions."

# --- Configuration ---
# System paths
NGINX_SITES_AVAILABLE="/etc/nginx/sites-available"
SYSTEMD_PATH="/etc/systemd/system"
BIN_PATH="/usr/local/bin"
WEB_ROOT="/var/www/html"

# Source paths
CONFIG_SRC="./configs"
BUILD_ROOT="."
WEB_SRC="./web"

# --- Build Spawner ---
echo "Building spawner..."
go build -o "$BUILD_ROOT/spawner" ./cmd/spawner

# --- Safety Check ---
# Ensure FCGI apps have been built
if [ ! -d "$WEB_SRC" ]; then
    echo "Error: Web directory for FCGI applications not found."
    echo "Please run './scripts/build.sh' first to build the FCGI applications."
    exit 1
fi

# --- Deployment Actions ---

# 1. Copy Spawner Executable
echo "Installing spawner executable to $BIN_PATH..."
sudo cp "$BUILD_ROOT/spawner" "$BIN_PATH/spawner"
sudo chmod +x "$BIN_PATH/spawner"

# 2. Copy systemd files
echo "Installing systemd service and socket files..."
sudo cp "$CONFIG_SRC/fcgi-spawner.service" "$SYSTEMD_PATH/"
sudo cp "$CONFIG_SRC/fcgi-spawner.socket" "$SYSTEMD_PATH/"

# 3. Copy Nginx configuration
echo "Installing Nginx configuration..."
sudo cp "$CONFIG_SRC/go-fcgi.conf" "$NGINX_SITES_AVAILABLE/"

# 4. Copy example FCGI applications to web root
echo "Deploying example applications to $WEB_ROOT..."
# Ensure web root exists
sudo mkdir -p $WEB_ROOT
sudo cp "$WEB_SRC"/*.fcgi "$WEB_ROOT/"

# 5. Set permissions for FCGI applications
echo "Setting permissions for applications in $WEB_ROOT..."
for f in "$WEB_ROOT"/*.fcgi; do
    [ -e "$f" ] || continue
    sudo chmod +x "$f"
    sudo chown www-data:www-data "$f"
done

echo "--- Deployment Complete ---"
echo ""
echo "Next steps:"
echo "1. sudo systemctl daemon-reload"
echo "2. sudo systemctl enable --now fcgi-spawner.socket"
echo "3. sudo ln -s $NGINX_SITES_AVAILABLE/go-fcgi.conf /etc/nginx/sites-enabled/"
echo "4. sudo nginx -t && sudo systemctl reload nginx"
echo "5. Test with: curl http://<your_server_ip>/hello.fcgi"
