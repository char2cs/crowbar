# Crowbar Production-Ready Refactor — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan wave-by-wave, task-by-task. Each task ends with an independently testable deliverable and a commit.

**Goal:** Make Crowbar a production-ready IDE (no chats) by implementing the entity-scoped storage + hierarchical HTTP/WS + fail-fast/good-path-async + frontend-virtualization refactor specified in `2026-06-18-backend-api-ws-refactor.md`, verified end-to-end live in Tauri.

**Architecture:** UUID-based per-entity filesystem layout (`~/.crowbar/projects/<P>/<R>/workspaces/<W>/{worktree,storages,threads,terminals}`); lazy-open per-entity `event_stream.db` (Asynx) + `view.db` (GORM) with an LRU-capped registry; hierarchical gin route tree `/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/...`; one typed `Broadcaster[T]` per entity channel with hierarchical prefix namespace filtering; domain mutations return `202` and deliver the good result over WebSocket; the frontend holds an IndexedDB entity cache seeded by GET and kept live by WS, with a native Tauri WS bridge over the unix socket.

**Tech Stack:** Go (gin, GORM, SQLite/WAL, Asynx event-sourcing), TypeScript/React (Zustand, TanStack Router, idb/IndexedDB), Tauri v2 (Rust, WKWebView, unix-socket sidecar).

**Reference docs (read these — they are the binding detail):**
- Design spec: `docs/superpowers/plans/2026-06-18-backend-api-ws-refactor.md`
- **Grounding maps** (exact current signatures, must-change list, new contracts, risks, test targets per subsystem): `docs/superpowers/plans/2026-06-19-refactor-grounding-maps.md` — every implementer subagent MUST read the relevant subsystem section.

---

## Plan Calibration (read first)

This is a **contract-driven task graph for subagent-driven execution**, not a literal line-by-line transcription. The refactor spans ~150 tasks across 19 waves; the *binding spec* for each task is: (1) the exact **new contracts** (signatures, struct/field lists, JSON tags, route paths, test names) given in the Canonical Contracts section and the grounding-maps doc, copied verbatim; (2) the named **files** to create/modify/test; (3) the **test targets**. Implementer subagents write the actual TDD code, grounded in the named source files plus the grounding-maps section for their subsystem. Exact values (names, types, tags, paths) are authoritative and must be used verbatim; the implementer supplies the function bodies.

---

## Global Constraints

Every task implicitly includes these. A reviewer checks each one.

**Rabbyte Software Standards (Go):**
- Mirror quiver.core layering: `internal/{engine,adapter,app,api,domain,core}`; implementation hidden in `internal/` sub-packages (`commands/`, `store/`, `projections/`, `mocks/`).
- One domain concept per file; **one `_test.go` per source file** (except struct-only files); **source files < 500 LOC** (split before the limit).
- **One parameter per line, ALWAYS** — signatures AND multi-arg calls; closing paren on its own line.
- **Early returns ALWAYS**; `else` is a smell. **Max 3 indentation levels per function** (level 3 must never exist — abstract instead).
- **Coverage ≥95% (100% is the standard).** No flaky tests. **NO `time.Sleep` in tests, EVER** — synchronise with Asynx's `SendWait` (blocks until projections apply) and the kit WS waiters (`ReadUntil` with a context deadline, `WaitNRegistered`, `WaitForWorkspaceState`).
- **Benchmarks (`*_bench_test.go`)** for performance-critical algorithms: path construction, broadcaster fan-out / prefix match, DB-registry lookup, merge-eligibility scan.
- CLEAN: guard clauses, composition over nesting, `fmt.Errorf("op: ctx: %w", err)`, gofumpt + goimports. Enforced by `.golangci.yml` (funlen 100/50, gocyclo 15, nestif ≤2, revive early-return).

**Frontend Standards:**
- Component files kebab-case; exported component PascalCase. Test files mirror `web/src/` under `web/src/__tests__/`, using `@/` imports.
- Narrow Zustand selectors; `getState()` only in handlers/effects; stores never import from `components/`.
- Always use `@/components/ui/*` and CSS-variable tokens; never hardcode colors.
- No legacy/migration code (pre-production): on daemon-version change, wipe IndexedDB + `~/.crowbar` and reseed.

**Spec binding rules:**
- One WS channel = one Go type (`Broadcaster[T]`); no envelopes; errors live on the entity (`LastError`), never as a separate WS frame.
- Domain-entity mutations (project/repo/workspace/thread) → `202`, good path over WS. Git read/stage/commit, LSP, files, search → sync `200`. Slow git (push/fetch/pull/merge/rebase) → `202` + WS.
- Workspace status enum: `new | locked | pr-conflicts | deleted | pr-merged | pr-open | pr-closed`.
- Every backend bug found during execution → a `TestRegression_*` written **red first**, then fixed.

---

## Resolved Design Decisions

The grounding pass surfaced gaps the spec left open. These are decided here and are binding. (Spec `Open Questions` updated to point here.)

**D1 — Tauri WebSocket bridge (net-new, CRITICAL).** `isWebSocketCapable()` returns false under the desktop `crowbar://` unix-socket transport, so the entire §6 live-cache model is dead on desktop without a bridge. Build a native WS-over-unix-socket bridge: a Rust Tauri command (`desktop/src-tauri`) dials the daemon's unix socket, performs the HTTP→WS upgrade for a given `/v0/...` path, and streams frames to JS over a Tauri Channel; a JS shim (`web/src/lib/ws/tauri-transport.ts`) presents a `WebSocket`-like object so `wsManager` works unchanged. Reuse the existing terminal PTY Channel bridge pattern (`crowbar-bridge.ts`, memory `project_unix_socket_transport`). `isWebSocketCapable()` returns true on desktop once the bridge exists. **Wave 16.**

**D2 — 202 vs synchronous id (ordering).** Project/repo/workspace POSTs return `202` with an **empty body**; the FE learns the new entity id from the WS DTO. Correctness comes from **snapshot-on-subscribe** (the broadcaster replays current matching state to every new subscriber) plus live frames — the FE subscribes to the scope, then POSTs; whichever of {snapshot, live frame} carries the entity, the cache converges. **Terminals are the explicit exception:** `POST .../terminals` returns `201 {sessionId}` synchronously (the FE needs the id immediately to open the PTY WS) **and** broadcasts a lifecycle `TerminalSessionDTO`. This matches the spec §3 terminal note over the §4 table.

**D3 — `pr-conflicts` provenance.** For this PR, `pr-conflicts` is produced **only by the local merge/rebase path** (worktree usecase transitions `Status=pr-conflicts` on a local merge-into-parent/rebase conflict, replacing the removed `PendingMerge`). Provider-reported PR conflicts are **deferred** (the `gh/glab mergeable` field is not threaded). The status name is unchanged; provider-conflicts later reuse it. §14 does not exercise provider conflicts.

**D4 — status precedence & idle state.** Default (no PR, not protected, no conflict) status is `new` and **persists** (remove the `new→""` transition; the empty-string status is gone). Precedence, highest first: `deleted` > `locked` (protected branch) > `pr-conflicts` (local) > provider PR status (`pr-open|pr-merged|pr-closed`) > `new`. Provider poll writes a `pr-*` status only when the workspace is not `locked` and not `pr-conflicts`. `domain.Workspace.Locked` (bool) is removed; protection is `Status==locked`; all merge/cascade guards read `Status==locked`.

**D5 — LRU registry eviction safety.** Hand-roll a small generic per-entity DB registry in `adapter` (no new external dep): map keyed by id-path → handle, a doubly-linked LRU list, a per-path open mutex, and a **refcount pin**. An entity is pinned while it has a live Asynx instance / active WS subscription / in-flight mutation; eviction (cap `maxOpenEntityDBs = 64` per registry) closes only the LRU-tail entry **with refcount 0**, and lazily reopens on next access (rebuilding the Asynx + projection graph). This avoids closing a handle a live writer holds (the flagged race). For §14's handful of entities nothing is ever evicted.

**D6 — terminal session persistence: NONE.** Terminals are ephemeral; no `terminal_sessions` `view.db`. The `Broadcaster[TerminalSessionDTO]` snapshot derives from the in-memory engine registry (`ListSessionsForWorkspace`). The spec §2 `terminal_sessions` table is deferred. Simplifies Wave 10 (no per-workspace terminal storages).

**D7 — threads use Asynx + per-workspace storage** (`.../workspaces/<W>/threads/storages/{event_stream,view}.db`), reusing the existing `ReviewThread` aggregate, exposed as the first-class `/threads` endpoint + `Broadcaster[ThreadDTO]`. **Wave 9.** (Not exercised by §14 but required for production-ready.)

**D8 — repo icon model.** Add `AvatarHasIcon bool` (on-disk image present) and `AvatarEmoji string` (emoji char, empty otherwise) to `domain.Repository`; keep `RemoteURL`; drop the overloaded `AvatarURL` for path/proxy derivation. Icon precedence: emoji > on-disk image > generated label/color. On-disk image is served at `/v0/projects/:p/repos/:r/icon`; emoji renders client-side from `RepoDTO.avatarEmoji`. This resolves the §12-vs-§2 contradiction in favour of §2 (the domain struct changes).

**D9 — global state layout.** `~/.crowbar/state/event_stream.db` + `~/.crowbar/state/view.db` (view holds `terminal_profiles` + `settings`). Drop the `events/` and `store/` subdirs. `metadata.go` gains a `State()` accessor independent of the removed `Events` template, plus a `Projects` root accessor (or delegates the projects tree to `worktreepath`).

**D10 — provider polling.** Global cron interval `5m` (`poll.GlobalCronInterval`); per-connection interval `1m` (`poll.PerConnectionInterval`). Cron sweeps workspaces with `PRUrl != ""` AND `Status ∉ {pr-merged, pr-closed}` (terminal states never re-polled). Per-connection poll: single workspace, started on `/workspaces/:wsId` WS subscribe via a refcounted `ProviderPollManager` (own handle map, mirrors `WatcherManager`), cancelled on disconnect; the actual Asynx command is issued on a `context.WithoutCancel`-derived ctx so a mid-tick cancel doesn't abort a write.

**D11 — chat stays TODO; chat WS removed.** Keep `domain/chat`, the chat repo (create/fork/rename/delete/list), and the chat usecase as dormant TODO. **Remove** the chats + chatStream WS broadcasters, `ChatStatusEvent`, `ChatFrame`, `PushChat`, `chatsSnapshot`, and all `/chats` routes (no producer after agent-run removal; no chat scope in §14). `branchchat`/`branch_review` keep listing chats without the agent-running indicator.

