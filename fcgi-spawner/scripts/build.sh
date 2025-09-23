#!/bin/bash
set -e

echo "--- Building Go Binaries ---"

# Ensure output directories exist
mkdir -p ./web

# Build the spawner
echo "Building spawner..."
go build -o ./spawner ./cmd/spawner

# Find all app directories in cmd/ and build them
for app_path in ./cmd/*; do
    if [ -d "$app_path" ]; then
        app_name=$(basename "$app_path")
        if [ "$app_name" != "spawner" ]; then
            echo "Building $app_name..."
            # The output name should be app_name.fcgi
            go build -o "./web/${app_name}.fcgi" "$app_path"
        fi
    fi
done

echo "--- Build Complete ---"
echo "Spawner executable is at: ./spawner"
echo "FCGI applications are in: ./web/"
