# S4a: The Tree. Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The hierarchy the user sees is pinned (web, black box, live); one asynx aggregate per project owns every sidebar row's parent and order; today's placement data is converted into it and its old homes (Node aggregate, `folders` table, persisted chat placement, the planner, the chat tree usecase) are deleted. The client does not change in this file except for two ARIA attributes.

**Architecture:** See the index, `docs/plans/2026-10-17-s4-stream-store-tree-plan.md` (R2–R4, R11–R13, R17–R19, Shared vocabulary). T2 swaps storage with today's meaning kept verbatim; T3 changes the meaning (explicit containers, rules, atomic moves) behind unchanged routes; T4 removes the chat tree usecase.

**Tech Stack:** Go, asynx v1, gorm + SQLite, gin, testify, `api/tests` black box, `core/migrate`; React, vitest + Testing Library; Tauri MCP.

**Spec:** architecture §3 "Sidebar placement has one owner, the Tree", §9 (rebind is S8's), §11 row S4; migration engine plan R1, R10 and the catalog contract.

---

### Task 1: Pin the hierarchy

**Files:**
- Modify: the component that renders `role="treeitem"` for a sidebar row (`web/src/components/sidebar/sidebar-row.tsx` or the part S3 T3 split it into): add `aria-level={depth + 1}` and `aria-label={row.label}` when not already present (ARIA tree pattern; no visual change).
- Test (extend): `web/src/__tests__/components/layout/sidebar-tree-surface.test.tsx` — a `describe('hierarchy pin')` block; add no `vi.mock` (the file already has four).
- Test (create): `api/tests/tree_placement_test.go` (black box).

**Interfaces:**
- Produces (web test-local, kept by T8, T10): `const PIN` (the fixture as plain DTO data), `loadPinFixture(): void` (writes `PIN` into whatever stores the sidebar reads at this commit), `outline(root: HTMLElement): string[]` (one line per `[role="treeitem"]` in DOM order: `'  '.repeat(level - 1) + label + ' [' + rowId + ']'`, from `aria-level`, `aria-label`, `data-sidebar-row-id`).
- Produces (Go test-local, rewritten by T11): `placementOf(t *testing.T, h *harness, projectID, rowID string) (parentID string, order int)` reading today's DTO fields.

- [ ] **Step 1: Write the web pin.** `PIN` is one project `p1` (home workspace `home-p1`) holding, in this intended hierarchy (comment each fixture row with why it sits where it does):

```
level 1  home chat "Notes" [c-notes]                      project-home chat, root order 0
level 1  repo "alpha" [oc-alpha]                          repo row, id = owning chat of alpha's home checkout, root order 1
level 2    chat "Repo question" [c-alpha-q]               chat in alpha's home checkout
level 2    branch "release/1.0" [ws-release]              locked branch, no owning chat → branch name, workspace id
level 2    branch "Feature A" [oc-feat]                   branch workspace ws-feat owned by chat oc-feat
level 3      chat "Follow-up" [c-follow]                  thread of oc-feat
level 3      branch "Fork of A" [oc-fork]                 ws-fork, git ParentID = ws-feat, in no folder
level 2    folder "Experiments" [f-alpha]                 repo-scoped folder
level 3      chat "Try X" [c-try]
level 1  folder "Ideas" [f-ideas]                         project-home folder, root order 2
level 2    chat "Draft" [c-draft]                         home chat filed in the folder
level 2    repo "beta" [oc-beta]                          repo filed in a home folder
```

`it('renders the pinned hierarchy')`: `loadPinFixture()`, render `SidebarTreeSurface` for `p1`, `expect(outline(container)).toEqual([...])` with the lines above. `it('never draws a row twice or a row without a label')`: every `rowId` in `outline` is unique and every label non-empty (the no-ghost-row rule). `it('offers no fork on a project-home row')`: open the row menu of `c-notes` and `f-ideas`; no fork item. If the current code places any row differently (which parent, which level, which position), stop and report: the pin must describe correct behaviour, never a defect. A label that differs only in wording is written as rendered.

- [ ] **Step 2: Run it.** `cd web && bunx vitest run src/__tests__/components/layout/sidebar-tree-surface.test.tsx`. Expected: FAIL only on missing `aria-level`/`aria-label`; then add the attributes and see it PASS.

- [ ] **Step 3: Write the black-box pin.** `TestTree_PlacementVerbsOverHTTP` drives, over HTTP, the verbs whose results T3 must keep byte-identical on the wire: create a home folder; create a home chat and place it in the folder at order 0; create a repo folder; import a branch (`newImportFixture`) and place its workspace in that folder; create a thread under the branch's owning chat; move the thread to the repo root; reorder the home root (chat to order 0); rename and delete the repo folder (its children move up). After each verb assert `placementOf` for every touched row (exact parent and order). Then `TestTree_PlacementRefusals` asserts today's refusal for: a folder moved into its own child, a chat moved under a chat of another workspace, a repo-scoped folder moved to the home level, a forked workspace filed away from its fork parent; each answers 409 with the code today's envelope gives (record them; T3 renames them per R19).

- [ ] **Step 4: Run it.** `GOBOX 'TestTree_'`. Expected: PASS (it pins today).

- [ ] **Step 5: Capture the live "before".** With P3 checked: `export CROWBAR_HOME=$(git rev-parse --show-toplevel)/.crowbar/s4-pin/home`; install the fake provider into that home exactly as S2 R25 does (so no real CLI can start; never send a prompt); `make dev-desktop CROWBAR_HOME=$CROWBAR_HOME`; `make seed CROWBAR_HOME=$CROWBAR_HOME`; create a second local repo with `git init` under `$CROWBAR_HOME/../repos/beta` and import it; then, through the app with Tauri MCP (`webview_interact`), build the `PIN` hierarchy (titles as in Step 1; ids differ). Capture with `webview_execute_js`: `[...document.querySelectorAll('[role="treeitem"]')].map(e => '  '.repeat(+e.getAttribute('aria-level') - 1) + e.getAttribute('aria-label')).join('\n')` and save it to `.crowbar/s4-pin/outline-before.txt`. Quit the app, then `tar -cf .crowbar/s4-pin/home-before.tar -C .crowbar/s4-pin home`. Nothing under `.crowbar/` is committed (it is git-ignored).

- [ ] **Step 6: Commit.**

```bash
git add web/src/components/sidebar/<row file> web/src/__tests__/components/layout/sidebar-tree-surface.test.tsx api/tests/tree_placement_test.go
git commit -m "test(sidebar): pin the rendered hierarchy and today's placement wire before the Tree replaces it

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: The Tree aggregate replaces Node, `folders` and chat placement

Storage swap only: every row keeps today's parent and order with today's meaning (a `""` parent is "the root of this row's scope"). T3 changes the meaning.

**Files:**
- Create: `api/internal/domain/tree/tree.go` (`Kind`, `Node`, `Tree` from the index; `KindOf(legacy string) (Kind, error)`).
- Create: `api/internal/app/repositories/tree/{repository.go,commands.go,ports.go}`; tests `repository_test.go`, `commands_test.go`, `ports_test.go`.
- Create: `api/internal/app/migratetree/{legacy.go,step6.go}`; tests `legacy_test.go`, `step6_test.go`.
- Modify: `api/internal/adapter/migrations/catalog.go` (`StepPlacementTree` = next free number, post-open, in-layout, name `placement_tree`, `Plan` counting Node rows, folders and chats with placement; `RewritesState: true`); `api/internal/app/legacy_steps.go` (register `migratetree.Step6Impl(...)` beside `legacyImpls`).
- Modify: `api/internal/app/container.go` (build `axTree` over new `adapters.TreeES()`/`TreeSS()` with `newAsynx[tree.Tree]`; drop `axNode`), `api/internal/adapter/container.go` (open `events/tree.db`, `events/tree_snapshots.db`; stop opening the three node files), `api/internal/app/repositories/container.go`, `api/internal/app/usecases/container.go` (the tree usecase's `Folders`/`Nodes` ports and its chat placement writes come from `repotree.Ports`), `api/internal/app/gorm.go` (no `Folders` store), `api/internal/app/repositories/chat/` (Chat `SchemaVersion` +1 with `StripRetiredPlacementFields` dropping `/parentId` and `/order` patches, registered in `app/container.go` as the workspace one is), `api/internal/domain/chat.go` (`ParentID`, `Order` tagged `json:"-"`), `api/internal/app/usecases/chat/{home_read.go,placement_reads.go}` (one overlay: every Chat the usecase returns, and every row the tree usecase's `Chats` port reads, gets `ParentID`/`Order` from the Tree; the Node overlay goes), every other reader of the Node repository (`grep -rn 'NodeReadDB\|repositories/node\|NodePlacements' api/internal`, e.g. the workspace and repo DTO placement fields) reads the Tree through `repotree.Ports`, `api/internal/app/owning_chats_reconcile.go` (placement half reads the Tree).
- Delete: `api/internal/app/repositories/node/` (all), the `Node`/`NodeKind` half of `api/internal/domain/node.go` (the file keeps `FileNode*`), `api/internal/domain/folder.go`, `api/internal/app/repositories/chat/internal/commands/{set_placement.go,set_order.go,placement_test.go}`, the Node/Folder stubs in `api/internal/app/usecases/mocks/mocks.go` (`NodePlacements`, folder store).
- Modify tests that build Node or Folder fixtures to use the Tree (grep `domain.Node{`, `domain.Folder{`, `NodeKind`); an oversize test file is not grown (index Global Constraints).

**Interfaces:**
- Produces: `domain/tree` types (index). `repotree.New(ax asynx.Asynx[tree.Tree]) *Repository` with `Get`, `Seed(ctx, t tree.Tree) (tree.Tree, error)` (refuses an existing aggregate with `tree.ErrExists`), `Put(ctx, projectID string, n tree.Node) (tree.Tree, error)`, `Delete(ctx, projectID, id string) (tree.Tree, error)`, `Forget(ctx, projectID string) error`; commands use `SendWait`, `ShouldSnapshot() == true`, event names `tree.seeded`, `tree.put`, `tree.deleted`.
- Produces (temporary, deleted in T3/T4): `repotree.Ports{Repo *Repository; Locate Locator}` implementing the tree usecase's `Nodes` and `Folders` interfaces and its chat `SetPlacement`/`SetOrder`; `type Locator interface { ProjectOf(ctx context.Context, id string, kind tree.Kind) (string, error) }` (chat → workspace → project; workspace → project; repo → project; folder → its repo's project or its home's project), backed by an id → project index filled on `Open(ctx)` from every project's `Get` and kept on every write.
- Produces: `migratetree.ReadLegacy(ctx context.Context, src LegacySources) (LegacyHome, error)`; `type LegacyHome struct { Nodes []LegacyNode; Folders []LegacyFolder; Chats []LegacyChat; Workspaces []LegacyWorkspace; Repos []LegacyRepo; Projects []LegacyProject }` (each carries exactly the fields the conversion reads: ids, kind/type, parent, order, name, workspace, repo, project, home flags, owning chat, git `ParentID`, `CreatedAt`); `migratetree.Step6Impl(repo *repotree.Repository, src LegacySources) migrate.Impl`.

- [ ] **Step 1: Write the failing tests.**

`repository_test.go` (temp-dir SQLite event and snapshot stores, the app's `newAsynx` options):
```go
func TestRepository_SeedThenGetReturnsTheTree(t *testing.T)
func TestRepository_SeedRefusesAnExistingTree(t *testing.T)            // ErrExists
func TestRepository_PutCreatesThenRepositionsANode(t *testing.T)
func TestRepository_DeleteOfAnUnknownNodeIsNotFound(t *testing.T)      // ErrUnknownNode
func TestRepository_GetOfAnAbsentProjectIsAnEmptyTree(t *testing.T)
func TestRepository_ForgetRemovesTheTree(t *testing.T)                 // Get after Forget → empty
```
`ports_test.go`: the `Nodes`/`Folders` contract the tree usecase relies on, against a real repository: `TestPorts_CreateGetListByParentRoundTrip`, `TestPorts_FolderSaveKeepsItsNameOnItsNode`, `TestPorts_ChatPlacementWritesTheChatsNode`, `TestPorts_LocateFindsTheProjectOfAFreshChat` (no node yet; resolved through its workspace).
`step6_test.go` writes a legacy home with test helpers in `legacy_test.go` (`writeLegacyChat(t, es, legacyChat)` and `writeLegacyNode(t, es, legacyNode)` send through a test-only asynx instance typed with the legacy struct, so the events carry real `parentId`/`order` patches; `insertFolder(t, db, LegacyFolder)` writes the `folders` table):
```go
// Review Focus 1: every legacy shape survives verbatim.
func TestPlacementTreeStep_CopiesEveryLegacyPlacement(t *testing.T)
// a home chat with a Node row, a repo chat with only Chat.ParentID/Order, a repo-scoped folder,
// a home folder with HomeID, a home folder written before HomeID (adopted by the home its
// neighbours name, else the oldest project), a repo node in a home folder, a workspace node,
// a fork's owning chat node, two siblings with equal order → each lands in its project's Tree
// with the same parent, order, kind and name.
func TestPlacementTreeStep_DeletesTheNodeFilesAndTheFoldersTable(t *testing.T)
func TestPlacementTreeStep_IsIdempotentAfterACrashMidway(t *testing.T) // Mark after project 1, rerun → project 2 seeded, project 1 untouched
func TestPlacementTreeStep_VerifyFailsOnAChatWithoutANode(t *testing.T)
func TestPlacementTreeStep_PlanCountsWhatItWillConvertAndDelete(t *testing.T)
```
`api/internal/app/repositories/chat/upcast_test.go` (the file named for the upcaster's source): `TestStripRetiredPlacementFields_DropsParentIDAndOrderPatches`, and a replay test: a chat whose history holds `replace /order` loads without error after the bump.

- [ ] **Step 2: Run them to verify they fail.** `GOTEST ./internal/app/repositories/tree/... ./internal/app/migratetree/... ./internal/app/repositories/chat/...`. Expected: compile failure.

- [ ] **Step 3: Implement.** The repository and commands as specified. `ReadLegacy` per index R13: the chat read model gives ids; a private `asynx` instance typed `legacyChat` over the chat event store (same schema version, the app's chat upcasters, a keep-all upcaster at the strip's version, no snapshot store, no subscribers) gives each chat's `ParentID`/`Order`; a private instance typed `legacyNode` over `events/node.db` (ids from `store/node.db`'s `nodes_read`) gives Node rows; `SELECT id, name, repo_id, home_id FROM folders` gives folders; workspaces, repos and projects come from the app's repositories. Conversion is verbatim: a node per Node row, a node per chat that has placement and no Node row, a folder's `Name` onto its node; project of each node by `Locator`'s rules; a legacy home folder with no `HomeID` goes to the home its neighbours name (today's `adoptHomeFolder` walk), else the oldest project by `CreatedAt`. `Apply`: per project not yet `Done`, `Seed`, `Mark`; then remove the three node files and `DROP TABLE folders`. `Verify`: every chat, folder, repo and non-home workspace that had placement has one node in its project's Tree; no node names a missing parent within its scope; the node files and the table are gone. Wire the tree usecase's ports to `repotree.Ports`. The usecase's planning code is untouched in this task.

- [ ] **Step 4: Run the tests.** Step 2's packages plus `GOTEST ./internal/app/usecases/chat/... ./internal/app/... ./internal/adapter/...` and `GOBOX 'TestTree_'` (T1's pin must pass unchanged). Expected: PASS. Then `make lint`, `make deadcode`, `go vet -tags noEmbed ./...`, `go vet -tags 'integration noEmbed' ./tests/...`.

- [ ] **Step 5: Commit.**

```bash
git add api/internal/domain/tree api/internal/app/repositories/tree api/internal/app/migratetree <each modified path>
git rm -r api/internal/app/repositories/node api/internal/domain/folder.go <each deleted path>
git commit -m "refactor(tree): one Tree aggregate per project owns placement; Node, folders and persisted chat placement go

Step placement_tree copies today's placement verbatim and deletes the node files and the folders table.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: The Tree decides: explicit containers, rules, atomic moves

**Files:**
- Modify: `api/internal/domain/tree/tree.go`; create `api/internal/domain/tree/{rules.go,verbs.go,resolve.go}`; tests `rules_test.go`, `verbs_test.go`, `resolve_test.go`.
- Modify: `api/internal/app/repositories/tree/commands.go` (replace `Put`/`Delete` with `Insert`, `Move`, `Rename`, `Remove`, `Promote`, `Replace`; each `Validate` calls the domain verb on `current`, `EmitEvent` returns its result), `repository.go`; delete `ports.go` and its test.
- Create: `api/internal/app/usecases/tree/{service.go,legacy.go}`; tests `service_test.go`, `legacy_test.go`.
- Create: `api/internal/app/migratetree/step7.go`, test `step7_test.go`; modify `catalog.go` (`StepTreeContainers`, post-open, in-layout, name `tree_containers`) and `legacy_steps.go`.
- Modify: `api/internal/domain/problem/code.go` (the six codes of R19, 409; replace whatever codes S3 T1 gave today's tree errors: `grep -n 'Cycle\|CrossWorkspace\|ForkChain' api/internal/domain/problem`), `web/src/lib/errors/` problem mirror (the same six codes and their user wording; R19, V4).
- Modify: the HTTP handlers of today's placement routes (`endpoints/chat/handlers/{chats.go,folders.go}` `PlaceChat`/folder CRUD for repo and home; `endpoints/workspace/handlers/placement.go`; `endpoints/repos/handlers/repos.go` order/folderId) to call `usecases/tree` through `FromLegacy`'s request translation; the DTO builders (`dto/agent.go`, `dto/workspace.go`, `dto/chat_worktree.go`, repo DTO) take placement from `ToLegacy`.
- Modify: `api/internal/app/usecases/chat/internal/tree/` verbs that create, delete or promote (`chats.go` `CreateChat`, `owning_chat.go`, `imported_chat.go`, `delete_chat.go`, `shared_workspace.go`) to call `usecases/tree` (`Place`, `Unplace`, `Promote`) instead of the planner.
- Delete: `api/internal/app/tree/` (all); in `api/internal/app/usecases/chat/internal/tree/`: `plan.go`, `walk.go`, `tree_snapshot.go`, `forest_scope.go`, `home_forest.go`, `legacy_rows.go`, `write_node.go`, `home_ports.go`, `validate.go`, `fork_chain.go`, `place_workspace.go`, `subtree.go`, `guard_not_working.go`, `errors.go`'s placement errors, and their tests; `tree.go`'s `Create`/`Rename`/`Move`/`Delete`/`ListInRepo`/`ListInHome`/`FolderScope`.

**Interfaces:**
- Consumes: T2's repository and types.
- Produces: the `domain/tree` verbs, `Facts`, `ResolveWorkspace`, `Ancestors` and errors (index); repository verbs `Insert(ctx, projectID string, n tree.Node, parentID string, index int, f tree.Facts)`, `Move(ctx, projectID, id, parentID string, index int, f tree.Facts)`, `Rename(ctx, projectID, id, name string)`, `Remove(ctx, projectID, id string, promote bool) (tree.Tree, []string, error)`, `Promote(ctx, projectID, chatID, workspaceID string, f tree.Facts)`, `Replace(ctx, t tree.Tree)`; `usecases/tree.Service` (index); `type LegacyPlacement struct { ProjectID string; Nodes []tree.Node }` and `FromLegacy`/`ToLegacy` (R11).
- `Facts` port (`usecases/tree`): `type Facts interface { ChatWorkspace(ctx, chatID string) (string, error); ForkParent(ctx, wsID string) (string, error); RepoHome(ctx, repoID string) (string, error); ProjectHome(ctx, projectID string) (string, error); Working(ctx, chatID string) (bool, error) }`, implemented in `usecases/container.go` over the chat, workspace and project usecases (Working reads the predicate the chat entity's server-folded `working` comes from).

- [ ] **Step 1: Write the failing domain tests.** Table tests, one case per rule of index R4 plus ordering:

`rules_test.go`:
```go
func TestMove_RefusesACycle(t *testing.T)                        // onto itself; under its own descendant → ErrCycle
func TestMove_RefusesAFolderUnderAChat(t *testing.T)             // ErrNotAContainer
func TestMove_RefusesARepoBelowProjectLevel(t *testing.T)        // repo under a workspace → ErrNotAContainer; under a home folder → ok
func TestMove_KeepsANodeInItsContext(t *testing.T)               // repo-scoped folder to home level, home chat into a repo → ErrCrossContext
func TestMove_RefusesAChatLeavingItsWorkspace(t *testing.T)      // thread under another branch → ErrCrossWorkspace; into a folder of the same workspace → ok
func TestMove_KeepsAForkUnderItsForkParent(t *testing.T)         // fork filed under its parent's sibling → ErrForkChain; into a folder inside the parent → ok
func TestMove_RefusesASubtreeHoldingAWorkingChat(t *testing.T)   // Facts.Working → ErrSubtreeWorking
func TestMove_TreatsFoldersAsTransparentForResolution(t *testing.T)
```
`verbs_test.go`:
```go
func TestMove_RenumbersBothLevelsDensely(t *testing.T)           // orders 0..n-1 at the old and the new parent, nobody else touched
func TestMove_WithinOneLevelReorders(t *testing.T)
func TestInsert_AtAnIndexShiftsLaterSiblings(t *testing.T)
func TestInsert_RefusesAnExistingID(t *testing.T)                 // ErrExists
func TestRemove_PromoteMovesChildrenToTheFoldersPlace(t *testing.T) // children keep their relative order at the folder's index
func TestRemove_CascadeReturnsTheWholeSubtree(t *testing.T)
func TestRename_RequiresANameAndAFolder(t *testing.T)             // "" → ErrNameRequired; a chat id → ErrNotAContainer
func TestPromote_ReplacesAChatNodeWithAWorkspaceNodeInPlace(t *testing.T) // same parent and order; the chat's children now under the workspace node
func TestVerbs_NeverMutateTheReceiver(t *testing.T)
```
`resolve_test.go`: `TestResolveWorkspace_WalksToTheNearestWorkspaceThenRepoThenHome`, `TestAncestors_StepsThroughFoldersAndCountsAWorkspaceAsItsOwningChat`, `TestAncestors_OfARootChatIsEmpty`.

- [ ] **Step 2: Run them to verify they fail.** `GOTEST ./internal/domain/tree/...`. Expected: compile failure.

- [ ] **Step 3: Implement the domain.** `Children` sorts by `Order`, then `ID`. A verb validates first (all rules over the moved subtree with `f`), then copies the map and renumbers. Errors are `*problem.Error` values with the R19 codes; messages are the user's wording (V4's for `ErrSubtreeWorking`).

- [ ] **Step 4: Write the failing service, legacy and step tests.**

`service_test.go` (real repository, fake `Facts`): `TestService_MoveGathersFactsForTheWholeSubtree`, `TestService_CreateFolderUsesTheClientID`, `TestService_DeleteFolderPromotesItsChildren`, `TestService_UnplaceReturnsEveryRemovedChat`, `TestService_AncestorsReadTheChatsProjectTree`.
`legacy_test.go`: `TestFromLegacy_MapsEachScopeRootToItsContainer` (home `""` → root; a repo chat's `""` → the repo node; a branch chat's `""` → its workspace node; a chat whose parent is a branch's owning chat → that workspace node; a workspace with no folder → its fork parent's node, else the repo node), `TestFromLegacy_OwningChatNodesBecomeTheirWorkspaceNode`, `TestToLegacy_IsTheInverseOnEveryPinnedRow` (T1's `PIN` hierarchy built both ways).
`step7_test.go`: `TestTreeContainersStep_KeepsTheRenderedHierarchy` (Review Focus 1: a step-6 tree of T1's `PIN` shape plus the legacy ambiguities → explicit containers whose `ToLegacy` equals the input), `TestTreeContainersStep_IsIdempotent`, `TestTreeContainersStep_VerifyRefusesAnImplicitRoot`.

- [ ] **Step 5: Run them to verify they fail.** `GOTEST ./internal/app/usecases/tree/... ./internal/app/migratetree/...`.

- [ ] **Step 6: Implement.** Service per index; `Move` reads the tree, computes `Subtree(nodeID)`, gathers `Facts` for its chats and workspaces, calls the repository. `FromLegacy` encodes R4's mapping (one function, used by step 7 and by the legacy routes). `ToLegacy` is its inverse for the DTOs. Step 7: per project not `Done`, read, `FromLegacy`, `Replace`, `Mark`; `Verify`: every non-root `ParentID` names a node of the same tree and `domain/tree`'s rules hold for every node with the current facts. Re-point the create/delete/promote verbs; delete the planner files listed above.

- [ ] **Step 7: Run everything touched.** `GOTEST ./internal/domain/... ./internal/app/... ./internal/api/...`, `GOBOX 'TestTree_'` (T1's pin passes unchanged except the refusal codes, renamed per R19 in this commit), web `bunx vitest run src/__tests__/lib/errors`. Then the Go and web gates.

- [ ] **Step 8: Commit.**

```bash
git commit -m "refactor(tree): the Tree decides moves with explicit containers; the placement planner goes

Step tree_containers rewrites every Tree; today's routes keep their shapes through FromLegacy/ToLegacy until the client moves to the Tree.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```
(`git add`/`git rm` by path as in T2.)

---

### Task 4: The chat tree usecase goes

**Files:**
- Move: the create/delete/promote verbs left in `api/internal/app/usecases/chat/internal/tree/` into the chat usecase, one file per verb family: `api/internal/app/usecases/chat/{create_chat.go,delete_chat.go,owning_chat.go,import_chat.go}` with their tests (moved, not rewritten; cases for deleted behaviour go).
- Modify: every caller of `agentusecase.TreeUsecase` (`usecases/container.go` `AgentChatFolder`, `endpoints/chat/handlers`, `usecases/worktree`, `usecases/internal/branchimport`, `usecases/project/{owning_chats.go,home_level.go}`) to call the chat usecase or `usecases/tree`; `usecases/chat/chat.go` `Ancestors` → `usecases/tree.Service.Ancestors`; `usecases/worktree/chat_forest.go` reads the Tree.
- Modify: `api/internal/domain/chat.go` (delete `ParentID`, `Order`), `usecases/chat/{home_read.go,placement_reads.go}` (delete the overlay; delete `placement_reads.go` if empty), and every reader the compiler names; DTO placement comes only from `ToLegacy`.
- Delete: `api/internal/app/usecases/chat/internal/tree/` (whole package, incl. `internal/lineage`), `TreeUsecase` and its mock.

**Interfaces:**
- Consumes: T3's `usecases/tree.Service`.
- Produces: the chat usecase methods the handlers already call, unchanged in signature (`CreateChat`, `DeleteChat`, `MintOwningChat`, `AttachOwningWorkspace`, `DiscardOwningChat`, `ImportBranchAsChat`); each creates the entity, then `tree.Place`; a failed `Place` discards the entity and returns the `Place` error (index spec issue 2); a delete runs `tree.Unplace` and deletes every returned chat.

- [ ] **Step 1: Write the failing tests** in the moved test files: `TestCreateChat_AFailedPlaceDiscardsTheChat`, `TestDeleteChat_DeletesEveryChatTheTreeRemoves`, `TestMintOwningChat_TheWorkspaceNodeKeepsItsPlace`; and `api/internal/app/usecases/chat/chat_test.go` `TestAncestors_ComeFromTheTree`.
- [ ] **Step 2: Run them to verify they fail.** `GOTEST ./internal/app/usecases/chat/...`.
- [ ] **Step 3: Implement.** Move the verbs; delete the package; delete the fields; fix every compile error by reading the Tree, never by restoring a field.
- [ ] **Step 4: Run.** `GOTEST ./internal/app/... ./internal/api/... ./internal/domain/...`, `GOBOX 'TestTree_'`, Go gates. Check `grep -rn 'chat/internal/tree\|\.ParentID' api/internal/app/usecases/chat --include='*.go'` lists no tree package import and no chat placement read.
- [ ] **Step 5: Commit.**

```bash
git commit -m "refactor(chat): chat lifecycle verbs place rows through the Tree; the chat tree usecase and chat placement fields go

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```
