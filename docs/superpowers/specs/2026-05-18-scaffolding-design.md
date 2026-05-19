# Crowbar Scaffolding — Design Spec

**Date:** 2026-05-18
**Phase:** Scaffolding
**Status:** Approved

---

## Overview

This spec covers the initial scaffolding of all four major Crowbar subsystems: the Go daemon, the React frontend, the Tauri desktop wrapper, and the CI/CD pipeline. The goal is a fully wired skeleton — compiling, building, and passing CI — with no placeholder logic, so the next phase can add features into a production-grade structure.

---

## Repository Structure

Single monorepo. All subsystems co-located.

```
crowbar/
├── api/                        # Go daemon
├── web/                        # React SPA (no Tauri coupling)
├── desktop/                    # Tauri desktop wrapper
├── flows/                      # YAML flow definitions (empty for now)
├── docker/
│   └── docker-compose.yml      # Cloud deployment (single crowbar service)
├── docs/
│   ├── plans/
│   └── superpowers/specs/
├── Makefile                    # Top-level dev commands
└── .github/
    └── workflows/
        ├── ci.yml
        ├── prerelease.yml
        ├── stable-release.yml
        └── backport.yml
```

---

## 1. Go Daemon (`api/`)

### Architecture

Mirrors Quiver.core's layered hexagonal architecture exactly. Same container/DI pattern, same layer responsibilities, Crowbar-specific engines.

```
api/
├── cmd/crowbar/
│   └── main.go                 # Entry point: Cobra root + serve/version commands
└── internal/
    ├── domain/                 # Core models: Project, Workspace, Flow, Memory
    ├── adapter/                # Storage: Asynx event store + GORM (SQLite)
    ├── app/                    # Usecases, repositories, SSE hub
    ├── engine/                 # Orchestrator, FlowRunner, AIBridge, MemoryEngine, DockerPool
    ├── api/                    # Gin HTTP server, versioned routes (/api/v0/...), SSE endpoint
    └── core/                   # Config, logging, gateway
```

**Layer responsibilities:**
- `domain/` — pure Go structs, no dependencies
- `adapter/` — SQLite via GORM for relational data; Asynx + SQLite for event sourcing; LanceDB Go client for vector storage
- `app/` — business logic interfaces (usecases) and their implementations; SSE hub for pushing events to connected clients
- `engine/` — long-running background components: Orchestrator (phase transitions), FlowRunner (YAML execution), AIBridge (Anthropic SDK, OpenAI-compatible), MemoryEngine (LanceDB read/write), DockerPool (container lifecycle)
- `api/` — Gin router, middleware (logging, recovery, timing), versioned handlers, SSE handler, static file serving (cloud mode)
- `core/` — config loading, structured logging, gateway (transport abstraction)

### Transport

Dual-gateway identical to Quiver.core. Selected via `--host` CLI flag:

- `unix://` (default) → `~/.crowbar/crowbar.sock` — local mode, no TCP port opened
- `tcp://0.0.0.0:3737` → cloud mode (default port 3737)

### Static File Serving (Cloud Mode)

In cloud mode, the Go binary embeds and serves `web/dist/` at `/` using Go's `embed` package. API routes are prefixed `/api/v0/`. The React app uses relative URLs — no CORS configuration needed.

### Storage

Both backends are embedded — no external services required:

| Layer | Backend | Path |
|---|---|---|
| Relational + events | SQLite via GORM + Asynx | `~/.crowbar/crowbar.db` |
| Vector (memory) | LanceDB embedded | `~/.crowbar/memory/` |

PostgreSQL support is deferred. The adapter layer is designed so a PostgreSQL implementation can be swapped in later without touching the app or engine layers.

### Module & Dependencies

- **Module**: `github.com/rabbytesoftware/crowbar/api`
- **Go version**: 1.26.2
- **Key deps**: `gin-gonic/gin`, `gorm.io/gorm`, `glebarez/sqlite`, `char2cs/asynx`, `spf13/cobra`, `lancedb/lancedb-go`, Anthropic Go SDK, `swaggo/swag`
- **Risk**: The LanceDB Go SDK is less mature than its Python/JS counterparts. Embedded mode availability in Go must be validated early in implementation; fallback is `sqlite-vec` (SQLite vector extension with a stable Go driver).

---

## 2. Frontend (`web/`)

### Architecture

Standalone React SPA. No `@tauri-apps/api` imports anywhere. Deployable independently — served by Go in cloud mode, embedded in the Tauri webview in local mode.

