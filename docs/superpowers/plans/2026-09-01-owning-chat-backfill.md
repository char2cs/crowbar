# Owning-chat backfill Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every existing `Workspace` gets a real owning `Chat` row (`Type: branch` for a locked branch/repo-home/project-home, `Type: chat` — a worktree chat — for a regular forked workspace), so that clicking "+" to create a thread or workspace under ANY branch-kind sidebar row resolves a valid `parentId` on the backend instead of silently refusing.

**Architecture:** This closes Open Question 3 ("Migration") of `docs/superpowers/specs/2026-08-23-unified-sidebar-design.md`, which was explicitly deferred: *"Every fork workspace on disk today has no chat at all ... a backfill is owed."* Research (this session) confirmed Stage 1 of that spec's sequencing (folders folding into `ChatTypeFolder`) is already done on this branch, and Stage 5 (atomic workspace+chat creation) landed as batch-2's Task 7 — this plan is the missing backfill for data that predates Stage 5, plus the two small pieces of code that assumed it would already exist: `checkParentKind`'s folder-only bypass, and the frontend's row-identity sourcing.

**Scope discipline:** This plan does NOT touch `buildSidebarTree`/`workspace-tree-utils.ts`'s `kind: 'workspace'` tree-node type, which 14 other frontend files depend on — deleting it is Stage 8 of the model spec ("the largest single piece of frontend work," with its own known trap) and is explicitly out of scope here. The fix below is narrower and surgical: once every workspace has a real owning chat, `rows-from-repo.ts` sources a branch row's *identity* (the id used for itself and threaded down as its children's `parentId`) from that real chat row, while everything else — `buildSidebarTree`'s three-argument shape, `PlacedWorkspace`, row *content* (branch name, working state, lock status) — is untouched.

**Tech Stack:** Go (asynx aggregates, gin handlers), TypeScript/React (Zustand-adjacent tree builder, no new store).

**Spec:** `docs/superpowers/specs/2026-08-23-unified-sidebar-design.md` — §3.1 (the four-row taxonomy), §5.1 ("each row ships its walks already resolved"), §9 Open Question 3 (migration, resolved by this plan).

## Global Constraints

- **"Locked/branch" is computed per-workspace, not a fixed list.** A workspace becomes a `Type: branch` owning chat iff `ws.Status == domain.WorkspaceStatusLocked || ws.IsDefault || ws.Kind == domain.WorkspaceKindHome` (confirmed: `ws.Status` already folds the user's `LockOverride` and the provider's protected flag into one authoritative badge via `nextProviderStatus`, `sync_provider_state.go` — no need to re-read `LockOverride` directly). Every other workspace with no owning chat gets `Type: chat` (a worktree chat).
- **Backfill ordering is topological, root-first.** A workspace's backfilled chat `ParentID` is its fork parent's *own newly-minted (or pre-existing) owning chat id* — so the backfill must process workspaces with empty `Workspace.ParentID` (or whose fork parent is the repo's default/home workspace) before their descendants. Process the whole DAG breadth-first from each repo's roots.
- **Backfill `Order`** is assigned by stable `CreatedAt` ascending among same-parent siblings — no real order data exists to preserve (`Workspace.FolderID`/`Order` are confirmed dead fields on the wire today, never populated), so this is the only meaningful choice and must not visibly reshuffle the sidebar.
- **Idempotent, always.** Gate per-workspace on `len(ListChatsByWorkspace(wsID)) == 0`. A workspace that already has an owning chat (created via Stage 5's atomic path, or backfilled on a prior run) is never touched again.
- **No fallback logic in the frontend.** The backend backfill runs once, synchronously, before the API starts serving any request. By the time any frontend code reads `repo.chats`, every workspace has a real owning chat. Code added in Task 4 must NOT contain a defensive "if no owning chat found, use the old Workspace-id path" branch — a missing entry is a bug to surface loudly (throw / console.error), not paper over.
- Same repo-wide conventions as the rest of this codebase: kebab-case component files, tests mirror `web/src/__tests__/` with `@/` imports, narrow store selectors, black-box `TestRegression_*` tests in `api/tests/` per this repo's convention, every task needs a regression test that fails on pre-fix code.

---

## Task 1: Backend — `AgentChatDTO` carries the row's `Type`

**Files:**
- Modify: `api/internal/api/v0/dto/agent.go` (`AgentChatDTO` struct, `AgentChatDTOFrom`)
- Test: `api/internal/api/v0/dto/agent_test.go` (create if it doesn't exist at that path — check first for the existing test file's actual name/location by looking for other `AgentChatDTOFrom` tests)

**Interfaces:**
- Produces: `AgentChatDTO.Type domain.ChatType` (wire key `"type"`), always present (not `omitempty` — `""` is not a real `ChatType` value, but every row this DTO ever serializes has a real `Type`, so there is no meaningful absent case to distinguish, unlike `ParentID`/`Order`'s documented "" /0-is-meaningful pattern).
- Consumed by: Task 4's frontend `SidebarChat.type` field.

- [ ] **Step 1: Read `AgentChatDTO` and `AgentChatDTOFrom` in full** (`api/internal/api/v0/dto/agent.go`) to see the exact existing field-copy pattern, and confirm `domain.Chat` (`api/internal/domain/chat.go`) has a `Type domain.ChatType` field to read from.

- [ ] **Step 2: Write the failing test.** Assert `AgentChatDTOFrom` on a `domain.Chat{Type: domain.ChatTypeBranch, ...}` produces an `AgentChatDTO` whose `Type == domain.ChatTypeBranch`, and that this DTO serializes to JSON containing `"type":"branch"`.

- [ ] **Step 3: Implement.** Add the field to the struct (with a doc comment following this file's existing style — see how `ParentID`/`Order` are documented as an example of the bar to match) and copy `c.Type` into it in `AgentChatDTOFrom`.

- [ ] **Step 4: Also verify (a not-yet-existing-data concern, not a code change unless this fails):** `ListChatsInRepo` (`api/internal/app/usecases/chat/repo_scope.go:28-45`) already includes `ChatTypeBranch` rows in its output — its only type-based exclusion is `ChatTypeFolder` (`repo_scope.go:56-58`, `rowInRepo`). Write a black-box regression test in `api/tests/` (`TestRegression_*` per convention) that seeds a workspace, directly constructs a `ChatTypeBranch` chat row owning it (via whatever internal test helper this repo already uses to seed chat rows — check `api/internal/app/usecases/chat/*_test.go`'s fixtures for the pattern), and asserts `GET /repos/:rid/chats` returns that row with `"type":"branch"` in its JSON body.

- [ ] **Step 5: Run the full test suite for touched packages**, confirm PASS, confirm no regression elsewhere `AgentChatDTO` is constructed (grep for other `AgentChatDTOFrom`/`AgentChatDTO{` call sites and check none breaks on the new required field).

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(chat): AgentChatDTO carries the row's Type"
```

---

## Task 2: Backend — `checkParentKind` accepts a `ChatTypeBranch` parent

**Files:**
- Modify: `api/internal/app/usecases/chat/internal/tree/validate.go` (`checkParentKind`, `:109-124`)
- Test: alongside the existing tests for `checkParentKind` — find them (likely `api/internal/app/usecases/chat/internal/tree/*_test.go`) and follow their exact fixture pattern.

**Interfaces:** none new — widens an existing internal validation function's accepted input, does not change its signature.

- [ ] **Step 1: Read `checkParentKind` in full** (`validate.go:109-124`) and the test(s) that currently prove `ChatTypeFolder` bypasses the workspace-match check, and the test that currently proves a `ChatTypeChat` parent in another workspace is refused (`ErrCrossWorkspace`) — this is `TestCreateChat_OwnWorktree_RefusesAChatParentInAnotherWorkspace` per prior research in this session, in `chat/internal/tree/chats_test.go:334` at time of writing; confirm it's still there and read it.

- [ ] **Step 2: Write the failing test.** A new chat placed with `parentId` naming a `ChatTypeBranch` row in workspace A, while the new chat's own resolved workspace differs (or is empty, the bubble case) from A, must NOT be refused with `ErrCrossWorkspace` — mirror the existing `ChatTypeFolder` bypass test's exact shape, swapped to `ChatTypeBranch`. Confirm this fails against current code (current code refuses it, since only `Type == ChatTypeFolder` bypasses).

- [ ] **Step 3: Implement.** Widen the bypass condition:

```go
func checkParentKind(
	row domain.Chat,
	workspaceID string,
	parentID string,
) error {
	if row.Type == domain.ChatTypeFolder || row.Type == domain.ChatTypeBranch {
		return nil
	}
	if row.WorkspaceID == workspaceID {
		return nil
	}
	return fmt.Errorf(
		"agent chat folder: parent %s belongs to workspace %s, not %s: %w",
		parentID, row.WorkspaceID, workspaceID, ErrCrossWorkspace,
	)
}
```

- [ ] **Step 4: Run the full test suite for the `tree` package**, confirm PASS, confirm the pre-existing `ChatTypeFolder` and same-workspace tests are unaffected.

- [ ] **Step 5: Commit**

```bash
git commit -m "fix(chat): a chat can be parented under a ChatTypeBranch row, not just a folder"
```

---

## Task 3: Backend — startup backfill mints owning chats for existing workspaces

**Files:**
- Create: `api/internal/app/usecases/chat/internal/tree/backfill.go` (or, if investigation in Step 1 finds a more fitting existing file/package for one-shot reconciliation logic, use that instead — state which and why in the commit)
- Modify: wherever the chat usecase is constructed and wired into the daemon's startup sequence (find this in Step 1 — likely `api/internal/app/usecases/chat/chat.go`'s constructor, or the daemon's own `main`/bootstrap file that wires up all usecases)
- Test: `api/internal/app/usecases/chat/internal/tree/backfill_test.go`; a black-box `TestRegression_*` in `api/tests/` seeding several unbackfilled workspaces (including a parent/child fork pair, to prove ordering) and asserting every one has exactly one owning chat, correctly typed and correctly parented, after the daemon starts.

**Interfaces:**
- Produces: a function, e.g. `BackfillOwningChats(ctx context.Context) error` (exact name/location decided in Step 1), invoked once at startup, before the API begins serving requests.
- Consumes: `ListChatsByWorkspace` (existing, confirmed present at `api/internal/app/usecases/chat/chat.go:432`) for the idempotency gate; whatever internal chat-construction primitive Step 1 finds reusable from Task 7's own-worktree machinery, MINUS the worktree-creation step.

- [ ] **Step 1: Read `api/internal/app/usecases/chat/own_worktree.go` in full** (Task 7's own-worktree chat construction, commit `f0efe51b` on this branch) to find the exact internal function(s) that (a) construct a new `Chat` row via `tree.CreateChat` and (b) call `SetWorkspace` to point it at an existing/already-provisioned workspace, separately from (c) the `WorktreeCreator` call that cuts a NEW worktree on disk. This backfill needs (a) and (b) only — every workspace it processes already has a worktree on disk (or is the project-home row, which has none by design). Also read `api/internal/app/usecases/chat/internal/tree`'s `CreateChat` signature directly to confirm what `ParentID`/`Order`/`Type`/`WorkspaceID` it accepts. Also find where the chat usecase and workspace usecase are both already available together at startup (a daemon bootstrap/wiring file) to decide where the one-shot call belongs — prefer wiring it in near other one-time startup reconciliation if any already exists (grep for patterns like "on startup" / "at boot" / a `Reconcile*` naming convention anywhere in `api/`); if none exists, add it as a plain call in the same place all usecases get constructed, clearly commented as a one-shot backfill.

- [ ] **Step 2: Write the failing black-box regression test first.** Seed (via whatever this repo's existing test helpers use to seed workspaces directly into the store, bypassing the API — check `api/tests/`'s existing fixtures) three workspaces in one repo with NO owning chats: a repo-home workspace (`IsDefault: true`), a locked workspace forked from it (`Status: WorkspaceStatusLocked`, `ParentID: <repo-home's id>`), and a regular unlocked workspace forked from the locked one (`ParentID: <locked workspace's id>`). Start the daemon (or invoke the backfill function directly, whichever this repo's black-box test convention uses for daemon-level tests — check an existing `TestRegression_*` for the pattern). Assert: the repo-home workspace has exactly one owning chat, `Type == ChatTypeBranch`, `ParentID == ""`. The locked workspace has exactly one owning chat, `Type == ChatTypeBranch`, `ParentID == ` the repo-home workspace's owning chat's id. The regular workspace has exactly one owning chat, `Type == ChatTypeChat`, `ParentID == ` the locked workspace's owning chat's id. Confirm this fails against current code (no backfill exists yet, so no owning chats are minted at all).

- [ ] **Step 3: Write a second failing test proving idempotency.** Run the backfill twice (or start/restart the daemon twice) against the same seeded data; assert each workspace still has exactly ONE owning chat after the second run, not two.

- [ ] **Step 4: Implement `BackfillOwningChats`** (or whatever Step 1 decided to name/place it): list every workspace across every repo; filter to those where `ListChatsByWorkspace` is empty; build the fork-parent DAG from `Workspace.ParentID` (root = empty `ParentID`, or `IsDefault`/`Kind == WorkspaceKindHome`); process breadth-first from each repo's roots so a parent's owning chat always exists before a child needs to reference it as `ParentID`; for each workspace, `Type = ChatTypeBranch` if `ws.Status == domain.WorkspaceStatusLocked || ws.IsDefault || ws.Kind == domain.WorkspaceKindHome`, else `ChatTypeChat`; construct the chat row via the primitive found in Step 1 (mint + `SetWorkspace`, no `WorktreeCreator` call); `Order` assigned by ascending `CreatedAt` among siblings sharing the same resolved parent.

- [ ] **Step 5: Wire the one-shot call into startup**, per what Step 1 found — before the API begins serving requests (a backfill racing a live create-chat request against the same workspace must not double-mint; confirm this ordering is actually enforced by where you place the call, not just assumed).

- [ ] **Step 6: Run the full test suite for touched packages and the black-box regression suite.** Confirm PASS, confirm the ordering and idempotency tests both go RED→GREEN.

- [ ] **Step 7: Commit**

```bash
git commit -m "feat(chat): backfill owning chat rows for every pre-existing workspace"
```

---

## Task 4: Frontend — branch rows source their identity from the real owning chat

**Files:**
- Modify: `web/src/components/layout/workspace-tree-utils.ts` (`SidebarChat` interface only — add `type` field; do NOT touch `SidebarTreeNode`, `buildSidebarTree`, `PlacedWorkspace`, or the `kind: 'workspace'` branch — out of scope per this plan's Scope Discipline section)
- Modify: `web/src/components/sidebar/lib/rows-from-repo.ts`
- Modify (wherever `repo.chats` is populated from the `/chats` response — find it in Step 1): the client-side type/mapper that currently discards or doesn't expect a `type` field on a chat DTO.
- Test: `web/src/__tests__/components/sidebar/lib/rows-from-repo.test.ts`

**Interfaces:**
- Consumes: Task 1's `AgentChatDTO.type` (wire key `"type"`), Task 3's guarantee that every workspace has exactly one owning chat by the time any frontend request completes.
- Produces: no change to `SidebarRow`'s own shape — only where its `id`/`workspaceId` values are sourced from for branch-kind rows.

- [ ] **Step 1: Read `rows-from-repo.ts` in full** (already read once this session — re-read for exact current line numbers before editing) and trace where `repo.chats` is populated from the wire response, to find the client-side `Chat`/DTO mapping type that needs a `type` field added (check `web/src/lib/store/sidebar.ts` first, per `rows-from-repo.ts`'s own import of `Repo` from there).

- [ ] **Step 2: Write the failing test.** In `rows-from-repo.test.ts`: given a repo whose `chats` array includes one `{id: 'branch-chat-1', type: 'branch', workspaceId: 'ws-locked', parentId: '', title: ''}` and whose `workspaces` array includes the matching `{id: 'ws-locked', branch: 'develop', ...}`, assert `rowsFromRepo(repo)` produces a `SidebarRow` with `id: 'branch-chat-1'` (NOT `id: 'ws-locked'`) — and that a chat placed as a thread with `parentId: 'ws-locked'` in the fixture (matching how a regular chat under that workspace is keyed today) renders with `parentId: 'branch-chat-1'` in the output row (i.e., children get the REAL chat id as their parent, not the workspace id). Also test the repo-home case: `repo.defaultWorkspaceId` matching a `type: 'branch'` chat's `workspaceId` produces a root row whose `id` is that chat's id, not `repo.defaultWorkspaceId` directly. Confirm both fail against current code.

- [ ] **Step 3: Implement.** Build a `Map<workspaceId, ownerChatId>` from `(repo.chats ?? EMPTY_CHATS).filter(c => c.type === 'branch')`. Use it in two places: (a) the repo-home row push (currently `id: homeId` at the top of `rowsFromRepo`) — look up `homeId` in the map and use the result as `id` (per this plan's Global Constraints, this lookup has NO fallback branch — if `homeId` is truthy the entry MUST exist post-Task-3, so an absent entry is a thrown error, not a silent skip); (b) the `else` branch inside `walk` for `node.kind === 'workspace'` — same lookup on `node.id` (the workspace id), same no-fallback rule, used as the pushed row's `id` AND as the value passed to the recursive `walk(node.children, <that id>)` call instead of `node.id`.

- [ ] **Step 4: Confirm no regression to row CONTENT** — `branchName`, `working`, `ownsWorktree`, `repoIcon` must still be sourced from the `Workspace` record exactly as today; only `id` (and the id threaded to children) changes.

- [ ] **Step 5: Run the full `web/src/__tests__/components/sidebar/` and `web/src/__tests__/components/layout/` directories.** Confirm PASS, confirm no regression to the batch-2 Task 8 regression tests (`rows-from-repo.test.ts`'s existing "protected/locked-branch" and "atomic-create rendering" `describe` blocks) — these should still pass unchanged in shape, just now resolving through the new id-sourcing path.

- [ ] **Step 6: Run `bun tsc --noEmit` and `bun run lint`.** Confirm clean (aside from pre-existing unrelated failures — verify via `git stash` comparison against unmodified HEAD, matching this session's established convention).

- [ ] **Step 7: Commit**

```bash
git commit -m "fix(sidebar): branch rows resolve their id from the real owning chat, not the workspace"
```

---

## Task 5 (inserted mid-execution): Backend — expose each workspace's real owning chat id

**Why inserted:** Task 4's implementer found, and measured, that widening its id-lookup map to include `type: 'chat'` owners (not just `type: 'branch'`) causes an id collision for a regular forked workspace: its owning chat is BOTH an ordinary conversation row `buildSidebarTree` already renders independently, AND the thing `rows-from-repo.ts`'s workspace-node branch would also try to render under the same id — one row becoming its own parent. Fully solving that client-side would mean touching `buildSidebarTree`'s `kind: 'workspace'` node handling, which this plan's Scope Discipline section explicitly rules out. The implementer correctly scoped their fix to `type: 'branch'` owners only (repo-home, locked branches) and disclosed that a regular fork's "+" action is still unfixed by Task 4 alone.

This task closes that gap the smallest possible way: tell the frontend a workspace's owning chat id directly, as a plain field alongside the workspace it already fetches — sidestepping the id-collision entirely, since it's a pointer FROM the workspace, not a second copy of the chat row merged into the same list.

**Files:**
- Modify: wherever the workspace DTO is defined and populated for the wire (find it — likely `api/internal/api/v0/dto/workspace.go` or similar; grep for the struct backing `GET /repos/:rid/workspaces` or wherever `repo.workspaces` on the frontend is actually sourced from).
- Test: alongside that DTO's existing tests, plus a black-box `TestRegression_*` in `api/tests/`.

**Interfaces:**
- Produces: a new field on the workspace wire DTO, e.g. `owningChatId string` (wire key `"owningChatId"`, empty string if genuinely none — should not happen post-Task-3 for any workspace this route serves, but do not assert that here; that is Task 6's frontend concern).
- Consumes: Task 3's guarantee that every workspace has exactly one owning chat; reuse Task 3's OWN branch-preferring resolution logic (`preferred`/`owningRows` in `api/internal/app/usecases/chat/internal/tree/backfill.go`, or whatever it now composes into after Task 3's fix round 2) rather than re-deriving "which chat owns this workspace" a second, possibly-inconsistent way. If that logic is private/internal to the backfill package, extract or expose the minimal piece needed (a `ResolveOwningChat(ctx, workspaceID) (chatID string, err error)`-shaped function) rather than duplicating its branch-preference rule.

- [ ] **Step 1: Read the current workspace DTO and its population path in full**, and read Task 3's final `preferred`/`owningRows` implementation (post fix-round-2) to find the right function to reuse or extract.

- [ ] **Step 2: Write the failing test.** A workspace DTO response includes `owningChatId` matching the real backfilled (or newly Task-7-created) owning chat's id — for both a locked/default/home workspace (should resolve to its `ChatTypeBranch` row, not a coincidental legacy chat, matching Task 3's own branch-preference invariant) and a regular forked workspace (resolves to its `ChatTypeChat` row).

- [ ] **Step 3: Implement**, reusing Task 3's resolution logic.

- [ ] **Step 4: Run the full test suite for touched packages**, confirm PASS, confirm no regression to Task 3's own tests (the resolution logic itself should not need to change — this task should be additive, reading it from a new call site).

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(workspace): expose the workspace's real owning chat id on the wire"
```

---

## Task 6 (inserted mid-execution): Frontend — regular forked workspaces resolve their real chat id too

**Files:**
- Modify: `web/src/components/sidebar/lib/rows-from-repo.ts` (the `else` branch inside `walk`, for `node.kind === 'workspace'` — the SAME branch Task 4 already touched for the `type: 'branch'` case)
- Modify: wherever `Workspace`'s client-side type is defined (`web/src/lib/store/sidebar.ts`, matching Task 4's Step 1 investigation) — add `owningChatId` to it, consuming Task 5's new wire field.
- Test: `web/src/__tests__/components/sidebar/lib/rows-from-repo.test.ts`

**Interfaces:**
- Consumes: Task 5's `Workspace.owningChatId`.
- Produces: no change to `SidebarRow`'s shape — only where a regular (non-`type:'branch'`) workspace-node's `id` is sourced from.

- [ ] **Step 1: Read Task 4's final diff in full** (`rows-from-repo.ts`'s current state, post-Task-4) before touching it — this task extends the SAME `else` branch, it does not introduce a parallel path.

- [ ] **Step 2: Write the failing test.** A regular (non-locked, non-default, non-home) workspace whose `Workspace.owningChatId` names a real chat id produces a `SidebarRow` with that id (not the workspace's own id), and children reparented onto it accordingly — mirroring Task 4's Step 2 test shape, for the case Task 4 explicitly left unfixed. Also assert: this does NOT reintroduce the id-collision Task 4's implementer found — the SAME chat id must not ALSO appear as an independent `kind: 'chat'` row in the output (check whether `buildSidebarTree`'s `chats` argument needs the same "withhold a chat that is about to be re-rendered as this workspace's identity" treatment Task 4 already built for `type: 'branch'` owners, or whether — since a regular fork's owning chat is a genuine, independently meaningful conversation row per the model spec's own §3.1 table — the CORRECT behavior here is actually to leave BOTH rows rendering as they do today (the pre-existing two-row split Task 8 of the prior plan already found intentional and left alone), and this task's id-sourcing applies ONLY to what a "+"-click sends as `parentId`, not to how many rows render. Resolve this by re-reading batch-2's Task 8 report and this session's own investigation of the two-row split before writing the implementation — do not guess.

- [ ] **Step 3: Implement**, informed by Step 2's investigation.

- [ ] **Step 4: Run the full `web/src/__tests__/components/sidebar/` directory.** Confirm PASS, confirm Task 4's existing tests (locked/default/home id-sourcing, the loading-gate, the dispatcher-routing fixes) are unaffected.

- [ ] **Step 5: Run `bun tsc --noEmit` and `bun run lint`.** Confirm clean aside from pre-existing failures.

- [ ] **Step 6: Commit**

```bash
git commit -m "fix(sidebar): a regular forked workspace's + action resolves its real chat id too"
```

---

## Task 7: Live verification and full-gate close-out

**Files:** none modified — verification only. If this task finds a real defect, fix it in the smallest possible diff to the file(s) actually at fault and note the fix in the commit; do not expand scope beyond what verification reveals broken.

- [ ] **Step 1: Restart the dev daemon** (this worktree's `enhancement/unify-sidebar` Tauri dev instance) so Task 3's startup backfill actually runs once against this environment's real ~7 seeded workspaces. Confirm via `mcp__tauri__ipc_get_backend_state` you're looking at THIS worktree's instance before trusting anything you see.

- [ ] **Step 2: Confirm no visual regression.** Screenshot the sidebar before and after (or compare against the last known-good screenshot from this session); every row that was a `branch`-kind row before must still render identically — same label, same branch name, same lock icon, same position.

- [ ] **Step 3: Live-verify the actual fix**, via `mcp__tauri__webview_interact`/`webview_find_element`, for THREE cases: (a) click "+" → "Create thread" on a locked branch row (e.g. `develop`/`main`); (b) click "+" → "Create workspace" on the repo-home row; (c) click "+" → "Create workspace" on an existing regular forked workspace row. All three must now produce a new row in the sidebar (or, if this dev environment currently has no enabled agent provider loaded — as was the case earlier this session — confirm via `mcp__tauri__read_logs` that the request reached the backend and got a real success response, not a `404`/`ErrCrossWorkspace`/`ErrNoForkParent` refusal; use `webview_execute_js` sparingly and only if the MCP `script_result` permission issue seen earlier this session has resolved — otherwise rely on `read_logs`/screenshots only).

- [ ] **Step 4: Confirm the AffordanceRow empty-container create control (Task 14, already shipped) still works** against a folder/branch row now backed by a real chat id — the two fixes must compose.

- [ ] **Step 5: Run the full backend and frontend gates one more time** — `go build ./... && go vet ./...`, the full Go test suite for touched packages, `bun tsc --noEmit`, `bun vitest run`, `bun run lint`. All must be clean (aside from the same pre-existing, unrelated failures already identified and disclosed earlier this session).

- [ ] **Step 6: Write a short closing note to the plan's SDD ledger** summarizing what was verified live vs. what remains test-only (e.g. if no enabled provider was available in this dev session to prove a full end-to-end success toast, say so explicitly, matching this session's established honesty standard about verification gaps).
