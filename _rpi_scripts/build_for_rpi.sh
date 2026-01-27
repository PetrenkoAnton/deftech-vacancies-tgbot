#!/bin/bash

cd "$(dirname "$0")"
source ../.prod.env

# Build script for Raspberry Pi ARM64

# Print status message
echo "Building for Raspberry Pi ARM64..."

# Build the Go binary for ARM64 without CGO
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o ../_bin/$PI_BINARY ../main.go

# Check if build succeeded
if [ $? -eq 0 ]; then
    # Print success message
    echo "Build successful. Binary created at ../_bin/$PI_BINARY"
else
    # Print failure message and exit
    echo "Build failed."
    exit 1
fi