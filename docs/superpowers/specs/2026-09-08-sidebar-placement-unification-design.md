# Sidebar Tree Unification — architecture spec

**Date:** 2026-09-08

**Status:** DRAFT — proposed, not yet implemented. Backs
[`2026-09-08-sidebar-placement-unification.md`](../plans/2026-09-08-sidebar-placement-unification.md).

**Scope:** where every sidebar row sits (its `ParentID` + `Order`), and what kind of
entity is allowed to BE a row. Touches `domain.Chat`, `domain.Repository`,
`domain.Workspace`, the chat-tree usecase package
(`api/internal/app/usecases/chat/internal/tree/`), the project usecase package
(`api/internal/app/usecases/project/`), and the frontend tree-building/interleave
logic (`web/src/components/layout/workspace-tree-utils.ts`,
`web/src/components/sidebar/lib/rows-from-repo.ts`,
`web/src/components/sidebar/lib/rows-from-home.ts`).

**Not in scope:** conversation content itself (turns, the ledger, hooks, providers —
`Chat` keeps all of this unchanged), `Workspace.ParentID` (fork/git lineage — a
different fact from sidebar position, see §2.4), `domain.Project.Order` (no shared
container, confirmed by audit — §1.4), and the asynx library itself.

---

## 1. What is wrong today

### 1.1 The bug that was caught live

Project home renders chats, folders, and every repo's own header row as siblings at
one level (`SidebarTree`'s `roots.sort(byOrder)`,
`web/src/components/sidebar/sidebar-tree.tsx`), comparing each row's `.order` field
directly, numerically, regardless of which backend aggregate produced it.

`domain.Chat.Order` (chats/folders) and `domain.Repository.Order` (repos) are two
independently densified integer sequences. Repo placement
(`api/internal/app/usecases/project/project.go`'s `UpdateRepo`, before today's
interim fix) densified via `ordering.go`'s `place()`/`reinsert()`, scanned via
`FindWhere(ProjectID)` filtered to `FolderID` — repos only, blind to chats/folders
sharing the same container.

`reinsert()` clamps a requested target to `len(slots)` **after removing the moved
row** — the count of *other repos* sharing the container. A project with exactly one
repo has zero others to clamp against, so `reinsert()` clamped every requested
position back to 0 regardless of what the drag asked for — caught live as "the repo
I just dragged snapped back to the very front, every time."

A same-day interim fix (`placeRepoInHomeContainer`, `project.go`) made the repo write
path read chat/folder siblings too, merging them into one `place()` call before
writing back to whichever aggregate each shifted row belongs to. It works — verified
live, network-captured, persisted across reload. This spec replaces that patch
entirely; it does not extend it.

### 1.2 Why the narrow fix — reuse `Chat.SetPlacement` for repos — doesn't work

A repo's own header row *already renders as* a `domain.Chat` row
(`Type: ChatTypeBranch`, the chat that owns the repo's default workspace,
`rows-from-repo.ts`'s home-row push). The apparently-obvious narrow fix: let this
row's placement be its owning chat's `ParentID`/`Order`, routed through `PlaceChat`
like a branch already is (§1.4).

This does not work, and the wall is load-bearing:

- `PlaceChat`'s `loadChat` (`api/internal/app/usecases/chat/internal/tree/plan.go:56-74`)
  refuses `ErrNotFound` unless `chat.WorkspaceID == callerWorkspaceID` — enforced by
  `TestPlaceChat_RefusesAChatFromAnotherWorkspace`. A repo's owning chat's
  `WorkspaceID` is permanently the repo's own default workspace (correct — it grounds
  the chat's CLI/cwd), but the repo needs to be positioned in PROJECT HOME's tree — a
  DIFFERENT scope. Calling `PlaceChat` with home's workspace id to reposition it is
  refused before anything runs.
- Even bypassing that, `workspaceSnapshotAround` (`plan.go:134-154`) scans siblings
  via `ListByWorkspace(workspaceID)` — for a repo-owning chat that would return the
  repo's OWN internal chats, not project home's.

### 1.3 The deeper problem this narrow fix would have papered over

The repo bug is a symptom of `domain.Chat` being asked to represent three unrelated
things through one `Type` enum: a real conversation, a folder (pure organization,
zero conversational behavior), and a locked/default/home workspace's own row (a
CONTAINER that can hold many real conversations — never one itself). Evidence this is
load-bearing overload, not convenient reuse:

- **A folder row.** Once `ParentID`/`Order` are set aside, a `ChatTypeFolder` row's
  only genuinely-used fields are `ID`, `Title` (its name), `RepoID`. Every other field
  on `domain.Chat` — `Model`, `Effort`, `PermissionLevel`, `Working`,
  `CurrentTurnStarted`, `AsyncWork`, `LedgerCursor` — is dead weight a folder never
  reads or writes. It is not a chat that happens to organize; it never was one.
