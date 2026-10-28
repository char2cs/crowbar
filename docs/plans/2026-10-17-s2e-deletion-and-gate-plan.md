# S2e — Delete the old read shapes, hygiene pass, gate: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The daemon stops serving what only the old client read: S1's LRU transcript projector, its endpoints and the lifecycle event frames (T11). The files of `features/agent` that no earlier task touched follow the comment rule (T12). The S2 gate is measured and checked live in the desktop app (T13).

**Architecture:** See the index, `docs/plans/2026-10-17-s2-chatstore-plan.md` (R1, R14, R18, R19, R23, R25, R28, gate baseline). T11 requires T1–T5 and runs right after T5 (P2, index Task order); T12 and T13 require T1–T10.

**Tech Stack:** Go, gin, testify, `api/tests`; web toolchain; the isolated noVNC container and its browser tools for the live check (no Tauri MCP in this environment; S3 OR6, S4 OR4).

**Spec:** S1 R13 ("S2 deletes them"); architecture §11 row S2 (gate); additions §5 (budgets).

Before any build: free disk on the worktree's filesystem (`df -h .`); under 8 GB free, stop and report. Go tests always run with `T=$(mktemp -d)`, `HOME=$T/home CROWBAR_HOME=` (index, Global Constraints).

---

### Task 11: Delete the daemon's R13 read projection

**Goal:** The ledger is read only through the feed, the blob route, and the two server-side readers that need text (`get_chat_log`, the handoff), which fold rows directly with `ledger.Messages`. The LRU projector, `/messages`, `/activity`, `/activity/:toolId/payload`, the `/choices` GET, `/pending-prompt`, the activity components and the lifecycle `message_delta`, `plan`, `choice`, `compaction_started`, `compaction_stopped`, `prompt_settled` frames are deleted, with the chat DTO's `session`.

