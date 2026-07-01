# Backend API + WebSocket + Storage Refactor

> **Status:** Design spec — not yet an implementation plan.
> **Scope:** In scope for `enhancement/ide-final-polish`. Goal is a production-ready IDE without chats.

---

## Vision & Principles

**Fail fast (HTTP), good path (WebSocket).** Every write endpoint validates synchronously and returns immediately. The happy-path result — workspace created, push completed, merge done — flows to the client exclusively via WebSocket. HTTP acknowledges; WebSocket delivers.

**Entity-scoped storage.** Each aggregate's data lives on disk inside the entity it belongs to. Deleting a workspace deletes its database. You can move a project directory and get all its state. This is the same model quiver.core uses per-namespace, but hierarchical: project → repo → workspace.

**Hierarchical routes.** HTTP and WebSocket routes mirror the entity tree. A WebSocket client subscribes to exactly the scope it needs — one workspace, all workspaces under a repo, or all repos under a project. No global `/ws/*` topics filtered client-side.

**Frontend virtualization.** The UI never re-fetches after a mutation. It holds a live entity cache, seeded by HTTP GET on first load, updated by WebSocket events in real time. Components read from cache. This eliminates the post-mutation fetch race that currently causes stale UI after branch import.

---

## 1. Filesystem Layout

### Current (URL-derived, not UUID-based)
```
~/.crowbar/
  state/
    events/workspace.db       ← ALL workspaces, all projects
    events/chat.db
    events/agent_run.db
    events/review_thread.db
    store/crowbar.db          ← ALL GORM rows: projects, repos, workspaces, chats
  projects/
    github.com/owner/repo/    ← derived from git remote URL
      workspaces/<wsId>/      ← git worktree root (git files spill here)
```

### Target (UUID-based, entity-scoped)
```
~/.crowbar/
  state/
    event_stream.db           ← global Asynx event store (app-level events only)
    view.db                   ← global GORM view/projection (terminal profiles, settings)
  projects/
    <PROJECT_UUID>/
      storages/
        event_stream.db       ← project Asynx event store
        view.db               ← project GORM projection (project row)
      <REPO_UUID>/
        icon                  ← stored icon bytes (png/jpeg/webp), no extension
        storages/
          event_stream.db     ← repo Asynx event store
          view.db             ← repo GORM projection (repo row)
        workspaces/
          <WS_UUID>/
            worktree/         ← git worktree lives here (clean boundary)
            threads/          ← review thread data
              storages/
                event_stream.db
                view.db
            terminals/        ← terminal session data
              storages/
                event_stream.db
                view.db
            chats/            ← TODO: chat (multi-agent conversation) — not in scope
            storages/
              event_stream.db ← workspace Asynx event store
              view.db         ← workspace GORM projection (status, branch, git counters, merge state)
```

### Key consequences

- **Worktree path** is always `<crowbarHome>/projects/<P>/<R>/workspaces/<W>/worktree/`. `worktreepath.go` takes `(crowbarHome, projectId, repoId, wsId)` — no `remoteURL` parameter needed.
- **Icon** is read from `<crowbarHome>/projects/<P>/<R>/icon` by `GET /v0/projects/:p/repos/:r/icon`. No external HTTP fetch, no GitHub proxy roundtrip.
- **Deleting a workspace** = `rm -rf <crowbarHome>/projects/<P>/<R>/workspaces/<W>` — deletes worktree, event store, threads, and terminals atomically.
- **DB initialization** is lazy per entity: opening a workspace's `event_stream.db` / `view.db` happens when the workspace is first accessed, not at daemon startup. Follows quiver's per-path mutex pattern.

---

## 2. Storage Architecture

Each aggregate has two separate databases: `event_stream.db` is the append-only Asynx event log; `view.db` is the GORM read-model (projection). They are always siblings inside the same `storages/` directory but are never mixed.

### Global (app-level)
`~/.crowbar/state/`
- `event_stream.db` — Asynx event store for app-level events (if any)
- `view.db` — GORM: `terminal_profiles`, `settings`

### Project scope
`~/.crowbar/projects/<P>/storages/`
- `event_stream.db` — Asynx event store; stream keys: `project.<event>.<projectId>`
- `view.db` — GORM: `projects` table (id, name, path, created_at)

### Repo scope
`~/.crowbar/projects/<P>/<R>/storages/`
- `event_stream.db` — Asynx event store; stream keys: `repo.<event>.<repoId>`
- `view.db` — GORM: `repositories` table (id, project_id, name, path, default_branch, remote_url, avatar_label, avatar_color, avatar_has_icon)

