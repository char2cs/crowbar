# S3d: No swallowed failure, durable read models, no discarded error, gate. Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `web.empty-catch`, `web.swallowed-promise` and `go.discarded-errors` reach 0 and lint keeps them there; read-model projections stop logging and dropping save failures; S3's gate passes.

**Architecture:** See the index, `docs/plans/2026-10-17-s3-operations-errors-plan.md` (R1, R17, R19, R20, R21, D1, Gate baseline). Requires T1–T6. T8 has an external precondition (R21) and no later task uses its code.

**Tech Stack:** ESLint flat config (`no-restricted-syntax`), golangci-lint (errcheck), asynx v1 (`WithSubscriber`, `models.Store`/`Tx`, `store/storetest`), gorm + SQLite.

**Spec:** architecture §6; §11 row S3 gate.

---

### Task 7: Web, no swallowed failure

**Files:**
- Delete: `web/src/lib/best-effort.ts`, `web/src/__tests__/lib/best-effort.test.ts`
- Create: `web/src/lib/local-storage.ts` (`readLocal(key: string): string | null`, `writeLocal(key: string, value: string): boolean`, `removeLocal(key: string): void`: storage that throws (private mode, blocked site data) reads as absent and writes as not written; this is the defined fallback for per-viewer conveniences, not a swallow), test `web/src/__tests__/lib/local-storage.test.ts`
- Modify: every site `node scripts/ratchet/counters.mjs --list web.empty-catch` and `--list web.swallowed-promise` print after T6, every `bestEffort(` call, and `web/eslint.config.js`
- Test: the existing test file of each unit whose failure becomes a `report()` (below)

**Interfaces:**
- Consumes: T2 `report`, `problemOf`.
- Produces: the lint rules below; `readLocal`/`writeLocal`/`removeLocal`.

Each site gets exactly one of three treatments:
- **(L) Local storage**: the catch was around `localStorage`/`sessionStorage`; use `readLocal`/`writeLocal`/`removeLocal`. Planning-time sites: `components/layout/sidebar-card-height.ts:46`, `components/layout/use-sidebar-panel.ts:41,52`, `features/console/lib/dock-prefs.ts:51`, `features/agent/composer/lib/prompt-stash-persistence.ts:146`, `features/terminal/lib/terminal-reconnect-map.ts:18`, `features/workspace/stores/workspace-persistence.ts:10`, `lib/persistence/retired-storage.ts:21`, `features/settings/lib/appearance-bootstrap.ts:128`.
- **(F) Defined fallback**: the failure has a documented default (a cache miss, an unparsable optional file, a font that is not installed). The catch returns that default explicitly (`return undefined`, `return []`), with a ≤1-line why. Planning-time sites: `lib/persistence/{cache-store.ts:28,entity-cache.ts:27,54,idb.ts:198,219}` (a cache read or write that fails is a miss; S4 deletes the cache), `features/file-explorer/file-explorer/lib/file-tree-gitignore.ts:67`, `features/terminal/utils/resolve-font.ts:151`, `components/sidebar/hooks/use-sidebar-drag.ts:481` (confirm it is a pointer-capture release).
- **(R) Report**: the user asked for something and it failed. `catch (err) { report(err, { action: '<what the user did>' }) }` or `.catch((err: unknown) => report(err, { action, key }))`. Planning-time sites: `features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:160,362,390`, `use-file-explorer-inline-editing.ts:115`, `features/git/components/review-thread-item.tsx:445`, `features/git/hooks/use-review-files-summary.ts:83`, `use-review-outline.ts:74`, `features/panes/stores/slices/buffer-slice.ts:422,435`, `features/terminal/utils/terminal-file-drop.ts:16`, `features/workspace/stores/hooks/use-workspace-threads-stream.ts:40`, `features/editor/components/html/html-preview-document.ts:85`, `features/editor/lib/monaco-adapters.ts:101`, `features/editor/lsp/format-buffer.ts:23`, `features/editor/lsp/lsp-client.ts:182,271,278,295,332` and `monaco-lsp-providers.ts:211` (key `lsp:<languageId>`, so a dead server toasts once), `features/files/lib/file-tree-handlers.ts:69`, `features/git/hooks/use-current-identity.ts:47`, `features/keymaps/hooks/use-console-keyboard.ts:15,26`, `features/panes/components/editor-pane.tsx:117`, `features/panes/lib/buffer-release.ts:69,72,85`, `features/workspace/lib/external-buffer-sync.ts:48,82`, `features/workspace/stores/hooks/use-workspace-file-tree.ts:62,70,90,106`, `features/workspace/stores/hooks/use-workspace-git.ts:67`, `features/workspace/stores/workspace-store-registry.ts:291`, `features/panes/components/prefetch-editor-chunks.ts:9,10`, `features/agent/composer/plate/attachments/excalidraw-preview.tsx:177`, `lib/boot.ts:61`, `lib/crowbar-bridge.ts:223`, `main.tsx:119`.