- **A locked branch / repo-home / project-home row.** This row exists *only* to give
  the tree something to hang a position off of.
  `BackfillOwningChats`/`EnsureOwningChat`/`adopt` (`owning_rows.go`) mint or
  retype a chat specifically so a workspace has an id the tree can address — the
  workspace itself (`domain.Workspace`) already carries everything the row needs to
  render (branch name, lock status, default/home flags). The relationship this
  represents is **one workspace, many real chats filed under it (1:N)** — an
  ordinary chat that owns its own ad-hoc, unlocked fork is the opposite shape
  (**one chat, at most one worktree it owns, 1:1**) — and today's model uses the SAME
  `ChatTypeBranch` value to mean both, distinguished only by `Workspace.Status`/
  `IsDefault`/`Kind`, checked in a different package (`owningChatType`,
  `owning_rows.go:246-255`) than the one that reads it.

Both are the identical smell §1.2 hit for repos: something that is not a
conversation, wearing a `domain.Chat` row because that was the only aggregate with a
`ParentID`/`Order` to reuse.

### 1.4 Audit: is the ordering-collision bug itself repeated elsewhere?

Checked before scoping the fix's *ordering* half, independent of §1.3's entity-shape
finding:

| Candidate | Verdict | Evidence |
|---|---|---|
| Branch vs. chat/folder, inside a repo's own tree | **Not bugged — already routed through one write path.** A branch's position is its owning chat's `Chat.ParentID`/`.Order`, written via `PlaceChat` against the repo's own workspace scope — never crosses a workspace boundary, so §1.2's wall never fires for it. | `api/internal/domain/workspace.go` (no `Order`/`FolderID` field); `dto/workspace_test.go:226-227`. |
| Project vs. anything | **No shared container.** Each project roots its own isolated tree. | `domain.Project.Order` is the only ordering field on Project. |
| Any other `Order` field on any domain struct | **None found.** Exactly `Chat.Order`, `Project.Order`, `Repository.Order` exist. | Full grep of `api/internal/domain/*.go`. |

The *collision* (two independently-densified sequences) has exactly one live instance
today: repos vs. home chats/folders. §1.3's finding is a separate, deeper issue this
spec also fixes — not because it is bugged today, but because it is what made §1.2's
wall exist in the first place, and it will keep producing this class of bug for the
next thing that isn't a chat and needs a place in the tree.

---

## 2. The fix

Two things, and they are separable but done together because the second is what makes
the first free of the special-casing §1.2 hit:

1. One dedicated entity owns EVERY row's position — nothing else.
2. `Chat` narrows to what it actually is: a conversation, optionally owning at most
   one un-locked worktree. Everything that is not a conversation stops being modeled
   as one.

### 2.1 The position entity

```go
type NodeKind string

const (
	NodeKindChat      NodeKind = "chat"      // a domain.Chat — a real conversation
	NodeKindFolder    NodeKind = "folder"    // a domain.Folder (new, §2.3)
	NodeKindWorkspace NodeKind = "workspace" // a domain.Workspace, referenced directly — locked/default/home
	NodeKindRepo      NodeKind = "repo"      // a domain.Repository, referenced directly
)

type Node struct {
	ID       string // == the referenced entity's own id — no separate id minted
	Kind     NodeKind
	ParentID string // another Node's ID, or "" for that tree's own root
	Order    int
}
```

One row per sidebar row, one densify algorithm (today's `ordering.go`
`place()`/`reinsert()`, generalized to read/write across kinds), one ancestry walk
for every "which context/workspace does this row belong to" question — today spread
across `nearestWorkspaceAnchor` (chat package), `repoScopeOf`, and this session's
interim `homeContainerChats` (project package). The walk becomes: follow `ParentID`
until a `Kind: workspace` row turns up (that IS the grounding workspace id directly,
no indirection through an owning chat), or the root is reached.

`ID` reuses the referenced entity's own id (already a globally unique UUID per
kind) — no third id space to keep in sync.

### 2.2 Why asynx, and why only this entity

`domain.Chat` is already asynx-backed
(`api/internal/app/repositories/chat/`). The asynx contract
(`Command[T].EmitEvent(current *T) T`) is state-based: a command returns the
aggregate's whole next state, and asynx wraps `PreviousAggregate` into the emitted
event automatically — undo-by-replay is a framework primitive
(`Asynx[T].Replay(ctx, id, fromVersion, toVersion, fn)`), not something this entity
has to hand-build (§5).

