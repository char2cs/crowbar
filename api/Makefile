.PHONY: dev build test lint

dev:
	go run ./cmd/crowbar serve

build:
	go build -o bin/crowbar ./cmd/crowbar

test:
	go test -race ./...

lint:
	go vet ./...