**Files:**
- Move first (pre-flight F4): the transcript package is not only the old client's plumbing. Its open-ask fold (`choices.go` `PendingChoicesOf` over `ledger.State`) and `PendingChoices` feed kept owners: `AnswerChoice` (`usecases/chat/answers.go:200-214`), `PendingChoiceSets`/`ReadPendingChoices` (`turn.go`), termwait (`runner/internal/termwait/evaluate.go:45`, `termwait.go:25`; R20 keeps termwait) and the answer desk (`shared/answerdesk/decide.go:17`, `answerdesk.go:130`), all on `domain.ActivityChoice`. Move the fold to a kept owner, `State.OpenAsks()` in `api/internal/domain/ledger` (beside `ledger.MessageFold`), and keep `domain.ActivityChoice` as the answer desk's, termwait's and `AnswerChoice`'s shape (replacing it with `ledger.Ask` is not in S2). `broadcastChoices` (`app/container.go:268`) and `seam.Choices` (`shared/seam/seam.go:55`) only publish the `choice` frame, so they go with it
- Delete: `api/internal/app/usecases/chat/internal/transcript/` (whole package and tests) and its users: the projector embed and the `MessageDelta`/choice publication in `api/internal/app/usecases/chat/ledger_feed.go` (`u.transcript.Apply`, `transcript.PendingChoicesOf`), its wiring in `assembly.go`, its use in `turn.go` (`ReadActivity`, `ToolPayload`; `ReadPendingChoices` reads `State.OpenAsks()`), `answers.go` (reads `State.OpenAsks()`), `broadcastChoices` in `app/container.go` and `Choices` in `shared/seam/seam.go`; entity frames (`working`, `terminal_wait`, `telemetry`, `worktree_state`) stay
- Modify: `api/internal/app/usecases/chat/internal/conversation/chatlog.go` and its test — today it reads `c.transcript.ChatTurns`/`c.transcript.Messages` (`:27`, `:96`); read the ledger (`Store.Read(ctx, chatID, after, limit)`, paged) and fold with the domain's message fold (`ledger.MessageFold`). `handoff.go` does not read the transcript and is unchanged
- Modify: `api/internal/app/usecases/chat/internal/conversation/conversation.go` — drop the `Transcript` interface, its dependency field and `c.transcript.Forget` (`:58-75`, `:105`, `:126`, `:232`)
- Delete: `api/internal/api/v0/endpoints/chat/handlers/{activity.go,activity_test.go,activity_components.go,pendingprompt.go,pendingprompt_test.go}`; `Messages`, `ToolPayload` and `Choices` (GET) from `handlers/chats.go` (594 lines; S3 T5 splits it into `chats_create.go`/`chats_read.go`/`chats_mutate.go`: if that has landed, `Messages`/`ToolPayload` go from the read part) and `handlers/answers.go` (keep `AnswerChoice`) and their tests
- Modify: `api/internal/api/v0/endpoints/chat/routes.go`, `api/internal/api/v0/endpoints/home/routes.go` and their tests — drop the five GET routes (`/messages`, `/activity`, `/activity/:toolId/payload`, `/choices`, `/pending-prompt`) in both mounts; `api/internal/api/v0/route_audit_test.go` (integration tag; the read surface at `:427`, the home table at `:541`) drops them too
- Modify: `api/internal/api/v0/dto/agent.go`, `agent_activity.go`, `agent_choice_frame.go`, `agent_event.go` (and their tests) — delete the activity, tool-call, subagent, interruption, choice-read, component, pending-prompt and ledger-page DTOs, the `AgentChatKind{MessageDelta,Plan,Choice,CompactionStarted,CompactionStopped,PromptSettled}` kinds and the frame fields only those frames carried, and the chat DTO's `session`
- Modify: the publication code that emits the six lifecycle event frames (find with `grep -rn 'AgentChatKind\(MessageDelta\|Plan\|Choice\|Compaction\|PromptSettled\)' api/internal`) — delete those emissions; entity frames stay
- Delete: the `domain` read shapes only the projector returned (S1 T7 kept `domain.Activity*` "until S2"; `scripts/deadcode-check.sh` lists the rest)
- Modify: `web/src/lib/ws/chat-frame-kinds.ts` — drop the six deleted kinds (`moved` went in T4); `web/src/features/agent/api/` — drop any frame type members that only those frames carried
- Create: `api/tests/kit/feed.go` (`//go:build integration`) — `FeedRows(t, baseURL, chatID string) []ledger.Row` (dial `since=0`, read until `ready`), unless `api/tests/chat_feed_test.go` already has such a helper: then move that helper into the kit rather than writing a second one
- Modify: every black-box test that asserted through the deleted reads (`grep -rln '/messages\|/activity\|/choices\|pending-prompt' api/tests`; at `d8404d2e` about 14 files in `api/tests/` plus `tests/integration/threads/threads_test.go` and `tests/integration/scripted/livemodel_test.go`, which are vet-only) — assert on `kit.FeedRows` instead; delete `regression_pending_prompt_test.go`'s cases for the deleted route, keeping any that assert prompt state through rows; oversize files only lose lines (S1 R17)

- [ ] **Step 1: Failing tests**

```go
// conversation/chatlog_test.go
func TestChatLog_ReadsTextFromLedgerRows(t *testing.T)
// domain/ledger (the test file of the source OpenAsks lands in)
func TestState_OpenAsksListsAsksWithNoResolution(t *testing.T)   // the cases transcript's PendingChoicesOf tests cover move here
// endpoints/chat/routes_test.go, endpoints/home routes test
func TestRoutes_ServeNoTranscriptReads(t *testing.T) // the six paths are absent from both route tables
```

- [ ] **Step 2: Run; they FAIL.**
- [ ] **Step 3: Delete** the package, handlers, routes, DTOs, frames and read shapes; rewire `chatlog.go`/`handoff.go`; move the black-box assertions onto `kit.FeedRows`; drop the web frame kinds.
- [ ] **Step 4: Run to green**

```bash
T=$(mktemp -d); cd api
HOME=$T/home CROWBAR_HOME= go test -tags noEmbed ./internal/app/usecases/chat/... ./internal/api/... ./internal/domain/... ./internal/app/
HOME=$T/home CROWBAR_HOME= go test -tags 'integration noEmbed' -run 'TestRouteAudit' ./internal/api/v0/
HOME=$T/home CROWBAR_HOME= go test -tags 'integration noEmbed' -count=1 -timeout 40m -run 'TestChatFeed_|TestRegression_|TestFakeProvider_|Chat|Agent' ./tests/
go vet -tags 'integration noEmbed' ./tests/integration/... ./tests/kit/...   # vet only, never run
golangci-lint run --build-tags noEmbed ./... && scripts/deadcode-check.sh && go vet -tags noEmbed ./...
cd ../web && bun tsc --noEmit && bunx vitest run src/__tests__/lib/ws src/__tests__/lib/store && bun run knip
cd .. && node scripts/ratchet/file-size.mjs && node scripts/ratchet/counters.mjs; rm -rf $T
```

