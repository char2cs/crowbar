# S3c: Creates and imports are drawn from operations; repos and projects. Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the user's complaint ("Sometimes Crowbar cannot import/create a branch nor says why it failed"): every branch import and create is an operation whose rows the sidebar draws until the entities arrive, each failure carries its reason, and nothing waits for a row to land. Then do the same for repos and projects and delete the last lossy channel.

**Architecture:** See the index, `docs/plans/2026-10-17-s3-operations-errors-plan.md` (R3, R4, R8–R13, R15, D5, Review Focus 1, 2, 4). Requires T1–T4.

**Tech Stack:** Go, gin, asynx (workspace upcaster), testify, the `api/tests` harness; React 19, zustand, TanStack Router, vitest + Testing Library.

**Spec:** architecture §7 row "Branch import/create fails with no reason", §8 "Operations are rows", §11 row S3 gate "Import/create failure visible in tests".

---

### Task 5: Creates and imports are drawn from operations

**Files (Go):**
- Modify: `api/internal/api/v0/endpoints/worktree/handlers/import.go` (body `{branches: [{name, chatId}], lock}`; validate each `chatId` is a UUID (400) and unused (409 `id_conflict`) before 202; items at accept: one per branch `{Ref:{chat, chatId}, Label: name, State: running}`)
- Modify: `api/internal/app/usecases/workspace/internal/hierarchy/import.go` and `api/internal/app/usecases/workspace` (exported `ImportInput`): `Branches []ImportBranch` with `type ImportBranch struct{ Name, ChatID string }`, `Lock bool`; `CreateFromImport(ctx context.Context, in ImportInput, p ImportProgress) error` with `type ImportProgress interface{ Branch(o ImportOutcome) }` and `type ImportOutcome struct{ Branch, ChatID, WorkspaceID string; Err error }`. The requested branch's chat uses `ChatID`; a branch that already has a workspace reports that workspace's owning chat id; an ancestor added from the PR graph reports its daemon-minted ids; a failed branch's placeholder takes `ChatID` for its owning chat and the outcome carries the cause; if the placeholder also fails, the outcome carries `errors.Join(cause, placeholderErr)`. `Lock` locks each imported workspace in the same work (this replaces the client's `armImportLockWatch`).
- Modify: `api/internal/app/usecases/chat/internal/tree/imported_chat.go` and the chat create path (`MintChat`, `MintOwningChat`, `api/internal/api/v0/endpoints/chat/handlers/chats.go` create, own-worktree create, single import): take the client's chat `id` (UUID, unused, else 400/409); the own-worktree create answers `{id, workspaceId}`
- Delete: `Workspace.LastError` (`api/internal/domain/workspace.go`), `api/internal/app/repositories/workspace/internal/commands/set_last_error.go` (+ test), `SetLastError` on the workspace repository and its interface, `WorkspaceDTO.LastError`, `ChatWorktreeDTO.LastError` and their mapping
- Modify: `api/internal/app/repositories/workspace/upcast.go` (R13: `SchemaVersion = 3`; `StripRetiredPlacementFields` → `StripRetiredFields`, also dropping `/lastError`), `api/internal/app/container.go` (register it for 1 and 2)
- Test: `api/internal/app/usecases/workspace/internal/hierarchy/import_test.go`, `api/internal/app/repositories/workspace/upcast_test.go`, `api/internal/api/v0/endpoints/worktree/handlers/import_test.go` (the test file named for `import.go`; create it if cases live elsewhere today), black box `api/tests/regression_import_failure_visible_test.go`

