# Agent Surface Hardening — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development.

**Goal:** Take the agent capability surface from "works" to production quality: make it fast, bounded, switchable per provider, and properly attributed in the UI.

**Priority order set by the owner:** performance is the single most important thing.

**Prior work:** `docs/superpowers/specs/2026-07-29-agent-capability-surface-design.md` and `docs/superpowers/plans/2026-07-29-agent-capability-surface.md` (complete, merged into this branch).

## Global Constraints

- MCP protocol revision exactly `2025-11-25`.
- Crowbar never writes into a provider's home directory.
- **No provider-specific code anywhere** — provider identity is data (descriptor `display_name`/`icon`, surfaced via `AgentProviderDTO`). The FE resolves by id from the providers list; no `if provider == "claude"` in Go or TS.
- No tool accepts a workspace/project/repo/runner/segment argument.
- Never declare `outputSchema`.
- Tool ceiling stays 8.
- Errors wrap `fmt.Errorf("<pkg>: <action>: %w", err)`; comments explain *why*.
- Go tests beside the code; **web tests in `web/src/__tests__/` mirroring `web/src/`**; component files kebab-case; `@/` imports in tests.
- Stores: narrow selectors, `getState()` only in handlers/effects, stores never import from `components/`.
- **No timing in tests.** No sleeps, no polling, no `Eventually`.
- Commit per task. Do NOT push. Do NOT open a PR.

## Key facts established by investigation

| fact | where |
|---|---|
| `get_review_scope` = ~10 git spawns; ~6 are duplicate `merge-base` | `branch_review.go:172` + `branch_review_files.go:74` both call `resolveDiffRef` |
| `GetFiles` has singleflight but **no result cache** | `branch_review_files.go:31` |
| 3 × `workspaces.Get` aggregate folds per tool call | `scope.go:117`, `branch_review.go:168`, `branch_review_files.go:70` |
| `workspaces.List` full scan on **every** MCP call | `scope.go:121` |
| `list_workspaces` = O(V) full chat-table scans | `tools_context.go:79` → `agentchat/internal/store/store.go:171` |
| `agent_chats_read` already has `workspace_id` **indexed** | `agentchat/internal/store/storage.go:20` |
| `AgentProviderPreference` is plain GORM; new column auto-migrates | `agent_provider_preference.go:7`, `sqlite.go:103` |
| Review threads are event-sourced but persisted as **JSON blobs** — new fields are additive, no migration | `reviewthread/internal/store/storage.go:14` |
| `Caller` already carries `ProviderID` and `ChatID` | `agenttools/scope.go:48` |
| `openAgentChat(store, wsId, chatId)` already does focus-if-open-else-open | `web/src/features/agent/lib/open-agent-chat.ts:16` |
| `ProviderIcon({svg, className})` exists; providers live at `s.agentChats.providers` | `provider-icon.tsx:15`, `agent-chats-slice.ts:51` |
| `review-thread-item.tsx` declares `wsId` but never uses it | `review-thread-item.tsx:40` |

---

# Phase A — Performance

Owner's stated priority. **Measure first**; every fix in this phase must show a before/after number.

### Task A1: A benchmark that counts git subprocesses and wall time

**Files:** create `api/internal/app/usecases/agenttools/perf_test.go`; touch `api/internal/engine/git/internal/exec/exec.go` only if a spawn counter needs a test hook.

Build a benchmark/test that, against a real temp git repo with a realistic branch (say 40 files, 200 hunks), reports for `get_review_scope` and `list_workspaces`: wall time, git subprocess count, and `workspaces.Get`/`ListChats` call counts. Counting is the point — wall time alone will not show the duplicate `merge-base`.

Prefer counting via the existing `perf` ring buffer (`api/internal/perf/`) if it can be enabled in-process; otherwise a test-only counter on the exec seam. **Record the baseline numbers in the report — every later task in this phase quotes them.**

### Task A2: Stop resolving the same diff ref twice

`get_review_scope` calls `GetBase` then `GetFiles("")`, and each independently runs `resolveDiffRef` (~3 `merge-base` spawns each). Collapse to one.