**D12 — test seams.** Both blackbox harnesses migrate in lockstep (`api/tests/kit/env.go` and `api/tests/harness_test.go`). Add a **mock-provider injection seam** to `kit.Env` (analogous to `PushGit`/`PushLSP`) so PR-status transitions are deterministic without sleeping. Add a configurable per-connection poll interval to `realtime.Service.New` for tests.

**D13 — `WorktreePath` dropped from the wire DTO.** All file/git/terminal/lsp ops are addressed by `(projectId, repoId, wsId)`; the backend resolves the worktree path server-side. `domain.Workspace` keeps `WorktreePath`; the wire `WorkspaceDTO` omits it. A task verifies no FE code reads `workspaceDTO.worktreePath`.

---

## Canonical Contracts (binding, verbatim)

These are the single source of truth; every wave references them. Full per-file detail (including current signatures and risks) is in the grounding-maps doc.

### Filesystem & paths (`app/usecases/internal/worktreepath`)
```go
func For(crowbarHome string, projectID string, repoID string, workspaceID string) string        // .../projects/<P>/<R>/workspaces/<W>/worktree
func StorageDir(crowbarHome string, projectID string, repoID string, workspaceID string) string  // .../workspaces/<W>/storages
func ThreadsStorageDir(crowbarHome string, projectID string, repoID string, workspaceID string) string // .../workspaces/<W>/threads/storages
func RepoDir(crowbarHome string, projectID string, repoID string) string                          // .../projects/<P>/<R>
func RepoStorageDir(crowbarHome string, projectID string, repoID string) string                   // .../projects/<P>/<R>/storages
func RepoIconPath(crowbarHome string, projectID string, repoID string) string                     // .../projects/<P>/<R>/icon
func ProjectDir(crowbarHome string, projectID string) string                                      // .../projects/<P>
func ProjectStorageDir(crowbarHome string, projectID string) string                               // .../projects/<P>/storages
func GlobalStateDir(crowbarHome string) string                                                    // .../state
func DefaultCrowbarHome() (string, error)                                                          // unchanged
// repoRelPath + all URL parsing DELETED. For/StorageDir/... return NO error.
const eventStreamDBName = "event_stream.db"
const viewDBName        = "view.db"
```

### Domain
```go
// domain/workspace_status.go
const (
    WorkspaceStatusNew         WorkspaceStatus = "new"
    WorkspaceStatusLocked      WorkspaceStatus = "locked"
    WorkspaceStatusPRConflicts WorkspaceStatus = "pr-conflicts"
    WorkspaceStatusDeleted     WorkspaceStatus = "deleted"
    WorkspaceStatusPROpen      WorkspaceStatus = "pr-open"
    WorkspaceStatusPRMerged    WorkspaceStatus = "pr-merged"
    WorkspaceStatusPRClosed    WorkspaceStatus = "pr-closed"
)
// domain/workspace.go — REMOVE: Locked, HasConflicts, PendingMerge, AgentRunning. ADD: Working bool, LastError string. Keep WorktreePath (server-side only).
// domain/repository.go — ADD: AvatarHasIcon bool, AvatarEmoji string. Keep RemoteURL. Remove AvatarURL overload.
// domain/git/merge.go — REMOVE PendingMerge struct. Keep MergeStrategy.
// domain/chat_status.go — keep only ChatStatusIdle. domain/branch_chat.go — drop IsActive.
```

### DTOs (`api/v0/dto`)
```go
type WorkspaceDTO struct {
    ID string `json:"id"`; RepoID string `json:"repoId"`; ProjectID string `json:"projectId"`
    Branch string `json:"branch"`; ParentID string `json:"parentId,omitempty"`; ForkPointSha string `json:"forkPointSha,omitempty"`
    Status domain.WorkspaceStatus `json:"status,omitempty"`; Working bool `json:"working"`; LastError string `json:"lastError,omitempty"`
    Added int `json:"added"`; Deleted int `json:"deleted"`; MergeStrategy gitdomain.MergeStrategy `json:"mergeStrategy"`
    CanMergeLocally bool `json:"canMergeLocally"`; ParentBranch string `json:"parentBranch,omitempty"`
    PRUrl string `json:"prUrl,omitempty"`; PRTitle string `json:"prTitle,omitempty"`; PRTargetBranch string `json:"prTargetBranch,omitempty"`
}
func WorkspaceDTOFrom(w domain.Workspace, elig workspace.MergeEligibility) WorkspaceDTO
func WorkspaceDTOList(workspaces []domain.Workspace, elig func(domain.Workspace) workspace.MergeEligibility) []WorkspaceDTO

type RepoDTO struct {
    ID string `json:"id"`; ProjectID string `json:"projectId"`; Name string `json:"name"`; Path string `json:"path"`
    DefaultBranch string `json:"defaultBranch"`; AvatarLabel string `json:"avatarLabel"`; AvatarColor string `json:"avatarColor"`
    AvatarURL string `json:"avatarUrl,omitempty"`; AvatarEmoji string `json:"avatarEmoji,omitempty"` // AvatarURL = "/v0/projects/<p>/repos/<id>/icon" when AvatarHasIcon
}
type ProjectDTO struct {
    ID string `json:"id"`; Name string `json:"name"`; Path string `json:"path"`; Status string `json:"status,omitempty"` /* ""|"deleted" */; LastActivity time.Time `json:"lastActivity"`
}
type ThreadReplyDTO struct { ID string `json:"id"`; ThreadID string `json:"threadId"`; Body string `json:"body"`; Author string `json:"author"`; CreatedAt time.Time `json:"createdAt"` }
type ThreadDTO struct {
    ID string `json:"id"`; ProjectID string `json:"projectId"`; RepoID string `json:"repoId"`; WorkspaceID string `json:"workspaceId"`
    FilePath string `json:"filePath"`; Line int `json:"line"`; Side string `json:"side"`; Body string `json:"body"`; Author string `json:"author"`
    Resolved bool `json:"resolved"`; CreatedAt time.Time `json:"createdAt"`; Replies []ThreadReplyDTO `json:"replies"`
}
type TerminalSessionDTO struct {
    ID string `json:"id"`; ProjectID string `json:"projectId"`; RepoID string `json:"repoId"`; WorkspaceID string `json:"workspaceId"`
    ProfileID string `json:"profileId,omitempty"`; Status string `json:"status"` /* "active"|"ended" */; CreatedAt time.Time `json:"createdAt"`; EndedAt *time.Time `json:"endedAt,omitempty"`
}
```

### Merge eligibility (`app/usecases/workspace`)
```go
type MergeEligibility struct { CanMergeLocally bool; ParentBranch string }
func (u *workspaceUsecase) MergeEligibilityFor(ws domain.Workspace, siblings []domain.Workspace) MergeEligibility
// rule: ParentID!="" AND a sibling s with s.ID==ParentID AND s.Status NOT IN {locked, deleted} → {true, s.Branch}; else {}
// MUST be on the Usecase interface. Current body uses !s.Locked — change to Status-based.
```

### Adapter registry (`adapter`)
```go
const maxOpenEntityDBs = 64
func (c *Container) WorkspaceES(projectID, repoID, wsID string) (asynxModels.Store, error)
func (c *Container) WorkspaceView(projectID, repoID, wsID string) (*gorm.DB, error)
func (c *Container) RepoES(projectID, repoID string) (asynxModels.Store, error)
func (c *Container) RepoView(projectID, repoID string) (*gorm.DB, error)
func (c *Container) ProjectES(projectID string) (asynxModels.Store, error)
func (c *Container) ProjectView(projectID string) (*gorm.DB, error)
func (c *Container) ThreadES(projectID, repoID, wsID string) (asynxModels.Store, error)
func (c *Container) ThreadView(projectID, repoID, wsID string) (*gorm.DB, error)
func (c *Container) GlobalView() *gorm.DB
func (c *Container) Close() error
// AgentRunES, ChatES REMOVED. Global state holds event_stream.db + view.db.
```

### WS transport (`api/v0/ws`)
```go
func PrefixMatch(prefix string, value string) bool  // hierarchical: "p/r" matches "p/r/w"; "" matches all; "p/r" does NOT match "p/r2/w"
type StreamDef[T any] struct { Namespace func(T) string; FlatNamespace bool; Serialize func(T)([]byte,error); Filters []FilterDef[T]; Snapshot func(scope string) []T; ScopeKey func(*gin.Context) string; OnSubscribe func(scope string); OnUnsubscribe func(scope string) }
// Snapshot gains a scope arg (per-entity lazy storage must not enumerate globally).
// FlatNamespace (RATIFIED W7-1): set on git/files/lsp whose Namespace is a bare wsId leaf
// (not a p/r/w hierarchical key). BuildPredicate skips hierarchical PrefixMatch for these and
// scopes them by their wsId Filter only — otherwise the structural p/r scope prefix can never
// prefix-match a bare wsId and every git/files/lsp WS event is silently dropped.
```

### Broadcaster namespaces (`api/v0/container.go`)
```go
projects   *ws.Broadcaster[dto.ProjectDTO]          // ns: d.ID
repos      *ws.Broadcaster[dto.RepoDTO]             // ns: d.ProjectID+"/"+d.ID
workspaces *ws.Broadcaster[dto.WorkspaceDTO]        // ns: d.ProjectID+"/"+d.RepoID+"/"+d.ID
threads    *ws.Broadcaster[dto.ThreadDTO]           // ns: p/r/w/id
terminals  *ws.Broadcaster[dto.TerminalSessionDTO]  // ns: p/r/w
git, files, lsp  // unchanged types, ns: wsId
// chats, chatStream REMOVED.
```

### Hub (`app/hub`)
```go
type Subscriber interface { PushProject(dto.ProjectDTO); PushRepo(dto.RepoDTO); PushWorkspace(dto.WorkspaceDTO); PushThread(dto.ThreadDTO); PushTerminalSession(dto.TerminalSessionDTO); PushGit(wsID string, status gitdomain.GitStatus); PushFile(domain.FileChangeEvent) }
type WebSocketHub interface { BroadcastProject(dto.ProjectDTO); BroadcastRepo(dto.RepoDTO); BroadcastWorkspace(dto.WorkspaceDTO); BroadcastThread(dto.ThreadDTO); BroadcastTerminalSession(dto.TerminalSessionDTO); BroadcastGit(wsID string, status gitdomain.GitStatus); BroadcastFile(domain.FileChangeEvent) }
// PushChat / BroadcastChat REMOVED.
```

