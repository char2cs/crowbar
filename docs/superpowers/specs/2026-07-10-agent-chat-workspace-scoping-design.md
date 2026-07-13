# Agentic-Chat Workspace Scoping + Hard Delete + On-Disk Layout — Design

**Date:** 2026-07-10
**Branch:** `frontend/chats`
**Status:** Approved design, pre-implementation
**Scope:** Backend only. The conversation-list frontend is the eventual objective of this branch but is **deferred until this backend piece lands** (per user direction).

## 1. Context

The agentic backend is a `char2cs/asynx` event-sourced `AgentChat` aggregate. Today its REST + WebSocket surface is **flat and global** (`/v0/agent/chats…`) — deliberately, because a chat carries its own `workspaceId`. There is no FE consuming it yet.

An `AgentChat` is not a global entity: it belongs to a workspace. This work makes the whole backend reflect that — the API, the live feed, and the on-disk artifacts all become workspace-scoped — and adds the missing hard-delete route. It also consolidates the agentic on-disk state (hook config, native transcripts, and the handoff **ledger**) into a single human-readable per-workspace directory that sits beside the git worktree.

### Key mechanism the design must preserve: provider switching via the ledger

Provider switch (Claude↔Codex) works through an on-disk, per-chat **ledger** (`api/internal/app/ledger/ledger.go`), which Crowbar builds itself from vendor-CLI hooks (it never reads a file the vendor wrote):

1. Each turn, the active CLI fires hooks → `crowbar hook user_prompt|turn_stop …` → `POST /agent/hooks` → `usecase.appendTurn` writes one `<seq>-<ts>-<role>-<provider>.turn` JSON file into the chat's ledger dir. The ledger is **per-chat, append-only, provider-tagged**, and spans every segment.
2. On switch, `SwitchProvider` grace-terminates the outgoing CLI, then `AssembleHandoff` → `ledger.RenderConversation()` concatenates all turns into a plain-text blob, wraps it in the configured `HandoffWrapper` prompt, and `spawnSegment` injects it into the new provider (Claude `--append-system-prompt`, Codex positional arg; a switch-*back* also `--resume`s the native session). The new provider picks up the full prior conversation; its turns append to the same ledger.

Two distinct "transcript" concepts exist and must not be conflated:
- **Crowbar ledger** — per-chat, provider-neutral, built from hooks. **This is the handoff source.**
- **Vendor-native transcripts** — per-segment, written by the CLI (Claude `.jsonl`, Codex rollout). Crowbar never reads them for handoff.

## 2. Goals / Non-Goals

