.PHONY: install dev build test test-coverage lint

install:
	bun install

# CROWBAR_DEV_PORT is the caller's per-worktree origin (see the root Makefile):
# a different port is a different browser storage.
dev:
	bun run dev $(if $(CROWBAR_DEV_PORT),-- --port $(CROWBAR_DEV_PORT) --strictPort)

build:
	bun run build

test:
	bun run test:coverage

test-coverage:
	bun run test:coverage

lint:
	bun run lint