### Route tree (`/v0`, hierarchical — full target)
```
GET/WS  /v0/projects                          POST 202 /v0/projects                          (Broadcaster[ProjectDTO])
GET/WS  /v0/projects/:projectId               DELETE 202 /v0/projects/:projectId
GET/WS  /v0/projects/:projectId/repos         POST 202 /v0/projects/:projectId/repos         (Broadcaster[RepoDTO])
GET/WS  /v0/projects/:projectId/repos/:repoId DELETE 202 /v0/projects/:projectId/repos/:repoId
GET/PUT/DELETE .../repos/:repoId/icon ; PUT .../icon/emoji ; PUT .../icon/github ; GET .../branches ; GET .../protected-branches
GET/WS  /v0/projects/:projectId/repos/:repoId/workspaces        POST 202 (body {branch, parentId?})   (Broadcaster[WorkspaceDTO])
GET/WS  /v0/projects/:projectId/repos/:repoId/workspaces/:wsId  DELETE 202
POST 202 .../workspaces/:wsId/{sync,merge-into-parent,reparent}
.../workspaces/:wsId/git/status (GET/WS) ; git read GET 200 ; stage/unstage/discard/commit POST 200 ; push/fetch/pull/merge/rebase POST 202+WS
.../workspaces/:wsId/files/{content,tree} GET ; POST/PATCH/DELETE/PUT files 200 ; WS .../files/ws
.../workspaces/:wsId/lsp/* POST/GET 200 ; WS .../lsp/ws
GET/WS .../workspaces/:wsId/threads ; POST 202-sync→broadcast ; GET/PATCH .../threads/:threadId ; POST .../threads/:threadId/replies
GET/WS .../workspaces/:wsId/terminals ; POST 201 {sessionId} ; DELETE 202 .../terminals/:sessionId ; WS .../terminals/:sessionId/ws (raw PTY)
GET .../workspaces/:wsId/{review,provider} ; PATCH .../review ; POST .../search[/replace]
top-level (NOT under /projects): GET/POST/PUT/DELETE /v0/settings/terminal/profiles ; GET /v0/system/prerequisites ; GET /v0/health
REMOVED: all /v0/ws/* dedicated routes (folded into dual-serve + .../files/ws, .../lsp/ws, .../terminals/:id/ws), /v0/runs/*, /v0/chats, /v0/repos (flat), /v0/workspaces (flat).
```

### Frontend (`web/src`)
```ts
// lib/types.ts — camelCase mirrors of the Go DTOs above (WorkspaceDTO, RepoDTO, ProjectDTO, ThreadDTO, ThreadReplyDTO, TerminalSessionDTO)
// lib/api.ts — hierarchical, 202-aware:
fetchRepos(projectId): Promise<RepoDTO[]>                              // GET /v0/projects/:p/repos
fetchWorkspaces(projectId, repoId): Promise<WorkspaceDTO[]>           // GET .../workspaces
fetchWorkspace(projectId, repoId, wsId): Promise<WorkspaceDTO>
postProject(name, path, quick?): Promise<void>                        // 202
postRepo(projectId, name, path): Promise<void>                        // 202
postWorkspace(projectId, repoId, branch, parentId?): Promise<void>    // 202
deleteWorkspace(projectId, repoId, wsId): Promise<void>               // 202
reparentWorkspace(projectId, repoId, wsId, newParentId): Promise<void>// 202
// lib/persistence/entity-cache.ts — stores 'crowbar_projects'|'crowbar_repos'|'crowbar_workspaces'|'crowbar_threads', keyed by 'id'
upsertEntity<T extends {id:string}>(store, dto): Promise<void>; getAllEntities<T>(store): Promise<T[]>; removeEntity(store, id): Promise<void>
// lib/ws/tauri-transport.ts — WebSocket-shim over the Tauri unix-socket Channel (D1)
```

---

## Wave Dependency Graph

```
W1 worktreepath UUID + status constants + domain field changes          (foundation, no deps)
W2 adapter lazy per-entity DB registry + LRU + global layout            (deps W1)
W3 agent-run removal + AgentRunning→Working rename + chat WS removal     (deps none; do early to de-couple)
W4 workspace DTO + merge eligibility + repo input/commands (status-based)(deps W1, W3)
W5 repositories layer rewiring to per-entity resolver                    (deps W2, W4)
W6 hub widening + per-entity DTO broadcasters + PrefixMatch + scoped snapshots (deps W4)
W7 hierarchical route re-nesting (all endpoints)                         (deps W1, W4, W6)
W8 fail-fast/202 + good-path-async for domain mutations                  (deps W6, W7)
W9 threads endpoint + Broadcaster[ThreadDTO] (Asynx, per-ws storage)     (deps W2, W6, W7)
W10 terminal workspace-scoping + TerminalSessionDTO lifecycle topic      (deps W6, W7)
W11 provider polling: 5m cron + 1m per-conn manager                      (deps W4, W6, W7)
W12 repo/project icon-on-disk + 202 + Project/Repo broadcasters          (deps W2, W5, W6, W7)
W13 blackbox harness migration + all TestRegression_* (backend gate)     (deps W5–W12)
W14 FE types + api client hierarchical + 202-aware                       (deps backend contracts frozen ≈ after W7/W8)
W15 FE entity cache (IndexedDB) + WS entity-stream client + reseed       (deps W14)
W16 FE Tauri WS bridge over unix socket (CRITICAL desktop transport)     (deps W15)
W17 FE stores → WS-driven cache; remove optimistic/refetch               (deps W15, W16)
W18 FE UI flows (OOBE, sidebar, repo settings, terminal, editor, git)    (deps W17)
W19 §14 live E2E in Tauri + real PR (final gate)                         (deps ALL)
```

Backend waves W1–W13 land first and keep `go test ./...` green throughout. Frontend waves W14–W18 begin once backend contracts are frozen (after W8). W19 is the human-and-Tauri-in-the-loop final gate.

---

## Execution Protocol

- **Method:** subagent-driven-development. One implementer subagent per task (fresh context, given: its task entry from this plan + the relevant grounding-maps section + the Canonical Contracts). After each task, a task-reviewer subagent gates spec-compliance + quality; fix loop on Critical/Important. A broad whole-branch review precedes W19.
- **TDD every task:** write the failing test(s) named in the task's *test targets* → run red → implement minimally → run green → `gofumpt`/`goimports` (or FE lint) → commit. Commit message ends with the `Co-Authored-By: Claude Opus 4.8 (1M context)` trailer.
- **No `time.Sleep` in tests.** Go: `Asynx.SendWait` (projection-synchronous) + kit WS waiters. FE: `vi.useFakeTimers` + event-driven deferreds + `fake-indexeddb`.
- **Coverage:** keep `go test ./... -cover` ≥95% on touched packages (100% target); `*_bench_test.go` for the named perf paths. FE vitest ratchet must not regress.
- **Durable ledger:** record each completed task in the SDD progress ledger (`$(git rev-parse --git-path sdd)/progress.md`) as `Wave W Task N: complete (commits <a>..<b>, review clean)`. Trust the ledger + `git log` after any compaction.
- **Green-trunk rule:** the branch must build and pass tests at every wave boundary. The `web/dist` go:embed is satisfied by a placeholder (`make` builds the bundle for the binary; `go test ./internal/...` and the kit/harness suites do not need it).
- **Bug protocol (esp. during W13/W19):** every defect → a `TestRegression_*` written red first, then fixed; if found during the live E2E, add it to the backend blackbox suite before fixing, then restart §14 from Step 0.

### Expand-contract sequencing (mandatory for the backend domain/storage core)

Removing a still-referenced symbol mid-stream breaks the build and defeats per-task TDD. **Never remove an old field/signature/store before all consumers are migrated.** Sequence every breaking change as: (1) **expand** — add the new field/accessor/status alongside the old (additive, green); (2) **migrate** — convert every reader/writer to the new path while the old still exists (dual-state, green); (3) **contract** — delete the old field + now-dead code in a final dedicated task (green). Concretely, the order is: add status constants + `Working`/`LastError`; convert all locking/conflict logic to `Status==locked`/`pr-conflicts` while `Locked`/`HasConflicts` bools still exist; add adapter per-entity accessors alongside the old global stores; switch `app.New`/`repositories.New` to the resolver; **only then** delete `Locked`/`HasConflicts`/`PendingMerge`/`AgentRunning` and the old global stores. This keeps `go test ./internal/...` green at every task boundary.

### Milestones & green gates (orchestration granularity)

The 19 waves are the task breakdown; execution proceeds in **milestones**, each ending green and verified by the orchestrator before the next begins:
- **M1 Foundation** (W1–W5, expand-contract ordered) → `go build ./internal/... && go test ./internal/... -count=1` green.
- **M2 Transport+routes** (W6–W8) → green + `route_audit` passes.
- **M3 Feature endpoints** (W9–W12) → green.
- **M4 Backend gate** (W13) → `go test ./... -tags integration` green; coverage ≥95% touched; bench baselines set.
- **M5 FE data+cache+bridge** (W14–W16) → `cd web && npm test` green.
- **M6 FE stores+flows** (W17–W18) → vitest green + `npm run build` + `make` desktop build.
- **M7 Live E2E** (W19) → §14 passes live in Tauri + real PR.

Within a milestone, tasks run **sequentially** (shared working tree — never parallel implementers). Orchestrate each milestone with a wave-runner workflow (sequential implement→review→fix per task), then the orchestrator verifies the green gate with Bash before advancing and records the milestone in the ledger + the `ide-production-quest` memory.

---

## Wave 1 — worktreepath UUID + status constants + domain field changes

**Goal:** Land the foundational contracts everything compiles against. Keep `go test ./...` green.
**Deps:** none. Grounding: `worktreepath + worktree usecase`, `Workspace domain / status`.

### Task 1.1 — worktreepath UUID rewrite
- **Modify:** `api/internal/app/usecases/internal/worktreepath/worktreepath.go`
- **Test:** `.../worktreepath_test.go` (rewrite), `.../worktreepath_bench_test.go` (new)
- **Contracts:** `For/StorageDir/ThreadsStorageDir/RepoDir/RepoStorageDir/RepoIconPath/ProjectDir/ProjectStorageDir/GlobalStateDir` (see Canonical Contracts). Delete `repoRelPath` + `net/url`/`strings` URL parsing. `DefaultCrowbarHome` unchanged. No error returns.
- **Test targets:** `TestFor_UUIDPath` (`.../projects/<P>/<R>/workspaces/<W>/worktree`), `TestStorageDir`, `TestThreadsStorageDir`, `TestRepoDir`, `TestRepoIconPath`, `TestProjectDir`, `TestFor_Deterministic`, `TestFor_DivergesByWorkspace`; `BenchmarkFor`, `BenchmarkStorageDir`. Remove all URL-based cases.
- **Note:** sole production caller is `worktree.go:121` (updated in 1.3). This is a 2-value→1-value breakage; everything downstream in this wave fixes in lockstep.