A real, already-shipped lesson governs this entity's command design: `SetOrder`'s own
doc comment (`internal/app/repositories/chat/internal/commands/set_order.go`) records
that `SetPlacement` re-stating a caller-read `ParentID` during a multi-row densify
raced the async read-model projection and silently reverted a just-filed thread to
root. The fix already proven in production: **mutate the minimum field per
command** — `SetOrder` (order only) for every row a densify pass merely shifts,
`SetPlacement` (parent + order) for the one row actually dragged — folding from the
aggregate's own current state, never a caller's possibly-stale read. `Node`'s
commands follow the identical split (§2.5).

`domain.Folder` (§2.3) is deliberately NOT asynx-backed — a rename is a single,
low-stakes CRUD fact with no drag-time densify race to guard against and no
motivated undo case; it stays plain GORM, the same tier `Repository`'s own
git-identity fields already live at. Event-sourcing is reserved for the thing that
actually has replay/undo value: position.

### 2.3 `domain.Folder` — new, and small on purpose

```go
type Folder struct {
	ID     string
	Name   string
	RepoID string // "" for a project-home folder, a repo id otherwise — same golden rule checkFolderContainer already enforces
}
```

Replaces `ChatTypeFolder` entirely. `ChatTypeFolder` and `domain.ChatType` itself stop
needing a `folder` value.

### 2.4 `domain.Workspace` and `domain.Repository`, referenced directly

