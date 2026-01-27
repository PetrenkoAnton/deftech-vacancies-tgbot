#!/bin/bash

cd "$(dirname "$0")"
source ../.env

# Script to copy binary to Raspberry Pi

# Print status message
echo "Copying binary to Raspberry Pi..."

# Copy the binary to Raspberry Pi using SCP
scp -i "$PI_KEY" ../_bin/$PI_BINARY "$PI_USER@$PI_HOST:$PI_ROOT_PATH/"

# Print status message
echo "Copying environment file to Raspberry Pi..."

# Copy the .prod.env file to Raspberry Pi using SCP
scp -i "$PI_KEY" ../.prod.env "$PI_USER@$PI_HOST:$PI_ROOT_PATH/.env"