### Task 1.2 — workspace status constants + domain field changes
- **Modify:** `api/internal/domain/workspace_status.go`, `api/internal/domain/workspace.go`, `api/internal/domain/git/merge.go`
- **Test:** `api/internal/domain/workspace_status_test.go` (new)
- **Contracts:** add `WorkspaceStatusLocked/PRConflicts/Deleted` (full 7-value enum). `domain.Workspace`: remove `Locked`, `HasConflicts`, `PendingMerge`; add `LastError string`. (`AgentRunning→Working` happens in W3 to localize the agent-run blast radius; if W3 runs first, this task only removes Locked/HasConflicts/PendingMerge + adds LastError.) Remove `PendingMerge` struct from `merge.go`.
- **Test targets:** `TestWorkspaceStatus_WireStrings` (all 7 constants == wire strings). Update `domain` tests that referenced removed fields.
- **Risk:** removing `PendingMerge` cascades to `set_pending_merge.go`/`clear_pending_merge.go` (handled in W4); removing `Locked` cascades to cascade/guards (W4). This task only changes the domain structs; dependent packages are fixed in W4 — so run 1.2 and W4's repository tasks close together, or temporarily keep the cascade compiling by reading `Status==locked`.

### Task 1.3 — worktree usecase: UUID path + branch-resolution seam + status-based guards
- **Modify:** `api/internal/app/usecases/worktree/worktree.go`, `api/internal/app/usecases/container.go` (wiring)
- **Test:** `.../worktree/worktree_test.go`, `worktree_bench_test.go`
- **Contracts:** `CreateChildInput` drops `RemoteURL`. Call `worktreepath.For(home, in.ProjectID, in.RepoID, wsID)` (no error). Add an injected `RemoteBranchExists` capability (see 1.4) and implement the §3 decision in `CreateChild`: if the branch exists on the remote → `git fetch origin <branch>` then `WorktreeAdd` checkout (`forkPoint` from the resolved ref); else → `WorktreeAddBranch` from `ParentBranch`. Replace `parent.Locked`/`newParent.Locked`/`root.Locked` guards with `Status==WorkspaceStatusLocked`; replace `handleMergeError`'s `SetPendingMerge` with a `Status=pr-conflicts` transition; `nodesFrom` derives `cascade.Node.Locked` from `Status==locked`. `removeOne` must `rm -rf` the whole `.../workspaces/<W>/` (worktree + storages + threads + terminals), passing the `.../worktree` path to `WorktreeRemove` but removing the parent dir.
- **Test targets:** `TestCreateChild_RemoteBranchExists_ChecksOut`, `TestCreateChild_RemoteBranchAbsent_CreatesLocal`, `TestCreateChild_UsesUUIDPathNotRemoteURL`, `TestCreateChild_AdoptMainWorktreeUnchanged`, `TestRemoveOne_RmRfsWorkspaceTree`, `TestMerge_LocalConflict_SetsPRConflicts`, guard tests on locked status; `BenchmarkMergeEligibilityFor` lives in W4.

### Task 1.4 — git engine: RemoteBranchExists primitive
- **Modify:** `api/internal/engine/git/git.go` (Engine interface), new `api/internal/engine/git/remote_branch_exists.go`; `app/usecases/mocks/git_ops_engine.go` (mock)
- **Test:** `api/internal/engine/git/remote_branch_exists_test.go`
- **Contracts:** `RemoteBranchExists(ctx context.Context, repoPath string, branch string) (bool, error)` → `git ls-remote --heads origin <branch>` (non-empty output ⇒ true). Add to the `Engine` interface and all mocks.
- **Test targets:** `TestRemoteBranchExists_True`/`_False` against a temp repo + bare remote (kit fixtures pattern); update the worktree usecase mock.

---

## Wave 2 — adapter lazy per-entity DB registry + LRU + global layout

**Goal:** Replace the 4 eager global stores + shared `crowbar.db` with lazy per-entity `event_stream.db`/`view.db` resolvers. **Deps:** W1. Grounding: `Storage & adapter container`.

### Task 2.1 — generic ref-counted LRU DB registry
- **Create:** `api/internal/adapter/registry.go` (generic `dbRegistry[T]` with map + LRU list + per-path mutex + refcount pin; `maxOpenEntityDBs=64`)
- **Test:** `api/internal/adapter/registry_test.go`, `registry_bench_test.go`
- **Contracts:** resolve-or-open with per-path mutex; eviction closes LRU-tail with refcount 0 only; lazy reopen. (See D5.)
- **Test targets:** `TestRegistry_LazyOpenCreatesDir`, `TestRegistry_ReturnsCachedHandle`, `TestRegistry_LRUEvictsAndClosesOldestUnpinned`, `TestRegistry_PinnedNotEvicted`, `TestRegistry_ConcurrentSamePath_OneOpen` (sync via `sync.WaitGroup`, no sleep); `BenchmarkRegistry_CacheHit`, `BenchmarkRegistry_LazyOpenMiss`.

### Task 2.2 — metadata + paths: global layout
- **Modify:** `api/internal/core/metadata/metadata.go`, `metadata.yaml`, `api/internal/core/paths/paths.go`
- **Test:** `metadata_test.go`, `paths_test.go`
- **Contracts:** `state` → `<home>/state` (own accessor, independent of the removed `events` template); global DBs at `state/event_stream.db` + `state/view.db`; add `GetProjectsPath()`/`GetProjectsPathAt`. Drop `Events()/Store()` from `paths` (keep `State()`).
- **Test targets:** `TestGetStateDirPath_IsStateDir`, `TestGetProjectsPath`, updated `paths` tests.

### Task 2.3 — adapter Container: per-entity accessors + global state + Close
- **Modify:** `api/internal/adapter/container.go`
- **Test:** `api/internal/adapter/container_test.go` (rewrite), `container_bench_test.go`
- **Contracts:** Container holds `crowbarHome`, the LRU registries (workspaceES/View, repoES/View, projectES/View, threadES/View), `globalES`/`globalView`, `lock`. Accessors per Canonical Contracts; each `MkdirAll`s its `storages/` dir and opens `event_stream.db`/`view.db` lazily. `GlobalView()` holds `terminal_profiles` + `settings`. `Close()` drains/closes ALL cached handles (fixes the crash-test SQLITE_BUSY-on-reopen hazard). Keep the global `.lock`.
- **Test targets:** `TestWorkspaceES_LazyOpenCreatesDBFile`, `TestWorkspaceES_ReturnsCachedHandleSecondCall`, `Test{Repo,Project,Thread}{ES,View}_LazyOpen`, `TestLRUEvictionClosesEvictedHandle`, `TestConcurrentWorkspaceES_NoDoubleOpen`, `TestGlobalView_HoldsProfilesAndSettings`, `TestClose_ClosesAllCachedHandles`, keep `TestRegression_StateDirSingleInstanceLock`. Remove `TestNew_BootsAllStores`.

### Task 2.4 — app asynx/gorm/container: lazy per-entity Asynx + split GORM stores
- **Modify:** `api/internal/app/asynx.go`, `api/internal/app/gorm.go`, `api/internal/app/container.go`
- **Test:** `app/gorm_test.go`, `app/container_test.go` (update)
- **Contracts:** `newAsynx[T]` invoked lazily per entity (via the adapter accessors), not 4× at startup. `GORMStores`: keep `TerminalProfiles` (+settings) global; `Projects`/`Repositories` become per-entity view resolvers (W5/W12 consume). `app.New` stops building global `axWorkspace/axChat/axAgentRun/axReviewThread` and stops passing `adapters.DB`; wires the per-entity resolver into `repositories.New`. (Agent-run/chat removal is W3.)
- **Test targets:** `TestNewGORMStores_GlobalOnlyHoldsTerminalProfiles`, updated startup wiring assertions.
- **Risk:** the single largest coupling point (`repositories.New` + `app.New`). Land 2.3/2.4/W5 close together — they compile as a unit.

---

## Wave 3 — agent-run removal + AgentRunning→Working + chat WS removal

**Goal:** Delete the agent-run concept and the dead chat-WS wiring; rename the workspace overlay to `Working`. **Deps:** none (do early). Grounding: `Agent-run removal + chat TODO boundary`.

### Task 3.1 — delete agent-run packages + routes + recovery
- **Delete:** `api/internal/domain/agent_run.go`, `agent_run_status.go`; `api/internal/api/v0/endpoints/agentrun/` (whole dir); `api/internal/app/repositories/agentrun/` (whole tree); `api/internal/app/repositories/agent_run_projection.go`; agent-run halves of `api/internal/app/repositories/recovery.go`.
- **Modify:** `api/internal/api/v0/router.go` (drop import + `agentrun.Register`), `api/internal/adapter/container.go` (drop `AgentRunES` field + `agent_run.db` from names + re-index — but note 2.3 already rewrote this file to the registry model, so here just ensure no agent-run store), `api/internal/app/container.go` (drop `axAgentRun`, `RegisterHubProjections`, `RecoverOrphans` body), `api/internal/app/repositories/container.go` (drop `AgentRun` field + param + overlay).
- **Delete tests:** `api/tests/integration/agentrun/agentrun_test.go`; remove `TestLifecycle_ChatAgentRunDrivesChatStatusOverWS`, `TestCrash_AgentRunRecoveryMarksOrphansError`, and all agent-run unit tests.
- **Test targets:** `TestRegression_RunsRoutesGone` (POST `/v0/.../runs`, GET `/v0/runs/running` → 404) — added in W13 once routes are hierarchical.

### Task 3.2 — AgentRunning→Working rename
- **Modify:** `api/internal/domain/workspace.go` (`Working bool` overlay, always false in scope), `api/internal/api/v0/dto/workspace.go` (`Working bool json:"working"` + mapping), `api/internal/app/repositories/container.go` (`broadcastWorkspace` sets `Working=false`; `ListWorkspacesWithOverlay`→`ListWorkspaces`), `api/internal/api/v0/snapshots.go` (`workspacesSnapshot`→`Workspace.List`).
- **Test:** `dto/workspace_test.go` (assert json key `working`), `repositories/container_test.go`.
- **Test targets:** `TestWorkspaceDTO_WorkingKey`, `TestBroadcastWorkspace_WorkingFalse`.

