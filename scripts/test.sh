#!/bin/bash
set -e
cd "$(dirname "$0")"
echo "Running tests..."
go test ./... -v -count=1
echo "Tests complete!"
