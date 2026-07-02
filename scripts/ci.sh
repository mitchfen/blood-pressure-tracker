#!/usr/bin/env bash
set -e

# Get the absolute path to the root directory
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." >/dev/null 2>&1 && pwd)"
cd "$DIR/src" || exit 1

echo "Downloading dependencies..."
go mod download

echo "Running go vet..."
go vet ./...

echo "Building project..."
go build -v -o ../server .

echo "Running tests..."
"$DIR/scripts/test.sh"