### Task 3.3 — chat-WS removal (keep chat domain as TODO)
- **Delete:** `api/internal/app/hub/chat_status_event.go`, `api/internal/api/v0/chat_frame.go`, `api/internal/app/repositories/chat/internal/commands/set_agent_running.go`, `reset_idle.go`.
- **Modify:** `api/internal/api/v0/container.go` (remove `chats`/`chatStream` broadcasters, `PushChat`, `chatsDef`/`chatStreamDef`), `api/internal/api/v0/snapshots.go` (remove `chatsSnapshot`), `api/internal/api/v0/router.go` (remove `chats.Register`), `api/internal/api/v0/endpoints/chats/` (remove from hierarchical mount — keep package dormant or delete routes wiring), `api/internal/domain/chat_status.go` (keep only `ChatStatusIdle`), `api/internal/domain/branch_chat.go` (drop `IsActive`), `app/usecases/internal/branchchat/branchchat.go` (drop agent-running read), `app/repositories/chat/chat.go` (drop `SetAgentRunning`/`ResetIdle`).
- **Test:** update `branchchat_test.go`, `chat_test.go`, hub tests.
- **Note (D11):** `domain/chat`, the chat repo CRUD, and the chat usecase stay (dormant TODO). Only the agent-running producer + chat WS surface are removed.

---

## Wave 4 — workspace DTO + merge eligibility + repo commands (status-based)

**Goal:** Finish the workspace read model: status-based locking/conflicts, eligibility-aware DTO. **Deps:** W1, W3. Grounding: `Workspace domain / status / DTO + merge eligibility`.

### Task 4.1 — merge eligibility (status-based) on the Usecase interface
- **Modify:** `api/internal/app/usecases/workspace/workspace.go`
- **Test:** `workspace_test.go`, `merge_eligibility_bench_test.go`
- **Contracts:** `MergeEligibility{CanMergeLocally bool; ParentBranch string}`; `MergeEligibilityFor(ws, siblings) MergeEligibility` on the `Usecase` interface; rule uses `s.Status != locked && s.Status != deleted` (replace the current `!s.Locked`).
- **Test targets:** `TestMergeEligibilityFor_NoParent/_ParentLocked/_ParentDeleted/_ParentIdle/_ParentMissing`; `BenchmarkMergeEligibilityFor_LargeSiblingSet`.

### Task 4.2 — eligibility-aware WorkspaceDTO converters
- **Modify:** `api/internal/api/v0/dto/workspace.go`
- **Test:** `dto/workspace_test.go` (rewrite)
- **Contracts:** new `WorkspaceDTO` field set (Canonical Contracts); `WorkspaceDTOFrom(w, elig)` + `WorkspaceDTOList(workspaces, eligFn)`. Drop `WorktreePath`, `Locked`, `HasConflicts`, `AgentRunning`, `PendingMerge` from the wire shape.
- **Test targets:** field-set + json-tag assertions; eligibility mapped into the DTO.

### Task 4.3 — repository commands: status seeding, remove Locked/HasConflicts/PendingMerge
- **Modify:** `api/internal/app/repositories/workspace/workspace.go` (`CreateInput` drops `Locked`; `SyncInput` drops `HasConflicts`; remove `SetPendingMerge`/`ClearPendingMerge`; add a set-`LastError` + set-`Status=locked` path), `internal/commands/create.go` (seed `Status` from `Protected`), `sync_working_tree_state.go` (local conflict → `Status=pr-conflicts`; remove `new→""`), `sync_provider_state.go` (Protected→`Status=locked` per D4 precedence; `prStatusToWorkspace` unchanged), delete `set_pending_merge.go`/`clear_pending_merge.go`.
- **Test:** `internal/commands/commands_test.go`
- **Test targets:** `TestCreate_SeedsLockedWhenProtected`, `TestSyncWorkingTree_LocalConflictSetsPRConflicts`, `TestSyncProvider_ProtectedSetsLocked_PrStatusOtherwise`, precedence cases (provider does not clobber locked/pr-conflicts).

---

## Wave 5 — repositories layer rewiring to per-entity resolver

**Goal:** Move the aggregate repositories from one global Asynx+DB to per-entity resolvers. **Deps:** W2, W4. Grounding: `Storage & adapter container` (risks), `repositories/container.go`.

### Task 5.1 — workspace repository resolves Asynx+view by (projectId, repoId, wsId)
- **Modify:** `api/internal/app/repositories/workspace/*` (construction + store wiring), `api/internal/app/repositories/container.go`
- **Test:** `repositories/workspace/*_test.go`, `repositories/container_test.go`
- **Contracts:** `repositories.New(db?, h, perEntityResolver)` — the workspace aggregate resolves its Asynx instance + `view.db` from the adapter registry by id; the Asynx projection/broadcast callback is (re)registered on lazy open (per D5). Drop `Chat`/`AgentRun`/`ReviewThread` global Asynx params (chat dormant via its own lazy path in W9; agent-run gone).
- **Test targets:** `TestWorkspaceRepo_ResolvesPerEntityDB`, `TestWorkspaceRepo_ReopenRebuildsProjection`, persistence-across-reopen (kit `BuildEnvAt` shared homeDir).
- **Risk:** highest-coupling task; `app.New` (2.4) + this must compile together. Keep the `WebSocketHub` broadcast callback intact (now emits `dto.WorkspaceDTO` — converted with eligibility resolved at broadcast time via a repo-scoped sibling read; see W6/W8).

## Wave 6 — hub widening + per-entity DTO broadcasters + PrefixMatch + scoped snapshots

**Goal:** One typed broadcaster per entity with hierarchical prefix filtering; hub carries DTOs. **Deps:** W4. Grounding: `WebSocket broadcaster, dual-serve, dispatch, hub fan-out, snapshots`.

### Task 6.1 — PrefixMatch + scoped Snapshot signature
- **Modify:** `api/internal/api/v0/ws/filter.go` (add `PrefixMatch`; `BuildPredicate` derives the client scope prefix from path params), `api/internal/api/v0/ws/stream_def.go` (`Snapshot func(scope string) []T`), `api/internal/api/v0/ws/broadcaster.go` (`snapshotFor` passes the client scope)
- **Test:** `ws/filter_test.go`, `ws/broadcaster_test.go`, `ws/broadcaster_bench_test.go`
- **Test targets:** `TestPrefixMatch_*` (exact `p/r/w`; parent `p/r` matches `p/r/w`; `p/r` rejects `p/r2/w`; `p` matches `p/r/w`; `""` matches all); `TestBroadcaster_PrefixNamespace_RepoScopedReceivesChildren`; `TestBroadcaster_ScopedSnapshot`; keep snapshot-before-live / no-truncation invariants (sync via `WaitNRegistered`, no sleep); `BenchmarkBroadcaster_PrefixNamespaceMatch`.

### Task 6.2 — hub Subscriber/WebSocketHub widening
- **Modify:** `api/internal/app/hub/subscriber.go`, `web_socket_hub.go`, `hub.go`; every implementer/mock (v0 `Container`, test doubles)
- **Test:** `hub_test.go`
- **Contracts:** add `PushProject/PushRepo/PushThread/PushTerminalSession` + `BroadcastProject/...`; switch `Workspace` to `dto.WorkspaceDTO`; remove `PushChat/BroadcastChat` (W3).
- **Test targets:** `TestHub_Broadcast{Project,Repo,Workspace,Thread,TerminalSession}_FansOut`; `var _ WebSocketHub` conformance.

### Task 6.3 — per-entity broadcaster defs + scoped snapshots + push conversion
- **Modify:** `api/internal/api/v0/container.go` (add `projects/repos/threads/terminals` broadcasters; `workspaces` becomes `Broadcaster[dto.WorkspaceDTO]`; namespaces per Canonical Contracts; drop query Filters in favour of prefix), `api/internal/api/v0/snapshots.go` (scoped `workspacesSnapshot(scope)` computing eligibility via `MergeEligibilityFor`; add `repoSnapshot`/`projectSnapshot`/`threadSnapshot`/`terminalSessionSnapshot`; rescope `git`/`lsp` snapshots to a single workspace)
- **Test:** `container_test.go`, `snapshots_test.go`, `container_test_hooks.go` (add `WaitN{Projects,Repos,Threads,Terminals}Registered`)
- **Contracts:** `PushWorkspace` converts `domain.Workspace`→`dto.WorkspaceDTO` with eligibility resolved from a repo-scoped sibling read (NOT inside the broadcaster hot path — compute before `Push`).
- **Test targets:** `TestWorkspacesSnapshot_ScopedToRepo`, `TestWorkspacesSnapshot_ComputesCanMergeLocally`, `TestContainer_PushRepo/PushProject/PushThread/PushTerminalSession_RouteByPrefix`. Remove chat container/snapshot tests.

---

## Wave 7 — hierarchical route re-nesting

**Goal:** Mount every endpoint under `/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/...`. **Deps:** W1, W4, W6. Grounding: `HTTP + WebSocket route table`.

### Task 7.1 — router nested group chain + param rename
- **Modify:** `api/internal/api/v0/router.go` (build `projects→:projectId→repos→:repoId→workspaces→:wsId` group chain; mount `health/system/settings` on top-level `rg`; pass `ws.DualServe` + entity broadcaster Handles to projects/repos/workspaces/threads/terminal; remove `agentrun.Register`, `chats.Register`)
- **Test:** `api/internal/api/v0/route_audit_test.go` (rewrite `specRoutes()`/`extraRoutes()` to the full hierarchical tree; update dual-serve path tests)
- **Risk (gin):** rename every `:id` → `:projectId`/`:repoId` consistently across projects/repos/provider handlers + their `c.Param()` reads, or gin panics at registration. Static segments (`/settings`, `/system`, `/health`) must stay OUTSIDE the `/projects` group.

### Task 7.2 — per-endpoint Register signatures + handler param reads (sync behavior preserved)
- **Modify:** `endpoints/{projects,repos,workspaces,git,files,editor,terminal,search,provider,review}/routes.go` + handlers — re-mount on the correct sub-group, read `c.Param("projectId"|"repoId"|"wsId")`, add dual-serve/broadcaster params where the route is `GET/WS`. **This task keeps response codes as-is** (still 200/201) — the 202 conversion is W8, isolating the route move from the async conversion.
- **Test:** each endpoint's `*_test.go` updated for path params; `route_audit` green.
- **Note:** `files/ws`, `lsp/ws`, `terminals/:sessionId/ws` become co-located WS routes (drop `/ws/*`). Git reads/stage/commit stay 200; provider/review/search stay 200.

