#!/bin/bash

# Build the Go project
echo "Building the project..."
go build -o ./bin/deftech-tgbot .

# Check if build was successful
if [ $? -eq 0 ]; then
    echo "Build successful. Terminating any existing instance..."
    pkill -f deftech-tgbot || true
    echo "Running the application..."
    ./bin/deftech-tgbot
else
    echo "Build failed."
    exit 1
fi