```
web/
├── src/
│   ├── routes/                 # TanStack Router file-based pages
│   ├── lib/
│   │   └── transport.ts        # Single transport abstraction (see below)
│   ├── domain/                 # TypeScript models mirroring Go domain
│   └── main.tsx
├── vite.config.ts
├── tsconfig.json
└── package.json                # Bun package manager
```

### Stack

- **Build**: Vite 8, Bun
- **Router**: TanStack Router (file-based, type-safe)
- **Data fetching**: TanStack Query
- **State**: Zustand
- **UI**: Tailwind CSS v4 + shadcn/ui
- **Testing**: Vitest + React Testing Library
- **Quality**: Prettier, ESLint, TypeScript strict mode

### Transport Abstraction

The frontend's only environment-aware code lives in `transport.ts`:

```ts
const cfg = (window as any).__CROWBAR__ ?? null

export const apiBase  = cfg?.api    ?? ''           // '' = relative URLs (cloud)
export const eventUrl = cfg?.events ?? '/api/events' // SSE endpoint
```

In Tauri, `window.__CROWBAR__` is injected before the app loads:
```js
window.__CROWBAR__ = { api: 'crowbar://api', events: 'crowbar://events' }
```

In cloud, nothing is injected. All calls use relative URLs. The rest of the frontend never references this config directly — it goes through typed hooks built on top of TanStack Query and `apiBase`.

### Events

Real-time updates use SSE (`EventSource`) in both modes:
- **Local**: `EventSource('crowbar://events')` — Tauri protocol handler streams from Go Unix socket
- **Cloud**: `EventSource('/api/events')` — standard HTTP SSE from Go

---

## 3. Tauri Desktop Wrapper (`desktop/`)

### Architecture

Minimal Rust layer. No `ConnectionManager`, no IPC proxy beyond the protocol handler. Tauri is a shell: it launches the Go sidecar, registers the `crowbar://` scheme, and manages the window.

```
desktop/
├── src-tauri/
│   ├── src/
│   │   ├── protocol/           # crowbar:// URI scheme handler
│   │   ├── sidecar/            # Go daemon lifecycle (spawn, health check, teardown)
│   │   └── lib.rs + main.rs
│   ├── binaries/               # crowbar-api-{TARGET_TRIPLE} sidecar executables
│   ├── Cargo.toml
│   └── tauri.conf.json
└── (no src/ — web/ is the frontend)
```

### `crowbar://` Protocol Handler

Registered via `register_asynchronous_uri_scheme_protocol`. Two logical routes:

**API requests** (`crowbar://api/*`):
1. Receive HTTP request from webview
2. Rewrite to `http://localhost/...` and send over Unix socket via `hyperlocal`
3. Return response to webview

**Event stream** (`crowbar://events`):
1. Open connection to Go's SSE endpoint over Unix socket
2. Stream chunks back to webview as they arrive
3. Note: Windows/WebView2 streaming support requires validation; long-polling fallback may be needed on that platform

### Sidecar Management

- At app start: spawn `crowbar-api --host unix://` sidecar, poll `crowbar://api/api/v0/health` until ready (200 OK)
- At app quit: send shutdown signal, wait for clean exit
- Sidecar binaries fetched from GitHub Releases at dev/build time (same `fetch-sidecar` Makefile pattern as Quiver.desktop)
- Binary naming: `crowbar-api-{darwin-arm64,darwin-amd64,linux-amd64,windows-amd64}`

### Initialization Script

`tauri.conf.json` injects before React loads:
```js
window.__CROWBAR__ = { api: 'crowbar://api', events: 'crowbar://events' }
```

### Key Dependencies

- Tauri v2, tauri-plugin-log, tauri-plugin-store, tauri-plugin-shell
- `hyperlocal` (Unix socket HTTP client)
- `tokio` (async runtime)
- Rust edition 2021, toolchain 1.85

---

## 4. CI/CD Pipeline

### Branch Model

Identical to Quiver:

| Branch prefix | Target | Purpose |
|---|---|---|
| `enhancement/*, feature/*, fix/*, refactor/*` | `develop` | Feature work |
| `hotfix/*, beta/*` | `master` | Release candidates |
| `backport/*` | `develop` | Auto-generated backports |

### PR Checks (`ci.yml`)

A `changes` job using `dorny/paths-filter` runs first and outputs which subsystems were touched. Three check jobs run in parallel, each gated on its paths:

```
changes → go-checks      (paths: api/**)
        → frontend-checks (paths: web/**)
        → tauri-checks    (paths: desktop/**)
```

All three are required status checks. A skipped job (no relevant paths changed) counts as passing.