---

## Wave 8 — fail-fast / 202 + good-path-async for domain mutations

**Goal:** Convert domain-entity mutations to validate-sync → `202` → background work → WS DTO. **Deps:** W6, W7. Grounding: `Workspace domain` + `HTTP route table` (risks).

### Task 8.1 — workspace create/delete/sync/merge/reparent → 202 + WS
- **Modify:** `endpoints/workspaces/handlers/{crud,sync}.go` + an async-runner helper
- **Test:** `crud_test.go`, `sync_test.go`
- **Contracts:** validate synchronously (input shape, repo exists, branch-name conflict, parent state) → `202` empty body → background goroutine runs the usecase → broadcast `WorkspaceDTO` (success: status transition; failure: `LastError` set). `Create` reads `repoId`/`projectId` from PATH; body `{branch, parentId?}`. `Delete` → 202 then `WorkspaceDTO{status:"deleted"}`.
- **Test targets:** `TestCreate_Returns202`, `TestCreate_ValidationFailsSync_4xx`, `TestDelete_Returns202`. (WS-outcome assertions are blackbox in W13.)

### Task 8.2 — slow git ops → 202 + WS; fast git ops stay 200
- **Modify:** `endpoints/git/handlers/write.go` + routes
- **Test:** `git handlers _test.go`
- **Contracts:** `push/fetch/pull/merge/rebase` → `202` then broadcast `WorkspaceDTO` (status update or `LastError`); `stage/unstage/discard/commit` stay sync `200`.
- **Test targets:** `TestPush_Returns202`, `TestCommit_Returns200`.

### Task 8.3 — list/detail populate merge eligibility
- **Modify:** `endpoints/workspaces/handlers/list.go`
- **Test:** `list_test.go`
- **Contracts:** `List` scoped by path params computes `MergeEligibilityFor` per row over the sibling slice; `Detail` loads same-repo siblings.
- **Test targets:** `TestList_ScopedByPathParams`, `TestList_PopulatesMergeEligibility`.

---

## Wave 9 — threads endpoint + Broadcaster[ThreadDTO]

**Goal:** First-class workspace-scoped threads (Asynx, per-ws storage). **Deps:** W2, W6, W7. Grounding: `WebSocket broadcaster` (ThreadDTO), `route table` (review→threads). (D7)

### Task 9.1 — ThreadDTO + threads endpoint extracted from review
- **Create:** `api/internal/api/v0/dto/thread.go`, `api/internal/api/v0/endpoints/threads/` (routes + handlers)
- **Modify:** `endpoints/review/routes.go` (keep `review` GET/PATCH; move threads out), `review` usecase → thread usecase scoped to the workspace; adapter `ThreadES/ThreadView` (W2) back the per-ws storage
- **Test:** `dto/thread_test.go`, threads handler tests
- **Contracts:** `ThreadDTO`/`ThreadReplyDTO` (Canonical Contracts); routes `GET/WS+POST .../threads`, `GET/PATCH .../threads/:threadId`, `POST .../threads/:threadId/replies`; sync mutation → broadcast `ThreadDTO`; `Broadcaster[ThreadDTO]` namespace `p/r/w/id`.
- **Test targets:** `TestThreadDTOFrom`, handler create/reply/resolve → broadcast.

---

## Wave 10 — terminal workspace-scoping + TerminalSessionDTO lifecycle topic

**Goal:** Sessions under the workspace; lifecycle broadcaster (PTY stays raw). **Deps:** W6, W7. Grounding: `Terminal engine`. (D2, D6)

### Task 10.1 — engine registry workspace dimension + ended callback
- **Modify:** `engine/terminal/terminal.go` (consume `workspaceID` in `Create`; add `ListSessionsForWorkspace`, `OnSessionEnded(fn)`), `engine/terminal/internal/registry/registry.go` (workspace-indexed `Add`/`ListByWorkspace`)
- **Test:** `registry_test.go`, `terminal_test.go`, `registry_bench_test.go`
- **Test targets:** `TestRegistry_ListByWorkspace_*`, `TestEngine_OnSessionEnded_FiresOnReap` (drive via `Kill`, select on a channel — no sleep); `BenchmarkListByWorkspace`.

### Task 10.2 — expanded TerminalSessionDTO + lifecycle broadcast + scoped routes
- **Modify:** `api/internal/api/v0/dto/terminal.go` (expand `TerminalSessionDTO` + `From`/`List`), `endpoints/terminal/{routes,handlers/sessions,handlers/ws,handlers/handlers}.go`, `api/internal/api/v0/container.go` (`terminals` broadcaster + `terminalsDef` snapshot from engine registry), `router.go` (pass `c.terminals.Handle` + dual-serve)
- **Test:** `dto/terminal_test.go`, terminal handler/route tests, `container_terminals_test.go`
- **Contracts (D2):** `POST .../terminals` → `201 {sessionId}` (sync) **and** broadcast `TerminalSessionDTO{status:active}`; `DELETE .../terminals/:sessionId` → `202` + `{status:ended}`; `GET/WS .../terminals` dual-served lifecycle; PTY `.../terminals/:sessionId/ws` raw. Snapshot derives from the in-memory registry (no view.db).
- **Test targets:** `TestTerminalsDef_NamespaceProjectRepoWs`, `TestCreateSession_201AndBroadcast`, `TestKillSession_202AndEndedBroadcast`, route-registration test.

---

## Wave 11 — provider polling (5m cron + 1m per-connection)

**Goal:** Two-tier polling per §11/D10. **Deps:** W4, W6, W7. Grounding: `Provider engine + polling`.

### Task 11.1 — poll intervals + cron filter
- **Modify:** `engine/provider/poll/poll.go` (`GlobalCronInterval=5m`, `PerConnectionInterval=1m`; `NewSweeper` default 5m; keep `newSweeperWithInterval` seam), `engine/provider/engine.go` (cron uses 5m; `PollOnView` unchanged), `app/container.go` (`startProviderSweep` 5m; widen `sweepTargets` to `PRUrl!="" && Status∉{pr-merged,pr-closed}`)
- **Test:** `poll_test.go`
- **Test targets:** update `TestNewSweeper_DefaultInterval`→5m; `TestSweeper_FiltersByPRUrlAndTerminalState` (no re-poll of merged/closed). Channel-notify sync, no sleep.

### Task 11.2 — ProviderPollManager + Service wiring + per-connection lifecycle hook
- **Create:** `api/internal/app/realtime/provider_poll_manager.go`
- **Modify:** `app/realtime/service.go` (`AcquireProviderPoll`/`ReleaseProviderPoll`; `New` gains `providerPoll ProviderPoller` + `perConnPollInterval` + clock; `Close`→`StopAll`), `api/internal/api/v0/container.go` (`withProviderPollLifecycle` applied to the `:wsId` workspace WS def only — `scopeWsID` no-ops on list scope)
- **Test:** `provider_poll_manager_test.go`, `service_test.go`
- **Contracts:** refcounted manager mirroring `WatcherManager`; on 0→1 for a wsId start a `PerConnectionInterval` ticker calling `PollWorkspace`; on 1→0 cancel; command issued on a `context.WithoutCancel` ctx. Inject a tiny interval in tests.
- **Test targets:** `TestProviderPollManager_Acquire_StartsPoll/_Release_StopsPoll/_Refcount/_BlankWsID_NoOp/_StopAll_Idempotent/_AcquireAfterClose_NoOp` (fake poller channel, no sleep); `TestService_Acquire/ReleaseProviderPoll`, `TestService_Close_StopsProviderPoll`.

### Task 11.3 — provider routes hierarchical + DTO + repo-path source
- **Modify:** `endpoints/provider/{routes,handlers/provider,handlers/handlers}.go`
- **Test:** `provider handlers _test.go`
- **Contracts:** `GET .../workspaces/:wsId/provider` + `GET .../repos/:repoId/protected-branches`; `State` emits `dto.ProviderStateDTOFrom`; `ProtectedBranches` uses the repo path (`RepoDir`), not a worktree path.

---

## Wave 12 — repo/project icon-on-disk + 202 + Project/Repo broadcasters

**Goal:** Entity-scoped repo/project storage, icon-on-disk, default GitHub avatar on import, 202+WS. **Deps:** W2, W5, W6, W7. Grounding: `Repo + Project domain/usecase/handlers, icon-on-disk`.

### Task 12.1 — repository domain + RepoDTO + avatar fetch helper
- **Modify:** `domain/repository.go` (`AvatarHasIcon bool`, `AvatarEmoji string`; drop `AvatarURL` overload), `api/internal/api/v0/dto/repo.go` (`AvatarURL` = `/v0/projects/<p>/repos/<id>/icon` when `AvatarHasIcon`; `AvatarEmoji` passthrough), `usecases/internal/avatar/avatar.go` (`FetchOwnerAvatarBytes(ctx, repoPath) ([]byte,string,error)`; `ScanRepoIcon` returns source path)
- **Test:** `repo_test.go` (dto), `avatar_test.go`
- **Test targets:** `TestRepoDTOFrom_ProxyURLHierarchical`, `TestRepoDTOFrom_EmojiPassthrough`, `TestFetchOwnerAvatarBytes_DegradesEmptyWhenNoGh`.

### Task 12.2 — import writes icon-on-disk + default GitHub avatar; per-entity persistence
- **Modify:** `usecases/project/project_import.go` (UUID paths; write repo icon to `RepoIconPath`; default to GitHub owner avatar bytes best-effort, `AvatarHasIcon`; persist repo→per-repo `view.db`, project→per-project `view.db`), `project_delete.go` (`rm -rf ProjectDir`, never touch the real repo `Path` / adopted-main-worktree)
- **Test:** `project_import_test.go`, `project_delete_test.go`
- **Test targets:** `TestImport_WritesRepoIconToEntityDir`, `TestImport_DefaultsToGithubAvatar`, `TestImport_AvatarFetchFailureLeavesGeneratedAvatar`, `TestDelete_RemovesProjectDirTree`, `TestDelete_NeverTouchesRealRepoPath`.

### Task 12.3 — repo/project handlers: hierarchical, icon-on-disk serve, 202+WS broadcasters
- **Modify:** `endpoints/repos/handlers/repos.go` (icon read/write via `RepoIconPath`; `PutIconGithub` downloads+stores bytes; `DeleteIcon` clears flag; add `DeleteRepo` 202; param reads), `endpoints/projects/handlers/projects.go` (`Import`/`Delete` → 202 + broadcast `ProjectDTO`; `List`/`Detail` dual-serve), routes for both
- **Test:** `repos_test.go`, `projects_test.go`
- **Test targets:** `TestIcon_ServesOnDiskBytes`, `TestPutIconGithub_StoresBytes`, `TestDeleteIcon_ResetsFlag`, `TestCreateRepo_Returns202`, `TestDeleteRepo_Returns202`, `TestImportProject_Returns202`.

