#!/bin/bash

# Create necessary directories with proper permissions
mkdir -p ./config
mkdir -p ./logs
mkdir -p ./data

# Ensure permissions (adjust UID/GID if needed for your Unraid setup)
chmod -R 755 ./logs

echo "Directories created and permissions set."