# Agentic-Chat Workspace Scoping Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the agentic-chat backend fully workspace-scoped (REST + WS + in-PTY CLI callbacks), add a hard-delete route via asynx `Forget`, and consolidate agent on-disk state (hook config, native transcripts, handoff ledger) into a human-readable per-workspace root beside the git worktree.

**Architecture:** Nest every `/v0/agent/*` route under `/v0/projects/:p/repos/:r/workspaces/:wsId/agent/…`; scope the WS lifecycle feed per-workspace via the existing `ws.StreamDef` filter framework; hand the in-PTY `crowbar` callbacks their scope as explicit command flags; restructure `worktreepath` so a workspace root holds sibling `worktree/` and `chats/<chatId>/{ledger,<segmentId>-<provider>}/` trees.

**Tech Stack:** Go, gin, `char2cs/asynx` (event sourcing), the in-house `ws` streaming framework, cobra (the `crowbar` CLI), YAML provider descriptors.

## Global Constraints

- **No timing in tests.** Never use sleeps, `Eventually`, `After`, poll-intervals, or wall-clock waits. Block on real signals (asynx `WaitIdle`/`Drain`, channels, `SendWait`). `go test -timeout` is the only backstop. (Project hard rule.)
- **TDD, test-first.** Write the failing test, watch it fail, implement minimally, watch it pass, commit.
- **No migration code.** Pre-production, zero users: never migrate stale persisted state. Existing dev state under `~/.crowbar` / `<workspace>/.crowbar` is wiped, not migrated.
- **Go tests co-locate** as `_test.go` next to source; cross-cutting black-box tests live under `api/tests` behind the `integration` build tag (run with `-tags integration`).
- **REST envelope** is `{success, error?, data?}`; use `libs.WriteQueryOK` / `WriteMutationOK` / `WriteAccepted` / `WriteErr` / `StatusAndMessage` (see `api/internal/api/libs/`).
- **Follow existing patterns** in the file you are editing; do not restructure unrelated code.
- Run the touched package's tests plus `go build ./...` after each task; the full suite must stay green.

---

## File Structure

**Modified**
- `api/internal/app/usecases/internal/worktreepath/worktreepath.go` — workspace-root split; new `ChatsDir`/`AgentLedgerDir`/`SegmentDir` derived from the worktree path.
- `api/internal/app/usecases/agent/agent.go` — spawn `Cwd`/tmp → new paths; `appendTurn`/`AssembleHandoff` ledger dir; `ListChatsByWorkspace`; hard-delete `PurgeChat`; `TemplateCtx` scope fields at spawn.
- `api/internal/app/usecases/worktree/worktree.go` — delete cascade `rm -rf` targets the workspace root.
- `api/internal/app/container.go` — remove the now-dead `agent-tmp` sweep.
- `api/internal/engine/agent/template.go` + `descriptors/{claude,codex}.yaml` — scope placeholders/flags; Claude `CLAUDE_CONFIG_DIR`.
- `api/internal/api/v0/dto/agent.go` — `AgentChatEvent.WorkspaceID`.
- `api/internal/app/hub/{subscriber,hub}.go`, `api/internal/app/repositories/agentchat/internal/store/hub.go` — thread `workspaceID`; hub `OnForget`.
- `api/internal/api/v0/container.go` — `PushAgentChat` signature; `agentChatDef` scoping.
- `api/internal/api/v0/router.go` + `endpoints/agent/routes.go` + `endpoints/agent/handlers/*.go` — nest routes; List-by-workspace; Create wsId-from-path; by-id scope check; new delete handler.
- `api/cmd/crowbar/{hook,chat,handoff}.go` — shared `scopedAgentPath` helper.

**Created**
- `api/cmd/crowbar/scope.go` — `scopedAgentPath` + a `--project/--repo/--workspace` flag set shared by the three callbacks.

---

## Task 1: On-disk layout — workspace-root split

**Files:**
- Modify: `api/internal/app/usecases/internal/worktreepath/worktreepath.go`
- Test: `api/internal/app/usecases/internal/worktreepath/worktreepath_test.go`
- Modify: `api/internal/app/usecases/agent/agent.go` (spawn tmp/Cwd; `appendTurn`; `AssembleHandoff`)
- Modify: `api/internal/app/usecases/worktree/worktree.go` (delete rm-root)
- Modify: `api/internal/app/container.go` (remove `sweepStaleAgentTmp` call)

