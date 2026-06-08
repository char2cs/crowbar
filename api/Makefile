.PHONY: dev build test test-integration bench bench-update test-coverage lint govet pr-checks missing-tests embed-web

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
	go test -tags noEmbed -race ./...

test-integration:
	go test -tags 'integration noEmbed' -race -v -timeout 600s -p 1 ./...

bench:
	@test -d tests/integration/bench || (echo 'ERROR: tests/integration/bench not found' && exit 1)
	go test -tags 'integration noEmbed' -run='^$$' -bench=. -benchmem -timeout 300s ./tests/integration/bench/...

bench-update:
	@test -d tests/integration/bench || (echo 'ERROR: tests/integration/bench not found' && exit 1)
	UPDATE_BASELINE=1 go test -tags 'integration noEmbed' -run='^$$' -bench=. -benchmem -timeout 300s ./tests/integration/bench/...

COVERPKG := $(shell go list -tags noEmbed ./... | grep -v '/cmd/' | paste -sd, -)

# NOTE: coverage gate covers unit tests only (-tags integration is intentionally excluded).
# Integration tests are exercised separately via `make test-integration`.
# The 95% threshold applies to unit-testable packages.
# /cmd/ packages are excluded by the grep filter.
# tests/integration and tests/kit packages are excluded implicitly because every file
# in those directories carries //go:build integration and go list omits them without -tags integration.
test-coverage:
	@go test -tags noEmbed -race -coverpkg=$(COVERPKG) -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | awk '/^total:/{gsub("%",""); if ($$3+0 < 95) {print "Coverage " $$3 "% is below 95% threshold"; exit 1} else {print "Coverage " $$3 "% OK"}}'
	@rm -f coverage.out

lint:
	golangci-lint run --build-tags noEmbed ./...

govet:
	go vet -tags noEmbed ./...

# test-integration is excluded here; run it separately or in CI with a live repo.
pr-checks: lint test test-coverage govet

missing-tests:
	@for d in $$(go list -tags noEmbed -f '{{.Dir}}' ./... | grep -v '/cmd/'); do \
		if ls $$d/*_test.go >/dev/null 2>&1; then continue; fi; \
		if grep -lq '^func [A-Z]' $$d/*.go 2>/dev/null; then \
			echo "package without tests: $$d"; \
		fi; \
	done
