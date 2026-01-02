#!/bin/bash

# Maintainerd Local Runner Script

set -e

echo "=== Maintainerd Local Runner ==="
echo ""

# Check if .env file exists
if [ ! -f .env ]; then
    echo "Error: .env file not found!"
    echo "Please create .env file with your credentials (see .env.example)"
    exit 1
fi

# Load environment variables
echo "Loading environment variables from .env..."
export $(cat .env | grep -v '^#' | xargs)

# Check if database exists, if not, initialize it
if [ ! -f maintainers.db ]; then
    echo ""
    echo "Database not found. Initializing..."
    if [ -f init_demo_db ]; then
        ./init_demo_db
    else
        echo "Error: init_demo_db executable not found. Please build it first:"
        echo "  go build -o init_demo_db cmd/init_demo_db/main.go"
        exit 1
    fi
    echo "Database initialized successfully!"
fi

# Check if main executable exists
if [ ! -f maintainerd ]; then
    echo ""
    echo "Main executable not found. Building..."
    go build -o maintainerd main.go
    echo "Build complete!"
fi

echo ""
echo "Starting Maintainerd server..."
echo "Press Ctrl+C to stop"
echo ""

# Run the application
./maintainerd --db-path=maintainers.db --addr=:2525
