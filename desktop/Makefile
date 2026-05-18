.PHONY: dev build test lint fetch-sidecar

dev: fetch-sidecar
	bunx tauri dev

build: fetch-sidecar
	bunx tauri build

test:
	cd src-tauri && cargo test

lint:
	cd src-tauri && cargo fmt --check && cargo clippy -- -D warnings

fetch-sidecar:
	@echo "Fetching crowbar-api sidecar..."
	@bash scripts/fetch-sidecar.sh
