# S4c: One row derivation, one UI store, the entity store cutover. Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The sidebar draws every row with one function over a `TreeIndex` and `RowViews`; its UI state lives in one store; then the `projects` scope fills the entity store with every project, repo, workspace, chat and Tree, and the client's second copies of server truth (`app-sync-engine`, the IndexedDB entity cache, the sidebar data stores, the ChatStore's entity half) are deleted with the daemon's `hub`, `ws` and `snapshots.go`.

**Architecture:** See the index, `docs/plans/2026-10-17-s4-stream-store-tree-plan.md` (R3, R10, R14, R16, R18, Shared vocabulary). T8 and T9 are refactors behind T1's pin (no behaviour change). T8's `legacy-tree-index.ts` turns today's placement fields into explicit containers with the rules `buildSidebarTree` applies today; T10 replaces it with `treeIndex(tree)` over the daemon's Tree and deletes it.

**Tech Stack:** React 19, zustand, vitest + Testing Library, `fake-indexeddb` (already used by `web/src/__tests__/lib/persistence`); Go, gin, testify, `api/tests` black box.

**Spec:** architecture §3 ("the client keeps no second copy of server truth", "panes hold ids"), §8 (delete `app-sync-engine`, the IndexedDB entity cache, the sidebar stores; "Thirty-seven sidebar files into one `features/workspace/tree` with one row-derivation function"; delete `hub`, `v0/ws`, `snapshots`).

---

### Task 8: One row-derivation function

**Files:**
- Create: `web/src/features/workspace/tree/{tree-index.ts,legacy-tree-index.ts,views.ts,rows.ts,input.ts}` (`TreeNode`, `TreeIndex`, `RowViews`, `ChatView`, `WorkspaceView`, `RepoView`, `TreeInput`, `deriveRows` as in the index; `legacyTreeIndex(repos: readonly Repo[], home: HomeTree, projectId: string): TreeIndex`; `viewsFromStores(repos: readonly Repo[], home: HomeTree): RowViews`; `input.ts`: `useTreeInput(projectId: string): TreeInput` for components and `treeInputFor(projectId: string): TreeInput` for handlers, the only two ways sidebar code obtains rows or row data, so T10 changes their bodies and not their callers).
- Create tests: `web/src/__tests__/features/workspace/tree/{rows.test.ts,legacy-tree-index.test.ts}`.
- Modify: every caller of `rowsForProject`/`rowsFromHome`/S3's `rowsFromOperations` and every sidebar module that reads row data out of `useSidebarStore`'s repos or the home-tree store (`web/src/components/layout/{sidebar-tree-surface.tsx,home-actions.ts,create-actions.ts,removal-plan.ts}`, `web/src/components/sidebar/space-scroller.tsx`, the parts S3 split `drop-actions.ts` and `row-actions.ts` into; `grep -rln 'useSidebarStore\|useHomeTreeStore' web/src/components` for the rest): they read through `useTreeInput`/`treeInputFor` and `deriveRows`.
- Modify: `scripts/ratchet/modules.json` (`web.sidebar` += `web/src/features/workspace/tree`; the moved lines stay in the same module, so no budget moves).
- Delete: `web/src/components/sidebar/lib/{rows-from-repo.ts,rows-from-home.ts,rows-for-project.ts}`, `buildSidebarTree` and its helpers in `web/src/components/layout/workspace-tree-utils.ts` (delete the file if nothing else remains), `web/src/lib/store/build-repo-tree.ts`, S3's `web/src/components/sidebar/lib/rows-from-operations.ts`; their tests (`rows-from-repo*.test.ts`, `rows-from-home.test.ts`, `rows-for-project.test.ts`, `build-repo-tree.test.ts`, `workspace-tree-folders.test.ts`, `rows-from-operations.test.ts`): each case that pins behaviour `deriveRows` keeps moves to `rows.test.ts` or `legacy-tree-index.test.ts`; cases for deleted behaviour go.

