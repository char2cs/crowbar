# Crowbar Backend — Build Waves (subagent orchestration)

> Execution plan for implementing the v0 backend specs (`api/docs/specs/v0/`)
> via parallel subagents. **Excludes the chat send-path and the Agentic Bridge
> spike.**
>
> Each `WAVE-N-*.md` file is a **paste-ready subagent prompt**. Fire them in wave
> order. The **gates** are non-negotiable — never start a wave until the previous
> gate passes, or parallel agents build against a moving target.

---

## ⛔ RABBYTE ENGINEERING STANDARDS — NON-NEGOTIABLE

At Rabbyte we are maniacs about clean-code principles. Every rule below is
mandatory and is **embedded verbatim in every wave prompt**. **Violating ANY of
them means the agent failed its task** — a reviewer will check each one and reject
non-compliant code. No exceptions, no "just this once."

**Structure & files**
1. **Replicate quiver.core's repository structure and philosophy exactly** —
   layered `internal/{engine,adapter,app,api,domain,core}`; implementation details
   hidden in `internal/` sub-packages (`commands/`, `store/`, `projections/`,
   `mocks/`, `strategies/`, …). Study `/Users/char2cs/Projects/Rabbyte/quiver.core`
   and mirror it (target layout below).
2. **One domain concept per file.** A domain file contains exactly ONE entity and
   its directly-related types/structs — nothing else. Never combine concepts.
3. **One test file per source file** (`foo.go` → `foo_test.go`). The ONLY
   exception is struct-only files (pure type/data declarations, no logic).
4. **Source files < 500 lines.** Nearing the limit = abstract/split NOW.

**Function shape**
5. **One parameter per line, ALWAYS** — every function signature AND every
   multi-arg call. Closing paren on its own line. No exceptions, ever.
6. **Early returns ALWAYS.** `else` is a smell — refactor to a guard clause +
   early return. The happy path flows to the bottom.
7. **Max 3 indentation levels per function.** Level 0 = body, 1 = an `if`, 2 = an
   `if` inside a `for`. **Level 3 must NEVER exist** — abstract it out.

**Testing & quality**
8. **Coverage: 95% minimum, 100% is the standard.** Untested code is incomplete.
9. **No flaky tests, anywhere.** Every test passes deterministically every time —
   regardless of order, parallelism, machine, or environment.
10. **NO `time.Sleep` in tests. EVER.** Synchronize with event-driven WebSocket
    watchers + channels, `sync.WaitGroup`, or condition-based wait helpers (poll a
    *condition* with a deadline, never sleep a fixed duration). Mirror
    quiver.core's `tests/kit` watcher pattern (`WaitForState`, `WaitForCount`, …).
11. **Benchmarks for performance-critical algorithms.** Crowbar is an IDE — it
    must be fast. Write `*_bench_test.go` for anything hot (diff parse, search
    match, tree walk, watcher fan-out, hunk-id, conflict parse, …).
12. **CLEAN principles** throughout: guard clauses, composition over deep nesting,
    `fmt.Errorf("op: ctx: %w", err)` wrapping, gofumpt + goimports, every error
    checked.

