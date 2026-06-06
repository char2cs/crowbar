# Wave 4 — API Consolidation: Execution Plan (subagent-driven)

> Controller artifact. Branch `sc-quantum-meissner-9e5b` (fast-forwarded to Wave 3 / 3dfbd45).
> Target structure mirrors quiver.core `internal/api/v0/` exactly (standard §1).

## Decisions (locked)
- **Envelope:** all routes return `{success, error, data?}` via `internal/api/libs` helpers
  (`WriteQueryOK`, `WriteMutationOK`, `WriteErr`, `WriteQueryWithStatus`). Mutations carry the
  affected id in `data` (e.g. `{id}`); queries carry `data`.
- **Prefix:** mount at `/v0/*` (no `/api`). Frontend `apiFetch` updated to unwrap `.data` and call `/v0`.
- **Migration:** FULL — existing flat handlers (lsp/review/provider/search/terminal/health) move
  into `dto/` + `endpoints/<x>/handlers/` + per-endpoint `routes.go`. New router.go ties it together.

## Target tree
```
internal/api/libs/response.go            # envelope helpers (+ apierr status mapping)
internal/api/v0/
  container.go                           # holds usecases + ws handler; New(); Register(rg)
  router.go                              # Register calls each endpoints/<x>.Register(rg, svc, wsHandle)
  dto/                                   # one DTO per file (request/response shapes)
  endpoints/<x>/routes.go + handlers/    # projects repos workspaces chats files editor git review provider search terminal health
  ws/                                    # broadcaster.go, dispatch.go, stream_def.go, filter.go, client.go (exist) + handler.go (7 topics) + snapshot wiring
```

## Backing usecases (Wave 3, all present on `app.Container.Usecases` / engines)
Project, ProjectImport, Workspace, Chat, File, Git, Terminal, ProviderSync, Worktree, BranchReview;
engines: Git, FS, Provider, Search, Terminal, LSP. GORM stores: Projects, Repositories, TerminalProfiles.

## Route inventory (02 §2 + §3) — target 57 REST + 7 WS

## Task DAG
- **T1 libs envelope** (blocks all) — `internal/api/libs/response.go` + apierr status mapper + tests.
- **T2 container/router skeleton + endpoint wiring contract** — defines new `v0.Container` (usecases + ws handler), `router.go` with `dispatch()` helper, empty `Register` calling endpoint Registers as they land. health endpoint migrated here as the pattern-proving first endpoint.
- **Endpoint tasks (each: dto + routes.go + handlers + tests, depend on T1+T2):**
  - T3 projects (list/create/get) + repos (list/get, protected-branches)
  - T4 workspaces + hierarchy (list **dual**, get, create, delete, merge-into-parent, reparent)
  - T5 chats (list/create/fork/rename/delete)
  - T6 files (tree/content get/content put/create/rename/delete)
  - T7 editor (blame + migrate 8 LSP POST + diagnostics GET)
  - T8 git read (status **dual**, log, diff, branches, stashes)
  - T9 git write (stage/unstage/discard/commit/push/pull/fetch/branches CRUD/checkout/stash/reset/merge/rebase)
  - T10 git conflicts + operation (conflicts list/resolve, operation continue/abort)
  - T11 review (migrate: get/strategy/threads/reply/resolve)
  - T12 provider (migrate: provider, protected-branches) — fold repo route ownership w/ T3 carefully
  - T13 search (migrate: search/replace)
  - T14 terminal (migrate: profiles CRUD, session create/kill, terminal WS)
- **T15 broadcasters (7 topics):** ws/handler.go with Workspaces(global+filters), Chats(wsId), Git(wsId — fix namespace), Files(wsId, no snapshot), LSP(wsId), Terminal(sessionId, PTYFrame), ChatStream(chatId placeholder). Correct StreamDef scoping.
- **T16 snapshot-on-subscribe:** Workspaces (compute agent-running/hasConflicts overlays at snapshot under reg lock), Chats, Git, LSP snapshots; Terminal ring replay; Files none. Dual-serve REST body = snapshot.
- **T17 lazy lifecycles:** FileWatcher refcount = subscribers(Files∪Git) per wsId; LSP refcount = subscribers(LSP) per wsId; first→start, last→stop; independent refcounts.
- **T18 final wiring:** mount in `internal.go` / `api` container; dual-serve dispatch on `GET /v0/workspaces` + `GET /v0/workspaces/:wsId/git/status`; RegisterHubProjections confirmed.
- **T19 frontend adaptation:** `web/src/lib/api.ts` apiFetch unwrap `.data`/`success`; `/api/v0`→`/v0` across web; fix tests.
- **T20 integration suite + benchmarks:** `tests/` integration (event-driven WS waiters, NO time.Sleep); benchmarks for broadcaster fan-out + snapshot serialization.
- **T21 final review + e2e:** build/vet/test/coverage; build web/dist; e2e walkthrough per DoD.

## Standards every task must satisfy
One concept/file; one `_test.go` per source; <500 LOC/file; one param per line; early returns, ≤3 indent;
coverage ≥95%; no `time.Sleep` in tests; `fmt.Errorf("op: ctx: %w", err)`; gofumpt+goimports; `.golangci.yml` clean.
Invoke `go-style` before writing Go.