### Workspace scope
`~/.crowbar/projects/<P>/<R>/workspaces/<W>/storages/`
- `event_stream.db` — Asynx event store; stream keys: `workspace.<event>.<wsId>`
- `view.db` — GORM: `workspaces` table (status, branch, git counters, merge state, provider state)

### Thread scope (inside workspace)
`~/.crowbar/projects/<P>/<R>/workspaces/<W>/threads/storages/`
- `event_stream.db` — Asynx event store; stream keys: `thread.<event>.<threadId>`
- `view.db` — GORM: `threads` table (id, workspace_id, file_path, line, side, body, author, resolved, created_at) + `thread_replies` table (id, thread_id, body, author, created_at)

Threads are file+line code review annotations (GitHub PR comment model): anchored to a specific file path and line number within the workspace diff. They are not commit-level or chat-level constructs. `side` indicates which side of the diff the comment lands on (old/new line).

### Terminal scope (inside workspace)
`~/.crowbar/projects/<P>/<R>/workspaces/<W>/terminals/storages/`
- `event_stream.db` — Asynx event store; stream keys: `terminal.<event>.<sessionId>`
- `view.db` — GORM: `terminal_sessions` table (id, workspace_id, profile_id, created_at, ended_at)

### Asynx stream key format
```
<entity>.<event-name>.<aggregateId>
```
Examples:
- `workspace.created.550e8400-...`
- `workspace.sync_working_tree_state.550e8400-...`
- `repo.icon_updated.b1a2c3d4-...`

Each Asynx instance is scoped to one entity and one DB file. The `adapter.Container` opens DB files lazily.

---

## 3. HTTP + WebSocket Routes

All routes live under `/v0/`. The same URL serves both REST and WebSocket via the quiver-style `dispatch()` middleware (checks `Upgrade: websocket` header).

### Route tree

```
/v0/projects
  GET/WS                          list all projects / stream project events
  POST                            create project (import from local path)

/v0/projects/:projectId
  GET/WS                          get project detail / stream this project's events
  DELETE                          delete project

/v0/projects/:projectId/repos
  GET/WS                          list repos under project / stream repo events
  POST                            add repo to project

/v0/projects/:projectId/repos/:repoId
  GET/WS                          get repo detail / stream this repo's events
  DELETE                          remove repo

/v0/projects/:projectId/repos/:repoId/icon
  GET                             serve icon bytes (from disk, not GitHub)
  PUT                             upload custom icon (multipart)
  DELETE                          reset to generated avatar

/v0/projects/:projectId/repos/:repoId/icon/emoji
  PUT                             set emoji icon

/v0/projects/:projectId/repos/:repoId/icon/github
  PUT                             fetch and store GitHub owner avatar

/v0/projects/:projectId/repos/:repoId/branches
  GET                             list git branches with hasWorkspace flag

/v0/projects/:projectId/repos/:repoId/protected-branches
  GET                             list protected branches from provider

/v0/projects/:projectId/repos/:repoId/workspaces
  GET/WS                          list workspaces / stream workspace-list events
  POST                            create workspace
                                  body: { branch: string, parentId?: string }
                                  returns 202; API resolves internally whether branch exists on
                                  remote (checkout) or not (create from parent then worktree add);
                                  the ready workspace DTO arrives on WS

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId
  GET/WS                          get workspace / stream this workspace's events
  DELETE                          delete workspace + worktree

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/sync
  POST                            202 — triggers working-tree sync, result via WS

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/merge-into-parent
  POST                            202 — triggers merge, result via WS

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/reparent
  POST                            202 — triggers rebase+reparent, result via WS

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/status
  GET/WS                          current status / stream status changes

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/diff
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/log
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/blame
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/branches
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/stashes
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/conflicts
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/commit-diff
  GET                             read-only git queries (sync, always fast)

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/stage
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/unstage
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/discard
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/commit
  POST                            200 OK — fast git ops, sync result is fine

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/push
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/fetch
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/pull
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/merge
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/git/rebase
  POST                            202 Accepted — slow git ops, result via WS

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/files/tree
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/files/content
  GET                             file system reads (sync)

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/files
  POST/PATCH/DELETE/PUT           file mutations (sync, small)

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/files/ws
  WS                              live file change events

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/search
  POST                            search (sync)

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/search/replace
  POST                            replace (sync, but can be large — 202 for big ops)

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/terminals
  GET/WS                          list sessions / stream terminal lifecycle events
  POST                            create terminal session → 201 {sessionId}

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/terminals/:sessionId
  DELETE                          kill terminal session

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/terminals/:sessionId/ws
  WS                              bidirectional PTY stream

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/lsp/ws
  WS                              LSP diagnostics stream

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/lsp/completion
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/lsp/hover
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/lsp/definition
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/lsp/references
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/lsp/rename
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/lsp/codeAction
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/lsp/documentSymbol
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/lsp/diagnostics
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/lsp/didOpen
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/lsp/didChange
/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/lsp/didClose
  POST/GET                        LSP protocol (sync)

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/review
  GET/PATCH                       review state (merge strategy, summary — sync)

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/threads
  GET/WS                          list threads / stream thread events (opened, replied, resolved)
  POST                            open thread (sync → broadcasts ThreadEvent)

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/threads/:threadId
  GET                             thread detail with all replies
  PATCH                           resolve / unresolve thread (sync → broadcasts ThreadEvent)

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/threads/:threadId/replies
  POST                            add reply (sync → broadcasts ThreadEvent)

/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/provider
  GET                             provider state (sync)

// TODO: /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/chats
//   Chat = multi-agent conversation (not in scope for this PR).
//   WorkspaceDTO.Working reflects whether a chat session is currently active.
//   WebSocket for this scope will follow the same Broadcaster[ChatDTO] pattern.

/v0/settings/terminal/profiles
  GET/POST/PUT/DELETE             terminal profiles (sync, global)

/v0/system/prerequisites
  GET                             system check

/v0/health
  GET                             liveness probe
```

