.PHONY: dev build test lint ci

dev:
	@echo "Starting Crowbar in development mode..."
	@$(MAKE) -C api dev &
	@$(MAKE) -C web dev &
	@$(MAKE) -C desktop dev

build:
	@$(MAKE) -C web build
	@$(MAKE) -C api build
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
