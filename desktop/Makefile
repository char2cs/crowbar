export PATH := $(HOME)/.bun/bin:$(HOME)/.cargo/bin:$(HOME)/.rustup/toolchains/stable-aarch64-apple-darwin/bin:$(PATH)
export RUSTUP_HOME := $(HOME)/.rustup
export CARGO_HOME := $(HOME)/.cargo

.PHONY: dev dev-bundle build test test-coverage lint fetch-sidecar

# Dev isolation: root all Crowbar state at <workspace>/.crowbar, not ~/.crowbar
# (see root Makefile). Debug builds also default to this via cfg(debug_assertions)
# in src-tauri/src/sidecar/mod.rs, which covers the dev-bundle .app launched by
# `open` (launchd does not inherit this env).
dev: export CROWBAR_HOME ?= $(abspath ..)/.crowbar
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