---

## Wave 13 — blackbox harness migration + all TestRegression_* (backend gate)

**Goal:** Both harnesses on hierarchical routes; the full regression suite is green and is the v0 contract. **Deps:** W5–W12. Grounding: `Existing test infrastructure and blackbox harness`.

### Task 13.1 — kit.Env + harness_test.go hierarchical migration + mock-provider seam
- **Modify:** `api/tests/kit/env.go` (hierarchical `RegisterProject/RegisterRepo/CreateWorkspace/CreateChildWorkspace` — 202 + learn id from WS; `DialProjects/DialRepos/DialWorkspaces/DialWorkspace/DialThreads/DialTerminals`; `WaitForWorkspaceState`/`WaitForWorkspaceLastError`; remove `DialChats`; add a mock-provider injection seam), `api/tests/kit/oracle.go` (status-based assertions; `WorktreePath==worktreepath.For(...)`), `api/tests/kit/suite.go`, `api/tests/harness_test.go`, `api/tests/fixtures_test.go` (hierarchical `listRepos`/`firstWorkspaceForRepo`; new `workspaceDTO` shape), `container_test_hooks.go` (`WaitN{Projects,Repos,Threads,Terminals}Registered`)
- **Test:** the kit itself is exercised by the suites below.
- **Risk:** BOTH harnesses must migrate in lockstep; every `MutationID`/`{data:{id}}` read of a create response breaks (202 has no id — learn from WS, dial-before-POST).

### Task 13.2 — regression suite: the v0 contract
- **Modify/Create:** `api/tests/regressions_test.go`, `api/tests/integration/{websocket,lifecycle,crash,terminal,provider}/*`, new `api/tests/integration/threads/threads_test.go`, `api/tests/integration/storage/*`
- **Test targets (all block on WS via context deadline / Asynx `SendWait` — NO time.Sleep):**
  - `TestRegression_WorkspaceCreate_202_then_WS_new_then_ready`; `..._Delete_202_then_WS_deleted`; `..._GitPush_202_then_WS_lastError_or_status`
  - `TestRegression_Workspaces_NamespaceFiltering` (project/repo/ws prefix; `AssertNoMessage` for out-of-prefix)
  - `TestRegression_CreateWorkspace_RemoteBranchExists_Checkout` vs `_RemoteBranchAbsent_CreateFromParent`
  - `TestRegression_MergeEligibility_TrueFalse` (parent locked/deleted vs idle)
  - `TestRegression_ProviderPoll_PROpenToMerged` (mock provider, injected short interval)
  - `TestRegression_PerConnectionPoll_StartsOnSubscribe_StopsOnClose`
  - `TestRegression_RepoIcon_serve_upload_github_reset` (GET `.../icon` bytes)
  - `TestRegression_WorkspaceStoragesPersistAcrossReopen` + `_AdapterLRUReopensEvictedDB` (crash suite)
  - `TestRegression_DeleteWorkspaceRemovesStoragesDir`; `TestRegression_DeleteProjectRemovesProjectDir_KeepsRealRepo`
  - `TestRegression_TerminalSession_LifecycleBroadcast` + `_NamespaceFiltering` + `_CWDIsWorktree`
  - `TestThreads_Open/Reply/Resolve_Broadcasts`
  - `TestRegression_RunsRoutesGone` (404); `TestRegression_AllReadEndpointsUseEnvelope` (hierarchical paths, no /chats /runs)
- **Gate:** `go test ./... -tags integration` green; coverage ≥95% on touched packages; benchmarks have baselines.

## Wave 14 — FE types + api client hierarchical + 202-aware

**Goal:** Frontend speaks the new contracts. **Deps:** backend contracts frozen (after W8). Grounding: `Frontend data layer`.

### Task 14.1 — canonical TS DTOs
- **Modify:** `web/src/lib/types.ts`
- **Test:** type-level + `web/src/__tests__/lib/types.test.ts` if present
- **Contracts:** `WorkspaceDTO`, `RepoDTO` (+`avatarEmoji`), `ProjectDTO`, `ThreadDTO`, `ThreadReplyDTO`, `TerminalSessionDTO` — camelCase mirrors (Canonical Contracts).

### Task 14.2 — api client hierarchical + 202-aware
- **Modify:** `web/src/lib/api.ts`, `web/src/lib/api/workspace.ts`
- **Test:** `web/src/__tests__/lib/api/mutation-contract.test.ts`, `workspace.test.ts`
- **Contracts:** `fetchRepos/fetchWorkspaces/fetchWorkspace`; `postProject/postRepo/postWorkspace/deleteWorkspace/reparentWorkspace` return `void` (202, no body); `apiFetch` tolerates 202. Remove `fetchLandingWorkspaceId`'s flat cross-project GET — landing resolves from per-project repos/workspaces or persisted last-active route.
- **Test targets:** assert hierarchical URLs + 202-as-success (no thrown error, no entity returned).

---

## Wave 15 — FE entity cache (IndexedDB) + WS entity-stream client + reseed

**Goal:** The §6 virtualization core: IndexedDB entity cache, WS-merge, reconnect reseed. **Deps:** W14. Grounding: `Frontend data layer` (cache/idb/manager).

### Task 15.1 — entity-cache module + IndexedDB v7 stores
- **Create:** `web/src/lib/persistence/entity-cache.ts`
- **Modify:** `web/src/lib/persistence/idb.ts` + `schemas.ts` (bump to v7; create `crowbar_projects/repos/workspaces/threads` keyed by `id`; daemon-version-change wipe-and-reseed)
- **Test:** `web/src/__tests__/lib/persistence/entity-cache.test.ts`, `idb-schema.test.ts`
- **Contracts:** `upsertEntity/getAllEntities/removeEntity`. Tests use `fake-indexeddb` + `resetDB`.

