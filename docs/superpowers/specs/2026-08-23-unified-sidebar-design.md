# The unified sidebar — one forest, one edge

**Date:** 2026-08-23

**Status:** design spec.

**Baseline.** This document assumes
[`2026-08-23-agents-chat-rearchitecture-design.md`](./2026-08-23-agents-chat-rearchitecture-design.md)
has **landed in full** — `engine/agents` owns the runner and the protocol,
`app/repositories/chat` is one aggregate holding thread, title, placement,
selection and activity, `app/usecases/chat` wires the engine to asynx and to the
frontend, and `api/v0/endpoints/chat` serves the routes. Nothing here revises a
decision in that spec. It extends three of its surfaces: the chat aggregate's
type field, the placement tree under `usecases/chat/internal/tree`, and the
route scope.

**Scope:** the sidebar, the workspace half of the usecase layer, and the two
frontend trees. `engine/agents`, the descriptor, and the protocol work are out
of scope and are not touched.

**Not in scope:** migration. Every fork workspace on disk today has no chat at
all (`worktree.CreateChild` mints none), so a backfill is owed. It is
deliberately deferred to its own session and this document only records the
assumption.

---

## 1. What is wrong today

Measured on `enhancement/unify-sidebar`.

| package | prod | test | what it is |
|---|---|---|---|
| `app/repositories/workspace` | 3,177 | 3,117 | asynx aggregate — **already the reference shape** |
| `app/usecases/worktree` | 1,990 | 4,230 | hierarchy, git verbs, cascade delete |
| `app/usecases/agentchatfolder` | 1,190 | 1,311 | the Chats tree |
| `api/v0/endpoints/workspaces` | 1,324 | 2,400 | 13 routes |
| `app/usecases/folder` | 896 | 1,009 | the sidebar tree |
| `app/usecases/workspace` | 440 | 575 | reads, merge eligibility |
| `app/tree` | 379 | 468 | the shared forest planner |
| `api/v0/endpoints/folders` | 227 | 427 | 4 routes |

Frontend, one widget implemented twice:

| | lines |
|---|---|
| `components/layout/` sidebar tree (7 files) | 2,744 |
| `features/agent/components/` chats panel (5 files) | 1,769 |

### 1.1 The tree edge was never given a meaning

`app/tree` is already shared by both panels, and its package doc states the
problem out loud rather than solving it:

> The workspace sidebar must never let a drag rewrite git lineage — organisation
> and lineage are separate edges there, and the tree is only ever told the
> organisational one. The chats panel has a single edge, so its drag is required
> to rewrite the very thing the sidebar protects. A core that encoded either rule
> would have to be fought by the other caller, so it encodes neither.

Everything below is downstream of that one unmade decision. Two folder domains,
two placement APIs, two drag implementations, and the `develop` ambiguity all
exist because no answer was ever picked.

### 1.2 One aggregate, two usecases

`domain.Workspace` is served by `usecases/workspace` (reads, merge eligibility)
and `usecases/worktree` (create, reparent, merge, cascade delete). The agent
subsystem failed as *one usecase with five concerns*; this fails as *one
aggregate with two usecases*. Opposite symptoms, same cause: the boundary was
never drawn at the aggregate.

Two of the baseline spec's own principles are violated outside the agent tree:

- `usecases/worktree/worktree.go` imports `os` and `path/filepath` — the **only**
  non-agent usecase that touches the filesystem.
- `usecases/branchreview/outline_cache.go` holds a `sync.Mutex` — the **only**
  non-agent usecase type that does. *"If a type in the usecase has a mutex, it is
  in the wrong layer."*

Neither package has an `internal/`; `usecases/worktree` is five flat files.

### 1.3 The same concept is declared twice

`domain.Folder` and `domain.AgentChatFolder` are near-verbatim copies — same
shape, same field comments, same rationale for being a plain GORM row rather than
an aggregate. They differ only in scope (repo vs workspace). Both their usecases
contain a file named `tree_snapshot.go`.