---

## 4. Fail Fast / Good Path Async Protocol

### The principle

**Check everything synchronously. Deliver the good result asynchronously.**

This applies **only to Asynx-backed domain entities**: projects, repos, workspaces, threads, terminal sessions. Git operations, LSP, file mutations, and search are pure computation — they stay synchronous and return `200` with their result directly.

### Two distinct response contracts

**Domain entity mutations (Asynx-backed) → 202:**

| Condition | HTTP response | Good path delivery |
|---|---|---|
| Validation fails | `4xx` — work never starts | — |
| Work accepted | `202 Accepted` — empty body | Updated entity DTO arrives on WS |

**Everything else (git ops, LSP, files, search) → 200:**

| Condition | HTTP response |
|---|---|
| Error | `4xx` |
| Success | `200 OK` with result in body |

### What validation happens synchronously (fail fast)

Every check that can be done without starting the work runs before returning `202`:

- Input shape and required fields
- Entity existence (repo exists, workspace exists, branch exists)
- State preconditions (workspace not locked, no pending merge, branch not protected)
- Filesystem preconditions (repo path accessible)
- Conflict detection (branch name already taken)

If any check fails → `4xx` immediately, no work started, no WS event.

### What happens after 202 (domain entity flow)

```
HTTP handler:
  1. Validate synchronously — 4xx on any failure
  2. Return 202 (entity stays in its current state during the operation)

Background goroutine:
  3. Execute the work (worktree add, push, merge, etc.)
  4. Asynx command: transition entity to final state
     — success: status updated, lastError cleared
     — failure: lastError set to the error message; status may also transition
       (e.g. push failure leaves status unchanged but sets lastError)
  5. Broadcaster pushes updated DTO to WS subscribers
```

No "in-progress" state is broadcast between steps 2 and 5. The UI disables the triggering action after the 202 at the call-site level; the entity itself does not carry an in-progress field.

### Domain entity operations (202 + WS outcome)

- `POST .../projects` — project creation
- `DELETE .../projects/:p` — project deletion
- `POST .../repos` — repo registration
- `DELETE .../repos/:r` — repo removal
- `POST .../workspaces` — `git worktree add`
- `DELETE .../workspaces/:w` — `git worktree remove` + rm -rf
- `POST .../workspaces/:w/merge-into-parent`
- `POST .../workspaces/:w/reparent`
- `POST .../workspaces/:w/git/push`
- `POST .../workspaces/:w/git/fetch`
- `POST .../workspaces/:w/git/pull`
- `POST .../workspaces/:w/git/merge`
- `POST .../workspaces/:w/git/rebase`
- `POST .../threads` — open thread
- `POST .../threads/:id/replies` — add reply
- `PATCH .../threads/:id` — resolve/unresolve
- `POST .../terminals` — create terminal session
- `DELETE .../terminals/:id` — kill terminal session

### Synchronous operations (200 + body)

- All GET endpoints
- Stage / unstage / discard / commit (fast local git, result in body)
- File read / write / rename / delete
- Search and replace
- LSP protocol calls
- Icon upload / delete
- Settings CRUD