**Files (web):**
- Create: `web/src/lib/store/create-draft.ts` (the sidebar's naming input: `{ draft: CreateDraft | null; start(d: CreateDraft): void; cancel(): void }`, `CreateDraft = { kind: 'branch' | 'chat'; projectId: string; repoId?: string; workspaceId: string; parentId: string; order: number; ownsWorktree: boolean }`), `web/src/components/sidebar/lib/rows-from-operations.ts`, `web/src/components/sidebar/operation-sidebar-row.tsx` (split out of `sidebar-row.tsx`, replaces `PendingSidebarRow`)
- Modify: `web/src/components/layout/create-actions.ts`, `thread-create.ts`, `home-actions.ts` (creates through `runOperation`; branch create opens its chat after `whenSettled`, D5; thread creates open at once, as today), `web/src/components/sidebar/lib/row-actions.ts` (`performImportBranches` through `runOperation`, `lock` sent; split under 500, R15), `web/src/components/sidebar/lib/rows-from-repo.ts` (no `lastError`; split under 500, R15), `components/sidebar/space-scroller.tsx`, `components/layout/sidebar-tree-surface.tsx`, `components/sidebar/lib/drop-actions.ts` (`rowsFromOperations` instead of `rowsFromPending`/`hideRowsForInFlightCreates`), `components/sidebar/sidebar-row.tsx`, `sidebar-row-actions.tsx` (reason from `selectEntityProblem`), `components/sidebar/types/sidebar-row.ts` (`pending?` → `operation?: { opId: string; state: OperationState; problem: Problem | null }`), `lib/workspace/placeholder.ts` (`placeholderReason(ws, problem)`), `lib/api.ts` (`importBranches(projectId, repoId, branches: {name: string; chatId: string}[], lock: boolean, opId: string): Promise<OperationReply>`; drop `lastError` mapping), `features/agent/api/agent-api.ts` (`createChat`, `createChatWithOwnWorktree` take `id` and `opId`), `lib/types.ts`, `lib/store/sidebar.ts`, `lib/store/build-repo-tree.ts` (workspace `lastError` gone), `components/layout/ide-shell.tsx` (unmount the deleted watcher), `features/panes/hooks/use-pane-keyboard.ts` (passes a minted id)
- Delete: `web/src/lib/store/pending-creates.ts`, `web/src/components/sidebar/lib/rows-from-pending.ts`, `web/src/components/layout/placeholder-toast-watcher.tsx`, `rowRepoScope` (`rows-from-repo.ts`), `panelRowsAtClick`, `failCreate`, `untilLanded`, `ROW_ARRIVAL_TIMEOUT_MS`, `waitForRow`, `chatHasLanded`, `forkHasLanded`, `waitForHomeTree`, `waitForHomeChat`, `waitForRootHomeChat`, `importedBranchHasLanded`, `waitForImportedBranch`, `armImportLockWatch`, `IMPORT_LOCK_SETTLE_TIMEOUT_MS`, and their tests (`pending-creates.test.ts`, `rows-from-pending.test.ts`, the `create-actions-pending-order`, `-branch-row`, `-chatless`, `-owner-not-listed` scenario files: cases that still describe behaviour move into `create-actions.test.ts`, which ends ≤500 lines; `space-scroller-create-round-trip.test.tsx`)
- Test: create `web/src/__tests__/lib/store/create-draft.test.ts`, `web/src/__tests__/components/sidebar/lib/rows-from-operations.test.ts`, `web/src/__tests__/components/sidebar/operation-sidebar-row.test.tsx`; rewrite `components/layout/{create-actions,thread-create,home-actions}.test.ts` (each ≤500); the split parts of `row-actions` get their own test files named for them

**Interfaces:**
- Consumes: T3 `operation.Service`, `Progress`, `runOperation`, `whenSettled`, `selectEntityProblem`, `isEntityPresent`, `dismissOperation`; T2 `report`.
- Produces: `rowsFromOperations(ops: readonly Operation[], scope: { projectId: string; repoId?: string }): SidebarRow[]`, one row per item that is not present, placed as the last child of `item.parentId ?? op.subject.id`, `id` = `item.ref.id`. A running or accepted item, or a succeeded one whose entity has not arrived → a creating row (spinner, label). A failed item, or an item of an `interrupted` operation, whose entity is absent → a failed row (glyph, "failed", the problem message, Dismiss → `dismissOperation`). A dismissed operation draws nothing. A failed item whose entity exists draws nothing: the entity row shows the reason through `selectEntityProblem` (Review Focus 4).

- [ ] **Step 1: Write the failing black-box tests.** `api/tests/regression_import_failure_visible_test.go`, sharing one setup helper `importWithOneBadBranch(t, h) (fixture, opID, badChatID, goodChatID)`: `newImportFixture`; the seed clone pushes branches `bad/x` and `good` to origin; the test creates a regular file named `bad` in the directory that holds main's workspace root (`filepath.Dir(worktreepath.WorkspaceRoot(<main worktree path>))`), so `git worktree add` for `bad/x` fails with "not a directory"; POST `import-batch` with `Idempotency-Key: <opID>` and `{branches:[{name:"bad/x",chatId:<A>},{name:"good",chatId:<B>}]}`.

```go
// TestRegression_BranchImportFailureIsVisible: an import whose branch cannot be checked out
// says which branch failed and why; the other branch lands.
func TestRegression_BranchImportFailureIsVisible(t *testing.T)
// 202, data.operation.id == opID, kind "branch.import", two running items.
// Settled: state failed, error.code "partial"; item "bad/x" failed, message contains "not a directory";
// item "good" succeeded with ref.id == B; B's workspace has a worktree path; A is a placeholder's owning chat.

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
func TestCreateFromImport_APlaceholderTakesTheClientChatID(t *testing.T)
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
GOTEST ./internal/app/usecases/workspace/... ./internal/app/repositories/workspace/... ./internal/api/v0/endpoints/worktree/...
# black box: -run 'TestRegression_(BranchImport|ImportRefuses)' ./tests/
cd web && bunx vitest run src/__tests__/components/sidebar src/__tests__/components/layout src/__tests__/lib/store
```

- [ ] **Step 5: Implement the Go side.** The import handler's work adapts `ImportOutcome` to `Progress.Item` (`Err == nil` → `succeeded`, else `failed` with `problem.Of(Err)`; ref `{chat, ChatID}`, label the branch name; T3's `Item` replaces by label; a failed item fails the operation as `partial`, R5). Delete `LastError` and `set_last_error` everywhere `go build` points; the import placeholder no longer sets it.