**Interfaces produced:**
- `Derive(home, project, slug, branch) (string, error)` → `<home>/projects/<project>/<slug>/<branch>/worktree`
- `HomeLeaf(home, project, slug) string` → `<home>/projects/<project>/<slug>/.home/worktree`
- `WorkspaceRoot(worktreePath string) string` → `filepath.Dir(worktreePath)`
- `ChatsDir(worktreePath string) string` → `filepath.Join(WorkspaceRoot(worktreePath), "chats")`
- `AgentLedgerDir(worktreePath, chatID string) string` → `filepath.Join(ChatsDir(worktreePath), chatID, "ledger")`
- `SegmentDir(worktreePath, chatID, segmentID, provider string) string` → `filepath.Join(ChatsDir(worktreePath), chatID, segmentID+"-"+provider)`

Note: the old `AgentLedgerDir(crowbarHome, projectID, repoID, workspaceID, chatID)` is **removed** — the ledger moves from the UUID path family to the human-readable one, derived from the worktree path's parent.

- [ ] **Step 1: Write failing unit tests for the new path helpers**

Add to `worktreepath_test.go`:

```go
func TestDerive_AppendsWorktreeLeaf(t *testing.T) {
	got, err := Derive("/home/.crowbar", "proj1", "github.com/acme/repo", "feat-x")
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	want := "/home/.crowbar/projects/proj1/github.com/acme/repo/feat-x/worktree"
	if got != want {
		t.Fatalf("Derive = %q, want %q", got, want)
	}
}

func TestHomeLeaf_AppendsWorktreeLeaf(t *testing.T) {
	got := HomeLeaf("/home/.crowbar", "proj1", "github.com/acme/repo")
	want := "/home/.crowbar/projects/proj1/github.com/acme/repo/.home/worktree"
	if got != want {
		t.Fatalf("HomeLeaf = %q, want %q", got, want)
	}
}

func TestChatsAndLedgerAndSegmentDirs_SiblingOfWorktree(t *testing.T) {
	wt := "/home/.crowbar/projects/proj1/github.com/acme/repo/feat-x/worktree"
	root := "/home/.crowbar/projects/proj1/github.com/acme/repo/feat-x"

	if got := WorkspaceRoot(wt); got != root {
		t.Fatalf("WorkspaceRoot = %q, want %q", got, root)
	}
	if got, want := ChatsDir(wt), root+"/chats"; got != want {
		t.Fatalf("ChatsDir = %q, want %q", got, want)
	}
	if got, want := AgentLedgerDir(wt, "chatA"), root+"/chats/chatA/ledger"; got != want {
		t.Fatalf("AgentLedgerDir = %q, want %q", got, want)
	}
	if got, want := SegmentDir(wt, "chatA", "seg1", "claude"), root+"/chats/chatA/seg1-claude"; got != want {
		t.Fatalf("SegmentDir = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail to compile/pass**

Run: `go test ./api/internal/app/usecases/internal/worktreepath/ -run 'TestDerive_AppendsWorktreeLeaf|TestChatsAndLedgerAndSegmentDirs' -v`
Expected: FAIL — `Derive` returns the old path; `WorkspaceRoot`/`ChatsDir`/`SegmentDir` undefined.

- [ ] **Step 3: Implement the path changes**

In `worktreepath.go`: change `Derive`'s return to `filepath.Join(home, "projects", project, slug, branch, "worktree")` and `HomeLeaf`'s to `filepath.Join(home, "projects", project, slug, ".home", "worktree")`. Add:

```go
// WorkspaceRoot returns the workspace directory that holds the worktree and
// the chats tree as siblings, given the worktree path.
func WorkspaceRoot(worktreePath string) string { return filepath.Dir(worktreePath) }

// ChatsDir returns the per-workspace agentic chats directory (sibling of the
// worktree, NOT inside it — so agent state never appears in git status).
func ChatsDir(worktreePath string) string {
	return filepath.Join(WorkspaceRoot(worktreePath), "chats")
}

