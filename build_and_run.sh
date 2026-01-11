#!/bin/bash

# Build the Go project
echo "Building the project..."
go build -o bin/miltech-tgbot .

# Check if build was successful
if [ $? -eq 0 ]; then
    echo "Build successful. Terminating any existing instance..."
    pkill -f miltech-tgbot || true
    echo "Running the application..."
    ./bin/miltech-tgbot
else
    echo "Build failed."
    exit 1
fi