- [ ] **Step 6: Implement the web side.** Rows come from `rowsFromOperations` merged after the entity rows; the naming row comes from `create-draft`. Placeholder reason: held-by path from the entity, problem from `selectEntityProblem(row.id)`.

- [ ] **Step 7: Run all of the above, then wider.**

```bash
GOTEST ./internal/app/... ./internal/api/... ./internal/domain/...
# black box: -run 'TestRegression_(BranchImport|ImportRefuses|Import|CreateOwnWorktree|SidebarCreate|ChatFirst|Unprovisioned|Promote)' ./tests/
cd web && bunx vitest run src/__tests__/components/sidebar src/__tests__/components/layout src/__tests__/lib src/__tests__/features/panes/hooks && bun tsc --noEmit
make lint && make deadcode
```
Expected: PASS; knip clean (every deleted export gone with its last importer).

- [ ] **Step 8: Live check** (`make dev-desktop`, isolated `CROWBAR_HOME`, socket path under the worktree or stop). In a scratch repo with an `origin` holding `bad/x` and `good`, create the file `bad` beside the default branch's workspace root, import both from the sidebar: two rows appear at once; `good` lands; `bad/x` shows "failed" with "not a directory" and Dismiss; a toast names it once. Restart the dev daemon: the reason is still on the row. Screenshots to the session scratchpad.

- [ ] **Step 9: Commit**