### Errors belong to the domain object

Errors on async operations are never sent over the WebSocket channel as a separate message. If `git push` fails, `lastError` is set to the error message (e.g. `"remote: Permission denied"`) and the broadcaster pushes the updated `WorkspaceDTO`. The client reads `lastError` and renders it inline alongside the workspace row. The WS channel only ever emits its single typed DTO.

---

## 5. WebSocket Broadcaster Architecture

### Core rule: one channel = one type

Each broadcaster emits exactly one Go type. No envelopes, no `type` discriminators, no `json.RawMessage`. The Go type parameter `Broadcaster[T]` enforces this at compile time. A client subscribed to the workspace channel receives `WorkspaceDTO` objects and nothing else — ever.

### Broadcaster instances

```
Broadcaster[ProjectDTO]         — /v0/projects (and /:projectId)
Broadcaster[RepoDTO]            — /v0/projects/:p/repos (and /:repoId)
Broadcaster[WorkspaceDTO]       — /v0/projects/:p/repos/:r/workspaces (and /:wsId)
Broadcaster[ThreadDTO]          — /v0/projects/:p/repos/:r/workspaces/:w/threads (and /:threadId)
Broadcaster[TerminalSessionDTO] — /v0/projects/:p/repos/:r/workspaces/:w/terminals
Broadcaster[GitStatusDTO]       — /v0/projects/:p/repos/:r/workspaces/:w/git/status
Broadcaster[FileEventDTO]       — /v0/projects/:p/repos/:r/workspaces/:w/files
Broadcaster[DiagnosticsDTO]     — /v0/projects/:p/repos/:r/workspaces/:w/lsp
// TODO: Broadcaster[ChatDTO] — not in scope
```

The PTY byte stream at `.../terminals/:sessionId/ws` is a raw bidirectional pipe — it is NOT a `Broadcaster[T]` and carries no typed DTO. `Broadcaster[TerminalSessionDTO]` covers only lifecycle events (session created, ended); the PTY data is handled separately.

### Namespace filtering

The broadcaster uses a namespace function to derive a routing key from the emitted DTO. Clients are matched by prefix:

```go
// WorkspaceDTO broadcaster
Namespace: func(d WorkspaceDTO) string {
    return d.ProjectID + "/" + d.RepoID + "/" + d.ID
}
```

- Client at `.../workspaces/:w` → receives events where namespace == `p/r/w`
- Client at `.../workspaces` (no wsId) → receives events where namespace has prefix `p/r/`
- Client at `.../repos/:r` → receives events where namespace has prefix `p/r`

Same broadcaster instance, same Push call — filtering happens per connected client.

### What each DTO carries

**WorkspaceDTO** — every field the client needs to render a workspace row or the IDE status bar:
```go
type WorkspaceDTO struct {
    ID              string
    RepoID          string
    ProjectID       string
    Branch          string
    ParentID        string
    ForkPointSha    string
    Status          string // "new" | "locked" | "pr-conflicts" | "deleted" | "pr-merged" | "pr-open" | "pr-closed"
    Working         bool   // true when a chat/agent session is active — TODO: always false until chat is implemented
    LastError       string // non-empty when the last async op failed; cleared on the next successful op
    Added           int
    Deleted         int
    MergeStrategy   string
    CanMergeLocally bool   // true when the parent workspace exists, is not locked, and is not deleted
    ParentBranch    string // branch name of the parent workspace (empty when CanMergeLocally is false)
    PRUrl           string
    PRTitle         string
    PRTargetBranch  string
}
```

**Status semantics:**
- `new` — workspace just created, worktree being checked out
- `locked` — workspace is protected; no mutations allowed
- `pr-conflicts` — conflicts exist, either in the local worktree (rebase/merge conflict) or on the cloud PR (provider reports unresolvable conflicts); both cases use the same status because the resolution flow is identical from the user's perspective
- `deleted` — tombstone; workspace is gone, emitted once so clients remove it from cache
- `pr-open` — PR exists and is open
- `pr-merged` — PR was merged on the provider
- `pr-closed` — PR was closed without merging

`Status` reflects what the workspace *is* — domain state only. There is no "in-progress" status for async ops (push, pull, etc.); the workspace stays at its current status until the operation completes and transitions it. `LastError` carries the failure reason when an async op fails; the client renders it inline. No separate error channel.

`CanMergeLocally` and `ParentBranch` are computed by the usecase layer from the sibling workspace list — no extra query (see §10).

