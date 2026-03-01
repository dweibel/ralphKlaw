#!/bin/bash
cd "$(dirname "$0")"
go test ./... -count=1 > test-output.txt 2>&1
echo "Tests complete. Output in test-output.txt"
tail -30 test-output.txt
