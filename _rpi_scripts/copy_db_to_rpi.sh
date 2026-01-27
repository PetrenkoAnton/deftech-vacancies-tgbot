#!/bin/bash

cd "$(dirname "$0")"
source ../.env

# Script to copy database to Raspberry Pi

# Print status message
echo "Copying database to Raspberry Pi..."

# Copy the database file to Raspberry Pi using SCP
scp -i "$PI_KEY" "../$DB_NAME" "$PI_USER@$PI_HOST:$PI_ROOT_PATH/"

# Print success message
echo "File copied successfully."