# S4b: One stream. Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `/v0/stream` exists end to end (daemon hub, frames, scopes, resources; client socket, scope holds, entity store) and carries operations, every workspace-scoped stream and the chat ledger. The operations stream, five stream definitions, `realtime` and the per-chat feed socket are deleted.

**Architecture:** See the index, `docs/plans/2026-10-17-s4-stream-store-tree-plan.md` (R5–R9, R16, R20, Shared vocabulary). The hub is the one publisher: one mutex, one version counter per epoch, one bounded queue per connection, one refcount per scope. A source per scope kind answers `sub`. The client's `stream.ts` owns the socket and routes frames to the store that owns each scope kind, which registers itself (core never imports a feature).

**Tech Stack:** Go, gin, gorilla/websocket, testify, `api/tests` black box; zustand, vitest + Testing Library, `wsManager` (`web/src/lib/ws/manager.ts`).

**Spec:** architecture §4 "The one stream", §3 "Daemon memory: watchers, LSP hosts, forge polls, refcounted by subscription", §8 (delete `realtime` lifecycles, `v0/ws`).

---

### Task 5: The one stream, with operations

**Files:**
- Create: `api/internal/domain/stream/{scope.go,frame.go}`; tests `scope_test.go`, `frame_test.go` with `testdata/frames.json` (one example of every frame type, the JSON the web decoder test copies).
- Create: `api/internal/app/stream/{hub.go,conn.go,refcount.go,projects_source.go}`; tests `hub_test.go`, `conn_test.go`, `refcount_test.go`, `projects_source_test.go`.
- Create: `api/internal/api/v0/endpoints/stream/{routes.go,handlers/handlers.go}` (`GET /v0/stream`: upgrade with the same origin allow-list as `terminal/handlers/ws.go`'s upgrader, wrap the connection as `stream.Conn`, `hub.Serve`); test `handlers/handlers_test.go`.
- Create: `api/internal/api/v0/stream_wire.go` (`func EncodeEntity(kind stream.EntityKind, v any) (json.RawMessage, error)`; T5 knows `operation`, encoded exactly as S3's frames carried it; an unknown kind is an error), test `stream_wire_test.go`.
- Modify: `api/internal/app/container.go` (mint `epoch := uuid.NewString()` once; build the hub with the `projects` source and the `Encoder` it now takes as a constructor argument; expose the hub to the API container), `api/internal/internal.go` (the composition root: `app.New(ctx, s.engines, s.adapter, post, v0.EncodeEntity)`), `api/internal/app/shutdown.go` (`hub.Close()` after the operations service shuts down), `api/internal/app/usecases/operation/service.go` (its `publish` is `func(op domain.Operation) { hub.Entity(stream.Projects, stream.KindOperation, op.ID, op) }`), `api/internal/api/v0/router.go`, `api/internal/api/v0/container.go` (or the stream-defs file S3 split out).
- Delete: `operationsDef`, its `Broadcaster`, the WebSocket half of `endpoints/operations` (`DELETE /v0/operations/:id` stays), `hub.BroadcastOperation`/`PushOperation`.
- Modify: `scripts/ratchet/modules.json` (`go.stream` += `api/internal/app/stream`, `api/internal/api/v0/endpoints/stream`; `web.core` += `web/src/core`), moving no budget (new code; R1).
- Test (black box): create `api/tests/stream_client_test.go` (helper), `api/tests/stream_test.go`; port every test that dials `/v0/operations` (`grep -rln '/v0/operations' api/tests`) to the helper, deleting S3's `awaitOperation` dialer in favour of `awaitOperation(t, c *streamClient, id string, done func(domain.Operation) bool)`.
- Web create: `web/src/core/stream/{frames.ts,stream.ts}`, `web/src/core/entities/entity-store.ts`; tests `web/src/__tests__/core/stream/{frames.test.ts,stream.test.ts}`, `web/src/__tests__/core/entities/entity-store.test.ts`.
- Web modify: `web/src/main.tsx` (`startStream()` replaces `startOperationsStream()`), `web/src/lib/store/operations.ts` (server operations are read from `entityStore` kind `operation`; the store keeps only local operations, R11 of S3; `applyOperationFrame` goes), `web/src/lib/store/run-operation.ts` (reads both), their tests.
- Web delete: `web/src/lib/store/operations-stream.ts` and its test.

**Interfaces:**
- Produces (Go): `domain/stream` and `app/stream` as in the index. `stream.Projects` (the `projects` scope value), `stream.KindOperation`. `(*Hub).Serve` reads `Request`s and writes `Frame`s until the connection or `ctx` ends; it releases every scope the connection held when it returns.
- Produces (web):

```ts
// core/stream/stream.ts
export interface ScopeKindHandler {
  since?(key: string): number                       // chat scopes (T7)
  frame(f: Frame): void                              // every frame of a scope of this kind
  epoch?(epoch: string): void                        // a new daemon epoch
}
export function registerScopeKind(kind: string, h: ScopeKindHandler): () => void
export function holdScope(scope: string): () => void
export function onSignal<T>(scope: string, kind: string, cb: (data: T) => void): () => void
export function startStream(): () => void
// core/entities/entity-store.ts registers itself for 'projects' (and 'ws' in T6) at module load
export type EntityKind = 'project' | 'repo' | 'workspace' | 'chat' | 'tree' | 'operation' | 'review_thread' | 'telemetry'
export interface EntityData { operation: Operation } // T6 and T10 add kinds
```
- The `operation` kind: `applyEntityFrame` for kind `operation` also calls `forgetLocalOperation(id)` (`lib/store/operations.ts`), so a server operation always replaces the local one with its id.

- [ ] **Step 1: Write the failing Go tests.**

`scope_test.go`: `TestParseScope_AcceptsEveryKind` (`projects`, `chat:c1`, `ws:w1`, `git:w1`, `files:w1`, `lsp:w1`), `TestParseScope_RefusesAnUnknownKindOrAMissingKey` (`problem.CodeOf == InvalidArgument`). `frame_test.go`: `TestFrame_EveryTypeMarshalsAsTheGoldenFile`.

`hub_test.go` (a fake `Conn` over channels; fake sources):
```go
func TestHub_FirstFrameIsReadyWithTheEpoch(t *testing.T)
func TestHub_SnapshotThenSyncedThenLiveFrames(t *testing.T)
func TestHub_LiveFramesNeverPrecedeOrUndercutTheSnapshot(t *testing.T) // 50 concurrent Entity calls while a sub snapshots: for each id, v strictly increases across snapshot and live frames
func TestHub_AnEntityFrameReachesOnlyConnectionsHoldingItsScope(t *testing.T)
func TestHub_AFullQueueClosesTheConnectionWithResync(t *testing.T)   // writer blocked; QueueSize+1 frames → CloseWith(4000, …)
func TestHub_AnUnknownScopeIsClosedRefusedAndTheConnectionLives(t *testing.T)
func TestHub_UnsubStopsFramesForThatScope(t *testing.T)
func TestHub_ReturningReleasesEveryHeldScope(t *testing.T)
```
`refcount_test.go`: `TestRefcount_StartsOnFirstHoldAndStopsOnLast` (two connections hold `git:w1`; one `Start`, one `Stop` after both release), `TestRefcount_AScopeWithoutResourcesIsFree`.

`api/tests/stream_test.go` (black box, via the helper: `dialStream(t, h) *streamClient`, `(c).Sub(t, scope string, since int64)`, `(c).Unsub(t, scope)`, `(c).Next(t, match func(stream.Frame) bool) stream.Frame` using the harness's existing reader deadline, no sleep):
```go
func TestStream_FirstFrameIsReady(t *testing.T)
func TestStream_ReadyCarriesANewEpochAfterRestart(t *testing.T)            // Review Focus 3: h.shutdown(), newHarnessAt on the same home, a new dial's epoch differs
func TestStream_ProjectsScopeSnapshotsOperations(t *testing.T)            // a settled operation from before the dial arrives as an entity frame before synced{projects}
func TestStream_AnOperationFrameFollowsItsStateChanges(t *testing.T)      // push failure (S3's fixture): accepted, running, failed in increasing v
func TestStream_AnUnknownScopeIsRefusedWithAProblem(t *testing.T)
```

- [ ] **Step 2: Run them to verify they fail.** `GOTEST ./internal/domain/stream/... ./internal/app/stream/... ./internal/api/v0/endpoints/stream/...`; `GOBOX 'TestStream_|TestOperations_|TestRegression_GitPush'`. Expected: compile failure.

- [ ] **Step 3: Implement the daemon side.** `Hub` holds `mu`, `seq uint64`, the connections, and per scope the set of connections and a refcount. `Entity` takes `mu`, increments `seq`, marshals once, enqueues to every connection holding the scope (non-blocking; a full queue closes that connection with 4000). `sub` takes `mu`, asks the scope's source to `Attach` (an entity source emits its frames, each stamped with the current `seq`), enqueues `synced`, registers the connection, increments the refcount (0→1 runs each `Resource.Start(key)`), releases `mu`. A ledger source (T7) attaches without `mu`. `unsub` and connection end decrement (1→0 runs `Stop`). The writer goroutine drains the queue to `Conn.WriteJSON`; the reader parses `Request`s. Re-point the operations service's publish; delete the operations stream def, its route's WebSocket half and the hub pair; port the black-box dialers.

- [ ] **Step 4: Run the Go tests.** Step 2's commands; then `GOTEST ./internal/app/... ./internal/api/...`, the Go gates, `go vet -tags 'integration noEmbed' ./tests/...`. Expected: PASS.

- [ ] **Step 5: Write the failing web tests.** `stream.test.ts` mocks `@/lib/ws/manager` with a fake that records `send` and exposes the `subscribe` callback (one `vi.mock`):
```ts
it('sends sub on the first hold of a scope and unsub on the last release')
it('re-sends sub for every held scope after a reconnect, with since from the kind handler')
it('a closed resync frame resubscribes that scope')
it('a closed refused frame reports its problem once and drops the scope')
it('an undecodable frame is reported and its scope resubscribed')
it('a ready frame with a new epoch reaches every registered kind before any entity frame')
it('a resubscribe snapshot removes an entity deleted while offline')   // Review Focus 2
it('onSignal delivers only its scope and kind')
```
`frames.test.ts`: `it('decodes every frame type')` (literals copied from `testdata/frames.json`), `it('rejects an unknown t')`, `it('rejects an unknown field')`.
`entity-store.test.ts`:
```ts
it('applies a newer version and ignores an older one')
it('an equal version keeps the stored object identity')
it('a gone frame removes the entity')
it('a new epoch replaces every entity even at a lower version')        // Review Focus 3
it('synced prunes the members of a scope not seen since its sub')
it('an operation frame forgets the local operation with its id')
```
Extend `web/src/__tests__/lib/store/operations.test.ts` and `run-operation.test.ts`: the S3 cases that fed server frames now feed `applyEntityFrame`; their assertions are unchanged.

- [ ] **Step 6: Run them to verify they fail**, then implement, then run: `cd web && bunx vitest run src/__tests__/core src/__tests__/lib/store/operations.test.ts src/__tests__/lib/store/run-operation.test.ts`, `bun tsc --noEmit`, and the web gates. Expected: PASS.

- [ ] **Step 7: Commit.**

```bash
git add api/internal/domain/stream api/internal/app/stream api/internal/api/v0/endpoints/stream web/src/core <each modified path> scripts/ratchet/modules.json
git rm web/src/lib/store/operations-stream.ts web/src/__tests__/lib/store/operations-stream.test.ts <each deleted path>
git commit -m "feat(stream): one /v0/stream with versioned entity frames; operations move onto it

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: Workspace scopes; `realtime` goes

**Files:**
- Create: `api/internal/app/stream/resources/{watcher.go,lsp.go,forge_poll.go,origin_sync.go}` (each `Resource`), moved from `api/internal/app/realtime/{watcher_manager.go,watcher_factory.go,linger_timer.go,lsp_manager.go,provider_poll_manager.go,origin_sync_manager.go}`: keep each start/stop body, its injected clock and timer factory, its `pollTimeout`; delete each manager's refcount map (the hub's refcount replaces all four). Tests move with them (`watcher_test.go`, `lsp_test.go`, `forge_poll_test.go`, `origin_sync_test.go`), their manager-refcount cases deleted.
- Create: `api/internal/app/stream/{ws_source.go,git_source.go,files_source.go,lsp_source.go}` and their tests: each ports its old `StreamDef.Snapshot` (`threadsDef` → review-thread entities of the workspace; `gitDef` → the current git status as one signal; `filesDef` → none; `lspDef` → current diagnostics as signals).
- Modify: the publishers: the watcher dispatcher (`Signal(git:<ws>, "git_status", ev)`, `Signal(files:<ws>, "file_change", ev)`), the LSP diagnostics publisher (`Signal(lsp:<ws>, "diagnostics", ev)`), `api/internal/app/repositories/reviewthread/internal/store/hub.go` (`Entity(ws:<ws>, review_thread, id, dto)`, `Gone` on delete); `api/internal/app/container.go` (resources map: `ws` → forge poll; `git` → watcher, origin sync; `files` → watcher; `lsp` → LSP host); `agentChatDef` loses its `ScopeKey`/`OnSubscribe`/`OnUnsubscribe` (the forge poll moved to `ws:`).
- Delete: `api/internal/app/realtime/` (all), `gitDef`, `filesDef`, `lspDef`, `threadsDef`, `terminalsDef`, `withWatcherLifecycle`, `withLSPLifecycle`, `withOriginSyncLifecycle`, `scopeWsID`, `scopeLSPOwnerID` and their broadcasters; the WebSocket halves of the git status route, `files/routes.go` `/ws`, `editor/routes.go` `/lsp/ws`, `threads/routes.go`'s stream, and `terminal/routes.go` `GET /terminals` (it becomes a plain `GET` to `h.ListSessions`); the matching `hub` Broadcast/Push methods.
- Test (black box): port every test that dials those endpoints (`grep -rln 'git/status\|/files/ws\|/lsp/ws\|/threads' api/tests`) to the stream helper.
- Web modify: `web/src/features/workspace/stores/hooks/use-workspace-git.ts` (`holdScope('git:'+wsId)` + `onSignal('git:'+wsId, 'git_status', …)`), `use-workspace-file-tree.ts` (hold `files:` before the REST tree read, so no change between the read and the attach is lost; `onSignal(…, 'file_change', …)`), `web/src/features/editor/lsp/lsp-client.ts` (hold `lsp:<wsId>`; the owning-chat `followScope` goes because the scope is the workspace), the workspace view that S2 R16 refcounts lifecycle subscriptions from also holds `ws:<wsId>` while mounted (forge poll); every reader of the per-workspace store's review threads reads `selectReviewThreads(s, wsId)` from `web/src/core/entities/select.ts` (create; `grep -rn 'threads' web/src/features/workspace/stores` for the slice and its readers).
- Web delete: `use-workspace-threads-stream.ts`, the per-workspace threads slice state and actions, and their tests.
- Web tests: extend `web/src/__tests__/features/workspace/stores/hooks/{use-workspace-git.test.ts,use-workspace-file-tree.test.ts}`, `web/src/__tests__/features/editor/lsp/lsp-client.test.ts`, create `web/src/__tests__/core/entities/select.test.ts`.

**Interfaces:**
- Consumes: T5.
- Produces: signal kinds `git_status` (today's `GitStatusEvent`), `file_change` (`FileChangeEvent`), `diagnostics` (today's diagnostics event); entity kind `review_thread` (`ThreadDTO`) in `EntityData`; `selectReviewThreads(s: EntityState, wsId: string): ThreadDTO[]` (memoized on `byKind.review_thread` identity).

- [ ] **Step 1: Write the failing tests.** Go (`api/internal/app/stream/`): `TestWsSource_SnapshotsTheWorkspacesReviewThreads`, `TestGitSource_SnapshotIsTheCurrentStatus`, `TestRefcount_TheWatcherIsSharedByGitAndFiles` (hold `git:w` and `files:w`: one watcher start; release both: one stop after its linger, driven by the injected timer). Black box: `TestStream_GitScopeSignalsAStatusChange` (write a file in an imported worktree → a `git_status` signal for that workspace), `TestStream_WsScopeCarriesReviewThreads` (post a review comment → a `review_thread` entity frame). Web:
```ts
// use-workspace-git.test.ts
it('holds git:<ws> while mounted and applies git_status signals')
// use-workspace-file-tree.test.ts
it('holds files:<ws> before reading the tree, so a change during the read is applied')
// lsp-client.test.ts
it('holds lsp:<ws> for an open document and releases it on close')
// select.test.ts
it('selectReviewThreads returns the workspace threads and keeps identity when nothing changed')
```
- [ ] **Step 2: Run them to verify they fail.** `GOTEST ./internal/app/stream/...`; `GOBOX 'TestStream_'`; `cd web && bunx vitest run <the four test files>`.
- [ ] **Step 3: Implement** as listed: move the resources, write the sources, re-point publishers and consumers, delete the definitions, routes, hub methods and `realtime`.
- [ ] **Step 4: Run.** Step 2's commands plus `GOTEST ./internal/app/... ./internal/api/...`, Go gates, `go vet -tags 'integration noEmbed' ./tests/...`, `bun tsc --noEmit`, web gates. `grep -rn 'realtime\.' api/internal --include='*.go'` finds nothing.
- [ ] **Step 5: Commit.**

```bash
git commit -m "refactor(stream): git, files, LSP and review threads ride the stream; realtime's four refcounts become the hub's one

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: The chat ledger rides the stream

**Files:**
- Create: `api/internal/app/stream/chat_source.go` (`Attach` calls the chat usecase's `OpenFeed(ctx, chatID, since)` and maps its frames: `row` → `row{scope, row}`; `ready{seq}` → `synced{scope, seq}`; `delta` → `delta{scope, delta}`; `resync` → `closed{scope, resync}`; `deleted` → `closed{scope, deleted}`; an unknown chat → `closed{scope, refused, not_found}`); test `chat_source_test.go`.
- Modify: `api/internal/app/usecases/chat/feed.go` (the feed's own `epoch` goes from `FeedFrame`; the connection's `ready` carries the daemon epoch, minted once in `app/container.go` and passed to the hub; `OpenFeed` stays), `api/internal/app/container.go` (`chat` source).
- Delete: `api/internal/api/v0/endpoints/chat/handlers/feed.go` and `feed_test.go`, the `/feed` route.
- Test (black box): port every test that dials `/feed` (`grep -rln '/feed' api/tests`) to the stream helper; add the two below.
- Web modify: the ChatStore module that attaches a feed (S2 T3; `grep -rln 'feed?since' web/src`): attach = `holdScope('chat:'+chatId)`, detach = release (S2 R17's lifetime rule unchanged); the ChatStore registers `registerScopeKind('chat', { since, frame, epoch })`: `since(key)` is `feeds[key]?.seq ?? 0`, `frame` applies `row`/`delta` with S2's `applyRow`/`applyDelta`, `synced` marks the feed live, `closed{resync}` is handled by `stream.ts` (resubscribe), `closed{deleted}` drops the feed; `epoch` resets every feed (S2's epoch rule, now from the connection).
- Web modify: `web/src/lib/ws/manager.ts` (delete `subscribe`'s `url` parameter, S2 R15; the feed was its only user).
- Web tests: extend the S2 test file of the module that attaches feeds and `web/src/__tests__/lib/ws/manager.test.ts` (delete the `url` cases).

**Interfaces:**
- Consumes: T5 (`registerScopeKind`, `holdScope`), S1 `OpenFeed`, S2 ChatStore folds.
- Produces: nothing new for later tasks; `chat:<id>` is the only way a client reads a ledger.

- [ ] **Step 1: Write the failing tests.**
```go
// chat_source_test.go
func TestChatSource_MapsEveryFeedFrame(t *testing.T)
func TestChatSource_AnUnknownChatIsRefused(t *testing.T)
// api/tests/stream_test.go
func TestStream_ChatScopeResumesFromSinceWithoutGapOrDuplicate(t *testing.T) // Review Focus 2: rows 1..5, sub since 0, unsub after 3, append 6..8, sub since 3 → rows 4..8 once each, then synced{seq: 8}
func TestStream_DeletedChatClosesItsScopeOnly(t *testing.T)                  // the projects scope on the same connection keeps receiving frames
```
```ts
it('attaching a feed holds chat:<id> and detaching releases it')
it('a resubscribe sends since = the last applied seq')
it('a deleted close drops the feed and leaves the others')
it('a new epoch resets every feed')
```
- [ ] **Step 2: Run them to verify they fail.** `GOTEST ./internal/app/stream/...`, `GOBOX 'TestStream_Chat|TestStream_Deleted|TestChatFeed_'`, the web test files.
- [ ] **Step 3: Implement** as listed.
- [ ] **Step 4: Run.** Step 2's commands, `GOTEST ./internal/app/usecases/chat/... ./internal/api/...`, Go and web gates. `grep -rn "/feed" web/src api/internal --include='*.ts' --include='*.tsx' --include='*.go' | grep -v __tests__` finds no route or URL.
- [ ] **Step 5: Commit.**

```bash
git commit -m "refactor(stream): chat ledgers are the chat:<id> scope; the per-chat feed socket goes

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```
