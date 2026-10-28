# S3d: No swallowed failure, durable read models, no discarded error, gate. Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `web.empty-catch`, `web.swallowed-promise` and `go.discarded-errors` reach 0 and lint keeps them there; read-model projections stop logging and dropping save failures; S3's gate passes.

**Architecture:** See the index, `docs/plans/2026-10-17-s3-operations-errors-plan.md` (R1, R17, R19, R20, R21, D1, Gate baseline, "Status and dispatch"). T7 requires T1–T6; T8 requires nothing from S3 and runs now in lane B (OR2), its precondition met by asynx v1.1.0 (R21); T9–T11 are done; T12 requires T7 and T8. Go commands use the index's `GOTEST`/`GOBOX`/`GOLINT` forms.

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

Each site gets exactly one of three treatments. The lists below were re-derived on 2026-10-28 (`counters.mjs --list web.empty-catch` 33, `--list web.swallowed-promise` 27, `grep -rn 'bestEffort(' web/src` 13 calls in 9 files outside the helper); T1, T2, T5 and T6 remove some first (marked), so re-run the lists at dispatch:
- **(L) Local storage**: the catch was around `localStorage`/`sessionStorage`; use `readLocal`/`writeLocal`/`removeLocal`. Sites: `components/layout/sidebar-card-height.ts:46`, `components/layout/use-sidebar-panel.ts:41,52`, `features/console/lib/dock-prefs.ts:51`, `features/agent/composer/lib/prompt-stash-persistence.ts:146`, `features/terminal/lib/terminal-reconnect-map.ts:18`, `features/workspace/stores/workspace-persistence.ts:10`, `lib/persistence/retired-storage.ts:21`, `features/settings/lib/appearance-bootstrap.ts:128`.
- **(F) Defined fallback**: the failure has a documented default (a cache miss, an unparsable optional file, a font that is not installed). The catch returns that default explicitly (`return undefined`, `return []`), with a ≤1-line why. Sites: `lib/persistence/{cache-store.ts:28,entity-cache.ts:27,54,idb.ts:202,223}` (a cache read or write that fails is a miss; S4 deletes the cache), `features/file-explorer/file-explorer/lib/file-tree-gitignore.ts:67`, `features/terminal/utils/resolve-font.ts:151`, `components/sidebar/hooks/use-sidebar-drag.ts:481` (a `setPointerCapture` mid-`beginDrag`, `:470-482`, not a release: there is no default to return, and a `return` would abort the drag setup; extract `tryCapturePointer(el, id): boolean` whose catch returns false, and trim the 8-line comment to ≤3), `features/panes/components/prefetch-editor-chunks.ts:9,10` (`bestEffort(import(…))`: a prefetch that fails is a later load), `features/terminal/utils/terminal-file-drop.ts:16` (`decodeFileUrl`'s "not a URL → null"), `features/git/components/review-thread-item.tsx:445` (the edit handler already toasts; the catch keeps the editor open), `features/editor/components/html/html-preview-document.ts:113` (`load(reference).catch(() => null)`: a missing optional asset in a preview, which would toast once per image), `features/git/hooks/use-review-files-summary.ts:83` and `use-review-outline.ts:74` (polled background reads whose fallback is the last value and a later tick; a report would toast on every tick).
- **(R) Report**: the user asked for something and it failed. `catch (err) { report(err, { action: '<what the user did>' }) }` or `.catch((err: unknown) => report(err, { action, key }))`. Empty catches: `features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx:161,354,382` (610 lines, oversize: split by responsibility before editing, R15, and delete its `MAX_LINES_BASELINE` entry in `web/eslint.config.js`, X2), `use-file-explorer-inline-editing.ts:115`, `features/workspace/stores/hooks/use-workspace-threads-stream.ts:40`. Swallowed promises: `features/editor/lib/monaco-adapters.ts:101`, `features/editor/lsp/format-buffer.ts:25`, `features/editor/lsp/lsp-client.ts:182,271,278,295,332` (key `lsp:<languageId>`, so a dead server toasts once), `lib/api/documents.ts:65` (S5), `features/files/lib/file-tree-handlers.ts:69`, `features/git/hooks/use-current-identity.ts:47`, `features/keymaps/hooks/use-console-keyboard.ts:15,26`, `features/panes/lib/buffer-release.ts:82`, `features/workspace/stores/hooks/use-workspace-file-tree.ts:66,94`, `features/workspace/stores/hooks/use-workspace-git.ts:67`, `features/agent/composer/plate/attachments/excalidraw-preview.tsx:177`, `lib/boot.ts:61`, `lib/crowbar-bridge.ts:223`, `main.tsx:119`. `bestEffort` calls: `features/workspace/stores/hooks/use-workspace-file-tree.ts:74,110`, `features/panes/stores/slices/buffer-slice.ts:375`, `features/panes/components/editor-pane.tsx:143`, `features/panes/lib/buffer-release.ts:79,97`, `features/editor/lib/buffer-save.ts:101` and `features/editor/documents/draft-store.ts:43,73` (S5's draft writes: a failed draft write is reported once per document key).
- **Already handled by an earlier task:** `lib/api.ts:169,190` (T1 moves them into `lib/api/fetch.ts` as a handled fallback), `daemon-health-listener.tsx:92`'s `bestEffort` (T2 deletes the read), `components/layout/removal-commit.ts:280` and `lib/ws/await-entity.ts:148` (T6).
- **S2-owned (OR3):** `features/agent/hooks/use-prompt-queue.ts:275,374,460`, `features/agent/lib/prompt-queue-persistence.ts:178,206`, `features/workspace/stores/hooks/use-workspace-agent-chats-stream.ts:156,197`, `features/workspace/stores/slices/agent-chats-slice.ts:39`, `features/agent/components/agent-chat-pane.tsx:478`. If S2 has not deleted them when T7 runs, they are treated by the same rules here.
- Gone since planning (skip): `buffer-slice.ts:422,435`, `monaco-lsp-providers.ts:211`, `external-buffer-sync.ts:48,82`, `workspace-store-registry.ts:291` no longer appear in either list.

A site S2 deleted is skipped; a site that appeared since is classified by the same rules. A site whose reading shows another category is moved to it, and the commit body says so.

- [ ] **Step 1: Write the failing tests.** For each (R) unit that has a test file, one case in that file, named `'a failed <action> reports why'`, that makes the dependency reject and asserts the `report` toast (`toastManager.add` called with the action as `title`). Where the unit has no test file, none is created for this (`CLAUDE.md`: no test only for coverage). `local-storage.test.ts`:
```ts
it('reads absent and writes false when storage throws')
it('round-trips a value when storage works')
```

- [ ] **Step 2: Run them to verify they fail.** `cd web && bunx vitest run <the test files Step 1 touched> src/__tests__/lib/local-storage.test.ts`.

- [ ] **Step 3: Apply the treatments; delete `best-effort.ts`.**

- [ ] **Step 4: Lint rules.** `web/eslint.config.js` has no `src/**` object: the general one is `files: ['**/*.{ts,tsx}']` (`:125`), which also lints `src/__tests__`, where three of these patterns are legitimate test plumbing (`__tests__/features/panes/hooks/use-pane-keyboard.test.ts:504`, `__tests__/lib/api.test.ts:231`, `__tests__/features/editor/documents/restore-documents.test.ts:7`). Add a dedicated object placed right after it, `{ files: ['src/**/*.{ts,tsx}'], ignores: ['src/__tests__/**'], rules: { 'no-restricted-syntax': ['error', …] } }` (the counter excludes tests the same way; check no later object redefines `no-restricted-syntax` for those files), with:

```js
{ selector: 'CatchClause > BlockStatement[body.length=0]',
  message: 'An empty catch hides a failure: return the fallback, or report() it.' },
{ selector: "CallExpression[callee.property.name='catch'] > ArrowFunctionExpression.arguments[body.type='BlockStatement'][body.body.length=0]",
  message: '.catch(() => {}) hides a failure: report() it.' },
{ selector: "CallExpression[callee.property.name='catch'] > ArrowFunctionExpression.arguments[body.type='Literal'][body.raw='null']",
  message: '.catch(() => null) hides a failure: handle it or report() it.' },
{ selector: "CallExpression[callee.property.name='catch'] > ArrowFunctionExpression.arguments[body.operator='void'][body.argument.type='Literal']",
  message: '.catch(() => void 0) hides a failure: handle it or report() it.' },
{ selector: "CallExpression[callee.property.name='catch'] > ArrowFunctionExpression.arguments[body.type='Identifier'][body.name='undefined']",
  message: '.catch(() => undefined) hides a failure: handle it or report() it.' },
```

The type guards matter: esquery compares `String(getPath(node))`, so a bare `[body.name='undefined']` matches every arrow whose body has no `name` (including the prescribed `.catch((err) => report(err, …))`), and a bare `[body.operator='void']` matches `.catch((e) => void report(e))`. A comment-only catch body has `body.length=0`, so it fails too (the counter treats it the same way).

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

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01DG5r7UkKfKiDuXTdNTGTXK"
```

---

### Task 8: asynx v1 and durable read models

**Precondition (R21): met.** asynx v1.1.0 (tagged 2026-10-19) fixes NF1 (a `Get` racing `Forget` resurrects the aggregate through an unlocked auto-snapshot; Crowbar's Workspace upcaster takes that path), NF2 (one busy aggregate stalls every aggregate on its shard) and NF7 (recovery is quadratic per aggregate and `Build` blocks on it). The gate checks ran on `f7b6fcbbe`/`82e4c38`; v1.1.0 adds 6 commits past the PR #48 merge `e007f6ff3` that were not verified, hence Step 0. Do not work around a library defect in Crowbar: a defect found in Step 0 stops the task and is reported upstream. `go.mod` pins v0.8.1 at refresh. The ledger's reference port and bounded probes (old scratchpad `asynx48/`) are lost (OR5): port from v1.1.0's `store/storetest` contract.

**Pre-flight items held for the T8 lane** (T8 was in flight when the 2026-10-28 pre-flight was applied; only D6 and D9 were written into this contract): D7 (cite `registerStoreProjection`, chat `store.go:273-280`, node, reviewthread `:198-206`; the chat projector's placement watch must be idempotent under redelivery; the reconcile-path save error is returned, not logged), D10 (drop `ConcurrentAtomicallyKeepsVersionsUnique`, `OutOfOrderAppendsReadInVersionOrder` and `AtomicallyHonoursACancelledContext`: `storetest` covers them as `ConcurrentAppendsOfOneVersionAdmitOne`, `AppendsOutOfVersionOrderReadInOrder`, `CancelledContextFailsEveryOperation`), D11 (`AggregateIDs`' callers are the four reconcilers via `serialize/aggregate_lister.go:12`), D15 (pin with `go get github.com/char2cs/asynx@v1.1.0 && go mod tidy`; `go.sum` has no v1.1.0 lines). The T8 lane rules on them.

**Files:**
- Modify: `api/go.mod`, `api/go.sum` (`github.com/char2cs/asynx v1.1.0`)
- Modify: `api/internal/adapter/eventstore/sqlite/event_store.go` (178 lines; implement v1 `models.Store`: `ReadFrom`, `ReadRange`, `GetReceipt`, `PendingDeliveries`, `AckDelivery`, `Atomically`; delete the old `Append` (`:62`)/`Delete` (`:148`) methods and the unused `Count` (`:132`); keep `AggregateIDs` (`:118`) for reconcile-on-open, R20; `snapshot_store.go` and `adapter/container.go` are its other users), create `api/internal/adapter/eventstore/sqlite/tx.go` (`models.Tx`: `Append`, `DeleteStream`, `DropDeliveries`, `PutReceipt`, `EnqueueDelivery`, `Native() any` returning `*sql.Tx`), tables `receipts(command_id PRIMARY KEY, accepted, aggregate_id, event_id, version, rejection)` and `deliveries(seq INTEGER PRIMARY KEY AUTOINCREMENT, aggregate_id, version, UNIQUE(aggregate_id, version))` created at open in the same database as `events` (R12: new tables, no data rewritten; the `events` table and `events:{id}` stream ids are unchanged); WAL, one write connection plus a separate read pool (D9): reads (`ReadFrom`, `ReadRange`, `PendingDeliveries`, `GetReceipt`) go through the read pool and do not wait for an open transaction; every write (`Atomically`, `AckDelivery`, which deletes a `deliveries` row) goes through the single write connection, as the event logs do today (`event_store.go:46`, `sqlite.go:56-59`), because SQLite admits one writer and two writer connections would contend on the busy timeout
- Test: `api/internal/adapter/eventstore/sqlite/event_store_test.go` (conformance + Crowbar's cases), `tx_test.go`
- Modify: `api/internal/app/asynx.go` (42 lines; `newAsynx` adds `WithDeliveryErrorHandler` logging aggregate, event, version, attempt and error at `slog.Error`; the `WithPublishErrorHandler` at `:34` no longer promises a reconcile), `api/internal/app/container.go` (509 lines, oversize: split by responsibility before adding if T3 has not; open each read-model store before its asynx instance and pass its durable subscriber as a builder option)
- Modify (construction order, D6): durable subscribers must be on the builder before `Build()`, but today the asynx instances are built first (`app/container.go:66,77,88,226`) and the read-model stores later, deeper, with the built instance as an argument: `repositories.New` (`app/container.go:253`) → `chat/event_store.go:286` `store.New(storeDB, es, ax, watch, opts...)`, `node/event_store.go:155`, `reviewthread/reviewthread.go:148`, `workspace/workspace.go:297` → `workspace/internal/store/store.go:95` `projections.RegisterStore(st, ax)`. So each `store.New` splits into `Open(storeDB) (*Store, error)` (no ax) and `Reconcile(ctx, es, ax) error`; `app/container.go` opens the four view DBs and stores before `newAsynx` and passes `Subscriber(st)` as its `opts`; `repositories.New` and the four repository constructors take the opened `*Store` instead of `storeDB`.
- Modify: the read-model projections `api/internal/app/repositories/{workspace/internal/store/projections,chat/internal/store,reviewthread/internal/store,node/internal/store}/store.go` (337/363/254/333 lines). At refresh only the workspace projection has `RegisterStore(st, ax)` (`projections/store.go:278`, with `ax.OnForget` at `:286`); chat, node and reviewthread wire their projector through `New(...)` (`store.go:92/85/74`). Each becomes `func Subscriber(st *Store) func(*asynx.Builder[T])` registering `WithSubscriber(<the same pattern>, handler)`; the handler returns the save error and handles `models.ForgetEventName` by deleting the row (replacing the `OnForget`s, including `repositories/container.go:168`'s `axAgentChat.OnForget(c.forgetAgentWorking)` if its job is a read-model delete); `saveWithRetry` and `isTransientIOError` (four copies) are deleted. Hubs (`…/hub.go`) stay live `Subscribe` handlers.
- Forget-marker idempotency (R21 obligation): upstream redelivers a failed Forget marker at startup (`Recover`; measured 4–14% double delivery). Every `ForgetEventName` handler is a row delete that succeeds when the row is already gone, and the delete reactor's purge tolerates an already-forgotten aggregate; a test per projection delivers the marker twice.
- Modify: the OCC retry loops on `ErrPipelineFailed` in `api/internal/app/repositories/chat/event_store.go` (572 lines, oversize: the deletion must bring it under 500, else split), `workspace/workspace.go` (874, oversize: `sendWithOCC`; split if T4 has not), `reviewthread/reviewthread.go` (355), `node/event_store.go` (331) are deleted (v1 serializes commands per aggregate in one process), with their jitter constants; `libs/status.go` (or, after T1, `problem/foreign.go`) keeps mapping `ErrPipelineFailed` → Conflict
- Modify: the quiesce comments that become stale because v1's `WaitPublish` no longer refuses: `repositories/container.go:190–209` and `repositories/drain/gate.go:90`
- Modify: `api/internal/app/repositories/workspace/internal/reactors/delete.go` only if v1 refuses the purge's `Forget` with `ErrForgetFromHandler`: the purge is not part of the handler, so its context derives from the reactor's own root context (with the reactor timeout) instead of `context.WithoutCancel(handlerCtx)`
- Modify: every fake or wrapper of `asynx.Asynx[T]` gains `PausedAggregates` and `ResumeDeliveries` (compile errors list them); `cmd/crowbar-seed-chat/fixture.go` builds with the same options as `newAsynx`; the tests that seed a poison row through the old `es.Append` (`repositories/workspace/internal/store/rebuild_test.go:217`, `repositories/chat/internal/store/store_test.go:229,292`) write it through the v1 `Tx` instead
- Keep (ledger, item 5): `QueueDepth` 1000; `ErrQueueFull` answers 503 with no retry (today `occSend` translates it; after T1 the `problem/foreign.go` row does, so deleting the loops loses nothing; if T8 lands before T1, the loops' translation stays until T1). The lost port preview pinned the adapter to one connection; this task keeps one write connection and adds a read pool (`TestEventStore_ReadsDoNotWaitForAnOpenTransaction`), and R20 uses no sync projection either way.

**Interfaces:**
- Consumes: nothing from earlier S3 tasks.
- Produces: `func Subscriber(st *Store) func(*asynx.Builder[domain.Workspace])` (and the same for Chat, ReviewThread, Node).

- [ ] **Step 0: Disk and the tag's own conformance, before any Crowbar change.** `df -h "$(git rev-parse --show-toplevel)"` (under 8 GB free: stop and report). Pin `github.com/char2cs/asynx v1.1.0` (never another tag). Port the adapter far enough to run v1.1.0's `storetest.Run` and `RunSnapshots` against Crowbar's SQLite store, and run them (`GOTEST -race -count=3 ./internal/adapter/eventstore/sqlite/...`). Every case passes, or the task stops and reports the failing case upstream.

- [ ] **Step 1: Write the failing tests.**

`event_store_test.go`:
```go
func TestEventStore_Conformance(t *testing.T) { storetest.Run(t, func(t *testing.T) models.Store { return openTemp(t) }) }
func TestEventStore_ConcurrentAtomicallyKeepsVersionsUnique(t *testing.T) // 2 goroutines append the same (stream, version): exactly one commits, the other wraps ErrPipelineFailed
func TestEventStore_ReadsDoNotWaitForAnOpenTransaction(t *testing.T)    // ReadFrom, ReadRange, PendingDeliveries and GetReceipt return while another goroutine holds Atomically open (channels, no sleep); writes, AckDelivery included, are not asserted: they wait for the writer
func TestEventStore_AtomicallyHonoursACancelledContext(t *testing.T)
func TestEventStore_OutOfOrderAppendsReadInVersionOrder(t *testing.T)
func TestEventStore_OpensADatabaseWrittenBeforeV1(t *testing.T)         // a fixture events table with rows written the old way reads back unchanged
```
Each projection's `store_test.go` (extend):
```go
func TestWorkspaceProjection_ASaveFailureIsRetriedNotDropped(t *testing.T) // a gorm callback fails the first save; the row is saved by the next delivery attempt (short WithDeliveryRetry in the test builder)
func TestWorkspaceProjection_ForgetDeletesTheRow(t *testing.T)
func TestWorkspaceProjection_AForgetDeliveredTwiceIsANoOp(t *testing.T) // R21 obligation: the second marker returns nil and leaves no row
```
(and the same three for chat, reviewthread, node, in their own test files; node has no `store_test.go` yet: create it; `chat/event_store_test.go` 632 and `node/event_store_test.go` 501 are oversize, so the OCC-loop cases they hold are deleted, not rewritten in place).

- [ ] **Step 2: Run them to verify they fail.**

```bash
GOTEST ./internal/adapter/eventstore/... ./internal/app/repositories/...
```

- [ ] **Step 3: Implement** the adapter, the subscribers and the wiring. Delete the OCC loops and run the workspace and chat repository tests that exercised them; a test that asserted a retry now asserts the second command simply waits its turn.

- [ ] **Step 4: Run the tests, then wider.**

```bash
GOTEST ./internal/adapter/... ./internal/app/... ./internal/api/... ./cmd/...
GOTEST -race ./internal/adapter/eventstore/... ./internal/app/repositories/...   # targeted -race only
GOBOX 'TestRegression_|TestOperations_'   # the top-level ./tests package only; -timeout 40m
(cd api && go vet -tags 'integration noEmbed' ./tests/...)
GOLINT
node scripts/ratchet/file-size.mjs && node scripts/ratchet/counters.mjs   # go.timers falls with the OCC jitter
```

- [ ] **Step 5: Boot on a copy of real-shaped data.** Build the pre-T8 daemon (from the task's base commit) and the T8 daemon into the session scratchpad. Start the pre-T8 one headless with `CROWBAR_HOME` set to a fresh directory under the session scratchpad (never `~/.crowbar`; the socket path must start with that directory), fill it with one repo, one import and `go run -tags noEmbed ./cmd/crowbar-seed-chat --workspace-id <id> --turns 20 --tool-calls 5` (the Makefile's `seed-chat` body), stop it, then start the T8 daemon on the same home. Confirm: it opens, the repos and their workspaces are the same as before the upgrade (`GET` them over that socket before and after, diff), and the daemon log has no delivery errors.

- [ ] **Step 6: Commit**

```bash
git add <each file this task changed, by path>
git commit -m "chore(api): asynx v1.1.0; read models are durable subscribers

A failed read-model save is retried and parks instead of being logged and
dropped; the OCC retry loops go now that commands are serialized.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01DG5r7UkKfKiDuXTdNTGTXK"
```

---

### Tasks 9–11: Go, no discarded error (**done**)

Status at refresh (2026-10-28): T10 merged (`ab2f911dd`, `a0f3d0782`), T11a merged (`6b00bfeec`, `0f75948c9`), T11b merged (`cd0c77a9`: every remaining counter site including T9's area, the 21 counter-invisible engine sites, 93 test-file errcheck fixes, errcheck `check-blank: true` in `api/.golangci.yml:48` with a test-only source exemption at `:105–109`). `go.discarded-errors` is 0 in the baseline. T9 is absorbed: its three named REDs are covered (`cmd/crowbar/scope_test.go` `TestScope_ASetenvFailureIsReturned`, `git/handlers/stash_test.go` `TestStashPush_AMalformedBodyIs400AndAnEmptyOneKeepsTheDefaults`; `app/shutdown.go:69` already returns the `errors.Join` of the four `Shutdown`s), and its `handlers/ws.go:90` carry is fixed (`4547a851`). Nothing below is dispatched; it is kept as the record of R19's rules, which T1–T8 and T12 still follow for any new code. Carries left by T11b are in the index ("Carries folded in").

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

### Task 9: http, app, cmd, adapter, core gateway (**absorbed by T11b**)

**Files:** the `--list` lines under `api/internal/api/`, `api/internal/app/` (incl. `container.go`, `shutdown.go` (the six `_ = ax*.Shutdown` → `errors.Join` into the returned error), `realtime/`, `repositories/*/hub.go`, `usecases/`), `api/cmd/`, `api/internal/adapter/`, `api/internal/core/{gateway,selfinstall,shellenv}`.

- [ ] **Step 1: Failing tests** (named cases at planning time): `api/internal/app/shutdown_test.go` `TestShutdown_ReturnsEveryInstanceShutdownError` (a second `Shutdown` returns asynx's `ErrAlreadyShuttingDown` joined); `api/cmd/crowbar/scope_test.go` `TestScope_ASetenvFailureIsReturned` (a key containing `=`); the git handlers' stash split part `TestStashPush_AMalformedBodyIs400AndAnEmptyOneKeepsTheDefaults` (`write.go:391` and `:484` discard the bind error today; the body is optional, so `io.EOF` keeps the defaults and anything else is 400).
- [ ] **Step 2: Run them to verify they fail** (`GOTEST` on those packages).
- [ ] **Step 3: Apply R19** to every listed line in the area.
- [ ] **Step 4: Verify.** `node scripts/ratchet/counters.mjs --list go.discarded-errors | grep -E 'api/(internal/(api|app|adapter|core/(gateway|selfinstall|shellenv))|cmd)/'; test $? -eq 1`, then `GOTEST ./internal/api/... ./internal/app/... ./internal/adapter/... ./internal/core/... ./cmd/...`, `GOLINT`.
- [ ] **Step 5: Commit** `fix(api): no discarded errors in http, app, cmd and adapters` (paths by name; trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`).

### Task 10: terminal, fs, search, git engines (**done**)

**Files:** the `--list` lines under `api/internal/core/terminal/` (`transport.go`, `internal/persistence/persistence.go`, `internal/session/*`, `internal/model/*`), `api/internal/engine/{fs,search,git}/`.

- [ ] **Step 1: Failing tests:** `persistence_test.go` `TestWriteBuf_ARenameFailureRemovesTheTempAndReportsBoth` (destination is a directory); `engine/search/internal/replace/replace_test.go` `TestWriteAtomic_ACleanupFailureIsJoined` if reachable, else none; `engine/git/would_merge_conflict_test.go` `TestWouldMergeConflict_AGitFailureIsReturned` (a path that is not a repository).
- [ ] **Step 2–4:** as T9, with the area grep `api/internal/(core/terminal|engine/(fs|search|git))/` and `GOTEST ./internal/core/terminal/... ./internal/engine/fs/... ./internal/engine/search/... ./internal/engine/git/...`.
- [ ] **Step 5: Commit** `fix(api): no discarded errors in the terminal, fs, search and git engines`.

### Task 11: agents and lsp engines, test kit; errcheck keeps it at 0 (**done**, T11a + T11b)

**Files:** the `--list` lines under `api/internal/engine/agents/` (`descriptorcheck/*`, `internal/modeldiscovery/{cache,manifest}.go`, `internal/protocol/internal/{apidriver,descriptor,wsrpc}/*`, `internal/exec/runner.go`), `api/internal/engine/lsp/`, `api/tests/kit/` (compile only, never run: a kit helper with a `*testing.T` in scope uses `require.NoError(t, err)`; one without returns the error), `api/.golangci.yml` (`linters-settings.errcheck.check-blank: true`, a tightening).

- [ ] **Step 1: Failing tests:** `descriptor/v3_test.go` `TestDescriptorV3_AMalformedOverlayIsAnError`; `lsp/internal/convert/commands_test.go` `TestConvertCommands_AMalformedArgumentIsAnError`; `lsp/internal/server/server_requests_test.go` `TestServerRequests_AMalformedParamsIsAnError`.
- [ ] **Step 2–3:** as T9.
- [ ] **Step 4: Verify.**

```bash
test -z "$(node scripts/ratchet/counters.mjs --list go.discarded-errors)"
GOTEST ./internal/engine/agents/... ./internal/engine/lsp/...
(cd api && go vet -tags 'integration noEmbed' ./tests/...)   # compiles the kit; never runs it
(cd api && golangci-lint run --build-tags noEmbed --new-from-rev= ./... && scripts/deadcode-check.sh)
```
The backlog lint must report no `errcheck` issue now that `check-blank` is on.

- [ ] **Step 5: Commit** `fix(api): no discarded errors in the agent and lsp engines; errcheck checks blanks`.

---

### Task 12: Gate

**Files:**
- Modify: `scripts/ratchet/counters-baseline.json`, `scripts/ratchet/loc-budgets-baseline.json`, `scripts/ratchet/file-size-baseline.json` (written by `--update`, lower only), `api/deadcode-baseline.txt` and `web/knip.json` (only entries S3 fixed and an earlier task missed, deleted; the three `Handlers.WaitAsync` lines leave in T4 and T6, the commits that delete those files, because `deadcode-check.sh` fails on a stale entry)
- Modify (carry from T11b): `api/.golangci.yml` drops the stray blank line at `:110` before `paths:` (whitespace only; nothing loosened)

- [ ] **Step 1: Disk.** `df -h "$(git rev-parse --show-toplevel)"`; under 8 GB free, stop and report.
- [ ] **Step 2: Ratchet.**

```bash
for g in file-size counters loc-budgets; do node scripts/ratchet/$g.mjs --update; done   # must report only shrinks; growth is a failure to fix, never to update
node --test scripts/ratchet/*.test.mjs
for g in file-size counters loc-budgets; do node scripts/ratchet/$g.mjs; done
node -e "const b=require('./scripts/ratchet/counters-baseline.json');for(const k of ['go.discarded-errors','web.empty-catch','web.swallowed-promise']){console.log(k,b[k]);if(b[k]!==0)process.exit(1)}"
```

`loc-budgets` was red before S3 (S1 R7): `--update` never raises a budget; report S3's per-module delta in the commit body and leave any key that is still over its budget red, for the user's R7 ruling.

- [ ] **Step 3: Nothing deleted came back.** This is a check, not where sites get fixed: T3–T6 own every site the grep could still find (comments and assertions in `projections/hub.go`, `tests/harness_test.go`, `tests/regressions_test.go`, `tests/fixtures_test.go`, `tests/kit/env.go`, `tests/integration/...`, `hierarchy/import.go:85`, `tests/regression_import_branches_test.go:424`). A hit here is a missed site of the task that owns it. "best-effort" appears in prose comments in 93 files, so the grep names the module and the helper, not the phrase.

```bash
grep -rn 'pending-creates\|reparent-settle\|lib/best-effort\|bestEffort\|await-entity\|StatusAndMessage\|WriteErrCode\|WriteAccepted\|BeginWork\|EndWork\|broadcastLastError\|runAsync\|WaitAsync\|ShutdownDetached\|LastError' api/internal api/tests web/src --include='*.go' --include='*.ts' --include='*.tsx'; test $? -eq 1
grep -rn 'lastError' web/src --include='*.ts' --include='*.tsx'; test $? -eq 1
grep -rln 'app/apperr' api --include='*.go'; test $? -eq 1
```

- [ ] **Step 4: Checks CI runs** (targeted tests only, never the full suites):

```bash
cd web && bunx eslint . --max-warnings 0 && bun run knip && bun tsc --noEmit && bunx prettier --check . && bun run lint:baselines && bun run lint:doctor
cd web && bunx vitest run src/__tests__/lib src/__tests__/components/sidebar src/__tests__/components/layout src/__tests__/components/projects src/__tests__/features/git src/__tests__/features/window
GOTEST ./internal/domain/... ./internal/app/... ./internal/api/... ./internal/adapter/... ./internal/
GOBOX 'TestRegression_|TestOperations_'   # -timeout 40m, no -race
cd api && golangci-lint run --build-tags noEmbed ./... && golangci-lint run --build-tags noEmbed --new-from-rev= ./... && scripts/deadcode-check.sh && go vet -tags noEmbed ./... && go vet -tags 'integration noEmbed' ./tests/...
```
CI also runs `bun run test:coverage` and the Go coverage floor over the full suites; S3 does not run them locally (user rule). If either job fails, add behaviour tests; never lower thresholds.

- [ ] **Step 5: Live check** (OR6: the noVNC container; one dev instance; window visible). Resolve `CROWBAR_HOME` under the worktree's git-ignored `/.crowbar/` and refuse to continue unless the socket path starts with the worktree path; never touch `~/.crowbar/crowbar.sock` or `~/.crowbar/state`; no provider CLI. In a scratch git repo under the session scratchpad with a bare `origin` holding branches `bad/x`, `good` and `main`:
  1. Add the repo (dialog waits, then opens `main`). Remove it and add it again: both through the dialog/tray, no timeout.
  2. Create a regular file `bad` beside `main`'s workspace root; import `bad/x` and `good`: two rows at once; `good` lands; `bad/x` shows "failed" with git's "Not a directory" and Dismiss; one toast.
  3. Restart the dev daemon: the failed row and its reason are still there.
  4. Move `origin` aside and push from `good`: the row spins, then a toast "Push good failed" with the reason; the workspace row shows the reason in its tooltip until a later push succeeds (move `origin` back, push again).
  5. Drag `good` under itself is refused by the drop policy; drag it under `main`'s sibling (if the PR graph allows) and confirm the reparent runs as an operation (spinner, then the row moves).
  6. Stop the dev daemon, start it with a pending migration step if the dev catalog offers one, else call `reportMigrating` from the browser console: the "Crowbar is upgrading its data · Step i of n" toast, then "Reconnected". Then `reportDaemonRefusal({code:'daemon_halted', message:'test'})`: the halt screen with the reason (F1).
  7. Screenshots of each step go to the session scratchpad; the report lists each outcome and T8's status. The user repeats steps 1–6 in the desktop app at the end of the branch (OR6).
- [ ] **Step 6: Commit** the lowered baselines and the `.golangci.yml` whitespace only.

```bash
git add scripts/ratchet/counters-baseline.json scripts/ratchet/loc-budgets-baseline.json scripts/ratchet/file-size-baseline.json api/.golangci.yml   # plus api/deadcode-baseline.txt web/knip.json when changed
git commit -m "chore(ratchet): S3 gate: discarded errors, empty catches and swallowed promises at 0

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01DG5r7UkKfKiDuXTdNTGTXK"
```

**Acceptance:** the three counters are 0 and lint enforces them (ESLint `no-restricted-syntax`, errcheck `check-blank`); every `TestRegression_*` S3 added is green (`RefusedCreateCarriesItsCode`, `GitPushFailureIsVisible`, `ReparentFailureIsVisible`, `BranchImportFailureIsVisible`, `BranchImportReasonSurvivesRestart`, `BranchImportRetryReturnsTheSameOperation`, `ImportRefusesATakenChatID`, `RepoAddFailureIsVisible`); the web tests showing a failed import's reason are green; the live check's steps are reported with screenshots; T8 is done (asynx v1.1.0 pinned, Forget-marker handling idempotent).
