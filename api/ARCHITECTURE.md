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
| `cmd/crowbar` | `serve` (the daemon), plus the in-PTY callbacks a vendor CLI runs: `mcp` (stdio MCP server exposing review tools), `handoff dump`. Hooks are not a subcommand: the CLI posts them to the daemon's hook server. `web_embed.go` / `web_noembed.go` select the embedded bundle. |
| `crowbar migrate --plan` | Read-only preview of what the next boot would migrate: layout, pending steps with their actions and counts, the `state/` backup decision, and the backups on disk. It has no apply path; the daemon migrates as it boots. Exits non-zero with the halt reason where the boot would halt. |
| `crowbar paths` | Prints `{"home","socket"}` as JSON: the one answer to "where is the Crowbar home and the daemon socket". The desktop shell and the e2e helper ask it instead of computing either. |
| `cmd/crowbar-seed`, `cmd/crowbar-seed-chat` | Dev-only fixture fillers for a running dev daemon (`make seed`). Never shipped. `seed-chat` takes the home lock and refuses a home that still has pending migration steps: boot the daemon once first. |

## Layers — `internal/`

`internal/internal.go` wires the layers bottom-up and owns the HTTP server:

```
core      home, migrate, bootgate, config, socket/TCP gateway, ipc client, PTY engine, safego …
engine    stateless capabilities over the OS: git, fs, search, lsp, provider, agents, mcp
adapter   persistence: SQLite event logs, snapshot stores, read-model DBs, view.db
app       aggregates (event-sourced), usecases, hub, the one stream and its resources
api       gin router, /v0 REST + WebSocket streams, static bundle
```

Each layer imports only the layers below it. Guards in code:
`engine/architecture_test.go` (engines never cross-import; engines never build
event stores), `app/repositories/architecture_test.go` (chat repositories never
broadcast to clients) and `core/home/architecture_test.go` (only `core/home`
resolves the Crowbar home).

### `core/`
Process-level plumbing with no domain knowledge:

- `home` — the one resolver of the Crowbar home (`~/.crowbar`, or `$CROWBAR_HOME`
  when set) and of every path and the daemon socket derived from it. Only this
  package reads the variable or spells `~/.crowbar`; an AST test fails the build
  otherwise. In a test binary it refuses a home outside `os.TempDir()`, so no
  test can reach the real one.
- `migrate` — the migration engine: the home lock, the `LAYOUT` version, the run
  ledger, the step contract and catalog, the `state/` backup, `Inspect` (the
  read-only preview). It knows no store and no domain; steps bring that.
- `bootgate` — the HTTP gate that answers health and refuses everything else
  until the real API is opened.
- `metadata` (name, version), `paths/worktreepath`, `config`, `gateway` (the
  socket at the address `home` names, or `tcp://` — plaintext on loopback for
  dev; a non-loopback bind terminates TLS and also binds the unix socket so
  `crowbar pair` works on the daemon's own host), `ipc` (the socket
  client the in-PTY callbacks use), `terminal` (the PTY session engine — spawn,
  resize, screen model, reap), `shellenv`/`binpath` (login-shell PATH and CLI
  lookup for a GUI-launched daemon), `safego` (panic-safe goroutines).

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
  selection. It holds no runner aggregate: the table of live CLIs is the chat
  usecase's, in memory, and what outlives a restart is the chat's ledger.
- `mcp` — JSON-RPC MCP protocol used by `crowbar mcp`.

### `adapter/`
Persistence only. Under `<home>/state`:

- `events/<type>.db` — append-only event log per aggregate type, plus
  `events/<type>_snapshots.db` (one upserted snapshot per aggregate, O(1) warm reads).
- `store/<type>.db` — the read-model projection per type.
- `store/chat_ledger.db` — each chat's append-only ledger (`domain/ledger`
  rows by `seq`), not event-sourced: one writer per chat owns its seqs and its
  folded state. Beside it, the open-work heads, the provider-session bindings
  and the dead letters (hooks the ingress could not place).
- `view.db` — plain CRUD tables (projects, repositories, terminal profiles and
  sessions, provider preferences, settings) behind generic GORM stores.