- [ ] **Step 5: Commit**

```bash
git add <every created, modified and deleted path, by name; deletions with git rm>
git commit -m "refactor(api): delete the transcript projection and the reads only the old client used

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01DG5r7UkKfKiDuXTdNTGTXK"
```

**Acceptance:** `grep -rn 'internal/transcript\|/activity\|pending-prompt' api/internal` finds nothing; `get_chat_log` and the handoff return the same texts as before for the recorded subagent session; answering an ask, termwait and the answer desk behave as before (their tests unchanged and green).

---

### Task 12: Chat-surface hygiene pass (R18)

**Goal:** Every file under `web/src/features/agent` that T3–T10 did not edit follows the comment rule (*why* in ≤3 lines; no narratives, incident history or restated code) and is under 500 lines. Code does not change, except dead code knip reports and the mechanical split of `streaming-value-patch.ts`.

**Files:**
- Modify: every `.ts`/`.tsx` under `web/src/features/agent` not in the diff of T3–T10 (list with `git diff --name-only <T2 commit>..HEAD -- web/src/features/agent` and take the complement), notably `composer/plate/*`, `controls/*`, `transcript/plate/*`, `hooks/use-chat-presentation.ts`, `chat/dnd-scope.tsx`, `chat/lib/*`, `components/descriptor-status.tsx`, `components/provider-switch-dropdown.tsx`, `terminal/*`, `shared/*`, `api/upload-chat-attachment.ts`, `activity/lib/*`
- Split: `web/src/features/agent/transcript/plate/streaming-value-patch.ts` (1,002 lines, 491 of them comments) — after trimming, if still over 500, move internal helpers into `transcript/plate/streaming-value-blocks.ts`; `streaming-value-patch.ts` keeps its exports so `streaming-value-patch.test.ts` stays byte-identical and green
- Modify: `transcript/plate/chat-fresh-text-plugin.tsx` — give its eslint directive a description and drop it from `UNDESCRIBED_DIRECTIVE_BASELINE` in `web/eslint.config.js`; drop `MAX_LINES_BASELINE` entries for every file now under the cap; drop `web/knip.json` entries that no longer have a finding

The fade, the follow and every animation are untouched: this task deletes comment lines and moves code between files, nothing else. A reviewer checks `git diff -w --stat` against `git diff -w | grep '^[-+]' | grep -v '^[-+]\s*\(//\|\*\|/\*\)'` (the second shows only the moved code).

- [ ] **Step 1: Trim and split.**
- [ ] **Step 2: Run the module's suite unchanged** — `cd web && bunx vitest run src/__tests__/features/agent` (no test file changes in this task).
- [ ] **Step 3: Checks** — `cd web && bun tsc --noEmit && bunx eslint src/features/agent --max-warnings 0 && bunx prettier --check src/features/agent && bun run knip && bun run lint:baselines && bun run lint:doctor`
- [ ] **Step 4: Commit**

```bash
git add <every modified and created path, by name>
git commit -m "chore(web): chat-surface comments state why in three lines

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01DG5r7UkKfKiDuXTdNTGTXK"
```

---

### Task 13: Gate

**Goal:** Prove the S2 gate: `features/agent` ≤ 12,000 lines; zero polling loops with `web.timers` lower; symptoms 4–8 closed by green regression tests; every check CI runs is clean; and, live in the desktop app, every row kind renders, send / Esc-retract / Stop / resume work, nothing remounts when a reply settles and every animation still plays.

**Files:**
- Modify: `scripts/ratchet/loc-budgets-baseline.json`, `scripts/ratchet/counters-baseline.json`, `scripts/ratchet/file-size-baseline.json` (written by `--update`, lower only)
- Nothing else is committed. The live-check driver lives in the session scratchpad (R25).

- [ ] **Step 1: Disk.** `df -h .` (macOS: `/System/Volumes/Data`); under 8 GB free, stop and report.
- [ ] **Step 2: Ratchet.**

