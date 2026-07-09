# Safe Pull + Conflict Surfacing — Implementation Plan

> **For agentic workers:** Use superpowers:subagent-driven-development to implement task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Make `git pull` safe-only (refuse + inform-only modal when it can't fast-forward, never wedge the tree) and make actual conflict states always light up the workspace warning icon.

**Architecture:** Backend refuses a non-fast-forwardable pull synchronously (HTTP 409 `not_fast_forwardable`) before the 202, and runs the async pull as `--ff-only`; two small fixes ensure any real conflict flips the workspace to `pr-conflicts`. Frontend adds an inform-only cossui modal (mirroring `detach-holder-modal.tsx`) shown when the refusal code comes back.

**Tech Stack:** Go (gin, gorm, char2cs/asynx) backend under `api/`; React + TypeScript + zustand + cossui (`@/components/ui/*`) frontend under `web/` (bun, vitest).

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-09-safe-pull-conflict-surfacing-design.md`.
- **Frozen contract:** refused pull = **HTTP 409**, error string exactly **`not_fast_forwardable`** (via `libs.WriteErr`). ff-able pull unchanged (`202`, async), now `--ff-only`.
- **No timing in tests** (HARD RULE): no sleeps, no `Eventually`/`After`, no poll-intervals. Block on real signals (asynx `WaitPublish`/`WaitQuiescent`/`WaitIdle`, channels). `go test -timeout` is the only backstop.
- Go: follow go-style. Backend black-box regression tests live in `api/tests` tagged `integration` (`TestRegression_*`); focused unit tests co-located per package convention.
- Web: component files **kebab-case**; exported component PascalCase. Test files mirror `web/src/` under `web/src/__tests__/`, using `@/` imports. Use `@/components/ui/*` + CSS-variable tokens only (no hardcoded colors). FE test/typecheck via bun.
- Implementers: do NOT `git commit` — leave changes staged/unstaged; the orchestrator integrates and commits.

---

## Backend

### Task B1: git-engine ff-check + ff-only pull

**Files:**
- Modify: `api/internal/engine/git/engine.go` (`Pull` ~:424; add `PullIsFastForward`)
- Modify: `api/internal/engine/git/git.go` (add `PullIsFastForward` to the engine interface near `Pull` ~:139)
- Test: `api/internal/engine/git/engine_test.go` (or the existing pull/ff test file)

**Interfaces:**
- Produces: `PullIsFastForward(ctx context.Context, repoPath, branch string) (bool, error)` — true iff the local branch can fast-forward to its upstream, i.e. it has **no** commits the upstream lacks. Local-only (no network): `git rev-list --count origin/<branch>..HEAD`; ff-able ⇔ count == 0. If `origin/<branch>` is unknown/absent, return `(false, nil)` (treat as "cannot ff" — safe default; the modal will explain).
- Produces: `engine.Pull` now runs `git pull --ff-only` for the default (`""`) mode.

- [ ] **Step 1 — failing test.** Add a table test in a temp repo: (a) local strictly behind `origin/b` → `PullIsFastForward` true; (b) local diverged (a commit not on `origin/b`) → false; (c) no upstream ref → false. Mirror the existing engine test helpers (look for how other `engine_test.go` cases build a temp repo with a fake `origin`).
- [ ] **Step 2 — run, expect FAIL** (`PullIsFastForward` undefined). `cd api && go test ./internal/engine/git/ -run TestPullIsFastForward`.
- [ ] **Step 3 — implement.** Add `PullIsFastForward` (pattern like `revListHasCommits` in `summary.go:75`, ref `"origin/"+branch+"..HEAD"`, ff-able ⇔ count==0; a non-zero exit / missing ref ⇒ `(false,nil)`). Add it to the interface in `git.go`. Change `Pull`'s default `flag` from `"--no-rebase"` to `"--ff-only"` (keep `"--rebase"` for `mode=="rebase"`). Update any mock engines that implement the interface (`api/internal/app/usecases/mocks/git_ops_engine.go`, and any `fakeGit`/`mockGitEngine` in touched packages) with a `PullIsFastForwardFn`.
- [ ] **Step 4 — run, expect PASS.** Same command + `go build ./...`.

### Task B2: Pull handler refuses non-ff synchronously (409)

**Files:**
- Modify: `api/internal/api/v0/endpoints/git/handlers/write.go` (`Pull` ~:249)
- Modify: `api/internal/api/v0/endpoints/git/handlers/handlers.go` (whatever the handler holds — it needs the git engine's `PullIsFastForward` and the workspace's repoPath + branch; follow how other handlers resolve `writePath`/branch)
- Test: `api/tests/integration/...` regression test `TestRegression_PullRefusesNonFastForward` (+ a handler-level test if the package has one)

**Interfaces:**
- Consumes: `PullIsFastForward` (B1). Frozen contract: 409 + error `not_fast_forwardable`.

- [ ] **Step 1 — failing test.** Regression test (integration tag, no timing): set up a workspace whose branch is diverged from its `origin/<branch>`; `POST /workspaces/:wsId/git/pull`; assert **409** and body `error == "not_fast_forwardable"`, and that the worktree is **clean afterward** (no `MERGE_HEAD`, no unmerged paths). Add a sibling case: a strictly-behind branch → **202**. Reuse the integration harness in `api/tests` (find an existing git-endpoint regression test to mirror setup/teardown and the asynx quiescence barrier).
- [ ] **Step 2 — run, expect FAIL.**
- [ ] **Step 3 — implement.** In `Pull`: resolve repoPath (as `mutate`/`writePath` does) and the branch; call `PullIsFastForward`. If `!ok` → `libs.WriteErr(ctx, http.StatusConflict, "not_fast_forwardable")` and `return` (do NOT `WriteAccepted`, do NOT `runAsync`). Else keep the existing `WriteAccepted` + `runAsync(... h.git.Pull(...))`. Resolve branch via the workspace/git engine the same way other handlers do (read the handler struct's deps; if branch isn't readily available, add a cheap engine `CurrentBranch`/reuse status — prefer an existing accessor).
- [ ] **Step 4 — run, expect PASS** + `go build ./...`.

### Task B3: `mutate` surfaces conflict on op error (Bug B, active path)

**Files:**
- Modify: `api/internal/app/usecases/git/git_write.go` (`mutate` ~:142)
- Test: `api/internal/app/usecases/git/git_write_test.go` (or the package's existing test file)

**Interfaces:**
- Consumes: existing `u.syncer.SyncWorkingTreeState(ctx, wsID, now)`.

- [ ] **Step 1 — failing test.** With a fake git op that returns a conflict error, assert `mutate` still calls `SyncWorkingTreeState(wsID, now)` (spy on the syncer) before returning the (wrapped) op error. Mirror existing `git_write_test.go` fakes.
- [ ] **Step 2 — run, expect FAIL.**
- [ ] **Step 3 — implement.** In `mutate`, when `op(repoPath)` returns an error: call `u.syncer.SyncWorkingTreeState(ctx, wsID, now)` (ignore/join its error — best-effort surfacing), then return the wrapped op error. Keep the success path identical. Guard against masking the original error (return the op error, not the sync error).
- [ ] **Step 4 — run, expect PASS.**

### Task B4: watcher emits conflict state during a rewrite (Bug B, passive path)

**Files:**
- Modify: `api/internal/engine/fs/internal/watch/watcher.go` (`fanOutGit` ~:358, `isRewriteInProgress` ~:410)
- Test: `api/internal/engine/fs/internal/watch/watcher_test.go`

**Interfaces:**
- Consumes: existing `ComputeWorkingTreeSummary`, `dispatcher.OnSyncWorkingTreeState`.

- [ ] **Step 1 — failing test.** Simulate a worktree with `MERGE_HEAD` present and unmerged paths; drive `fanOutGit`; assert the dispatcher receives an `OnSyncWorkingTreeState` with `HasConflicts: true` (today it is suppressed). Keep a second assertion that the noisy per-file `OnGitStatus` storm is still guarded during a rewrite (don't regress the 6Hz-storm fix). Mirror the existing `watcher_test.go` fakes/dispatcher spy.
- [ ] **Step 2 — run, expect FAIL.**
- [ ] **Step 3 — implement.** Restructure `fanOutGit` so the `isRewriteInProgress()` early-return no longer skips the working-tree summary. Minimal approach: keep the per-file `ComputeStatus`/`OnGitStatus` block behind the rewrite guard (storm protection), but always run the `ComputeWorkingTreeSummary` → change-detect → `OnSyncWorkingTreeState` block (so `HasConflicts`/added/deleted still broadcast during a rewrite). Preserve the existing `prevConflict`/`prevAdded`/etc. dedupe.
- [ ] **Step 4 — run, expect PASS** + `go build ./...` + `go test ./internal/engine/... ./internal/app/... ./tests/...` for touched packages.

---

## Frontend

### Task F1: pull-conflict modal (cossui) + store + mount

**Files:**
- Create: `web/src/features/git/stores/use-pull-conflict-modal-store.ts`
- Create: `web/src/components/layout/pull-conflict-modal.tsx`
- Modify: `web/src/components/layout/ide-shell.tsx` (import + render `<PullConflictModal />` next to `<DetachHolderModal />` ~:249)
- Test: `web/src/__tests__/components/layout/pull-conflict-modal.test.tsx`

**Interfaces:**
- Produces: `usePullConflictModalStore` with `{ target: { wsId: string; branch: string } | null; open(t): void; close(): void }` (mirror `useDetachModalStore`).
- Produces: `PullConflictModal` component (default hidden; renders when `target` set).

- [ ] **Step 1 — failing test.** Mirror `web/src/__tests__/components/layout/detach-holder-modal.test.tsx`: renders nothing when target is null; when `open({wsId,branch})` is called, renders the title (e.g. "Can't pull — branch has diverged"), a description mentioning the branch, and a single "Got it" button that calls `close()`.
- [ ] **Step 2 — run, expect FAIL** (module missing). `cd web && bun run test -- pull-conflict-modal`.
- [ ] **Step 3 — implement.** Store mirrors `detach-modal-store` (find it: `@/features/window/stores/detach-modal-store`). Component mirrors `detach-holder-modal.tsx` structure exactly — `Dialog`, `DialogPopup className="max-w-md"`, `DialogHeader`, `DialogTitle`, `DialogDescription` (inform-only copy: this branch has local commits origin doesn't, so it can't fast-forward; Crowbar won't merge automatically to avoid conflicts; resolve then pull again), `DialogFooter` with a single primary `<Button onClick={close}>Got it</Button>`. Tokens only. Mount in `ide-shell.tsx`.
- [ ] **Step 4 — run, expect PASS** + `bun run` typecheck.

### Task F2: surface the 409 code + open modal from both Pull triggers

**Files:**
- Modify: `web/src/features/git/api/git-remotes-api.ts` (`GitRemoteActionResult`, `gitRemoteOp`, `pullChanges`)
- Modify: `web/src/features/git/components/branch-section.tsx` (`runRemote` ~:63)
- Modify: `web/src/features/git/components/git-actions-menu.tsx` (`handlePull` ~:105)
- Test: `web/src/__tests__/features/git/api/git-remotes-api.test.ts` (+ a branch-section handler test if one exists)

**Interfaces:**
- Consumes: `usePullConflictModalStore` (F1). Contract: backend 409 error string `not_fast_forwardable`.

- [ ] **Step 1 — failing test.** Test `pullChanges` returns `{ success:false, code:'not_fast_forwardable' }` when the API rejects with a 409 whose envelope error is `not_fast_forwardable` (mock `apiFetch`; inspect how it throws — status and/or message). Test that other failures return `{success:false, error}` with **no** `code`.
- [ ] **Step 2 — run, expect FAIL.**
- [ ] **Step 3 — implement.** Extend `GitRemoteActionResult` with optional `code?: 'not_fast_forwardable'`. In `gitRemoteOp`, detect the refusal (inspect `apiFetch`'s thrown error — its status if exposed, else the message text `not_fast_forwardable`) and set `code`. In `branch-section.tsx` `runRemote`: when `!res.success && res.code === 'not_fast_forwardable'`, call `usePullConflictModalStore.getState().open({ wsId, branch })` instead of `setRemoteError`. In `git-actions-menu.tsx` `handlePull`: same branch — open the modal on the code (thread the `wsId`/branch it has; if it only has `repoPath`, resolve the wsId it already uses elsewhere in the menu). Keep non-code failures on the existing toast/error path.
- [ ] **Step 4 — run, expect PASS** + `bun run` typecheck + FE test suite for touched files.

---

## Self-review notes
- Spec coverage: B1/B2 = safe-only pull (spec B1); B3/B4 = surfacing (spec B2); F1 = modal (spec F1); F2 = wiring (spec F2); icon unchanged (spec F3). ✔
- Contract `not_fast_forwardable` used identically in B2 (produce) and F2 (consume). ✔
- No blanket auto-abort of Merge/Rebase (non-goal respected) — B3 only *surfaces*. ✔
