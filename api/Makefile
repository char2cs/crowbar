.PHONY: dev build test lint embed-web

# Copy the built web/dist into the cmd package for go:embed.
# web/dist must be built first (cd ../web && bun run build).
embed-web:
	rm -rf cmd/crowbar/web/dist
	mkdir -p cmd/crowbar/web
	cp -r ../web/dist cmd/crowbar/web/dist

dev:
	go run ./cmd/crowbar serve

build: embed-web
	go build -o bin/crowbar ./cmd/crowbar

test:
	go test -race ./...

lint:
	go vet ./...