### 1.4 The hierarchy rules live only in the frontend

`guardReparent` checks self-parent, an unprovisioned parent, and leaf-only. It
has **no repo check**. The same-repo rule exists in exactly one place in the
product: `web/src/components/layout/drop-rules.ts:80`. The API will accept a
cross-repo reparent today and fail somewhere down in git.

The same layer disagrees with itself: `guardReparent` returns
`ErrChildHasChildren` for a non-leaf, while `removal-plan.ts` records that *"a
workspace takes its whole subtree: the delete cascades server-side."* You may
delete a subtree but not move one.

### 1.5 A durable path is derived from a field that must become mutable

```go
chatsDir, err := u.ws.AgentChatsDir(ctx, chat.WorkspaceID)
led, err := ledger.Open(worktreepath.AgentLedgerDir(chatsDir, chat.ID))
```

The transcript's location is a function of the chat's workspace. In the model
below a chat may have **no** workspace at all, and one that has a workspace may
acquire or change it. The same call already reroutes when `WorktreePath` is
empty or outside crowbar home, so a blocked workspace's chat writes its
transcript to one directory while blocked and a different one once provisioned.
That bug is live today; the model makes it structural.

### 1.6 The frontend resolves one global 33 times

33 call sites read the active workspace from a single global. With two pane
groups holding chats from two different workspaces, "which workspace do Files and
Git show" has no answer that a global can give.

---

## 2. Principles

The baseline spec's three, unchanged and applied here:

1. **One public file per package.** `<name>.go` is the package's whole exported
   face; everything else lives under that package's `internal/`.
2. **Engines never cross-import.**
3. **The usecase holds no machinery.** If a type in the usecase has a mutex, it
   is in the wrong layer.

Two more, specific to this work:

4. **One edge.** The sidebar is a single ordered forest with a single parent
   edge. Every structural question — what a row runs in, what it forks from,
   what it reads — is a walk up that one edge.
5. **Derive, do not store.** A fact recoverable from the edge is not a field. A
   second field restating a derivable one is a corruption waiting for the two to
   disagree.

---

## 3. The model

> **The sidebar is one ordered forest of rows; a workspace is a resource a row
> may own.**

### 3.1 Four rows, two facts

`domain.ChatType` already exists as a forward-compatible marker with one live
value and no v0 writer for the second. It becomes the closed row taxonomy:

```go
const (
    ChatTypeChat     ChatType = "chat"      // a conversation; opens
    ChatTypeBranch   ChatType = "branch"    // a locked branch, a repo home, the project home
    ChatTypeFolder   ChatType = "folder"    // organisation only
    ChatTypeWorkflow ChatType = "workflow"  // unchanged, still no writer
)
```

with `Chat.WorkspaceID` becoming **optional**. Two facts, four rows:

| row | `Type` | `WorkspaceID` | opens | carries a branch |
|---|---|---|---|---|
| worktree chat | `chat` | set | yes | yes |
| bubble chat | `chat` | empty | yes | no |
| locked branch (`develop`, repo home, project home) | `branch` | set | **no** | yes |
| folder | `folder` | empty | no | no |

`domain.Folder` and `domain.AgentChatFolder` are both deleted into the `folder`
row. A locked branch is a folder that happens to hold a worktree — that is
precisely why a chat can sit under it owning nothing.

**Locked means no commits and no branches here, not no process here.** A bubble
under `develop` runs in `develop`'s worktree. That is what `develop` having a
workspace is for.

### 3.2 Three walks, one edge

Every structural question is one traversal of `ParentID`, and `folder` rows are
transparent to all three:

| question | walk |
|---|---|
| **cwd** — where does this CLI run? | nearest ancestor-or-self with a **provisioned** `WorkspaceID` |
| **fork parent** — what does a new branch fork from? | same walk, excluding self |
| **reads** — what conversation does this inherit? | ancestors with `Type == chat` (`chatlineage.Walk`, unchanged) |

