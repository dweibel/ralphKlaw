#!/bin/bash
cd "$(dirname "$0")"
echo "Running tests with coverage..."
go test ./... -coverprofile=/tmp/ralphklaw-cover.out -covermode=atomic
echo ""
echo "Coverage summary:"
go tool cover -func=/tmp/ralphklaw-cover.out | tail -1