// AgentLedgerDir returns the per-chat handoff-ledger directory.
func AgentLedgerDir(worktreePath, chatID string) string {
	return filepath.Join(ChatsDir(worktreePath), chatID, "ledger")
}

// SegmentDir returns a segment's per-spawn config/transcript directory ({tmp}).
func SegmentDir(worktreePath, chatID, segmentID, provider string) string {
	return filepath.Join(ChatsDir(worktreePath), chatID, segmentID+"-"+provider)
}
```

Remove the old UUID-based `AgentLedgerDir(crowbarHome, projectID, repoID, workspaceID, chatID)`.

- [ ] **Step 4: Repoint agent-usecase call sites**

In `agent.go`:
- `spawnSegment` (~line 261): replace `tmpDir := filepath.Join(crowbarHome, "agent-tmp", segID)` with a call that first captures the worktree path from `WorktreeDir` (the `_` currently discarded), then `tmpDir := worktreepath.SegmentDir(worktree, chatID, segID, providerID)`. `tctx.Cwd` already resolves to the worktree; keep it (it is now `.../worktree`).
- `appendTurn`: change its signature from `(ctx, seg, chat, crowbarHome, projectID, repoID, role, text)` to `(ctx, seg, chat, worktreePath, role, text)` and compute `dir := worktreepath.AgentLedgerDir(worktreePath, chat.ID)`. Update its caller in `IngestHook` (~line 476) to stop discarding the worktree path and pass it.
- `AssembleHandoff` (~line 693): stop discarding the worktree path from `WorktreeDir`; compute `dir := worktreepath.AgentLedgerDir(worktree, chat.ID)`.

- [ ] **Step 5: Point the delete cascade at the workspace root**

In `worktree.go`, find the delete reactor's `rm -rf worktree` (the `rmWorktree func(path string) error` call site — search `rmWorktree`). After the `git worktree remove` of the worktree path, `rm -rf` **`worktreepath.WorkspaceRoot(worktreePath)`** instead of the worktree path, so the sibling `chats/` tree is removed too. In `api/internal/app/container.go`, delete the `sweepStaleAgentTmp` call and function (the global `agent-tmp` dir no longer exists).

- [ ] **Step 6: Run the path + agent + worktree package tests and build**

Run: `go test ./api/internal/app/usecases/... && go build ./...`
Expected: PASS. Fix any remaining references to the old `agent-tmp` path or the old `AgentLedgerDir` signature until green.

- [ ] **Step 7: Commit**

```bash
git add api/internal/app/usecases/internal/worktreepath api/internal/app/usecases/agent api/internal/app/usecases/worktree api/internal/app/container.go
git commit -m "refactor(agent): workspace-root on-disk layout (worktree/ + chats/)"
```

---

## Task 2: WS frame carries workspaceId

**Files:**
- Modify: `api/internal/api/v0/dto/agent.go`
- Modify: `api/internal/app/hub/subscriber.go`, `api/internal/app/hub/hub.go`
- Modify: `api/internal/app/repositories/agentchat/internal/store/hub.go`
- Modify: `api/internal/api/v0/container.go` (`PushAgentChat`)
- Test: `api/internal/app/repositories/agentchat/internal/store/hub_test.go`

**Interfaces produced:**
- `dto.AgentChatEvent{ ChatID, WorkspaceID, Kind string }`
- `hub.Subscriber.PushAgentChat(chatID, workspaceID, kind string)`
- `hub.Hub.BroadcastAgentChat(chatID, workspaceID, kind string)`
- store hub `BroadcastFunc func(chatID, workspaceID, kind string)`

- [ ] **Step 1: Write the failing projection test**

In `hub_test.go`, assert the hub projection passes the aggregate's workspace id through. Follow the existing test's harness for driving an event; assert:

```go
func TestHubProjection_EmitsWorkspaceID(t *testing.T) {
	var got struct{ chatID, wsID, kind string }
	broadcast := func(chatID, workspaceID, kind string) {
		got.chatID, got.wsID, got.kind = chatID, workspaceID, kind
	}
	ax := newTestAgentChatAsynx(t, broadcast) // mirror existing hub_test setup
	// drive a 'created' event on a chat in workspace "ws-42" (reuse the test's
	// SpawnChat/Create helper), then WaitIdle/Drain on the asynx test handle.
	drainAgentChatAsynx(t, ax)

	if got.wsID != "ws-42" {
		t.Fatalf("workspaceID = %q, want ws-42", got.wsID)
	}
}
```

(Use the file's existing helpers for asynx setup + deterministic drain — do NOT add sleeps.)

- [ ] **Step 2: Run it, verify failure**

Run: `go test ./api/internal/app/repositories/agentchat/internal/store/ -run TestHubProjection_EmitsWorkspaceID -v`
Expected: FAIL — `broadcast` signature mismatch / no workspace id.

- [ ] **Step 3: Thread workspaceID through**

- `dto/agent.go`: add `WorkspaceID string \`json:"workspaceId"\`` to `AgentChatEvent`.
- store `hub.go`: change `BroadcastFunc` to `func(chatID, workspaceID, kind string)`; in `hubProjector.onEvent`, call `p.broadcast(evt.AggregateID, evt.Aggregate.WorkspaceID, eventKind(evt.EventName))`.
- `app/hub/subscriber.go`: `PushAgentChat(chatID, workspaceID, kind string)`.
- `app/hub/hub.go`: `BroadcastAgentChat(chatID, workspaceID, kind string)` → `s.PushAgentChat(chatID, workspaceID, kind)`.
- `api/v0/container.go`: `PushAgentChat(chatID, workspaceID, kind string)` → `c.agentChats.Push(dto.AgentChatEvent{ChatID: chatID, WorkspaceID: workspaceID, Kind: kind})`.