A site S1 or S2 deleted is skipped; a site that appeared since is classified by the same rules. A site whose reading shows another category is moved to it, and the commit body says so.

- [ ] **Step 1: Write the failing tests.** For each (R) unit that has a test file, one case in that file, named `'a failed <action> reports why'`, that makes the dependency reject and asserts the `report` toast (`toastManager.add` called with the action as `title`). Where the unit has no test file, none is created for this (`CLAUDE.md`: no test only for coverage). `local-storage.test.ts`:
```ts
it('reads absent and writes false when storage throws')
it('round-trips a value when storage works')
```

- [ ] **Step 2: Run them to verify they fail.** `cd web && bunx vitest run <the test files Step 1 touched> src/__tests__/lib/local-storage.test.ts`.

- [ ] **Step 3: Apply the treatments; delete `best-effort.ts`.**

- [ ] **Step 4: Lint rules.** In `web/eslint.config.js`, add to the `no-restricted-syntax` rule of the config object that lints `src/**` (create the rule there if absent; never a separate object that a later one overrides):

```js
{ selector: 'CatchClause > BlockStatement[body.length=0]',
  message: 'An empty catch hides a failure: return the fallback, or report() it.' },
{ selector: "CallExpression[callee.property.name='catch'] > ArrowFunctionExpression.arguments[body.type='BlockStatement'][body.body.length=0]",
  message: '.catch(() => {}) hides a failure: report() it.' },
{ selector: "CallExpression[callee.property.name='catch'] > ArrowFunctionExpression.arguments[body.type='Literal'][body.raw='null']",
  message: '.catch(() => null) hides a failure: handle it or report() it.' },
{ selector: "CallExpression[callee.property.name='catch'] > ArrowFunctionExpression.arguments[body.operator='void']",
  message: '.catch(() => void 0) hides a failure: handle it or report() it.' },
{ selector: "CallExpression[callee.property.name='catch'] > ArrowFunctionExpression.arguments[body.name='undefined']",
  message: '.catch(() => undefined) hides a failure: handle it or report() it.' },
```

A comment-only catch body has `body.length=0`, so it fails too (the counter treats it the same way).

- [ ] **Step 5: Verify.**

```bash
node scripts/ratchet/counters.mjs --list web.empty-catch; test -z "$(node scripts/ratchet/counters.mjs --list web.empty-catch)"
test -z "$(node scripts/ratchet/counters.mjs --list web.swallowed-promise)"
cd web && bunx eslint . --max-warnings 0 && bun run knip && bun tsc --noEmit && bunx prettier --check . && bun run lint:baselines && bun run lint:doctor
cd web && bunx vitest run <the test files Step 1 touched> src/__tests__/lib
```
Each `test -z` exits non-zero if a site is left; nothing echoes a pass.

- [ ] **Step 6: Commit**