Types: `workspace`, `review_thread`, `agent_chat`, `node` (sidebar placement).
The adapter takes no lock:
the boot's migration session holds the home from before the stores open until
after they close.

`adapter/migrations` is Crowbar's catalog of migration steps — each step's
number, name, phase and offline plan — and the implementation of the steps that
run before any store opens. The post-open implementations live in `app`, beside
the usecases they call.

### `app/`
- **Aggregates** (`repositories/`): each type is one
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
- **`stream/`**: the one `/v0/stream` socket. A client holds scopes (`ws:`,
  `git:`, `files:`, `lsp:`, `doc:`, `forge:` …); each scope kind names its
  daemon work in `stream/resources`, refcounted per key across every client and
  kind: the workspace watcher runs while `git:` or `files:` is held, a host
  document's watch while its `doc:` is, the LSP host for `lsp:` and the forge
  poll only for the viewed workspace's `forge:`. A `chat:<id>` scope is the
  chat's ledger: rows after the sub's `since`, `synced` at the head, then live
  rows and streamed-text deltas, followed outside the hub's lock. A chat that
  is gone or whose ledger cannot be read is refused on its own scope.
- `Shutdown` is ordered: kill PTYs and join their exit callbacks (they write
  runner exits), close the drain gate, wait out reactors, drain each asynx
  singleton — all before the adapter closes the DBs.

### `api/`
- `container.go`: gin with a pre-auth read deadline (TLS requests, lifted by the
  `/v0` bearer guard), logger, timing, recovery, CORS (`middleware/`, origins in
  `origin/`), body limit; mounts `/v0`, debug routes, and the static bundle
  (`static.go`: SPA fallback, precompressed `.gz` siblings, immutable `/assets/*`).
- `v0/router.go`: the route table. Hierarchy
  `/v0/projects/:projectId/repos/:repoId/...` with guards that reject ids from
  another scope; the flat `/v0/chats/:chatId/...` group resolves the chat's
  worktree once per request (`reqscope`) and serves git, files, terminal, lsp,
  search, editor and review for that chat. The chat's ledger is not a route:
  it is the stream's `chat:<id>` scope.
- `v0/endpoints/<group>/`: one package per group (`handlers/` + `routes.go`);
  `v0/dto`: wire types.
- `v0/ws`: `Broadcaster[T]` + `StreamDef` — each stream filters its clients by
  scope and sends a snapshot on subscribe (`v0/snapshots.go`) followed by deltas
  from the hub. `Stream` serves one socket from a channel on the same write
  loop and keepalive.

## Boot and migration

`internal.New` boots the daemon in six steps:

1. `home.ResolveDir` — `home.Resolve()` (or `home.At(dir)` for `WithHomeDir`).
2. Bind the home's socket (`gateway.New(host, h.Socket())`) and start serving
   the `bootgate` at once; `/v0/health` says `migrating`.
3. `migrations.Begin` takes the home lock (`state/.lock`), reads `LAYOUT` and
   the ledger, removes half-written backups and closes a run a crash left open.
   The session is published to the gate, then `RunPreOpen` runs, so health
   names each step as it starts: it plans every pending step of both phases,
   checks git where a plan needs it, backs up `state/` when a plan rewrites it,
   and runs the pre-open steps — step 2, and step 6 `chat-ledger`, so the
   legacy chats are ledger rows before any store opens.
4. Engines, adapter, and `app.New(ctx, engines, adapter, session)`: the session
   is the post-open runner, which `app.New` calls before the runner reconcile
   (a step can hand it a runner a crashed old daemon left) and the owning-chat
   reconcile. Then the API, the `crowbar` binary
   installed into `<home>/bin`, and the hook server, listening and published.
5. `gate.Open(api handler)`: the server, serving since step 2, starts passing
   requests to the API.
6. Close: servers, app and databases first, the session (the lock) last.