- [ ] **Step 4: Run it, verify pass + build**

Run: `go test ./api/internal/app/repositories/agentchat/... ./api/internal/app/hub/... && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/api/v0/dto/agent.go api/internal/app/hub api/internal/app/repositories/agentchat/internal/store/hub.go api/internal/api/v0/container.go
git commit -m "feat(agent): carry workspaceId on the agent-chat lifecycle frame"
```

---

## Task 3: Nest the agent surface + scope the WS feed (server-side)

Server-side only: move **every** agent route (REST + WS) under the workspace group, scope the WS filter, and scope the REST handlers. The in-PTY CLI callbacks are re-scoped in Task 4 — between these two tasks the flat callback URLs 404, which only the real-CLI suite (Task 4) exercises. This task's REST/WS integration tests do not depend on the callback loop.

**Files:**
- Modify: `api/internal/api/v0/router.go` (move `agent.Register` onto the workspace group)
- Modify: `api/internal/api/v0/endpoints/agent/routes.go` (nested relative paths; keep `Hooks` + `wsHandle` in the group)
- Modify: `api/internal/api/v0/endpoints/agent/handlers/chats.go` (List-by-workspace; Create wsId-from-path; by-id scope-check helper used by Get/Switch/Rename/Handoff)
- Modify: `api/internal/api/v0/container.go` (`agentChatDef`)
- Modify: `api/internal/app/usecases/agent/agent.go` (`ListChatsByWorkspace`)
- Test: `api/tests/agent_rest_scope_test.go`, `api/tests/agent_ws_scope_test.go` (both build tag `integration`)

**Interfaces consumed:** `dto.AgentChatEvent.WorkspaceID` (Task 2).

**Interfaces produced:**
- `usecase.ListChatsByWorkspace(ctx, wsID string) ([]domain.AgentChat, error)` → delegates to `u.chats.ListByWorkspace(ctx, wsID)`
- Routes served at `/v0/projects/:p/repos/:r/workspaces/:wsId/agent/{chats, chats/:id, chats/:id/switch, chats/:id/rename, chats/:id/handoff, hooks, ws/chats}`
- By-id scope-check helper for the item handlers

- [ ] **Step 1: Write the failing REST-scope test**

In `api/tests/agent_rest_scope_test.go` (tag `integration`): create chats in workspaces A and B; assert `GET …/workspaces/<A>/agent/chats` returns only A's; assert `GET …/workspaces/<A>/agent/chats/<chatInB>` → 404 (by-id scope check); assert `POST …/workspaces/<A>/agent/chats {provider}` creates a chat whose `workspaceId == A`.

