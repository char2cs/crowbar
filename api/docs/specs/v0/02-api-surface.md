# Crowbar Backend — API Surface

> **Status:** Approved
> **Date:** 2026-06-03
> **Depends on:** `00-architecture-and-domain.md`
> **Scope:** The complete REST + WebSocket route table. Per-subsystem behavior
> (git execution, file watching, PTY, LSP, etc.) is detailed in the companion
> specs; this document is the canonical route index.

---

## 1. Conventions

- **All routes are prefixed `/v0/`** (REST and WebSocket alike), matching
  quiver.core's `v.Prefix()`. There is no `/api` segment.
- **Uniform response envelope** (quiver.core style):
  ```
  { success: bool, error: string|null, data: any? }
  ```
  Mutation responses carry the affected entity id; query responses carry `data`.
- **All path IDs are UUIDs.**
- **Dual-serve via `dispatch()`** — live-data read routes serve a normal REST
  response by default and **upgrade to their WebSocket stream when the request
  carries `Upgrade: websocket`**. One URL, two modes. Applied to the live-read
  routes noted with **(dual)** below.
- **Branch-name path params** (`:branch`) are URL-encoded (branch names may
  contain `/`).

---

## 2. REST Routes

Grouped by subsystem. **Bold** = state-changing. **(dual)** = also serves its WS
stream on `Upgrade: websocket`.

### 2.1 Projects & Repositories — UX §1, §2

```
GET    /v0/projects                       list all projects
POST   /v0/projects                       import a project { name, path }
GET    /v0/projects/:id                    project detail
GET    /v0/repos                            list repos (?projectId= filter)
GET    /v0/repos/:id                        repo detail + workspace tree
```

### 2.2 Workspaces — UX §2, §3.2, §22 (worktrees)

See `07-workspace-worktree-hierarchy.md` for worktree mechanics, local merge,
and re-parenting.

```
GET    /v0/workspaces            (dual)     list (?repoId=) — flat, tree built client-side
GET    /v0/workspaces/:id                    full Workspace object (see 00 §5.3) — REST only
                                            (live updates for this row arrive on the global /v0/ws/workspaces)
POST   /v0/workspaces                       create { repoId, branch, parentId? } → status:new
                                            (locked resolved via provider engine)
DELETE /v0/workspaces/:id                   delete (cascades to children, skips locked)
POST   /v0/workspaces/:childId/merge-into-parent  { strategy }   local child→parent merge
POST   /v0/workspaces/:childId/reparent           { newParentId } rebase --onto (child must be a leaf)
```

> `GET /v0/workspaces/:id` returns the **full `Workspace`** (`00` §5.3) and is
> **REST-only** — it is not dual-serve, because there is no single-`:id` WS stream
> (the global `/v0/ws/workspaces` already carries this row's live updates, keyed by
> `id`). Only the **list** route `GET /v0/workspaces` is dual-serve, upgrading to
> that global stream. (The minimal `WorkspacePayload { id, repoId, branch }` from
> UX §3.2 is just the subset the navigation resolver reads; not a separate shape.)

### 2.3 Chats (lifecycle only) — UX §4

```
GET    /v0/workspaces/:wsId/chats           flat list for workspace
POST   /v0/workspaces/:wsId/chats           create root chat { title? }
POST   /v0/chats/:id/fork                    fork (from current tip) → child with parentId
PATCH  /v0/chats/:id                          rename { title }
DELETE /v0/chats/:id                          delete
```
> Message sending + agent response stream are **post-spike** (Agentic Bridge).

### 2.4 File Explorer & Content — UX §6, §21, §31

```
GET    /v0/workspaces/:wsId/files/tree       file tree (?path= for lazy subtree)
GET    /v0/workspaces/:wsId/files/content    read { path } → { content, encoding }
PUT    /v0/workspaces/:wsId/files/content    save { path, content }
POST   /v0/workspaces/:wsId/files             new file/folder { path, type }
PATCH  /v0/workspaces/:wsId/files             rename/move { path, newPath }
DELETE /v0/workspaces/:wsId/files             delete { path }
```

### 2.5 Editor Support & LSP — UX §7, §26

```
GET    /v0/workspaces/:wsId/blame              git blame { path } → BlameEntry[]
POST   /v0/workspaces/:wsId/lsp/completion     completions { path, position }
POST   /v0/workspaces/:wsId/lsp/hover          hover docs { path, position }
POST   /v0/workspaces/:wsId/lsp/definition     go-to-definition { path, position }
POST   /v0/workspaces/:wsId/lsp/references     find references { path, position }
POST   /v0/workspaces/:wsId/lsp/rename         rename symbol { path, position, newName }
POST   /v0/workspaces/:wsId/lsp/codeAction     quick fixes { path, range }
POST   /v0/workspaces/:wsId/lsp/documentSymbol symbols in a file (go-to-symbol, UX §16) { path }
GET    /v0/workspaces/:wsId/lsp/diagnostics    current diagnostics (also pushed via WS)
```
LSP request/response stays on REST (low-latency, request-scoped). Only
**diagnostics** push over WS (they arrive asynchronously).

### 2.6 Git — Read — UX §8, §9, §10

```
GET    /v0/workspaces/:wsId/git/status   (dual)   GitStatus { branch, ahead, behind, files[] }
GET    /v0/workspaces/:wsId/git/log               ?limit=50&skip=0 → Commit[]
GET    /v0/workspaces/:wsId/git/diff              working-tree or commit diff (?path=, ?commit=)
GET    /v0/workspaces/:wsId/git/branches          Branch[]
GET    /v0/workspaces/:wsId/git/stashes           Stash[]
```
Each hunk in a returned diff carries a stable **`hunkId`** (hash of its `@@`
header + content) so the frontend can address it for hunk-level staging.