```bash
node --test scripts/ratchet/*.test.mjs
for g in file-size counters loc-budgets; do node scripts/ratchet/$g.mjs --update; done   # writes only shrinks; read the output: a `grew` line is a failure to fix
node scripts/ratchet/file-size.mjs && node scripts/ratchet/counters.mjs
node scripts/ratchet/loc-budgets.mjs   # red at base for keys S2 does not own (index, Gate baseline); report, never raise
node -e "const b=require('./scripts/ratchet/loc-budgets-baseline.json');console.log('chat-surface',b['web.chat-surface']);process.exit(b['web.chat-surface']<=12000?0:1)"
node -e "const b=require('./scripts/ratchet/counters-baseline.json');console.log('web.timers',b['web.timers']);process.exit(b['web.timers']<=72?0:1)"
```

If `web.chat-surface` is over 12,000, report physical and code-only lines per directory against the index's targets and bring the cut list to the USER (P26; no blank-line squeezing); never move code out of `features/agent`. List every red loc key with its delta against the S2 base.

- [ ] **Step 3: Zero polling loops.** The timer matches under `features/agent` are exactly the R14 allowlist:

```bash
node scripts/ratchet/counters.mjs --list web.timers | grep 'web/src/features/agent/' | cut -d: -f1 | sort -u > "$SCRATCH/timers.txt"
printf '%s\n' web/src/features/agent/activity/lib/text-clock.ts web/src/features/agent/activity/working-line.tsx web/src/features/agent/hooks/use-slash-catalog.ts | sort > "$SCRATCH/allow.txt"
diff "$SCRATCH/allow.txt" "$SCRATCH/timers.txt"
grep -rn 'agentChats\|useChatMessages\|useAgentActivity\|usePromptQueue\|chatLedgerStore' web/src; test $? -eq 1
```

