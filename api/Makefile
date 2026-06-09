.PHONY: dev build test test-integration bench test-coverage lint pr-checks missing-tests embed-web

embed-web:
	@test -d ../web/dist || (echo "ERROR: web/dist not found. Run 'cd ../web && bun run build' first." && exit 1)
	rm -rf cmd/crowbar/web/dist
	mkdir -p cmd/crowbar/web
	cp -r ../web/dist cmd/crowbar/web/dist

dev:
	go run -tags noEmbed ./cmd/crowbar serve --host tcp://127.0.0.1:3737

build: embed-web
	go build -o bin/crowbar ./cmd/crowbar

test:
	go test -race ./...

test-integration:
	@if [ -d tests ]; then go test -tags integration -race ./tests/...; else echo "no tests/ directory yet (integration suite lands in a later wave)"; fi

bench:
	go test -run=^$$ -bench=. -benchmem ./...

COVERPKG := $(shell go list ./... | grep -v '/cmd/' | paste -sd, -)

test-coverage:
	@go test -race -coverpkg=$(COVERPKG) -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | awk '/^total:/{gsub("%",""); if ($$3+0 < 95) {print "Coverage " $$3 "% is below 95% threshold"; exit 1} else {print "Coverage " $$3 "% OK"}}'
	@rm -f coverage.out

lint:
	golangci-lint run ./...

pr-checks: lint test test-coverage
	go vet ./...

missing-tests:
	@for d in $$(go list -f '{{.Dir}}' ./...); do \
		if ls $$d/*_test.go >/dev/null 2>&1; then continue; fi; \
		if grep -lq '^func [A-Z]' $$d/*.go 2>/dev/null; then \
			echo "package without tests: $$d"; \
		fi; \
	done