The cwd walk's "provisioned" qualifier is the whole handling of a blocked
workspace: the slot is set, the workspace has no `WorktreePath`, the walk
continues past it to the ancestor. The row stays visibly blocked and stays
usable. There is no special case and no placement change — a blocked row that
were re-parented to fix its cwd would strip its ownership, orphan the
unprovisioned workspace, and drop out of the tree.

### 3.3 What is derived

Nothing about ownership is stored. `ownsWorktree` is `WorkspaceID != ""`;
`forkParentID` is a walk. **`Workspace.ParentID` becomes a maintained
projection** of the fork-parent walk rather than an authored field, so the three
consumers the source names — merge eligibility, the diff base, the reparent leaf
guard — keep reading what they read today. `Workspace.FolderID` and
`Workspace.Order` are deleted outright: placement lives on the row.

---

## 4. The verbs

### 4.1 Create

One command replaces every create path:

```go
type CreateChild struct {
    ParentID    string // a row, or "" for the project root
    OwnWorktree bool
    ProviderID  string
}
```

`false` sets no `WorkspaceID` and does no git work at all. `true` resolves the
fork parent and cuts a branch from it.

**The default is inherited from the parent.** Under a row that carries a branch
the new row is a worktree; under a chat that carries none it is a chat. Nobody
sets the kind twice in a row. A child of a locked row therefore defaults to
owning a worktree, which is the `develop` rule falling out rather than being
written.

The branch name is generated server-side — the generator must collision-check
against real refs, which a client cannot do — and the row is marked provisional
until renamed. `Workspace.BranchProvisional` clears on rename. An agent-facing
`set_branch_name` tool mirrors `set_chat_title`'s `user > agent > derived`
precedence, so the agent renames the branch once the task is achieved.

`RenameBranch` is safe to call from an agent running inside the branch's own
worktree: since its rewrite it is *"a git ref rename and one record write —
nothing on disk is touched."* No process is displaced.

### 4.2 Promotion

A bubble becomes a worktree chat by filling an empty slot. It keeps its id, its
title, its placement and every turn it has taken; only the ground under it
changes.

1. `worktree.CreateChild` from the resolved fork parent.
2. Set `WorkspaceID` on the row.
3. Tear down the runner, `AssembleHandoff`, respawn the same provider in the new
   cwd.
4. Append a `[Crowbar]` ledger note recording the move, as `lineageNoteText`
   already does for a lineage change.

Step 3 is not new machinery. `SwitchProvider` performs exactly this — quit,
assemble the ledger, start a new runner with the handoff injected — for a
different reason. Native resume is unavailable because a vendor session is
cwd-keyed, so promotion always takes the existing *"spawned fresh with the whole
ledger"* branch.

**A worktree is never demoted.** Git could prove a clean tree safe to discard;
the operation is still not offered. The branch and its commits would have
nowhere to go.

### 4.3 Movement

A move re-points the ground under a row: a bubble gets a new cwd, a worktree row
gets replayed onto a new parent. Both require the CLI to be torn down and
respawned.

- **A move takes the whole subtree, and so does a delete.** One rule, both
  directions. The current split — cascade on delete, `ErrChildHasChildren` on
  reparent — goes.
- **A working row does not move.** Working means the row itself or any row in the
  subtree it would take. The refusal is expressed by removing every drop target,
  not by colouring one: nothing can be landed on, so there is no near-miss to
  hunt for.
- **A bubble moves freely; a worktree row's move is a rebase** and stays behind
  the git guards. A bubble carrying worktree-owning descendants is a rebase too,
  because those descendants resolve their fork parent past it.
- **Cross-repo movement is legal only for rows owning no worktree**, subtree
  included. A different repo means a different remote, branch and worktree path,
  so it is not a reparent at all.