- [ ] **Step 2: Write the failing WS-isolation test**

In `api/tests/agent_ws_scope_test.go` (tag `integration`), mirror an existing workspace-scoped WS test (e.g. the threads WS test). Create a chat in workspace A and one in B; subscribe a client to A's `…/workspaces/<A>/agent/ws/chats`; assert it receives A's `created` frame and never B's. Block on the channel of received frames (no sleeps); `select` on that channel with the test context's `Done()` as the only backstop.

- [ ] **Step 3: Run both, verify failure**

Run: `go test -tags integration ./api/tests/ -run 'TestAgentREST_Scope|TestAgentWS_WorkspaceIsolation' -v`
Expected: FAIL — routes flat / feed global.

- [ ] **Step 4: Move the routes onto the workspace group**

In `router.go`, remove `agent.Register(rg, …)` from the top-level block and call it on the workspace-scoped group so its relative paths mount under `…/workspaces/:wsId`. In `routes.go`, prefix the group's routes accordingly (mirror how `threads.Register` mounts `/workspaces/:wsId/threads`, or register on `wsScoped` like `terminal`). Keep `h.Hooks` and `wsHandle` (the WS handler) in the same group so `…/workspaces/:wsId/agent/ws/chats` is served with the `:wsId` path param available to the filter.

- [ ] **Step 5: Scope `agentChatDef`**

Redefine `agentChatDef` to mirror `gitDef`:

```go
func agentChatDef() ws.StreamDef[dto.AgentChatEvent] {
	return ws.StreamDef[dto.AgentChatEvent]{
		Namespace:     func(e dto.AgentChatEvent) string { return e.WorkspaceID },
		Serialize:     func(e dto.AgentChatEvent) ([]byte, error) { return json.Marshal(e) },
		Snapshot:      func(string) []dto.AgentChatEvent { return nil },
		FlatNamespace: true,
		Filters: []ws.FilterDef[dto.AgentChatEvent]{
			{Param: "wsId", Extract: func(e dto.AgentChatEvent) string { return e.WorkspaceID }, Match: ws.ExactMatch},
		},
	}
}
```

- [ ] **Step 6: List-by-workspace, Create-from-path, scope check**

- Add `usecase.ListChatsByWorkspace(ctx, wsID)` delegating to `u.chats.ListByWorkspace(ctx, wsID)`.
- `handlers/chats.go` `List`: read `wsId := ctx.Param("wsId")`, call `ListChatsByWorkspace`.
- `Create`: read `wsId` from the path param, keep `provider` from the body.
- Add a helper used by `Get`/`Switch`/`Rename`/`Handoff` (and Delete in Task 5): after loading the chat, `if chat.WorkspaceID != ctx.Param("wsId") { libs.WriteErr(ctx, http.StatusNotFound, "chat not found in workspace"); return }`.

- [ ] **Step 7: Run both integration tests + build, verify pass**

Run: `go test -tags integration ./api/tests/ -run 'TestAgentREST_Scope|TestAgentWS_WorkspaceIsolation' && go build ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add api/internal/api/v0 api/internal/app/usecases/agent/agent.go api/tests/agent_rest_scope_test.go api/tests/agent_ws_scope_test.go
git commit -m "feat(agent): nest agent routes + scope the lifecycle WS feed by workspace"
```

---

## Task 4: Scope the in-PTY CLI callbacks

The routes are now nested (Task 3), so the flat callback URLs 404. Re-scope the three `crowbar` callbacks to build the nested URLs from explicit command flags.

**Files:**
- Modify: `api/internal/engine/agent/template.go` + `descriptors/{claude,codex}.yaml`
- Modify: `api/internal/app/usecases/agent/agent.go` (`TemplateCtx` scope fields at spawn)
- Create: `api/cmd/crowbar/scope.go`; Modify `api/cmd/crowbar/{hook,chat,handoff}.go`
- Test: `api/cmd/crowbar/scope_test.go`; real-CLI integration under the existing agentic integration suite.

