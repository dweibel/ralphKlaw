#!/bin/bash
cd "$(dirname "$0")"
go test ./internal/state -v
