export PATH := $(HOME)/.bun/bin:$(HOME)/.cargo/bin:$(HOME)/.rustup/toolchains/stable-aarch64-apple-darwin/bin:$(PATH)
export RUSTUP_HOME := $(HOME)/.rustup
export CARGO_HOME := $(HOME)/.cargo

.PHONY: dev dev-solo dev-bundle build test test-coverage lint fetch-sidecar

# Dev isolation: root all Crowbar state at <workspace>/.crowbar, not ~/.crowbar
# (see root Makefile). Debug builds also default to this via cfg(debug_assertions)
# in src-tauri/src/sidecar/mod.rs, which covers the dev-bundle .app launched by
# `open` (launchd does not inherit this env).
dev: export CROWBAR_HOME ?= $(abspath ..)/.crowbar
dev: fetch-sidecar
	bunx @tauri-apps/cli dev

# Same as `dev`, but safe to run while OTHER checkouts are running their own.
#
# `dev` is not: its beforeDevCommand starts with `pkill -f vite`, which matches
# every Vite on the machine rather than this worktree's, and it pins port 5173 so
# two checkouts fight over it anyway. On a machine where several worktrees are
# worked on at once that kills someone else's session outright.
#
# This target starts its own Vite on a port derived from the worktree path, runs
# no pkill, and puts the branch in the window title so a human — or another agent
# reading `ps` — can tell whose window it is before touching it.
DEV_PORT ?= $(shell printf '%d' $$(( 5200 + $$(echo "$(abspath ..)" | cksum | cut -d' ' -f1) % 300 )))
DEV_TAG  ?= $(shell cd .. && git rev-parse --abbrev-ref HEAD 2>/dev/null || echo detached)
dev-solo: export CROWBAR_HOME ?= $(abspath ..)/.crowbar
dev-solo: fetch-sidecar
	@echo "dev-solo: port $(DEV_PORT), window title 'Crowbar — $(DEV_TAG)'"
	@cd ../web && (npm run dev -- --port $(DEV_PORT) --strictPort &) && \
	  until curl -sf -o /dev/null http://localhost:$(DEV_PORT); do sleep 0.5; done
	bunx @tauri-apps/cli dev --config '{"build":{"beforeDevCommand":"","devUrl":"http://localhost:$(DEV_PORT)"},"app":{"windows":[{"title":"Crowbar — $(DEV_TAG)"}]}}'

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