**Goals**
- Every agent REST + WS route is nested under `/v0/projects/:p/repos/:r/workspaces/:wsId/agent/…`.
- The agent's own CLI callbacks (hook / rename / handoff) are workspace-scoped, carrying scope as **explicit command arguments** (never environment variables).
- A user can hard-delete a chat (asynx `Forget`), and every scoped client sees it disappear live.
- The WS feed is delivered per-workspace (a client subscribed to workspace W receives only W's frames).
- All agentic on-disk state consolidates under one human-readable per-workspace root, beside the worktree.

**Non-Goals**
- The conversation-list frontend (separate, later work on this branch).
- Physical per-workspace **storage partitioning** of the aggregate. The `AgentChat` aggregate stays a single global event store keyed by a `workspaceId` field (same posture as review threads). "Scoped" here means routing + access + delivery + on-disk artifacts, not the aggregate's physical DB.
- Moving `StorageDir` / `ThreadsStorageDir` (they stay on the UUID path family; out of scope).
- Data migration. This is pre-production: existing dev state is wiped, not migrated.

## 3. Design

### 3.1 Uniform route nesting

Move `agent.Register` off the top-level `rg` and onto the workspace-scoped group (mirroring `terminal` on `wsScoped` / `threads` on `repoScoped` in `api/internal/api/v0/router.go`). All routes nest:

```
GET    /v0/projects/:p/repos/:r/workspaces/:wsId/agent/chats
POST   /v0/projects/:p/repos/:r/workspaces/:wsId/agent/chats
GET    …/agent/chats/:id
POST   …/agent/chats/:id/switch
POST   …/agent/chats/:id/rename           (?source=agent applies the agent-precedence rule)
GET    …/agent/chats/:id/handoff
DELETE …/agent/chats/:id                   ← new (§3.3)
POST   …/agent/hooks
GET(WS) …/agent/ws/chats
```

Nesting inherits the existing `scopeWorkspaceToPath` middleware (`wsId ⊂ repo ⊂ project`). Rename stops being a special case — agent and user both hit the nested path; the agent just adds `?source=agent`.

**By-id scope check (hardening).** By-id handlers assert `chat.WorkspaceID == :wsId` → 404 on mismatch, so the path scope is real defense (catches a workspace-delete race or a malformed caller), not decorative.

**List / Create.**
- `List` → new `usecase.ListChatsByWorkspace(wsID)` backed by the existing repo `ListByWorkspace(ctx, wsID)`; `wsId` from the path.
- `Create` → `wsId` from the path (dropped from the body); `provider` stays in the body.

### 3.2 CLI callbacks scoped via explicit command args

The three agent-invoked `crowbar` subcommands (`hook`, `chat rename`, `handoff dump`) call back over the `unix://` IPC socket and today hit flat, chatID-addressed URLs. They gain scope as **command flags**, resolved at spawn:

- Extend `engine/agent.TemplateCtx` with `ProjectID`, `RepoID`, `WorkspaceID` (already returned together by `WorkspaceReader.WorktreeDir(wsID) → (crowbarHome, projectID, repoID, worktree)`; no new lookup).
- The descriptor hook-command templates (`descriptors/claude.yaml`, `descriptors/codex.yaml`) add `--project {project_id} --repo {repo_id} --workspace {workspace_id}`. The system-prompt-injected `crowbar chat rename` instruction and the switch-time `crowbar handoff dump` invocation gain the same flags.
- `api/cmd/crowbar/{hook,chat,handoff}.go` gain a shared `scopedAgentPath()` helper that builds `/v0/projects/<p>/repos/<r>/workspaces/<w>/agent/…` from the flags.

No environment variables are introduced; scope travels explicitly with each call. Repo-home chats are structurally identical — the repo-home workspace has a designated `wsId` and resolves through `WorktreeDir` like any other workspace.

### 3.3 Hard delete via asynx `Forget`

New `DELETE …/agent/chats/:id` → a usecase method that:
1. best-effort terminates the active segment's PTY (reusing the existing `DeleteChat` teardown logic; `ErrSessionNotFound` swallowed, other failures logged, delete proceeds), then
2. calls the repo's `Forget(ctx, id)` → `ax.Forget` (asynx hard delete: tombstone + purge).

**Broadcast.** `ax.Forget` notifies `OnForget` handlers on topic `asynx.aggregate.forget` — which the `agentchat.*`-subscribed hub projection does not see today, so a hard delete would purge silently. The store projection already registers `OnForget` (drops the read row). **Add an `OnForget` on the hub projection** that broadcasts a scoped `deleted` frame, reading `evt.Aggregate.WorkspaceID` (populated on the forget event, as the store projection's existing `onForget` reads `evt.Aggregate.ID`). This keeps the "one projection = one source of frames" invariant (no manual usecase broadcast).

The workspace-delete cascade already hard-`Forget`s each chat (`forgetAgentChats`), so it is unaffected. The soft `Delete` command / `DeleteChat` usecase thereby loses its only caller (the FE-facing single-delete is now `Forget`); the implementation plan decides whether to retire it or keep it dormant for a future soft-delete need.

### 3.4 WS feed scoped per workspace

- Add `WorkspaceID` to `dto.AgentChatEvent` (currently `{chatId, kind}`).
- Thread `workspaceID` through the push chain: the store hub projection's `BroadcastFunc` reads `evt.Aggregate.WorkspaceID`; `hub.Subscriber.PushAgentChat(chatID, workspaceID, kind)`; `container.PushAgentChat` emits `AgentChatEvent{ChatID, WorkspaceID, Kind}`.
- Redefine `agentChatDef` (`api/internal/api/v0/container.go`) like `gitDef`/`filesDef`: `Namespace = e.WorkspaceID`, `FlatNamespace: true`, `Filters: [{Param: "wsId", Extract: e.WorkspaceID, Match: ExactMatch}]`.
- Mount the WS route under the workspace group so the `:wsId` path param resolves the filter.

### 3.5 On-disk layout consolidation

Today two path families exist (`worktreepath.go`): the **human-readable** worktree (`projects/<project>/<slug>/<branch>`, `slug` = `host/owner/repo`) and the **UUID** family (`projects/<projectId>/<repoId>/workspaces/<wsId>/…`) used by the ledger/storages.

Consolidate the agentic state into the human-readable family under a single workspace root:

```
.crowbar/projects/<projectId>/<repoUrl=slug>/<branchName>/     ← workspace root
├── worktree/                                                  ← git checkout (moved down one level)
└── chats/<chatId>/
    ├── ledger/                                                ← per-chat handoff transcript
    │   └── 0000000N-<ts>-<role>-<provider>.turn
    └── <segmentId>-<provider>/                                ← {tmp}: hook config + vendor-native transcript
        ├── settings.json + claude-home/     (Claude)
        └── codex-home/                        (Codex)
```

**From → to:**
| Artifact | Today | After |
|---|---|---|
| Worktree | `projects/<project>/<slug>/<branch>` | `…/<branch>/worktree` |
| Repo-home worktree | `projects/<project>/<slug>/.home` | `…/.home/worktree` |
| Ledger | `projects/<project>/<repoId>/workspaces/<wsId>/agent-ledger/<chatId>` | `…/<branch>/chats/<chatId>/ledger` |
| Segment tmp (`{tmp}`) | `<crowbarHome>/agent-tmp/<segId>` (global) | `…/<branch>/chats/<chatId>/<segmentId>-<provider>` |

**`worktreepath.go` refactor.** Introduce `WorkspaceRootDir(home, project, slug, branch)` = `projects/<project>/<slug>/<branch>`; then `WorktreeDir = WorkspaceRootDir + "/worktree"`, `ChatsDir = WorkspaceRootDir + "/chats"`, `AgentLedgerDir = ChatsDir + "/<chatId>/ledger"`, `SegmentDir = ChatsDir + "/<chatId>/<segmentId>-<provider>"`. `Derive`/`HomeLeaf` return the `/worktree` leaf.

**Decided defaults:**
- Old segment dirs are **kept** on segment-end (reaped when the chat/workspace is deleted) — cheap and debuggable.
- Segment dir names carry a `-<provider>` suffix for greppability.

**Consequences.**
- Spawn `Cwd` → `…/worktree`; `git worktree add` target → `…/worktree`.
- Delete cascade `rm -rf`s the **workspace root** (kills `worktree/` + `chats/` together). The global `agent-tmp` sweep (`sweepStaleAgentTmp`) becomes dead code and is removed.
- Because `chats/` sits *beside* the worktree (not inside it), hook config and transcripts never appear in `git status` — no `.gitignore` / `.git/info/exclude` handling needed.

### 3.6 Claude native transcript redirect

Codex already writes its native transcript inside the segment (via `CODEX_HOME = {tmp}/codex-home`). Claude currently leaks its transcript to global `~/.claude/projects/…`. Add a `set_env` step to `claude.yaml`: `CLAUDE_CONFIG_DIR = {tmp}/claude-home`, mirroring Codex. Both CLIs' native transcripts then land inside the segment dir and die with the workspace.

**Risk (must validate live):** that Claude's transcripts actually follow `CLAUDE_CONFIG_DIR`. Asserted via a real-CLI integration test that checks a `.jsonl` appears under `claude-home/` — not claimed working until observed.

## 4. Files touched (accounting)

- **Routing/handlers:** `api/internal/api/v0/router.go`, `endpoints/agent/routes.go`, `endpoints/agent/handlers/{chats,switch,handoff,hooks}.go` (+ new delete handler), `container.go` (`PushAgentChat`, `agentChatDef`).
- **DTO/hub:** `dto/agent.go` (`AgentChatEvent.WorkspaceID`), `app/hub/{hub,subscriber}.go`, `agentchat/internal/store/hub.go` (`BroadcastFunc` + hub `OnForget`).
- **Usecase:** `usecases/agent/agent.go` (`ListChatsByWorkspace`, hard-delete method, `TemplateCtx` scope fields, spawn `Cwd`/tmp paths).
- **Paths:** `usecases/internal/worktreepath/worktreepath.go` (+ consumers: `app/container.go`, `usecases/container.go`, `usecases/worktree/worktree.go`, `usecases/project/project_import.go`, `app/container.go` `sweepStaleAgentTmp` removal).
- **CLI:** `api/cmd/crowbar/{hook,chat,handoff}.go` (+ shared `scopedAgentPath`).
- **Descriptors:** `engine/agent/descriptors/{claude,codex}.yaml` (scope flags; Claude `CLAUDE_CONFIG_DIR`).

## 5. Testing

- Rewrite `/v0/agent/*` integration tests onto the nested paths.
- New cases: by-id scope-mismatch → 404; hard-delete purges + broadcasts a **scoped** `deleted` frame; WS workspace isolation (W's client never sees V's frames); `ListByWorkspace` filtering.
- On-disk: ledger + segment dirs land under `…/<branch>/chats/<chatId>/…`; worktree under `…/<branch>/worktree`; workspace-delete `rm -rf` removes both.
- Real-CLI integration: the three scoped callbacks resolve against the nested URLs; a provider switch still hands off the ledger; Claude transcript appears under `claude-home/`.

## 6. Risks / open items

- **Claude `CLAUDE_CONFIG_DIR` transcript redirect** — unverified; gated by a live integration assertion (§3.6).
- **Path-consumer sweep** — the `worktreepath` refactor touches several usecases; the implementation plan enumerates and updates each call site.
- **In-flight PTYs across daemon upgrade** — agents spawned with old flat callback URLs would 404 after upgrade; acceptable (boot reconcile ends segments; pre-prod, no migration).

## 7. Follow-up (out of scope)

- Conversation-list frontend (the branch's eventual objective).
- Aligning `StorageDir` / `ThreadsStorageDir` to the human-readable family (consistency, not required here).
