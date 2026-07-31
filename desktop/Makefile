export PATH := $(HOME)/.bun/bin:$(HOME)/.cargo/bin:$(HOME)/.rustup/toolchains/stable-aarch64-apple-darwin/bin:$(PATH)
export RUSTUP_HOME := $(HOME)/.rustup
export CARGO_HOME := $(HOME)/.cargo

.PHONY: dev dev-bundle build test test-coverage lint fetch-sidecar

# Dev isolation: root all Crowbar state at <workspace>/.crowbar, not ~/.crowbar
# (see root Makefile). Debug builds also default to this via cfg(debug_assertions)
# in src-tauri/src/sidecar/mod.rs, which covers the dev-bundle .app launched by
# `open` (launchd does not inherit this env).
dev: export CROWBAR_HOME ?= $(abspath ..)/.crowbar

# Worktree label for the dev window title (see root Makefile). Repeated here so
# a bare `cd desktop && make dev` is labelled too; `?=` leaves the root
# Makefile's value alone when it delegates here.
crowbar_root := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
dev: export CROWBAR_DEV_LABEL ?= $(shell git -C $(crowbar_root) rev-parse --abbrev-ref HEAD 2>/dev/null)
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
