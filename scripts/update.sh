#!/usr/bin/env bash
# Get the absolute path to the root directory
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." >/dev/null 2>&1 && pwd)"
cd "$DIR/src" || exit 1

go get -u ./...
go mod tidy
