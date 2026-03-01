.PHONY: build test clean

build:
	mkdir -p bin
	go build -o bin/ralphklaw ./cmd/ralphklaw

test:
	go test ./... -count=1 -v

test-cover:
	go test ./... -count=1 -coverprofile=coverage.out
	go tool cover -func=coverage.out

clean:
	rm -rf bin/
	rm -f coverage.out
	go clean ./...
