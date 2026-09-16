# Sidebar Placement Unification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One entity (`domain.Node`) owns the `ParentID`/`Order` of every sidebar row — chat, folder, workspace (locked branch / repo-home / project-home), and repo — at every tree level. This closes the repo-ordering bug class for good (no more two aggregates racing over one sibling space) and retires the machinery that exists only to give a non-chat thing a chat row to hang a position off of.

**Architecture:** Implements `docs/superpowers/specs/2026-09-08-sidebar-placement-unification-design.md` in full. Read it first — this plan does not repeat its reasoning, only its instructions. Where this plan's task detail and the spec disagree, the spec wins; record the conflict in the SDD ledger as a ruling either way.

**Research baseline:** `docs/superpowers/plans/.node-migration-research.md`, captured at HEAD `a940a4689`, has exact current line numbers, the full asynx command template (`Chat`'s own `create.go`/`set_order.go`/`set_placement.go`/`event_store.go`/`store.go`), the actual (larger than the spec's summary) `owning_rows.go`/`backfill.go` machinery and its 15-file consumer blast radius, and the frontend's current state including `home-tree.ts` (a store surface the spec didn't name). **Read it before writing or dispatching any task brief** — it corrects several of the spec's simplifying assumptions (noted inline below where it does). Line numbers in that file will drift as tasks land; each task's own Step 1 re-reads the real file before trusting a cited number.

**Tech stack:** Go (asynx event-sourced aggregates + plain GORM entities, gin handlers), TypeScript/React (Zustand-adjacent stores, dnd-kit tree).

**Spec:** `docs/superpowers/specs/2026-09-08-sidebar-placement-unification-design.md` — §2 (the target model), §3 (phased migration, which this plan's task grouping follows), §4 (the deletion list), §6 (open questions this plan's tasks resolve as they go).

## Global Constraints

