# WAVE 0 — Foundation (single agent, sequential)

You are implementing the **foundation** of the Crowbar Go backend — a complete
rewrite to match the design specs. Nothing else can be built until this is solid,
so do **not** rush past the gate.

## Read first
- `api/docs/specs/v0/00-architecture-and-domain.md` (your primary spec)
- `api/docs/specs/v0/03-realtime-websockets.md` §1–§2, §6–§7 (the hub /
  `Broadcaster[T]` framework — NOT the seven concrete broadcasters yet)
- `api/ARCHITECTURE.md`
- Reference implementation to mirror: `/Users/char2cs/Projects/Rabbyte/quiver.core`
  — read `internal/internal.go`, `internal/{engine,adapter,app,api}/container.go`,
  `internal/app/container.go` (Asynx wiring), `internal/api/ws/broadcaster.go`.

## ⛔ Rabbyte standards — NON-NEGOTIABLE (violating ANY = task failure)

These apply to every line you write. A reviewer checks each one.
1. **Replicate quiver.core's structure** (`/Users/char2cs/Projects/Rabbyte/quiver.core`):
   layered `internal/{engine,adapter,app,api,domain,core}`; implementation hidden in
   `internal/` sub-packages (`commands/`, `store/`, `projections/`, `mocks/`).
2. **One domain concept per file**; **one `_test.go` per source file** (except
   struct-only files); **source files < 500 LOC** (split before the limit).
3. **One parameter per line, ALWAYS** — signatures AND multi-arg calls; closing
   paren on its own line.
4. **Early returns ALWAYS** — `else` is a smell. **Max 3 indentation levels per
   function** — level 3 must NEVER exist; abstract instead.
5. **Coverage ≥95%** (100% is the standard). **No flaky tests.** **NO `time.Sleep`
   in tests, EVER** — synchronize with event-driven WS watchers / condition-based
   wait helpers (mirror quiver `tests/kit`: `WaitForState`, `WaitForCount`, …).
6. **Benchmarks (`*_bench_test.go`) for performance-critical algorithms** — Crowbar
   is an IDE, it must be fast.
7. **CLEAN**: guard clauses, composition over nesting, `fmt.Errorf("op: ctx: %w",
   err)`, gofumpt + goimports. Enforced by `.golangci.yml` (funlen 100/50, gocyclo
   15, nestif ≤2, revive early-return). Full statement: `docs/prompts/README.md`.

**Project basics:** module `github.com/char2cs/crowbar/api` (fix any
`rabbytesoftware/*` → `char2cs/*`); Go 1.26.2; **invoke the `go-style` skill before
writing Go**; layered `engine → adapter → app → api` (lower never imports higher);
this is a **rewrite** to match `api/docs/specs/v0/`.

## Step 0 — Demolish the old scaffold (do this first)
Delete/replace what does not match the specs: the `Task`/`KanbanItem`/`Flow`
domain, `engine/flow/`, `engine/agent/`, the MCP repository, the fixture-backed
handlers + `internal/fixtures/`, and the SSE `events.go`. Fix **all** imports
`rabbytesoftware/*` → `char2cs/crowbar/api/*`. Stand up `.golangci.yml` (mirror
quiver's) and the `Makefile` targets (`test`, `test-integration`, `bench`,
`test-coverage`, `lint`, `pr-checks`, `missing-tests`) **before** writing new code,
so the standards are enforced from line one.

## Build
1. **`core/`** — `config/` (loads `~/.crowbar/config.yaml` over embedded defaults;
   intelligence-tier → model map), `paths/` (`Events()`, `Store()`, `Runs()`,
   `Logs()`, lazy mkdir, `WithHomeDir(dir)` for test isolation), `metadata/`.
2. **`domain/`** — every type from `00` §5: `Project`, `Repository` (incl.
   `defaultBranch`), `Workspace` (all fields incl. `projectId`, `forkPointSha`,
   `mergeStrategy`, `pendingMerge`, PR fields), `Chat`, `AgentRun`, `ReviewThread`,
   `TerminalProfile`, and the status/enum types. State machines per `00` §6.
3. **`adapter/`** — SQLite event stores (one file per Asynx aggregate under
   `~/.crowbar/state/events/*.db`, 8 shards / queue depth 1000) + the GORM
   connection (`~/.crowbar/state/store/crowbar.db`).
4. **`app/`** — construct the Asynx instances (Workspace, Chat, AgentRun,
   ReviewThread) from the adapter stores; the `hub` with the typed `WebSocketHub`
   interface (`BroadcastWorkspace`, `BroadcastChat`, …) and subscriber fan-out;
   a stub `RegisterHubProjections(hub)` (wired fully in Wave 3). AgentRun crash
   recovery scaffold (`00` §6.2) — running→error, idempotent chat reconcile.
5. **`api/`** — the generic **`Broadcaster[T]`** with `StreamDef[T]`
   (`Namespace`, `Serialize`, `Filters`), the `dispatch()` REST/WS helper, the
   snapshot-on-subscribe contract hook (`03` §1a), middleware, and a `/v0/health`
   route. Do **not** implement the seven concrete topics yet.
6. **`internal.go`** — wire the four containers in order
   (engine → adapter → app → api), `WithHomeDir` plumbed through for tests.
7. **`go.mod`** — add `char2cs/asynx`, `gorilla/websocket`, `creack/pty`,
   `stretchr/testify`. (Skip `acp-go-sdk` and `mcp-go` — bridge/chat, deferred.)

## Out of scope
The concrete engines (git/fs/terminal/…), the concrete broadcasters, and any
real subsystem logic. Just the skeleton + contracts.

## GATE 0 — Definition of done (must demonstrate)
- `go build ./...`, `go vet ./...`, and `make lint` clean; **≥95% coverage** on
  new packages; one `_test.go` per source file; **zero `time.Sleep` in tests**.
- Old scaffold demolished; no `rabbytesoftware/*` imports remain.
- Containers boot from `cmd/crowbar`.
- A unit/integration test shows **one Asynx aggregate round-tripping** (send a
  command → event persisted → reload reconstructs state).
- A test shows a **trivial `Broadcaster[T]` pushing a message to a connected WS
  client** (use the quiver.core pattern).
- `GET /v0/health` returns OK.

Report what you built, the test command to reproduce the gate, and anything in
`00`/`03` that was ambiguous.