Preferred shape: give `branchreview` an exported method that returns base **and** files together in one resolution — the tool wants exactly that pair. Alternatively memoise `resolveDiffRef` per `(repoPath, branch tips)` with the same immutable-key discipline `cacheableOutlineKey` uses (`branch_review_outline.go:78`) — note `resolveDiffRef` is **not pure** (it depends on branch tips), so a naive cache is wrong.

Keep `GetBase` on the interface; the agent surface may use the combined call.

### Task A3: `list_workspaces` — one scan, not V

`tools_context.go:79` calls `ChatReads.ListByWorkspace` per visible workspace, and each is a full table scan plus a JSON unmarshal per row. Hoist a single `ListChats()` outside the loop and bucket by `WorkspaceID` in Go. That is local to the tool and needs no repository change.

If you also push the predicate into SQL (`agent_chats_read.workspace_id` is already indexed), do it as a **separate commit** so it can be reverted independently.

### Task A4: Collapse redundant workspace reads

Three `workspaces.Get` folds per tool call, each replaying the event log and each firing `reconciler.OnOpen`. And `Resolve` does a full `workspaces.List` on **every** MCP call, including `set_chat_title` which needs no workspace tree at all.

Two independent wins:
- Resolve the caller's workspace once and pass it down rather than re-fetching.
- Make `Caller.Visible` **lazy** — compute the visible set only when a tool actually reads it. `set_chat_title`, `post_review_comment`, `reply_*` and `resolve_*` need `CanSee` or the caller's own workspace, not the full list; only `list_workspaces` and `get_chat_log` need the tree. Keep `CanSee` correct either way — this is an authority path, so a lazy miss must fail closed.

### Task A5: Re-measure and report

Re-run A1's benchmark. Report before/after for both tools. If any fix did not move its number, say so — a perf change that does not show up is not a perf change.

---

# Phase B — Bounded results

### Task B1: Cap and paginate

Design §9 says *"Replies are capped and lists paginate."* Neither is implemented.

- `list_review_threads`: cap messages rendered per thread (keep the root plus the most recent N, and state how many were elided), and paginate the thread list with an explicit `offset`/`limit` in the schema. Say in the rendered text when results were truncated — silent truncation reads as "that's all there is".
- `get_chat_log`: cap the returned turns, most recent first, with the same explicit statement.
- `get_review_scope`: cap the changed-file list.

Pick limits that keep a full result comfortably inside a model turn on **codex**, where tool schemas are not deferred and context is scarcest. State the chosen numbers and the reasoning in the report.

---

# Phase C — Per-provider MCP toggle

### Task C1: The preference and its wire

Add `MCPDisabled bool` to `domain.AgentProviderPreference` — **negative polarity, so the zero value means enabled** and existing rows do not silently lose MCP when GORM adds the column. Mirror the existing `Disabled` treatment exactly: DB stores `MCPDisabled`, the wire exposes `MCPEnabled` on `AgentProviderDTO`, and the PUT body takes `mcpDisabled` (`providers.go:40`). Set it in `ResolveProviders` (`agent.go:2185`). `ReplaceProviderPreferences` needs no change.

### Task C2: Make the injection conditional

Add a named `mcp_injection: []InjectStep` field to the descriptor, parallel to the existing `ContextInject`/`ResumeContextInject` pair, and move the MCP registration steps out of `config_injection` into it in both descriptors. Append it in `spawnRunner` only when the provider's MCP flag is on — the exact shape `contextInject(d, resuming)` already uses (`agent.go:2102`, appended at `:884`).

**Do not filter `config_injection` by template token.** `{runner_token}` appears in only 2 of codex's 5 MCP steps; `mcp_servers.crowbar.command`, `.env_vars` and `default_tools_approval_mode` carry no token and would survive, registering a half-configured server. `Descriptor.Validate` must not require the new field, so a third-party descriptor omitting it is a no-op.

Add a guard mirroring `requireProviderEnabled` (`agent.go:2116`). Tests: with the flag off, the rendered argv contains **no** MCP registration at all for either provider, and the CLI still spawns and its hooks still fire.

### Task C3: The toggle in the Providers table

