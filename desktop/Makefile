export PATH := $(HOME)/.bun/bin:$(HOME)/.cargo/bin:$(HOME)/.rustup/toolchains/stable-aarch64-apple-darwin/bin:$(PATH)
export RUSTUP_HOME := $(HOME)/.rustup
export CARGO_HOME := $(HOME)/.cargo

.PHONY: dev dev-bundle build test test-coverage lint fetch-sidecar

dev: fetch-sidecar
	bunx @tauri-apps/cli dev

# Debug bundle: real .app with the macOS 26 Tahoe adaptive icon.
# No hot reload — rebuild manually when source changes.
dev-bundle: fetch-sidecar
	bunx @tauri-apps/cli build --debug --bundles app
	open src-tauri/target/debug/bundle/macos/Crowbar.app

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
