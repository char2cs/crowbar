# S4d: Moves, creates and the gate. Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A drop, a folder edit, a create and a removal are each one request against the Tree. The legacy placement routes, the DTO placement fields and the client's placement planning are deleted. The sidebar lives in `features/workspace/tree`, the gate holds, and the upgraded live home shows exactly the hierarchy it showed before S4.

**Architecture:** See the index, `docs/plans/2026-10-17-s4-stream-store-tree-plan.md` (R4, R11, R16, R18, R19, Shared vocabulary, D9). The drop indicator keeps a client mirror of the Tree's rules for feedback only; the daemon decides. A drop shows at once through a pending move in the tree UI store, cleared when the move's reply (the Tree entity at its version) is applied.

**Tech Stack:** Go, gin, testify, `api/tests` black box; React 19, zustand, vitest + Testing Library; Tauri MCP (`webview_execute_js`, `webview_interact`, `webview_screenshot`).

**Spec:** architecture §3 ("A create is one Tree write plus one entity write, published together with one operation"), §8 (merge the sidebar into `features/workspace/tree`), §11 S4 gate.

---

### Task 11: Moves go through the Tree

**Files (Go):**
- Create: `api/internal/api/v0/endpoints/tree/{routes.go,handlers/handlers.go}` (R19's four routes over `usecases/tree.Service`; the move reply carries `hub.VersionOf(stream.KindTree, projectID)`), test `handlers/handlers_test.go`.
- Modify: `api/internal/app/stream/hub.go` (`VersionOf`: the hub records the `v` of the last frame per entity key), `api/internal/api/v0/router.go`.
- Delete: `ToLegacy` (`usecases/tree/legacy.go` keeps `FromLegacy` for step 7), `PATCH …/chats/:id/placement` (repo and home), `PATCH …/workspaces/:id/placement`, `…/chats/folders` CRUD (repo and home) and their handlers (`endpoints/chat/handlers/folders.go`, `endpoints/workspace/handlers/placement.go`, `PlaceChat`), `order`/`folderId` from the repo PATCH body, `ChatDTO.parentId/order`, `WorkspaceDTO.folderId/order`, `ChatWorktreeDTO.folderId/order`, `RepoDTO.order/folderId`, `FolderDTO`, and their tests.
- Modify (black box): `api/tests/tree_placement_test.go` (T1): `placementOf` reads the `tree` entity from the `projects` scope; each expectation is restated in explicit-container terms (R4) with a comment naming the T1 value it replaces; the refusal test hits `POST …/tree/moves` and asserts R19's codes.

**Files (web):**
- Create: `web/src/features/workspace/tree/{api.ts,drop-policy.ts,drop.ts}`; tests `web/src/__tests__/features/workspace/tree/{drop-policy.test.ts,drop.test.ts}`.
- Move: `web/src/components/sidebar/hooks/use-sidebar-drag.ts` (751 lines: split by responsibility, gesture vs. auto-scroll vs. ghost), `web/src/components/tree-dnd/{drop-core.ts,drop-dom.ts}`, `web/src/components/layout/{drag-ghost.tsx,drop-indicator.tsx,edge-scroll.ts}` → `web/src/features/workspace/tree/drag/` (gesture, animation and feel unchanged; tests move to the mirrored path).
- Modify: `web/src/features/workspace/tree/{tree-ui-store.ts,rows.ts}` (`pendingMove` applied by `deriveRows` as the row's position while set), every folder create/rename/delete caller (row menu, inline rename, create space panel) → `api.ts`.
- Delete: `web/src/lib/api/sidebar-placement.ts`, `web/src/lib/store/applied-placement.ts`, the placement planning in what S3 left of `drop-actions.ts` (densify, scope roots, landing) and `components/sidebar/lib/sidebar-drop-policy.ts`, and their tests (`drop-actions*.test.ts`, `sidebar-drop-policy.test.ts`, `applied-placement.test.ts`, `repo-vs-repo-drop-line.test.ts`, `home-row-vs-repo-header-drop-line.test.ts`): a case pinning which drop the indicator allows moves to `drop-policy.test.ts`; a case pinning where a drop lands moves to `drop.test.ts`; cases for deleted planning go.

**Interfaces:**
- Consumes: T3 service, T5 hub, T8 `SidebarRow.nodeId`, T9 store, T10 entity store.
- Produces:

```ts
// api.ts
export interface Placement { parentId: string; index: number } // mirrors domain/tree.Placement
export function moveNode(projectId: string, nodeId: string, parentId: string, index: number): Promise<void> // applies the reply as an entity frame
export function createFolder(projectId: string, f: { id: string; name: string; parentId: string; index: number }): Promise<void>
export function renameFolder(projectId: string, id: string, name: string): Promise<void>
export function deleteFolder(projectId: string, id: string): Promise<void>
// drop-policy.ts: R4's rules over a TreeIndex and RowViews, for the indicator only
export function canDrop(index: TreeIndex, views: RowViews, nodeId: string, parentId: string): boolean
// drop.ts: a drop target → the move it means (DropMode: drop-core's 'before' | 'after' | 'into')
export interface DropTarget { nodeId: string; mode: DropMode }
export function moveForDrop(index: TreeIndex, dragged: string, target: DropTarget): { parentId: string; index: number } | null
export interface PendingMove { projectId: string; nodeId: string; parentId: string; index: number }
```

- [ ] **Step 1: Write the failing Go tests.** `handlers_test.go`: `TestMove_RepliesWithTheTreeAndItsVersion`, `TestMove_ARefusalIs409WithItsCode`, `TestCreateFolder_UsesTheClientID`, `TestDeleteFolder_MovesChildrenUp`; black box as above. Run: `GOTEST ./internal/api/v0/endpoints/tree/...`, `GOBOX 'TestTree_'`. Expected: FAIL.
- [ ] **Step 2: Implement the Go side** and delete the legacy routes and fields. Run Step 1's commands, `GOTEST ./internal/app/... ./internal/api/...`, Go gates. `grep -rn 'ToLegacy\|FolderDTO\|/placement' api/internal --include='*.go'` finds nothing.
- [ ] **Step 3: Write the failing web tests.**
```ts
// drop-policy.test.ts: one case per R4 rule, mirroring domain/tree's rules_test.go case names
it('refuses a folder inside a chat'); it('refuses a node into its own subtree'); it('keeps a node in its context')
it('refuses a thread under another workspace'); it('keeps a fork under its fork parent')
// drop.test.ts
it('a drop before a row means its parent at its index')
it('a drop inside a container means its last child')
it('a drop onto its own place is no move')
// tree-ui-store.test.ts (extend) / rows.test.ts (extend)
it('a pending move draws the row at its new place at once')
it('the reply clears the pending move in the same update that applies the tree')   // no frame shows the old place
it('a refused move clears the pending move and reports the refusal once')       // V2
```
- [ ] **Step 4: Implement the web side**; move the drag modules; delete the planning. Run the new and moved tests, T1's pin, `bun tsc --noEmit`, web gates. Expected: PASS.
- [ ] **Step 5: Commit.**

```bash
git commit -m "refactor(tree): a drop is one Tree move; legacy placement routes, DTO placement fields and client placement planning go

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 12: Creates and removals are one Tree write

**Files (Go):**
- Modify: the create routes the sidebar uses (thread create, branch create, fork, project-home chat create, branch import; `grep -rn 'h.Create\b\|ImportBatch\|Fork' api/internal/api/v0/endpoints`): each body accepts `placement: {parentId, index}` (optional; absent = the end of the natural container R4 names) and passes it to the T4 verb, which places the node in the same operation; `domain.OperationItem` gains `Placement *tree.Placement` (`type Placement struct { ParentID string; Index int }` in `domain/tree`), set by those verbs, so an in-flight row knows where it will land; the S3 TS mirror of `Operation` gains it.
- Test: extend the black-box test file of each create route (`grep -rln` the route path in `api/tests`) with Step 1's tests.

**Files (web):**
- Create: `web/src/features/workspace/tree/{create.ts,remove.ts,actions.ts}`; tests `web/src/__tests__/features/workspace/tree/{create.test.ts,remove.test.ts,actions.test.ts}`.
- Delete: `web/src/components/layout/{create-actions.ts,thread-create.ts,home-actions.ts,removal-plan.ts,removal-commit.ts,trash-actions.ts}` and what S3 left of `web/src/components/sidebar/lib/row-actions.ts`, plus their tests (`create-actions*.test.ts`, `thread-create.test.ts`, `home-actions.test.ts`, `removal-plan*.test.ts`, `removal-commit.test.ts`, `trash-actions.test.ts`, `row-actions.test.ts`, `recents-home-chat-branch-create.test.ts`): cases that pin surviving behaviour move to the new test files; the second "place it" request after a create goes.
- Modify: every caller (row menu, row action buttons, recents band, space header, removal tray, removal confirm dialog, keyboard shortcuts: `grep -rln` each deleted module) → the new modules; `rows.ts` draws an in-flight item at `item.placement`.

**Interfaces:**
- Consumes: T4 verbs, T9 store, T10 entity store and `run-operation`, T11 `api.ts`.
- Produces:

```ts
// create.ts: one request per create, the row drawn from its operation item until its node exists
export function createThread(projectId: string, parentNodeId: string, index: number): Promise<string> // chat id
export function createBranch(projectId: string, fromWorkspaceId: string, name: string, placement: Placement): Promise<string>
export function createHomeChat(projectId: string, placement: Placement): Promise<string>
export function importBranches(projectId: string, repoId: string, branches: { name: string; chatId: string }[], placement: Placement): Promise<void>
// remove.ts: plan from the Tree, commit through today's delete requests (the daemon cascades through Tree.Unplace)
export function planRemoval(input: TreeInput, rowId: string): RemovalPlan // RemovalPlan keeps S3's shape (rows, worktrees at risk, counts)
export function commitRemoval(plan: RemovalPlan): Promise<void>
// actions.ts: rename, open, promote and the rest of today's row actions, unchanged in behaviour
```

- [ ] **Step 1: Write the failing tests.** Go: `TestCreateThread_PlacesTheNewRowWhereAsked`, `TestImportBatch_EachItemCarriesItsPlacement`, `TestCreateHomeChat_WithoutPlacementLandsAtTheEndOfTheRoot`. Web:
```ts
it('a thread create sends one request carrying its placement')
it('the in-flight row sits at its placement until its node arrives, then the node row replaces it with no gap')
it('a failed create keeps its row at its placement with the reason')       // S3 V2, now placed
it('planRemoval of a folder lists its subtree; of a branch, its worktree at risk')
it('commitRemoval sends one delete per removed root')
```
- [ ] **Step 2: Run to verify they fail**, **Step 3: implement**, **Step 4: run** the new tests, T1's pin, every test file that imported a deleted module, Go and web gates. Expected: PASS.
- [ ] **Step 5: Commit.**

```bash
git commit -m "refactor(tree): a create carries its placement and is one Tree write plus one entity write; removals plan from the Tree

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 13: Gate and live check

**Files:**
- Move: every remaining file of `web/src/components/sidebar/` and `web/src/components/tree-dnd/`, and the sidebar files of `web/src/components/layout/` (the `sidebar-*`, `space-*`, `removal-tray.tsx`, `removal-confirm-dialog.tsx`, `create-space-panel.tsx`, `recents-*`, `row-*`, `inline-rename-input.tsx`, `workspace-row-base.ts`, `workspace-branch-icon.tsx`, `repo-avatar.tsx`, `repo-icon-mark.tsx`, `project-icon-mark.tsx`, `keep-set.ts`, `use-card-resize-drag.ts`, `use-carousel-scroll-sync.ts`, `use-measured-height.ts`, `use-sidebar-panel.ts`, `open-actions.ts`, `detach-holder-modal.tsx`) into `web/src/features/workspace/tree/` (`git mv`, imports updated, tests moved to the mirrored path). `web/src/lib/store/` must then hold nothing the sidebar uses (`ls` it; anything left is named in the commit body with its owner).
- Modify: `scripts/ratchet/modules.json` (D9's files named under their owning keys; stale prefixes removed: `api/internal/app/hub`, `api/internal/app/realtime`, `api/internal/api/v0/ws`, `api/internal/api/v0/snapshots.go`, `web/src/components/app-sync-engine.ts`, `web/src/components/app-sync-provider.tsx`, `web/src/components/sidebar`, `web/src/components/tree-dnd`), `scripts/ratchet/loc-budgets-baseline.json` (move D9's lines from `web.sidebar` to their keys by hand first, then `make ratchet ARGS=--update` writes every lower number), `scripts/ratchet/counters-baseline.json` and `file-size-baseline.json` (lower numbers from `--update`).
- Modify: the comments of every moved file to the ≤3-line *why* rule (comment changes only).

- [ ] **Step 1: Move and re-key.** Run T1's pin, `cd web && bunx vitest run src/__tests__/features/workspace/tree src/__tests__/components/layout`, `bun tsc --noEmit`, web gates. Expected: PASS.
- [ ] **Step 2: Measure the gate.** `node scripts/ratchet/loc-budgets.mjs` (or `make ratchet`): `web.sidebar` ≤ 5,000. If it is above, stop and report the per-file list (largest first) and D9's status; never loosen the budget and never move a file whose owner is the sidebar. Then the index's gate table: the `websocket.Upgrader` grep lists exactly three sites (stream, terminal PTY, console log tail); `grep -rn 'newAsynx\[' api/internal/app` lists Workspace, Chat, ReviewThread, Tree; the reseed-GET grep finds only user-initiated reads; `grep -rn 'crowbar_\|loadCache\|saveCache\|entity-cache' web/src --include='*.ts*' | grep -v __tests__` lists only `idb.ts`'s upgrade chain and schema types.
- [ ] **Step 3: Ratchet.** `make ratchet ARGS=--update`, then `make ratchet`, `make pr-checks` (or each gate it runs, if it runs a full suite: never a full `vitest run`; follow `CLAUDE.md`'s targeted rule and run the Go and web gates listed in the index). Expected: PASS, every number lower or equal.
- [ ] **Step 4: Live check (Review Focus 1, 5).** `df -h /System/Volumes/Data` (P3). `rm -rf .crowbar/s4-pin/home && tar -xf .crowbar/s4-pin/home-before.tar -C .crowbar/s4-pin` (T1's home, untouched since T1). `make dev-desktop CROWBAR_HOME=$(git rev-parse --show-toplevel)/.crowbar/s4-pin/home`. Then with Tauri MCP:
  1. `$CROWBAR_HOME/migrate/ledger.jsonl` records `placement_tree` and `tree_containers` applied and verified; `$CROWBAR_HOME/state/events/node.db` does not exist.
  2. Capture the outline with T1's script; `diff` against `.crowbar/s4-pin/outline-before.txt`: empty.
  3. In the app: drag a thread into a repo folder and back; drag a home folder above a repo; create a folder, rename it, delete it (its children move up, in order); try to drop a folder onto a chat (the indicator refuses; no request is sent); try to drop a thread under another branch (the indicator refuses). After each step capture the outline and check it against the expected change only; `webview_screenshot` once per step for the record (not committed).
  4. Quit and relaunch: the outline equals the one before quitting; the skeleton shows until the first snapshot (V1).
  5. In the webview: `indexedDB.databases()` reports `crowbar` at version 10, and opening it without a version lists the same 17 object store names as `.crowbar/s4-pin/home-before.tar`'s era (read only; nothing is written).
  6. Record, from `performance` entries, the time from navigation start to the first `[role="treeitem"]` (D5's number) in the commit body.
  Quit the app. Never touch `~/.crowbar` or the production app.
- [ ] **Step 5: Commit.**

```bash
git add scripts/ratchet <moved paths>
git commit -m "chore(sidebar): the sidebar lives in features/workspace/tree; S4 gate ratcheted

web.sidebar <n> (from 18,562), go.stream <n> (from 3,278); stream definitions 1; aggregates 4.
Live: the upgraded pin home renders the identical outline; crowbar IndexedDB still v10 with 17 stores.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```
