# Sync restyling/v2 with develop — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `origin/develop`'s 11 commits (native context menus, chat attachments, Codex streaming/subagent fixes, storage-integrity fixes, atomic provider switch) into `restyling/v2`, without losing restyling/v2's UI restyle or its chat-usecase backend refactor.

**Architecture:** A `git merge origin/develop` is already in progress in this worktree (started with `git config merge.conflictstyle diff3`, so every conflict marker shows base/ours/theirs). **Do not run `git merge` again or `git merge --abort`.** 43 files are currently conflicted; every other file from develop's 436 changed files already applied automatically. This plan resolves the 43 conflicts task-by-task, `git add`-staging each as it's finished, then does ONE final `git commit` to conclude the merge (git will not allow more than one commit for this merge — that is not a shortcut being skipped, it's how `git merge` works).

**Tech Stack:** Go (api/), TypeScript/React (web/), Rust/Tauri (desktop/src-tauri/).

**Spec:** `docs/superpowers/specs/2026-09-15-develop-sync-merge-design.md`

## Global Constraints

- Never run `git merge --abort`, `git checkout --ours/--theirs` as a blind bulk resolution, or `git stash` on this worktree (shared stash stack — see project conventions).
- A resolution that silently drops a real behavior fix (from either branch) is a plan failure, not an acceptable shortcut. Every conflict below was individually diagnosed against `git show <sha>` for the branch that introduced it — the resolutions are not guesses.
- Never run the full `vitest run` or full Go test suite. Targeted/modified-file tests only, per this project's standing rule.
- Preserve restyling/v2's visual/CSS values exactly as they are today UNLESS the conflicting value is something restyling/v2 never touched (verify via `git show 0043620fc:<path>` — if restyling/v2's copy is identical to that base commit for the specific line in question, it's fair game to take develop's change).
- After each task, run `git diff --check <files>` to confirm no leftover `<<<<<<<`/`=======`/`>>>>>>>` markers, then `git add` exactly the files that task named.
- The merge-base commit is `0043620fc`. `origin/develop` tip is `493991137`.

---

### Task 1: Resolve Go backend — activity/turn core

**Files:**
- Modify (resolve conflict): `api/internal/app/repositories/chat/activity/activity.go`
- Modify (resolve conflict): `api/internal/app/usecases/chat/internal/turn/turn.go`

**Context:** Both files contain a "convergent fix" — restyling/v2 and develop independently fixed the same stuck-spinner bug, but develop's fix is a strict superset (confirmed: `api/internal/app/usecases/chat/internal/turn/observation.go` and `.../turn/nested.go`, already merged cleanly, call `t.activity.OpenNestedSubagent(...)` and `t.activity.StopSubagent(ctx, chatID, subagentID, agentType, message, now)` — a 5-arg signature with `message` — meaning `activity.go`'s `eventSourced` MUST implement exactly develop's shape or the package will not compile).

- [ ] **Step 1: Resolve `activity.go`**

Take **develop's side of both conflicts in full** — delete restyling/v2's version of the `CompleteTool` comment and the `StopSubagent`/`OpenNestedSubagent` block entirely, keep develop's. Concretely, both hunks should end up reading exactly as the `origin/develop` side already shows in the conflict (i.e. resolve by deleting the `<<<<<<< HEAD` and `||||||| 0043620fc` sections and everything between them and `=======`, keeping the `=======`...`>>>>>>> origin/develop` content, then removing the marker lines themselves). This is safe because develop's version already uses `sendWait` (the same fix restyling/v2 converged on independently) and additionally defines `OpenNestedSubagent`, which `observation.go` already requires.

- [ ] **Step 2: Resolve `turn.go`**