**Layout and ledger.** `<home>/LAYOUT` holds the layout version (a decimal
integer; absent means 0, a legacy home). The catalog's `Known` layout is where
its bump steps end; a `LAYOUT` newer than that, or one that is not a
non-negative integer, halts the boot before anything is written. A *bump* step
(`To == From+1`, pre-open only) is pending exactly while `LAYOUT == From`, and
runs Apply, then Verify, then writes `LAYOUT`, then records `step.done`. An in-layout step is
done when the ledger says so. The ledger is `<home>/migrate/ledger.jsonl`:
append-only JSON lines, fsynced per record, mode 0600, outside `state/`, so
restoring `state/` alone does not rewind it: a backup carries the `LAYOUT` and
ledger to restore with it. A boot with nothing pending writes nothing,
not even `migrate/`. It records `run.start`, `backup`, `step.start`,
`checkpoint`, `step.done`, `step.fail`, `run.end`. A torn final line is dropped;
any other unreadable line is `ledger_corrupt`.

**Steps.** `migrate.Step` is the offline half (number, name, phase, from/to,
a read-only `Plan`); `migrate.Impl` is the online half (`Apply`, checkpointed
and resumable, then a real postcondition `Verify`). A post-open step must sit
at the `Known` layout. The five legacy fix-ups the marker files under
`state/upgrades/` used to track are steps 1 to 5 (`project-homes`,
`workspace-paths-table`, `chat-types`, `chat-providers-from-history`,
`hook-deliveries`). An existing marker is adopted once: recorded, deleted, the
step not re-run. Over rows it cannot replay a legacy step stays best-effort: it
counts them as `skipped_rows` and completes; only an engine failure or a failed
Verify halts. Step 6 `chat-ledger` (pre-open, in-layout) restates the legacy
chat records (`agent_runner.db`, `agent_activity.db`, `<home>/chats/*/prompt-requests`)
as ledger rows, checkpointed per chat: a chat whose ledger already has rows is
counted `skipped_chats`, and a CLI the old daemon never saw exit is left live for
the boot reconcile to end.

Step 9 `ledger-layout` bumps `LAYOUT` to 1, the first layout that may lose the
legacy files: a build that knows only layout 0 halts on it (`layout_newer`) instead
of re-creating them empty. Step 6 and the post-open steps sit at layout 1, so step 6
still recurs after the bump. Step 10 `retire-legacy` (pre-open, layout 1, recurring)
deletes the legacy databases (`agent_activity*`, `agent_runner*`, `node*` under
`state/events` and `state/store`), `state/upgrades/`, `chats/*/prompt-requests/` and
the old `<home>/bin/crowbar` copy, only once a run has completed at layout 1, step 6
owes no chat, and each file is byte-identical to its copy in the newest backup; a file
that is not is kept and noted. Step 8 `chat-attachments` moves each chat's
attachments to `<home>/chats/<chat-id>/attachments`, their one home. Step 11
`owner-only` narrows, once, what an older build left with the umask to 0600 files
and 0700 directories, outside users' checkouts. See `docs/plans/2026-11-06-home-tidy.md`.

**Backup.** When a pending plan sets `RewritesState` and `state/` holds files,
the run first writes `<home>/migrate/backups/<run-id>.tar.gz` (built as a
`.partial`, renamed once it reads back as written) after checking free space
for the raw size plus 64 MiB. It holds, under their home-relative names,
`state/`, `LAYOUT`, `migrate/ledger.jsonl` and the paths a plan names in
`Plan.BacksUp` (step 6 names `chats/*/prompt-requests`), all as they stood before
the run: restoring them together rewinds the data and the record of what was done
to it, so the next boot runs the steps again. A retry after a halt keeps the
earlier attempt's backup. A halt's message names the backup in force. At `Begin`,
once the ledger's latest run completed, every backup but the newest is deleted;
`migrate/backups/README.txt` and `crowbar migrate --plan` say how to restore
(quit Crowbar; `rm -rf state && tar -xzf migrate/backups/<run-id>.tar.gz` in the home).

**Halt, not exit.** A failure or refusal (`layout_newer`, `layout_unreadable`,
`ledger_corrupt`, `io_failed`, `plan_failed`, `preflight_failed`,
`step_failed`, `verify_failed`, `no_implementation`) keeps the daemon alive,
holding the lock and the socket, with no store open. `io_failed` is `Begin`
unable to put the home in order (remove a half-written backup, close a dangling
run in the ledger), or unable to take the lock itself (`state/` cannot be made,
the lock file cannot be opened). In the second case the session holds no lock
and runs nothing, so only the socket still refuses a second daemon. A home
directory that cannot be created at all is the one boot error that exits:
there is nothing to hold or report on. The `migrate.Halt` carries the reason,
the step, the layouts, and the ledger and backup paths. A supervisor that
respawned it would only meet the same failure. A cancelled context is the other
exception: the run ends `interrupted`, the process exits, and the next boot
resumes from the checkpoints.

