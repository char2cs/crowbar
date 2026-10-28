# S4b: One stream. Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `/v0/stream` exists end to end (daemon hub, frames, scopes, resources; client socket, scope holds, entity store) and carries operations, every workspace-scoped stream, S5's host-document watch (index OR2) and the chat ledger. The operations stream, six stream definitions, `realtime` and the per-chat feed socket are deleted.

**Architecture:** See the index, `docs/plans/2026-10-17-s4-stream-store-tree-plan.md` (R5–R9, R16, R20, Shared vocabulary). The hub is the one publisher: one mutex, one version counter per epoch, one queue per connection that bounds live frames only (snapshots and replays are backpressured, R8), one refcount per (resource, key) (R9). A source per scope kind answers `sub`. The client's `stream.ts` owns the socket and routes frames to the store that owns each scope kind, which registers itself (core never imports a feature).

**Tech Stack:** Go, gin, gorilla/websocket, testify, `api/tests` black box; zustand, vitest + Testing Library, `wsManager` (`web/src/lib/ws/manager.ts`).

**Spec:** architecture §4 "The one stream", §3 "Daemon memory: watchers, LSP hosts, forge polls, refcounted by subscription", §8 (delete `realtime` lifecycles, `v0/ws`).

---

### Task 5: The one stream, with operations