Take **develop's side in full** for both hunks. Restyling/v2's fix re-sourced the precondition check from `GetChat` (a lagging projection) to `LoadChat` (the authoritative log-fold) but kept the check inline in `restateAsyncWork`. Develop's fix removes the check from `restateAsyncWork` entirely and relies on `StopTurn.Validate` (in `api/internal/app/repositories/chat/internal/commands/stop_turn.go`, already merged cleanly — confirmed it already has `if c.Restate { if current.CurrentTurnStarted != nil {...}; if current.AsyncWork == c.AsyncWork {...} }`, evaluated atomically against the authoritative fold at commit time by asynx). Develop's approach is strictly more race-proof (no read-then-decide window at all, vs. restyling's LoadChat-then-decide which still has a narrower window), and the function's own error handling already treats `asynxModels.ErrValidation` as an expected no-op — confirming the code downstream already assumes this simplification. Delete the `chat, err := t.chats.LoadChat(...)` / `GetChat(...)` block and its two comment variants; keep develop's comment and go straight to `open, err := t.OpenWork(ctx, chatID)`.

- [ ] **Step 3: Build and verify**

Run: `cd api && go build ./internal/app/repositories/chat/activity/... ./internal/app/usecases/chat/internal/turn/...`
Expected: compiles clean (this alone won't succeed until Task 2's files are also resolved, since they're in the same module — if it fails only on unrelated still-conflicted files, that's expected at this point; check the error is not about `activity.go`/`turn.go` themselves).

- [ ] **Step 4: Stage**

```bash
git add api/internal/app/repositories/chat/activity/activity.go api/internal/app/usecases/chat/internal/turn/turn.go
```

---

### Task 2: Resolve Go backend — attach.go, spawn.go, and their tests, plus the repo-wide stale `worktreepath` import

**Files:**
- Modify (resolve conflict): `api/internal/app/usecases/chat/internal/runner/attach.go`
- Modify (resolve conflict): `api/internal/app/usecases/chat/internal/runner/attach_internal_test.go`
- Modify (resolve conflict): `api/internal/app/usecases/chat/internal/runner/spawn.go`
- Modify (mechanical import fix, NOT a merge conflict — see below): `api/internal/app/usecases/chat/spawn_attachments_test.go`, `api/internal/app/usecases/chat/internal/runner/dispatch_attachments.go`, `api/internal/app/usecases/chat/internal/runner/dispatch_attachments_internal_test.go`, `api/internal/app/usecases/chat/internal/runner/dispatch_attachments_api_internal_test.go`, `api/internal/app/usecases/chat/internal/turn/attachments.go`, `api/internal/app/usecases/chat/internal/turn/turn_open.go`
- Modify (mechanical import fix ONLY — the import line at the top of the file, not its conflict body, which Task 3 owns separately): `api/internal/app/usecases/chat/turn_test.go`

**Context:** `attach.go` has two INDEPENDENT fixes to the same `CreateCommand` call: restyling/v2 fixed the session-key argument (`live.WorkspaceID` → `chatID`), develop fixed the env argument (`nil` → `os.Environ()`, confirmed live-critical: passing `nil` left the native view with no `PATH`/`HOME`, so every hook-wired command died with exit 127). Both are needed. `spawn.go` is an import-path collision: restyling/v2 moved the `worktreepath` package to `core/paths/worktreepath` (confirmed: `api/internal/app/usecases/internal/worktreepath/` no longer exists on disk), while develop's diff is against the pre-move path and also adds a new `promptsigil` import (confirmed the package exists post-merge at `api/internal/app/usecases/chat/internal/shared/promptsigil/`).

**Discovered during Task 1 (ruling recorded in the ledger):** the `worktreepath` package move affects more than the one conflicted file. Seven MORE files — all from develop's chat-attachments feature (`dispatch_attachments*.go`, `spawn_attachments_test.go`, `turn/attachments.go`, `turn/turn_open.go`) plus `turn_test.go`'s import line — merged in with NO conflict marker (git saw only one side touch them) but still import the now-deleted `api/internal/app/usecases/internal/worktreepath` path. Confirmed: the package's public API (`AttachmentsDir`, `RestoreDurableAttachmentRefs`) is unchanged in its new home at `api/internal/core/paths/worktreepath`, so this is a pure import-path string fix, one line per file, no logic changes — the same fix as `spawn.go`'s Step 3 below, just not conflict-marked.

- [ ] **Step 1: Resolve `attach.go`**

Find the conflicted line (inside `SwitchToTerminal`) and resolve to:

```go
	termSessID, err := rs.term.CreateCommand(ctx, chatID, tctx.Cwd, argv, os.Environ(),
```

(`os` is already imported in this file's non-conflicted import block; `chatID` is the function's own parameter, already in scope.)

- [ ] **Step 2: Resolve `attach_internal_test.go`**

This test's fake mirrors `attach.go`'s call. Resolve the struct fields to:

```go
	chatID     string
	cwd        string
	argv       []string
	env        []string
	termSessID string
```

Resolve the fake `CreateCommand`'s signature to:

```go
	_ context.Context, chatID, cwd string, argv, env []string, onExit func(),
```

Resolve the call-recording line to:

```go
	f.created = append(f.created,
		fakeTermCall{chatID: chatID, cwd: cwd, argv: argv, env: env, termSessID: id})
```

- [ ] **Step 3: Resolve `spawn.go`**

Resolve the import block to (alphabetical, matching goimports):

```go
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/promptsigil"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
```

- [ ] **Step 4: Fix the import line in the 7 non-conflicted files**

In each of `api/internal/app/usecases/chat/spawn_attachments_test.go`, `api/internal/app/usecases/chat/internal/runner/dispatch_attachments.go`, `api/internal/app/usecases/chat/internal/runner/dispatch_attachments_internal_test.go`, `api/internal/app/usecases/chat/internal/runner/dispatch_attachments_api_internal_test.go`, `api/internal/app/usecases/chat/internal/turn/attachments.go`, `api/internal/app/usecases/chat/internal/turn/turn_open.go`, and `api/internal/app/usecases/chat/turn_test.go` — change the import line:

```go
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
```
to:
```go
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
```

Nothing else in these files changes — every call site (`worktreepath.AttachmentsDir(...)`, `worktreepath.RestoreDurableAttachmentRefs(...)`) keeps working unchanged since the package name is the same, only its import path moved. For `turn_test.go` specifically: touch ONLY this import line — its conflict markers elsewhere in the file belong to Task 3, do not resolve them here.

- [ ] **Step 5: Build and verify**

Run: `cd api && go build ./internal/app/usecases/chat/...`
Expected: compiles clean except for files still holding unresolved conflict markers from OTHER tasks (Tasks 3, 4, 5 — not yet done). If the build fails specifically citing the files this task touched, that's a real problem to fix before reporting.

- [ ] **Step 6: Stage**

```bash
git add api/internal/app/usecases/chat/internal/runner/attach.go api/internal/app/usecases/chat/internal/runner/attach_internal_test.go api/internal/app/usecases/chat/internal/runner/spawn.go api/internal/app/usecases/chat/spawn_attachments_test.go api/internal/app/usecases/chat/internal/runner/dispatch_attachments.go api/internal/app/usecases/chat/internal/runner/dispatch_attachments_internal_test.go api/internal/app/usecases/chat/internal/runner/dispatch_attachments_api_internal_test.go api/internal/app/usecases/chat/internal/turn/attachments.go api/internal/app/usecases/chat/internal/turn/turn_open.go
```

Do NOT `git add api/internal/app/usecases/chat/turn_test.go` yet — it still has unrelated unresolved conflict markers (Task 3 owns staging it once those are resolved too).

---

### Task 3: Resolve Go backend — harness_test.go, settle_internal_test.go, turn_test.go

**Files:**
- Modify (resolve conflict): `api/internal/app/usecases/chat/harness_test.go`
- Modify (resolve conflict): `api/internal/app/usecases/chat/internal/runner/settle_internal_test.go`
- Modify (resolve conflict): `api/internal/app/usecases/chat/turn_test.go`

**Context:** All three conflicts here are purely additive — each side added independent new test fixture fields or entirely new test functions at the same insertion point, with an EMPTY base (confirmed via diff3: `||||||| 0043620fc` shows nothing between the markers for the real conflicts in the latter two files). Union both sides; nothing to reconcile.

- [ ] **Step 1: Resolve `harness_test.go`**

Merge both sides' struct fields:

```go
	home        string
	projectID   string
	repoID      string
	worktree    string
	chatsDir    string
	err         error
	lastWorkspaceID string
	worktreeDirIDs  []string
	worktreeErr     error // fails only WorktreeDir, leaving AgentChatsDir callers unaffected
```

(Keep restyling/v2's doc comments on `lastWorkspaceID`/`worktreeDirIDs` verbatim — they were in the `HEAD` block above the fields.)

- [ ] **Step 2: Resolve `settle_internal_test.go`**

This is an "AA" conflict (both sides added the file new, independently). Keep BOTH bodies concatenated: restyling/v2's `newTestRunnersWithPrompts`/`TestPromptJournalDirFor_*` tests, followed by develop's `stubChatsForSettle`/`stubConversationsForSettle`/`settledCall`/`settleFixture`/`TestRegression_SettleDelivery*` tests. Merge the two `import` blocks at the top (union all imports from both: `errors`, `path/filepath`, `testing`, `context`, `time`, `github.com/stretchr/testify/assert`, `github.com/stretchr/testify/require`, `github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal`, `agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"`, `github.com/char2cs/crowbar/api/internal/domain"`) — there is exactly one `package runner` line, one `import (...)` block, both function sets below it.

- [ ] **Step 3: Resolve `turn_test.go`**

Note: this file's `import` line for `worktreepath` was already fixed by Task 2 Step 4 (an unrelated, non-conflicted stale-import fix) — leave that line alone, it is not part of your conflict. Same pattern for the conflict hunks themselves: keep BOTH sets of test functions — restyling/v2's `TestRegression_RestatingAsyncWorkDecidesOnTheLogNotTheProjection` and `TestRegression_TwoPiecesOfWorkClosingAtOnce_StopsTheSpinner`, followed by develop's `TestRegression_CodexSubagentsDrainOneAtATime_SpinnerFollowsTheLastOne`, `TestRegression_CodexCompactionTurnNeverStopsTheChat`, `TestRegression_CodexFailedCompactionTurnRecordsNoFailureNotice`, `TestRegression_CodexManualCompactionIsLabelledManual`, `TestRegression_CodexAutomaticCompactionIsNotLabelledManual`, `TestRegression_CodexAutoCompactionMidPromptDoesNotSettleTheRealDelivery`. No renames needed — verify no duplicate helper names between the two blocks (`stopPayload` vs `stopPayloadFor` are already distinctly named).

- [ ] **Step 4: Run the tests**

Run: `cd api && go test ./internal/app/usecases/chat/... -run 'TestPromptJournalDirFor|TestRegression_SettleDelivery|TestRegression_RestatingAsyncWork|TestRegression_TwoPiecesOfWork|TestRegression_Codex' -v`
Expected: all PASS. (This exercises everything just merged in this task, plus Task 1's turn.go fix.)

- [ ] **Step 5: Stage**

```bash
git add api/internal/app/usecases/chat/harness_test.go api/internal/app/usecases/chat/internal/runner/settle_internal_test.go api/internal/app/usecases/chat/turn_test.go
```

---

### Task 4: Resolve Go API layer — repo-scoped routing gains develop's attachments + pending-prompt routes

**Files:**
- Modify (resolve conflict): `api/internal/api/v0/endpoints/chat/routes.go`
- Modify (resolve conflict): `api/internal/api/v0/route_audit_test.go`
- Modify (resolve conflict): `api/internal/api/v0/dto/agent.go`
- Modify (resolve conflict): `api/internal/api/libs/status.go`

**Context:** restyling/v2 re-scoped every chat route from workspace-scoped (`wsScoped`, `.../workspaces/:id/chats`) to repo-scoped (`repoScoped`, `.../repos/:repoId/chats` — confirmed via `routes.go`'s own package doc: "a chat's workspace is optional and mutable and a URL that names one goes stale"). Develop is unaware of this and still registers on `wsScoped`, but ALSO adds three routes restyling/v2 doesn't have yet: chat attachments (POST/GET/HEAD) and pending-prompt (GET). Port develop's three new routes onto the `repoScoped` group, do not adopt develop's `wsScoped` naming.

- [ ] **Step 1: Resolve `routes.go`**

Keep the function signature's parameter named `repoScoped` (not `wsScoped`). In the route list, keep every existing `repoScoped.*` line as-is (including `repoScoped.POST("/chats/:id/promote", h.Promote)`, which develop's list doesn't have at all — that's restyling/v2's own §4.2 feature, unrelated to this sync, keep it). Insert develop's three new routes, rewritten onto `repoScoped`, in the position develop had them (right after the existing `/activity/:toolId/payload` route, before `/choices`):

```go
	repoScoped.POST("/chats/:id/attachments", h.UploadAttachment)
	repoScoped.GET("/chats/:id/attachments/:file", h.Attachment)
	// HEAD, not just GET: the web client's own file-card size label is a
	// HEAD against this exact route (chat-asset-resolver.ts). With no HEAD
	// route registered it always 404'd — Attachment itself never checks the
	// request method, so net/http's own HEAD handling (real headers, body
	// suppressed) is all a route pointed at it needs.
	repoScoped.HEAD("/chats/:id/attachments/:file", h.Attachment)
```

And insert develop's pending-prompt route right after `repoScoped.POST("/chats/:id/prompts", h.SubmitPrompt)`:

```go
	repoScoped.GET("/chats/:id/pending-prompt", h.PendingPrompt)
```

Verify `h.UploadAttachment`, `h.Attachment`, `h.PendingPrompt` exist on the `agenthandlers` package (they should already exist from develop's non-conflicting handler files that merged in cleanly — `api/internal/api/v0/endpoints/chat/handlers/attachments.go` and `pendingprompt.go` were both new files in develop's diff and are NOT in the conflict list, so they merged in automatically).

- [ ] **Step 2: Resolve `route_audit_test.go`**

This file's job is to assert the exact route list. Wherever the conflict shows `repo + "..."` on HEAD's side and `ws + "..."` on develop's side for an EXISTING route, keep `repo +`. For develop's NEW assertions (the attachments POST/GET/HEAD lines and the pending-prompt GET line), rewrite them onto `repo +` and add them to the expected list at the position matching Step 1's insertion points.

- [ ] **Step 3: Resolve `dto/agent.go`**

Both conflicts here are purely additive — keep both sides' fields/consts, nothing overlaps:

```go
	// (existing Worktree field's doc comment and field, from HEAD, unchanged)
	Worktree *ChatWorktreeDTO `json:"worktree,omitempty"`

	// (develop's PromptConsumed doc comment and field, unchanged)
	PromptConsumed bool `json:"promptConsumed,omitempty"`
```

and

```go
	// (HEAD's AgentChatKindWorktreeState doc comment, unchanged)
	const AgentChatKindWorktreeState = "worktree_state"

	// (develop's AgentChatKindPlan doc comment, unchanged)
	const AgentChatKindPlan = "plan"
```

Confirmed load-bearing: `api/internal/api/v0/container.go` (already merged, non-conflicted) already references `AgentChatKindPlan`.

- [ ] **Step 4: Resolve `status.go`**

Purely additive doc-comment merge — combine both bullet lists into one (both describe `404 Not Found` and `400 Bad Request` cases; keep restyling/v2's folder/Chats-panel sentinel line AND develop's `repoattachments.ErrNotFound`/`folder.ErrFolderNameRequired` lines). Add develop's two new imports (`repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"` and `"github.com/char2cs/crowbar/api/internal/app/usecases/folder"`) to the top of the file.

- [ ] **Step 5: Build**

Run: `cd api && go build ./internal/api/... `
Expected: compiles clean.

- [ ] **Step 6: Run the route audit test**

Run: `cd api && go test ./internal/api/v0/... -run TestRouteAudit -v`
Expected: PASS.

- [ ] **Step 7: Stage**

```bash
git add api/internal/api/v0/endpoints/chat/routes.go api/internal/api/v0/route_audit_test.go api/internal/api/v0/dto/agent.go api/internal/api/libs/status.go
```

---

### Task 5: Resolve Go integration tests — fixtures, import-branches, regressions, and the folders DU

**Files:**
- Modify (resolve conflict): `api/tests/fixtures_test.go`
- Modify (resolve conflict): `api/tests/regression_import_branches_test.go`
- Modify (resolve conflict): `api/tests/regressions_test.go`
- Delete (confirm): `api/tests/regression_folders_test.go`

**Context:** restyling/v2 replaced a WebSocket-dial-and-await pattern (`h.dial(".../workspaces")` + `readUntil`) with `h.Quiesce()` + a plain synchronous list read, for the specific fixtures that resolve "the chat for the main managed worktree" — this is NOT a stylistic choice, it flows from the chat-scoped-API refactor (Task 4): the point of that refactor is that a client reads the CHAT list, not a joined workspace stream, to learn this. Develop, unaware of the refactor, patched the OLD dial-based pattern with an `h.Quiesce()` barrier to fix a real flake (detailed in its own comment: the dial can race the async projection that would answer it). Since restyling/v2's `Quiesce()`-then-plain-read approach has no dial-before-broadcast race to begin with (Quiesce drains ALL pending projections before the plain read happens), it does not need develop's WS-dial barrier — but MUST use `h.Quiesce()`, which it already does.

- [ ] **Step 1: Resolve `fixtures_test.go`**

For BOTH conflicted regions (`importProject` and `importHomeDetached` fixtures): keep restyling/v2's (`HEAD`) version — `h.Quiesce()` followed by `listChats`/plain field reads — discarding develop's `workspacesWS := h.dial(...)` / `readUntil` approach entirely for these two fixtures. Keep HEAD's comments (they already explain why this reads back rather than dialing).

- [ ] **Step 2: Resolve `regression_import_branches_test.go`**

Same pattern — keep HEAD's `h.Quiesce()` + `listChats` loop for `mainWSID`, discard develop's dial-based version.

- [ ] **Step 3: Resolve `regressions_test.go`**

Two separate conflicts here, resolved differently:

1. `createChildWorkspace`: restyling/v2 did NOT touch this function's body at all (only reworded the adjacent `syncBaseline` doc comment). Keep the function body from develop's side IN FULL, including its added `h.Quiesce()` before `return id` (a real, independent fix for read-model lag after creating a child workspace — nothing to do with the dial-vs-Quiesce architecture question above, since this function genuinely still creates a workspace via `/workspaces`, which is unaffected by the chat-routing refactor). For the `syncBaseline` doc comment immediately below it, keep restyling/v2's reworded text ("hits the chat-keyed sync verb (spec §4.3)").
2. The `waitForWorkComplete(t, ..., wsID)` line: keep restyling/v2's variable name (`conn` — confirm by reading a few lines above the conflict that `conn` is the name already in scope in this function; if instead it turns out `conn` was a mistaken/unintroduced rename, use whatever local variable HEAD's surrounding code actually established). Add develop's trailing `h.Quiesce()` call (with its comment) immediately after, since it's a genuine, independent read-model-lag fix.

- [ ] **Step 4: Resolve the `regression_folders_test.go` deletion**

This file is currently sitting in the working tree as develop's version (git leaves the "modified" side's content for a modify/delete conflict). Confirm it should stay deleted: restyling/v2 retired the whole `usecases/folder` package in favor of the unified chat-tree (`11b72c720`), and its regression scenarios were re-covered under `api/tests/regression_agent_chat_folders_test.go` / `regression_agent_chat_threads_test.go` / `api/internal/app/usecases/chat/internal/tree/move_test.go`.

Develop's only actual change to this file since the fork was adding one `h.Quiesce()` inside `TestRegression_WorkspaceMoveRefusedWhenItWouldSplitAForkChain` (a read-model-lag flake fix, not a new test). Search the current test suite for equivalent coverage:

```bash
grep -rn "func Test" api/internal/app/usecases/worktree/worktree_test.go api/internal/app/usecases/project/project_import_test.go api/tests/regression_agent_chat_*.go | grep -i fork
```

If no test exercises "a workspace move refused because it would split a fork chain," that coverage was genuinely dropped by the retirement and needs a new regression test in whichever file now owns workspace-move business rules (likely `api/internal/app/usecases/chat/internal/tree/move_test.go`, alongside its existing `TestMove_Refuses*` tests) — port the scenario from `git show 0043620fc:api/tests/regression_folders_test.go` (`TestRegression_WorkspaceMoveRefusedWhenItWouldSplitAForkChain`, lines ~221-259 at that revision) adapted to the new tree-usecase test harness, INCLUDING an `h.Quiesce()`-equivalent barrier if the new harness has the same read-after-write timing hazard. If equivalent coverage already exists (even under a different name), no action needed beyond confirming it and noting so.

Finally: `git rm api/tests/regression_folders_test.go` (it's already gone from the working tree; this records the deletion for the merge).

- [ ] **Step 5: Run the affected tests**

Run: `cd api && go test ./tests/... -run 'TestImport|TestRegression' -v`
Expected: all PASS (this is an integration test package — do not run the full `api/tests` suite beyond this filtered set, per the no-full-suite rule; this filter already covers everything touched in this task).

- [ ] **Step 6: Stage**

```bash
git add api/tests/fixtures_test.go api/tests/regression_import_branches_test.go api/tests/regressions_test.go
git rm api/tests/regression_folders_test.go
# plus any new test file created in Step 4, if the fork-chain coverage gap was real
```

---

### Task 6: Resolve web frontend — small/mechanical conflicts

**Files:**
- Modify (resolve conflict): `web/src/features/agent/api/agent-api.ts`
- Modify (resolve conflict): `web/src/features/agent/chat/agent-empty-document.tsx`
- Modify (resolve conflict): `web/src/features/agent/styles/composer.css`
- Modify (resolve conflict): `web/src/features/terminal/components/terminal-tab.tsx`
- Modify (resolve conflict): `web/src/features/workspace/stores/slices/agent-chats-slice.ts`

- [ ] **Step 1: Resolve `agent-api.ts`**

Take HEAD's side of the `chatBase` conflict in full (the `repoChatsBaseForWorkspace(wsId)`-based implementation, exported, with its doc comment). Develop's change here was only "export this function," which HEAD's version already does — no work is lost. Develop's OLD implementation (`${workspaceBase(wsId)}/chats`) would 404 against the repo-scoped routes from Task 4.

- [ ] **Step 2: Resolve `agent-empty-document.tsx`**

Two hunks:

1. The `firstLineTop`/`FIRST_LINE_TOP` conflict: keep HEAD's **function** (`firstLineTop(headerClearancePx)`) and its **16px-based** math (`48 + headerClearancePx + 27.2`) — do NOT adopt develop's `14px`/`23.8` value. Restyling/v2's composer font-size is a value neither branch's OWN feature work depends on; develop's `14px` change is an unrelated tweak that would visibly shrink the composer text, and the user has explicitly asked to preserve the current restyle exactly. (Flagged, not silently dropped: if a smaller composer font was actually wanted, revisit this specific value deliberately later — it is a one-line, easily revisited change, not entangled with anything else.)
2. The button-row conflict: keep HEAD's outer `banner ? (...) : (...)` structure. Inside the `else` branch's `<div className="grp">`, insert develop's `{attachmentsReady && (<ComposerPlusButton .../>)}` block immediately before the `<button type="button" className={cn('send', ...)}>` element, exactly as develop had it (with its `onOpenExcalidraw`/`onOpenAttachFile` handlers unchanged). Confirm `attachmentsReady`, `wsId`, `chatId`, `loadExcalidrawDesign`, `parseExcalidrawScene`, `setExcalidrawInitialScene`, `setModal` are already in scope elsewhere in this file (they should be — they're used by develop's already-merged, non-conflicting code in the rest of this same file for the Excalidraw/attach-file modals).

- [ ] **Step 3: Resolve `composer.css`**

Keep HEAD's `padding-top: calc(48px + var(--agent-header-clearance, 0px));` line (restyling/v2's header-clearance feature) AND keep `font-size: 16px;` (do not take develop's `14px` — same reasoning as Step 2.1; these two values must move together, and we're keeping the 16px/27.2 pairing).

- [ ] **Step 4: Resolve `terminal-tab.tsx`**

Merge: use HEAD's renamed store (`windowPaneStore`, imported at the top of the file from `@/features/panes/stores/window-pane-store` — confirm it's already imported) with develop's real fix (strip the buffer from every pane's membership before closing it, since `closeBuffer` only tears down once no pane references the id):

