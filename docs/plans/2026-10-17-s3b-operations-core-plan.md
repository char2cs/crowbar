# S3b: Operations core, git and workspace verbs. Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Operations exist end to end (table, service, boot pass, stream, client store, toasts), and every git and workspace verb that answers 202 runs as one. The git and worktree copies of `runAsync`, `SetLastError` as an outcome channel, the in-memory working overlay and `reparent-settle` are deleted.

**Architecture:** See the index, `docs/plans/2026-10-17-s3-operations-errors-plan.md` (Shared vocabulary, R2–R8, R11, R14, R15, R16, "Status and dispatch"). Requires T1, T2, and T8 merged (OR2). S4 T5 later deletes this task's `operationsDef`, `operations-stream.ts` and `hub.BroadcastOperation/PushOperation`, so keep them self-contained. Go commands use the index's `GOTEST`/`GOBOX`/`GOLINT` forms.

**Tech Stack:** Go, gin, gorm + SQLite, gorilla/websocket (`api/internal/api/v0/ws` `StreamDef`/`Broadcaster`), testify; zustand, vitest + Testing Library.

**Spec:** architecture §3 (Operations), §5 (operation lifecycle), §6, §8 "Operations are rows".

---

### Task 3: Operations, end to end; git verbs move onto them

**Files:**
- Create: `api/internal/domain/operation.go` (types in the index), `api/internal/app/usecases/operation/service.go` (`New`, `Start`, `Fail`, `Dismiss`, `Snapshot`, `Shutdown`), `api/internal/app/usecases/operation/recover.go` (`Recover`)
- Test: `api/internal/app/usecases/operation/{service_test.go,recover_test.go}`, `api/internal/domain/operation_test.go` only if `operation.go` gains behaviour (it should not)
- Modify: `api/internal/app/gorm.go` (`storesqlite.NewFromDB[domain.Operation, string](db)` beside the other `view.db` tables, `gorm.go:37–72`), `api/internal/app/container.go` (509 lines, oversize: build the service with that store, `hub.BroadcastOperation` and `time.Now`; call `Recover` before `New` returns; split by responsibility under 500 first, R15), `api/internal/app/shutdown.go` (`operations.Shutdown` first, before the `errors.Join` of the four asynx `Shutdown`s at `:69`), `api/internal/app/hub/{hub.go,subscriber.go}` (`BroadcastOperation(op domain.Operation)` / `PushOperation(op domain.Operation)`)
- Modify: `api/internal/api/v0/container.go` (875 lines, oversize: a `Broadcaster[domain.Operation]` with `operationsDef`: `Namespace` = id, `FlatNamespace: true`, `Snapshot` = `Service.Snapshot`; `PushOperation`; put `operationsDef` in its own file, as S5 put the file/document defs in `file_streams.go`, and move the other stream defs out so `container.go` ends under 500, R15), `api/internal/api/v0/router.go` (351 lines)
- Create: `api/internal/api/v0/endpoints/operations/{routes.go,handlers/handlers.go}` (`GET /v0/operations` WebSocket, `DELETE /v0/operations/:id`), test `handlers/handlers_test.go`
- Modify: `api/internal/api/libs/response.go` (`WriteOperation`; `IdempotencyKey(c) string` reads the header)
- Modify: `api/internal/api/v0/endpoints/git/handlers/write.go` (543 lines, oversize; Push, Fetch, Pull, Merge, Rebase → `ops.Start` + `libs.WriteOperation`, today `WriteAccepted` at `:222,238,254,428,458`; split by responsibility under 500, R15: index writes, branches, stash (its split part already has `stash_test.go`), sync/merge), `git/handlers/handlers.go` (`async detached.Ops` at `:86` and the working/last-error setter interfaces go; `*operation.Service` is a constructor argument), `git/routes.go` (returns no shutdown; it names `LastError` today), `api/internal/api/v0/router.go:227` (no detached append for git)
- Delete: `api/internal/api/v0/endpoints/git/handlers/async.go` (57 lines: `runAsync`, `Shutdown`, `recordLastError`) and its tests
- Test (black box): `api/tests/operations_test.go` (stream reader helper + generic cases), `api/tests/regression_git_push_failure_visible_test.go`
- Web create: `web/src/lib/store/operations.ts`, `web/src/lib/store/run-operation.ts`, `web/src/lib/store/entity-presence.ts`, `web/src/lib/store/operations-stream.ts` (`startOperationsStream(): () => void`, one `wsManager.subscribe('/v0/operations', …)` applying frames), `web/src/components/layout/operations-watcher.tsx`
- Web create: `web/src/lib/errors/operation-failed.ts`
- Web modify: `web/src/lib/errors/report.ts` (ignore `OperationFailedError`), `web/src/main.tsx` (call `startOperationsStream()` next to `installGlobalReporting()`), `web/src/lib/api/fetch.ts` (`readEnvelope` parses a 202 body; a request may carry `Idempotency-Key`), `web/src/features/git/api/git-remotes-api.ts` (`pushChanges(wsId, opId)`/`pullChanges(wsId, opId)` → `Promise<OperationReply>`), `web/src/features/git/components/branch-section.tsx` (push/pull through `runOperation`), `web/src/components/layout/ide-shell.tsx` (mount `OperationsWatcher`), `web/src/components/sidebar/sidebar-row.tsx` (680 lines, oversize: spinner when `row.working || busy`; split under 500, R15)
- Web test: create `web/src/__tests__/lib/store/{operations.test.ts,run-operation.test.ts,entity-presence.test.ts}`, `web/src/__tests__/components/layout/operations-watcher.test.tsx`; extend `web/src/__tests__/features/git/components/branch-section.test.tsx`, `components/sidebar/sidebar-row.test.tsx` (1114 lines, oversize: add no lines; the busy case goes in the test file of the part it lands in after the split)
- `isEntityPresent` at refresh (S2 not started, OR1/A8): projects and repos from `lib/store/sidebar.ts`'s `useSidebarStore`, workspaces and chats from the workspace store registry (`features/workspace/stores/`, chats in `slices/agent-chats-slice.ts`); grep the readers at dispatch. S2 T4 repoints the chat kind at the ChatStore.