A locked branch, a repo's own checkout, and project home stop needing a chat minted
or adopted to represent them (`BackfillOwningChats`, `EnsureOwningChat`, `adopt`,
`foldOwningChats` — all of `owning_rows.go`'s reason to exist — deleted, §3 Task 5).
`Node{Kind: workspace, ID: workspace.ID}` IS the row; the frontend reads branch name,
lock status, and default/home flags straight off `Workspace` (and, for a repo's own
row, `Repository`) the same way it already reads a folder's name off `Folder`.

**The 1:N / 1:1 line this draws, precisely:**

- `Workspace`/`Repository` rows (`Kind: workspace` / `Kind: repo`) — CONTAINERS. Many
  `Node{Kind: chat}` rows may have one as their `ParentID`. Never a conversation
  themselves.
- `Chat` rows — conversations. May optionally own AT MOST ONE worktree — an
  ad-hoc, unlocked fork (today's `ChatTypeChat` with `WorkspaceID` set is already
  exactly this; nothing changes here). A chat's own worktree is never locked,
  default, or home — the moment a worktree becomes one of those, it is a
  `Workspace`-kind row going forward, not a chat wearing one.

`Workspace.ParentID` (fork lineage — which branch this one was forked FROM, a git
fact) is unrelated to `Node.ParentID` (which folder this branch is organized under,
a UI fact) and stays exactly where it is — dragging a branch into a "Bugs" folder
must never touch what it forked from.

### 2.5 Commands

- `Create{ID, Kind, ParentID, Order}` — mint a `Node` (new chat, new folder, repo
  import, workspace fork/lock).
- `SetOrder{Order}` — order-only; every OTHER row a densify pass shifts to make room.
- `SetPlacement{ParentID, Order}` — parent + order; the one row actually dragged.
- `Delete{}` — remove a `Node`. Promotion-on-delete (today's folder `Delete`,
  re-parenting children to the deleted folder's own parent) stays USECASE-level
  orchestration — a sequence of `SetPlacement` calls followed by one `Delete` — not a
  new low-level command, mirroring how `Delete` is already built today.

### 2.6 Golden rule, expressed once

Today's container rules — `checkFolderContainer`'s repo-scope match (chat package),
`checkChatContainer`/`checkParentKind`'s same-workspace-only-under-a-plain-chat rule,
and this session's now-deleted `validateRepoFolder` (project package) — collapse into
one check on `Node.Create`/`SetPlacement`, expressed against the walk in §2.1:

- A `repo`-kind `Node`'s destination must resolve to project home — never a
  repo-scoped container, including its own.
- A `folder`-kind `Node` keeps today's rule: home stays with home, a repo's own
  folder stays with that repo.
- A `chat`-kind `Node` may file under a folder or a workspace/repo row with NO
  workspace check (organization only, no lineage implied) — under ANOTHER chat, the
  same-workspace rule applies, because that is thread lineage: it determines what
  conversation this one continues, not just where it renders.
- A `workspace`-kind `Node` may only be re-filed within its own repo's tree — never
  into project home or another repo (a branch is not a repo's own entry).

---

## 3. Migration, phased

Each phase leaves the tree fully working and fully tested — nothing waits for a
later phase to be correct.

1. **Build `Node`** — domain, commands, read-model projection (a GORM `nodes` table,
   synced by a save-only projection off the same event stream `agentchat`'s own store
   already uses, lazy self-heal via `asynx.Replay` on an empty/stale read), its own
   test suite.
2. **Build `domain.Folder`** — plain GORM CRUD, its own store, its own tests.
3. **Migrate repos onto `Node`** — delete `Repository.FolderID`/`Order`,
   `placeRepoInHomeContainer`, `densifyRepos`'s cross-aggregate branch, and
   `project.HomeFolders`'s widened surface (all added earlier today as the interim
   fix).
4. **Migrate project-home chats/folders onto `Node`/`Folder`** — retires
   `rows-from-home.ts`'s `HomeRepoPlacement`/`repoPositions` stand-in machinery (also
   built earlier today) entirely: home rows and repo rows become the same kind of
   read, not two reconciled separately.
5. **Migrate repo-internal placement onto `Node`** — branches (referencing
   `Workspace` directly), repo-internal chats/folders. Deletes
   `BackfillOwningChats`, `EnsureOwningChat`, `adopt`, `foldOwningChats`
   (`owning_rows.go`), `ChatTypeBranch`, `ChatTypeFolder`, and `resolveOwnership`'s
   ownership-union complexity (`rows-from-repo.ts`) — all superseded once a
   workspace-kind `Node` can reference `Workspace` directly and needs no chat proxy.
6. **Cleanup pass** — confirm nothing outside this migration still reads
   `Chat.ParentID`/`.Order` for placement purposes (only thread-lineage/cwd
   resolution should remain, per §2.1's walk), confirm `go build ./...` and the full
   suite are green with the old fields actually removed, not just unused.

This is the full scope — every row kind, every tree level. Nothing is deferred to a
"someday" phase; phases 1-2 are foundation, 3-6 are migration + deletion of the code
this replaces.

---

## 4. What this deletes

Not just adds. Confirms the redesign is a net simplification, not a net new layer
beside the old one:

- `Repository.FolderID`, `Repository.Order` (added today, removed today's other
  interim code with them).
- `placeRepoInHomeContainer`, `densifyRepos`'s cross-aggregate branch,
  `project.HomeFolders`'s widened surface (all interim, this session).
- `rows-from-home.ts`'s `HomeRepoPlacement`/`repoPositions` stand-in injection
  (interim, this session).
- `owning_rows.go` in full: `BackfillOwningChats`, `EnsureOwningChat`, `adopt`,
  `alreadyOwned`, `owningChatType`, `forkParentOf`, `owningRows`, `preferred`,
  `rootsFirst` — the entire backfill exists to solve "give a workspace a chat id the
  tree can address," which stops being a problem once a workspace-kind `Node` can
  reference `Workspace` directly.
- `foldOwningChats` (`rows-from-repo.ts`) and `resolveOwnership`'s ownership-union
  read (`chatOfWorkspace`/`ownerOfChat`) — the two-sided reconciliation between a
  `Workspace` record and its (formerly adopted) owning chat, needed only because the
  chat WAS the row's identity.
- `ChatTypeBranch`, `ChatTypeFolder` — `domain.ChatType` needs no enum once `Chat`
  represents exactly one thing.

---

## 5. Undo

Deliberately deferred, and deliberately not precluded. The per-row command
granularity (§2.2) is the prerequisite; the feature itself — a `GestureID`
correlating one drag's set of commands, an `Undo(gestureID)` that replays each
touched `Node` to its pre-gesture version via `asynx.Replay` and re-emits it as a new
command — is a separable addition once `Node` exists and is proven correct in
production, not a dependency of this spec.

---

## 6. Open questions

1. §2.6's container rules are stated as invariants here; the plan's Task for §2.6
   should verify each against the FULL existing test suite for
   `checkFolderContainer`/`checkChatContainer`/`checkParentKind` before those
   functions are deleted — every existing case they cover needs a `Node`-side
   equivalent, not just the ones this spec's authors thought to name.
2. Cross-project home-folder scoping (today's `validateRepoFolder`'s own disclosed
   gap: a root-level home `Folder` carries no field naming which project it belongs
   to) is inherited, not introduced, by this migration. `domain.Folder`'s `RepoID`
   (§2.3) is exactly as scoped as today's chat-based folder was — closing the gap
   for real (a project anchor on every root-level home folder) is a candidate for
   Phase 4's task but not mandatory to it; decide when Task 4 is scoped in detail.
3. Does a `Node{Kind: repo}`'s `Delete` need to run before or after
   `domain.Repository` itself is deleted, or can they race safely? Confirm against
   the existing repo-delete usecase before Task 3 lands.
4. Phase 5 deletes `ChatTypeBranch` — confirm no OTHER consumer (telemetry, an
   external API contract, a frontend type guard outside the files this spec already
   names) matches on that value before it is removed; a grep pass is Task 5's own
   first step, not assumed clean here.
