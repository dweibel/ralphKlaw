#!/bin/bash
cd "$(dirname "$0")"
go test ./... -count=1 -short