**Interfaces:**
- Consumes: T1's `PIN`, `loadPinFixture`, `outline`.
- Produces: the index's `features/workspace/tree` vocabulary. `SidebarRow` gains `nodeId: string` (the Tree node the row draws; it differs from `id` for workspace and repo rows, R18), which T11's moves send. `RowViews` carries exactly the fields `SidebarRow` shows today (label sources, branch name, lock, status, added/deleted, working, has-view, placeholder, provisional label, repo avatar), and nothing else. `legacyTreeIndex` applies, once, today's placement rules (folderId → fork parent → workspace ground → root for workspaces; parent → workspace ground → root for chats; a cycle re-rooted; an owning chat folded into its workspace row; project-home rows at the root with the owning home chat excluded) and returns explicit containers in R4's shape, so `deriveRows` never reads a placement fallback.

- [ ] **Step 1: Write the failing tests.** `rows.test.ts` (hand-built `TreeIndex` and `RowViews`, no stores):
```ts
it('draws a repo row with its home checkout owning chat as the row id')       // R18
it('draws a workspace row by its owning chat id, else by the workspace id')    // R18
it('labels a locked branch by its branch name and an owned branch by its chat title')
it('draws children in index order, depth-first')
it('draws project-home chats and folders at the root between repo rows by order')
it('marks project-home rows not forkable')
it('never draws a node it has no view for, and reports it once')               // no ghost row
it('skips rows being removed only where today skips them')
it('draws an in-flight create from its operation item until its node exists')
```
`legacy-tree-index.test.ts`: one case per rule above (`it('files a workspace under its fork parent when it has no folder')`, `it('re-roots a cycle')`, `it('folds an owning chat into its workspace')`, …), each built from the cases moved out of the deleted test files.

- [ ] **Step 2: Run them to verify they fail.** `cd web && bunx vitest run src/__tests__/features/workspace/tree`.
- [ ] **Step 3: Implement** `deriveRows`, `legacyTreeIndex`, `viewsFromStores`; switch the callers; delete the old derivers.
- [ ] **Step 4: Run.** The new tests, T1's pin (`src/__tests__/components/layout/sidebar-tree-surface.test.tsx`, unchanged), `src/__tests__/components/sidebar` and `src/__tests__/components/layout` test files that import the touched modules (`grep -rl` the deleted names first: none may remain), `bun tsc --noEmit`, web gates. Expected: PASS.
- [ ] **Step 5: Commit.**