`sortable-provider-row.tsx` — insert a second `Switch` between the connected dot (`:63`) and the existing enable switch (`:65`), labelled for MCP/tools. Thread the new flag through `AgentProvider` (`agent-api.ts:54`), `mapProvider`'s defaults (`:102`), `ProviderPreference` (`:68`), and all four pure mappers in `provider-preferences.ts`. Route it through the single `commit(orderedIds, disabledById)` funnel in `providers-settings.tsx:93` — do **not** add a second call site. Update the legend copy at `:191-194`.

Tests in `web/src/__tests__/features/settings/`, mirroring the existing provider-settings tests.

---

# Phase D — Agent attribution in review threads

### Task D1: Carry provider and chat on a review message

Add `ProviderID` and `ChatID` (both `omitempty`) to `domain.ReviewMessage`. Thread them through `reviewthread.OpenInput`, the `Reply` signature (it already takes 7 positional args — **convert to a `ReplyInput` struct**), and the `OpenReviewThread` / `ReplyReviewThread` commands.

No migration: both persistence surfaces are JSON blobs. Historical messages read empty forever — the UI must degrade gracefully to today's behaviour when they are absent.

`agenttools` already has both values on `Caller`; populate them at `openInputFor` (`tools_review.go:300`) and the reply site (`:386`). The human/UI path passes empty.

### Task D2: The wire

Both DTOs: `ReviewMessageDTO` (`dto/review.go:10`) and `ThreadDTO`/`ThreadReplyDTO` (`dto/thread.go:12,30`) — remembering the root message is flattened onto `ThreadDTO` itself, so the fields are needed at both root and reply level.

### Task D3: The UI

`review-thread-item.tsx`:
- Replace the hard-coded `{ name: 'Agent' }` in `resolveAuthorDisplay` (`:72`) with the provider's **`displayName` and `icon`**, resolved by id from the providers list — the established idiom is `agent-chat-tab-icon.tsx:27-31`. Use `ProviderIcon`. **No provider-specific code**; an unknown or absent id falls back to today's generic agent rendering.
- Show the **originating chat's title** on the message, resolved from `s.agentChats.chats` by `chatId`, falling back to `UNTITLED_CHAT_LABEL`.
- Make that title activate the chat via `openAgentChat(store, wsId, chatId)` — which already focuses an open tab or opens a new one. `wsId` is already a prop and currently unused (`:40`).

Tests in `web/src/__tests__/features/git/`: provider name+icon render, the fallback when `providerId` is absent, and that clicking the chat title calls `openAgentChat` with the right ids.

---

# Phase E — Correctness gaps

### Task E1: Idempotency on `reply_to_review_thread`

`post_review_comment` has an idempotency key; reply does not, and there is a concrete trigger — `writeLine` can fail on a broken pipe *after* the daemon committed the reply, the relay keeps serving, and the client retries into a duplicate. Add the same optional `idempotencyKey`, reusing the existing `Idempotency` type. Test the retry collapses.

### Task E2: The multi-hunk anchor test

`anchorInAnyHunk` requires the whole range inside **one** hunk, and the only fixture exercised so far is one file / one hunk — deliberately easy mode. Build a multi-hunk fixture and cover: an anchor spanning two hunks (rejected), an anchor on a function signature sitting in unchanged context outside the nearest hunk (rejected), and a valid anchor in the second hunk (accepted).

**Report what the rejection message actually says**, because a model has to recover from it. If the message does not make the legal move obvious, that is a finding worth raising before shipping.

---

# Phase F — Real-model coverage

### Task F1: Drive the three untested tools

`resolve_review_thread`, `list_workspaces` and `get_chat_log` have never been touched by a real model. Add integration tests under `api/tests/integration/agent/` following the existing `agent_mcp_test.go` patterns — `mcpBarrier` for traffic assertions, `drive` for the PTY, `kit.Await` for waits, **no timing**.

Cover: an agent resolving a thread whose finding it addressed; an agent listing workspaces in a hierarchy and reading a sibling chat's log. Assert on **MCP traffic**, not just end state.

These make real model calls. Write exactly what is needed, no retry loops. **Report what the model actually did** — especially whether `resolve_review_thread` is used judiciously, since that is the one verb whose correct use is pure judgement.