```bash
git add <each file in this task's Files list that git status --short shows changed, by path>   # git rm the deleted ones by path
git commit -m "fix: a branch import or create that fails says which and why

Imports and creates are operations; the sidebar draws their rows until the
entities arrive and keeps a failed row with its reason. pending-creates and
every landing watcher are gone.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


---

### Task 6: Repos and projects are operations; the last lossy channel goes

**Files (Go):**
- Modify: `api/internal/api/v0/endpoints/projects/handlers/projects.go` (Create → `project.create`, subject and single item `{project, <id minted at accept>}`, label "Add <name>"; Delete → `project.delete`, subject and item the project, label "Remove <name>"; `BeginDelete` and its broadcast stay before the 202), `api/internal/api/v0/endpoints/repos/handlers/repos.go` (Create → `repo.create`, subject `{project, p}`, item `{repo, <id minted at accept>}`; the work adds `{workspace, <default ws id>}` when it exists; Delete → `repo.delete`; split under 500, R15; delete the ignored `createRequest.ID`)
- Modify: `api/internal/app/usecases/project/project_import.go` (`Create` and `ImportRepo` take the id minted at accept and report the default workspace through a `RepoProgress interface{ Workspace(id, branch string) }`; split under 500), `api/internal/app/usecases/project/project_delete.go` (a failed delete returns its error instead of saving `LastError`; `Resume` becomes `PendingDeletes(ctx) ([]PendingDelete, error)` with `type PendingDelete struct{ Kind domain.OperationKind; Ref domain.EntityRef; ProjectID, Label string; Run func(context.Context) error }`; split under 500)
- Modify: `api/internal/app/container.go` (boot: for each `PendingDelete`, `operations.Start` with a daemon-minted id, replacing the slog-only `Resume` call at `container.go:191`)
- Delete: `Repository.LastError`, `Project.LastError` (`api/internal/domain/{repository,project}.go`), `RepoDTO.LastError`, `ProjectDTO.LastError`; `projects/handlers/async.go`, `repos/handlers/async.go` and every `WaitAsync` (tests wait on the recorded operation); `api/internal/api/v0/endpoints/internal/detached/`; `ShutdownDetached` (`api/internal/api/v0/container.go:108`, `api/internal/api/container.go:68`, `api/internal/internal.go:373`, `api/tests/kit/env.go:261,311`)
- Modify (R2): every remaining `libs.WriteAccepted` whose handler finishes in the request answers `libs.WriteNoContent(c)` (new: `c.Status(http.StatusNoContent)`): `grep -rn 'WriteAccepted' api/internal --include='*.go' | grep -v _test` lists them (chat rename, selection, delete, stop, close, switch-to-native, abandon answer, terminal kill session at planning time). `WriteAccepted` stays only on routes whose outcome is a ledger row (prompt submission, compact); its doc comment says so.
- Test: `repos/handlers/create_test.go`, `repos_test.go` (absorb `repos_extra_test.go` cases that still apply; CI rejects `*_extra_test.go` names going forward and this task deletes it), `projects/handlers/projects_test.go`, `project/project_delete_test.go`, `project/project_import_test.go`, black box `api/tests/regression_repo_add_failure_visible_test.go`

**Files (web):**
- Modify: `web/src/lib/api.ts` (`postRepo`, `postProject`, `deleteRepo`, `deleteProject` take `opId`, return `OperationReply`), `components/projects/add-repository-modal.tsx` (runs `repo.create`, `await whenSettled`, then the existing workspace read and sidebar seeding it does today, then navigates to the item's workspace), `components/projects/import-project-modal.tsx`, `components/layout/create-space-panel.tsx` (`project.create`, then the project from the store by the item id), `components/layout/removal-commit.ts` (`releaseWhenGone` → `whenSettled` of the delete; a rejection restores the row and `report`s), `lib/store/build-repo-tree.ts` (`deleteError` from `selectEntityProblem` at the reader instead of `RepoDTO.lastError`), `lib/types.ts`
- Delete: `web/src/lib/ws/await-entity.ts` and `web/src/__tests__/lib/ws/await-entity.test.ts`
- Test: `components/projects/add-repository-modal.test.tsx`, `import-project-modal.test.tsx`, `components/layout/create-space-panel.test.tsx`, `components/layout/removal-commit.test.ts`

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
`newImportFixture`; read main's workspace root, delete the repo (`DELETE …/repos/:r` with a key; `awaitOperation` succeeded); create a regular file at the path of main's workspace root (`<…>/main`); add the same repo path again with a key. Assert 202 with `kind "repo.create"`; settled `failed`, message contains "not a directory"; `GET …/repos` does not list it.

`project_delete_test.go`:
```go
func TestPendingDeletes_ListsDeletesARestartInterrupted(t *testing.T)
func TestDeleteRepo_AFailureIsReturnedNotSaved(t *testing.T)
```
Handler tests: `TestCreate_AnswersWithTheOperation` (repos, projects), and each converted in-request handler's existing test now expects 204.

Web:
```ts
// add-repository-modal.test.tsx
it('adding waits on the operation, then opens the default workspace')
it('a failed add re-enables the dialog; the reason comes from the operation')
// removal-commit.test.ts
it('a remove releases the tray once the repo is gone')
it('a failed remove restores the row and reports the reason')
// import-project-modal.test.tsx / create-space-panel.test.tsx
it('import waits on the operation and hands over the project from the store')
```

- [ ] **Step 2: Run them to verify they fail.**

```bash
GOTEST ./internal/app/usecases/project/... ./internal/api/v0/endpoints/repos/... ./internal/api/v0/endpoints/projects/...
# black box: -run 'TestRegression_RepoAddFailureIsVisible' ./tests/
cd web && bunx vitest run src/__tests__/components/projects src/__tests__/components/layout/create-space-panel.test.tsx src/__tests__/components/layout/removal-commit.test.ts
```

- [ ] **Step 3: Implement** as listed. Labels: "Add <repo name>", "Remove <repo name>", "Add <project name>", "Remove <project name>"; resumed deletes use the same labels.

- [ ] **Step 4: Run the tests, then wider.**

```bash
GOTEST ./internal/app/... ./internal/api/... ./internal/domain/... ./internal/
# black box: -run 'TestRegression_(RepoAdd|RepoDelete|DeleteWorkAtRisk|DeletedRepo|RepoScoping|RepoProject|Unowned)|TestProjects_|TestHome_' ./tests/
(cd api && go vet -tags 'integration noEmbed' ./tests/...)
cd web && bunx vitest run src/__tests__/components/projects src/__tests__/components/layout src/__tests__/lib && bun tsc --noEmit
make lint && make deadcode
```

- [ ] **Step 5: Commit**

```bash
git add <each file in this task's Files list that git status --short shows changed, by path>   # git rm the deleted ones by path
git commit -m "refactor: repos and projects are operations; lastError and the detached runners are gone

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

