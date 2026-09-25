# Crowbar daemon — architecture

Module `github.com/char2cs/crowbar/api`. A local, single-user daemon: it owns
projects, git repositories and worktrees, PTY terminals, and the agent CLIs
(Claude Code, Codex) running inside them, and serves the React app in `web/`
over REST + WebSocket. The Tauri shell (`desktop/`) and a browser are both just
clients.

Build and test with `-tags noEmbed` (the default build embeds `web/dist`, see
`make embed-web`).

## Binaries — `cmd/`

| Binary | Role |
| --- | --- |
| `cmd/crowbar` | `serve` (the daemon), plus the in-PTY callbacks a vendor CLI runs: `hook <event>` (posts a hook envelope to the daemon over the unix socket and, for an answerable permission prompt, stays alive until a human decides), `mcp` (stdio MCP server exposing review tools), `handoff dump`. `web_embed.go` / `web_noembed.go` select the embedded bundle. |
| `cmd/crowbar-seed`, `cmd/crowbar-seed-chat` | Dev-only fixture fillers for a running dev daemon (`make seed`). Never shipped. |

## Layers — `internal/`

`internal/internal.go` wires the layers bottom-up and owns the HTTP server:

```
core      config, paths, socket/TCP gateway, ipc client, PTY engine, safego …
engine    stateless capabilities over the OS: git, fs, search, lsp, provider, agents, mcp
adapter   persistence: SQLite event logs, snapshot stores, read-model DBs, view.db
app       aggregates (event-sourced), usecases, hub, realtime lifecycles
api       gin router, /v0 REST + WebSocket streams, static bundle
```

Each layer imports only the layers below it. Guards in code:
`engine/architecture_test.go` (engines never cross-import; engines never build
event stores) and `app/repositories/architecture_test.go` (chat repositories never
broadcast to clients).

### `core/`
Process-level plumbing with no domain knowledge: `metadata` (home dir,
`~/.crowbar` or `$CROWBAR_HOME`), `paths`, `config`, `gateway` (unix socket at
`~/.crowbar/crowbar.sock`, or `tcp://` for dev), `ipc` (the socket client the
in-PTY callbacks use), `terminal` (the PTY session engine — spawn, resize,
screen model, reap), `shellenv`/`binpath` (login-shell PATH and CLI lookup for a
GUI-launched daemon), `selfinstall` (copies the binary to `<home>/bin` so vendor
hooks can call `crowbar hook` by absolute path), `safego` (panic-safe goroutines).

### `engine/`
Each engine is a facade over one capability, with its implementation in
`internal/` sub-packages. `engine.Container` holds them all:

- `git` — git CLI wrapper: status, diff, log, branches, stash, worktrees, conflicts, identity.
- `fs` — file reads/writes and the fsnotify watcher.
- `search` — workspace text search.
- `lsp` — language-server host (graceful absence: no server → empty results).
- `provider` — GitHub/GitLab detection and PR-status polling.
- `agents` — everything vendor-specific about agent CLIs: YAML descriptors
  (`internal/protocol/internal/descriptor/descriptors-v3/{claude,codex}.yaml`)
  mapped to spawn plans, hook/telemetry interpretation, model discovery and
  selection. `agents/runner` is the RUNNER aggregate: one live vendor CLI in one PTY.
- `mcp` — JSON-RPC MCP protocol used by `crowbar mcp`.

### `adapter/`
Persistence only. Under `<home>/state`:

- `events/<type>.db` — append-only event log per aggregate type, plus
  `events/<type>_snapshots.db` (one upserted snapshot per aggregate, O(1) warm reads).
- `store/<type>.db` — the read-model projection per type.
- `view.db` — plain CRUD tables (projects, repositories, terminal profiles and
  sessions, provider preferences, settings) behind generic GORM stores.

Types: `workspace`, `review_thread`, `agent_chat`, `agent_activity` (a chat's
turns and tool calls — separate so a tool-call storm never queues behind sidebar
writes), `agent_runner`, `node` (sidebar placement). The adapter also takes the
single-instance lock on the home dir.

### `app/`
- **Aggregates** (`repositories/`, `engine/agents/runner`): each type is one
  `github.com/char2cs/asynx` singleton routing many ids by shard hash. Commands
  append events; projections fold them into the read model; post-commit reactors
  run side effects behind `repositories/drain`'s shutdown gate.
- **Usecases** (`usecases/`): the operations the API calls — `chat` (agent
  chats: create, send, stop, answer permission prompts, attachments, runner
  moves on /clear and /resume, and the client fan-out), `workspace`, `worktree`
  (chat → worktree resolution, locking), `project` (import/delete), `git`,
  `file`, `terminal`, `branchreview`, `provider`.
- **`tree/`**: a sibling-ordered forest planner used by sidebar and chat-panel moves.
- **`hub/`**: fans domain broadcasts out to WebSocket subscribers.
- **`realtime/`**: refcounted lazy lifecycles — the file/git watcher starts on
  the first Files∪Git subscriber of a workspace and stops on the last; the LSP
  host and provider PR polling likewise.
- `Shutdown` is ordered: kill PTYs and join their exit callbacks (they write
  runner exits), close the drain gate, wait out reactors, drain each asynx
  singleton — all before the adapter closes the DBs.

### `api/`
- `container.go`: gin with logger, timing, recovery, CORS (`middleware/`, origins
  in `origin/`), body limit; mounts `/v0`, debug routes, and the static bundle
  (`static.go`: SPA fallback, precompressed `.gz` siblings, immutable `/assets/*`).
- `v0/router.go`: the route table. Hierarchy
  `/v0/projects/:projectId/repos/:repoId/...` with guards that reject ids from
  another scope; the flat `/v0/chats/:chatId/...` group resolves the chat's
  worktree once per request (`reqscope`) and serves git, files, terminal, lsp,
  search, editor and review for that chat.
- `v0/endpoints/<group>/`: one package per group (`handlers/` + `routes.go`);
  `v0/dto`: wire types.
- `v0/ws`: `Broadcaster[T]` + `StreamDef` — each stream filters its clients by
  scope and sends a snapshot on subscribe (`v0/snapshots.go`) followed by deltas
  from the hub.

## Request flows

- **UI action** → REST handler → usecase → aggregate command → event appended →
  projection updates the read model → hub → WebSocket broadcaster → clients.
- **Agent turn**: the chat usecase spawns the vendor CLI in a PTY (`core/terminal`)
  from the descriptor's spawn plan. The CLI's hooks run `crowbar hook`, which
  posts to the daemon over the unix socket; the agents engine interprets the
  envelope and the chat usecase records activity and fans it out.
- **Watchers**: subscribing to a workspace's files/git stream starts its watcher;
  changes flow back as git-status/file frames.

## Tests

- Unit tests sit next to the code (`foo_test.go`), with hand-written fakes in
  `internal/mocks` packages.
- `tests/` is the black-box suite (build tag `integration`) over a real daemon,
  HTTP, WebSocket and SQLite, via `tests/kit`; `tests/integration/<concern>/`
  groups the heavier scenarios (crash, concurrency, bench).
