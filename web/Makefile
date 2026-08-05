.PHONY: install dev build test test-coverage lint

install:
	bun install

# CROWBAR_DEV_PORT is the caller's per-worktree origin (see desktop/Makefile).
# Honour it so a standalone `make dev-web` lands on the SAME origin the desktop
# app uses — a different port is a different browser storage, so mixing the two
# would give the same worktree two divergent entity caches.
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