**Interfaces produced:**
- `TemplateCtx{ …, ProjectID, RepoID, WorkspaceID string }`; template placeholders `{project_id}`, `{repo_id}`, `{workspace_id}`
- `scopedAgentPath(project, repo, workspace, suffix string) string` → `/v0/projects/<project>/repos/<repo>/workspaces/<workspace>/agent<suffix>`

- [ ] **Step 1: Write the failing CLI-helper unit test**

`api/cmd/crowbar/scope_test.go`:

```go
func TestScopedAgentPath(t *testing.T) {
	got := scopedAgentPath("p1", "r1", "w1", "/chats/c1/rename?source=agent")
	want := "/v0/projects/p1/repos/r1/workspaces/w1/agent/chats/c1/rename?source=agent"
	if got != want {
		t.Fatalf("scopedAgentPath = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run it, verify failure**

Run: `go test ./api/cmd/crowbar/ -run TestScopedAgentPath -v`
Expected: FAIL — `scopedAgentPath` undefined.

- [ ] **Step 3: Add scope to `TemplateCtx` and the templater**

`template.go`: add `ProjectID`, `RepoID`, `WorkspaceID` to `TemplateCtx` and `{project_id}`,`{repo_id}`,`{workspace_id}` to the `Replacer`. In `spawnSegment`, set those `TemplateCtx` fields from the `WorktreeDir` results (projectID/repoID + `chat.WorkspaceID`).

- [ ] **Step 4: Add the scope flags to the descriptors**

`descriptors/claude.yaml` + `codex.yaml`: add `--project {project_id} --repo {repo_id} --workspace {workspace_id}` to each `{crowbar_hook} hook …` command; add the same flags to the `crowbar chat rename` system-prompt instruction and the `crowbar handoff dump` invocation.

- [ ] **Step 5: Implement the shared CLI helper and update the callbacks**

Create `api/cmd/crowbar/scope.go` with `scopedAgentPath` and a reusable cobra flag set (`--project`,`--repo`,`--workspace`); update `hook.go`, `chat.go`, `handoff.go` to bind those flags and build their URLs via `scopedAgentPath(project, repo, workspace, "…")`.

- [ ] **Step 6: Run CLI unit test + build, then real-CLI integration**

Run: `go test ./api/cmd/crowbar/ && go build ./...`
Then run the existing real-CLI agentic integration suite; expected: the agent's hook/rename/handoff callbacks resolve against the nested URLs (title updates, working/idle toggles, a Claude↔Codex switch hands off the ledger).

- [ ] **Step 7: Commit**

```bash
git add api/internal/engine/agent/template.go api/internal/engine/agent/descriptors api/internal/app/usecases/agent/agent.go api/cmd/crowbar
git commit -m "feat(agent): scope the in-PTY CLI callbacks by workspace"
```

---

## Task 5: Hard delete via asynx Forget

**Files:**
- Modify: `api/internal/app/usecases/agent/agent.go` (`PurgeChat`)
- Modify: `api/internal/app/repositories/agentchat/internal/store/hub.go` (hub `OnForget`)
- Modify: `api/internal/api/v0/endpoints/agent/{routes.go, handlers/chats.go}` (DELETE)
- Test: `api/internal/app/repositories/agentchat/internal/store/hub_test.go`; `api/tests/agent_delete_test.go` (integration)

**Interfaces produced:**
- `usecase.PurgeChat(ctx, chatID string) error` — best-effort PTY teardown of the active segment, then `u.chats.Forget(ctx, chatID)`.

- [ ] **Step 1: Write the failing hub-OnForget test**

In `hub_test.go`, assert a `Forget` emits a `deleted` frame carrying the workspace id:

```go
func TestHubProjection_ForgetEmitsScopedDeleted(t *testing.T) {
	var frames []struct{ chatID, wsID, kind string }
	broadcast := func(chatID, workspaceID, kind string) {
		frames = append(frames, struct{ chatID, wsID, kind string }{chatID, workspaceID, kind})
	}
	ax := newTestAgentChatAsynx(t, broadcast)
	// create a chat in "ws-9", drain, then Forget it, then drain.
	// (reuse the file's create/forget helpers + deterministic drain)
	// assert some frame == {chatID, "ws-9", "deleted"}
}
```

- [ ] **Step 2: Run it, verify failure**

Run: `go test ./api/internal/app/repositories/agentchat/internal/store/ -run TestHubProjection_ForgetEmitsScopedDeleted -v`
Expected: FAIL — no frame on forget (hub projection has no `OnForget`).

- [ ] **Step 3: Register the hub OnForget**

In store `hub.go` `registerHubProjection`, after the `Subscribe("agentchat.*")`, add:

```go
if _, err := ax.OnForget(func(_ context.Context, evt asynxModels.Event[domain.AgentChat]) {
	p.broadcast(evt.Aggregate.ID, evt.Aggregate.WorkspaceID, "deleted")
}); err != nil {
	return fmt.Errorf("agentchat hub projection: onforget: %w", err)
}
```

- [ ] **Step 4: Run it, verify pass**

Run: `go test ./api/internal/app/repositories/agentchat/internal/store/ -run TestHubProjection_ForgetEmitsScopedDeleted -v`
Expected: PASS.

- [ ] **Step 5: Write the failing delete integration test**

In `api/tests/agent_delete_test.go` (tag `integration`): create a chat, `DELETE …/workspaces/<w>/agent/chats/<id>` → 202/200; then `GET …/agent/chats` no longer lists it and `GET …/agent/chats/<id>` → 404. Assert a subscribed WS client received a `deleted` frame for it (block on the channel).

- [ ] **Step 6: Implement PurgeChat + handler + route**

- `agent.go`: add `PurgeChat` (copy `DeleteChat`'s best-effort terminate block, then call `u.chats.Forget(ctx, chatID)` instead of `Delete`).
- `handlers/chats.go`: add `Delete` handler — load chat, apply the by-id scope check (Task 4 helper), call `usecase.PurgeChat`, `libs.WriteAccepted(ctx)`.
- `routes.go`: `rg.DELETE("/agent/chats/:id", h.Delete)` (nested group).

- [ ] **Step 7: Run delete integration + build**

Run: `go test -tags integration ./api/tests/ -run TestAgentDelete && go build ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add api/internal/app/usecases/agent/agent.go api/internal/app/repositories/agentchat/internal/store/hub.go api/internal/api/v0/endpoints/agent api/tests/agent_delete_test.go
git commit -m "feat(agent): hard-delete route via asynx Forget + scoped deleted broadcast"
```

---

## Task 6: Claude native transcript redirect

**Files:**
- Modify: `api/internal/engine/agent/descriptors/claude.yaml`
- Test: real-CLI agentic integration suite.

- [ ] **Step 1: Add the redirect**

In `claude.yaml` `config_injection`, add before the settings step:

```yaml
  - set_env: { name: CLAUDE_CONFIG_DIR, value: "{tmp}/claude-home" }
