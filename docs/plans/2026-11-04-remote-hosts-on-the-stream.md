# Remote hosts on the stream. Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every paired host's projects, repos, workspaces, chats, files, git and lsp scopes reach the web through the same `/v0/stream` protocol and land in the same entity store as the local daemon's, each entity tagged with the host that sent it. Chat presence (develop #234's "attached viewers") becomes a hub-owned entity on the projects scope, and active sessions are derived from the store. No second socket per feature, no second store, no REST list mirrors.

**Architecture:** One `/v0/stream` connection per host (local, plus one per configured remote), all feeding `entityStore`. The host is never read from a payload: it is the host of the socket the frame came on (the rule develop's `mergeProjectFrame` already followed). Store keys and client-side scope names are host-qualified; entity `id`s, routes and REST paths stay raw. Presence is owned by the daemon's hub, which already counts who holds each `chat:<id>` scope.

**Tech Stack:** Go (`api/internal/app/stream`, `api/internal/domain/stream`, gin, gorilla/websocket, testify, `api/tests` black box); zustand, vitest + Testing Library, `wsManager` (`web/src/lib/ws/manager.ts`); Tauri bridge unchanged.

**Spec:** `docs/superpowers/specs/2026-09-27-remote-instances-design.md` (hosts, pairing, TLS pin, bearer); `docs/plans/2026-10-17-s4-stream-store-tree-plan.md` (R5–R9 one stream, R19 versions, R20 golden frames).

---

## What is broken on the branch (merge `b5cbefd1`)

- `web/src/core/stream/stream.ts` is a module singleton for one socket: `ENDPOINT = '/v0/stream'`, one `ready`, one `epoch`, one `holds` map. `wsManager.subscribe(ENDPOINT, …)` dials only the local daemon, so a paired host's scopes are never subscribed.
- `web/src/core/entities/views-from-entities.ts:158` stamps every project `hostId: LOCAL_HOST.id` ("the one stream is the local daemon's").
- `web/src/lib/host-resolve.ts` still gates every remote call (`useHostResolved`, `hostIdForChat` via `lib/workspace-scope.ts`), but remote rows have no source: develop's per-host REST+WS list (`lib/store/projects.ts`, deleted in the merge) was the only one.
- Chat-scoped reads carry no host: `readBlob` (`features/agent/api/agent-chat-reads.ts:272`) calls `apiFetchRaw` with a bare `/v0/chats/:id/blobs/:ref`; `chat-feed.ts` holds `chat:<id>` on the local socket.
- `web/src/lib/store/active-sessions.ts` (147 lines) survived the merge with no feeder: nothing calls `applyFrame`/`seedScope` since `chat_presence.go` and `ws/hold.go` (`git show develop:api/internal/api/v0/chat_presence.go`) were deleted with `/v0/chats/:id/attach`. The Active section (`components/sidebar/active-sessions-section.tsx`), the kill menu (`features/agent/hooks/use-kill-agent-menu-items.tsx`), `use-native-sidebar.ts:290` and `active-sessions-settings.tsx` read an always-empty store, which CLAUDE.md calls a stub that ships.
- The phone (`hasLocalDaemon() === false`) has no local daemon, so on the branch it has no data source at all.

## Decisions

**D1. Who owns a remote entity.** The remote daemon does, and its stream is the only way its rows reach the store. No REST list is read into the store beside the stream (develop's `readHostRows` goes for good).

**D2. Host-qualified keys, raw ids.** `entityKey(hostId, id)` is `id` for `local` and `` `${hostId}~${id}` `` for a remote host. `~` cannot occur in a host id (`REMOTE_HOST_ID = /^[a-z0-9-]+$/`, `lib/host-path.ts`) nor in a daemon id (uuid). Every `byKind[kind]` record, `members` entry and operation is keyed this way. The store stamps `hostId` onto a project's `data` at ingest (the one kind whose shape names a host); every other record's host is its key's (`splitHostKey`). `data.id`, routes (`/ide/$projectId/…`), pane layouts and REST paths keep the raw id.
- Why keyed per host: versions are per daemon epoch (R19), and one host's `synced` prune, epoch reset or removal must never touch another host's row, even one with the same raw id.
- Why raw ids elsewhere: 85 call sites resolve hosts from raw ids (`grep -rn hostIdFor web/src`), and raw is what a host's REST routes take.
- Compat: local keys are bare, so every persisted local reference is unchanged: `crowbar.activeProject`, sidebar folds (`lib/persistence/sidebar-ui`), the terminal reconnect map, drafts and daemon-held pane layouts. Remote references develop builds persisted (active project, folds) are raw ids too, so they resolve again once their host's projects scope syncs. No migration. Develop's `projects-by-host` localStorage cache is orphaned and nothing reads it.
- Twins: the same raw id on two hosts (a copied database). Both rows are held under their own keys. The views show the first by host rank, and `host-resolve` keeps refusing with its existing "exists on more than one host" reason.

**D3. Client scope names.** `hostScope(hostId, scope)` follows the same rule (`ws:w1` local, `host-1~ws:w1` remote). On the wire, each socket sends and receives bare scopes; the host stream qualifies a scope on receipt and strips the qualifier on send. Kind handlers get `(hostId, frame)` with the bare scope.

**D4. One stream per host, lifecycle from `features/hosts`.** There is one `HostStream` per configured host plus `local` when `hasLocalDaemon()`. Opening, parking and redialing already belong to `wsManager`: a host-qualified endpoint `qualify(hostId, '/v0/stream')` parks while `mayDial` is false (`down`, `needs-repair`, unpaired) and reopens when `subscribeHostStatus` says `up`. Revocation or a changed identity closes the socket, `followStreamFailures` maps the 401 or pin failure to `needs-repair`, and the channel parks. A ready frame resubscribes every held scope from its `since`, exactly as for local today. Removal (`onHostRemoved`) stops that host's stream and calls `forgetHost(hostId)`, which drops every key with its prefix plus its `members`/`fresh`/`synced`/`loaded`/`unread`/epoch entries.

**D5. Offline host.** A host that is `down` or `needs-repair` keeps its last snapshot's rows, because nothing said they are gone. The sidebar draws them under that host's `HostStatusIcon`, and writes fail through `apiFetch` as today. `loaded[hostScope(h,'projects')]` is still false before the host's first sync, so `host-resolve` reads its ids as unknown, not absent. `hostUnavailability` (`lib/host-unavailability.ts`) already turns that into the pane notice.

**D6. Presence is a hub entity.** Holding `chat:<id>` *is* viewing. The hub's holder count per chat scope (`h.scopes["chat:<id>"]`, one entry per connection) is published as entity kind `presence`, id = chat id, data `{viewers:n}`, on the projects scope, and as `gone` at 0. The count includes the viewing client itself, as develop's attach socket did.
- Why not a field on the chat entity: the chat row is published by `chatPublisher` (`app/sidebar_publish.go`) at the chat snapshot's own `rev`. A second writer of the same key would need a rev it does not own, and R19's stale-rev drop would lose one writer's change.
- The projects snapshot carries the current presence rows. They are emitted in `emitReadLocked` under `h.mu`, the same lock under which live presence frames are stamped, so ordering stays R7.

**D7. Active sessions are derived.** `selectActiveSessions(s)` lists the chats with `liveRunnerId`, joined to `presence` by entity key and sorted by title then key. It reads every host, so a phone sees each paired host's running agents. `lib/store/active-sessions.ts` is deleted. `viewersLabel` and the types move next to the selector.

**D8. Kill is flat.** `POST /v0/chats/:chatId/kill` answers 204 on the `chatScoped` group (`api/internal/api/v0/router.go:117`, guarded by `resolveChatWorktree`). The nested `repoScoped.POST("/chats/:id/kill")` (`endpoints/chat/routes.go:130`) and `home.POST("/chats/:id/kill")` (`endpoints/home/routes.go:136`) are deleted. The web calls `killChat(hostId, chatId)`.

**D9. Operations per host.** An operation id is client-minted (the `Idempotency-Key`, a uuid), so the operations view stays keyed by it: `operationsState()` folds every host's streamed rows by raw id, so the one a host streams replaces the local one (`forgetLocalOperation(id)`) and `whenSettled(id)` settles on it, and records `hostOf[id]`, the host that streamed it. `dismissOperation` sends the DELETE there. The POST already goes to its entity's host through the caller's URL builder, so `OperationStart` needs no host. A created item counts as present once a row with its id is held on the operation's host.

## Line budgets (source, all < 500 after)

| File | Now | After | Note |
|---|---|---|---|
| `web/src/core/stream/stream.ts` | 228 | ~110 | registry, public API, `startStream`/`startHostStreams` |
| `web/src/core/stream/host-stream.ts` (new) | 0 | ~210 | today's per-socket state, one instance per host |
| `web/src/core/stream/host-scope.ts` (new) | 0 | ~30 | `entityKey`, `hostScope`, `splitHostScope` |
| `web/src/core/entities/entity-store.ts` | 290 | ~330 | host in frames, `epochs`, `forgetHost`, `entityOf`, `presence` |
| `web/src/core/entities/views-from-entities.ts` | 187 | ~205 | host from key, `hostRank` from develop's `projects.ts` |
| `web/src/core/entities/active-sessions.ts` (new) | 0 | ~60 | `selectActiveSessions`, `viewersLabel` |
| `web/src/lib/store/active-sessions.ts` | 147 | deleted | |
| `web/src/lib/host-resolve.ts` | 143 | ~120 | reads `hostId` off held rows; no `workspace-scope` |
| `web/src/features/agent/stores/chat-feed.ts` | 152 | ~160 | attachments keyed by entity key |
| `web/src/lib/ws/manager.ts` | 324 | 324 | unchanged |
| `api/internal/app/stream/hub.go` | 438 | ~446 | two calls into presence |
| `api/internal/app/stream/presence.go` (new) | 0 | ~70 | |
| `api/internal/app/stream/read.go` | 195 | ~200 | presence rows in the projects snapshot |
| `api/internal/domain/stream/frame.go` | 122 | ~124 | `KindPresence` |

`scripts/ratchet/modules.json` needs no new module (new files land in `go.stream`, `web.core`). Deleting `lib/store/active-sessions.ts` and the nested kill routes lowers LOC, so run `make ratchet ARGS=--update` after T2 and T9.

## Tasks

Each task lands alone and green, its failing test first. Go tasks T1–T3 are independent of the web tasks. Web tasks run in order T4 → T9.

### T1: Presence entity (Go)

**Files:** `api/internal/domain/stream/frame.go` (`KindPresence EntityKind = "presence"`), `testdata/frames.json` (one `presence` entity and one `presence` gone), `api/internal/app/stream/presence.go` (new: `Presence{Viewers int}`, the value the hub hands its encoder; `presenceLocked(scope stream.Scope)` stamps `h.seq+1` and fans out `{viewers:len(h.scopes[name])}`, or gone at 0, on `stream.Projects`; `syncedLocked` queues every held `chat:` scope's presence before a projects `synced`), `hub.go` (`registerLocked`/`releaseLocked` call `presenceLocked` for `KindChat`, only when the holder set changed; `subLocked` ends with `syncedLocked`), `read.go` (`emitReadLocked` ends with `syncedLocked`), `api/internal/api/v0/stream_wire.go` (`EncodeEntity` maps `Presence` to `dto.PresenceDTO{Viewers int}` in `dto/entity.go`). Web: `core/stream/frames.ts` `KINDS` += `presence` (the golden file test copies the JSON); the store keeps ignoring the kind until T5 adds it to `EntityData`.
- Presence bypasses `publishEntity`'s R19 check: a gone at rev 0 there refuses every later value at rev 0, so a chat viewed again would never count again. The hub is presence's only writer, under `h.mu`, so its frames need no rev and no stamp.

- [ ] **Step 1: failing tests.** `presence_test.go` (fake conns as in `hub_test.go`): `TestPresence_TwoConnectionsHoldingAChatCountTwo`, `TestPresence_TheLastReleaseSendsGone`, `TestPresence_ADroppedConnectionCountsAsALeave` (`dropLocked`), `TestPresence_AProjectsSnapshotCarriesTheCurrentCounts`, `TestPresence_ResubscribingTheSameConnectionDoesNotCountTwice`, `TestPresence_VersionsIncreaseAcrossSnapshotAndLive` (50 concurrent holds/releases while a projects sub snapshots). `frame_test.go` golden covers `presence`. Black box `api/tests/stream_test.go`: `TestStream_HoldingAChatIsViewingIt` (two `dialStream` clients hold `chat:<id>`, a third holding `projects` sees viewers 1 then 2, then gone after both unsub).
- [ ] **Step 2:** `GOTEST ./internal/app/stream/... ./internal/domain/stream/...`; `GOBOX 'TestStream_HoldingAChat'`. Expected: compile failure, then red.
- [ ] **Step 3:** implement; `golangci-lint run --build-tags noEmbed ./...` = 0.

### T2: Flat kill (Go)

**Files:** `endpoints/chat/routes.go` (`Register` takes `chatScoped` from `router.go` and mounts `chatScoped.POST("/kill", h.Kill)`; the repo-scoped one goes), `endpoints/chat/handlers/switch.go` (`Kill` reads `:chatId`; `resolveChatWorktree` has already 404'd an unknown chat, so it needs no workspace), delete `endpoints/home/routes.go:136`, `route_audit_test.go`. Web: `killChat(hostId, chatId)` in `features/agent/api/agent-chat-commands.ts:194` posts to `chatBaseOn(hostId, chatId)/kill` (`lib/workspace-scope-url.ts`; a hand-written `/v0/chats` path fails `no-restricted-syntax`), and `kill-agent.ts` passes `hostIdForChat(chatId)` until T9 gives each session its `hostId`.

- [ ] **Step 1: failing tests.** `api/tests/agent_stop_test.go` (extend): `TestKill_FlatRouteEndsTheRunnerAndAnswers204` (a livestub chat with a live runner; the chat entity's `liveRunnerId` empties on the projects scope; it replaces `TestRegression_AgentKillEndsTheLiveRunnerAndKeepsTheChat`), `TestKill_AnUnknownChatIs404`, `TestKill_TheNestedRouteIsGone` (404 on the repo and home mounts). Web: `agent-chat-commands.test.ts` (`killChat('host-1','c1')` posts `crowbar://host-1/v0/chats/c1/kill`), `kill-agent.test.ts` (the kill goes to the chat's host; an unresolved host is reported and kills nothing).
- [ ] **Step 2:** red; **Step 3:** implement, delete the nested handlers' tests that only covered the old path, `make deadcode`.

### T3: The stream over a paired TLS connection (Go, black box)

**Files:** `api/tests/harness_test.go` (`newPairedHarness(t)`: `httptest.NewTLSServer(apiContainer.Handler())`, mint and redeem a code through `POST /v0/pair/code` and `POST /v0/pair`, and keep the bearer), `api/tests/stream_client_test.go` (`dialStreamAs(t, h, token)` sets `Authorization: Bearer`).

- [x] **Step 1: tests.** `TestRemoteStream_ServesProjectsAndWorkspaceScopes`, `TestRemoteStream_RefusesWithoutABearer` (401 at upgrade), `TestRemoteStream_RefusesADocScope` (`admitHostDocument` with `TransportRemote`), `TestRemoteStream_RevokingTheClientClosesTheStream` (`DELETE /v0/clients/:id`, then the reader sees EOF; `revocableWriter` must hold the hijacked conn).
- [x] **Step 2:** run. All passed at once, so this lands with T4. `bootHarness` takes the daemon fingerprint: minting a pairing code refuses an empty one. If all pass at once, this task is the daemon half of T4 and lands with it rather than alone (no coverage-only commit). A red revoke test is a real bug: fix it in `api/internal/api/v0/client_conns.go`.

### T4: One stream per host (web core)

**Files:** create `host-stream.ts`; slim `stream.ts` (`host-scope.ts` lands with T5, its first user). API (all take the host explicitly; nothing defaults to local):

```ts
export interface ScopeKindHandler {
  since?(hostId: string, key: string): number
  subscribed?(hostId: string, scope: string): void
  frame(hostId: string, f: Frame): void
  epoch?(hostId: string, epoch: string): void
  forget?(hostId: string): void                     // the host was removed
}
export function holdScope(hostId: string, scope: string): () => void
export function watchScope(hostId: string, scope: string, watch: ScopeWatch): () => void
export function onSignal<T>(hostId: string, scope: string, kind: string, cb: (d: T) => void): () => void
export function resubscribe(hostId: string, scope: string): void
export function holding(hostId: string, scope: string): boolean
export function streamOpened(hostId: string): Promise<void>
export function startStream(): () => void          // local when hasLocalDaemon(), plus startHostStreams()
```

`startHostStreams` follows `useHostConfigStore` (one `HostStream` per configured host, each holding its `projects`) and `onHostRemoved` (stop the stream, then call every handler's `forget?(hostId)`, which T5 implements). Callers (`use-workspace-effects.ts:23`, `use-workspace-git.ts:81`, `use-workspace-file-tree.ts:96`, `lsp-client.ts:155`, `external-buffer-sync.ts:126`, `use-ide-shell-workspace-retention.ts:75`, `chat-feed.ts:131`, `unread-notice.tsx`, `shell-landing.tsx`) switch from `useHostResolved` to `useResolvedHost`, and pass the host they already gate on. Fixture `__tests__/__fixtures__/stream-daemon.ts`: `streamDaemonFor(hostId)` and `heldScopes(prefix, hostId = 'local')`; `setup.ts`'s socket routes each `<host>/v0/stream` to that host's daemon, which says ready on subscribe, so `pairRemoteHost` needs no change. Until T5 the entity store ignores a remote host's frames.

- [ ] **Step 1: failing tests** (`__tests__/core/stream/stream.test.ts`, extend): `a scope held on host-1 is subscribed on crowbar://host-1/v0/stream, bare`, `a frame from host-1 reaches the kind handler with host-1`, `a drop of host-1 leaves local scopes synced`, `host-1's ready resubscribes only host-1's holds`, `removing host-1 stops its socket and tells every handler to forget it`, `no local socket opens when the install has no local daemon`, `a hold taken before host-1's ready goes out on it` (parking is the socket manager's; the stream sees only the ready that follows `up`).
- [x] **Step 2:** red; **Step 3:** implement; tsc, eslint, knip, `lint:doctor`.

### T5: The entity store across hosts (web core)

**Files:** `core/stream/host-scope.ts` (`entityKey`, `hostScope`, `splitHostKey`). `core/entities/entity-store.ts`: `withFrame(s, hostId, f)` keys by `entityKey`, stamps a project's `hostId`, and qualifies `f.scope`. `epoch: string` becomes `epochs: Record<string,string>`, and `resetForEpoch(hostId, e)` voids that host's versions only. Add `forgetHost(hostId)` and `entityOf(s, kind, hostId, id)`. `patchEntities`/`placeEntities`/`forgetEntities` take keys. Replace the raw lookups (`use-chat-entity.ts:19`, `chat-outbox.ts:93,202`, `applied-placement.ts:40`, `create-project.ts:22`, `resolve-root-path.ts:6`, `persistence/hydrate.ts:97`, `panes/lib/chat-project.ts:17,26`) with `entityOf` and the host they resolve; the writers of answers (`applied-placement.ts`, `removal-commit.ts`, `add-repository-modal.tsx`, `chat-outbox.ts`) key by the host they wrote to, and `chat-lifecycle.ts` maps keys back to raw ids. Operations (`operations.ts:107`, `holdsEntity` in `tree/input.ts`) stay keyed as their ids until T7 files them per host (D9). `views-from-entities.ts:158` reads the host from `data.hostId`, sorts by `hostRank` then `byOrder` (moved from `git show develop:web/src/lib/store/projects.ts`), and dedupes twins by raw id. Add `presence: PresenceDTO` to `EntityData`.

- [x] **Step 1: failing tests** (`__tests__/core/entities/entity-store.test.ts`, extend; `applied-placement.test.ts`: `a host-1 placement leaves local's twins alone`; fixture `sidebar-entities.ts` gains a `hostId` argument): `two hosts' rows with one raw id are both held`, `host-1's synced prunes none of local's rows`, `host-1's new epoch voids only host-1's versions`, `forgetHost drops that host's rows and scopes, nothing else`, `a payload naming another hostId is filed under the socket's host`. `views-from-entities.test.ts`: `projects list local first, then hosts in pairing order`, `a twin shows once`. Each local-only existing test passes unchanged, which proves the compat claim.
- [x] **Step 2:** red; **Step 3:** implement.

### T6: Host resolution reads the owner (web)

**Files:** `lib/host-resolve.ts`. `resolveProjectHost`/`resolveWorkspaceHost`/`resolveChatHost` collect the `hostId`s of held rows with that raw id (project, workspace, chat entities; an index per version of the kind's records). A held row decides; the recorded workspace scope (and the owner hop through it) is only the fallback for an id no row holds yet, as a route records one before its host syncs: dropping it would move 34 test files' worlds for no behaviour. An absent id is unknown until this install's daemon (if it has one) and every configured host that is up have `loaded` their projects scope, and at least one has; absent after that. `unreadHosts` is the configured hosts not `loaded` yet. `noRemoteEntityCanExist` keeps its `hasPairedRemote` rule. `subscribeHostResolution` also follows host statuses. `host-unavailability.ts` keeps its API. Fixtures: `loadProjects` lands each named host's snapshot; a paired host's fake daemon leaves its projects sub to the test.

- [x] **Step 1: failing tests** (`__tests__/lib/host-resolve.test.ts`, extend, using `projectOn`): `a chat streamed by host-1 resolves to host-1 with no workspace scope recorded`, `an id no up host has listed after all synced is absent`, `an id while host-1 is still syncing is unknown`, `a twin is unresolved naming both hosts`, `on the phone, a host's synced list is the list`.
- [x] **Step 2:** red; **Step 3:** implement.

### T7: Chat scopes and chat REST per host (web)

**Files:** `features/agent/stores/chat-feed.ts` holds the chat scope on its attachment's host and its `since` answers only that host (T4); feeds stay keyed by raw chat id, since a twin chat id resolves to no host and so attaches nowhere. `readBlob(hostId, chatId, ref)` builds its URL with `chatBaseOn`, and `tool-payload.tsx` reads nothing until `useChatHost` names the host. The other chat REST callers (`chatBase`, `upload-chat-attachment.ts`, the asset resolvers) already resolve through `hostIdForChat`, which reads the chat's row since T6. Operations as in D9 (`core/entities/operations.ts`, `run-operation.ts`, `entity-store.ts`'s unstreamed-reply prune, `features/workspace/tree/input.ts`).

- [x] **Step 1: failing tests:** `__tests__/features/agent/stores/chat-feed.test.ts` (`a host-1 chat subs on its host and resumes from its seq when that host reconnects, not when local does`), `agent-chat-reads.test.ts` (`readBlob goes to crowbar://host-1/v0/chats/c1/blobs/…`), `__tests__/core/entities/run-operation.test.ts` (`settles on host-1's frame for it, which replaces the local one`, `dismissing it asks host-1`), `features/workspace/tree/input.test.ts` (`counts its item present once that host streams the row it names`).
- [x] **Step 2:** red; **Step 3:** implement.

### T8: Offline and removed hosts in the sidebar; the phone (web)

**Files:** `components/layout/sidebar-project-switcher.tsx` already draws each remote mark with its host's dot (#218), and its callers feed it `useSelect(selectProjects)`, which T5 orders by host and T5's `forgetHost` empties of a removed host: no change. `features/hosts/components/host-projects-pending.tsx` says "No projects" for a host whose projects scope has synced empty, and its status for one that has not. `app-main.tsx:38` already calls `startStream()`, which covers the phone (no local socket).

- [x] **Step 1: failing tests:** `__tests__/components/layout/sidebar-project-switcher.test.tsx` (`keeps a down host's projects listed under its down dot`, `drops a removed host's projects at once`: both green at once, T5 did the work), `__tests__/features/hosts/components/host-projects-pending.test.tsx` (`lands on the first project host-1 streams`, `says a host that has synced holds no project, and shows the status of one that has not`), each fed by that host's stream fixture.
- [x] **Step 2:** red; **Step 3:** implement.

### T9: Active sessions from the store (web)

**Files:** create `core/entities/active-sessions.ts` (`selectActiveSessions(s): ActiveSession[]`, each with `hostId`, raw `chatId`, `workspaceId`, `title`, `viewers` from `presence`; plus `viewersLabel`, `useActiveSessionList`). Rewire `components/sidebar/active-sessions-section.tsx`, `features/agent/hooks/use-kill-agent-menu-items.tsx`, `features/window/hooks/use-native-sidebar.ts:290,392`, `features/settings/components/tabs/active-sessions-settings.tsx`, `features/agent/lib/kill-agent.ts` (`attached` becomes `viewers`). Delete `lib/store/active-sessions.ts` and its test. Port the test's cases to the selector instead of keeping them.

- [ ] **Step 1: failing tests:** `__tests__/core/entities/active-sessions.test.ts` (`a chat with a live runner and two presence viewers is listed with 2`, `a running chat nobody holds is listed with 0`, `a viewed chat with no runner is not listed`, `host-1's running chat is listed beside local's`, `presence gone drops the count, not the session`), `__tests__/components/sidebar/active-sessions-section.test.tsx` (fixture: entity frames, not store writes), `use-kill-agent-menu-items.test.tsx` (`kill posts to the session's host`).
- [ ] **Step 2:** red; **Step 3:** implement; knip, `make ratchet ARGS=--update`.

## Test plan

- **Hub level (Go unit):** T1's presence tests beside `hub_test.go`, which has the fake `Conn` and fake sources. The existing `TestHub_LiveFramesNeverPrecedeOrUndercutTheSnapshot` pattern covers presence ordering.
- **Black box (Go, `api/tests`, `-tags 'integration noEmbed'`):** T1 presence across real connections, T2 kill, T3 paired TLS stream and revoke.
  - Two daemons: `TestTwoDaemons_IdsAndEpochsAreIndependent` boots `newHarness` and `newPairedHarness` on separate homes and creates a project on each. One test client holds `projects` on both and checks each synced carries only its own rows, and that restarting one changes only its epoch.
  - The daemon knows nothing of other hosts. What the two-daemon test proves is the web's assumption: scopes, versions and epochs are per daemon.
  - `tests/kit` (`kit.Env` over the unix socket, `env.go` already 1009 lines) gains nothing. Two-daemon helpers live in `api/tests` beside the harness.
- **Store level (web):** T4/T5 with `streamDaemonFor(hostId)`. Fixtures are frames a test writes as the daemon would. No recorded daemon output is edited (no fabricated fixtures).
- **Component (web):** T8/T9 through `useSelect`, ≤3 `vi.mock` per file, `waitFor` only.

## Risks

- **Startup fan-out.** N hosts means N projects snapshots at boot. Each is backpressured per socket (R8), and the store applies each in one write at its synced. Watch render count with three hosts.
- **Host stamped from payload.** A remote daemon could send `hostId` in data. `withFrame` overwrites it with the socket's host (T5 test), never the reverse.
- **Raw-id lookups missed.** A missed `byKind.x[id]` reads only local rows for a remote id. T5 lists every site at the merge base. Add a lint guard (`no-restricted-syntax` on `byKind.*[` outside `core/entities`) if the list grows during review. Do not baseline it.
- **Presence counts self.** "1 viewer" on the only device watching is the viewing client. That matches develop's behaviour; changing the wording is a product call, not part of this plan.
- **StrictMode/HMR double start.** `startStream` must be idempotent per host (a second call returns the first's stop). Covered in T4.
- **Removal racing a frame.** A frame for a host removed after the frame arrived is dropped because `forgetHost` also unregisters that host's stream before the store forgets.
- **`files:`/`git:`/`lsp:` resources start on the remote daemon** for each held scope (refcount). A down host's holds are parked client-side and re-sent on `up`. The daemon released them when the socket dropped, so nothing leaks.

## Verify manually (desktop, isolated `CROWBAR_HOME`)

1. Pair a second dev daemon (docker `docker/docker-compose.yml`, or a second home on another port). Its projects appear under its host group within one snapshot. Open a repo, a workspace, a file, git status, an LSP hover and a chat on it.
2. Stop the remote daemon. The dot goes down, rows stay, and a chat pane shows the host notice. Start it again: the stream resubscribes, the open chat resumes from its seq without duplicate rows, and git and the file tree refresh.
3. Revoke the desktop from the remote (`crowbar clients` or `DELETE /v0/clients/:id`). The stream closes, the dot shows needs-repair, and nothing redials in a loop.
4. Re-key the remote (new TLS identity). The identity-changed notice appears (`host-revoke.ts`) and nothing reaches the store from it.
5. Remove the host in settings. Its rows, tabs' data and Active entries leave at once, and local is untouched.
6. Open one remote chat in two windows: the phone or settings Active list shows 2 viewers, then 1 after closing one, then 0 with the runner still live. Kill from the menu ends it (204) and it leaves the Active list.
7. Phone build (no local daemon): it lands on the first remote project and shows Active sessions for every paired host.
