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

## House rules
- Module `github.com/char2cs/crowbar/api`; fix any `rabbytesoftware/*` imports to
  `char2cs/*`. Go 1.26.2. **Invoke the `go-style` skill before writing Go.**
- Layered: `engine → adapter → app → api`; lower never imports higher.
- This is a rewrite — delete/replace the scaffold's `Task`/`KanbanItem`/`Flow`
  domain and the fixture-backed handlers where they don't match the specs.

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
- `go build ./...` and `go vet ./...` clean.
- Containers boot from `cmd/crowbar`.
- A unit/integration test shows **one Asynx aggregate round-tripping** (send a
  command → event persisted → reload reconstructs state).
- A test shows a **trivial `Broadcaster[T]` pushing a message to a connected WS
  client** (use the quiver.core pattern).
- `GET /v0/health` returns OK.

Report what you built, the test command to reproduce the gate, and anything in
`00`/`03` that was ambiguous.