Removed fields from older DTO shape:
- `PendingOp` — eliminated; no in-progress indicator at entity level
- `HasConflicts bool` — subsumed by `status: "pr-conflicts"`
- `Locked bool` — subsumed by `status: "locked"`
- `HasPR bool` — derivable from `PRUrl != ""`; PR statuses already signal existence

The sidebar branch indicator derives from the workspace list the client already holds:
```typescript
// client-side, no repo entity involved
const workspacesWithPR = workspaces.filter(w => w.prUrl !== '').length
const hasConflicts = workspaces.some(w => w.status === 'pr-conflicts')
```

**RepoDTO** — repo-level data only, no workspace aggregation:
```go
type RepoDTO struct {
    ID            string
    ProjectID     string
    Name          string
    Path          string
    DefaultBranch string
    AvatarLabel   string
    AvatarColor   string
    AvatarURL     string  // proxied /icon endpoint
}
```

**ThreadDTO** — full thread with all replies on every push (threads are small):
```go
type ThreadDTO struct {
    ID          string
    WorkspaceID string
    FilePath    string
    Line        int
    Side        string  // "old" | "new"
    Body        string
    Author      string
    Resolved    bool
    CreatedAt   time.Time
    Replies     []ThreadReplyDTO
}
```

**GitStatusDTO**, **FileEventDTO**, **DiagnosticsDTO** — unchanged from current shape, just pushed through the new scoped broadcaster.

---

## 6. Frontend Virtualization

### The model

The client owns a local cache of every entity it cares about, persisted to **IndexedDB**. This makes the cache survive page reloads and positions the app to work with a remote backend in the future without architectural change.