---

## 5. Target tree

```
core/paths/worktreepath/         MOVED from app/usecases/internal/ — the last
                                 filesystem calls leave the usecase layer

domain/
  chat.go                        THE row. ParentID = the one edge. ChatType is the
                                 closed taxonomy. WorkspaceID optional and mutable.
  workspace.go                   resource only; ParentID demoted to a projection,
                                 FolderID and Order deleted
  folder.go                    ✗ deleted → ChatTypeFolder
  agent_chat_folder.go         ✗ deleted → ChatTypeFolder

app/
  tree/                          unchanged code; its "encodes neither" caveat is
                                 deleted, because there is now one edge to encode

  usecases/
    chat/
      chat.go  types.go
      internal/
        translate/               (baseline)
        fanout/                  (baseline)
        tree/                    the WHOLE sidebar forest — chats, branches and
                                 folders, every repo. Absorbs agentchatfolder and
                                 folder/tree_snapshot.go.
        promote/                 fill the workspace slot; handoff respawn
        tools/                   (baseline) + set_branch_name
    workspace/
      workspace.go               worktree/ merged in: one aggregate, one usecase
      internal/
        hierarchy/               fork point, merge eligibility, cascade
        provision/               worktree add / remove / repair
    worktree/                  ✗ deleted → workspace/
    folder/                    ✗ deleted → chat/internal/tree
    agentchatfolder/           ✗ deleted → chat/internal/tree
    branchreview/                outline cache moves to the adapter layer

  repositories/
    chat/
      chat.go                    (baseline) + the row taxonomy and the workspace slot
      internal/
        commands/                + SetWorkspace, SetPlacement
        content/                 keyed by CHAT id, never by workspace
    workspace/                   already the reference shape; keeps it
      internal/commands/         − reparent.go, set_placement.go → chat/

api/v0/endpoints/
  chat/
    chat.go                      /repos/:repoId/chats/...   NOT /workspaces/:wsId/
    internal/handlers/
      tree.go                    the one placement route
  workspaces/                    − /reparent
  folders/                     ✗ deleted → chat/internal/handlers/tree.go
```

### 5.1 The wire contract

```
GET    /repos/:rid/tree                 the forest — chats, branches, folders
PATCH  /repos/:rid/tree                 {id, parentId, index} — the ONE placement call
POST   /repos/:rid/chats                {parentId, ownWorktree, providerId}
POST   /repos/:rid/chats/:id/promote    fill the workspace slot
GET    /repos/:rid/workspaces/:wsId     resource only: git status, counts, PR, lock
WS     /repos/:rid/stream               one stream, replacing two
```

Each row ships its walks **already resolved** — `ownsWorktree`, `workspaceId`,
`forkParentId` — so the client never re-implements `chatlineage`. A client that
computes them is a second implementation that will drift.

Chats stop being addressed through a workspace. That is not cosmetic: a row's
workspace is now optional and mutable, so a URL that names one is a URL that goes
stale.

---

## 6. Invariants

Load-bearing. Each needs a test that fails when it is inverted.

1. **A row is one of four kinds, and only a `chat` row carries a conversation.**
   The taxonomy is closed; a fifth kind is a deliberate vocabulary change.
2. **No durable path is derived from `Chat.WorkspaceID`.** It is mutable and may
   be empty. The ledger and the content store key on the chat id.
3. **Ownership is never stored.** `ownsWorktree` is the emptiness of one field.
   A second field restating it is rejected in review and in test.
4. **A move and a delete take the same set** — the row and its whole subtree.
5. **A working row does not move**, where working includes any row in the subtree
   that would travel with it.
6. **A worktree is never demoted**, clean tree or not.
7. **Cross-repo movement requires that no row in the moving subtree owns a
   worktree.**
8. **The runner points at the chat; the chat never points back.** Carried
   forward verbatim from the baseline spec, unchanged by any of the above.

