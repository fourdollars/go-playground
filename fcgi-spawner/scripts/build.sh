#!/bin/sh
set -e

echo "--- Building Go FCGI Applications ---"

# Ensure output directory exists
mkdir -p ./web

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
echo "FCGI applications are in: ./web/"
