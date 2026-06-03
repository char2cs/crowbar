# WAVE 1 — Git spine (single agent: 04 git → then 05 fs/watcher)

You are implementing the **spine** of the backend — the git engine and the
filesystem/watcher engine. These are the most depended-on and the riskiest, so
build and **verify against a real git repo** before declaring done. Do `04`
fully, then `05` (it depends on git for status decorations).

**Prerequisite:** Wave 0 (foundation) has passed GATE 0 — domain types, Asynx,
hub, and `Broadcaster[T]` exist.

## Read first
- `api/docs/specs/v0/04-git-subsystem.md` (primary)
- `api/docs/specs/v0/05-filesystem-and-watcher.md` (primary)
- `api/docs/specs/v0/03-realtime-websockets.md` §2, §5, §6 (producer classes,
  the one-fs-event fan-out, lazy ref-counted lifecycle)
- `api/docs/specs/v0/00-architecture-and-domain.md` §5.3, §6.1 (the
  `SyncWorkingTreeState` command + its per-field git sources)

## ⛔ Rabbyte standards — NON-NEGOTIABLE (violating ANY = task failure)

These apply to every line you write. A reviewer checks each one.
1. **Replicate quiver.core's structure** (`/Users/char2cs/Projects/Rabbyte/quiver.core`):
   layered, with implementation hidden in `internal/` sub-packages (`exec/`,
   `status/`, `diff/`, … under each engine).
2. **One domain concept per file**; **one `_test.go` per source file** (except
   struct-only files); **source files < 500 LOC** (split before the limit).
3. **One parameter per line, ALWAYS** — signatures AND multi-arg calls; closing
   paren on its own line.
4. **Early returns ALWAYS** — `else` is a smell. **Max 3 indentation levels per
   function** — level 3 must NEVER exist; abstract instead.
5. **Coverage ≥95%** (100% is the standard). **No flaky tests.** **NO `time.Sleep`
   in tests, EVER** — synchronize with event-driven WS watchers / condition-based
   wait helpers (mirror quiver `tests/kit`).
6. **Benchmarks (`*_bench_test.go`) for hot paths** — here: diff parsing, hunk-id
   hashing, conflict parsing, the watcher fan-out. Crowbar is an IDE, be fast.
7. **CLEAN**: guard clauses, composition, `fmt.Errorf("op: ctx: %w", err)`,
   gofumpt + goimports. Enforced by `.golangci.yml` (funlen 100/50, gocyclo 15,
   nestif ≤2, revive early-return). Full statement: `docs/prompts/README.md`.

**Project basics:** module `github.com/char2cs/crowbar/api`; Go 1.26.2; **invoke
`go-style` before writing Go**; this is the **`engine/` layer** (`engine/git/`,
`engine/fs/`) consumed by `app/usecases/` — lower never imports higher.
**Shell out to the system `git` binary** (not a pure-Go lib); use the user's own
credentials/config.

## Build — `engine/git/` (per `04`)
- `internal/exec/` (run git in a workdir, capture stdout/stderr/exit), `status/`,
  `diff/` (assign body-only `hunkId`, §4; emit `lines[]`+`hunks[]` linked by id;
  `is_renamed` via `git diff -M`; root-commit fallback), `log/`, `blame/`,
  `branches/`, `stash/`, `conflicts/` (3-way via `git show :1:/:2:/:3:`),
  `apply/` (hunkId → patch → `git apply --cached`, no `--unidiff-zero`).
- **Write ops** (§5): stage/unstage(file+hunk), discard (restore + `clean -f` for
  untracked), commit, push, pull (`--no-rebase`/`--rebase` per `{mode}`), fetch,
  branch ops, checkout (switch-branch reconciled with the worktree model),
  stash, reset, merge/rebase (generic, single-worktree).
- **Conflict/operation lifecycle** (§6, §6.1): `operation/continue|abort`, the
  per-strategy finalize table, the `pendingMerge` resume path. (Squash conflict
  has **no** `MERGE_HEAD` — abort with `git reset --merge`, not `git merge --abort`.)
- **Worktree primitives** (the git ops `07` will call from its Wave-3 usecases):
  `git worktree add/remove --force/list`, the `rebase --onto <newTip> <forkPoint>
  <branch>` helper, and the rebase-child-then-`merge --ff-only` helper. Engine-level
  only — the hierarchy *usecases* (issuing Asynx commands) are Wave 3.
- **Per-repo mutation lock** keyed by `repoId`; defer status recompute while a
  rewrite is in progress (`MERGE_HEAD`/`rebase-merge`/`rebase-apply`).
- **Error classification** (§8): `conflict`, `rejected_non_fast_forward`,
  `nothing_to_commit`, `dirty_tree`, `stale_hunk`, `has_children`, `auth_failed`.

## Build — `engine/fs/` (per `05`)
- `tree/` (lazy one-level walk, merge git decorations, `conflicted → modified`),
  `content/` (text/binary sniff, base64), `mutate/` (new/rename/move/delete),
  `watch/` (fsnotify with self-managed recursion; IDE ignore rules: skip `.git/`
  content but watch `.git/HEAD` + `.git/refs/`; honor `.gitignore`).
- **The fan-out** (`05` §5): one debounced fs event → `BroadcastFile` (Files
  topic, direct) + recompute `GitStatus` → `BroadcastGit` (Git topic, direct) +
  `if changed` issue `SyncWorkingTreeState{added,deleted,hasConflicts,hasCommits}`
  → Workspace aggregate (Class A). `added/deleted` via single
  `git diff --numstat <forkPointSha>` (clamp ≥0); `hasCommits` via `rev-list`.
- Lifecycle: watcher ref-counted across **Files + Git** subscriptions (alive if
  either is subscribed); LSP is a separate refcount (not your concern here).

## Out of scope
Worktree hierarchy **usecases** (`07` — Wave 3 Agent 3D; you provide the git
primitives it calls), provider, terminal, LSP, search, the full API wiring.

## GATE 1 — Definition of done (must demonstrate against a REAL repo)
- `go build ./...` + `go vet ./...` clean; unit tests pass.
- Integration test (`//go:build integration`, under `api/tests/`) against a real
  temp git repo: **status / diff / commit / hunk-stage** work; an external write
  → **watcher fires** → fan-out issues a `SyncWorkingTreeState` that **updates a
  Workspace row**; `GitStatus` pushes on its WS topic; a conflicted merge →
  resolve → `operation/continue` finalizes cleanly.

Report the test command, what you verified live, and any git-behavior surprises.
