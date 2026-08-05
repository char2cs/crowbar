export PATH := $(HOME)/.bun/bin:$(HOME)/.cargo/bin:$(HOME)/.rustup/toolchains/stable-aarch64-apple-darwin/bin:$(PATH)
export RUSTUP_HOME := $(HOME)/.rustup
export CARGO_HOME := $(HOME)/.cargo

.PHONY: dev dev-solo dev-bundle build test test-coverage lint fetch-sidecar

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

# ORIGIN ISOLATION — the second half of dev isolation, and the one that was
# missing. CROWBAR_HOME above isolates the DAEMON (projects, store, socket,
# logs). It does nothing for the WEBVIEW: IndexedDB, localStorage and service
# worker registrations are keyed by ORIGIN, and every dev app used to be served
# from the same hardcoded http://localhost:5173. Several worktrees running at
# once therefore shared one browser storage and quietly corrupted each other —
# observed as an entity cache holding 16 repos across 16 project ids from
# instances this worktree never ran, a sidebar that renders empty because the
# tree is built from that cache, and every /ide/<proj>/<repo>/<ws> deep link
# bouncing to /home because the route guard reads it too. Nothing surfaced,
# because a failed cache seed is deliberately swallowed (lib/ws/entity-stream.ts).
#
# Deriving the port from THIS worktree's path gives each one its own origin and
# therefore its own storage. It is a hash, not a counter, so the port is stable
# across restarts (a moving port would orphan storage on every launch) and needs
# no coordination between worktrees. Override with CROWBAR_DEV_PORT to pin one.
#
# cksum is POSIX and agrees on macOS and Linux for the first field; 600 ports
# starting at 5300 stays clear of Vite's own 5173 default, so a colleague still
# on the old config does not land on top of us.
#
# Hashed from the WORKTREE ROOT, not this directory, so `make dev-desktop` from
# the root and `cd desktop && make dev` derive the SAME port. They must: a
# different port is a different origin, and the same worktree would otherwise
# keep two divergent entity caches depending on which way it was launched.
crowbar_ws_root := $(abspath $(crowbar_root)/..)
crowbar_dev_port := $(shell printf '%s' '$(crowbar_ws_root)' | cksum | awk '{print 5300 + ($$1 % 600)}')
dev: export CROWBAR_DEV_PORT ?= $(crowbar_dev_port)

# --strictPort is load-bearing: without it Vite silently takes the next free
# port while devUrl below still points at the requested one, and the window
# loads nothing. Failing loudly names the conflict instead. Note there is
# deliberately NO `pkill -f vite` here — the committed beforeDevCommand used to
# carry one, and it killed every OTHER worktree's dev server on the machine.
dev: fetch-sidecar
	@echo "dev origin: http://localhost:$(CROWBAR_DEV_PORT)  (isolated per worktree)"
	bunx @tauri-apps/cli dev --config '{"build":{"devUrl":"http://localhost:$(CROWBAR_DEV_PORT)","beforeDevCommand":"cd ../web && npm run dev -- --port $(CROWBAR_DEV_PORT) --strictPort"}}'

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
