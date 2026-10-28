# S3c: Creates and imports are drawn from operations; repos and projects. Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the user's complaint ("Sometimes Crowbar cannot import/create a branch nor says why it failed"): every branch import and create is an operation whose rows the sidebar draws until the entities arrive, each failure carries its reason, and nothing waits for a row to land. Then do the same for repos and projects and delete the last lossy channel.

**Architecture:** See the index, `docs/plans/2026-10-17-s3-operations-errors-plan.md` (R3, R4, R8–R13, R15, D5, Review Focus 1, 2, 4, "Status and dispatch"). Requires T1–T4. S4 T8/T9 fold this task's `rows-from-operations.ts` and `create-draft.ts` into the Tree. Go commands use the index's `GOTEST`/`GOBOX`/`GOLINT` forms.

**Tech Stack:** Go, gin, asynx (workspace upcaster), testify, the `api/tests` harness; React 19, zustand, TanStack Router, vitest + Testing Library.

**Spec:** architecture §7 row "Branch import/create fails with no reason", §8 "Operations are rows", §11 row S3 gate "Import/create failure visible in tests".

---

### Task 5: Creates and imports are drawn from operations

**Files (Go):**
- Modify: `api/internal/api/v0/endpoints/worktree/handlers/import.go` (body `{branches: [{name, chatId}], lock}`; body binding and "each `chatId` is a UUID" stay synchronous 400s before `Start`; "each `chatId` is unused" (409 `id_conflict`) is the `Spec.Validate` that `Start` runs only for a new key, so a retry with the same key after the first import created its chats answers 202 with the first operation instead of 409, Review Focus 1; items at accept: one per branch `{Ref:{chat, chatId}, Label: name, State: running}`)
- Modify: `api/internal/app/usecases/workspace/internal/hierarchy/import.go` (348 lines; `ImportInput` at `:19`, `CreateFromImport` at `:76`, and the `CreateFromImport` signatures in `hierarchy/worktree.go:64` (1816 lines, oversize: touch only that signature) and `usecases/workspace/workspace.go:220` (530 lines, oversize: split by responsibility before adding)) and `api/internal/app/usecases/workspace` (exported `ImportInput`): `Branches []ImportBranch` with `type ImportBranch struct{ Name, ChatID string }`, `Lock bool`; `CreateFromImport(ctx context.Context, in ImportInput, p ImportProgress) error` with `type ImportProgress interface{ Branch(o ImportOutcome) }` and `type ImportOutcome struct{ Branch, ChatID, WorkspaceID string; Err error }`. The requested branch's chat uses `ChatID`; a branch that already has a workspace reports that workspace's owning chat id; an ancestor added from the PR graph reports its daemon-minted ids; a failed branch keeps the chat its attempt minted with `ChatID` as its placeholder's owning chat (no purge and re-mint, below) and the outcome carries the cause; if the placeholder also fails, the outcome carries `errors.Join(cause, placeholderErr)`. `Lock` maps onto the existing `ImportedBranch.ForceLocked` (`hierarchy/owning_chats.go:29`, honoured at `worktree.go:337,407,793`) per requested branch, so the workspace is created locked rather than locked by a second command (this replaces the client's `armImportLockWatch`). `CreateFromImport`'s signature change also reaches `hierarchy/{worktree_createfromimport_test.go,import_resolve_holder_path_test.go,worktree_import_regression_test.go,worktree_import_holder_regression_test.go}`.
- Modify (the failed attempt's chat is the placeholder's owner): today `createImportedWorktreeChat` (`usecases/chat/internal/tree/imported_chat.go:21-49`) mints the chat, and on a spawn failure `discard`s it with `PurgeChat` (`tree/chats.go:192-201`, which forgets the aggregate and only logs its own failure); `importPlaceholder` (`hierarchy/import.go:277-291`) then calls `MintOwningChat` again. Reusing the client's id there would recreate a just-forgotten id, which a redelivered Forget marker (R21, 4–14% after T8) deletes from the read model, or hit `id_conflict` when the purge failed. So `ImportBranchAsChat` returns the minted, placed chat with the spawn error, and `importPlaceholder` attaches the placeholder to that chat instead of minting one; `discard` is no longer called on the import path.
- Modify: the chat create path takes the client's chat `id` (UUID, else 400): `Conversations.MintChat` (`internal/conversation/conversation.go:313-326`, `chatID := uuid.NewString()` today) and `Usecase.MintChat` (`usecases/chat/chat.go:272`) gain an `id` parameter, rippling through `tree.Agent`, `usecases/mocks/mocks.go` (`MintChat`), `chat/home_read_test.go:59`, `MintOwningChat` (`tree/owning_chat.go:49-63`), `createImportedWorktreeChat` and `hierarchyOwningChats` (`usecases/container.go:861-917`; 1052 lines, oversize: if the adapter changes, move it out to its own file first). A taken id is refused by the aggregate's create command (the stream already exists → `problem.IDConflict`), never by a handler lookup, which would race and miss a purged id. `api/internal/api/v0/endpoints/chat/handlers/chats.go` (605 lines, oversize: split by responsibility before adding into `chats_create.go`, `chats_read.go`, `chats_mutate.go`; the index's S2 rebase note names them) create, own-worktree create, single import pass the id; the own-worktree create answers `{id, workspaceId}`
- Delete: `Workspace.LastError` (`api/internal/domain/workspace.go`), `api/internal/app/repositories/workspace/internal/commands/set_last_error.go` (it has no test file), the `ws.LastError = ""` resets in `commands/{set_lock.go:72,sync_working_tree_state.go:54,sync_provider_state.go:52}` (R9: a later success clears it on the client), `SetLastError` on the workspace repository and its interface (`repositories/workspace/workspace.go:167–170,674`), `WorkspaceDTO.LastError`, `ChatWorktreeDTO.LastError` and their mapping, the `LastError` uses in `worktree/handlers/{import.go,hierarchy.go,sync.go,handlers.go}` left after T4, and the stale "behind a LastError frame" comment at `worktree/handlers/lock.go:27`; the stale `runAsync` comment at `hierarchy/import.go:85`
- Delete/modify (black box, kit and integration readers of `LastError`, X4; compile-only suites, the commit body says "integration suites compile-only; user to run"): `api/tests/kit/env.go:875-896` `WaitForWorkspaceLastError` (deleted; and the `runAsync` comment at `env.go:103`), `api/tests/integration/lifecycle/lifecycle_test.go:71-112` (`TestLifecycle_SyncClearsLastError` becomes an operation-success case, or is deleted if no behaviour remains), the `LastError` fields of `api/tests/fixtures_test.go:54,416,428,475`, `api/tests/regressions_test.go:792-881` (`TestRegression_ImportDefaultBranchStaysHomeWithPlaceholder` and the detach-holder case assert the outcome through `LastError`: they read the operation instead), the comment at `api/tests/regression_import_branches_test.go:424`
- Modify (carry from T11b): `api/internal/app/usecases/internal/provision/provision.go:77`: only "no local branch" leaves `localTip` empty; any other `RevParse` failure is returned wrapped, so an import or create that cannot read the local tip fails with that reason instead of skipping the discarded-tip warning. The engine's `revParse` (`engine/git/worktree_add_branch.go:35-45`) returns an untyped `RequireSuccess` error today, so there is nothing to tell "absent" apart by: it runs `rev-parse --verify --quiet <rev>` and wraps `git.ErrBranchNotFound` (`engine/git/errors.go:19`) when git exits 1 with no output (absent), any other failure as now. `hierarchy/worktree.go:747` reads `localTip, localErr` the same way and is treated the same. Create `provision_test.go` (none exists) with `TestExistingBranch_ARevParseFailureIsReturned` and `TestExistingBranch_AnAbsentLocalBranchIsNotAFailure`, driven through the exported `ExistingBranch` (`:39`) with a fake `Git` that returns `ErrBranchNotFound` for the absent case and another error for the failure case.
- Modify: `api/internal/app/repositories/workspace/upcast.go` (R13: `SchemaVersion` 2 → 3 at `:14`; `StripRetiredPlacementFields` (`:39`) → `StripRetiredFields`, also dropping `/lastError`), `api/internal/app/container.go` (`:73–80` registers `WithUpcaster(1, …)` today; register it for 1 and 2)
- Test: `api/internal/app/usecases/workspace/internal/hierarchy/import_test.go`, `api/internal/app/usecases/chat/chat_test.go` (`TestMintChat_ATakenIDIsAnIDConflict`), create `api/internal/app/repositories/workspace/upcast_test.go` (none exists, and no test names `StripRetiredPlacementFields` today), `api/internal/api/v0/endpoints/worktree/handlers/import_test.go` (exists), black box `api/tests/regression_import_failure_visible_test.go`

**Files (web):**
- Create: `web/src/lib/store/create-draft.ts` (the sidebar's naming input: `{ draft: CreateDraft | null; start(d: CreateDraft): void; cancel(): void }`, `CreateDraft = { kind: 'branch' | 'chat'; projectId: string; repoId?: string; workspaceId: string; parentId: string; order: number; ownsWorktree: boolean }`), `web/src/components/sidebar/lib/rows-from-operations.ts`, `web/src/components/sidebar/operation-sidebar-row.tsx` (split out of `sidebar-row.tsx`, replaces `PendingSidebarRow`)
- First step (C13): perform S2 T0's split of `features/agent/api/agent-api.ts` (1381 lines) into files named for what they export: chat reads, chat commands, chat creates, providers/settings, every part under 500, moves only (amendment A9 tells S2 T0 to skip this file). Then `createChat`, `createChatWithOwnWorktree` (`:1110-1173` at refresh, in the chat-creates part) take `id` and `opId`. S2 T3–T5 also edit these parts (A8).
- Modify: `web/src/components/layout/create-actions.ts`, `thread-create.ts`, `home-actions.ts` (creates through `runOperation`; branch create opens its chat after `whenSettled`, D5; thread creates open at once, as today), `web/src/components/sidebar/lib/row-actions.ts` (671 lines, oversize: `performImportBranches` through `runOperation`, `lock` sent; split under 500, R15), `web/src/components/sidebar/lib/rows-from-repo.ts` (702 lines, oversize: no `lastError`, no `rowRepoScope` (`:411`, used by `space-scroller.tsx:94` and `drop-actions.ts:49,207`); split under 500 by moves only, so S4 T8 deletes whole files: the icon-fields/`chatIconIndex` half (`:156-260`) against the tree walk; the walk builds `placeholderReason` (`:192-203`) from a `problemOf(id)` lookup passed in, so it reads no store), `components/sidebar/space-scroller.tsx` (479), `components/layout/sidebar-tree-surface.tsx`, `components/sidebar/lib/drop-actions.ts` (split by T4; `rowsFromOperations` instead of `rowsFromPending`/`hideRowsForInFlightCreates` (`rows-from-pending.ts:58`)/`rowRepoScope`), `components/sidebar/sidebar-row.tsx` (split by T3), `components/layout/workspace-row-base.ts` (it names `PendingSidebarRow` today), `sidebar-row-actions.tsx` (reason from the newer of `selectEntityProblem(s, row.id)` and `selectEntityProblem(s, row.workspaceId)`: import items name the chat, git/workspace/purge operations name the workspace, `types/sidebar-row.ts:6,16`), `components/sidebar/types/sidebar-row.ts` (`pending?` → `operation?: { opId: string; state: OperationState; problem: Problem | null }`), `lib/workspace/placeholder.ts` (`placeholderReason(ws, kind, problem)` from T4 drops its `ws.lastError` fallback), `components/layout/placeholder-toast-watcher.tsx` (kept, C8: it toasts every unprovisioned placeholder, including the ones repo add and boot create, which are not operations, with the **Fix…** action that opens the Detach modal; it loses its `lastError` text and skips a workspace whose newest operation already failed, which `OperationsWatcher` toasted), `lib/api.ts` (`importBranches(projectId, repoId, branches: {name: string; chatId: string}[], lock: boolean, opId: string): Promise<OperationReply>`; drop `lastError` mapping), `lib/types.ts`, `lib/store/sidebar.ts`, `lib/store/build-repo-tree.ts` (workspace `lastError` gone), `features/panes/hooks/use-pane-keyboard.ts` (passes a minted id), `components/layout/create-actions.ts` (also loses `forkHasLanded`, `:77`). `thread-create.ts` names `awaitEntity` only in a doc comment (`:41`): reword it; the real callers are T6's modals.
- Modify (baselines, X2): `web/eslint.config.js` deletes the `MAX_LINES_BASELINE` entry of every listed file this task brings under 600 lines or deletes: `src/components/sidebar/lib/row-actions.ts`, `src/components/sidebar/lib/rows-from-repo.ts`, `src/features/agent/api/agent-api.ts`, and among the tests `row-actions.test.ts`, `rows-from-repo.test.ts`, `space-header.test.tsx` and `agent-api.test.ts` if they shrink under it; `bun run lint:baselines` confirms.
- Delete: `web/src/lib/store/pending-creates.ts`, `web/src/components/sidebar/lib/rows-from-pending.ts`, `rowRepoScope` (`rows-from-repo.ts`), `panelRowsAtClick`, `failCreate`, `untilLanded`, `ROW_ARRIVAL_TIMEOUT_MS`, `waitForRow`, `chatHasLanded`, `forkHasLanded`, `waitForHomeTree`, `waitForHomeChat`, `waitForRootHomeChat`, `importedBranchHasLanded`, `waitForImportedBranch`, `armImportLockWatch`, `IMPORT_LOCK_SETTLE_TIMEOUT_MS`, and their tests (`pending-creates.test.ts`, `rows-from-pending.test.ts`, the `create-actions-pending-order`, `-branch-row`, `-chatless`, `-owner-not-listed` scenario files: cases that still describe behaviour move into `create-actions.test.ts`, which ends ≤500 lines; `space-scroller-create-round-trip.test.tsx`)
- Test: create `web/src/__tests__/lib/store/create-draft.test.ts`, `web/src/__tests__/components/sidebar/lib/rows-from-operations.test.ts`, `web/src/__tests__/components/sidebar/operation-sidebar-row.test.tsx`; rewrite `components/layout/{create-actions,thread-create,home-actions}.test.ts` (each ≤500; at refresh `create-actions.test.ts` 525 and `thread-create.test.ts` 571 are oversize); the split parts of `row-actions` (test 857 lines) and `rows-from-repo` (test 1065; scenario files `rows-from-repo-{chatless,own-default-branch}.test.ts`) get their own test files named for them; `space-scroller.test.tsx` (1145) is not grown; `components/layout/placeholder-toast-watcher.test.tsx` (`it('a placeholder whose operation already failed is not toasted twice')`). `vi.mock` cap (X3): `thread-create.test.ts` has 4, `home-actions.test.ts` 5, `row-actions.test.ts` 5, and `create-actions.test.ts` 3: each rewritten or extended file ends at ≤3 by injecting `runOperation`/`send` and seeding the operations store instead of mocking the API module; new split-part test files start at ≤3.

**Interfaces:**
- Consumes: T3 `operation.Service`, `Progress`, `runOperation`, `whenSettled`, `selectEntityProblem`, `isEntityPresent`, `dismissOperation`; T2 `report`.
- Produces: `rowsFromOperations(ops: readonly Operation[], scope: { projectId: string; repoId?: string }): SidebarRow[]`, one row per item that is not present, placed as the last child of `item.parentId ?? op.subject.id`, `id` = `item.ref.id`. A running or accepted item, or a succeeded one whose entity has not arrived → a creating row (spinner, label). A failed item, or an item of an `interrupted` operation, whose entity is absent → a failed row (glyph, "failed", the problem message, Dismiss → `dismissOperation`). A dismissed operation draws nothing. A failed item whose entity exists draws nothing: the entity row shows the reason through `selectEntityProblem` (Review Focus 4).

- [ ] **Step 1: Write the failing black-box tests.** `api/tests/regression_import_failure_visible_test.go`, sharing one setup helper `importWithOneBadBranch(t, h) (fixture, opID, badChatID, goodChatID)`: `newImportFixture`; the seed clone pushes branches `bad/x` and `good` to origin; the test creates a regular file named `bad` in the directory that holds main's workspace root (`filepath.Dir(worktreepath.WorkspaceRoot(<main worktree path>))`), so `git worktree add` for `bad/x` fails with "Not a directory" (git prints strerror capitalised, `engine/git/worktree_add_branch.go:77`; assertions use `strings.Contains(strings.ToLower(msg), "not a directory")`); POST `import-batch` with `Idempotency-Key: <opID>` and `{branches:[{name:"bad/x",chatId:<A>},{name:"good",chatId:<B>}]}`.

```go
// TestRegression_BranchImportFailureIsVisible: an import whose branch cannot be checked out
// says which branch failed and why; the other branch lands.
func TestRegression_BranchImportFailureIsVisible(t *testing.T)
// 202, data.operation.id == opID, kind "branch.import", two running items.
// Settled: state failed, error.code "partial"; item "bad/x" failed, message contains "not a directory";
// item "good" succeeded with ref.id == B; B's workspace has a worktree path; A is a placeholder's owning chat.
// RED at base: the 202 body is empty, so there is no data.operation.

// TestRegression_BranchImportReasonSurvivesRestart: the reason is still there after a restart.
func TestRegression_BranchImportReasonSurvivesRestart(t *testing.T) // h.crash(), restart on the same home, snapshot has opID with the same item error

// TestRegression_BranchImportRetryReturnsTheSameOperation (Review Focus 1)
func TestRegression_BranchImportRetryReturnsTheSameOperation(t *testing.T) // same key and body twice → same op id; exactly one workspace for "good"

// TestRegression_ImportRefusesATakenChatID
func TestRegression_ImportRefusesATakenChatID(t *testing.T) // chatId of an existing chat → 409 id_conflict; no operation with that key in the snapshot
```

- [ ] **Step 2: Write the failing unit tests.**

`hierarchy/import_test.go`:
```go
func TestCreateFromImport_ReportsEachBranchOutcome(t *testing.T)
func TestCreateFromImport_UsesTheClientChatIDs(t *testing.T)
func TestCreateFromImport_AFailedBranchKeepsItsChatAsThePlaceholderOwner(t *testing.T) // the placeholder's owning chat is the attempt's chat (ChatID); nothing is purged or re-minted
func TestCreateFromImport_AFailedPlaceholderStillReportsTheBranch(t *testing.T)
func TestCreateFromImport_LocksWhenAsked(t *testing.T)
```
`upcast_test.go`:
```go
func TestStripRetiredFields_DropsLastErrorPatches(t *testing.T) // a v2 event whose patches set /lastError replays into a Workspace without error
```

- [ ] **Step 3: Write the failing web tests.**

`rows-from-operations.test.ts`:
```ts
it('a running import draws one creating row per branch under its repo')
it('a failed item draws its reason and a Dismiss action')           // the user's bug: the reason is on screen
it('a failed item whose entity exists draws only the entity')       // Review Focus 4
it('an interrupted import row says Crowbar stopped before it finished') // Review Focus 2
it('a succeeded item disappears once its entity is present')
it('a dismissed failed operation draws nothing')
it('a failed push on the workspace shows on its chat row')         // the row reads the problem by workspaceId too
```
`operation-sidebar-row.test.tsx`:
```ts
it('shows the branch name and the reason an import failed')
it('Dismiss dismisses the operation')
```
`row-actions` (the split part holding `performImportBranches`):
```ts
it('import sends one minted chat id per branch and the lock flag')
it('a refused import (409) shows its reason on each branch row')
```
`create-actions.test.ts`:
```ts
it('a branch create draws its row from the operation and opens the chat once the branch exists')
it('a branch create the daemon refuses keeps its row with the reason')
it('confirming a name starts the create; cancelling clears the draft')
```
`thread-create.test.ts`: `it('a thread create draws its row from the operation and opens at once')`. `home-actions.test.ts`: `it('a home thread create draws its row from the operation')`. `create-draft.test.ts`: start/cancel.

- [ ] **Step 4: Run them to verify they fail.**

```bash
GOTEST ./internal/app/usecases/workspace/... ./internal/app/usecases/internal/provision/... ./internal/app/repositories/workspace/... ./internal/api/v0/endpoints/worktree/...
GOBOX 'TestRegression_(BranchImport|ImportRefuses)'
cd web && bunx vitest run src/__tests__/components/sidebar src/__tests__/components/layout src/__tests__/lib/store
```

- [ ] **Step 5: Implement the Go side.** The import handler's work adapts `ImportOutcome` to `Progress.Item` (`Err == nil` → `succeeded`, else `failed` with `problem.Of(Err)`; ref `{chat, ChatID}`, label the branch name; T3's `Item` replaces by label; a failed item fails the operation as `partial`, R5). Delete `LastError` and `set_last_error` everywhere `go build` points; the import placeholder no longer sets it.

- [ ] **Step 6: Implement the web side.** Rows come from `rowsFromOperations` merged after the entity rows; the naming row comes from `create-draft`. Placeholder reason: held-by path from the entity, problem from the newer of `selectEntityProblem(s, row.id)` and `selectEntityProblem(s, row.workspaceId)`.

- [ ] **Step 7: Run all of the above, then wider.**

```bash
GOTEST ./internal/app/... ./internal/api/... ./internal/domain/... ./internal/engine/git/...
GOBOX 'TestRegression_(BranchImport|ImportRefuses|Import|CreateOwnWorktree|SidebarCreate|ChatFirst|Unprovisioned|Promote|DetachHolder)'
cd web && bunx vitest run src/__tests__/components/sidebar src/__tests__/components/layout src/__tests__/lib src/__tests__/features/panes/hooks src/__tests__/features/agent/api && bun tsc --noEmit && bun run knip && bun run lint:baselines
GOLINT && (cd api && go vet -tags 'integration noEmbed' ./tests/... && go vet -tags 'integration noEmbed' ./internal/...)
node scripts/ratchet/file-size.mjs && node scripts/ratchet/counters.mjs
```
Expected: PASS; knip clean (every deleted export gone with its last importer); `web.timers` falls (`ROW_ARRIVAL_TIMEOUT_MS`, `IMPORT_LOCK_SETTLE_TIMEOUT_MS`).

- [ ] **Step 8: Live check** (OR6: the noVNC container, isolated `CROWBAR_HOME` under the worktree's `/.crowbar/`, socket path under the worktree or stop). In a scratch repo with an `origin` holding `bad/x` and `good`, create the file `bad` beside the default branch's workspace root, import both from the sidebar: two rows appear at once; `good` lands; `bad/x` shows "failed" with "not a directory" and Dismiss; a toast names it once. Restart the dev daemon: the reason is still on the row. Screenshots to the session scratchpad.

- [ ] **Step 9: Commit**

```bash
git add <each file in this task's Files list that git status --short shows changed, by path>   # git rm the deleted ones by path
git commit -m "fix: a branch import or create that fails says which and why

Imports and creates are operations; the sidebar draws their rows until the
entities arrive and keeps a failed row with its reason. pending-creates and
every landing watcher are gone.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01DG5r7UkKfKiDuXTdNTGTXK"
```


---

### Task 6: Repos and projects are operations; the last lossy channel goes

**Files (Go):**
- Modify: `api/internal/api/v0/endpoints/projects/handlers/projects.go` (322 lines; the create route is `POST /projects` → `Import` at `:225`, → `project.create`, subject and single item `{project, <id minted at accept>}`, label "Add <name>"; `Delete` at `:265` → `project.delete`, subject and item the project, label "Remove <name>"; `BeginDelete` and its broadcast stay before the 202; `async detached.Ops` at `:85` and `WriteAccepted` at `:245,279` go), `api/internal/api/v0/endpoints/repos/handlers/repos.go` (1051 lines, oversize: `Create` at `:374` → `repo.create`, subject `{project, p}`, item `{repo, <id minted at accept>}`; the work adds one item `{workspace, <wsID>}` per protected branch (below); `DeleteRepo` at `:571` → `repo.delete`; `WriteAccepted` at `:416,600` and `async detached.Ops` at `:169` go; split under 500, R15, and in the split un-squash the 131-column `PRLinkDTO` literal at `:852` (T11b carry); delete the ignored `createRequest.ID` (`:359`))
- Modify: `api/internal/app/usecases/project/project_import.go` (926 lines, oversize: `Create` (`:346`) and `ImportRepo` (`:403`) take the id minted at accept and report each protected branch through a `RepoProgress interface{ Workspace(id, branch string, err error) }`; split under 500). `importOneRepo` sets `committed = true` (`:533`) before `provisionProtectedWorktrees` (`:536`, `:712-731`), which only `slog.Warn`s each failure: that warn-and-continue is the lossy channel this task removes. Each protected branch becomes an item `{workspace, <wsID>}` (the managed worktree's own uuid, `:768`, not `RepoHomeID`), succeeded or failed with its cause; a failed one makes the operation `failed{partial}` while the repo stays committed and listed with its repo-home workspace.
- Modify: `api/internal/app/usecases/project/project_delete.go` (560 lines, oversize: a failed delete returns its error instead of saving `LastError`; with `LastError` gone, `DeleteRepo`'s re-mark guard (`:253`, `!repo.Deleting || repo.LastError != ""`) becomes "(re)mark when the repo is not deleting, else idempotent", and the clears at `:187,286` go; `Resume` (`:346`) becomes `PendingDeletes(ctx) ([]PendingDelete, error)` with `type PendingDelete struct{ Kind domain.OperationKind; Ref domain.EntityRef; ProjectID, Label string; Run func(context.Context) error }`; split under 500)
- Modify: `api/internal/app/container.go` (boot, after T3's `Recover` so it cannot mark the boot's own operations `interrupted`: for each `PendingDelete`, `operations.Run` with a daemon-minted id, replacing the slog-only `Resume` call at `container.go:155`. `Run` settles before it returns, so invariant D5, "a project or repo delete a crash or a failure stopped is finished before anything is served", still holds; a failed resume is a recorded `failed` operation, not a log line)
- Delete: `Repository.LastError`, `Project.LastError` (`api/internal/domain/{repository,project}.go`), `RepoDTO.LastError`, `ProjectDTO.LastError`; `projects/handlers/async.go`, `repos/handlers/async.go` (30 lines each) and every `WaitAsync` (tests wait on the recorded operation), with their two `Handlers.WaitAsync` lines in `api/deadcode-baseline.txt`; `api/internal/api/v0/endpoints/internal/detached/` (99 + 80 test lines); the four `c.detached = append(...)` in `router.go:122,132,190,227` (T3/T4 removed the last two); `ShutdownDetached` (`api/internal/api/v0/container.go:107–110`, `api/internal/api/container.go:66–68`, `api/internal/internal.go:370`, `api/tests/kit/env.go:262,312`)
- Modify (R2): every remaining `libs.WriteAccepted` answers `libs.WriteNoContent(c)` (new: `c.Status(http.StatusNoContent)`): at refresh, after T3–T6, the callers left are the chat handlers `Stop`, `Close`, `SwitchToNative` (`chat/handlers/switch.go:94,117,187`) and `Rename`, `SetSelection`, `Delete` (`chats.go:481,561,604`), all of which finish in the request. Prompt submission answers 200 (`PromptSubmissionDTO`, S1) and compact uses `WriteMutationOK(…, 202, …)`, so `WriteAccepted` loses its last caller: delete it and its test. `WriteMutationOK(…, http.StatusAccepted, sid)` in `terminal/handlers/sessions.go:148` (kill session) finishes in the request too: it answers `http.StatusOK` with the same body. Web callers of each converted route accept 204/200 (`readEnvelope` already treats 204 as empty success).
- Test: `repos/handlers/create_test.go` (592 lines, oversize: not grown), `repos_test.go` (588, oversize: not grown); `repos_extra_test.go` (835; CI rejects the name) is deleted: its cases that still apply move to the test files named for `repos.go`'s split parts; likewise `project/project_import_coverage_test.go` (643) and `project_import_more_errors_test.go` move into the test files of `project_import.go`'s split parts and are deleted (`project_import_test.go` 1510 is not grown); `projects/handlers/projects_test.go`, `project/project_delete_test.go` (503, not grown: cases go to the split part's test file), black box `api/tests/regression_repo_add_failure_visible_test.go`

**Files (web):**
- Modify: `web/src/lib/api.ts` (`postRepo`, `postProject`, `deleteRepo`, `deleteProject` take `opId`, return `OperationReply`), `components/projects/add-repository-modal.tsx` (runs `repo.create`, `await whenSettled`, then the existing workspace read and sidebar seeding it does today, then navigates to the item's workspace; a `partial` failure whose repo item succeeded is not a failed add: it navigates to the repo home, and the failed branch is told by the operations watcher's toast and drawn as a failed item row under the repo (`rowsFromOperations`, `item.parentId` = the repo id) with its reason and Dismiss (V6); only a failed repo item re-enables the dialog), `components/projects/import-project-modal.tsx`, `components/layout/create-space-panel.tsx` (`project.create`, then the project from the store by the item id; all three use `awaitEntity` today), `components/layout/removal-commit.ts` (`releaseWhenGone` (`:93`) → `whenSettled` of the delete; a rejection restores the row and `report`s; `:76` reads `project.lastError` today and reads `selectEntityProblem(s, project.id)` instead; the `web.swallowed-promise` site at `:280` is a separate unload path (`flushDrainingRemovals`, `void sendRemoval(entry, { keepalive: true }).catch(() => {})`): it becomes `.catch((err: unknown) => report(err, { action: 'Remove' }))`, harmless if the page is gone, and the keepalive send carries the same `Idempotency-Key` as the visible removal so a delete both paths send is one operation), `lib/store/build-repo-tree.ts` (`:190-191`: `deleting` becomes `repo.deleting && selectEntityBusy(repo.id)` and `deleteError` becomes `selectEntityProblem(repo.id)`, read where the tree is built, instead of `RepoDTO.lastError`), `lib/types.ts`. The routes R2 turns into 204/200 need no web change: `stopChat`/`closeChat`/`switchToNative`/`renameChat`/`setSelection`/`deleteChat` (`agent-api.ts:1215-1300` at refresh) ignore the body and `readEnvelope` treats 204 as empty; only their Go handler tests change.
- Delete: `web/src/lib/ws/await-entity.ts` (its `:148` swallow goes with it) and `web/src/__tests__/lib/ws/await-entity.test.ts`
- Test: `components/projects/add-repository-modal.test.tsx`, `import-project-modal.test.tsx`, create `components/layout/create-space-panel.test.tsx` (none exists), `components/layout/removal-commit.test.ts`

**Interfaces:**
- Consumes: T3, T5.
- Produces: no `LastError` field remains in `api/internal` (`grep -rn 'LastError' api/internal --include='*.go'` prints nothing) and no `lastError` in `web/src` outside tests of deleted behaviour; `libs.WriteNoContent(c *gin.Context)`.

- [ ] **Step 1: Write the failing tests.**

`api/tests/regression_repo_add_failure_visible_test.go`:
```go
// TestRegression_RepoAddFailureIsVisible: adding a repository that cannot be set up reports
// why; the client used to wait 30 s for a repo that had been rolled back.
func TestRegression_RepoAddFailureIsVisible(t *testing.T)
```
`newImportFixture`; read main's workspace root, delete the repo (`DELETE …/repos/:r` with a key; `awaitOperation` succeeded); make the repo's `<slug>` directory (the parent of main's workspace root, which the re-add must create again) a regular file (ledger F2: a file at `<…>/main` alone is removed or bypassed by the re-add, so the test could not fail); add the same repo path again with a key. The repo itself is committed before the protected worktrees are provisioned (`project_import.go:533-536`), so the failure is partial, not a rollback. Assert 202 with `kind "repo.create"`; settled `failed`, `error.code == "partial"`; the `main` item failed with a message whose lower-cased text contains "not a directory"; the repo item succeeded and `GET …/repos` lists the repo with its repo-home workspace. At base it fails on the empty 202 body (no `data.operation`), and the swallowed `slog.Warn` is why. Confirm the test fails at base before implementing.

`project_delete_test.go`:
```go
func TestPendingDeletes_ListsDeletesARestartInterrupted(t *testing.T)
func TestDeleteRepo_AFailureIsReturnedNotSaved(t *testing.T)
func TestDeleteRepo_AStoppedDeleteIsMarkedAgain(t *testing.T) // a repo left Deleting=false after a failure is re-marked; a repo already deleting is left as is
```
`project_import` split part's test: `func TestImportRepo_AProtectedWorktreeFailureIsAFailedItem(t *testing.T)` (the repo is kept; `RepoProgress` gets the branch with its error). `app/container_test.go` (or the split part's): `func TestNew_ResumedDeletesSettleBeforeItReturns(t *testing.T)` (D5).
Handler tests: `TestCreate_AnswersWithTheOperation` (repos, projects), and each converted in-request handler's existing test now expects 204.

Web:
```ts
// add-repository-modal.test.tsx
it('adding waits on the operation, then opens the default workspace')
it('a failed add re-enables the dialog; the reason comes from the operation')
it('a partly failed add opens the repo home; the failed branch is drawn with its reason') // V6
// removal-commit.test.ts (5 vi.mock today: ends at ≤3, X3)
it('a remove releases the tray once the repo is gone')
it('a failed remove restores the row and reports the reason')
it('an unload-time remove that fails is reported')
// import-project-modal.test.tsx / create-space-panel.test.tsx
it('import waits on the operation and hands over the project from the store')
```

- [ ] **Step 2: Run them to verify they fail.**

```bash
GOTEST ./internal/app/usecases/project/... ./internal/api/v0/endpoints/repos/... ./internal/api/v0/endpoints/projects/...
GOBOX 'TestRegression_RepoAddFailureIsVisible'
cd web && bunx vitest run src/__tests__/components/projects src/__tests__/components/layout/create-space-panel.test.tsx src/__tests__/components/layout/removal-commit.test.ts
```

- [ ] **Step 3: Implement** as listed. Labels: "Add <repo name>", "Remove <repo name>", "Add <project name>", "Remove <project name>"; resumed deletes use the same labels.

- [ ] **Step 4: Run the tests, then wider.**

```bash
GOTEST ./internal/app/... ./internal/api/... ./internal/domain/... ./internal/
GOBOX 'TestRegression_(RepoAdd|RepoDelete|DeleteWorkAtRisk|DeletedRepo|RepoScoping|RepoProject|Unowned)|TestProjects_|TestHome_'
(cd api && go vet -tags 'integration noEmbed' ./tests/...)   # the kit's ShutdownDetached calls are gone
cd web && bunx vitest run src/__tests__/components/projects src/__tests__/components/layout src/__tests__/lib src/__tests__/features/agent/api && bun tsc --noEmit && bun run knip
GOLINT   # scripts/deadcode-check.sh confirms the two WaitAsync lines are gone
(cd api && go vet -tags 'integration noEmbed' ./internal/...)
node scripts/ratchet/file-size.mjs && node scripts/ratchet/counters.mjs
grep -rn 'LastError\|WriteAccepted' api/internal api/tests --include='*.go'; test $? -eq 1
```

- [ ] **Step 5: Commit**

```bash
git add <each file in this task's Files list that git status --short shows changed, by path>   # git rm the deleted ones by path
git commit -m "refactor: repos and projects are operations; lastError and the detached runners are gone

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01DG5r7UkKfKiDuXTdNTGTXK"
```

