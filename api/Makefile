.PHONY: dev build test test-integration bench test-coverage lint pr-checks missing-tests embed-web

embed-web:
	@test -d ../web/dist || (echo "ERROR: web/dist not found. Run 'cd ../web && bun run build' first." && exit 1)
	rm -rf cmd/crowbar/web/dist
	mkdir -p cmd/crowbar/web
	cp -r ../web/dist cmd/crowbar/web/dist

dev:
	go run -tags noEmbed ./cmd/crowbar serve

build: embed-web
	go build -o bin/crowbar ./cmd/crowbar

test:
	go test -race ./...

test-integration:
	go test -tags integration -race ./tests/...

bench:
	go test -run=^$$ -bench=. -benchmem ./...

test-coverage:
	@go test -race -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | awk '/^total:/{gsub("%",""); if ($$3+0 < 95) {print "Coverage " $$3 "% is below 95% threshold"; exit 1} else {print "Coverage " $$3 "% OK"}}'
	@rm -f coverage.out

lint:
	golangci-lint run ./...

pr-checks: lint test test-coverage
	go vet ./...

missing-tests:
	@for f in $$(find internal cmd -name '*.go' ! -name '*_test.go'); do \
		t=$${f%.go}_test.go; \
		if [ ! -f "$$t" ] && ! grep -qL 'type .* struct' "$$f" 2>/dev/null; then \
			echo "missing test: $$f"; \
		fi; \
	done