Enforced by `.golangci.yml` (replicate quiver's): `funlen` (100 lines / 50 stmts),
`gocyclo` (15), `nestif` (≤2), `revive` early-return, `gofumpt` + `goimports`.

---

## Target repository structure (mirror quiver.core)

```
api/
├── cmd/crowbar/                 CLI entry (cobra, signals)
├── internal/
│   ├── internal.go              root container: engine → adapter → app → api
│   ├── core/                    config/ paths/ metadata/ logger/ gateway/
│   ├── domain/                  ONE entity per file: project.go repository.go
│   │                            workspace.go chat.go agent_run.go review_thread.go …
│   ├── engine/                  git/ fs/ terminal/ provider/ lsp/ search/
│   │                            (impl hidden in each engine's internal/ sub-pkgs)
│   ├── adapter/                 eventstore/sqlite/  store/sqlite/
│   ├── app/
│   │   ├── repositories/        one pkg per aggregate; internal/{commands,store,projections,mocks}
│   │   ├── usecases/            (+ mocks/)
│   │   └── hub/                 WebSocketHub + projections
│   └── api/
│       ├── v0/
│       │   ├── dto/
│       │   ├── endpoints/{projects,workspaces,git,files,terminal,…}/handlers/
│       │   └── ws/              Broadcaster[T], StreamDef, dispatch()
│       ├── libs/apierr/
│       └── middleware/
├── tests/
│   ├── kit/                     suite.go env.go client.go typed_client.go
│   │                            ws_client.go repos.go oracle.go helpers.go bench.go
│   └── integration/{concern}/   one package per concern (lifecycle, git, worktree, …)
├── .golangci.yml                funlen 100/50 · gocyclo 15 · nestif 2 · revive early-return
└── Makefile                     test · test-integration (-p 1) · bench · test-coverage
                                 · lint · pr-checks · missing-tests
```

---

## Order & gates

```
WAVE 0  Foundation + scaffold demolition (sequential)        → GATE 0
WAVE 1  Git spine: 04 git → 05 fs/watcher (sequential)       → GATE 1   ★ "first runnable" near here
WAVE 2  Engines (4 parallel agents): 07 · 06 · 11 · 08
WAVE 3  App layer + 09 review + 10 LSP (LSP last/optional)
WAVE 4  API consolidation (02)                               → GATE 4 (backend green)
WAVE 5  Integration suite to quiver parity (tests/kit + tests/integration)
WAVE 6  Tauri + Web + backend: wire all 3, e2e + pressure test (chrome-devtools MCP)
```

| Wave | File | Specs / scope | Parallelism |
|------|------|---------------|-------------|
| 0 | [WAVE-0-foundation.md](./WAVE-0-foundation.md) | 00, 03 framework + demolish old scaffold | 1 agent |
| 1 | [WAVE-1-git-spine.md](./WAVE-1-git-spine.md) | 04, 05 | 1 agent |
| 2 | [WAVE-2-engines.md](./WAVE-2-engines.md) | 07, 06, 11, 08 | **4 parallel** |
| 3 | [WAVE-3-app-layer.md](./WAVE-3-app-layer.md) | app aggregates/usecases, 09, 10 | partial parallel |
| 4 | [WAVE-4-api.md](./WAVE-4-api.md) | 02 | 1 agent |
| 5 | [WAVE-5-integration-suite.md](./WAVE-5-integration-suite.md) | tests/kit + tests/integration | 1 agent |
| 6 | [WAVE-6-app-e2e.md](./WAVE-6-app-e2e.md) | wire web + Tauri + backend; e2e + stress | 1 driver agent |

## Gates

- **GATE 0** — old scaffold demolished; module path `char2cs/*` everywhere;
  containers boot; one Asynx aggregate round-trips; a trivial `Broadcaster[T]`
  pushes to a WS client; `make lint` + `go build ./...` clean.
- **GATE 1** — against a **real git repo**: status / diff / commit / hunk-stage
  work; the watcher fan-out drives a `SyncWorkingTreeState` updating a Workspace
  row; `GitStatus` pushes on a WS topic.
- **GATE 4** — every `02` route resolves; `make pr-checks` green (fmt, vet, lint,
  build, coverage ≥95%, integration, bench).

## Deferred (the "without chat" exclusions)

- **`01` Chat lifecycle** — scaffold the Chat aggregate + AgentRun *shape* only;
  **no send path**.
- **`12` Agentic Bridge** — pending spike, not implemented.

## Definition of done (every wave)

`go build ./...`, `go vet ./...`, and `make lint` clean; **coverage ≥95%** on new
packages (100% the goal); one `_test.go` per source (except struct-only files);
**zero `time.Sleep` in tests**; benchmarks for hot paths; the wave's gate
demonstrated. **All twelve Rabbyte standards hold.**