**Interfaces:**
- Consumes: T1 `problem`; T2 `report`.
- Produces: everything under "Go: operations" and "Web" (operations store, `runOperation`, `whenSettled`, `dismissOperation`, `isEntityPresent`) in the index. `selectEntityBusy` and `selectEntityProblem` exist from this task. `libs.WriteOperation(c *gin.Context, op domain.Operation)`, `libs.IdempotencyKey(c *gin.Context) string`. The git handler constructor gains `ops *operation.Service`.
- Item semantics used by T5/T6: `Progress.Item` replaces the item with the same `Label`, else appends; every call bumps `Version` and publishes.
- `web/src/lib/errors/operation-failed.ts`: `class OperationFailedError extends Error { readonly problem: Problem; readonly operationId: string }`; `report()` returns early for it (the operations watcher already toasted that failure).

- [ ] **Step 1: Write the failing Go tests.** The service tests use a temp-dir SQLite `view.db` opened the way `app/gorm.go` opens it, a recording `publish` (channel of `domain.Operation`), and a fixed clock.

`service_test.go`:
```go
func TestService_StartRecordsAcceptedThenRunsTheWork(t *testing.T)    // published states accepted(v1), running(v2), succeeded(v3); the row matches the last frame
func TestService_FailedWorkCarriesItsProblem(t *testing.T)            // work returns problem.New(problem.Conflict, "held") → failed, Error {conflict, "held"}
func TestService_AFailedItemFailsTheOperationAsPartial(t *testing.T)  // 2 items, one Item(...) with Error → op failed, code partial, message "1 of 2 failed"
func TestService_StartIsIdempotentOnTheKey(t *testing.T)              // Review Focus 1: same ID, kind, subject twice → same op; work ran once
func TestService_AReusedKeyForOtherWorkIsAConflict(t *testing.T)      // same ID, other subject → CodeOf(err) == OperationIDConflict
func TestService_AMalformedKeyIsInvalid(t *testing.T)                 // ID "x" → InvalidArgument; empty ID → a minted UUID
func TestService_ShutdownInterruptsRunningWork(t *testing.T)          // work blocks on <-ctx.Done(); Shutdown returns after the op is failed{interrupted}
func TestService_FailRecordsDaemonStartedWork(t *testing.T)           // Fail(spec, err) → one frame, state failed, Error from problem.Of(err)
func TestService_DismissMarksASettledOperation(t *testing.T)          // dismissed true, version bumped, published
func TestService_DismissingRunningWorkIsAConflict(t *testing.T)       // OperationRunning; unknown id → NotFound
```
`recover_test.go`:
```go
func TestService_RecoverFailsWhatARestartInterrupted(t *testing.T)    // Review Focus 2: seeded accepted + running rows → failed{interrupted, "Crowbar stopped before this finished"}
func TestService_RecoverKeepsTheNewest500Settled(t *testing.T)       // 503 settled rows → the 3 oldest by updatedAt are gone; running rows are never pruned
```
`endpoints/operations/handlers/handlers_test.go`:
```go
func TestDismiss_ReturnsTheDismissedOperation(t *testing.T)
func TestDismiss_RunningWorkIs409(t *testing.T) // envelope code operation_running
```
`api/tests/operations_test.go`: a helper `awaitOperation(t *testing.T, h *harness, id string, done func(domain.Operation) bool) domain.Operation` that dials `/v0/operations` and reads frames until `done` (deadline from the harness's existing stream readers, no sleep), and
```go
func TestOperations_SnapshotCarriesSettledOperationsAcrossARestart(t *testing.T) // push fails (as below), h.crash(), restart; a fresh dial's snapshot has the op with the same error
```
`regression_git_push_failure_visible_test.go`:
```go
// TestRegression_GitPushFailureIsVisible: a push that fails in the background reaches the
// client with its reason instead of only the daemon log.
func TestRegression_GitPushFailureIsVisible(t *testing.T)
```
`newImportFixture`, import `feature/a`, commit a file in its worktree, then move `origin` aside (`os.Rename(fx.origin, fx.origin+".gone")`). `POST /v0/chats/:chatId/git/push` with `Idempotency-Key: <uuid>` → 202, `data.operation.id` equals the key, `kind == "git.push"`, `subject == {workspace, <wsId>}`. `awaitOperation` until settled: state `failed`, `error.message` non-empty and names the remote path.

- [ ] **Step 2: Run them to verify they fail.**

```bash
GOTEST ./internal/app/usecases/operation/... ./internal/api/v0/endpoints/operations/...
GOBOX 'TestRegression_GitPushFailureIsVisible|TestOperations_'
```
Expected: compile failure, then the push test fails on an empty 202 body.

- [ ] **Step 3: Implement the Go side.** `Start`: validate the key (empty → `uuid.NewString()`; not a UUID → `problem.InvalidArgument`); insert `accepted` (a duplicate primary key → read the row: same kind and subject → return it, else `OperationIDConflict`); publish; run `work` on a goroutine the service tracks (a `sync.WaitGroup` and a context it cancels in `Shutdown`), setting `running` first. On return: `nil` and no failed item → `succeeded`; a failed item → `failed{partial}`; an error → `failed` with `problem.Of(err)`; `ctx.Err() != nil` → `failed{interrupted}`. Every state change saves the row, bumps `Version`, sets `UpdatedAt` from the clock and publishes the saved row. A save error during `work`'s life is returned from `Shutdown` joined, and logged where nothing can return it (the service is the caller of last resort). Git handlers: `ops.Start(ctx, operation.Spec{ID: libs.IdempotencyKey(c), Kind: domain.OpGitPush, Subject: {workspace, wsID}, ProjectID, Label: "Push " + branch}, func(ctx, _ operation.Progress) error { return h.git.Push(ctx, wsID, time.Now()) })`, then `libs.WriteOperation(c, op)`; a `Start` error is `libs.Fail`. Same for Fetch, Pull, Merge, Rebase (labels "Fetch", "Pull", "Merge <ref>", "Rebase onto <ref>").

- [ ] **Step 4: Run the Go tests.** Same as Step 2 plus `GOTEST ./internal/api/v0/... ./internal/app/...`, `GOLINT`, `(cd api && go vet -tags 'integration noEmbed' ./tests/...)`. Expected: PASS.

- [ ] **Step 5: Write the failing web tests.**

`operations.test.ts`:
```ts
it('a frame inserts an operation and indexes it under its subject and items')
it('an older frame never replaces a newer one')                       // Review Focus 3
it('a server frame replaces the local operation with the same id')
it('a reconnect never drops a local operation still in flight')       // Review Focus 3: frames for other ids leave a local accepted entry in place
it('selectEntityProblem returns the newest failed operation naming the entity, cleared by a later success')
it('selectEntityProblem returns a failed item error for the item ref')
it('selectEntityBusy is true only while an operation naming the entity is accepted or running')
```
`run-operation.test.ts` (mock the transport by passing `send`):
```ts
it('inserts a local accepted operation before send resolves')
it('a 202 reply adopts the server operation')
it('a synchronous reply settles the local operation with its items')
it('a refused send fails the local operation with the problem')
it('whenSettled resolves once the operation succeeded and every created item is present')
it('whenSettled rejects with OperationFailedError carrying the problem')
it('whenSettled resolves a delete once its items are absent')
it('whenSettled for a branch create waits for the chat and its workspace')
it('whenSettled for a git push resolves on succeeded alone')
```
`report.test.ts` (extend): `it('ignores an OperationFailedError, which the operations watcher already showed')`.
`entity-presence.test.ts`: one case per kind (project, repo, workspace, chat) seeding the store that holds it after S2.
`operations-watcher.test.tsx` (mock `@/features/window/stores/toast-store`):
```ts
it('toasts the label and reason when an operation it saw running fails')
it('does not toast an operation that was already failed when first seen')
```
`branch-section.test.tsx`:
```ts
it('push starts a git.push operation and a failure shows its reason')
```
`sidebar-row` (split part's test):
```ts
it('a workspace row spins while an operation naming it runs')
```

- [ ] **Step 6: Run them to verify they fail.**

```bash
cd web && bunx vitest run src/__tests__/lib/store/operations.test.ts src/__tests__/lib/store/run-operation.test.ts src/__tests__/lib/store/entity-presence.test.ts src/__tests__/components/layout/operations-watcher.test.tsx src/__tests__/features/git/components/branch-section.test.tsx src/__tests__/components/sidebar
```

- [ ] **Step 7: Implement the web side.** `runOperation` mints `crypto.randomUUID()`, inserts `{…start, id, state:'accepted', items: start.items ?? [], version: 0, local: true}`, calls `send(id)`, then: a reply with `operation` → `applyOperationFrame(reply.operation)`; with `items` → the local op becomes `succeeded` with them; a throw → `failed` with `problemOf(err)`. It returns the local op synchronously. `whenSettled` subscribes to the operations store and to the stores `isEntityPresent` reads, resolving or rejecting on the first state that decides it; it has no timeout (R8). `OperationsWatcher` keeps a ref of the states it has seen and toasts (`toast.show({ key: op.id, type: 'error', message: op.label + ' failed', description: (op.error ?? firstItemError).message })`) on a transition into `failed`.

- [ ] **Step 8: Run the web tests, then the checks.** Step 6's command, `src/__tests__/lib/api`, `bun tsc --noEmit`, and the per-commit web checks.

- [ ] **Step 9: Move the budgets** (R1): detached git work leaves `go.http` for `go.service`; state both numbers.

- [ ] **Step 10: Commit**

```bash
git add <each file in this task's Files list that git status --short shows changed, by path>   # git rm the deleted ones by path
git commit -m "feat: operations are rows with a terminal state the user sees; git verbs run as operations

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01DG5r7UkKfKiDuXTdNTGTXK"
```

---

### Task 4: Workspace verbs are operations; the working overlay and `reparent-settle` go

**Files:**
- Modify: `api/internal/api/v0/endpoints/worktree/handlers/{sync.go,hierarchy.go,import.go,handlers.go}` (sync, merge-into-parent, rebase-onto-parent, reparent, retry-provision, detach-holder → `ops.Start`; import-batch → `ops.Start` with kind `branch.import`, subject `{repo, repoID}`, no items yet (T5 adds them); the working and last-error setter interfaces go, with `async detached.Ops` at `handlers.go:176`; `*operation.Service` is a constructor argument; `WriteAccepted` at `sync.go:24`, `import.go:87`, `hierarchy.go:79,152,192,217,243` become `WriteOperation`), `worktree/routes.go` (no shutdown; it names `LastError` today), `api/internal/api/v0/router.go:190` (no detached append for worktree)
- Delete: `api/internal/api/v0/endpoints/worktree/handlers/async.go` (75 lines), `async_internal_test.go`; the `WaitAsync` calls in `worktree/handlers/hierarchy_test.go` and `chat_scoped_test.go` become waits on the recorded operation (the handlers' tests build the service with a recording publish)
- Modify: `api/internal/app/repositories/container.go` (658 lines, oversize: delete `BeginWork`, `EndWork`, `IsWorking`, `inflight` (`:64–70`, `:123`), and `rebroadcast`'s overlay half; `WorkingFor` keeps the agent half; split under 500, R15), `api/internal/app/repositories/workspace/internal/reactors/delete.go` (`purgeUntilDone` at `:220`; its two give-ups at `:231,241` call an `onGiveUp(ctx, tomb domain.Workspace, err error)` constructor argument instead of logging "deferring to boot sweep"), `api/internal/app/repositories/workspace/workspace.go` (874 lines, oversize; split under 500, R15; if T8 merged first its OCC loop is already gone), `api/internal/app/container.go` (wire `onGiveUp` to `operations.Fail` with kind `workspace.purge`, subject the workspace, label "Remove <branch>", and a message ending "Crowbar retries when it next starts")
- Test: `api/internal/app/repositories/workspace/internal/reactors/delete_test.go` (extend), `api/internal/app/repositories/container_test.go` (645 lines, oversize: delete the overlay cases, add `TestWorkingFor_IsTheAgentHalfOnly` only within what the deletion frees, else in the test file of the split part), worktree handler tests (update), black box `api/tests/regression_reparent_failure_visible_test.go`
- Web modify: `web/src/lib/api/workspace.ts` (`reparentWorkspace`, `rebaseOntoParent`, `retryProvision`, `detachHolder` take `opId` and return `OperationReply`), `web/src/features/git/api/review-api.ts` (`mergeIntoParent` likewise), callers `web/src/features/git/components/{merge-popover.tsx,branch-section.tsx}`, `web/src/components/sidebar/sidebar-row-actions.tsx` (Retry), `web/src/components/layout/detach-holder-modal.tsx`, `web/src/components/sidebar/lib/drop-actions.ts` (999 lines, oversize: `fireRowPlacementCall`: `await whenSettled(runOperation(…reparent…).id)` then `placeWorkspace`; a rejection reaches `performSidebarDrop`'s catch, which calls `report(err, { action: 'Move' })`; split under 500 by responsibility, R15), `web/src/components/sidebar/lib/row-actions.ts` (671 lines; it also imports `watchReparent`: that use goes the same way; the file's split is T5's)
- Web delete: `web/src/components/sidebar/lib/reparent-settle.ts` (115 lines), `web/src/__tests__/components/sidebar/lib/reparent-settle.test.ts`
- Web test: `web/src/__tests__/components/sidebar/lib/drop-actions.test.ts` (2584 lines, oversize: replace the `watchReparent` cases, add none; new cases go in the test file of the split part they exercise; the scenario files `drop-actions-{chatless-reparent,cross-project,held-folder-children,repo-header-id-space,unmounted-home-siblings}.test.ts` are updated where they reach `watchReparent`, never grown), `features/git/components/merge-popover.test.tsx`, `components/sidebar/sidebar-row-actions.test.tsx`, `components/layout/detach-holder-modal.test.tsx`

**Interfaces:**
- Consumes: T3 `operation.Service`, `runOperation`, `whenSettled`, `report`.
- Produces: `reactors.RegisterDeleteReactor` (`delete.go:140`; there is no `NewDeleteReactor`) gains `onGiveUp func(ctx context.Context, tomb domain.Workspace, err error)` (required, R: constructor argument). Workspace DTO `working` is the agent half only (amendment A1).

- [ ] **Step 1: Write the failing tests.**

`api/tests/regression_reparent_failure_visible_test.go`:
```go
// TestRegression_ReparentFailureIsVisible: a reparent the daemon cannot complete reports why;
// the client used to infer it from a lastError change and a 30 s timeout.
func TestRegression_ReparentFailureIsVisible(t *testing.T)
```
Import `feature/a` and `feature/b` (both children of `main`), then POST `…/chats/<feature/a chat>/reparent` with `newParentId` = `feature/a`'s own workspace id and a key. `hierarchy.Reparent` refuses it in the background (`guardReparent`, `usecases/workspace/internal/hierarchy/worktree.go:1359`, returns `ErrSelfParent` for a move under itself). Assert 202 with the op, then `awaitOperation` → `failed`, `error.code` is the code T1 gave `ErrSelfParent` (not `internal`), `error.message` non-empty; `feature/a`'s workspace still has parent `main`.

`delete_test.go`:
```go
func TestDeleteReactor_AGiveUpIsReportedNotLogged(t *testing.T) // purger always fails, reactor timeout short via WithReactorTimeout; onGiveUp called once with the tomb and the last error
```
`container_test.go`: delete the BeginWork/EndWork cases; add
```go
func TestWorkingFor_IsTheAgentHalfOnly(t *testing.T)
```
Web, `drop-actions` split-part test:
```ts
it('a reparent the daemon refuses reports its reason and skips the placement call')
it('a reparent that succeeds places the workspace after the row moved')
```
`merge-popover.test.tsx`: `it('merge into parent runs an operation and closes')`. `sidebar-row-actions.test.tsx`: `it('Retry runs a retry_provision operation')`. `detach-holder-modal.test.tsx`: `it('Detach runs a detach_holder operation and closes')`.

- [ ] **Step 2: Run them to verify they fail.**

```bash
GOTEST ./internal/app/repositories/... ./internal/api/v0/endpoints/worktree/...
GOBOX 'TestRegression_ReparentFailureIsVisible'
cd web && bunx vitest run src/__tests__/components/sidebar/lib src/__tests__/features/git/components src/__tests__/components/sidebar/sidebar-row-actions.test.tsx src/__tests__/components/layout/detach-holder-modal.test.tsx
```

- [ ] **Step 3: Implement.** Kinds and labels: `workspace.sync` "Sync <branch>", `workspace.merge_into_parent` "Merge <branch> into <parent>", `workspace.rebase_onto_parent` "Rebase <branch> onto <parent>", `workspace.reparent` "Move <branch> under <parent>", `workspace.retry_provision` "Retry <branch>", `workspace.detach_holder` "Detach <path>", `branch.import` "Import <n> branches". The merge-into-parent work keeps its `DeleteCascade` of a leaf inside the same `Work`. Delete the overlay and its frame rebroadcast; `enrichFrame` sets `Working` from `WorkingFor` only. Client verbs pass `(opId) => verb(..., opId)` to `runOperation` and add the `Idempotency-Key` header.

- [ ] **Step 4: Run the tests.** Step 2's commands, plus `GOTEST ./internal/app/... ./internal/api/...`, `GOBOX 'TestRegression_(ReparentFailure|Import|Promote|BranchChildren|Placement)'`, `GOLINT`, `(cd api && go vet -tags 'integration noEmbed' ./tests/...)`, and `cd web && bunx vitest run src/__tests__/components/sidebar src/__tests__/features/git && bun tsc --noEmit`; then the ratchets (`file-size`, `counters`). Expected: PASS; `go.timers`/`web.timers` may fall (reparent-settle's timeout).

- [ ] **Step 5: Commit**

```bash
git add <each file in this task's Files list that git status --short shows changed, by path>   # git rm the deleted ones by path
git commit -m "refactor: workspace verbs are operations; the working overlay and reparent-settle are gone

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01DG5r7UkKfKiDuXTdNTGTXK"
```

