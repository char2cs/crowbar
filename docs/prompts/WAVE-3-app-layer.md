# WAVE 3 — App layer + Worktree Hierarchy + Branch Review + LSP

After the engines (Wave 2) land, build the **application layer** that ties them
to the domain aggregates and the hub: the aggregate command sets + usecases +
hub projections (3A), the worktree hierarchy (3D, the signature feature), Branch
Review (3B), then LSP (3C, last / optional).

**Sub-ordering:** **3A first** — it lands the Workspace aggregate command set;
**3D (hierarchy) and the provider-poll wiring depend on it**. 3B can run alongside
3A once the ReviewThread commands exist; 3C (LSP) is independent and goes last.

## ⛔ Rabbyte standards — NON-NEGOTIABLE (violating ANY = task failure)

A reviewer checks each one.
1. **Replicate quiver.core's structure** — `internal/app/repositories/<aggregate>/`
   with `internal/{commands,store,projections,mocks}`; mirror it exactly.
2. **One domain concept per file**; **one `_test.go` per source file** (except
   struct-only files); **source files < 500 LOC** (split before the limit).
3. **One parameter per line, ALWAYS** — signatures AND multi-arg calls; closing
   paren on its own line.
4. **Early returns ALWAYS** — `else` is a smell. **Max 3 indentation levels per
   function** — level 3 must NEVER exist; abstract instead.
5. **Coverage ≥95%** (100% is the standard). **No flaky tests.** **NO `time.Sleep`
   in tests, EVER** — event-driven WS watchers / condition-based wait helpers
   (mirror quiver `tests/kit`).
6. **Benchmarks (`*_bench_test.go`) for hot paths** (e.g. projection rebuilds,
   aggregate reconstruction) — Crowbar is an IDE, be fast.
7. **CLEAN**: guard clauses, composition, `fmt.Errorf("op: ctx: %w", err)`,
   gofumpt + goimports. Enforced by `.golangci.yml` (funlen 100/50, gocyclo 15,
   nestif ≤2, revive early-return). Full statement: `docs/prompts/README.md`.

**Project basics:** module `github.com/char2cs/crowbar/api`; Go 1.26.2; **invoke
`go-style` before writing Go**; layered `engine → adapter → app → api` (lower never
imports higher); reference `quiver.core` `internal/app/` (repositories, usecases,
hub projections) + `api/ARCHITECTURE.md`.

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
  composing the engines. (Worktree-hierarchy usecases are Agent 3D.)
- **Wire the provider poll** (Wave-2 engine) → `SyncProviderState` command on the
  Workspace aggregate (`08` §5).
> Scaffold the AgentRun *shape* and Chat aggregate, but **do not** build the
> message-send/run path (deferred — `12`).
> **3A lands the Workspace aggregate command set first** — 3D and the provider
> wiring depend on it.

---

## Agent 3D — Worktree Hierarchy (`07`) ★ signature feature  (depends on 3A's Workspace commands)

**Read:** `api/docs/specs/v0/07-workspace-worktree-hierarchy.md`, `00` §5.3/§6.1,
`04` §5/§6.1 (the Wave-1 git **primitives** you call: worktree add/remove,
rebase-onto, ff-only).
**Owns:** the Workspace-hierarchy **usecases** in `app/usecases/` — orchestrating
the Wave-1 git primitives + 3A's Workspace Asynx commands (do not fork either).
**Build:** worktree-backed workspace create (`git worktree add`, record
`forkPointSha`); local merge — all **three** strategies (merge/squash append-only
in parent; **rebase = rebase child onto parent then `git merge --ff-only`**;
update kept-child `forkPointSha` for every strategy); re-parent
(`git rebase --onto <newParentTip> <forkPointSha>`, leaf-guard → `has_children`);
cascade delete (`git worktree remove --force` then `git branch -D`, skip locked);
the `pendingMerge` marker + cross-worktree conflict resume; locked = chat-only.
**Verify:** real repo — child create → commit → local merge (each strategy) →
parent advances correctly; re-parent a leaf; re-parent with children is rejected;
delete cascade skips a locked child. **This is the one to get exactly right.**

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