```bash
git add web/src/features/workspace/tree web/src/__tests__/features/workspace/tree scripts/ratchet/modules.json <each modified path>
git rm <each deleted file and test>
git commit -m "refactor(sidebar): one deriveRows over an explicit tree index replaces four row derivers

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 9: One tree UI store

**Files:**
- Create: `web/src/features/workspace/tree/tree-ui-store.ts`; test `web/src/__tests__/features/workspace/tree/tree-ui-store.test.ts`.
- Modify: every reader and writer of the stores below (`grep -rln` each store's hook name), `web/src/lib/persistence/hydrate.ts` (`hydrateSidebar` writes `collapsedChatRows` into the tree UI store), `web/src/lib/persistence/sidebar-ui.ts` (unchanged record shape: a production build reads the same IndexedDB `sidebar-ui` record).
- Delete: `web/src/lib/store/{sidebar-selection.ts,sidebar-inline-rename.ts,sidebar-removal.ts,sidebar-ui.ts}`, S3's `create-draft.ts`, the UI slice of `web/src/lib/store/sidebar.ts` (`collapsedChatRows` and the other view-only fields; its data half stays until T10), and their tests (`sidebar-selection.test.ts`, `sidebar-inline-rename.test.ts`, `sidebar-removal.test.ts`, `sidebar-collapse.test.ts`, S3's `create-draft.test.ts`): their cases move to `tree-ui-store.test.ts`.

**Interfaces:**
- Produces:

```ts
export interface TreeUiState {
  selection: { anchor: string | null; ids: ReadonlySet<string> }
  renaming: string | null                              // row id
  collapsed: ReadonlySet<string>                        // row ids, persisted as sidebar-ui.collapsedChatRows
  removals: readonly RemovalEntry[]                     // the removal tray (S3's shape, countdown unchanged)
  draft: CreateDraft | null                             // S3's create-draft naming input
  pendingMove: PendingMove | null                       // T11
}
export const treeUiStore: StoreApi<TreeUiState & TreeUiActions>
export function useTreeUi<T>(selector: (s: TreeUiState) => T): T
```
`TreeUiActions` carries exactly the actions the deleted stores exported (renamed only where two collided), so callers change their import and hook name, not their logic.

- [ ] **Step 1: Write the failing tests** by moving the deleted stores' cases into `tree-ui-store.test.ts`, plus `it('persists collapsed rows in the sidebar-ui record a production build reads')` (the record written equals today's shape: `{ collapsedChatRows: string[] }` plus whatever other keys today writes).
- [ ] **Step 2: Run to verify they fail**; **Step 3: implement and switch every caller**; **Step 4: run** the new test, T1's pin, every test file that imported a deleted store (`grep -rl`), `bun tsc --noEmit`, web gates. Expected: PASS.
- [ ] **Step 5: Commit.**

```bash
git commit -m "refactor(sidebar): selection, rename, collapse, removal tray and create drafts live in one tree UI store

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 10: The `projects` scope fills the entity store