**Health.** `GET /v0/health` answers `{"success":true,"data":{"status",
"version","pid","detail"}}` from the first moment of the boot:

| `status` | Meaning | `detail` |
| --- | --- | --- |
| `migrating` | Preparing the home. | Progress (`phase`, `step`, `index`, `total`) once a step runs. |
| `halted` | The boot stopped. | The `Halt`; the reason in `detail.message`. |
| `ok` | The API is open. | None. |

While the gate is closed every other route answers 503 `daemon_migrating` or
`daemon_halted`. The gate speaks through the API's own CORS middleware. The
desktop holds a `migrating` or `halted` daemon instead of killing it (a gate
refusing routes is not a wedge) and replaces its window with a halt screen
carrying `detail.message`.

**One socket rule.** `home.Home.Socket()` is the only place a socket path is
decided: `<home>/crowbar.sock` for the default home; for any non-default home
(`CROWBAR_HOME`, `WithHomeDir`), `$TMPDIR/crowbar-<fnv1a64 of the cleaned
home>.sock` (macOS caps `sun_path` at 104 bytes). The daemon binds the socket
of the home it holds. The desktop shell and the e2e helper run `crowbar paths`
and use the socket and daemon log path (`home.Home.DaemonLog()`) it prints; the
daemon hands its bound socket to the callbacks it spawns instead of letting
them derive one.

## Request flows

- **UI action** → REST handler → usecase → aggregate command → event appended →
  projection updates the read model → hub → WebSocket broadcaster → clients.
- **Agent turn**: the chat usecase spawns the vendor CLI in a PTY (`core/terminal`)
  from the descriptor's spawn plan. The CLI posts each hook (natively or through
  curl) to the daemon's loopback hook server at
  `.../chats/runners/:segid/hooks/:provider/:event`, authenticated by the runner
  token. This HTTP ingress is the only hook path (the old socket relay is
  retired): it resolves the runner in the live table, parses the payload through
  the runner's descriptor and appends ledger rows under the chat's lock,
  answering once they commit; a hook it cannot place is dead-lettered. A hook
  that opens an answerable ask is held open, and the human's answer is its
  response. Each committed row reaches the chat's `chat:<id>` stream scope by
  `seq`; the chat's snapshot and its client broadcasts follow from
  the same commit.
- **Watchers**: holding a workspace's `git:` or `files:` scope starts its
  watcher; changes flow back as `git_status` and `file_change` signals. A host
  document (a file outside every worktree) is a `doc:<absolute path>` scope,
  admitted only over the local socket (`host_document_requires_local_transport`)
  and only in its canonical spelling. Its watch is armed before the scope's
  `synced`, so a read after it misses no change. The watch covers that one file:
  FSEvents rooted at the file, or inotify on its parent directory (re-armed on
  the nearest existing ancestor while the directory is gone). On Linux each held
  path costs an inotify instance, capped per user, so an arm the kernel refuses
  is refused as `watch_limit` and the tab says the system ran out of file
  watches. A refused watch is surfaced, never retried silently: the document
  opens nowhere and its tab says why; tabs of one path share one armed watch.

## Tests

- Unit tests sit next to the code (`foo_test.go`), with hand-written fakes in
  `internal/mocks` packages.
- `tests/` is the black-box suite (build tag `integration`) over a real daemon,
  HTTP, WebSocket and SQLite, via `tests/kit`; `tests/integration/<concern>/`
  groups the heavier scenarios (crash, concurrency, bench).
- The fake provider harness (`tests/kit/fakeprovider.go`, `tests/kit/fakecli`)
  installs a provider descriptor into the test home whose CLI records its launch
  and behaves like `cat`; a test plays the provider by posting hooks to the
  hook server itself. No real provider CLI runs in a test.
