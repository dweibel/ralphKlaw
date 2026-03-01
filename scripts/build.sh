#!/bin/bash
set -e
cd "$(dirname "$0")/.."
mkdir -p bin
echo "Building ralphKlaw..."
go build -o bin/ralphklaw ./cmd/ralphklaw
echo "Build successful! Binary at bin/ralphklaw"
