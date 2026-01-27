#!/bin/bash

source ../.env

# Script to copy binary to Raspberry Pi

# Print status message
echo "Copying binary to Raspberry Pi..."

# Copy the binary to Raspberry Pi using SCP
scp -i "$PI_KEY" _bin/$PI_BINARY "$PI_USER@$PI_HOST:$PI_ROOT_PATH/"

# Print success message
echo "File copied successfully."