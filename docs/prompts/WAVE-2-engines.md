# WAVE 2 — Engines (FOUR agents in parallel)

After GATE 1 passes, fan out these four **independent** engine agents
concurrently. Each owns its own package(s) and **must not touch another agent's
code** or the foundation. They all consume the Wave 0 contracts and (for some)
the Wave 1 git engine.

## ⛔ Rabbyte standards — NON-NEGOTIABLE for all four agents (violating ANY = task failure)

A reviewer checks each one on every agent's output.
1. **Replicate quiver.core's structure** (`/Users/char2cs/Projects/Rabbyte/quiver.core`):
   layered, implementation hidden in `internal/` sub-packages.
2. **One domain concept per file**; **one `_test.go` per source file** (except
   struct-only files); **source files < 500 LOC** (split before the limit).
3. **One parameter per line, ALWAYS** — signatures AND multi-arg calls; closing
   paren on its own line.
4. **Early returns ALWAYS** — `else` is a smell. **Max 3 indentation levels per
   function** — level 3 must NEVER exist; abstract instead.
5. **Coverage ≥95%** (100% is the standard). **No flaky tests.** **NO `time.Sleep`
   in tests, EVER** — event-driven WS watchers / condition-based wait helpers
   (mirror quiver `tests/kit`).
6. **Benchmarks (`*_bench_test.go`) for hot paths** (per-agent: hierarchy diff,
   PTY fan-out, search match/walk, provider parse) — Crowbar is an IDE, be fast.
7. **CLEAN**: guard clauses, composition, `fmt.Errorf("op: ctx: %w", err)`,
   gofumpt + goimports. Enforced by `.golangci.yml` (funlen 100/50, gocyclo 15,
   nestif ≤2, revive early-return). Full statement: `docs/prompts/README.md`.

**Project basics:** module `github.com/char2cs/crowbar/api`; Go 1.26.2; **invoke
`go-style` before writing Go**; layered `engine → adapter → app → api` (lower never
imports higher); reference `quiver.core` + `api/ARCHITECTURE.md`.

**Done (each agent)** = `go build ./...` + `go vet ./...` + `make lint` clean,
unit tests pass at **≥95% coverage**, benchmarks for the hot paths, and an
integration test (`//go:build integration`) demonstrates the engine's core
behavior with **no `time.Sleep`**.

---

## Agent 2A — Worktree Hierarchy (`07`) ★ signature feature

**Read:** `api/docs/specs/v0/07-workspace-worktree-hierarchy.md`,
`00` §5.3/§6.1, `04` §5/§6.1 (the git primitives you call).
**Owns:** the Workspace-hierarchy **usecases** in `app/usecases/` + any
`engine/git` helpers needed (coordinate via the git engine's public API — do not
fork it).
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

## Agent 2B — Terminal / PTY (`06`)

**Read:** `api/docs/specs/v0/06-terminal-pty.md`, `03` §3 (Terminal topic).
**Owns:** `engine/terminal/` (`session/`, `registry/`, `profile/`) + the terminal
REST handlers + the bidirectional WS handler + `TerminalProfile` GORM CRUD.
**Build:** `creack/pty` session spawn in the workspace dir; in-memory registry;
ring buffer (default 64 KB); **atomic snapshot-and-register on attach** (one
sequence point under the session mutex); **multiple WS per session** with
per-client send queues + drop-on-overflow; resize (SIGWINCH); the wire protocol
(`{data}` / `{type:resize,…}` / server output). Profiles resolve shell/cwd/startup.
**Verify:** open a session, attach two WS clients, confirm fan-out + ring-buffer
replay-on-reattach with no loss/dup; resize works.

---

## Agent 2C — Global Search (`11`)

**Read:** `api/docs/specs/v0/11-global-search.md`.
**Owns:** `engine/search/` (`walk/`, `ignore/`, `match/`, `replace/`) + the two
search REST handlers.
**Build:** pure-Go (no `rg`); concurrent bounded worker-pool walk; `.gitignore`
hierarchy + skip `.git`; `regexp` (RE2) with the toggle→pattern mapping
(`(?i)`, `\b…\b`, `QuoteMeta`); `doublestar` include/exclude; null-byte binary
skip; `FindAllIndex` offsets; **result cap (default 1000) + `truncated` flag**;
replace writes to disk (rejects on `locked`) → triggers the watcher.
**Verify:** search a temp tree honors gitignore + toggles + cap; replace edits
files and the watcher picks it up.

---

## Agent 2D — Git Provider Engine (`08`)

**Read:** `api/docs/specs/v0/08-git-provider-engine.md`, `00` §5.3/§6.4.
**Owns:** `engine/provider/` (`github/`, `gitlab/`, `detect/`, `poll/`) + the two
read-only provider REST routes.
**Build:** **read-only** `GitProvider` interface (`ProtectedBranches`,
`PullRequestForBranch`) over the **`gh` / `glab` CLIs** (required; graceful
disable + config-list fallback when absent); PR selection rules (pushed branches
only, head-match same repo, most-recent-open wins); polling (on-view via
`GET …/provider` + 60s background sweep of open-PR workspaces); on change issue
`SyncProviderState` to the Workspace aggregate (no new broadcaster). Never create
PRs.
**Verify:** with `gh` present, resolve protected branches + a branch's PR state
and drive a Workspace `pr-*` badge; with the CLI absent, capability disables
gracefully.