- **Command granularity.** Every write to `Node` is either `SetOrder{Order}` (this row only, order changes, parent doesn't) or `SetPlacement{ParentID, Order}` (this row moved). Never re-state a caller-read `ParentID` on a row a densify pass is merely shifting — this is the exact race `Chat`'s own `SetOrder` doc comment documents from production. Mirror `internal/commands/set_order.go`/`set_placement.go` (`agentchat` package) literally — copy `*current`, mutate only the field(s) named.
- **`ChatTypeWorkflow` survives.** `domain.ChatType`'s closed taxonomy is four values today (`chat`, `branch`, `folder`, `workflow` — confirmed via `domain/chat_type_test.go`'s `TestChatType_ClosedTaxonomy`), not the three the spec's prose assumes. Every task that narrows the taxonomy narrows it to `{ChatTypeChat, ChatTypeWorkflow}` — `ChatTypeWorkflow` has no v0 writer yet but is not this plan's to remove.
- **The four-tier context-anchor invariant is not the spec's two-tier summary.** `validate.go`'s `nearestWorkspaceAnchor` enforces `Project → Repo → Locked branch → Parent unlocked branch` — a folder may reorder freely within whatever anchor it already sits under, but never jump to a different locked branch's subtree or out to the bare repo root, even though a repo-scope check alone would allow it. This is pinned by a live-caught bug (the function's own doc comment) and by existing tests. Whichever task ports the golden rule to `Node` (Task 8) must preserve all four tiers exactly — do not simplify to "home vs. repo."
- **`-tags noEmbed` on every Go command.** `go build -tags noEmbed ./cmd/crowbar`, `go vet -tags noEmbed ./...`, `go test -tags noEmbed -race -count=1 ./...` (scoped to touched packages during a task; the full run is Task 11's job). Per this repo's standing hazard memory, a plain `go build ./...` without the tag can pass while the tagged build fails — never cite the untagged form in a task brief.
- **`bun tsc`, not `bunx tsc`** (a different package). Vitest scoped to touched paths (`bun vitest run <path>`) during a task — never the bare full-repo run. `bun run lint` for the final sweep only.
- **Container wiring is three files, not one.** `api/internal/app/container.go` (`app.New`, constructs every per-type asynx singleton, then calls `repositories.New`), `api/internal/app/repositories/container.go` (`repositories.New`, builds each aggregate's `EventStore`/`Store`, assigns `Container` fields), `api/internal/app/usecases/container.go` (`usecases.New`, builds usecases from the `*repositories.Container`, not read in the research pass — read it fresh in Task 1). A task that adds a new aggregate touches all three.
- **Every task needs a regression test that fails on pre-fix code.** Black-box `TestRegression_*` in `api/tests/` for cross-layer behavior (wire contracts, boot-time wiring); package-level tests otherwise, following the existing test's own placement convention for whatever it's replacing (see research §9 for current file locations).
- **No fallback/dead branches for a case that can't happen post-migration.** E.g. once Task 7 makes every `Workspace` mint a `Node{Kind:workspace}` row unconditionally at creation, code reading a workspace's row must not also carry an "if missing, do X" branch — a missing row is a bug to surface loudly, not paper over (mirrors this repo's own `docs/superpowers/plans/2026-09-01-owning-chat-backfill.md`'s identical rule, Global Constraints, 4th bullet).
- Same repo-wide conventions as the rest of this codebase: kebab-case component files, tests mirror `web/src/__tests__/` with `@/` imports (no `features/X/tests/` directories), narrow store selectors (`useXxxStore((s) => s.field)`, never bare `useXxxStore()`), stores never import from `components/`.
- **Never bare `git stash`** — this worktree is shared with other sessions.

---

## Task 1: Backend — `domain.Node`, the position aggregate

**Files:**
- Create: `api/internal/domain/node.go` (`Node`, `NodeKind` + its four const values)
- Create: `api/internal/app/repositories/node/` — mirror `api/internal/app/repositories/chat/`'s structure exactly: `node.go` (`ErrNotFound`), `event_store.go` (`EventStore` interface + `eventSourced` impl, `occSend`/`sendWithOCC` copied from `agentchat`'s own — confirmed by research to be per-package boilerplate, not a shared helper), `internal/commands/{create,set_order,set_placement}.go`, `internal/store/{store.go,hub.go,storage.go}` (read-model projection + live-update broadcast, mirroring `agentchat`'s `store.New`/`registerStoreProjection` pattern — the frontend needs live position updates for cross-tab/cross-client drag sync, same as chat rows already get).
- Modify: `api/internal/app/container.go` (`app.New`) — construct `axNode asynx.Asynx[domain.Node]` via `newAsynx[domain.Node](adapters.NodeES(), adapters.NodeSS())`, mirroring the existing `axAgentChat` construction line exactly; pass it into `repositories.New(...)`.
- Modify: wherever `adapters.AgentChatES()`/`AgentChatSS()` are defined (grep `adapter.Container` — research didn't trace this file) — add `NodeES()`/`NodeSS()` alongside them.
- Modify: `api/internal/app/repositories/container.go` (`repositories.New`) — add `axNode` as a new constructor parameter, call `node.NewEventSourced(axNode, adapters.NodeES(), adapters.NodeReadDB(), nodeWatch)` mirroring the `agentchat.NewEventSourced` call, assign to a new `Container.Node` field. Add `axNode.WaitPublish()` to `WaitQuiescent` (research: every per-type asynx instance is drained there; a test using `WaitQuiescent` as its read-your-writes barrier will flake without this).
- Modify: `api/internal/app/usecases/container.go` (`usecases.New`) — read this file fresh (not traced in the research pass) to see how other repository-backed usecases are constructed from `*repositories.Container`; wire a `Node`-backed usecase surface the same way (this task creates only the repository layer's `EventStore`/read-model — a thin usecase wrapper if this repo's convention always has one between repository and API handler, otherwise skip if repositories are consumed directly elsewhere).
- Test: `api/internal/domain/node_test.go`, `api/internal/app/repositories/node/*_test.go` (mirror `agentchat`'s own test file names and shapes).

**Interfaces:**
```go
// api/internal/domain/node.go
type NodeKind string

const (
	NodeKindChat      NodeKind = "chat"
	NodeKindFolder    NodeKind = "folder"
	NodeKindWorkspace NodeKind = "workspace"
	NodeKindRepo      NodeKind = "repo"
)

type Node struct {
	ID       string   `gorm:"primaryKey"`
	Kind     NodeKind
	ParentID string
	Order    int
}
```
- `EventStore` surface (mirror `agentchat.EventStore`, narrowed to what Node needs): `Create(ctx, id, kind, parentID, order) (domain.Node, error)` (SendWait — every later task that mints a row needs the read-your-write barrier the way `agentchat.Create` already provides it), `SetOrder(ctx, id, order) error` (async Send), `SetPlacement(ctx, id, parentID, order) error` (async Send), `GetNode(ctx, id) (domain.Node, error)`, `ListByParent(ctx, parentID) ([]domain.Node, error)` (the sibling-space read every densify pass needs), `Forget(ctx, id) error` (hard delete via `ax.Forget` — mirror `agentchat.Forget` exactly; **do not** build a separate `Delete` domain command with `Validate`/`EmitEvent` — there is no "next state" to emit for a deletion, `Forget` is the asynx-native primitive for this and `agentchat` already establishes the pattern; this corrects spec §2.5's assumption of a `Delete{}` command).
- Consumed by: Tasks 3, 5, 6, 7, 8, 10 (every later task reads/writes `Node` through this surface — nothing else may touch its GORM table directly).

- [ ] **Step 1: Read `api/internal/app/repositories/chat/event_store.go`, `internal/commands/{create,set_order,set_placement}.go`, and `internal/store/store.go` in full** (all captured verbatim in the research file — read the research file first, then confirm against the live files, since research budget did not permit reading `hub.go`/`storage.go` in full). Also read `api/internal/app/usecases/container.go` and `api/internal/app/asynx.go`'s tail (past line 31, not read in research) to see `newAsynx[T]`'s final `.Build()` call and how a usecase wraps a repository today.

- [ ] **Step 2: Write failing tests first** — a `Node` created via `Create` round-trips through `GetNode`; `SetOrder` changes only `Order` (a concurrent `SetPlacement` reading stale `ParentID` from before the `SetOrder` lands must not be clobbered — this is the exact race `agentchat.SetOrder`'s doc comment describes; write a test that reproduces it against `Node` the same way, if `agentchat` has one, mirror it); `SetPlacement` changes both fields; `ListByParent` returns siblings in no particular guaranteed order (the caller densifies); `Forget` hard-deletes (a `GetNode` after returns `ErrNotFound`).

- [ ] **Step 3: Implement**, mirroring the `agentchat` package's exact structure and the `occSend`/OCC-retry pattern (`maxOCCAttempts = 8`, `ErrValidation` never retried/422, `ErrQueueFull` never retried/503, `ErrPipelineFailed` retried).

- [ ] **Step 4: Wire through all three container layers** per the Files list above. Confirm `go build -tags noEmbed ./cmd/crowbar` succeeds with the new aggregate constructed but nothing else in the codebase referencing it yet (this task is purely additive).

- [ ] **Step 5: Run `go test -tags noEmbed -race -count=1 ./internal/domain/... ./internal/app/repositories/node/... ./internal/app/repositories/...`.** Confirm PASS.

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(node): add the Node position aggregate — one entity owns every sidebar row's ParentID/Order"
```

---

## Task 2: Backend — `domain.Folder`, plain GORM

**Files:**
- Create: `api/internal/domain/folder.go` (`Folder{ID, Name, RepoID}` — `RepoID == ""` means a project-home folder)
- Create: the store/repo wiring — read `api/internal/adapter/store/*.go` first (research flagged this as unread — find the generic GORM store type used by `domain.Repository`, seen as `store.ScopedStore[domain.Repository, string]` in `project.go:151`) and use the SAME generic constructor for `Folder`, not a bespoke implementation.
- Modify: `api/internal/app/repositories/container.go` — wire `Container.Folder` alongside the existing `Repository` store construction, following its exact pattern (no asynx — plain CRUD, per spec §2.2's reasoning: a rename has no drag-time densify race to guard and no motivated undo case).
- Test: `api/internal/domain/folder_test.go`, plus the store wiring's own test (mirror `Repository`'s store test file, wherever it is).

**Interfaces:**
```go
// api/internal/domain/folder.go
type Folder struct {
	ID     string `gorm:"primaryKey"`
	Name   string
	RepoID string
}
```
- Consumed by: Tasks 5, 6, 8, 10.

- [ ] **Step 1: Read `domain.Repository`'s full store wiring** (`api/internal/adapter/store/*.go`, plus wherever `Container.Repository`/the repo GORM store is constructed in `repositories/container.go`) to confirm the exact generic constructor call shape.

- [ ] **Step 2: Write failing tests** — CRUD round-trip (create, get, rename, list-by-repo, list-project-home i.e. `RepoID==""`, delete).

- [ ] **Step 3: Implement**, mirroring `Repository`'s wiring exactly — same generic store type, same `TableName()` convention, same `Container` field pattern.

- [ ] **Step 4: Run `go test -tags noEmbed -race -count=1 ./internal/domain/... ./internal/app/repositories/...`.** Confirm PASS. Confirm `go build -tags noEmbed ./cmd/crowbar` still succeeds — this task, like Task 1, is purely additive.

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(folder): add the Folder entity — plain GORM, no chat-shaped fields"
```

---

## Task 3: Backend — repos migrate onto `Node`

**Depends on:** Task 1.

**Files:**
- Modify: `api/internal/domain/repository.go` — delete `Order`, `FolderID` fields (today's interim Phase-A fields).
- Modify: `api/internal/app/usecases/project/project.go` — `UpdateRepo` (l.240-299 as of research) routes an `in.Order != nil` update through `Node.SetPlacement`/`.SetOrder` on the repo's own `Node{Kind:repo}` row. **Sequencing constraint, not in the original draft of this task — read carefully:** home chats/folders are NOT yet Node-backed at this point in the plan (that's Task 5). A repo's sibling space at project home still includes real chats/folders whose position lives on `Chat.ParentID`/`.Order` until Task 5 lands. Deleting the cross-aggregate merge entirely here would either break "repo interleaves into a home folder among real children" or silently make repo placement blind to chat/folder siblings — reintroducing the exact bug class this plan exists to fix, as a temporary regression. **Do not delete `homeContainerChats` or the merge shape in this task.** Instead, adapt `placeRepoInHomeContainer`'s existing merge so the REPO side reads/writes through `Node.ListByParent`/`.SetOrder`/`.SetPlacement` (replacing `Repository.Order`/`densifyRepos` for that one kind), while the CHAT/FOLDER side keeps reading/writing through the existing `homeContainerChats`/`Chat.SetOrder` path, unchanged. Rename/reshape `placeRepoInHomeContainer` as needed to reflect it now merges a `Node`-backed kind with a `Chat`-backed kind rather than two `Chat`/`Repository` reads, but keep the merge itself alive. Task 5 is the one that deletes this merge for good, once chats/folders are ALSO Node-backed and everyone reads from one `Node.ListByParent` call. `validateRepoFolder` (l.317-334 — its cross-project home-folder gap, spec §6 Open Question 2, is superseded: a `Node{Kind:repo}` row's `ParentID` is validated the same way every other kind's is, by whichever task lands the golden rule's Node-side equivalent — Task 8; if Task 8 hasn't landed yet when this task runs, port `validateRepoFolder`'s existing check narrowly rather than blocking on Task 8).
- Modify: wherever a repo is first imported/created (find the repo-import usecase — not traced in research; grep `project.go`/`repos.go` for the creation path) — mint a `Node{Kind:repo, ID: repo.ID}` row via `Node.Create` at the same point a `Repository` row is created.
- Modify: `WorkspaceRelocator`/`HomeFolders` interfaces (`project.go`) — narrow back down now that the cross-aggregate merge is gone; these were widened by today's interim fix specifically to read chat/folder siblings — Node's `ListByParent` replaces that read entirely, so the widened surface (`ListByWorkspace`, `ListChats`, `SetOrder` added to `HomeFolders`) should shrink back to what `UpdateRepo` actually needs post-migration (likely just `Get`, plus whatever the repo-import path needs).
- Test: `api/internal/app/usecases/project/ordering_test.go` (has today's `TestUpdateRepo_PlacesAgainstHomeChatsToo` — this becomes a Node-based test proving the same bug stays fixed, not a chat-sibling-merge test), `owning_chats_test.go`/`owning_chats_fake_test.go` (research flagged these as possibly `placeRepoInHomeContainer`'s dedicated fixtures — read them first; delete or repurpose accordingly).

**Interfaces:**
- Consumes: Task 1's `Node.Create`/`.SetOrder`/`.SetPlacement`/`.ListByParent`.
- Produces: no new wire shape — `RepoDTO`'s `folderId`/`order` fields (added by today's interim fix) now read from the repo's `Node` row instead of `Repository.FolderID`/`.Order` directly; find and update the DTO population path (`api/internal/api/v0/dto/repo.go`) accordingly.

- [ ] **Step 1: Read `project.go`, `ordering.go` in full at current HEAD** (research has the full text as of `a940a4689` — re-read live to catch drift) plus the repo-import usecase (not traced — find it).

- [ ] **Step 2: Write the failing regression test first** — the ORIGINAL bug, now against `Node`: a single-repo project, drag the repo to a non-zero position, assert it lands there and stays there (not clamped to 0). Also port `TestUpdateRepo_PlacesAgainstHomeChatsToo`'s two subtests (repo sorts after/before a home chat, repo interleaves into a home folder among real children) to prove the Node-based path gets the same right answer today's interim fix does.

- [ ] **Step 3: Implement.** `place()`/`reinsert()` (`ordering.go`) are reused unchanged — per spec §2.1 and the research's confirmation, they operate on an abstract `[]slot`, not on `Repository`/`Chat` specifically; the only new code is building that `[]slot` from `Node.ListByParent` and writing changed rows back via `Node.SetOrder`/`.SetPlacement` (mirroring `plan.go`'s `writeRow`'s `if !Reparented(id) { SetOrder } else { SetPlacement }` dispatch — this is the literal per-row decision this task's write-back must replicate, per research §5).

- [ ] **Step 4: Run `go test -tags noEmbed -race -count=1 ./internal/app/usecases/project/... ./internal/domain/...`.** Confirm PASS, confirm the ORIGINAL clamp-to-0 bug's regression test goes RED on a revert and GREEN on this task's code.

- [ ] **Step 5: Commit**

```bash
git commit -m "fix(project): repos place through Node, not a chat-sibling cross-aggregate merge"
```

---

## Task 4: Frontend — repo rows read/write through `Node`

**Depends on:** Task 3 (needs the wire contract it produces).

**Files:**
- Modify: `web/src/components/sidebar/lib/rows-from-home.ts` — delete `HomeRepoPlacement`/`HomeRepoPosition`/the `repoPositions` stand-in-injection return shape; a repo's position is now read directly off the wire the same way a chat's or folder's is.
- Modify: `web/src/components/sidebar/space-scroller.tsx` — delete the post-hoc `repoPositions` overwrite of each repo row's `parentId`/`order`.
- Modify: `web/src/components/layout/workspace-tree-utils.ts` and/or `rows-from-repo.ts` as needed for `buildSidebarTree`'s repo-stand-in (`repoStandins: SidebarChat[]`) mechanism to be replaced by reading the repo's real position fields.
- Test: `web/src/__tests__/components/sidebar/lib/rows-from-home.test.ts`, `.../space-scroller.test.tsx`.

**Interfaces:**
- Consumes: Task 3's repo DTO shape (position sourced from `Node`, same wire field names `folderId`/`order` as today — no breaking wire change, only where the backend reads them from).

- [ ] **Step 1: Read the current `rows-from-home.ts`, `space-scroller.tsx` in full** (research has both captured; re-read live for drift) to find every place today's stand-in mechanism is threaded through.

- [ ] **Step 2: Write the failing test first** — same drag-repo-to-non-zero-position scenario as Task 3's backend test, now asserting the RENDERED row order matches, with the stand-in machinery deleted (a test that currently passes only because of the post-hoc `repoPositions` overwrite must still pass without it).

- [ ] **Step 3: Implement.** Delete the stand-in/overwrite code paths; confirm nothing else in the render path still expects `rowsFromHome`'s old `{rows, repoPositions}` return shape (search for other call sites) — if none, simplify the return to a plain `rows` array again.

- [ ] **Step 4: Run `bun vitest run web/src/__tests__/components/sidebar/lib/rows-from-home.test.ts web/src/__tests__/components/sidebar/space-scroller.test.tsx`.** Confirm PASS.

- [ ] **Step 5: Run `bun tsc` and `bun run lint` scoped to touched files.** Confirm clean (aside from pre-existing unrelated failures, verified via `git stash` comparison against unmodified HEAD).

- [ ] **Step 6: Commit**

```bash
git commit -m "fix(sidebar): repo rows read position from the wire directly, no more stand-in injection"
```

---

## Task 5: Backend — project-home chats/folders migrate onto `Node`/`Folder`

**Depends on:** Task 1, Task 2.

**Files:**
- Modify: `api/internal/app/usecases/chat/internal/tree/tree.go`'s folder CRUD (`ListInRepo`, `Create`, `placeNewFolder`, `discardFolder`, `Rename`, `Move`, `Delete`) — for the home-scoped case (`RepoID == ""`), these now operate on `domain.Folder` (identity/name) + `domain.Node` (position) instead of a `ChatTypeFolder`-typed `Chat` row. Repo-scoped folders (`RepoID != ""`) are Task 8's job — do not touch that path here; this task is scoped to project home only. If the existing functions don't cleanly split by scope, note the seam you introduce in the SDD ledger for Task 8 to extend.
- Modify: wherever a home-scoped chat is placed (`PlaceChat`/`placeChat` in `chats.go`, for the `workspaceID == ""` bubble case, or wherever project-home chat creation routes) — position writes for a home-scoped chat go through `Node.SetOrder`/`.SetPlacement` on a `Node{Kind:chat, ID: chat.ID}` row instead of `Chat.SetOrder`/`.SetPlacement` directly. **Chat's own `ParentID`/`Order` fields are not deleted in this task** (repo-scoped chats still use them until Task 8) — only the home-scoped WRITE path is redirected; a home-scoped chat's `Chat.ParentID`/`.Order` fields become write-once-at-creation-then-ignored for this scope, cleaned up fully in Task 11.
- Modify: `plan.go`'s `globalSnapshot`/`globalSnapshotAround` (whole-forest read for folder CRUD) — for the home scope, read from `Node.ListByParent` + `Folder` instead of `ListByWorkspace("")` + `ChatTypeFolder` rows.
- **Delete Task 3's interim merge.** Task 3 deliberately kept a repo/chat cross-aggregate merge alive (adapted `placeRepoInHomeContainer`, in `project.go`) because home chats/folders weren't Node-backed yet at that point. Now that this task makes them Node-backed too, that merge is no longer needed — every home-scope sibling (chat, folder, repo) reads from ONE `Node.ListByParent` call. Delete the merge helper in `project.go` and have `UpdateRepo`'s `Order` path read siblings the same direct way this task's own chat/folder placement does. This is the point where the unification actually lands for project home — confirm it by checking that `project.go` no longer imports or calls into the chat package for sibling reads at all.
- Test: mirror whatever test file currently covers home-scoped folder CRUD and home-scoped `PlaceChat` (research: no separate `validate_test.go`/`plan_test.go` — coverage lives in `chats_test.go`/`move_test.go`/`tree_test.go`; re-grep before writing the brief).

**Interfaces:**
- Consumes: Task 1's `Node` surface, Task 2's `Folder` surface.
- Produces: no wire-shape change for the home endpoints — same `folderId`/`order`/`parentId` fields, sourced differently server-side.

- [ ] **Step 1: Read `tree.go`'s folder CRUD, `plan.go`, `chats.go`'s home-scoped path, and the actual test file(s) covering them, live at current HEAD.**

- [ ] **Step 2: Write failing tests first** — create/rename/move/delete a home-scoped folder now backed by `Folder`+`Node`; place a home-scoped chat, confirm it densifies correctly against home-scoped folder AND repo `Node` rows in the same sibling space (this is the direct Node-native equivalent of Task 3's now-deleted cross-aggregate merge — proves the whole POINT of the unification: repos, chats, and folders share one sibling read at project home, natively, no merge step needed).

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run the scoped test suite for the `tree` package and `project` package.** Confirm PASS.

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(chat): project-home folders and chat placement migrate onto Node/Folder"
```

---

## Task 6: Frontend — project-home chats/folders read/write through `Node`/`Folder`

**Depends on:** Task 5.

**Files:**
- Modify: `web/src/lib/store/home-tree.ts` — `getHomeTree`, `applyHomeFolders`, `subscribeHomeTree` source folders from `Folder` and positions from `Node` instead of `Chat`-typed folder rows (research: this store was not named in the spec at all — it's the per-project home-tree state surface, genuinely new to this plan, and per this repo's CLAUDE.md ("`lib/store/` is for server-state-adjacent structures") it stays exactly where it is; only its fetch/apply internals change).
- Modify: `web/src/components/sidebar/lib/rows-from-home.ts` — now that Task 4 already removed the repo stand-in mechanism, this task removes the LAST reason `rows-from-home.ts` needs its own bespoke tree-building call at all; if `buildSidebarTree` can take `Node`-sourced rows directly, this file may collapse substantially — confirm and simplify.
- Test: `web/src/__tests__/lib/store/home-tree.test.ts`, `.../rows-from-home.test.ts`.

**Interfaces:**
- Consumes: Task 5's home-scoped wire contract (same field names, new server-side source).

- [ ] **Step 1: Read `home-tree.ts` in full live** (research has it captured at 153 lines; re-read for drift) plus its two consumers (`space-content-actions.ts`, `sidebar-drop-policy.ts`'s `resolveHomeRowScope` usage) to confirm nothing outside this file needs to change.

- [ ] **Step 2: Write failing tests first** — mirror `home-tree.test.ts`'s existing shape, updated for the new source fields.

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run `bun vitest run web/src/__tests__/lib/store/home-tree.test.ts web/src/__tests__/components/sidebar/lib/rows-from-home.test.ts`.** Confirm PASS.

- [ ] **Step 5: Run `bun tsc` and `bun run lint` scoped to touched files.**

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(sidebar): home tree reads folders/positions from Folder/Node"
```

---

## Task 7: Backend — branches (locked / repo-home / project-home workspaces) reference `Workspace` directly via `Node`

**Depends on:** Task 1.

**Files:**
- Modify: wherever a `Workspace` row is created (fork, lock, repo import's default workspace, project's home workspace) — mint a `Node{Kind:workspace, ID: ws.ID}` row unconditionally at creation time, via `Node.Create`. Find every creation call site (grep `workspaces.Create`/equivalent — not traced in research).
- Modify: `api/internal/app/usecases/provider/provider_sync.go` (l.73, l.197 per research) — delete the `EnsureOwningChat` calls entirely. Per research's key finding: under the Node model there is no more "does this now-locked workspace have an owning row" question — a workspace's `Node{Kind:workspace}` row already exists unconditionally from creation, so a provider poll reporting a branch newly protected needs no reactive fixup. This is a pure deletion, not a replacement — do not write a `Node`-flavored equivalent call here.
- Modify: `api/internal/app/usecases/workspace/workspace.go` (l.291, l.452 per research) — same deletion, same reasoning, for the manual-lock path.
- Modify: `api/internal/app/repositories/container.go:515` — the workspace DTO's `owningChatId` field now resolves via a direct `Node`/`Workspace` lookup (the workspace's OWN id is its position row's id — no resolution needed at all; consider whether `owningChatId` as a field name still makes sense or should become `nodeId`/be dropped in favor of the frontend already having the workspace's own id — decide and note the wire-contract change, if any, in the commit).
- Test: wherever workspace creation is tested (fork, lock, repo import) — assert a `Node{Kind:workspace}` row exists immediately after, no separate backfill step needed.

**Interfaces:**
- Consumes: Task 1's `Node.Create`.
- Produces: every `Workspace` has a `Node` row from the moment it exists — this is the invariant Task 8 and Task 9 build on.

- [ ] **Step 1: Read every `Workspace`-creation call site live** (fork, lock/protect, repo import's default workspace, project's home workspace creation) and the two `EnsureOwningChat` runtime call sites in full, at current HEAD.

- [ ] **Step 2: Write failing tests first** — creating a workspace via each path immediately produces a matching `Node{Kind:workspace}` row (no backfill, no lazy mint). Also a regression test proving `provider_sync.go`'s branch-newly-protected path still works end-to-end with `EnsureOwningChat` deleted (nothing breaks from removing a call that's now a no-op).

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run the scoped test suite for `workspace`, `provider`, and `project` packages** (repo/project-home workspace creation lives in `project`). Confirm PASS.

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(workspace): every workspace gets a Node row at creation — no more reactive EnsureOwningChat"
```

---

## Task 8: Backend — repo-internal chats/folders migrate onto `Node`/`Folder`; the golden rule ports to `Node`

**Depends on:** Task 1, Task 2, Task 7.

**Files:**
- Modify: `tree.go`'s folder CRUD — extend Task 5's Node/Folder-backed implementation to the repo-scoped case (`RepoID != ""`), completing the split Task 5 started.
- Modify: `chats.go`'s `PlaceChat`/`placeChat` — repo-scoped chat placement writes through `Node` the same way Task 5 did for home-scoped.
- Modify: `validate.go` — port `checkFolderContextMove`/`nearestWorkspaceAnchor` (l.43-138 per research) to walk `Node.ParentID` instead of `Chat.ParentID`, stopping at the first `NodeKind == workspace` row. **This is simpler under Node** (no `ResolveOwningChat` tiebreak needed — a `workspace`-kind `Node` unambiguously IS the anchor, since Task 7 guarantees exactly one per workspace) **but the four-tier semantic itself (Global Constraints, above) must survive unchanged** — port the existing test suite for this function 1:1 before considering this task done, not just the cases that happen to be easy.
- Port `checkChatContainer`/`checkParentKind` (l.222-313) similarly — folder/workspace parent always OK; the `ownWorktree`-forking-off-an-existing-worktree-owning-row case still needs its own check (now against `Node`'s parent chain, not `Chat.WorkspaceID`).
- Delete `Repository.FolderID`/`.Order`-adjacent leftovers if any remain (should already be gone per Task 3).
- Test: the 25+ `TestPlaceChat_*` tests in `chats_test.go`, `move_test.go`'s working-subtree test, whatever covers `checkFolderContextMove` today (re-grep, per research's own caution that this distribution wasn't fully traced).

**Interfaces:**
- Consumes: Task 1, 2, 7's surfaces.
- Produces: repo-scoped chat/folder placement now uses the same code path home-scoped placement uses (Task 5) — the two scopes should converge to nearly identical logic, differing only in which `Folder.RepoID`/anchor root applies.

- [ ] **Step 1: Read `validate.go`, `chats.go`, `tree.go` in full live**, and re-grep the OTHER tree-package files research flagged as unread this pass (`owning_chat.go`, `shared_workspace.go`, `subtree.go`, `walk.go`, `worktree_spec.go`, `imported_chat.go`, `delete_preview.go`, `lineage.go`) for any `ChatTypeBranch`/`ChatTypeFolder`/`owning_rows.go` symbol reference this task's changes would otherwise miss.

- [ ] **Step 2: Port the FULL existing test suite for `checkFolderContextMove`/`nearestWorkspaceAnchor`, `checkParentKind`, and `PlaceChat`'s repo-scoped cases to the Node-backed implementation before writing new tests** — every case the old suite covers needs a passing Node-side equivalent (spec §6 Open Question 1). Only then add new tests for anything Node-specific.

- [ ] **Step 3: Implement**, informed by Step 1/2, not guessed.

- [ ] **Step 4: Now that both home (Task 5) and repo-scoped folder placement are Node-backed, narrow `domain.ChatType`'s closed taxonomy to `{ChatTypeChat, ChatTypeWorkflow}`** — update `chat_type_test.go`'s `TestChatType_ClosedTaxonomy` and `commands/create.go:45-52`'s `validChatType` switch. Do NOT do this before this step — `ChatTypeFolder` is still load-bearing for repo-scoped folders until this task's own Step 3 lands. `ChatTypeBranch` narrowing happens in Task 9, alongside `owning_rows.go`'s deletion — don't jump ahead of that here even though this task also touches the taxonomy.

- [ ] **Step 5: Run `go test -tags noEmbed -race -count=1 ./internal/app/usecases/chat/... ./internal/domain/...`.** Confirm PASS, confirm every ported test from Step 2 is present and green.

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(chat): repo-internal folders and chat placement migrate onto Node; golden rule ports to Node's parent chain"
```

---

## Task 9: Backend — delete `owning_rows.go`/`backfill.go`, retire `ChatTypeBranch`

**Depends on:** Task 7, Task 8.

**Files:**
- Delete: `api/internal/app/usecases/chat/internal/tree/owning_rows.go`, `backfill.go`, their test files (`owning_rows_test.go`, `backfill_test.go`).
- Modify every one of the 15 consumer call sites research identified (its own table, reproduced here for this task's brief):
  - `api/tests/kit/env.go:649,659` — test harness helper using `EnsureOwningChat`/`ResolveOwningChat`; replace with a direct `Node`/`Workspace` read.
  - `api/internal/app/container.go:688` (call) and its surrounding `backfillOwningChats` function (`app.New`, l.163 calls it) — delete both; there is nothing left to backfill (Task 7 made every workspace mint its `Node` row at creation).
  - `api/internal/app/repositories/container.go:515` — already updated by Task 7; confirm no remaining reference.
  - `api/internal/app/usecases/chat/internal/tree/owning_chat.go:145` — read this file in full first (not read in research), update its internal resolution logic to use `Node`/`Workspace` directly.
  - `api/internal/app/usecases/chat/internal/tree/validate.go` (`ownsWorkspace`, called from `nearestWorkspaceAnchor`) — **NOT already superseded, correcting an error in this plan's earlier draft.** Task 8 deliberately KEPT `ownsWorkspace`/`ResolveOwningChat` as-is (verified correct by Task 8's own review) specifically because locked/unlocked branches' owning rows were still entirely `Chat`-based until this task retypes them. Now that this task gives every locked/default/home workspace a genuine `Node{Kind:workspace}` identity (no more `Chat` proxy at all), `nearestWorkspaceAnchor`'s anchor test should become a direct `Node.Kind == NodeKindWorkspace` check — no `ownsWorkspace`/`ResolveOwningChat` tiebreak needed at all. This is the point where Task 8's own doc comment ("this fully materializes once Task 9 gives locked branches their own Node identity") gets closed out. Port `nearestWorkspaceAnchor`'s full existing test suite (again) to confirm the four-tier invariant survives this final simplification.
  - `api/internal/app/usecases/chat/aliases.go:158` — delete the `agentusecase.ResolveOwningChat` re-export; update its callers (`home.go:50`, `worktree.go:197`, below) to read `Node`/`Workspace` directly instead.
  - `api/internal/app/usecases/provider/provider_sync.go`, `api/internal/app/usecases/workspace/workspace.go` — already updated by Task 7; confirm no remaining reference.
  - `api/internal/api/v0/container.go:328`, `api/internal/api/v0/dto/chat_worktree.go:76` — comment-only references naming the old rule; update the comments or delete if no longer relevant.
  - `api/internal/api/v0/endpoints/home/handlers/home.go:50`, `api/internal/api/v0/endpoints/chat/handlers/worktree.go:197` — update to read the workspace's own `Node`/id directly.
- Modify: `domain/chat_type.go` — narrow the closed taxonomy further, to `{ChatTypeChat, ChatTypeWorkflow}` if Task 8 hasn't already gotten it there (Task 8 handled `ChatTypeFolder`; this task removes `ChatTypeBranch`). Update `chat_type_test.go` and `commands/create.go`'s `validChatType` accordingly.
- Test: every test file for the 15 call sites above that has coverage; a black-box `TestRegression_*` proving a fresh workspace (no backfill ever run) still resolves correctly everywhere `ResolveOwningChat` used to be consulted.

**Interfaces:** none new — pure deletion + call-site rewiring to Task 1/2/7's already-landed surfaces.

- [ ] **Step 1: Grep the whole `api/` tree for every remaining reference to `owning_rows.go`'s exports, `BackfillOwningChats`, `EnsureOwningChat`, `ResolveOwningChat`, `ChatTypeBranch`** — research's 15-file table is a snapshot from before Tasks 5/7/8 landed; several will already be gone. Confirm the true remaining set before editing.

- [ ] **Step 2: For each remaining call site, write or update its test to assert the NEW (Node/Workspace-direct) behavior first**, confirm it fails against current code, then implement.

- [ ] **Step 3: Delete `owning_rows.go`, `backfill.go`, and their test files.** Confirm `go build -tags noEmbed ./cmd/crowbar` succeeds with them gone (proves no remaining reference).

- [ ] **Step 4: Run `go test -tags noEmbed -race -count=1 ./...`.** Confirm PASS — this is the first full-suite run since Task 1; earlier tasks scoped to touched packages, this task's deletion has the widest blast radius so it earns the full run.

- [ ] **Step 5: Commit**

```bash
git commit -m "refactor(chat): delete owning-chat backfill machinery, retire ChatTypeBranch — Node/Workspace replace it directly"
```

---

## Task 10: Frontend — repo-internal chat/folder/branch rows read/write through `Node`

**Depends on:** Task 7, Task 8, Task 9.

**Files:**
- Modify: `web/src/components/sidebar/lib/rows-from-repo.ts` — delete `homeOwningChatId`, `Ownership`/`resolveOwnership`, `foldOwningChats`; `rowsFromRepo` builds rows directly from `Node`-sourced position data plus the real `Workspace`/`Chat`/`Folder`/`Repository` content records — no more "find the chat that owns this workspace" indirection anywhere.
- Modify: `web/src/components/layout/workspace-tree-utils.ts` — `buildSidebarTree` cuts over from `PlacedWorkspace`/`SidebarChat`/`SidebarFolder`'s embedded `parentId`/`order` fields (today's model, built against the OLDER spec `docs/superpowers/specs/2026-08-23-unified-sidebar-design.md` — **not** this plan's spec; research confirms this file's own doc comments cite that older spec's §3.1/§9.2, so don't follow ITS internal "see spec" pointers when editing it) to reading `Node` rows directly for position, joined against separate content lookups for each kind.
- Modify: the ~14 other frontend files research flagged as depending on `buildSidebarTree`'s `kind: 'workspace'` tree-node shape (the prior `2026-09-01-owning-chat-backfill` plan's own Scope Discipline section named this as "the largest single piece of frontend work" and deliberately deferred it — this plan's Task 10 IS that deferred work, now backed by a simpler underlying model that should make the cutover more mechanical than it would have been then, since `Node` removes the id-collision class of bug that plan's Task 6 had to work around).
- Modify: `web/src/lib/types.ts` — narrow `ChatType` to `'chat' | 'workflow'` matching Task 9's backend narrowing.
- Test: `web/src/__tests__/components/sidebar/lib/rows-from-repo.test.ts` and every test file for the ~14 dependent files (find them via the same grep the research pass used, re-run fresh).

**Interfaces:**
- Consumes: Task 7, 8's wire contracts.

- [ ] **Step 1: Read `rows-from-repo.ts`, `workspace-tree-utils.ts` in full live** (both already fully read earlier in this project's history — re-read for drift given how much churn precedes this task) and grep fresh for every file depending on `kind: 'workspace'`'s current shape — do not trust the "14 files" count without re-confirming, since Tasks 4/6 may have already changed some of them.

- [ ] **Step 2: Write failing tests first**, covering: a locked branch row renders correctly sourced from `Workspace` directly (no owning-chat lookup); a repo-home row likewise; an ordinary chat that owns an unlocked worktree still renders as a `chat`-kind row (1:1, per spec §2.4 — this case must NOT collapse into a `workspace`-kind row); drag-and-drop across all four kinds at this level still obeys the golden rule (Task 8's ported four-tier invariant) — write a frontend-side test for at least the "folder can't jump into a different locked branch's subtree" case, matching the backend's own pinned regression.

- [ ] **Step 3: Implement.** This is the largest single task in this plan — if it grows unwieldy mid-implementation, split it (mirror `owning-chat-backfill`'s own precedent of inserting Tasks 5/6/7 mid-execution when a single-task scope proved too large) rather than forcing one oversized diff through review.

- [ ] **Step 4: Run `bun vitest run web/src/__tests__/components/sidebar/ web/src/__tests__/components/layout/`.** Confirm PASS.

- [ ] **Step 5: Run `bun tsc` and `bun run lint`** (repo-wide at this point, since this task's blast radius is wide) — confirm clean aside from pre-existing unrelated failures.

- [ ] **Step 6: Commit**

```bash
git commit -m "fix(sidebar): repo-internal rows read Node directly — Workspace/Repository referenced without a chat proxy"
```

---

## Task 11: Cleanup sweep and full-gate close-out

**Depends on:** all prior tasks.

**Files:** none modified unless a real defect is found (fix it in the smallest diff to the actual file at fault; do not expand scope).

- [ ] **Step 1: Grep the whole repo for every symbol this plan deletes** — `ChatTypeBranch`, `ChatTypeFolder`, `Repository.FolderID`, `Repository.Order`, `placeRepoInHomeContainer`, `owning_rows.go`'s exports, `BackfillOwningChats`, `EnsureOwningChat`, `ResolveOwningChat`, `foldOwningChats`, `resolveOwnership`, `homeOwningChatId`, `HomeRepoPlacement`/`repoPositions`. Confirm zero remaining references outside this plan's own commit history/docs.

- [ ] **Step 2: Confirm `domain.Chat.ParentID`/`.Order` are actually dead for placement purposes** (per spec §2.1's walk — only thread-lineage/cwd resolution should still read them, and per this plan's Task 5/8, even that resolution now walks `Node`, not `Chat.ParentID`). If they're genuinely unused, delete them; if something legitimate still reads them, document what and why in the commit message rather than silently leaving dead fields.

- [ ] **Step 3: Run the full backend gate** — `go build -tags noEmbed ./cmd/crowbar`, `go vet -tags noEmbed ./...`, `go test -tags noEmbed -race -count=1 ./...`. All must be clean.

- [ ] **Step 4: Run the full frontend gate** — `bun tsc`, `bun vitest run` (full run — this is the one point in the whole plan where the full suite is warranted, matching this task's whole-branch-cleanup scope), `bun run lint`. All must be clean aside from pre-existing, unrelated, already-disclosed failures.

- [ ] **Step 5: Delete the scratch research file** `docs/superpowers/plans/.node-migration-research.md` — it served its purpose; do not ship it.

- [ ] **Step 6: Commit**

```bash
git commit -m "chore(sidebar): cleanup sweep — confirm no dead references to the pre-Node placement machinery"
```

---

## Task 12: Live E2E verification

**Files:** none modified unless verification finds a real defect — fix it in the smallest diff to the file actually at fault.

- [ ] **Step 1: Start the dev daemon for THIS worktree** (`enhancement/unify-sidebar`'s Tauri dev instance). Confirm via `mcp__tauri__ipc_get_backend_state` you're looking at this worktree's own instance, not another session's, before trusting anything you see (per this session's standing hazard memory: never touch the prod socket, derive the dev socket from this worktree's `CROWBAR_HOME`).

- [ ] **Step 2: Re-verify the ORIGINAL bug report scenario** — a single-repo project, drag the repo to any non-zero position, confirm it lands there and persists across a reload. Network-capture the request/response if useful, matching this session's earlier verification standard.

- [ ] **Step 3: Verify drag-and-drop for every row kind at every tree level** — project home: chat, folder, repo (reorder, move into/out of a folder); a repo's own tree: chat, folder, locked branch (reorder, move into/out of a folder, confirm the four-tier context-anchor invariant — a folder cannot be dragged into a different locked branch's subtree, and this must visibly refuse or bounce back, not silently succeed).

- [ ] **Step 4: Verify creation still works for every row kind** — new chat, new folder, new locked-branch fork, new repo import — at both project home and inside a repo's own tree, confirming each new row gets a `Node` row immediately (no backfill lag, no missing-position flash).

- [ ] **Step 5: Confirm no visual regression** — screenshot the sidebar before invoking this task's changes (or compare against the last known-good screenshot in this session's history) and after; every row must render identically in content (name, icon, lock state, branch name) — only the underlying position mechanism changed.

- [ ] **Step 6: Confirm the full backend+frontend gate (Task 11) is still green after any fixes this task made.**

- [ ] **Step 7: Write a short closing note to the SDD ledger** summarizing what was verified live vs. test-only, matching this session's established honesty standard about verification gaps (e.g. if this dev environment has no enabled provider loaded to prove a full end-to-end chat-creation toast, say so explicitly).