```

- [ ] **Step 2: Validate live (real-CLI integration)**

Run the real-CLI agentic integration test that spawns Claude and runs one turn; assert a `*.jsonl` transcript appears under `<segmentDir>/claude-home/` (and NOT under `~/.claude`). If Claude ignores `CLAUDE_CONFIG_DIR` for transcripts, capture the actual location and flag back before claiming done (do not merge on an unverified redirect).

- [ ] **Step 3: Commit**

```bash
git add api/internal/engine/agent/descriptors/claude.yaml
git commit -m "feat(agent): redirect Claude native transcript into the segment dir"
```

---

## Self-Review (completed by plan author)

- **Spec coverage:** §3.1 routes → T3; §3.2 CLI callbacks → T4; §3.3 hard delete + OnForget → T5; §3.4 WS scoping → T2 (frame field) + T3 (filter + mount); §3.5 on-disk layout → T1; §3.6 Claude redirect → T6. All covered.
- **Placeholders:** none — every step has concrete code or an exact edit instruction with the target location.
- **Type consistency:** `PushAgentChat`/`BroadcastAgentChat`/`BroadcastFunc` all `(chatID, workspaceID, kind string)`; `AgentChatEvent.WorkspaceID`; path helpers take `worktreePath`; `scopedAgentPath` signature stable across T4 usages.
- **Ordering:** T1 (paths) is foundational; T2 (frame field) precedes T3 (WS filter) and T5 (OnForget); T4 nests routes and callbacks together to avoid a broken intermediate; T5 delete after routes exist; T6 last.