```ts
    const state = windowPaneStore.getState()
    for (const pane of Object.values(state.panes)) {
      if (pane.bufferIds.includes(bufferId)) {
        state.paneActions.removeBufferFromPane(pane.id, bufferId)
      }
    }
    state.bufferActions.closeBuffer(bufferId)
  }, [bufferId])
```

(Deps array is `[bufferId]` only — `windowPaneStore` is a static module-level import, not a hook value, matching this file's own established convention elsewhere, e.g. its existing `windowPaneStore.getState().paneActions` call a few lines below.)

- [ ] **Step 5: Resolve `agent-chats-slice.ts`**

Merge all four imports (no actual collision — `TranscriptScrollPosition` appears identically on both sides):

```ts
import { chatReadMark } from '@/features/agent/lib/chat-read-order'
import { clearScrollPosition } from '@/features/agent/hooks/lib/transcript-scroll-positions'
import type { ParsedExcalidrawScene } from '@/features/agent/composer/plate/attachments/excalidraw-scene'
import type { TranscriptScrollPosition } from '@/features/agent/hooks/use-transcript-anchor'
```

- [ ] **Step 6: Typecheck**

Run: `cd web && bun tsc --noEmit` (do NOT use `bunx tsc` — different package, see project conventions)
Expected: no new errors from these 5 files (other files may still show errors until later tasks resolve their conflicts — that's expected here).

- [ ] **Step 7: Stage**

```bash
git add web/src/features/agent/api/agent-api.ts web/src/features/agent/chat/agent-empty-document.tsx web/src/features/agent/styles/composer.css web/src/features/terminal/components/terminal-tab.tsx web/src/features/workspace/stores/slices/agent-chats-slice.ts
```

---

### Task 7: Resolve web frontend — agent-chat-view.tsx (attachments DnD/asset-provider wrapping)

**Files:**
- Modify (resolve conflict): `web/src/features/agent/chat/agent-chat-view.tsx`

**Context:** Develop wraps the chat surface in `<DndScope><ChatMarkdownAssetProvider wsId={chatId} chatId={chatId}>...</ChatMarkdownAssetProvider></DndScope>` (needed for attachment drag-and-drop and markdown-embedded asset resolution) and adds a new `provider: string` prop (the EFFECTIVE provider, distinct from the existing `providerId` prop — confirmed additive, not a rename, from its own doc comment: "see its own doc above for why the two are never the same prop"). Restyling/v2 independently added `blankSignpost`/`onChatGone` props and a `headerClearanceStyle`/`headerClearancePx` mechanism plus a `ref={setChatSurfaceEl}` is develop's addition for drag-and-drop drop-zone measurement. All of these are independent, additive, and must all survive.

- [ ] **Step 1: Resolve the import conflict**

```ts
import type { KeyboardEvent, ReactNode, Ref } from 'react'
import { DndScope } from '@/features/agent/chat/dnd-scope'
```

- [ ] **Step 2: Resolve the props-interface conflict**

Keep HEAD's `blankSignpost?: ReactNode` and `onChatGone?: () => void` props with their doc comments, AND add develop's `provider: string` prop with its doc comment, immediately after (this is additive — the existing `providerId` prop stays untouched, wherever it's declared outside this conflict).

- [ ] **Step 3: Resolve the destructured-params conflict**

```ts
  blankSignpost,
  onChatGone,
  provider: effectiveProviderId,
```

- [ ] **Step 4: Resolve the three `<section>` wrapping conflicts**

For each of the three conflicting `<section className="agent-chat chat" ...>` blocks (transcript-only, empty-document, and populated-with-dock-height), wrap with develop's `<DndScope><ChatMarkdownAssetProvider wsId={wsId} chatId={chatId}>...</ChatMarkdownAssetProvider></DndScope>`, while preserving every one of HEAD's props/attributes on the `<section>` and its children exactly as HEAD had them:

- Transcript-only section: keep `style={headerClearanceStyle}`.
- Empty-document section: keep `style={headerClearanceStyle}`, and on the nested `<AgentEmptyDocument>`, keep every HEAD prop (`headerClearancePx`, `banner={blankSignpost}`, etc.) AND add develop's `wsId={wsId}` `chatId={chatId}` props (needed by Task 6 Step 2's `ComposerPlusButton`/Excalidraw wiring inside `AgentEmptyDocument`).
- Populated (dock-height) section: keep the `style={{ ...headerClearanceStyle, '--agent-dock-h': ..., '--agent-scrollbar-w': ... } as React.CSSProperties}` object (HEAD's spread of `headerClearanceStyle` merged with the existing dock/scrollbar CSS vars — do not drop the `...headerClearanceStyle` spread), and add develop's `ref={setChatSurfaceEl}` attribute on the `<section>`.

- [ ] **Step 5: Typecheck**

Run: `cd web && bun tsc --noEmit -p . 2>&1 | grep agent-chat-view`
Expected: no errors reported for this file.

- [ ] **Step 6: Stage**

```bash
git add web/src/features/agent/chat/agent-chat-view.tsx
```

---

### Task 8: Resolve web frontend — agent-chat-pane.tsx (the highest-risk file in this merge)

**Files:**
- Modify (resolve conflict): `web/src/features/agent/components/agent-chat-pane.tsx`

**Context:** Read this whole context block before touching the file — getting this wrong either breaks the pane-attach flow or silently drops the cancellation/hang fix. Confirmed via grep: `bufferId` is NOT a prop of this component any more (the prop is `paneId: string`), and `repointAgentChatBuffer` is a DELETED method — restyling/v2's own comment at (pre-merge) line 485 says so explicitly: `` `setPaneChat` (not the deleted `repointAgentChatBuffer`, which guarded on a buffer...) ``. So every one of develop's hunks that calls `s.bufferActions.repointAgentChatBuffer(bufferId, ...)` MUST be rewritten onto `windowPaneStore.getState().paneActions.setPaneChat(paneId, ...)` — this is not a style preference, the old call would not compile.

Conversely, `REVIVE_REQUEST_BOUND_MS`, `reviveInFlightByChatId`, and `revivesInFlight` (develop's cancellation/hang-prevention infrastructure for `revive()`) are ALREADY present and used elsewhere in this file's non-conflicting regions (confirmed: module-level consts and a `revivesInFlight = useRef(0)` plus later usages around an auto-revive effect already merged in cleanly) — so develop's entire cancellation architecture for `revive()` must be the base structure for that hunk; restyling/v2's addition (the "never ran" chat detection) gets woven into it, not the other way around.

- [ ] **Step 1: Resolve the `mountedRef` / `releaseDisplacementRef` conflict**

Purely additive — keep both:

```ts
  const mountedRef = useRef(true)
  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  const releaseDisplacementRef = useRef<(() => void) | null>(null)
  const beginDisplacement = useCallback((chatId: string) => {
    releaseDisplacementRef.current?.()
    releaseDisplacementRef.current = holdDisplacement(chatId)
  }, [])
  const endDisplacement = useCallback(() => {
    releaseDisplacementRef.current?.()
    releaseDisplacementRef.current = null
  }, [])
  useEffect(() => () => releaseDisplacementRef.current?.(), [])
```

(Keep both doc comments, HEAD's above `mountedRef`, develop's above `releaseDisplacementRef`.)

- [ ] **Step 2: Resolve the `adopt` conflict**

Resolve to:

```ts
  const adopt = useCallback(async (signal?: AbortSignal): Promise<boolean> => {
    const ticket = claimChatRead()
    const fetched = await getChat(wsId, shownChatId, signal)
    if (!mountedRef.current) return false
    const s = store.getState()
    if (acceptChatRead(wsId, fetched.id, ticket)) {
      s.upsertAgentChat(fetched, ticket)
      s.setAgentChatWorking(fetched.id, fetched.working === true)
    }
    const chat = store.getState().agentChats.chats.find((c) => c.id === fetched.id) ?? fetched
    if (!chat.liveRunnerId) return false
    windowPaneStore.getState().paneActions.setPaneChat(paneId, chat.id, chat.liveRunnerId)
    if (chat.terminalSessionId) seedAttach(wsId, chat.terminalSessionId)
    setAttachment({ state: 'attached', sessionId: chat.terminalSessionId || null })
    return true
  }, [store, wsId, paneId, shownChatId])
```

Keep HEAD's doc comment above the function (the "TWO RACERS" explanation) unchanged. This combines HEAD's ordering-ticket + `mountedRef` guard + `setPaneChat` with develop's `signal` parameter threaded through `getChat`.

- [ ] **Step 3: Resolve the `refreshChatWorking` opening + preceding comment**

Discard develop's `refreshGeneration` ref and its comment entirely (superseded — see below). Keep only HEAD's one-line comment and its ticket-based opening:

```ts
  // Ordered against every other single-chat read for the same reason adopt is.
  const refreshChatWorking = useCallback(async (): Promise<boolean> => {
    const ticket = claimChatRead()
    const fetched = await getChat(wsId, shownChatId)
```

This is safe because the rest of `refreshChatWorking`'s body (already non-conflicting, just below) already references `fetched`/`ticket`/`acceptChatRead` — confirming HEAD's naming is what the rest of the function is already written against. Develop's `refreshGeneration` counter guards the identical race with a weaker, function-local mechanism; the `claimChatRead`/`acceptChatRead` registry is the established, more general pattern already used by `adopt` above and by `use-workspace-agent-chats-stream.ts` (Task 9) — dropping develop's redundant local counter loses no coverage.

- [ ] **Step 4: Resolve the `revive` conflict**

This is develop's structure (cancellation/bound/registry) as the base, with restyling/v2's never-ran-chat branch woven in. Resolve to:

```ts
  // Failure is a FIRST-CLASS OUTCOME, not an edge: the backend refuses outright to resume
  // a chat with no recorded conversation ("no conversation to resume" — a CLI that died
  // before its session-start hook ever fired leaves one), and the CLI itself may be gone
  // from the PATH. Both land in `idle: failed`, which is the one place the Resume button
  // still appears. It never retries by itself.
  //
  // A chat with no `activeProviderId` has never had ANY runner placed on it — no CLI
  // ever reached its session-start hook, so there is no conversation on record at all.
  // resumeChat is REFUSED OUTRIGHT for it every time; switchProviderLocked's own doc
  // comment names this exact case as the one it already handles correctly, so that is
  // the call a never-run chat needs — an ordinary fresh spawn, not a resume.
  //
  // `externalSignal` lets a CALLER's own cleanup (the auto-revive effect below)
  // cancel a revive still in flight when it unmounts or re-runs — without it,
  // an effect firing this and unmounting moments later (the pane's buffer/tab
  // closing mid-resume) left the request running for up to the FULL bound,
  // still holding the daemon's per-chat spawn-gate mutex, with nothing on
  // screen left to show for it. Merged into the internal bound, not a
  // replacement for it: the timeout still fires even for a caller (the Resume
  // button) that passes none.
  const revive = useCallback(
    async (externalSignal?: AbortSignal) => {
      attemptedRef.current.add(shownChatId) // spend the budget BEFORE awaiting anything
      const neverRan = activeProviderId === ''
      const startProvider = neverRan ? providers.find((p) => p.enabled) : undefined
      const verb = neverRan ? 'start' : 'resume'
      setAttachment({
        state: 'reviving',
        message: neverRan ? 'Starting this chat…' : 'Resuming this chat…',
      })
      revivesInFlight.current += 1
      const abort = new AbortController()
      let boundFired = false
      const bound = setTimeout(() => {
        boundFired = true
        abort.abort()
      }, REVIVE_REQUEST_BOUND_MS)
      const forwardExternalAbort = () => abort.abort()
      externalSignal?.addEventListener('abort', forwardExternalAbort)
      const run = (async () => {
        try {
          if (neverRan) {
            if (!startProvider) throw new Error('No agent provider is enabled')
            await switchProvider(wsId, shownChatId, startProvider.id, abort.signal)
          } else {
            await resumeChat(wsId, shownChatId, abort.signal)
          }
          if (!(await adopt(abort.signal))) fail()
        } catch (err: unknown) {
          if (abort.signal.aborted && !boundFired) return
          fail()
          const name = providers.find((p) => p.id === chatProviderId)?.displayName || 'the agent'
          toastSpawnFailure(
            abort.signal.aborted ? new Error('The daemon did not answer the resume.') : err,
            name,
            verb,
          )
        } finally {
          clearTimeout(bound)
          externalSignal?.removeEventListener('abort', forwardExternalAbort)
          revivesInFlight.current -= 1
        }
      })()
      reviveInFlightByChatId.set(shownChatId, run)
      try {
        await run
      } finally {
        if (reviveInFlightByChatId.get(shownChatId) === run) {
          reviveInFlightByChatId.delete(shownChatId)
        }
      }
    },
    [wsId, shownChatId, adopt, fail, providers, chatProviderId, activeProviderId],
  )
```

Note the two things carried over from restyling/v2 into develop's structure: the `neverRan`/`startProvider`/`verb` computation (now placed before `setAttachment`, since the message depends on it), and `activeProviderId` added to the dependency array. Everything else is develop's cancellation architecture unchanged. Do NOT duplicate the "A CHAT THE LIST NEVER MENTIONS" effect that follows this function in develop's version — it is not part of any conflict marker, it merges in automatically right after.

- [ ] **Step 5: Typecheck**

Run: `cd web && bun tsc --noEmit -p . 2>&1 | grep agent-chat-pane`
Expected: no errors for this file. Pay particular attention to any complaint about `bufferId` or `repointAgentChatBuffer` still being referenced — that means a leftover conflict marker or a missed rewrite.

- [ ] **Step 6: Run this component's tests**

Run: `cd web && bun vitest run web/src/__tests__/features/agent/components/agent-chat-pane.test.tsx`
Expected: PASS. (This test file is itself one of the 43 conflicts — resolved in Task 11 — so do this step again after Task 11 if it's not done yet; for now confirm no crash/import errors.)

- [ ] **Step 7: Stage**

```bash
git add web/src/features/agent/components/agent-chat-pane.tsx
```

---

### Task 9: Resolve web frontend — use-transcript-anchor.ts (scroll-anchoring: highest animation risk)

**Files:**
- Modify (resolve conflict): `web/src/features/agent/hooks/use-transcript-anchor.ts`

**Context:** This is the transcript scroll-restore/eased-follow logic — read `[[feedback_preserve_all_web_animations]]`-equivalent care into this one. Three INDEPENDENT concerns are tangled in these 5 conflict hunks:

1. **The "eased mode arms too early" bug** — restyling/v2 and develop each fixed this independently, via different mechanisms. restyling/v2's fix (`ARM_QUIET_MS = 150`, a wall-clock debounce that RESETS on every call to `scheduleArm`) is a strict improvement over develop's fix (a fixed double-`requestAnimationFrame` wait): restyling/v2's own comment cites a measured settle burst spanning "~800ms / ~99 sampled frames," which a 2-frame wait cannot cover, while a resetting wall-clock debounce covers a burst of any length. **Take restyling/v2's mechanism for this concern.**
2. **The "pin the turn to the top while it's being answered" feature** (`pinnedTop`, `pinnedRow`, `pinShifted`, `pinGraceUntil`, `READER_INPUT_MS`, `PIN_SETTLE_GRACE_MS`) — entirely develop's, entirely new, unrelated to (1). **Keep in full.**
3. **The "don't ease a big reposition jump" size test** (`easeFrom`, comparing the gap to `el.clientHeight`) — entirely develop's, entirely new, unrelated to (1) and (2). **Keep in full.**

- [ ] **Step 1: Resolve the constant declarations (top of file)**

Keep restyling/v2's `ARM_QUIET_MS` constant and its doc comment. Keep develop's `READER_INPUT_MS` and `PIN_SETTLE_GRACE_MS` constants and their doc comments. All three coexist (they serve different purposes — none replace the others). Order: `ARM_QUIET_MS`, then `READER_INPUT_MS`, then `PIN_SETTLE_GRACE_MS` (matching each side's own internal order).

- [ ] **Step 2: Resolve the ref declarations**

Keep `const armTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)` (restyling/v2's — replaces develop's `armFrame`, since the arm mechanism itself is being replaced per concern 1 above). Keep every one of develop's OTHER ref declarations verbatim and in full, with their doc comments: `pinnedTop`, `pinnedRow`, `pinShifted`, `boxed`, `tabHidden`, `lastFromBottom`, `revealFromBottom`, `easeFrom`, `lastInputAt`, `pointerHeld`, `pinGraceUntil`. (Do NOT keep develop's `armFrame` ref — it's superseded by `armTimer`.)

- [ ] **Step 3: Resolve `scheduleArm`'s body**

```ts
      clearTimeout(armTimer.current)
      armTimer.current = setTimeout(() => {
        easedArmed.current = true
        revealFromBottom.current = null
      }, ARM_QUIET_MS)
```

(restyling/v2's timeout mechanism, carrying over develop's `revealFromBottom.current = null` side-effect from inside its old nested-RAF callback — that side-effect is still needed for the reveal-from-bottom feature elsewhere in this file.)

- [ ] **Step 4: Resolve the instant-vs-eased decision block**

Keep develop's structure in full (the `pinShifted` early-return, the size-test with `easeFrom`), and layer restyling/v2's no-op-write guard onto BOTH places that write `el.scrollTop = target` inside it:

```ts
      // The pinned row just moved because content above it changed height.
      // Land, do not glide — see `pinShifted`'s own note in `applyTailRoom`.
      if (pinShifted.current) {
        pinShifted.current = false
        if (el.scrollTop !== target) el.scrollTop = target
        return
      }
      // INSTANT while still settling (see
      // UseTranscriptAnchorOptions.loadingHistory), and instant for any gap
      // bigger than a viewport whether settling or not.
      //
      // (... keep develop's full "THE SIZE TEST IS THE LOAD-BEARING HALF" and
      // "MEASURED FROM WHERE THE CATCH-UP BEGAN" comment blocks unchanged ...)
      if (target - el.scrollTop <= 1) easeFrom.current = null
      const from = easeFrom.current ?? el.scrollTop
      if (!easedArmed.current || target - from > el.clientHeight) {
        if (el.scrollTop !== target) el.scrollTop = target
        easeFrom.current = null
        // (continue with whatever develop's block does after this line, unchanged)
```

- [ ] **Step 5: Resolve the cleanup/unmount effect**

```ts
      pinnedTop.current = null
      pinnedRow.current = null
      clearTimeout(armTimer.current)
```

- [ ] **Step 6: Typecheck**

Run: `cd web && bun tsc --noEmit -p . 2>&1 | grep use-transcript-anchor`
Expected: no errors. In particular confirm nothing still references `armFrame` (it was deliberately removed).

- [ ] **Step 7: Run this hook's tests**

Run: `cd web && bun vitest run` (targeted to the conflicted test file once Task 11 resolves it — for now, confirm no import/compile errors by running any currently-passing test that imports this hook).

- [ ] **Step 8: Stage**

```bash
git add web/src/features/agent/hooks/use-transcript-anchor.ts
```

---

### Task 10: Resolve web frontend — use-workspace-agent-chats-stream.ts

**Files:**
- Modify (resolve conflict): `web/src/features/workspace/stores/hooks/use-workspace-agent-chats-stream.ts`

**Context:** Both sides independently fixed the identical "a stale `refetchOne` resolves after a newer one and clobbers fresher state" race. restyling/v2 used `claimChatRead()`/`acceptChatRead(wsId, chatId, ticket)` — confirmed ALREADY used elsewhere in this exact file (line ~441, non-conflicting) for a different single-chat read path, making it the established, file-wide convention. Develop used a local `chatFetchSeq: Map<string, number>` scoped only to this one function. Take restyling/v2's approach for consistency; no coverage is lost since it's the more general mechanism guarding the identical race.

- [ ] **Step 1: Resolve the three conflict hunks**

Discard develop's `chatWrites` companion comment/counter changes and its `chatFetchSeq` Map entirely (do not introduce it). Keep HEAD's version throughout: `const ticket = claimChatRead()` before the fetch, and

```ts
        if (!acceptChatRead(wsId, chatId, ticket)) {
          return getOrCreateWorkspaceStore(wsId)
            .getState()
            .agentChats.chats.some((c) => c.id === chatId)
        }
        getOrCreateWorkspaceStore(wsId).getState().upsertAgentChat(chat, ticket)
```

after the fetch resolves. (Note: verify whether develop's surrounding, non-conflicting diff in this file changed anything else about `chatWrites` bookkeeping that HEAD's ticket path still needs to increment — read ~20 lines around the resolved hunk after resolving, and if `chatWrites++` was removed by taking HEAD's side where develop's still increments it elsewhere, confirm nothing else in the file expects `chatWrites` to have been bumped for this call path. If in doubt, keep whatever HEAD already does here unchanged — HEAD's own version of this exact function.)

- [ ] **Step 2: Typecheck**

Run: `cd web && bun tsc --noEmit -p . 2>&1 | grep use-workspace-agent-chats-stream`
Expected: no errors.

- [ ] **Step 3: Stage**

```bash
git add web/src/features/workspace/stores/hooks/use-workspace-agent-chats-stream.ts
```

---

### Task 11: Resolve the native context menu bridge (Rust + TS)

**Files:**
- Modify (resolve conflict): `web/src/lib/crowbar-bridge.ts`
- Modify (resolve conflict): `desktop/src-tauri/src/lib.rs`

**Context:** `web/src/components/ui/context-menu.tsx` and `block-context-menu.tsx` (develop's actual native-menu component rewrite) were NEVER touched by restyling/v2 and are not in the conflict list — they already applied cleanly. Only the bridge file and the Tauri command registration conflict, and both conflicts here are pure "two independent additions landed at the same insertion point" — nothing to reconcile beyond keeping both.

- [ ] **Step 1: Resolve `crowbar-bridge.ts`'s import conflict**

Drop `workspaceBase` entirely (confirmed unused elsewhere in this file — restyling/v2 already removed it as part of its repo-scoped-routing refactor). Add develop's new type import:

```ts
import type { ContextMenuItem } from '@/components/ui/context-menu'
```

- [ ] **Step 2: Resolve `crowbar-bridge.ts`'s function-block conflict**

Keep both blocks, in either order (no dependency between them) — HEAD's `setTrafficLightPosition`, then develop's `toNativeMenuEntries`/`showNativeContextMenu` section (its imports — `Menu`, `MenuItemOptions`, `SubmenuOptions`, `PredefinedMenuItemOptions`, `tauriInvoke`, `isTauri` — are already present in this file's non-conflicting top-of-file imports).

- [ ] **Step 3: Resolve `lib.rs`'s two conflicts**

Same pattern — keep both new functions (HEAD's `traffic_light_container_frame`/`set_traffic_light_position`, develop's `popup_native_context_menu`), and in the `tauri::generate_handler!` macro's command list, include BOTH `set_traffic_light_position` and `popup_native_context_menu`.

- [ ] **Step 4: Build both**

Run: `cd web && bun tsc --noEmit -p . 2>&1 | grep crowbar-bridge`
Run: `cd desktop/src-tauri && cargo check`
Expected: both clean.

- [ ] **Step 5: Stage**

```bash
git add web/src/lib/crowbar-bridge.ts desktop/src-tauri/src/lib.rs
```

---

### Task 12: Resolve mechanical files — package.json, bun.lock, Cargo.lock

**Files:**
- Modify (resolve conflict): `web/package.json`
- Modify (resolve conflict): `web/bun.lock`
- Modify (resolve conflict, if still present): `desktop/src-tauri/Cargo.lock`

**Context:** `package.json`'s conflict is a genuine restyle decision (restyling/v2 swapped the `jetbrains-mono` font packages for `geist-mono`, confirmed: HEAD's `web/package.json` has ONLY `geist-mono`, no `jetbrains-mono`, anywhere) collided with develop's unrelated addition of `@excalidraw/excalidraw` (needed for the attachments feature — confirmed used by `web/src/features/agent/composer/plate/attachments/excalidraw-scene.ts` and `excalidraw-preview.tsx`, both of which merged in cleanly and are not in the conflict list).

- [ ] **Step 1: Resolve `package.json`**

Keep restyling/v2's font swap (only `@fontsource-variable/geist-mono` and `@fontsource/geist-mono`, no jetbrains-mono packages), and add develop's new dependency alphabetically:

```json
    "@excalidraw/excalidraw": "^0.18.1",
    "@fontsource-variable/geist-mono": "^5.3.0",
    "@fontsource/geist-mono": "^5.3.0",
```

- [ ] **Step 2: Regenerate the lockfile — do not hand-merge it**

```bash
cd web && rm -f bun.lock && bun install
```

Confirm `git diff --stat web/bun.lock` shows a real, non-empty diff (the lockfile regenerated) and `git status` no longer shows it as conflicted.

- [ ] **Step 3: Check Cargo.lock**

```bash
cd desktop/src-tauri && git diff --name-only --diff-filter=U | grep Cargo.lock
```

If still conflicted: `cargo check` (from Task 11) should have already regenerated a valid lockfile as a side effect once `Cargo.toml`'s own (non-conflicting) dependency bumps from PR #168 landed — if the conflict markers are still literally present in the file, run `rm Cargo.lock && cargo generate-lockfile` instead, then re-run `cargo check` to confirm it still builds.

- [ ] **Step 4: Stage**

```bash
git add web/package.json web/bun.lock desktop/src-tauri/Cargo.lock
```

---

### Task 13: Resolve the remaining conflicted test files + the deleted-test DU cases

**Files:**
- Modify (resolve conflict): `web/src/__tests__/features/agent/api/agent-api.test.ts`
- Modify (resolve conflict): `web/src/__tests__/features/agent/chat/agent-chat-view.test.tsx`
- Modify (resolve conflict): `web/src/__tests__/features/agent/components/agent-chat-pane.test.tsx`
- Modify (resolve conflict): `web/src/__tests__/features/agent/hooks/use-transcript-anchor.test.tsx`
- Modify (resolve conflict): `web/src/__tests__/features/panes/components/new-tab-view.test.tsx`
- Modify (resolve conflict): `web/src/__tests__/features/workspace/stores/hooks/use-workspace-agent-chats-stream.test.ts`
- Delete (confirm): `web/src/__tests__/features/agent/tree/agent-chats-panel-perf.test.tsx`, `agent-chats-panel-rerender.test.tsx`, `agent-chats-panel.test.tsx`, `hooks/use-agent-chat-folders.test.tsx`, `lib/chat-removal.test.ts`, `lib/chat-tree-commit.test.ts`
- Delete (confirm, or port coverage): `web/src/__tests__/features/workspace/stores/slices/buffer-slice-new-tab.test.ts`, `buffer-slice.test.ts`
- Port onto the current `paneId` model (discovered during Task 8's review — these merged in cleanly, never conflicted, so nothing else in this plan covers them, and they currently do not compile): `web/src/__tests__/features/agent/components/agent-chat-pane-resume-wedge.test.tsx`, `agent-chat-pane-multi-pane-resume-race.test.tsx`, `agent-chat-pane-sibling-displacement.test.tsx`, `agent-chat-pane-unknown-chat-wedge.test.tsx`

- [ ] **Step 0: Port the 4 orphaned `agent-chat-pane-*` regression tests**

These are develop's ONLY regression coverage for the exact revive/resume/cancellation hang-fix architecture Task 8 merged (`REVIVE_REQUEST_BOUND_MS`, `reviveInFlightByChatId`, `AbortController`-based cancellation, sibling-pane displacement). Each currently mounts `AgentChatPane` with a `bufferId` prop (no `paneId`) and drives it through `store.getState().bufferActions.openContent(...)` / reads `s.buffers` — the pre-refactor buffer model. Confirmed broken: `bufferId` doesn't exist on `AgentChatPaneProps` any more (the prop is `paneId`), `bufferActions`/`buffers` don't exist on `WorkspaceState`, and they import a removed `AgentChatContent` export.

For each of the 4 files: read it in full to understand what scenario it's proving (the wedge/race/displacement bug it's named for), then rewrite its setup to use `paneId` and `windowPaneStore`'s `paneActions.setPaneChat`/pane-creation calls — following whatever pattern `web/src/__tests__/features/agent/components/agent-chat-pane.test.tsx` (Task 13 Step 1, already resolved by the time you reach this) uses to mount `AgentChatPane` under the current pane model. Do not weaken or skip the actual assertion each test makes (e.g. "the daemon never answers within the bound" or "a sibling pane's revive is not clobbered") — only the harness/mounting code should change, not what's being proven. If a scenario genuinely cannot be expressed under the current model (the feature it exercised no longer exists in that shape), say so explicitly in your report rather than deleting the file silently.

- [ ] **Step 1: Resolve the 6 production-code test files**

For each of `agent-api.test.ts`, `agent-chat-view.test.tsx`, `agent-chat-pane.test.tsx`, `use-transcript-anchor.test.tsx`, `new-tab-view.test.tsx`, `use-workspace-agent-chats-stream.test.ts`: resolve their conflicts to match whatever Tasks 6–10 decided for the corresponding production file (e.g. `agent-chat-pane.test.tsx`'s assertions about `bufferId`/`repointAgentChatBuffer` must be updated to `paneId`/`setPaneChat` if develop's version of the test still references the old shape — the file list overlap at `web/src/__tests__/features/agent/components/agent-chat-pane.test.tsx:318` already shows a comment referencing `repointAgentChatBuffer`, so check this specifically). Add test coverage for whichever NEW behavior a task above introduced that the pre-existing tests don't already cover (e.g. Task 8's `neverRan`/`switchProvider` branch in `revive()`, Task 9's `ARM_QUIET_MS` debounce behavior) if such coverage doesn't already exist in one of the two sides' test content.

- [ ] **Step 2: Confirm the `agent/tree/*` test deletions**

Confirmed via `git log`: `f119a402b refactor(sidebar): retire the workspace tree and the chats panel` deleted these on restyling/v2's side, and the whole "Chats panel tree" component these tests exercised no longer exists (superseded by the current sidebar architecture — see `[[project_view_architecture_foundation_validated]]`). Develop's changes to these 6 files were fixes to a component that has been fully retired. Confirm (`grep -rn "AgentChatsPanel\|useAgentChatFolders" web/src/features/` should return nothing under the old `tree/` location) and leave deleted: `git rm` each of the 6 files.

- [ ] **Step 3: Confirm or port the `buffer-slice*.test.ts` deletions**

`web/src/__tests__/features/panes/stores/slices/buffer-slice.test.ts` already exists at the NEW location (confirmed on disk) — restyling/v2 already moved these tests when it hoisted pane/buffer state into the window-level store (`bed7f3ede`). Diff develop's version of the OLD file against its own fork-point copy to see what, if anything, develop added or changed since the fork:

```bash
git diff 0043620fc origin/develop -- web/src/__tests__/features/workspace/stores/slices/buffer-slice.test.ts web/src/__tests__/features/workspace/stores/slices/buffer-slice-new-tab.test.ts
```

If develop only modified pre-existing assertions to match its own now-superseded code shape, no porting is needed — `git rm` both files. If develop added a genuinely new test case, port that specific case's intent into `web/src/__tests__/features/panes/stores/slices/buffer-slice.test.ts` (or `buffer-slice-eviction.test.ts`, whichever new-location file covers that behavior), adapted to the current store shape, then `git rm` the two old files.

- [ ] **Step 4: Run every test file touched in this task**

Run: `cd web && bun vitest run web/src/__tests__/features/agent/api/agent-api.test.ts web/src/__tests__/features/agent/chat/agent-chat-view.test.tsx web/src/__tests__/features/agent/components/agent-chat-pane.test.tsx web/src/__tests__/features/agent/hooks/use-transcript-anchor.test.tsx web/src/__tests__/features/panes/components/new-tab-view.test.tsx web/src/__tests__/features/workspace/stores/hooks/use-workspace-agent-chats-stream.test.ts web/src/__tests__/features/panes/stores/slices/buffer-slice.test.ts web/src/__tests__/features/agent/components/agent-chat-pane-resume-wedge.test.tsx web/src/__tests__/features/agent/components/agent-chat-pane-multi-pane-resume-race.test.tsx web/src/__tests__/features/agent/components/agent-chat-pane-sibling-displacement.test.tsx web/src/__tests__/features/agent/components/agent-chat-pane-unknown-chat-wedge.test.tsx`
Expected: all PASS.

- [ ] **Step 5: Stage**

```bash
git add web/src/__tests__/features/agent/api/agent-api.test.ts web/src/__tests__/features/agent/chat/agent-chat-view.test.tsx web/src/__tests__/features/agent/components/agent-chat-pane.test.tsx web/src/__tests__/features/agent/hooks/use-transcript-anchor.test.tsx web/src/__tests__/features/panes/components/new-tab-view.test.tsx web/src/__tests__/features/workspace/stores/hooks/use-workspace-agent-chats-stream.test.ts
git add web/src/__tests__/features/agent/components/agent-chat-pane-resume-wedge.test.tsx web/src/__tests__/features/agent/components/agent-chat-pane-multi-pane-resume-race.test.tsx web/src/__tests__/features/agent/components/agent-chat-pane-sibling-displacement.test.tsx web/src/__tests__/features/agent/components/agent-chat-pane-unknown-chat-wedge.test.tsx
git rm web/src/__tests__/features/agent/tree/agent-chats-panel-perf.test.tsx web/src/__tests__/features/agent/tree/agent-chats-panel-rerender.test.tsx web/src/__tests__/features/agent/tree/agent-chats-panel.test.tsx web/src/__tests__/features/agent/tree/hooks/use-agent-chat-folders.test.tsx web/src/__tests__/features/agent/tree/lib/chat-removal.test.ts web/src/__tests__/features/agent/tree/lib/chat-tree-commit.test.ts
git rm web/src/__tests__/features/workspace/stores/slices/buffer-slice-new-tab.test.ts web/src/__tests__/features/workspace/stores/slices/buffer-slice.test.ts
```

---

### Task 14: Full verification and the merge commit

**Files:** none (verification + the single merge commit)

- [ ] **Step 1: Confirm zero remaining conflicts**

```bash
git diff --name-only --diff-filter=U
```

Expected: empty output. If anything remains, stop and resolve it following this plan's methodology (base-vs-ours-vs-theirs, check for downstream dependents before assuming either side wins) before proceeding — do not force-resolve with `--ours`/`--theirs`.

- [ ] **Step 2: Full Go build**

```bash
cd api && go build ./...
```
Expected: clean.

- [ ] **Step 3: Full Rust build**

```bash
cd desktop/src-tauri && cargo build
```
Expected: clean.

- [ ] **Step 4: Full TS typecheck**

```bash
cd web && bun tsc --noEmit
```
Expected: clean.

- [ ] **Step 5: Targeted test sweep**

Run every test file this merge touched (union of Tasks 3, 5, 8, 9, 10, 13's test commands) plus the Go packages from Tasks 1, 2, 4. Do NOT run the full `vitest run` or full `go test ./...` — per this repo's standing rule, targeted/modified-file tests only.

- [ ] **Step 6: Conclude the merge**

```bash
git status  # confirm nothing unstaged/untracked that shouldn't be
git commit
```

Use the default merge commit message git prepares (listing `origin/develop` and summarizing conflicts) — do not rewrite it into a squash-style message; this is a merge commit and should read as one. Append the standard attribution line as usual.

- [ ] **Step 7: Restore the repo's default merge conflict style**

```bash
git config --unset merge.conflictstyle
```
(This was set to `diff3` only to make the conflicts in this plan easier to read — revert it now that the merge is committed, so it doesn't silently change behavior for the next unrelated merge in this repo.)

---

### Task 15: Live verification in `make dev-desktop`

**Files:** none (manual verification pass — per project convention, live-verify in the real Tauri app, not headless)

- [ ] **Step 1: Launch**

```bash
make dev-desktop
```

- [ ] **Step 2: Native context menus**

Right-click every surface that used to render a React context menu (sidebar rows, chat messages, editor, terminal). Confirm each now shows the OS-native menu (this can't be checked by automated DOM tooling — native menus aren't in the element tree).

- [ ] **Step 3: Restyle integrity**

Open the composer and empty-document view; confirm the font size, header clearance, and every other restyled visual is pixel-identical to how it looked before this merge (this is what Task 6 Steps 2–3 specifically protected).

- [ ] **Step 4: Chat attachments**

Attach an image and a CSV, open the Excalidraw takeover from the new `ComposerPlusButton` (Task 6 Step 2 / Task 7), confirm both round-trip.

- [ ] **Step 5: Transcript scroll behavior**

Send a prompt long enough to trigger a multi-frame settle burst; confirm the transcript does not visibly jump/snap (Task 9's `ARM_QUIET_MS` debounce), and that the turn pins to the top while it's being answered (Task 9's `pinTurnToTop` feature) without any snapping — a state change that snaps is a defect, not a passable regression.

- [ ] **Step 6: Provider switch + subagent tracking**

Start a chat, delegate to a subagent, and switch providers mid-conversation; confirm no duplicate/lost messages and that the subagent shelf reflects reality (Tasks 1, 8).

- [ ] **Step 7: Terminal attach/close**

Open a chat's native terminal view in two panes, close one; confirm the PTY correctly tears down only once no pane still references it (Task 6 Step 4).

- [ ] **Step 8: Report**

Summarize pass/fail for each of the above to the user before considering this merge done. Any fail is a real regression from this merge and must be root-caused and fixed, not noted as a caveat.
