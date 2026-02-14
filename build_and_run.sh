#!/bin/bash

# Update build version
echo "Updating build version →"
./update_build_version.sh

# Build the Go project
echo "Building the project →"
go build -ldflags "-X main.version=$(cat VERSION) -X main.buildVersion=$(cat VERSION_BUILD)" -o ./_bin/deftech-tgbot .

# Check if build was successful
if [ $? -eq 0 ]; then
    echo "Build successful."
    echo "Terminating any existing instance →"
    pkill -f deftech-tgbot || true
    echo "Running the application →"
    ./_bin/deftech-tgbot
else
    echo "Build failed."
    exit 1
fi