```bash
git add <each file this task changed, by path>   # git rm web/src/lib/best-effort.ts web/src/__tests__/lib/best-effort.test.ts
git commit -m "refactor(web): no empty catch, no swallowed promise, no bestEffort

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: asynx v1 and durable read models

**Precondition (R21):** the user has named an asynx release after v1.0.0 that fixes NF1 (a `Get` racing `Forget` resurrects the aggregate through an unlocked auto-snapshot; Crowbar's Workspace upcaster takes that path), NF2 (one busy aggregate stalls every aggregate on its shard) and NF7 (recovery is quadratic per aggregate and `Build` blocks on it), and whose release passes those probes. If no release is named, skip this task, do T9–T11, and report T8 as blocked at T12. Do not work around a library defect in Crowbar.

**Files:**
- Modify: `api/go.mod`, `api/go.sum` (`github.com/char2cs/asynx` at the named tag)
- Modify: `api/internal/adapter/eventstore/sqlite/event_store.go` (implement v1 `models.Store`: `ReadFrom`, `ReadRange`, `GetReceipt`, `PendingDeliveries`, `AckDelivery`, `Atomically`; delete the old `Append`/`Delete` methods and the unused `Count`; keep `AggregateIDs` for reconcile-on-open, R20), create `api/internal/adapter/eventstore/sqlite/tx.go` (`models.Tx`: `Append`, `DeleteStream`, `DropDeliveries`, `PutReceipt`, `EnqueueDelivery`, `Native() any` returning `*sql.Tx`), tables `receipts(command_id PRIMARY KEY, accepted, aggregate_id, event_id, version, rejection)` and `deliveries(seq INTEGER PRIMARY KEY AUTOINCREMENT, aggregate_id, version, UNIQUE(aggregate_id, version))` created at open in the same database as `events` (R12: new tables, no data rewritten; the `events` table and `events:{id}` stream ids are unchanged); WAL and more than one connection (reads and `AckDelivery` run outside the open transaction)
- Test: `api/internal/adapter/eventstore/sqlite/event_store_test.go` (conformance + Crowbar's cases), `tx_test.go`
- Modify: `api/internal/app/asynx.go` (`newAsynx` adds `WithDeliveryErrorHandler` logging aggregate, event, version, attempt and error at `slog.Error`; the publish handler's message no longer promises a reconcile), `api/internal/app/container.go` (open each read-model store before its asynx instance and pass its durable subscriber as a builder option)
- Modify: the read-model projections `api/internal/app/repositories/{workspace/internal/store/projections,chat/internal/store,reviewthread/internal/store,node/internal/store}/store.go`: `RegisterStore(st, ax)` becomes `func Subscriber(st *Store) func(*asynx.Builder[T])` registering `WithSubscriber(<the same pattern>, handler)`; the handler returns the save error and handles `models.ForgetEventName` by deleting the row (replacing their `OnForget`); `saveWithRetry` and `isTransientIOError` are deleted. Hubs (`…/hub.go`) stay live `Subscribe` handlers.
- Modify: the OCC retry loops on `ErrPipelineFailed` in `api/internal/app/repositories/chat/event_store.go`, `workspace/workspace.go`, `reviewthread/reviewthread.go`, `node/event_store.go` are deleted (v1 serializes commands per aggregate in one process), with their jitter constants
- Modify: `api/internal/app/repositories/workspace/internal/reactors/delete.go` only if v1 refuses the purge's `Forget` with `ErrForgetFromHandler`: the purge is not part of the handler, so its context derives from the reactor's own root context (with the reactor timeout) instead of `context.WithoutCancel(handlerCtx)`
- Modify: every fake or wrapper of `asynx.Asynx[T]` gains `PausedAggregates` and `ResumeDeliveries` (compile errors list them); `cmd/crowbar-seed-chat/fixture.go` builds with the same options as `newAsynx`

**Interfaces:**
- Consumes: nothing from earlier S3 tasks.
- Produces: `func Subscriber(st *Store) func(*asynx.Builder[domain.Workspace])` (and the same for Chat, ReviewThread, Node).

- [ ] **Step 0: Disk and precondition.** `df -h /System/Volumes/Data` (under 8 GB free: stop and report). Confirm the tag with the user's message; never pick one.

- [ ] **Step 1: Write the failing tests.**

`event_store_test.go`:
```go
func TestEventStore_Conformance(t *testing.T) { storetest.Run(t, func(t *testing.T) models.Store { return openTemp(t) }) }
func TestEventStore_ConcurrentAtomicallyKeepsVersionsUnique(t *testing.T) // 2 goroutines append the same (stream, version): exactly one commits, the other wraps ErrPipelineFailed
func TestEventStore_ReadsDoNotWaitForAnOpenTransaction(t *testing.T)    // ReadFrom and AckDelivery return while another goroutine holds Atomically open (channels, no sleep)
func TestEventStore_AtomicallyHonoursACancelledContext(t *testing.T)
func TestEventStore_OutOfOrderAppendsReadInVersionOrder(t *testing.T)
func TestEventStore_OpensADatabaseWrittenBeforeV1(t *testing.T)         // a fixture events table with rows written the old way reads back unchanged
```
Each projection's `store_test.go` (extend):
```go
func TestWorkspaceProjection_ASaveFailureIsRetriedNotDropped(t *testing.T) // a gorm callback fails the first save; the row is saved by the next delivery attempt (short WithDeliveryRetry in the test builder)
func TestWorkspaceProjection_ForgetDeletesTheRow(t *testing.T)
```
(and the same two for chat, reviewthread, node, in their own test files).

- [ ] **Step 2: Run them to verify they fail.**

```bash
GOTEST ./internal/adapter/eventstore/... ./internal/app/repositories/...
```

- [ ] **Step 3: Implement** the adapter, the subscribers and the wiring. Delete the OCC loops and run the workspace and chat repository tests that exercised them; a test that asserted a retry now asserts the second command simply waits its turn.

- [ ] **Step 4: Run the tests, then wider.**

```bash
GOTEST ./internal/adapter/... ./internal/app/... ./internal/api/... ./cmd/...
# black box: -run 'TestRegression_|TestOperations_' ./tests/   (the top-level ./tests package only)
(cd api && go vet -tags noEmbed ./... && go vet -tags 'integration noEmbed' ./tests/...)
make lint && make deadcode
```

- [ ] **Step 5: Boot on a copy of real-shaped data.** Build the daemon into the session scratchpad; start it with `CROWBAR_HOME` set to a fresh directory under the session scratchpad that `make seed-chat` and one import have filled while running the pre-T8 binary (built from `HEAD~1` into the scratchpad). Never point it at `~/.crowbar`. Confirm: it opens, the sidebar's rows are the same as before the upgrade (`GET` the repos and their workspaces before and after, diff), and the daemon log has no delivery errors.

- [ ] **Step 6: Commit**

```bash
git add <each file this task changed, by path>
git commit -m "chore(api): asynx <tag>; read models are durable subscribers

