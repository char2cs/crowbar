# WAVE 3 — App layer + Branch Review + LSP

After the engines (Wave 2) land, build the **application layer** that ties them
to the domain aggregates and the hub, then Branch Review, then LSP (last /
optional). The app-layer work and `09` can run partly in parallel; do `10` LSP
last.

Shared house rules:
- Module `github.com/char2cs/crowbar/api`. Go 1.26.2. **Invoke `go-style` first.**
- Layered `engine → adapter → app → api`; lower never imports higher.
- Reference `quiver.core` `internal/app/` (repositories, usecases, hub
  projections) + `api/ARCHITECTURE.md`.

---

## Agent 3A — App-layer aggregates, usecases, hub projections (the integration core)

**Read:** `00` (domain + DI), `01` §2–§8 (Chat aggregate — lifecycle ONLY, no
send path), `03` §7 (`RegisterHubProjections`), `08` §5 (`SyncProviderState`),
and each engine's public API.
**Build:**
- **Asynx command sets** for the aggregates: Workspace (`SyncWorkingTreeState`,
  `SyncProviderState`, `SetMergeStrategy`, create/hierarchy/merge/reparent,
  `TouchActivity`), Chat (`CreateChat`, `ForkChat` from-tip, `RenameChat`,
  `DeleteChat` cascade, `AgentRunStarted/Completed` projection), ReviewThread
  (`OpenThread`, `ReplyThread`, `ResolveThread`, `ReopenThread`).
- **Repositories** (GORM: Project, Repository, TerminalProfile; Asynx-backed read
  models for the aggregates) + the **project-import usecase** (`00` §5.7: repo
  discovery, avatar gen, `defaultBranch`, adopt existing worktrees with
  merge-base-seeded `forkPointSha`) + the **`Project.lastActivity` best-effort
  GORM roll-up**.
- **`RegisterHubProjections(hub)`** — wire every Asynx subscription to the right
  `hub.BroadcastX`; AgentRun sub drives Chat status + Workspace `agent-running`
  overlay; finalize the AgentRun crash-recovery two-pass.
- **Usecases** for projects/repos/workspaces/chats(lifecycle)/files/git/terminal,
  composing the engines.
> Scaffold the AgentRun *shape* and Chat aggregate, but **do not** build the
> message-send/run path (deferred — `12`).

---

## Agent 3B — Branch Review (`09`)

**Read:** `api/docs/specs/v0/09-branch-review.md`, `04` §3 (diff), `08` §5.
**Owns:** the `ReviewThread` aggregate wiring (with 3A) + the `BranchReview`
composite read-model usecase + the review/thread REST handlers.
**Build:** `GET …/review` assembles description (placeholder — AI gen is
bridge-deferred) + `mergeStrategy` (from the Workspace aggregate) + the branch
diff (`git diff <base>...<branch>`, base = parent branch or repo `defaultBranch`)
+ threads + `BranchChat[]` (Chat projection). Thread CRUD via the aggregate;
re-fetch-on-mutation (no dedicated broadcaster). `PATCH …/review` →
`SetMergeStrategy`. **No PR-create endpoint.**

---

## Agent 3C — LSP host (`10`) — DO LAST / OPTIONAL

**Read:** `api/docs/specs/v0/10-lsp.md`.
**Owns:** `engine/lsp/` (`registry/`, `server/`, `manager/`, `convert/`) + the
LSP REST handlers + the diagnostics WS topic.
**Build:** LSP host/proxy — spawn language servers (ship `gopls` +
`typescript-language-server` first; `pyright`/`jdtls`/`clangd` config-extensible),
JSON-RPC over stdio; **frontend-driven document sync** (track open URIs, not
content; **replay `didOpen` on server respawn**); REST request/response for
completion/hover/def/refs/rename/codeAction/documentSymbol; diagnostics push over
WS; lazy per-workspace lifecycle with an **independent refcount** (decoupled from
the file watcher); graceful absence when no server installed.
> If time-boxing v0, this whole agent can be deferred — the editor works without
> it. Ship it after the rest is green.

## Done (all of Wave 3)
`go build ./...` + `go vet ./...` clean; integration tests show the hub
projections firing end-to-end (a git mutation updates a Workspace row over WS; a
thread post updates the review read-model; project import creates repos +
adopts worktrees).