### Task 15.2 — WS entity-stream client (merge, not refetch) + reconnect reseed
- **Modify:** `web/src/lib/store/loadable-slice.ts` (or a new entity-stream slice) — `applyDelta` upserts the complete DTO by id (status `deleted` → remove after animation); remove the debounced refetch; `web/src/lib/ws/manager.ts` (reconnect sentinel → full GET reseed, never a DTO merge); `web/src/lib/ws/types.ts` (drop thin envelopes; frames are complete DTOs)
- **Test:** `web/src/__tests__/lib/store/loadable-slice.test.ts`, `web/src/__tests__/lib/ws/entity-stream.test.ts` (new), `ws/manager.test.ts`
- **Contracts:** dial-before-fetch startup; hierarchical subscription URLs; reconnect sentinel routed to reseed.
- **Note:** keep git/chat stores on the legacy refetch slice (don't break them); DTO channels use the new merge path.

---

## Wave 16 — FE Tauri WS bridge over unix socket (CRITICAL)

**Goal:** Live WS on desktop. Without this the entire §6 model is dead in Tauri and §14 cannot pass. **Deps:** W15. Grounding: `Frontend data layer` risk (isWebSocketCapable false on desktop), memory `project_unix_socket_transport`. (D1)

### Task 16.1 — Rust WS-over-unix-socket command + Channel
- **Modify:** `desktop/src-tauri/src/lib.rs` (+ a new module) — a command that, given a `/v0/...` path, dials the daemon unix socket, performs the HTTP→WS upgrade, and streams frames to JS over a Tauri Channel; bidirectional send; close on drop. Update `capabilities/default.json` if a new permission is needed.
- **Test:** Rust unit test for the upgrade/frame plumbing where feasible; primarily validated live in W19.

### Task 16.2 — JS WebSocket-shim transport + wire into wsManager
- **Create:** `web/src/lib/ws/tauri-transport.ts` (a `WebSocket`-like shim backed by the Channel)
- **Modify:** `web/src/lib/ws/url.ts` (`isWebSocketCapable()` true on desktop when the bridge is present), `web/src/lib/ws/manager.ts` (use the Tauri transport on desktop, native `WebSocket` in browser)
- **Test:** `web/src/__tests__/lib/ws/tauri-transport.test.ts` (mock the Tauri Channel; assert frame in/out + open/close lifecycle)
- **Contracts:** the shim presents `onopen/onmessage/onclose/onerror/send/close` so `wsManager` is transport-agnostic.

---

## Wave 17 — FE stores → WS-driven cache; remove optimistic/refetch

**Goal:** Sidebar/projects/workspace stores become projections of the WS-fed cache. **Deps:** W15, W16. Grounding: `Frontend data layer` + `Frontend UI flows`.

### Task 17.1 — build-repo-tree + sidebar status model
- **Modify:** `web/src/lib/store/build-repo-tree.ts` (DTO §5 shape; `toSidebarStatus` passes status through; drop `agentRunning`/`locked`/`hasConflicts` overlays — `locked`/`pr-conflicts`/`deleted` are first-class statuses; add `working`/`canMergeLocally`/`parentBranch`/`prUrl`...), `web/src/lib/store/sidebar.ts` (`WorkspaceStatus` = the 7-value union, drop `agent-running`; derive repo badges from the workspace cache; WS-driven add/delete, not optimistic BFS)
- **Test:** `web/src/__tests__/lib/store/build-repo-tree.test.ts`, `sidebar.test.ts`

### Task 17.2 — workspace-list/projects stores per-project + WS-live
- **Modify:** `web/src/lib/store/workspace-list.ts` (per-`(projectId,repoId)` fetch + WS), `web/src/lib/store/projects.ts` (project WS stream; drop double-refetch), `web/src/lib/store/workspace-route-guard.ts` (gate on per-repo seed), `web/src/components/app-sync-provider.tsx` (§7 startup: GET projects → per repo: WS subscribe + GET seed)
- **Test:** `workspace-list.test.ts`, `projects.test.ts`, `workspace-route-guard.test.ts`

### Task 17.3 — workspace-tree-context: 202 + WS-driven, threaded ids
- **Modify:** `web/src/components/layout/workspace-tree-context.tsx` (thread `projectId`+`repoId`; 202 create/delete/reparent; no optimistic node — rely on WS `status:new`→ready and `status:deleted` tombstone; render `lastError` inline), `web/src/lib/api/workspace.ts`
- **Test:** `web/src/__tests__/components/layout/workspace-tree-context.test.tsx`

---

## Wave 18 — FE UI flows (OOBE, sidebar, repo settings, terminal, editor, git)

**Goal:** Every §14 user-facing flow talks to the hierarchical API/WS and the live cache. **Deps:** W17. Grounding: `Frontend UI flows for §14 E2E`.

### Task 18.1 — OOBE + project/repo modals (subscribe-before-POST)
- **Modify:** `web/src/components/oobe/oobe-screen.tsx`, `components/projects/import-project-modal.tsx` (no `fetchProject(id)` refetch — resolve from `/v0/projects` WS), `components/projects/add-repository-modal.tsx` (postRepo only; resolve repo/ws ids from WS; navigate after cache), `components/workspace/new-workspace-page.tsx` (source repos from cache; navigate after WS DTO)
- **Test:** `__tests__/components/projects/{import-project-modal,add-repository-modal}.test.tsx`, `oobe` test

### Task 18.2 — repo-settings-panel hierarchical (branches + icon)
- **Modify:** `web/src/components/layout/repo-settings-panel.tsx` (accept `projectId` prop; `GET .../repos/:r/branches`; branch import `POST .../workspaces` 202+WS; icon PUT/DELETE/emoji/github under `.../repos/:r/icon`)
- **Test:** `__tests__/components/layout/repo-settings-panel.test.tsx`

### Task 18.3 — workspace-scoped consumers: files/git/lsp/terminal/editor
- **Modify:** `web/src/features/files/lib/file-tree-api.ts`, `features/file-system/controllers/platform.ts`, `features/git/api/*.ts` + `git-store.ts` + `git-commit-panel.tsx`, `features/workspace/stores/hooks/use-workspace-effects.ts`, `lib/crowbar-bridge.ts` (terminal create/PTY hierarchical), `features/terminal/components/terminal.tsx`, `features/editor/components/editor-surface.tsx`, `components/layout/ide-shell.tsx` (drop synthetic `/repos/<id>` root; use route params), `routes/_shell/index.tsx` (landing without flat cross-project GET)
- **Test:** `__tests__/features/{file-system/platform,git/git-status-api,git/git-commits-api}.test.ts`, `__tests__/lib/crowbar-bridge.test.ts`, `__tests__/lib/ws/contract-paths.test.ts`
- **Contracts:** every workspace-scoped URL/WS threads `projectId`+`repoId`+`wsId` from TanStack route params; git reads/stage/commit stay 200; push/pull 202+WS; remove any `workspaceDTO.worktreePath` read (D13).

---

## Wave 19 — §14 live E2E in Tauri + real PR (final gate)

**Goal:** Prove Crowbar is IDE-ready by driving the spec §14 acceptance test end-to-end in the running Tauri app, with the real `Rabbyte/crowbar` PR. **Deps:** ALL. This wave is performed by the orchestrator (not a fresh implementer), using the Tauri MCP for live interaction and verification.

### Pre-flight
- Whole-branch code review (most-capable model) over `git merge-base enhancement/ide-final-polish HEAD..HEAD`; fix Critical/Important.
- `go test ./... -tags integration` green; `cd web && npm test` green; `npm run build` (web bundle) succeeds; `make` desktop build succeeds.
- Wipe dev state: clear IndexedDB + `rm -rf ~/.crowbar/projects ~/.crowbar/state`. Start a fresh `make dev-desktop` (close any existing one).

### The walkthrough (spec §14, UI-only, no cheats; via Tauri MCP)
1. OOBE → add project **Rabbyte** (`~/Projects/Rabbyte`).
2. OOBE → add repo **Crowbar** (`~/Projects/Rabbyte/crowbar`).
3. Verify `develop` auto-imported (else import via Repository Settings); verify default GitHub owner avatar; upload a custom photo (verify live), reset to GitHub.
4. Create child workspace off `develop`, branch `epoch/first-pr` (remote-absent → create-from-parent); navigate into the IDE.
5. Terminal tab → run `claude`; instruct it to add a README line acknowledging Claude Code via Crowbar's terminal; let it edit + exit.
6. Editor → open `README.md`, append a second line noting the edit was made via Crowbar's editor; verify saved.
7. Git panel → stage `README.md`, commit `feat: demonstrate Crowbar terminal + editor integration`; verify counters reset + commit in log.
8. Terminal → `git push -u origin epoch/first-pr` then `gh pr create --base develop ...`; verify the PR URL and the workspace status transitions to `pr-open`.

### Failure protocol (every defect)
1. Diagnose (HTTP/WS/UI/DTO). 2. Write a `TestRegression_*` (backend) or vitest (frontend) **red first**, no `time.Sleep`. 3. Fix minimally → green. 4. Wipe state, restart §14 from Step 0. No fix ships without a red-first regression test.

### Definition of done
All 8 steps pass live in Tauri; the real PR exists on `Rabbyte/crowbar`; full automated suite green; live verification captured (Tauri screenshots). Then open the PR `refactor/entity-scoped-api-ws` → `enhancement/ide-final-polish` and report back.

---

## Storage Core Design (binding — supersedes W2.3/W2.4/W5.1 detail)

The grounding proved the per-entity storage cannot be made purely additive (the new `WorkspaceES(p,r,w)` *method* collides with the existing `WorkspaceES` *field*; and ID-only methods like `Get(ctx,id)` can't resolve a per-entity path). This is one coherent migration, green only at its end. **Scope discipline (minimize blast radius):** ONLY the **workspace** aggregate becomes per-entity event-sourced now. chat + reviewthread keep their **global** event stores (`state/event_stream.db`-area). projects, repositories, terminal_profiles, settings stay on the **global** `state/view.db` (projects/repos migrate to per-entity view.db in W12).

### Adapter Container (rewrite, `api/internal/adapter/container.go`)
Fields: `crowbarHome string`; `chatES, reviewThreadES asynxModels.Store` (global, unchanged files); `workspaceES *Registry[asynxModels.Store]` + `workspaceView *Registry[*gorm.DB]` (per-entity, keyed `"p/r/w"`); `globalView *gorm.DB` (= `GlobalStateDir/view.db`); `lock *instanceLock`. Drop the old `WorkspaceES` field and the shared `crowbar.db`.
Accessors: `WorkspaceES(p,r,w) (asynxModels.Store, error)` (openFn: `os.MkdirAll(worktreepath.StorageDir(home,p,r,w),0o750)` then `eventsqlite.NewEventStore(dir/event_stream.db)`); `WorkspaceView(p,r,w) (*gorm.DB, error)` (openFn: `storesqlite.OpenDB(dir/view.db)`); `GlobalView() *gorm.DB`; `ChatES()`/`ReviewThreadES()` (or keep as exported fields named `ChatES`/`ReviewThreadES` — no method collision). `Close()` closes both registries (`CloseAll`) + globalView + chat/review stores + lock. Global state files live under `GlobalStateDir(home)` = `<home>/state`.

### Location index (`state/view.db`)
`type workspaceLocation struct { ID string gorm:"primaryKey"; ProjectID string; RepoID string }` table `workspace_locations`. Lives in `globalView`. Written on Create, read to resolve `id→(p,r)`, removed on Delete. Keep a tiny store for it (own file `internal/store` of the workspace repo, or a thin gorm helper).

### Workspace repository (rewrite internals; the `Workspace` **interface is UNCHANGED**)
Construct with `New(adapters *adapter.Container, broadcast store.BroadcastFunc, asynxFactory func(asynxModels.Store)(asynx.Asynx[domain.Workspace],error), locations <locationStore>)` (the `asynxFactory` is injected by `app` to avoid an import cycle on `newAsynx`). Hold a per-entity `*Registry[*wsEntity]` where `wsEntity{ ax asynx.Asynx[domain.Workspace]; store store.Store }`.
- `entityFor(ctx,id)`: `loc := locations.Get(id)` → `es := adapters.WorkspaceES(loc.P,loc.R,id)`; `view := adapters.WorkspaceView(loc.P,loc.R,id)`; `ax := asynxFactory(es)`; `st := store.New(view, ax, broadcast)`; cache in the registry.
- `Create(in)`: `locations.Save({in.ID,in.ProjectID,in.RepoID})` FIRST, then `entityFor(in.ID).ax.SendWait(CreateWorkspace{...})`.
- `Get/Sync*/SetMergeStrategy/Touch/Reparent/UpdateForkPoint/SetParentFromPR/Delete`: `entityFor(id)` → delegate (`Delete` also evicts the entity + removes the index row).
- `List()`: read all `workspace_locations` rows → for each, `entityFor(row.ID).ax.Get(row.ID)` → append (or read its view row). Repo-scoped variants filter by `RepoID` in the index.

### app / repositories rewiring
`app.New`: build `gormStores` from `adapters.GlobalView()` (was `adapters.DB`); build chat/reviewthread global Asynx from `adapters.ChatES()/ReviewThreadES()`; pass `adapters` + a `newAsynx[domain.Workspace]` factory into `repositories.New`. `repositories.New(adapters, h, axChat, axReviewThread, asynxFactory)`: workspace repo built per the above; chat/reviewthread unchanged.

### Tests
Adapter: `TestWorkspaceES_LazyOpenCreatesEventStreamDB`, `TestWorkspaceView_LazyOpenCreatesViewDB`, `TestWorkspaceES_CachedSecondCall`, `TestClose_ClosesAllAndLock`, keep `TestRegression_StateDirSingleInstanceLock`. Workspace repo: `TestCreate_WritesLocationIndexAndPerEntityStores`, `TestGet_ResolvesViaIndex`, `TestList_AcrossEntities`, `TestDelete_RemovesIndexRow`, persistence-across-reopen (`BuildEnvAt` shared homeDir). Synchronize via Asynx `SendWait`, NO time.Sleep. The on-disk assertions (`event_stream.db`/`view.db` exist at the entity path) prove §2.

---

## Appendix — Self-Review (plan vs spec)

- **Spec coverage:** §1 filesystem→W1/W2/W12; §2 storage→W2/W5/W9; §3 routes→W7; §4 fail-fast/202→W8 (+12); §5 broadcasters/DTOs→W6 (+4); §6 FE virtualization→W15/W17; §7 FE routes→W14/W18; §8 worktreepath→W1; §9 adapter/LRU→W2; §10 merge eligibility→W4; §11 provider polling→W11; §12 removals→W3; §13 testing→every task + W13; §14 E2E→W19. All covered.
- **Decisions resolving spec gaps:** D1–D14 above; spec `Open Questions` to be annotated with a pointer to this section.
- **Type consistency:** all signatures/DTOs/routes are taken verbatim from one Canonical Contracts section; waves reference it rather than re-deriving.