A failed read-model save is retried and parks instead of being logged and
dropped; the OCC retry loops go now that commands are serialized.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Tasks 9–11: Go, no discarded error

One task per area, same shape. Each runs `node scripts/ratchet/counters.mjs --list go.discarded-errors` first and treats every line in its area by R19. Planning-time counts (after S1 deletes `usecases/chat/internal/runner`, `cmd/crowbar/hook_answer.go` and the hook relay): T9 ≈ 40, T10 ≈ 54, T11 ≈ 44.

R19, as code:
- an error the function can return → returned, wrapped `fmt.Errorf("<op>: %w", err)`;
- cleanup after a failure (`os.Remove(tmp)`, `f.Close()` on the failure path) → `errors.Join(failure, cleanupErr)`;
- `Close` on a successful read path → joined into the function's result with a named return;
- a teardown with no caller (a connection's `defer Close` in a pump goroutine, a watcher's backend) → `if err := x.Close(); err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, os.ErrClosed) { slog.Warn(...) }`;
- a step documented as not failing the operation (directory fsync after a completed rename) → `slog.Warn` with the error, then continue;
- `hash.Hash.Write`, `fmt.Fprintf` into a `hash.Hash` or `strings.Builder` (never fail) → bare call;
- process signalling where "already gone" is success → `if err := p.Signal(...); err != nil && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) { return … }`;
- `json.Unmarshal`/`yaml.Unmarshal` → handled: returned, or the documented fallback with its why;
- `_ = ctx` and other silencers → deleted;
- a gin response write after headers → `c.Error(err)` (gin's error list, which the logging middleware reports);
- boot work with no caller (`app/container.go` model warmup, `RestorePersistedSessions`, `ProviderSync.SyncFromState`) → `slog.Warn` with the error.

Tests: where a changed function's new failure is reachable through its public API, its test file gets one case named `Test<Func>_<Failure>IsReturned` (or `…IsLogged`); otherwise none (`CLAUDE.md`: no test only for coverage).

### Task 9: http, app, cmd, adapter, core gateway

**Files:** the `--list` lines under `api/internal/api/`, `api/internal/app/` (incl. `container.go`, `shutdown.go` (the six `_ = ax*.Shutdown` → `errors.Join` into the returned error), `realtime/`, `repositories/*/hub.go`, `usecases/`), `api/cmd/`, `api/internal/adapter/`, `api/internal/core/{gateway,selfinstall,shellenv}`.

- [ ] **Step 1: Failing tests** (named cases at planning time): `api/internal/app/shutdown_test.go` `TestShutdown_ReturnsEveryInstanceShutdownError` (a second `Shutdown` returns asynx's `ErrAlreadyShuttingDown` joined); `api/cmd/crowbar/scope_test.go` `TestScope_ASetenvFailureIsReturned` (a key containing `=`); the git handlers' stash split part `TestStashPush_AMalformedBodyIs400AndAnEmptyOneKeepsTheDefaults` (`write.go:391` and `:484` discard the bind error today; the body is optional, so `io.EOF` keeps the defaults and anything else is 400).
- [ ] **Step 2: Run them to verify they fail** (`GOTEST` on those packages).
- [ ] **Step 3: Apply R19** to every listed line in the area.
- [ ] **Step 4: Verify.** `node scripts/ratchet/counters.mjs --list go.discarded-errors | grep -E 'api/(internal/(api|app|adapter|core/(gateway|selfinstall|shellenv))|cmd)/'; test $? -eq 1`, then `GOTEST ./internal/api/... ./internal/app/... ./internal/adapter/... ./internal/core/... ./cmd/...`, `make lint`, `make deadcode`.
- [ ] **Step 5: Commit** `fix(api): no discarded errors in http, app, cmd and adapters` (paths by name; trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`).

### Task 10: terminal, fs, search, git engines

**Files:** the `--list` lines under `api/internal/core/terminal/` (`transport.go`, `internal/persistence/persistence.go`, `internal/session/*`, `internal/model/*`), `api/internal/engine/{fs,search,git}/`.

- [ ] **Step 1: Failing tests:** `persistence_test.go` `TestWriteBuf_ARenameFailureRemovesTheTempAndReportsBoth` (destination is a directory); `engine/search/internal/replace/replace_test.go` `TestWriteAtomic_ACleanupFailureIsJoined` if reachable, else none; `engine/git/would_merge_conflict_test.go` `TestWouldMergeConflict_AGitFailureIsReturned` (a path that is not a repository).
- [ ] **Step 2–4:** as T9, with the area grep `api/internal/(core/terminal|engine/(fs|search|git))/` and `GOTEST ./internal/core/terminal/... ./internal/engine/fs/... ./internal/engine/search/... ./internal/engine/git/...`.
- [ ] **Step 5: Commit** `fix(api): no discarded errors in the terminal, fs, search and git engines`.

### Task 11: agents and lsp engines, test kit; errcheck keeps it at 0

**Files:** the `--list` lines under `api/internal/engine/agents/` (`descriptorcheck/*`, `internal/modeldiscovery/{cache,manifest}.go`, `internal/protocol/internal/{apidriver,descriptor,wsrpc}/*`, `internal/exec/runner.go`), `api/internal/engine/lsp/`, `api/tests/kit/` (compile only, never run: a kit helper with a `*testing.T` in scope uses `require.NoError(t, err)`; one without returns the error), `api/.golangci.yml` (`linters-settings.errcheck.check-blank: true`, a tightening).

- [ ] **Step 1: Failing tests:** `descriptor/v3_test.go` `TestDescriptorV3_AMalformedOverlayIsAnError`; `lsp/internal/convert/commands_test.go` `TestConvertCommands_AMalformedArgumentIsAnError`; `lsp/internal/server/server_requests_test.go` `TestServerRequests_AMalformedParamsIsAnError`.
- [ ] **Step 2–3:** as T9.
- [ ] **Step 4: Verify.**

```bash
test -z "$(node scripts/ratchet/counters.mjs --list go.discarded-errors)"
GOTEST ./internal/engine/agents/... ./internal/engine/lsp/...
(cd api && go vet -tags 'integration noEmbed' ./tests/...)   # compiles the kit; never runs it
make lint && make lint-backlog && make deadcode
```
`make lint-backlog` must report no `errcheck` issue now that `check-blank` is on.

- [ ] **Step 5: Commit** `fix(api): no discarded errors in the agent and lsp engines; errcheck checks blanks`.

---

### Task 12: Gate

**Files:**
- Modify: `scripts/ratchet/counters-baseline.json`, `scripts/ratchet/loc-budgets-baseline.json`, `scripts/ratchet/file-size-baseline.json` (written by `--update`, lower only), `api/deadcode-baseline.txt` and `web/knip.json` (only entries S3 fixed, deleted)

- [ ] **Step 1: Disk.** `df -h /System/Volumes/Data`; under 8 GB free, stop and report.
- [ ] **Step 2: Ratchet.**

```bash
make ratchet ARGS=--update      # must report only shrinks; growth is a failure to fix, never to update
make ratchet
node -e "const b=require('./scripts/ratchet/counters-baseline.json');for(const k of ['go.discarded-errors','web.empty-catch','web.swallowed-promise']){console.log(k,b[k]);if(b[k]!==0)process.exit(1)}"
```

- [ ] **Step 3: Nothing deleted came back.**

```bash
grep -rn 'pending-creates\|reparent-settle\|best-effort\|await-entity\|StatusAndMessage\|WriteErrCode\|BeginWork\|EndWork\|broadcastLastError\|runAsync\|WaitAsync\|LastError' api/internal web/src --include='*.go' --include='*.ts' --include='*.tsx'; test $? -eq 1
grep -rn 'lastError' web/src --include='*.ts' --include='*.tsx'; test $? -eq 1
```

- [ ] **Step 4: Checks CI runs** (targeted tests only, never the full suites):

```bash
cd web && bunx eslint . --max-warnings 0 && bun run knip && bun tsc --noEmit && bunx prettier --check . && bun run lint:baselines && bun run lint:doctor
cd web && bunx vitest run src/__tests__/lib src/__tests__/components/sidebar src/__tests__/components/layout src/__tests__/components/projects src/__tests__/features/git src/__tests__/features/window
GOTEST ./internal/domain/... ./internal/app/... ./internal/api/... ./internal/adapter/... ./internal/
# black box: -run 'TestRegression_|TestOperations_' ./tests/
make lint && make lint-backlog && make deadcode && (cd api && go vet -tags noEmbed ./... && go vet -tags 'integration noEmbed' ./tests/...)
```
CI also runs `bun run test:coverage` over the full suite; S3 does not run it locally (user rule). If that job fails, add behaviour tests; never lower thresholds.

- [ ] **Step 5: Live check in Tauri** (`make dev-desktop`; one dev instance; window visible). Resolve `CROWBAR_HOME` as the Makefile does and refuse to continue unless the socket path starts with the worktree path; never touch `~/.crowbar/crowbar.sock` or `~/.crowbar/state`. In a scratch git repo under the session scratchpad with a bare `origin` holding branches `bad/x`, `good` and `main`:
  1. Add the repo (dialog waits, then opens `main`). Remove it and add it again: both through the dialog/tray, no timeout.
  2. Create a regular file `bad` beside `main`'s workspace root; import `bad/x` and `good`: two rows at once; `good` lands; `bad/x` shows "failed · not a directory" and Dismiss; one toast.
  3. Restart the dev daemon: the failed row and its reason are still there.
  4. Move `origin` aside and push from `good`: the row spins, then a toast "Push good failed" with the reason; the workspace row shows the reason in its tooltip until a later push succeeds (move `origin` back, push again).
  5. Drag `good` under itself is refused by the drop policy; drag it under `main`'s sibling (if the PR graph allows) and confirm the reparent runs as an operation (spinner, then the row moves).
  6. Stop the dev daemon, start it with a pending migration step if the dev catalog offers one, else drive `reportDaemonState` through `mcp__tauri__webview_execute_js`: the "Crowbar is upgrading its data · Step i of n" toast, then "Reconnected".
  7. Screenshots of each step go to the session scratchpad; the report lists each outcome, and T8's status (done, or blocked on the asynx release).
- [ ] **Step 6: Commit** the lowered baselines only.

```bash
git add scripts/ratchet/counters-baseline.json scripts/ratchet/loc-budgets-baseline.json scripts/ratchet/file-size-baseline.json
git commit -m "chore(ratchet): S3 gate: discarded errors, empty catches and swallowed promises at 0

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

**Acceptance:** the three counters are 0 and lint enforces them (ESLint `no-restricted-syntax`, errcheck `check-blank`); every `TestRegression_*` S3 added is green (`RefusedCreateCarriesItsCode`, `GitPushFailureIsVisible`, `ReparentFailureIsVisible`, `BranchImportFailureIsVisible`, `BranchImportReasonSurvivesRestart`, `BranchImportRetryReturnsTheSameOperation`, `ImportRefusesATakenChatID`, `RepoAddFailureIsVisible`); the web tests showing a failed import's reason are green; the live check's steps are reported with screenshots; T8 is done or reported blocked on R21.