**Files (Go):**
- Modify: `api/internal/app/stream/projects_source.go` (snapshot: every project, repo, workspace, chat, Tree and operation, then `synced`), `api/internal/api/v0/stream_wire.go` (`EncodeEntity` for `project`, `repo`, `workspace`, `chat`, `tree`, `telemetry`: today's DTO mappers; `tree` → `TreeDTO` with `ToLegacy`-free explicit nodes), `api/internal/app/stream/chat_source.go` (the chat scope's snapshot also emits the chat's `telemetry` entity; S2 R10's GET on attach goes).
- Modify (publishers; one `Entity`/`Gone` call each, replacing the `hub` call it sits beside): project and repo usecases (create, patch, delete, icon), the Workspace and Chat aggregate stream subscribers (`repositories/{workspace,chat}/internal/store/hub.go` become `stream.go`, publishing on `projects`), the chat working-state change point S1/S2 publish from (`working` is part of the chat entity), telemetry (`chat:<id>`, kind `telemetry`), the Tree aggregate subscriber (new, `repositories/tree/stream.go`, kind `tree` on `projects`). Every command an operation's work sends uses `SendWait` (R16; `grep -rn '\.Send(' api/internal/app/repositories` lists what to change).
- Delete: `projectsDef`, `reposDef`, `agentChatDef` and the lifecycle routes (`router.go` `chatScoped.GET("/ws")`, `endpoints/chat/routes.go` `/chats/ws`, the home-tree WebSocket routes in `endpoints/home/routes.go`), the `dispatch(rest, ws)` dual-serving (routes become their REST half), `api/internal/api/v0/ws/` (all), `api/internal/app/hub/` (all), `api/internal/api/v0/snapshots.go`, the chat snapshot package's lifecycle half (`usecases/chat/internal/shared/snapshot`: keep only what `projects_source` calls; delete the package if nothing), `hub` wiring in every container.
- Test (black box): port every test that dials a deleted endpoint (`grep -rln '/ws"\|/chats/ws\|home/chats/ws\|"/v0/projects"' api/tests`) to the stream helper; `api/tests/stream_test.go` gains the two tests below; `go vet -tags 'integration noEmbed' ./tests/...` must compile (`api/tests/kit` included).

**Files (web):**
- Create: `web/src/core/entities/select.ts` additions (`selectProject`, `selectRepo`, `selectWorkspace`, `selectChat`, `selectTree`, `selectChatsByWorkspace`), `web/src/features/workspace/tree/views-from-entities.ts` (`viewsFromEntities(s: EntityState): RowViews`), `treeIndex(tree: TreeDTO): TreeIndex` in `tree-index.ts`.
- Move: `web/src/lib/store/{operations.ts,run-operation.ts}` → `web/src/core/entities/` (tests move to `web/src/__tests__/core/entities/`); `whenSettled` resolves on `succeeded` (R16); delete `web/src/lib/store/entity-presence.ts` and its test.
- Modify: `web/src/features/workspace/tree/input.ts` (`useTreeInput`/`treeInputFor` read `selectTree`, `viewsFromEntities` and the operations store), `web/src/core/stream/stream.ts` (`startStream` holds `projects` for the app's lifetime), `web/src/features/agent/stores/chat-store.ts` (delete `chats`, `byWorkspace`, `scopes`, `telemetry`, the lifecycle subscription and its reducer; every reader switches to the entity store selectors: `grep -rn 's\.chats\b\|byWorkspace\|s\.telemetry\|s\.scopes' web/src`), the sidebar surface and space scroller (`deriveRows({ index: treeIndex(selectTree(s, projectId)), views: viewsFromEntities(s), … })`; the skeleton shows until `synced.projects`), `web/src/routes/__root.tsx` (no `AppSyncProvider`), `web/src/lib/store/workspace-route-guard.ts` (reads the entity store after `synced.projects`; `sidebar-sync.ts`'s pre-navigation write goes), `web/src/components/projects/add-repository-modal.tsx` (waits on `whenSettled` only; no entity-cache read), `web/src/lib/persistence/hydrate.ts` (`placeRestoredChatMembers` becomes a synchronous read of `selectChat(id).workspaceId`, run once by the app shell's effect when `synced.projects` turns true; `findProjectChats` and its GETs go), `web/src/lib/persistence/idb.ts` (delete `healEntityStores`, `CROWBAR_CACHE_VERSION`, `wipeEntityCache`, `maybeWipeOnVersionChange`; keep version 10, the v1–v10 upgrade chain and the `VersionError` open-as-is fallback; R14), `web/src/lib/api.ts` (delete the read helpers only the deleted code called: `fetchRepos`, `fetchHomeChats`, `fetchRepoChats`, folder listing, as `grep` confirms).
- Delete: `web/src/components/app-sync-engine.ts`, `web/src/components/app-sync-provider.tsx`, `web/src/lib/persistence/{entity-cache.ts,cache-store.ts}`, `web/src/lib/store/{loadable-slice.ts,home-tree.ts,folder-signal.ts,sidebar.ts,sidebar-sync.ts,projects.ts,project-from-dto.ts,project-visibility.ts,workspace-list.ts,repo-tree.ts}`, `web/src/lib/ws/{entity-stream.ts,structural-chat-folder-frame.ts,chat-frame-kinds.ts}`, `web/src/features/workspace/stores/hooks/use-workspace-provider-stream.ts`, `web/src/features/workspace/tree/legacy-tree-index.ts`, `viewsFromStores`, and their tests (`app-sync-*.test.tsx` ×6, `entity-cache.test.ts`, `cache-store.test.ts`, `wipe.test.ts`, `loadable-slice.test.ts`, `projects*.test.ts`, `project-visibility*.test.ts`, `home-tree*.test.ts`, `folder-signal.test.ts`, `sidebar*.test.ts` left after T9, `workspace-list.test.ts`, `entity-stream.test.ts`, `legacy-tree-index.test.ts`, the `hydrate.test.ts` cases for `findProjectChats`): a case pinning behaviour that survives moves to the test of the code that now owns it.
- Modify tests: T1's pin: `loadPinFixture()` now applies `PIN` as entity frames (`applyEntityFrame` for its project, repos, workspaces, chats and its `TreeDTO` with explicit containers, then `applySynced('projects')`); the assertions do not change.

**Interfaces:**
- Consumes: T5 (hub, stream client, entity store), T8 (`deriveRows`), T9.
- Produces: `EntityData` gains `project: ProjectDTO; repo: RepoDTO; workspace: WorkspaceDTO; chat: ChatDTO; tree: TreeDTO; telemetry: ChatTelemetry`; the selectors above; `ChatStore` is `feeds` + `outbox` only.

- [ ] **Step 1: Write the failing Go tests.**
```go
// api/internal/app/stream/projects_source_test.go
func TestProjectsSource_SnapshotsEveryKindThenSynced(t *testing.T)
// api/tests/stream_test.go
func TestStream_ProjectsScopeCarriesTheSidebar(t *testing.T)            // import a project with a repo, a branch and a thread: project, repo, workspaces, chats and the Tree arrive before synced
func TestStream_OperationSucceededFollowsItsEntities(t *testing.T)     // Review Focus 4: a two-branch import; each created chat and workspace entity frame precedes the operation's succeeded frame on the one connection
func TestStream_ADeletedChatIsGoneOnTheProjectsScope(t *testing.T)
```
- [ ] **Step 2: Run them to verify they fail.** `GOTEST ./internal/app/stream/...`; `GOBOX 'TestStream_'`.
- [ ] **Step 3: Implement the Go side**: the source, the encoders, the publishers, `SendWait`; delete the definitions, routes, `ws`, `hub`, `snapshots.go`; port the black-box dialers.
- [ ] **Step 4: Run the Go side.** Step 2's commands, `GOTEST ./internal/app/... ./internal/api/...`, Go gates, `go vet -tags 'integration noEmbed' ./tests/...`. `grep -rn 'app/hub\|v0/ws"\|StreamDef' api/internal --include='*.go'` finds nothing.
- [ ] **Step 5: Write the failing web tests.**
```ts
// web/src/__tests__/core/entities/select.test.ts
it('selectChatsByWorkspace keeps identity while no chat changed')
it('selectTree returns the project tree and undefined before synced')
// web/src/__tests__/features/workspace/tree/views-from-entities.test.ts
it('builds every view field SidebarRow shows from entities')
// web/src/__tests__/lib/persistence/idb-version-skew.test.ts (extend)
it('an S4 build opens a v10 database without changing its version or its stores') // Review Focus 5: a v10 database with records in every crowbar_* and cache store → getDB(); version 10, the same 17 store names, every record still there, localStorage crowbar:cache-version untouched
it('a database a newer build bumped opens as-is')
// web/src/__tests__/lib/persistence/hydrate.test.ts (cases replaced, file not grown)
it('places a restored chat pane from the entity store once projects is synced')
it('drops a restored chat pane whose chat is not in the synced snapshot')
// web/src/__tests__/core/entities/run-operation.test.ts (moved)
it('whenSettled resolves on succeeded without waiting for entity presence')
```
- [ ] **Step 6: Run to verify they fail**, then implement the web side, switch every reader, delete the files.
- [ ] **Step 7: Run.** `cd web && bunx vitest run src/__tests__/core src/__tests__/features/workspace/tree src/__tests__/lib/persistence src/__tests__/components/layout/sidebar-tree-surface.test.tsx` plus every test file that imported a deleted module (`grep -rl` each deleted path under `src/__tests__`: none may remain), `bun tsc --noEmit`, web gates. `grep -rn "crowbar_projects\|crowbar_chats\|cache-version\|loadCache\|saveCache" web/src --include='*.ts*' | grep -v __tests__` lists only the upgrade chain in `idb.ts` and its schema types. Expected: PASS.
- [ ] **Step 8: Commit.**

```bash
git commit -m "refactor(stream): the projects scope fills the entity store; app-sync-engine, the IndexedDB entity cache, hub, ws and snapshots go

The crowbar IndexedDB keeps version 10 and every store; nothing reads or writes the entity caches any more.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```
(`git add`/`git rm` by path.)