(`$SCRATCH` is the session scratchpad. Each command's exit status is the check; nothing echoes a pass.)

- [ ] **Step 4: Web checks CI runs** (targeted tests only, never the full suite):

```bash
cd web && bunx eslint . --max-warnings 0 && bun run knip && bun tsc --noEmit && bunx prettier --check . && bun run lint:baselines && bun run lint:doctor
cd web && bunx vitest run src/__tests__/features/agent src/__tests__/features/workspace src/__tests__/features/tabs src/__tests__/features/panes src/__tests__/components/sidebar src/__tests__/lib/ws src/__tests__/reduced-motion.test.ts
cd web && bunx vitest run -t 'symptom [4-8]' src/__tests__/features/agent   # every symptom test, green
```

CI also runs `bun run test:coverage` over the full suite; S2 does not run it locally (user rule). If that job fails, add behaviour tests; never lower `vite.config.ts` thresholds.

- [ ] **Step 5: Go checks.**

```bash
T=$(mktemp -d); cd api
HOME=$T/home CROWBAR_HOME= go test -tags noEmbed ./internal/app/usecases/chat/... ./internal/domain/... ./internal/api/...
HOME=$T/home CROWBAR_HOME= go test -tags 'integration noEmbed' -count=1 -run 'TestRegression_Symptom|TestRegression_C27_|TestChatFeed_|TestFakeProvider_' ./tests/
golangci-lint run --build-tags noEmbed ./... && scripts/deadcode-check.sh && go vet -tags noEmbed ./... && go vet -tags 'integration noEmbed' ./tests/integration/... ./tests/kit/...
rm -rf $T
```

- [ ] **Step 6: Live check in the noVNC container** (isolated `CROWBAR_HOME` under the worktree's git-ignored `/.crowbar/`, R25 fake; one dev instance, started with the direct commands the handoff names, never make targets; keep the window visible, an occluded webview throttles rAF). The WebKitGTK height-animation check (index O3) is re-run by the USER on the desktop at the end of the branch.
  1. Resolve `CROWBAR_HOME` as the Makefile does (`<worktree>/.crowbar`) and refuse to continue unless the socket path starts with the worktree path; never touch `~/.crowbar/crowbar.sock` or `~/.crowbar/state`.
  2. Build the fake CLI into the scratchpad: `cd api && go build -tags integration -o "$SCRATCH/fakecli" ./tests/kit/fakecli`.
  3. Write `<worktree>/.crowbar/descriptors/fake.yaml` with `yq` from the shipped `claude.yaml`, applying exactly `kit.InstallFakeProvider`'s edits (S1 T1): `spawn.cmd` and `runtime.spawn.cmd` = the fake CLI; the dropped keys; `hooks_injection` = the four `set_env` steps with `FAKECLI_DIR` = `$SCRATCH/fake`. Restart the dev daemon if the provider list does not show `fake`.
  4. Create a chat on `fake` in a scratch project. Send a prompt: dispatch `new InputEvent('beforeinput', { inputType: 'insertText', data: 'hello', bubbles: true, cancelable: true })` on `[data-slate-editor]` through the container browser's script tool, then the named key Enter. If the editor does not take synthetic input, stop and ask the USER to type the prompts of steps 4–8.
  5. Read the runner id and token from `$SCRATCH/fake/<runner>.json`, then post hooks with `curl --unix-socket <worktree>/.crowbar/crowbar.sock -H 'X-Crowbar-Runner-Token: …' http://crowbar/v0/…/chats/runners/<runner>/hooks/fake/<event>`: replay `api/tests/testdata/hooks/claude-subagent-session` with its session id rewritten to the chat's bound one, then hand-written payloads for a failed tool, an MCP call (`mcp__echo__shout`), an edit with a 3-line diff, a permission request (answer it with a click), task create/complete, pre/post compact, a notification, a stop failure, and a `SessionStart` with a new session id.
  6. **Every row kind renders:** for each kind in the index vocabulary that a hook can produce, a row with `data-row-id` exists and has non-empty text (the container browser's DOM snapshot). Reasoning and commentary are Codex-only and the fake speaks the Claude hook surface: record them as covered by T9's tests, and list the gap in the report.
  7. **No remount flash:** before a streamed reply settles, install a `MutationObserver` on the scroller (script tool) and keep a reference to the reply's `[data-row-id]` element; after its complete row and `turn_ended`, the same element is connected and the observer recorded no removal of it.
  8. **Send / Esc-retract / Stop / resume:** Esc in the empty composer right after a send returns the text (step 4's prompt, before posting `user_prompt`); Stop during a replay shows "Stopping…" then "Interrupted · you stopped this turn"; kill the fake CLI's pid, send again → a new runner and the chat continues; restart the dev daemon → the feed resumes (new epoch, rows continue, no duplicate rows); create `$SCRATCH/fake/refuse-<session>` and resume → the lost-session row with "Start a new conversation".
  9. **Animations:** while a reply streams, `.chat-fresh-text` spans exist; a new row carries `data-arriving` in its first frames; sampling `scrollTop` on rAF for 500 ms while following shows intermediate values (an eased follow, not a jump); the working-line verb rotates. **Diagram fences (R26):** post assistant-text deltas that open a `mermaid` fence, stream its body over several frames, then close it: the block shows `data-state="skeleton"` while open, `diagram` once closed, on the same element, and sampling its height on rAF across the swap shows intermediate values; repeat with a reply that ends inside the fence: it settles as `code` with no skeleton left; repeat once with `excalidraw`.
  10. **Long chat:** stop the dev daemon, `cd api && CROWBAR_HOME=<that home> go run ./cmd/crowbar-seed-chat -workspace-id <id> -turns 200 -tool-calls 5`, start it, open the chat: rows are visible on the first screenshot (no blank, symptom 6), scrolling to the top and back stays smooth (screenshots before/after).
  11. Screenshots of steps 6–10 go to the scratchpad; the report lists each step's outcome.
- [ ] **Step 7: Commit** the lowered baselines only.

```bash
git add scripts/ratchet/loc-budgets-baseline.json scripts/ratchet/counters-baseline.json scripts/ratchet/file-size-baseline.json
git commit -m "chore(ratchet): S2 gate lowers the chat-surface and timer baselines

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01DG5r7UkKfKiDuXTdNTGTXK"
```

**Acceptance:** `web.chat-surface` ≤ 12,000; `web.timers` ≤ 72 with exactly the R14 allowlist under `features/agent`; every `symptom 4`–`symptom 8` test, every `C27` test and every `TestRegression_Symptom5_`/`TestRegression_C27_` test green; the live check's eleven steps reported with screenshots, the Codex-only gap named.