**`go-checks`** (triggered by `api/**`):
- Branch model validation
- `go vet`, `golangci-lint`
- Race-detected tests: `go test -race ./...`
- Coverage ≥ 90% (enforced in Docker with CGO_ENABLED=1)
- Multi-platform build validation (Linux, macOS, Windows)
- Swagger doc generation + diff check

**`frontend-checks`** (triggered by `web/**`):
- Prettier format check
- ESLint
- TypeScript type check (`tsc --noEmit`)
- Vitest coverage ≥ 95%
- Vite build (verify `dist/` is producible)

**`tauri-checks`** (triggered by `desktop/**`):
- `cargo fmt --check`
- `cargo clippy -- -D warnings`
- `cargo audit`
- `cargo tarpaulin` coverage ≥ 95% (excluding Tauri-coupled files)
- Tauri debug build (sidecar placeholder, not full binary)

### Release Pipeline

**Pre-release (`prerelease.yml`)** — triggered by push to `beta/**` or `hotfix/**`:
1. Validate all three subsystems (full CI run, not path-filtered)
2. Cross-compile Go daemon for all platforms/arches
3. Build Tauri app on Linux, macOS, Windows runners
4. Publish GitHub pre-release with:
   - `crowbar-api-{platform}-{arch}` binaries + SHA256 checksums
   - Tauri installers: DMG, AppImage, Deb, MSI, NSIS

**Stable release (`stable-release.yml`)** — triggered by merge to `master` from `beta/*` or `hotfix/*`:
1. Same full build
2. Derive stable tag from branch name
3. Generate changelog
4. Publish GitHub release (non-pre-release)
5. Trigger backport workflow

**Backport (`backport.yml`)** — called by stable release:
1. Create `backport/{date}-{version}` branch from master
2. Open PR against `develop`
3. Auto-merge if no conflicts; post warning otherwise

---

## 5. Deployment

### Local (Tauri)

- Tauri launches `crowbar-api --host unix://` sidecar
- Go listens on `~/.crowbar/crowbar.sock` — no TCP port opened
- Tauri webview loads `web/dist/` (embedded in app bundle)
- `window.__CROWBAR__` injected; frontend uses `crowbar://` scheme
- Data stored in `~/.crowbar/` (SQLite + LanceDB)

### Cloud (`docker/docker-compose.yml`)

Single container. No external database.

```yaml
services:
  crowbar:
    image: ghcr.io/rabbytesoftware/crowbar:latest
    ports:
      - "3737:3737"
    volumes:
      - crowbar-data:/root/.crowbar
    environment:
      - CROWBAR_HOST=tcp://0.0.0.0:3737

volumes:
  crowbar-data:
```

- Go serves API at `/api/v0/` and React SPA at `/`
- SQLite and LanceDB both live in the mounted volume
- No `window.__CROWBAR__` injected; frontend uses relative URLs

---

## Acceptance Criteria — End-to-End Integration

The scaffolding phase is not complete until all three subsystems are connected and working together in **both** deployment modes. Each scenario below must pass before the phase is closed.

### Local (Tauri)

1. `make dev` (or equivalent) fetches the sidecar binary, starts the Go daemon, and launches the Tauri app without errors
2. Tauri spawns `crowbar-api --host unix://` and the protocol handler is live
3. The React SPA loads inside the Tauri webview via `crowbar://`
4. A real API call (`crowbar://api/api/v0/health`) from the frontend reaches the Go daemon and returns a response
5. The SSE event stream (`crowbar://events`) opens and at least one keep-alive or ping event flows to the frontend
6. Quitting the Tauri app cleanly shuts down the Go sidecar

### Cloud (Docker Compose)

1. `docker compose -f docker/docker-compose.yml up` starts the `crowbar` container without errors
2. The Go daemon serves the embedded React SPA at `http://localhost:3737/`
3. The SPA loads in a browser; no broken assets, no console errors on load
4. A real API call (`/api/v0/health`) from the browser reaches the Go daemon and returns a response
5. The SSE event stream (`/api/events`) opens in the browser and at least one keep-alive or ping event is received
6. Data written by the API persists across container restarts (SQLite + LanceDB volume mount)

---

## Key Invariants

1. `web/` has zero imports from `@tauri-apps/api` — enforced by ESLint rule
2. Frontend environment awareness is limited to reading `window.__CROWBAR__` in `transport.ts`
3. The Go daemon is the single source of truth in both deployment modes
4. Local deployment opens no TCP ports
5. All three subsystems must pass their checks before a release can be cut
6. Storage is always SQLite + LanceDB embedded; PostgreSQL is a future adapter swap
