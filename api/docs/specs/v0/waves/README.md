# Crowbar Backend — Build Waves (subagent orchestration)

> Execution plan for implementing the v0 backend specs (`api/docs/specs/v0/`)
> via parallel subagents. **Excludes chat send-path and the Agentic Bridge spike.**
>
> Each `WAVE-N-*.md` file is a **paste-ready subagent prompt**. Fire them in
> wave order. The two **gates** are non-negotiable — do not start a wave until the
> previous gate passes, or parallel agents build against a moving target.

## Order & gates

```
WAVE 0  Foundation (sequential)            → GATE 0
WAVE 1  Git spine: 04 git → 05 fs/watcher  → GATE 1   ★ "first runnable" near here
WAVE 2  Engines (4 parallel agents): 07 hierarchy · 06 terminal · 11 search · 08 provider
WAVE 3  App layer + 09 review + 10 LSP (LSP last/optional)
WAVE 4  API consolidation (02)
```

| Wave | File | Specs | Parallelism |
|------|------|-------|-------------|
| 0 | [WAVE-0-foundation.md](./WAVE-0-foundation.md) | 00, 03 (framework) | 1 agent, sequential |
| 1 | [WAVE-1-git-spine.md](./WAVE-1-git-spine.md) | 04, 05 | 1 agent (04 then 05) |
| 2 | [WAVE-2-engines.md](./WAVE-2-engines.md) | 07, 06, 11, 08 | **4 agents in parallel** |
| 3 | [WAVE-3-app-layer.md](./WAVE-3-app-layer.md) | app aggregates/usecases, 09, 10 | partial parallel |
| 4 | [WAVE-4-api.md](./WAVE-4-api.md) | 02 | 1 agent |

## Gates (must pass before the next wave)

- **GATE 0** — containers boot; one Asynx aggregate round-trips (command → event →
  reload); a trivial `Broadcaster[T]` pushes to a connected WS client;
  `go build ./...` + `go vet ./...` clean.
- **GATE 1** — against a **real git repo**: status / diff / commit / hunk-stage
  work; the file watcher fires and its fan-out drives a `SyncWorkingTreeState`
  command that updates a Workspace row; `GitStatus` pushes on a WS topic.

## Deferred (the "without chat" exclusions)

- **`01` Chat lifecycle** — scaffold the Chat aggregate + AgentRun *shape* only
  (other code references them); **no send path**.
- **`12` Agentic Bridge** — pending spike, not implemented.

## House rules (every agent must follow — repeated in each prompt)

- Module path `github.com/char2cs/crowbar/api`. Fix any `rabbytesoftware/*`
  imports to `char2cs/*`. Go 1.26.2.
- **Invoke the `go-style` skill before writing Go.**
- Layered architecture: `engine → adapter → app → api`; a lower layer never
  imports a higher one.
- Reference patterns: `/Users/char2cs/Projects/Rabbyte/quiver.core` (containers,
  `Broadcaster[T]`, `dispatch()`, Asynx wiring) and `api/ARCHITECTURE.md`.
- This is a **rewrite** to match `api/docs/specs/v0/`. Discard the existing
  `Task` / `KanbanItem` / `Flow` domain and fixture-backed handlers where they
  don't match the specs.
- Tests: Go `_test.go` co-located; integration tests under `api/tests/` with
  `//go:build integration`, run `go test -tags integration -race ./tests/...`.
- Definition of done for every wave: `go build ./...` and `go vet ./...` clean,
  new unit tests pass, and the wave's gate (if any) is demonstrated.
