.PHONY: dev dev-api dev-web dev-desktop build test lint ci docker-up docker-down

# Parallel dev: starts all three subsystems
dev:
	@$(MAKE) dev-api & $(MAKE) dev-web & $(MAKE) dev-desktop; wait

dev-api:
	$(MAKE) -C api dev

dev-web:
	$(MAKE) -C web dev

dev-desktop:
	$(MAKE) -C desktop dev

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

ci: lint test build

docker-up:
	@$(MAKE) -C web build
	docker compose -f docker/docker-compose.yml up --build -d
	@echo "Crowbar running at http://localhost:3737"

docker-down:
	docker compose -f docker/docker-compose.yml down