### 2.7 Git — Write — UX §8, §22

```
POST   /v0/workspaces/:wsId/git/stage             { paths[] } or hunk { path, hunkId }
POST   /v0/workspaces/:wsId/git/unstage           { paths[] } or hunk { path, hunkId }
POST   /v0/workspaces/:wsId/git/discard           { paths[] }
POST   /v0/workspaces/:wsId/git/commit            { subject, body? }
POST   /v0/workspaces/:wsId/git/push              { }
POST   /v0/workspaces/:wsId/git/pull              { }
POST   /v0/workspaces/:wsId/git/fetch             { }
POST   /v0/workspaces/:wsId/git/branches          create { name, source? }
PATCH  /v0/workspaces/:wsId/git/branches/:branch  rename { newName }
DELETE /v0/workspaces/:wsId/git/branches/:branch  delete branch
POST   /v0/workspaces/:wsId/git/checkout          switch branch { branch }
POST   /v0/workspaces/:wsId/git/stash             { message? }
POST   /v0/workspaces/:wsId/git/stash/:id         apply/pop { mode: "apply"|"pop" }
DELETE /v0/workspaces/:wsId/git/stash/:id         drop
POST   /v0/workspaces/:wsId/git/reset             { mode: "soft"|"mixed"|"hard", commit }
POST   /v0/workspaces/:wsId/git/merge             { branch }
POST   /v0/workspaces/:wsId/git/rebase            { onto }
```

**Hunk-level staging** uses a backend-assigned `hunkId`: the frontend sends
`{ path, hunkId }`, the backend reconstructs the patch fragment and pipes it to
`git apply --cached`. The frontend never deals with raw patch text.

### 2.8 Conflict Resolution — UX §24

```
GET    /v0/workspaces/:wsId/git/conflicts          conflicting files + ConflictHunk[]
POST   /v0/workspaces/:wsId/git/conflicts/resolve  { path, conflictHunkId, resolution, resolvedContent? }
```
> `conflictHunkId` is `ConflictHunk.id` (§2.8 / `04` §6) — distinct from the
> staging `hunkId` of §2.7 / `04` §4. Named explicitly to avoid conflating them.

### 2.9 Branch Review — UX §11

See `09-branch-review.md`.

```
GET    /v0/workspaces/:wsId/review                    BranchReview { description, mergeStrategy, diff, threads, conversations }
PATCH  /v0/workspaces/:wsId/review                    set merge strategy { mergeStrategy }
POST   /v0/workspaces/:wsId/review/threads            post comment { filePath, lineNumber, side, body }
POST   /v0/workspaces/:wsId/review/threads/:id/reply  reply { body }
PATCH  /v0/workspaces/:wsId/review/threads/:id        resolve/reopen { isResolved }
```

> **No PR-create endpoint.** Crowbar never creates PRs on the provider — the
> user or an agent does (e.g. `gh pr create`). Crowbar only *reads* PR state.

### 2.9b Git Provider (read-only) — UX §2, §23

See `08-git-provider-engine.md`. Live PR/protection updates ride the Workspaces
WS; these REST routes are on-demand reads.

```
GET    /v0/workspaces/:wsId/provider       current PRInfo + protected flag
GET    /v0/repos/:id/protected-branches    protected-branch list for the repo
```

### 2.10 Global Search — UX §25

```
POST   /v0/workspaces/:wsId/search           { query, caseSensitive, wholeWord, regex, include[], exclude[] } → SearchResult[]
POST   /v0/workspaces/:wsId/search/replace   { query, replacement, scope }
```

### 2.11 Terminal — UX §12, §29

```
GET    /v0/settings/terminal/profiles         list profiles
POST   /v0/settings/terminal/profiles         create
PATCH  /v0/settings/terminal/profiles/:id      update
DELETE /v0/settings/terminal/profiles/:id      delete
POST   /v0/workspaces/:wsId/terminals          create PTY session { profileId? } → { sessionId }
DELETE /v0/terminals/:sessionId                kill session
```

### 2.12 Health

```
GET    /v0/health
```

---

## 3. WebSocket Endpoints

Each is a `Broadcaster[T]` (quiver.core pattern). The **namespace key** (in
parentheses) scopes delivery so a client subscribed for one workspace/chat/
session never receives another's events.

```
WS  /v0/ws/workspaces?repoId=        (global)     full Workspace objects: status, +N/-N, hasConflicts, PR, agent-running
WS  /v0/ws/chats?wsId=               (wsId)       chat list status (idle ↔ agent-running), payload carries chatId
WS  /v0/ws/git?wsId=                 (wsId)       live GitStatus on every disk change
WS  /v0/ws/files?wsId=               (wsId)       FileChangeEvent (created/modified/deleted/renamed)
WS  /v0/ws/lsp?wsId=                 (wsId)       Diagnostic[] pushes
WS  /v0/ws/terminals/:sessionId      (sessionId)  bidirectional PTY stream
WS  /v0/ws/chats/:chatId/stream      (chatId)     agent response stream — POST-SPIKE
```

The dual-serve routes in §2 (marked **(dual)**) expose the same data on their
REST URL when upgraded; the dedicated `/v0/ws/*` endpoints above are the
always-WS channels for push-only event streams.

---

## 4. Open Items Tracked Elsewhere

- Agent **message-send** + **content stream** frame protocol — post-spike
  (`12-agentic-bridge-spike.md`).
- Detailed payload shapes per subsystem — in each subsystem's own spec
  (git, files, terminal, review/PR, LSP, search).
