.PHONY: dev build test lint embed-web

# Copy the built web/dist into the cmd package for go:embed.
# web/dist must be built first (cd ../web && bun run build).
embed-web:
	@test -d ../web/dist || (echo "ERROR: web/dist not found. Run 'cd ../web && bun run build' first." && exit 1)
	rm -rf cmd/crowbar/web/dist
	mkdir -p cmd/crowbar/web
	cp -r ../web/dist cmd/crowbar/web/dist

# Dev mode skips embedding — the Vite dev server handles the frontend.
dev:
	go run -tags noEmbed ./cmd/crowbar serve

build: embed-web
	go build -o bin/crowbar ./cmd/crowbar

test:
	go test -race ./...

lint:
	go vet ./...
