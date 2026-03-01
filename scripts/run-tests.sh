#!/bin/bash
cd "$(dirname "$0")"
echo "=== Running all tests ==="
go test ./... -count=1
echo ""
echo "=== Test Summary ==="
go test ./... -count=1 2>&1 | grep -E "^(ok|FAIL|---)"
