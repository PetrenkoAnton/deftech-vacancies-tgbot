#!/bin/bash

# Script to update VERSION_BUILD file with current datetime
# Usage: ./increment_version.sh

VERSION_FILE="VERSION_BUILD"

# Generate current datetime in format: YYYYMMDD-HHMMSS
current_datetime=$(date +"%Y%m%d-%H%M%S")

# Write new version
echo "$current_datetime" > "$VERSION_FILE"

echo "Updated VERSION_BUILD to: $current_datetime"