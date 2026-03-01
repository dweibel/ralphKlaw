#!/bin/bash
cd "$(dirname "$0")"
echo "Running tests..."
go test ./... -count=1 > /tmp/ralphklaw-test-output.txt 2>&1
cat /tmp/ralphklaw-test-output.txt
echo ""
echo "Exit code: $?"