**Files:**
- Create: `api/internal/domain/stream/{scope.go,frame.go}`; tests `scope_test.go`, `frame_test.go` with `testdata/frames.json` (one example of every frame type, the JSON the web decoder test copies).
- Create: `api/internal/app/stream/{hub.go,conn.go,refcount.go,projects_source.go}`; tests `hub_test.go`, `conn_test.go`, `refcount_test.go`, `projects_source_test.go`.
- Create: `api/internal/api/v0/endpoints/stream/{routes.go,handlers/handlers.go}` (`GET /v0/stream`: upgrade with the same origin allow-list as `terminal/handlers/ws.go`'s upgrader, wrap the connection as `stream.Conn`, `hub.Serve`); test `handlers/handlers_test.go`.
- Create: `api/internal/api/v0/stream_wire.go` (`func EncodeEntity(kind stream.EntityKind, v any) (json.RawMessage, error)`; T5 knows `operation`, encoded exactly as S3's frames carried it; an unknown kind is an error), test `stream_wire_test.go`.
- Modify (oversize at pre-flight: `api/v0/container.go` 875, split by responsibility before adding; `app/container.go` is 455, keep it under 500): `api/internal/app/container.go` (mint `epoch := uuid.NewString()` once; build the hub with the `projects` source and the `Encoder` it now takes as a constructor argument; expose the hub to the API container), `api/internal/internal.go` (the composition root, `internal.go:216` at refresh: `app.New(ctx, s.engines, s.adapter, post)` becomes `app.New(ctx, s.engines, s.adapter, post, v0.EncodeEntity)`), `api/internal/app/shutdown.go` (`hub.Close()` after the operations service shuts down), `api/internal/app/usecases/operation/service.go` *(S3 T3)* (its `publish` is `func(op domain.Operation) { hub.Entity(stream.Projects, stream.KindOperation, op.ID, op, 0) }`; an operation has no asynx event version), `api/internal/api/v0/router.go`, `api/internal/api/v0/container.go` (stream defs live there and in `file_streams.go` at refresh).
- Delete *(all S3 T3)*: `operationsDef`, its `Broadcaster`, the WebSocket half of `endpoints/operations` (`DELETE /v0/operations/:id` stays), `hub.BroadcastOperation`/`PushOperation`.
- Modify: `scripts/ratchet/modules.json` (`go.stream` += `api/internal/app/stream`, `api/internal/api/v0/endpoints/stream`; `web.core` += `web/src/core`), moving no budget (new code; R1).
- Test (black box): create `api/tests/stream_client_test.go` (helper), `api/tests/stream_test.go`; port every test that dials `/v0/operations` (`grep -rln '/v0/operations' api/tests`; none at refresh, S3 T3 adds them) to the helper, built on the harness's `dial`, deleting S3's `awaitOperation` dialer in favour of `awaitOperation(t, c *streamClient, id string, done func(domain.Operation) bool)`.
- Web create: `web/src/core/stream/{frames.ts,stream.ts}`, `web/src/core/entities/entity-store.ts`; tests `web/src/__tests__/core/stream/{frames.test.ts,stream.test.ts}`, `web/src/__tests__/core/entities/entity-store.test.ts`.
- Web modify: `web/src/main.tsx` (`startStream()` replaces S3 T3's `startOperationsStream()`), `web/src/lib/store/operations.ts` *(S3 T3)* (server operations are read from `entityStore` kind `operation`; the store keeps only local operations, R11 of S3; `applyOperationFrame` goes), `web/src/lib/store/run-operation.ts` *(S3 T3)* (reads both), their tests.
- Web delete: `web/src/lib/store/operations-stream.ts` *(S3 T3)* and its test.

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

`scope_test.go`: `TestParseScope_AcceptsEveryKind` (`projects`, `chat:c1`, `ws:w1`, `git:w1`, `files:w1`, `lsp:c1`), `TestParseScope_RefusesAnUnknownKindOrAMissingKey` (`problem.CodeOf == InvalidArgument`). `frame_test.go`: `TestFrame_EveryTypeMarshalsAsTheGoldenFile`.

`hub_test.go` (a fake `Conn` over channels; fake sources):
```go
func TestHub_FirstFrameIsReadyWithTheEpoch(t *testing.T)
func TestHub_SnapshotThenSyncedThenLiveFrames(t *testing.T)
func TestHub_LiveFramesNeverPrecedeOrUndercutTheSnapshot(t *testing.T) // 50 concurrent Entity calls while a sub snapshots: for each id, v strictly increases across snapshot and live frames
func TestHub_AnEntityFrameReachesOnlyConnectionsHoldingItsScope(t *testing.T)
func TestHub_AFullQueueClosesTheConnectionWithResync(t *testing.T)   // writer blocked; QueueSize+1 live frames → CloseWith(4000, …)
func TestHub_ASnapshotLargerThanTheQueueIsDeliveredWhole(t *testing.T) // 3×QueueSize entities, one sub, no close (R8, F3)
func TestHub_EntityReturnsTheVersionItStamped(t *testing.T)            // and keeps the last (rev, v) per key (R19, F4)
func TestHub_AnUnknownScopeIsClosedRefusedAndTheConnectionLives(t *testing.T)
func TestHub_UnsubStopsFramesForThatScope(t *testing.T)
func TestHub_ReturningReleasesEveryHeldScope(t *testing.T)
```
`refcount_test.go`: `TestRefcount_StartsOnFirstHoldAndStopsOnLast` (two connections hold `git:w1`; one `Start`, one `Stop` after both release), `TestRefcount_AResourceUnderTwoKindsStartsOnce` (one fake resource listed under two kinds, both held for one key: one `Start`, one `Stop`; F10), `TestRefcount_AScopeWithoutResourcesIsFree`.

`api/tests/stream_test.go` (black box, via the helper: `dialStream(t, h) *streamClient`, `(c).Sub(t, scope string, since int64)`, `(c).Unsub(t, scope)`, `(c).Next(t, match func(stream.Frame) bool) stream.Frame` using the harness's existing reader deadline, no sleep):
```go
func TestStream_FirstFrameIsReady(t *testing.T)
func TestStream_ReadyCarriesANewEpochAfterRestart(t *testing.T)            // Review Focus 3: h.shutdown(), newHarnessAt on the same home, a new dial's epoch differs
func TestStream_ProjectsScopeSnapshotsOperations(t *testing.T)            // a settled operation from before the dial arrives as an entity frame before synced{projects}
func TestStream_AnOperationFrameFollowsItsStateChanges(t *testing.T)      // push failure (S3's fixture): accepted, running, failed in increasing v
func TestStream_AnUnknownScopeIsRefusedWithAProblem(t *testing.T)
```

- [ ] **Step 2: Run them to verify they fail.** `GOTEST ./internal/domain/stream/... ./internal/app/stream/... ./internal/api/v0/endpoints/stream/...`; `GOBOX 'TestStream_|TestOperations_|TestRegression_GitPush'` (the last two exist once S3 T3 lands). Expected: compile failure.

- [ ] **Step 3: Implement the daemon side.** `Hub` holds `mu`, `seq uint64`, the connections, and per scope the set of connections and a refcount. `Entity` takes `mu`, increments `seq`, records `(rev, seq)` for the key, marshals once, enqueues to every connection holding the scope (non-blocking; a full queue closes that connection with 4000; only live frames count toward the bound), and returns the `seq` it stamped. `sub` takes `mu`, asks the scope's source to `Attach` (an entity source emits its frames, each stamped with the current `seq`, through a backpressured `emit` that blocks while the writer drains and never closes on overflow; index R8, F3), enqueues `synced` (logging at debug the snapshot's frame count and how long `mu` was held, which T13 reads), registers the connection, increments the refcount of each (resource, key) the scope kind lists (0→1 runs that `Resource.Start(key)`), releases `mu`. A ledger source (T7) attaches without `mu`. `unsub` and connection end decrement (1→0 runs `Stop`); a resource listed under several kinds is counted once per key across them (R9, F10). The writer goroutine drains the queue to `Conn.WriteJSON`; the reader parses `Request`s. Re-point the operations service's publish; delete the operations stream def, its route's WebSocket half and the hub pair; port the black-box dialers.

- [ ] **Step 4: Run the Go tests.** Step 2's commands; then `GOTEST ./internal/app/... ./internal/api/...`, the Go gates, `go vet -tags 'integration noEmbed' ./tests/...`. Expected: PASS.

- [ ] **Step 5: Write the failing web tests.** `stream.test.ts` mocks `@/lib/ws/manager` with a fake that records `send` and exposes the `subscribe` callback (one `vi.mock`):
```ts
it('sends sub on the first hold of a scope and unsub on the last release')
it('re-sends sub for every held scope after a reconnect, with since from the kind handler')
it('a closed resync frame resubscribes that scope')
it('a closed refused frame reports its problem once and drops the scope')
it('an undecodable frame is reported once and its scope dropped')      // R20, ledger F22
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

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01DG5r7UkKfKiDuXTdNTGTXK"
```

---

### Task 6: Workspace scopes; `realtime` goes

**Files:**
- Create: `api/internal/app/stream/resources/{watcher.go,host_watch.go,lsp.go,forge_poll.go,origin_sync.go}` (each `Resource`), moved from `api/internal/app/realtime/` as it is at refresh: `watcher_manager.go`, `watcher_factory.go`, `watcher_proc.go`, `linger_timer.go` → `watcher.go` (+ split files if over 500); `host_watch_manager.go` (S5) → `host_watch.go`; `lsp_manager.go`, `lsp_lifecycle.go` → `lsp.go`; `provider_poll_manager.go` → `forge_poll.go`; `origin_sync_manager.go` → `origin_sync.go`; `watcher_dispatcher.go` and `git_status_provider.go` move beside the publishers they feed; `service.go` (the five Acquire/Release pairs) is deleted. Keep each start/stop body, its injected clock and timer factory, its `pollTimeout`, and S3's join-on-stop fixes (`a5d4b536`, `6e0cdce1`); delete each manager's refcount map (the hub's per-(resource, key) refcount replaces all five, so the watcher shared by `git:` and `files:` still starts once; R9, F10). Tests move with them, one per source (`watcher_test.go`, `host_watch_test.go`, `lsp_test.go`, `forge_poll_test.go`, `origin_sync_test.go`, from today's `*_internal_test.go` and `manager_seams_internal_test.go`), their manager-refcount cases deleted.
- Create: `api/internal/app/stream/{ws_source.go,git_source.go,files_source.go,lsp_source.go,doc_source.go}` and their tests: each ports its old `StreamDef.Snapshot` (`threadsDef` → review-thread entities of the workspace; `gitDef` → the current git status as one signal; `filesDef` → none; `lspDef` → current diagnostics as signals; `documentsDef` → none, the client reads the document once `synced` arrives, as it does after the socket opens today). `Resource.Start` returns an error; a failed start closes the scope `refused` with its problem (the host watch's `ErrWatchLimit` keeps a code of its own; OR2).
- Modify: the publishers: the watcher dispatcher (`Signal(git:<ws>, "git_status", ev)`, `Signal(files:<ws>, "file_change", ev)`), the LSP diagnostics publisher (`Signal(lsp:<owningChatId>, "diagnostics", ev)`: the session is keyed by chat id today, `lspOwnerID` in `endpoints/editor/handlers/handlers.go:255-265`; F9), `api/internal/app/repositories/reviewthread/internal/store/hub.go` (`Entity(ws:<ws>, review_thread, id, dto)`, `Gone` on delete); the host watch's change publisher (`Signal(doc:<path>, "file_change", ev)`); `api/internal/app/container.go` (resources map: `ws` → forge poll; `git` → watcher, origin sync; `files` → watcher; `lsp` → LSP host; `doc` → host watch); delete `withChatProviderPollLifecycle` and `scopeChatWorktreeID` (`api/v0/container.go:259-290`, `AcquireProviderPoll`: the forge poll moved to `ws:`), so `agentChatDef` no longer wraps them.
- Delete: `api/internal/app/realtime/` (all), `gitDef`, `filesDef`, `lspDef`, `threadsDef`, `terminalsDef`, `documentsDef`, `withWatcherLifecycle`, `withHostWatchLifecycle`, `hostWatchAdmit`, `withLSPLifecycle`, `withOriginSyncLifecycle`, `scopeWsID`, `scopeLSPOwnerID` and their broadcasters (`api/v0/container.go`, `api/v0/file_streams.go`); the WebSocket halves of `git/routes.go:58` (`/status`), `files/routes.go:59` and `home/routes.go:83` (`/files/ws`), `editor/routes.go:72` (`/lsp/ws`), `threads/routes.go:27` and `home/routes.go:89` (`/threads`), `documents/routes.go:21` (`/documents/ws`, the route goes), and `terminal/routes.go:40` `GET /terminals` (it becomes a plain `GET` to `h.ListSessions`); the matching `hub` Broadcast/Push methods.
- Test (black box): port every test that dials those endpoints (`grep -rln 'git/status\|/files/ws\|/lsp/ws\|/threads\|/documents/ws' api/tests`; `files_test.go` holds the documents ones) to the stream helper.
- Web modify: `web/src/features/workspace/stores/hooks/use-workspace-git.ts` (`holdScope('git:'+wsId)` + `onSignal('git:'+wsId, 'git_status', …)`), `use-workspace-file-tree.ts` (hold `files:` before the REST tree read, so no change between the read and the attach is lost; `onSignal(…, 'file_change', …)`), `web/src/features/editor/lsp/lsp-client.ts` (hold `lsp:<owningChatId>`; `followScope` stays and re-holds when the workspace's owning chat changes, `lsp-client.ts:121-133`; keying by the workspace would be a visible change needing a V-entry, F9), the workspace view that S2 R16 *(S2 T4)* refcounts lifecycle subscriptions from also holds `ws:<wsId>` while mounted (forge poll); `web/src/features/workspace/lib/external-buffer-sync.ts` holds `doc:<path>` and `onSignal(…, 'file_change', …)` in place of `documentWatchEndpoint`'s socket (`lib/api/documents.ts`), keeping S5's watch-limit wording on a `refused` close; every reader of the per-workspace store's review threads reads `selectReviewThreads(s, wsId)` from `web/src/core/entities/select.ts` (create; `grep -rn 'threads' web/src/features/workspace/stores` for the slice and its readers).
- Web delete: `use-workspace-threads-stream.ts`, the per-workspace threads slice state and actions, and their tests.
- Web tests: create `web/src/__tests__/features/workspace/stores/hooks/{use-workspace-git.test.ts,use-workspace-file-tree.test.ts}` (neither exists at refresh), extend `web/src/__tests__/features/editor/lsp/lsp-client.test.ts` and the existing test of `external-buffer-sync.ts` (`find web/src/__tests__ -name 'external-buffer-sync*'`), create `web/src/__tests__/core/entities/select.test.ts`.

**Interfaces:**
- Consumes: T5.
- Produces: signal kinds `git_status` (today's `GitStatusEvent`), `file_change` (`FileChangeEvent`), `diagnostics` (today's diagnostics event); entity kind `review_thread` (`ThreadDTO`) in `EntityData`; `selectReviewThreads(s: EntityState, wsId: string): ThreadDTO[]` (memoized on `byKind.review_thread` identity).

- [ ] **Step 1: Write the failing tests.** Go (`api/internal/app/stream/`): `TestWsSource_SnapshotsTheWorkspacesReviewThreads`, `TestGitSource_SnapshotIsTheCurrentStatus`, `TestRefcount_TheWatcherIsSharedByGitAndFiles` (hold `git:w` and `files:w`: one watcher start; release both: one stop after its linger, driven by the injected timer). Black box: `TestStream_GitScopeSignalsAStatusChange` (write a file in an imported worktree → a `git_status` signal for that workspace), `TestStream_WsScopeCarriesReviewThreads` (post a review comment → a `review_thread` entity frame), `TestStream_DocScopeSignalsAnExternalChange` and `TestStream_DocScopeAtTheWatchLimitIsRefused` (ported from `files_test.go`'s documents cases). Web:
```ts
// use-workspace-git.test.ts
it('holds git:<ws> while mounted and applies git_status signals')
// use-workspace-file-tree.test.ts
it('holds files:<ws> before reading the tree, so a change during the read is applied')
// lsp-client.test.ts
it('holds lsp:<owningChatId> for an open document and releases it on close')
it('re-holds the lsp scope when the workspace owning chat changes')
// select.test.ts
it('selectReviewThreads returns the workspace threads and keeps identity when nothing changed')
```
- [ ] **Step 2: Run them to verify they fail.** `GOTEST ./internal/app/stream/...`; `GOBOX 'TestStream_'`; `cd web && bunx vitest run <the four test files>`.
- [ ] **Step 3: Implement** as listed: move the resources, write the sources, re-point publishers and consumers, delete the definitions, routes, hub methods and `realtime`.
- [ ] **Step 4: Run.** Step 2's commands plus `GOTEST ./internal/app/... ./internal/api/...`, Go gates, `go vet -tags 'integration noEmbed' ./tests/...`, `bun tsc --noEmit`, web gates. `grep -rn 'realtime\.' api/internal --include='*.go'` and `grep -rn 'documents/ws' api/internal web/src` find nothing.
- [ ] **Step 5: Commit.**

```bash
git commit -m "refactor(stream): git, files, LSP and review threads ride the stream; realtime's five refcounts become the hub's one

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01DG5r7UkKfKiDuXTdNTGTXK"
```

---

### Task 7: The chat ledger rides the stream

**Files:**
- Create: `api/internal/app/stream/chat_source.go` (`Attach` calls the chat usecase's `OpenFeed(ctx, chatID, since)` and maps its frames; the replay before `synced` goes through the backpressured `emit` and never counts toward the live bound (R8, F3): `row` → `row{scope, row}`; `ready{seq}` → `synced{scope, seq}`; `delta` → `delta{scope, delta}`; `resync` → `closed{scope, resync}`; `deleted` → `closed{scope, deleted}`; an unknown chat → `closed{scope, refused, not_found}`); test `chat_source_test.go`.
- Modify: `api/internal/app/usecases/chat/feed.go` (the feed's own `epoch` goes from `FeedFrame`; `FeedDelta` moves from `usecases/chat/internal/ingress/live.go` to `domain/ledger` so `domain/stream.Frame` can carry it, and `Frame.Delta` is added; the connection's `ready` carries the daemon epoch, minted once in `app/container.go` and passed to the hub; `OpenFeed` stays), `api/internal/app/container.go` (`chat` source).
- Delete: `api/internal/api/v0/endpoints/chat/handlers/feed.go` and `feed_test.go`, the `/feed` route (`api/v0/router.go:120`, `chats.GET("/:chatId/feed", …)`).
- Test (black box): port every test that dials `/feed` (`grep -rln '/feed' api/tests`: `chat_feed_test.go`'s six `TestChatFeed_*` at refresh) to the stream helper; add the two below.
- Web modify: the ChatStore module that attaches a feed *(S2 T3; no web code reads the feed at refresh)* (`grep -rln 'feed?since' web/src`): attach = `holdScope('chat:'+chatId)`, detach = release (S2 R17's lifetime rule unchanged); the ChatStore registers `registerScopeKind('chat', { since, frame, epoch })`: `since(key)` is `feeds[key]?.seq ?? 0`, `frame` applies `row`/`delta` with S2's `applyRow`/`applyDelta`, `synced` marks the feed live, `closed{resync}` is handled by `stream.ts` (resubscribe), `closed{deleted}` drops the feed; `epoch` resets every feed (S2's epoch rule, now from the connection).
- Web modify: `web/src/lib/ws/manager.ts` (230 lines at pre-flight; delete S2 R15's optional third argument `url?: () => string` *(S2 T3)*, whose only user was the feed; `subscribe(endpoint, cb)` itself stays, `stream.ts` uses it for `/v0/stream`; F18).
- Web tests: extend the S2 test file of the module that attaches feeds and `web/src/__tests__/lib/ws/manager.test.ts` (delete the `url` cases).

**Interfaces:**
- Consumes: T5 (`registerScopeKind`, `holdScope`), S1 `OpenFeed`, S2 ChatStore folds.
- Produces: nothing new for later tasks; `chat:<id>` is the only way a client reads a ledger.

- [ ] **Step 1: Write the failing tests.**
```go
// chat_source_test.go
func TestChatSource_MapsEveryFeedFrame(t *testing.T)
func TestChatSource_AnUnknownChatIsRefused(t *testing.T)
// api/internal/app/stream/hub_test.go
func TestHub_ALedgerReplayLargerThanTheQueueIsDelivered(t *testing.T)      // sub since 0 on a chat with 3×QueueSize rows: every row, no close (R8, F3)
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

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01DG5r7UkKfKiDuXTdNTGTXK"
```
