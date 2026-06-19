export PATH := $(HOME)/.bun/bin:$(HOME)/.cargo/bin:$(HOME)/.rustup/toolchains/stable-aarch64-apple-darwin/bin:$(PATH)
export RUSTUP_HOME := $(HOME)/.rustup
export CARGO_HOME := $(HOME)/.cargo

.PHONY: dev dev-api dev-web dev-desktop dev-bundle build test test-coverage lint pr-checks ci docker-up docker-down

# Parallel dev: starts all three subsystems
dev:
	@$(MAKE) dev-api & $(MAKE) dev-web & $(MAKE) dev-desktop & wait

dev-api:
	$(MAKE) -C api dev

dev-web:
	$(MAKE) -C web dev

dev-desktop:
	$(MAKE) -C desktop dev

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
