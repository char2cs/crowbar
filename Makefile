export PATH := $(HOME)/.bun/bin:$(HOME)/.cargo/bin:$(HOME)/.rustup/toolchains/stable-aarch64-apple-darwin/bin:$(PATH)
export RUSTUP_HOME := $(HOME)/.rustup
export CARGO_HOME := $(HOME)/.cargo

.PHONY: dev dev-api dev-web dev-desktop dev-bundle seed web-install build test test-coverage lint pr-checks ci docker-up docker-down

# Dev isolation: every dev target roots Crowbar state (projects, store, socket,
# logs) at <this workspace>/.crowbar instead of ~/.crowbar, so a dev instance
# never collides with the production app. Scoped to dev targets only — tests
# pin the ~/.crowbar default and must run without the override.
dev dev-api dev-web dev-desktop dev-bundle seed: export CROWBAR_HOME ?= $(CURDIR)/.crowbar

# Which worktree launched this dev instance. Debug builds put it in the window
# title (see apply_dev_window_title in desktop/src-tauri/src/lib.rs) so that the
# several dev apps running on one machine — one per worktree, all of them a
# `target/debug/crowbar-desktop` window called "Crowbar" — can be told apart.
#
# Asked of THIS Makefile's directory, not the caller's: git answers a question
# about a directory outside a worktree with whichever repository ENCLOSES it, so
# a `make -f .../Makefile` run from elsewhere would confidently label the window
# with a stranger's branch. Empty output (no git, not a repo) is fine — the
# window keeps its plain "Crowbar" title rather than a half-filled one.
crowbar_root := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
dev dev-api dev-web dev-desktop dev-bundle: export CROWBAR_DEV_LABEL ?= $(shell git -C $(crowbar_root) rev-parse --abbrev-ref HEAD 2>/dev/null)

# Which ORIGIN this worktree's dev app is served from. CROWBAR_HOME above
# isolates the daemon; this isolates the webview, whose IndexedDB, localStorage
# and service workers are keyed by origin and were shared by every worktree back
# when the dev URL was a hardcoded localhost:5173. See desktop/Makefile for the
# full rationale and the corruption it caused. Derived from the same worktree
# root as desktop/Makefile so both entry points agree; override to pin one.
dev dev-web dev-desktop: export CROWBAR_DEV_PORT ?= $(shell printf '%s' '$(crowbar_root)' | cksum | awk '{print 5300 + ($$1 % 600)}')

# Parallel dev. dev-web is deliberately NOT started here: dev-desktop's
# beforeDevCommand already runs Vite on CROWBAR_DEV_PORT, and a second one on
# the same port now fails outright under --strictPort. It used to "work" only
# because the old beforeDevCommand opened with `pkill -f vite`, silently killing
# the dev-web this target had just launched (and every other worktree's too).
# `make dev-web` remains available on its own for a browser-only session.
dev:
	@$(MAKE) dev-api & $(MAKE) dev-desktop & wait

dev-api:
	$(MAKE) -C api dev

dev-web:
	$(MAKE) -C web dev

# Preinstall web deps before launching the desktop app: Tauri's beforeDevCommand
# runs the Vite dev server (cd ../web && npm run dev), which needs web/node_modules
# to already exist. Kept as a prerequisite so a fresh checkout just works.
dev-desktop: web-install
	$(MAKE) -C desktop dev

# Fills the RUNNING dev daemon with a throwaway repo, project, repo, feature
# workspace with a real diff, and review threads — so a new workspace does not
# start with ten minutes of import dialogs. Idempotent; safe to re-run.
# HOST defaults to the desktop sidecar's unix socket; pass
# HOST=tcp://127.0.0.1:3737 for the `make dev-api` flow.
HOST ?= unix://
seed:
	@cd api && go run -tags noEmbed ./cmd/crowbar-seed --host $(HOST)

web-install:
	$(MAKE) -C web install

# Debug bundle with macOS 26 Tahoe adaptive icon — no hot reload.
dev-bundle:
	@echo "Building web..."
	@$(MAKE) -C web build
	@echo "Building debug bundle..."
	@$(MAKE) -C desktop dev-bundle

build:
	@echo "Building web..."
	@$(MAKE) -C web build
	@echo "Building Go daemon..."
	@$(MAKE) -C api build
	@echo "Building Tauri app..."
	@$(MAKE) -C desktop build

test:
	@$(MAKE) -C api test
	@$(MAKE) -C web test
	@$(MAKE) -C desktop test

lint:
	@$(MAKE) -C api lint
	@$(MAKE) -C web lint
	@$(MAKE) -C desktop lint

test-coverage:
	@$(MAKE) -C api test-coverage
	@$(MAKE) -C web test-coverage
	@$(MAKE) -C desktop test-coverage

pr-checks: lint test-coverage build
	@echo "All PR checks passed! ✓"

ci: lint test build

docker-up:
	@$(MAKE) -C web build
	docker compose -f docker/docker-compose.yml up --build -d
	@echo "Crowbar running at http://localhost:3737"

docker-down:
	docker compose -f docker/docker-compose.yml down