---

## 7. Sequencing

Each stage is independently green on the full CI gate and committed before the
next begins.

| stage | change | why there |
|---|---|---|
| **1** | `ChatType` gains `branch` and `folder`; both folder domains fold into it; placement becomes one table. | Nothing else can be expressed until the taxonomy exists. Independent of every other stage. |
| **2** | `Chat.WorkspaceID` becomes optional and mutable; the ledger and `internal/content` re-key onto the chat id. | §1.5 is a live bug. It must be fixed before anything makes the field move. |
| **3** | The three walks land in `usecases/chat/internal/tree`. `Workspace.ParentID` becomes a maintained projection; `FolderID` and `Order` are deleted. | Depends on 1. Everything downstream reads these. |
| **4** | Drop rules move server-side. Move takes the subtree; the working-row refusal. | Depends on 3 — legality is a question about walks. |
| **5** | One `CreateChild`; promotion via handoff respawn; generated provisional branch names; `set_branch_name`. | Depends on 2 and 3. |
| **6** | `usecases/worktree` + `usecases/workspace` merge; `worktreepath` promoted to `core/paths`; the `branchreview` mutex leaves the usecase layer. | Independent of 1–5; can run in parallel. |
| **7** | Routes: chats leave `/workspaces/:wsId/`; one placement route; `endpoints/folders` folds in. Frontend clients updated in the same commit. | Last on the backend — it breaks every agent URL. |
| **8** | Frontend: one tree renderer; pane layout hoisted out of the per-workspace store registry; active workspace derives from the focused group. | Depends on 7. |

Stage 8 is the largest single piece of frontend work and the one with a known
trap: `pane-slice.ts` lives in the workspace store registry, and that registry is
**destroyed on workspace switch**. Two groups holding chats from two different
workspaces cannot both live in a per-workspace store. The layout becomes
window-level; each group's content keys off the chat id and resolves its
workspace through the row.

---

## 8. Success criteria

- No package under `app/usecases` imports `os` or `path/filepath`.
- No type under `app/usecases` holds a mutex.
- One folder concept exists in `domain/`; `grep -r AgentChatFolder` returns
  nothing.
- One placement route; `endpoints/folders` is gone; no chat route names a
  workspace.
- The frontend has one tree renderer, and `drop-rules.ts` no longer decides
  legality — it may only style what the server has already answered.
- Each of the eight §6 invariants has a test that fails when the invariant is
  inverted.
- Coverage at or above the current floor at every stage boundary.

---

## 9. Open questions

1. **Is `chat` still the right name?** The aggregate now holds folders and locked
   branches as well as conversations. Renaming it a second time is expensive and
   the baseline spec has only just landed the first rename; the alternative is an
   aggregate whose name describes one of its four row kinds.
2. **Cross-repo movement and conversation lineage.** A bubble moved to another
   repo keeps reading chat ancestors that live in the repo it left. That is
   coherent — lineage is what it reads, not where it runs — but it means a
   repo's chats are not a closed set, and `GET /repos/:rid/tree` cannot resolve
   every ancestor it references.
3. **Migration**, deferred by decision. Every fork workspace on disk has no chat,
   so an owner row must be minted for each before any of this renders.
4. **Demotion when the tree is clean** is refused by §4.2 rather than gated on
   git. Recorded as a decision, not an oversight.
5. **What the ⌘K switcher lists.** Drawn as openable rows only — no folders, no
   locked branches, no repo homes.

---

## 10. Reference

The design canvas is the visual half of this document:
<https://claude.ai/code/artifact/5a1008de-282b-494a-bd8d-1b9c123efdee>

Frames of record: **Kind** (the row taxonomy, the create control, inherited
defaults, promotion), **Busy** (the refused move), **Drag** (the permitted one),
**Twoup** (one chat per pane group), **Naming** (the provisional state).
