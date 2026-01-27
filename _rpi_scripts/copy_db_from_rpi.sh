#!/bin/bash

source ../.env

# Script to copy database from Raspberry Pi to local

# Print status message
echo "Copying database from Raspberry Pi..."

# Copy the database file from Raspberry Pi using SCP
scp -i "$PI_KEY" "$PI_USER@$PI_HOST:$PI_ROOT_PATH/$DB_NAME" "$DB_NAME"

# Print success message
echo "Database copied successfully."