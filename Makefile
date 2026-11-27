export PATH := $(HOME)/.bun/bin:$(HOME)/.cargo/bin:$(HOME)/.rustup/toolchains/stable-aarch64-apple-darwin/bin:$(PATH)
export RUSTUP_HOME := $(HOME)/.rustup
export CARGO_HOME := $(HOME)/.cargo

.PHONY: e2e meter dev dev-api dev-web dev-desktop seed seed-chat web-install build test test-coverage lint pr-checks ci docker-up docker-down ios-device ios-sim ios-doctor

# Dev isolation: every dev target roots Crowbar state (projects, store, socket,
# logs) at <this workspace>/.crowbar instead of ~/.crowbar, so a dev instance
# never collides with the production app. Scoped to dev targets only — tests
# pin the ~/.crowbar default and must run without the override.
dev dev-api dev-web dev-desktop seed seed-chat: export CROWBAR_HOME ?= $(CURDIR)/.crowbar

# Which ORIGIN `make dev-web` serves this worktree from. CROWBAR_HOME above
# isolates the daemon; this isolates the browser storage (IndexedDB,
# localStorage, service workers), which is keyed by origin. Hashed from this
# Makefile's directory, so it is stable across restarts; override to pin one.
crowbar_root := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
dev-web: export CROWBAR_DEV_PORT ?= $(shell printf '%s' '$(crowbar_root)' | cksum | awk '{print 5300 + ($$1 % 600)}')

# Parallel dev: the daemon and the shell. `make dev-web` runs the panes in a
# browser on their own.
dev:
	@$(MAKE) dev-api & $(MAKE) dev-desktop & wait

dev-api:
	$(MAKE) -C api dev

dev-web:
	$(MAKE) -C app/panes dev

# The GPUI shell (crowbar). Its panes are the web bundle, so the web deps are
# installed first; it talks to the daemon `make dev-api` runs.
dev-desktop: web-install
	cd app && cargo run -p crowbar

# Fills the RUNNING dev daemon with a throwaway repo, project, repo, feature
# workspace with a real diff, and review threads — so a new workspace does not
# start with ten minutes of import dialogs. Idempotent; safe to re-run.
# HOST defaults to the desktop sidecar's unix socket; pass
# HOST=tcp://127.0.0.1:3737 for the `make dev-api` flow.
HOST ?= unix://
seed:
	@cd api && go run -tags noEmbed ./cmd/crowbar-seed --host $(HOST)

WORKSPACE_ID ?=
TURNS ?= 20
TOOL_CALLS ?= 5

seed-chat:
	@test -n "$(WORKSPACE_ID)" || (echo "usage: make seed-chat WORKSPACE_ID=<id> [TURNS=20] [TOOL_CALLS=5]" && exit 1)
	@cd api && go run -tags noEmbed ./cmd/crowbar-seed-chat --workspace-id $(WORKSPACE_ID) --turns $(TURNS) --tool-calls $(TOOL_CALLS)

# End-to-end suite against a real dev instance of this checkout, in an isolated
# CROWBAR_HOME and webview origin (tools/scripts/e2e/README.md), driving the web bundle
# in a headless Chrome. The Tauri desktop target is gone: Tauri is mobile-only.
# ARGS passes flags through, e.g. make e2e ARGS=--only=console,recents
ARGS ?=
e2e:
	@bun tools/scripts/e2e/run.mjs --target=browser $(ARGS)

# Endpoint-security meter: execs, file opens, held fds and file changes caused
# by the desktop app and its daemon, per scenario (tools/scripts/meter/README.md).
# make meter ARGS='--only=burst --idle=60 --shims'
meter:
	@bun tools/scripts/meter/run.mjs $(ARGS)

# Release iOS app (no dev server). Free Apple ID: see docs/ios-device.md.
ios-device ios-sim ios-doctor: ios-%:
	@bash app/mobile/scripts/ios-install.sh $* $(if $(DRY_RUN),--dry-run)

web-install:
	$(MAKE) -C app/panes install

build:
	@echo "Building web..."
	@$(MAKE) -C app/panes build
	@echo "Building Go daemon..."
	@$(MAKE) -C api build
	@echo "Building the desktop shell..."
	cd app && cargo build --release -p crowbar

test:
	@$(MAKE) -C api test
	@$(MAKE) -C app/panes test
	@$(MAKE) -C app/mobile test

lint:
	@$(MAKE) -C api lint
	@$(MAKE) -C app/panes lint
	@$(MAKE) -C app/mobile lint

test-coverage:
	@$(MAKE) -C api test-coverage
	@$(MAKE) -C app/panes test-coverage
	@$(MAKE) -C app/mobile test-coverage

pr-checks: lint test-coverage build
	@echo "All PR checks passed! ✓"

ci: lint test build

docker-up:
	@$(MAKE) -C app/panes build
	docker compose -f tools/docker/docker-compose.yml up --build -d
	@echo "Crowbar running at https://localhost:$${CROWBAR_PORT:-3737} (self-signed TLS; pair with: docker compose -f tools/docker/docker-compose.yml exec crowbar crowbar pair)"

docker-down:
	docker compose -f tools/docker/docker-compose.yml down
