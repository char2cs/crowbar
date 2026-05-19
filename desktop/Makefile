.PHONY: dev build test test-coverage lint fetch-sidecar

dev: fetch-sidecar
	bunx @tauri-apps/cli dev

build: fetch-sidecar
	bunx @tauri-apps/cli build

test:
	cd src-tauri && cargo test

test-coverage:
	cd src-tauri && cargo test

lint:
	cd src-tauri && cargo fmt --check && cargo clippy -- -D warnings

fetch-sidecar:
	@echo "Fetching crowbar-api sidecar..."
	@bash scripts/fetch-sidecar.sh