**Startup sequence (client's responsibility):**
1. Open WebSocket connections for the scopes needed — receiving live events immediately
2. Read cache from IndexedDB — instant, no network, renders the UI immediately
3. Seed each entity via HTTP GET — merges into cache by ID, overwrites stale IndexedDB entries
4. Done — WS keeps the cache live from this point

**After any mutation:**
- Client sends `POST/PUT/DELETE`
- `4xx` → show error inline, nothing changes
- `202` → disable the triggering action at call-site level (button disabled, etc.); entity stays at current cached state
- WS event arrives with updated DTO → merge into cache, persist to IndexedDB, re-render (sets `lastError` if failed)

**No re-fetch after mutations. No jobId correlation.**

### Cache merge rule

Every WS message is a complete DTO. Merge is always:
```typescript
cache.set(dto.id, dto)         // upsert — write through to IndexedDB
// deletion signaled by status: "deleted" — keep in cache briefly for animation,
// then remove from IndexedDB after transition completes
```

`status: "deleted"` keeps the one-type-per-channel rule intact — no separate delete frame type.

### IndexedDB schema

One object store per entity type, keyed by `id`:
```
crowbar_projects    { id, projectDTO }
crowbar_repos       { id, repoDTO }
crowbar_workspaces  { id, workspaceDTO }
crowbar_threads     { id, threadDTO }
```

On daemon version change: wipe all object stores and re-seed from GET. No migration logic.

### Subscription scopes

| UI view | WebSocket subscriptions |
|---|---|
| Project list / OOBE | `.../projects` |
| Sidebar (repo + workspace list) | `.../repos` + `.../workspaces` |
| IDE view | `.../workspaces/:w` + `.../git/status` + `.../files` + `.../lsp` |
| Review panel | `.../threads` |
| Terminal | `.../terminals/:sessionId/ws` (PTY byte stream — separate from DTO broadcasters) |

### Derived views (client-side, no backend involvement)

The sidebar branch indicator and repo-level status badges are derived from the workspace cache — never from the repo entity:
```typescript
const repoWorkspaces = workspaceCache.filter(w => w.repoId === repoId)
const hasPR = repoWorkspaces.some(w => w.prUrl !== '')
const hasConflicts = repoWorkspaces.some(w => w.status === 'pr-conflicts')
const hasWorking = repoWorkspaces.some(w => w.working)
```

---

## 7. Route Path Migration: Frontend

The frontend currently uses flat routes like:
- `GET /v0/repos` — list all repos
- `POST /v0/workspaces` — create workspace
- `GET /ws/workspaces` — workspace WS stream

These all need to migrate to the hierarchical routes.

### Sidebar startup sequence

The IDE displays one project at a time. The `projectId` is always available from the TanStack Router URL (`/ide/:projectId/:repoId/:wsId`). On mount:

1. `GET /v0/projects/:projectId/repos` — fetch all repos for this project
2. For each repo, subscribe to `WS /v0/projects/:p/repos/:r/workspaces` — live workspace events
3. For each repo, `GET /v0/projects/:p/repos/:r/workspaces` — seed workspace cache

The sidebar never needs a cross-project workspace fetch. All three IDs are already in the URL; every API call within the IDE has them available from TanStack Router params.

Store functions that currently call `/v0/workspaces` without a project scope need to receive `projectId` + `repoId` from their call site. The frontend API client (`web/src/lib/api.ts` and the store layer) needs updated URLs throughout.

---

## 8. worktreepath.go Rewrite

### Current signature
```go
func For(crowbarHome, remoteURL, workspaceID string) (string, error)
```

### New signature
```go
func For(crowbarHome, projectID, repoID, workspaceID string) string
// No error — UUID-based path construction never fails
```

### New formula
```go
func For(crowbarHome, projectID, repoID, workspaceID string) string {
    return filepath.Join(crowbarHome, "projects", projectID, repoID, "workspaces", workspaceID, "worktree")
}

func StorageDir(crowbarHome, projectID, repoID, workspaceID string) string {
    return filepath.Join(crowbarHome, "projects", projectID, repoID, "workspaces", workspaceID, "storages")
}

func RepoDir(crowbarHome, projectID, repoID string) string {
    return filepath.Join(crowbarHome, "projects", projectID, repoID)
}

func ProjectDir(crowbarHome, projectID string) string {
    return filepath.Join(crowbarHome, "projects", projectID)
}
```

---

## 9. Adapter / Container Changes

The `adapter.Container` currently opens all event stores at startup:
```go
// BEFORE (all global)
arrowES, _ := sqlite.NewEventStore(filepath.Join(eventsPath, "workspace.db"))
```

The new pattern is lazy-open per entity, using a registry with per-path mutexes (quiver's `paths.ensure()` pattern):

```go
// AFTER (per-entity lazy open, LRU-capped registry)
const maxOpenWorkspaceDBs = 64 // LRU cap; evict LRU entry when exceeded

type AdapterContainer struct {
    crowbarHome string
    mu          sync.RWMutex
    workspaceES *lru.Cache[string, *sqlite.EventStore] // key: "projectID/repoID/wsID"
    // ...
}

func (c *AdapterContainer) WorkspaceES(projectID, repoID, wsID string) (*sqlite.EventStore, error) {
    key := projectID + "/" + repoID + "/" + wsID
    c.mu.RLock()
    if es, ok := c.workspaceES.Get(key); ok {
        c.mu.RUnlock()
        return es, nil
    }
    c.mu.RUnlock()
    c.mu.Lock()
    defer c.mu.Unlock()
    dir := worktreepath.StorageDir(c.crowbarHome, projectID, repoID, wsID)
    os.MkdirAll(dir, 0755)
    es, err := sqlite.NewEventStore(filepath.Join(dir, "event_stream.db"))
    if err != nil {
        return nil, err
    }
    c.workspaceES.Add(key, es)
    return es, nil
}
```

The LRU cap (`maxOpenWorkspaceDBs = 64`) bounds the number of simultaneously open SQLite handles. When a new entry evicts the LRU, the evicted `*sqlite.EventStore` is closed. The same pattern applies to the `view.db` registry and to per-repo / per-project DB handles.

Similarly, Asynx instances are created lazily per workspace rather than globally.

---

## 10. Merge Eligibility

`CanMergeLocally` and `ParentBranch` are resolved by the **usecase layer**, not the domain entity. They are computed each time a workspace is loaded or a workspace list is returned, using only the siblings already in the result set — no additional DB query.

### Rule

A workspace `W` can merge locally when:
1. `W.ParentID` is non-empty
2. A sibling workspace `P` exists where `P.ID == W.ParentID`
3. `P.Status != "locked"` and `P.Status != "deleted"`

```go
// In the workspace usecase layer
func MergeEligibilityFor(ws domain.Workspace, siblings []domain.Workspace) (canMerge bool, parentBranch string) {
    if ws.ParentID == "" {
        return false, ""
    }
    for _, s := range siblings {
        if s.ID == ws.ParentID {
            eligible := s.Status != domain.WorkspaceStatusLocked && s.Status != domain.WorkspaceStatusDeleted
            return eligible, s.Branch
        }
    }
    return false, ""
}
```

This method is called in any usecase that returns a `WorkspaceDTO` (get, list, post-mutation push). The DTO fields `CanMergeLocally` and `ParentBranch` are always populated from this computation — never persisted to the view DB.

---

## 11. Provider Polling

The Git provider (GitHub/GitLab) is the source of truth for PR status (`pr-open`, `pr-merged`, `pr-closed`). Crowbar polls the provider to keep workspace status up to date.

### Two-tier polling

| Tier | Interval | Trigger | Scope |
|---|---|---|---|
| Global cron | Every 5 minutes | Daemon-internal scheduler | All workspaces with a PR URL |
| Per-connection | Every 1 minute | WS client connected to `.../workspaces/:wsId` | Only the workspace the client is viewing |

The 1-minute per-connection poll runs only while a client holds an active WebSocket to a specific workspace. It stops when the WS disconnects. This gives fast feedback in the IDE when watching a specific workspace.

### Flow

```
Cron / per-connection tick:
  1. Call provider API: GET PR status for workspace
  2. If status changed → Asynx command: update workspace status
  3. Broadcaster pushes updated WorkspaceDTO to all WS subscribers for that workspace
```

The provider call is the only place `Status` can transition to `pr-merged` or `pr-closed`. The global cron handles workspaces nobody is actively watching; the per-connection tick handles the workspace a developer has open.

### Implementation note

The per-connection poll is a goroutine started in the WS upgrade handler, cancelled when the WS closes. The global cron is a daemon-level service, started once at startup.

---

## 12. What Stays Unchanged

These are explicitly out of scope for this refactor:
- Domain model structs (`domain.Workspace`, `domain.Repository`, etc.)
- Asynx command/event definitions (the commands themselves, just the DB paths change)
- Git operation implementations (stage, commit, diff, etc.)
- LSP engine
- Terminal PTY engine
- Search engine
- Provider sweep logic
- Chat domain (future feature)

---

## 13. Testing Requirements

Every piece of code produced by this refactor must satisfy all three layers before it is considered done.

### Layer 1 — Unit tests (100% coverage)

- One `*_test.go` per source file, co-located (mirroring the file under `api/internal/`)
- **100% statement coverage** is the target; the CI gate is ≥95% and must not regress
- No `time.Sleep` — synchronise with event-driven helpers (`WaitForState`, `WaitForCount`, or equivalent Asynx test API)
- Benchmarks (`*_bench_test.go`) for every performance-sensitive path: path construction, broadcaster fan-out, DB registry lookup, merge eligibility scan

### Layer 2 — Blackbox integration tests

All new or changed HTTP + WebSocket contract surfaces get a `TestRegression_*` in `api/tests/` (build tag `integration`). These tests:

- Spin up the real daemon against a temp `~/.crowbar/` directory
- Call the real HTTP endpoints and assert status codes + response bodies
- For 202 endpoints, open the real WebSocket and assert the expected DTO arrives (no `time.Sleep` — block on WS message with a context deadline)
- Cover every route added or modified by this refactor:
  - `POST /v0/projects/:p/repos/:r/workspaces` → 202 + WS `WorkspaceDTO{status:"new"}` → WS `WorkspaceDTO{status:...}` on ready
  - `DELETE .../workspaces/:w` → 202 + WS `WorkspaceDTO{status:"deleted"}`
  - `POST .../workspaces/:w/git/push` → 202 + WS `WorkspaceDTO` with updated status or `lastError`
  - `GET/WS .../workspaces` namespace filtering (project-scoped, repo-scoped, ws-scoped)
  - Provider polling status transitions (`pr-open` → `pr-merged`) via mock provider
  - Merge eligibility (`CanMergeLocally` true/false based on sibling state)
  - Workspace creation: remote branch exists → checkout; does not exist → create from parent

Any bug found during the E2E acceptance test (§14) that is not already caught by Layers 1 or 2 **must** be added to Layer 2 before the fix ships. This is the regression-test-first rule.

---

## 14. E2E Acceptance Test

This is the single acceptance test that gates the refactor as production-ready. It is manual and sequential. Every step must be achieved **exclusively through the Crowbar UI — no CLI shortcuts, no direct DB writes, no backend curl**. Each failure triggers TDD: write a failing test that reproduces the bug, then fix it. After any fix, **wipe state and restart from Step 0**.

### Step 0 — Clean slate

Wipe all persisted state before each run:
- Clear Crowbar IndexedDB (all object stores: `crowbar_projects`, `crowbar_repos`, `crowbar_workspaces`, `crowbar_threads`)
- Delete `~/.crowbar/projects/` and `~/.crowbar/state/`
- Relaunch daemon

### Step 1 — OOBE: Add project

- Walk through the OOBE welcome screens
- At the "Add your first project" step, add a project named **Rabbyte** pointing to `~/Projects/Rabbyte`
- Verify the project appears and OOBE advances

### Step 2 — OOBE: Add repo

- At the "Add a repository" step, add the repo **Crowbar** from `~/Projects/Rabbyte/crowbar`
- Verify the repo appears in the sidebar under Rabbyte

### Step 3 — Verify develop + repo icon

- Open Repository Settings for Crowbar
- Verify `develop` is listed as an existing workspace (it should have been imported automatically as the default branch). If not, import it through the Repository Settings branch list.
- Verify the repo icon has defaulted to the GitHub owner avatar (fetched on import). Note the current icon.
- Upload a custom photo via the "Upload" button in Repository Settings. Verify the icon updates in the sidebar.
- Reset the icon back to GitHub avatar via "GitHub" button. Verify it reverts.

### Step 4 — Create child workspace

- From the sidebar, create a new child workspace with parent `develop`, branch name `epoch/first-pr`
- Crowbar checks the remote: `epoch/first-pr` does not exist → creates the branch locally from `develop` and sets up the worktree
- Verify the new workspace appears in the sidebar with `status: new` briefly, then transitions to the idle state
- Navigate into that workspace in the IDE (`/ide/:projectId/:repoId/:wsId`)

### Step 5 — Terminal: Claude Code

- Open the Terminal tab within the `epoch/first-pr` workspace
- Start a new terminal session
- Run `claude` (Claude Code CLI)
- Give it brief context: "This is the Crowbar IDE repository. Add a line to README.md acknowledging that Claude Code can be invoked through Crowbar's integrated terminal."
- Let Claude Code make the change and exit

### Step 6 — Editor: Manual edit

- In the Crowbar editor, open `README.md` in the `epoch/first-pr` workspace
- Manually append a second line below Claude Code's addition: a note that the edit was made directly through Crowbar's built-in editor
- Verify the file is saved (no unsaved-changes indicator)

### Step 7 — Git: Stage and commit

- Open the Git interface for the `epoch/first-pr` workspace
- Stage `README.md`
- Write a commit message: `feat: demonstrate Crowbar terminal + editor integration`
- Commit
- Verify the workspace's Added/Deleted counters reset and the commit appears in the git log

### Step 8 — Terminal: Push and open PR

- Open the Terminal tab again
- Run: `git push -u origin epoch/first-pr`
- Then: `gh pr create --title "feat: Crowbar terminal + editor demo" --base develop --body "Demonstrates Claude Code via terminal and direct editor edits through Crowbar UI"`
- Verify the PR URL appears; verify the workspace status transitions to `pr-open` in the sidebar (via the 1-minute provider poll or sooner if the WS pushes it)

### Failure protocol

When any step fails:

1. **Diagnose**: identify the exact failure — HTTP status, WS event missing, UI not updating, wrong DTO field, etc.
2. **Write test first** (Layer 1 or Layer 2 from §13): create a `TestRegression_<DescriptiveName>` in `api/tests/` (integration tag) or the appropriate `*_test.go` file that proves the bug. No `time.Sleep`. Run it — it must fail red.
3. **Fix**: make the minimal change. Run the test — it must pass green.
4. **Wipe state and restart from Step 0.**

No fix ships without a passing regression test that was red before the fix.

---

## Open Questions

> **Additional gaps surfaced during implementation planning** (Tauri WS bridge, 202-vs-id ordering, `pr-conflicts` provenance, status precedence, LRU eviction safety, icon model, terminal persistence, chat-WS removal, test seams) are resolved as decisions **D1–D14** in `2026-06-19-refactor-implementation-plan.md` §Resolved Design Decisions, grounded by `2026-06-19-refactor-grounding-maps.md`.

1. **Migration of existing data.** ✅ **Resolved**: Pre-production, no users. On daemon version change, wipe dev IndexedDB and `~/.crowbar/state/` (old layout). Apply new UUID layout from scratch. No migration logic.

2. **Terminal sessions.** ✅ **Resolved**: Terminal creation moves to `/v0/projects/:p/repos/:r/workspaces/:w/terminals`. The session ID is sufficient for the WS PTY connection. Working directory defaults to the workspace worktree path.

3. **Chats (formerly "agent runs").** ✅ **Resolved**: The agent run concept is eliminated. A chat is a multi-agent conversation — a future feature not in scope for this PR. Routes are marked TODO. `WorkspaceDTO.Working` is the boolean indicator for an active chat session.

4. **WS reconnection replay.** Implementing "replay events since timestamp" requires the event store to support time-indexed queries. Asynx event stores are currently version-indexed. Either add a `created_at` column and index, or use a sequence number that the client tracks. **Deferred**: IndexedDB cache + full GET seed on reconnect is the current recovery path.
