# S2 — Window-level ChatStore fed by push: Implementation Plan (index)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Tasks run in the order of "Task order" below: each one assumes every earlier task in that order, and all of S1 (T1–T13 and the S1 final-review fix dispatch), is committed.

**Goal:** One window-level `ChatStore`, filled only by S1's per-chat feed (rows by `seq`, plus an ephemeral delta side channel) and the scope lifecycle feed. It renders each row kind with one renderer under a stable key, and sends prompts through an outbox state machine that only ledger facts advance, with `retract` so Esc after send gives the prompt back. Every chat poll, the streaming copy, client prompt persistence, the chat slice in workspace stores, remount-on-settle and the daemon's R13 read projection are deleted.

**Architecture:** `features/agent/stores/` holds the ChatStore: lifecycle entities per chat (`chats`), a per-chat feed state (`feeds`: rows folded into items keyed by stable id, one order key, turn facts, live text) and the outbox. Every component subscribes with a narrow selector to the one slice it draws, so a delta re-renders its streaming row and nothing else. The feed is the only source of message state on the client: no REST transcript read, no lifecycle message frame and no second store survives T3 (R28). S1 already shipped the final delta shape, the post-`ready` snapshot and `answerable` on asks (S2 ledger P3), so the daemon gains only a blob read route, a typed session-lost send error (C27) and one verb (`retract`); S2 ends by deleting the read projection S1 kept for the old client.

**Tech Stack:** React 19, zustand (vanilla `createStore` + a selector hook), `@tanstack/react-virtual`, Plate (the only markdown renderer), `@/components/ui/*`, phosphor icons, shiki, vitest + Testing Library (`web/src/__tests__/`), Go (`api/`, `-tags noEmbed`), gin, gorilla/websocket, testify, `api/tests` black-box suite over S1's `kit.FakeProvider`.

**Spec:** `docs/plans/2026-10-16-crowbar-architecture.md` §3, §4 ("The one stream"), §5, §11 row S2 · `docs/plans/2026-10-16-chat-surface-report.md` §2 (symptoms 4–8), §4.2 (outbox diagram), §4.4 (client) · `docs/plans/2026-10-16-chat-surface-additions-design.md` §3.2, §4 A–J, §5 budgets · the S1 plan (`docs/plans/2026-10-17-s1-chat-ledger-plan.md` and s1a–s1d): row vocabulary, R4, R5, R7, R10, R13, T10 feed. Executors read the sections a task cites.

## Plan files

| File | Tasks |
|---|---|
| `docs/plans/2026-10-17-s2a-daemon-contract-plan.md` | T1 blobs by ref and the typed session-lost send error (C27) · T2 `retract` |
| `docs/plans/2026-10-17-s2b-store-and-feed-plan.md` | T0 mechanical pre-split (P13) · T3 the transcript comes from the feed (symptoms 4, 7; streaming diagram fences) · T4 chat entities move to the ChatStore |
| `docs/plans/2026-10-17-s2c-outbox-scroll-composer-plan.md` | T5 outbox and Esc → retract (symptom 5; C27 web half) · T6 scroll without a settle gate (symptom 6) · T7 one composer that serializes on send (symptom 7) |
| `docs/plans/2026-10-17-s2d-rows-plan.md` | T8 tool and MCP rows (A, B) · T9 subagent card, todo strip, working line, reasoning (C, D, E, J; symptom 8) · T10 approvals, errors, interrupt, lost session (F, G, H; C27 toasts, truncated ToolInput) |
| `docs/plans/2026-10-17-s2e-deletion-and-gate-plan.md` | T11 delete the daemon's R13 projection · T12 chat-surface hygiene pass · T13 gate |

13 tasks plus the T0 pre-split, one commit each. T3 is the large cutover: the transcript, its live text and every REST read move to the feed together, because the old row list joins messages and tool calls that the feed delivers as one ordered stream.

## Task order (S2 ledger P2, P13)

T1 → T2 → T0 → T3 → T4 → T5 → **T11** → T6 → T7 → T8 → T9 → T10 → T12 → T13. T11 runs right after T5 (P2): once T5 has moved the last REST read (`getPendingPrompt`, kept by T3 per P12) onto rows, nothing reads the projector, and deleting it early stops later tasks from leaning on it. T0 is a behaviour-free split of the oversize files T3–T5 rewrite, so the T3 diff shows only the cutover.

Dependencies outside S2: all of S1, including its final-review fix dispatch (C25, C28, C30, and the `adapter/store/ledger` coverage lane, which touches the blob store T1 reads). S2 does **not** depend on S4; S4 depends on S2 (S4b replaces R15's `url` resolver with `holdScope('chat:<id>')`, S4c deletes T4's `chats`/`byWorkspace`/`scopes`/`telemetry`). S3's error envelope work (S3 D1) owns the generic "raw daemon text in a toast" class; S2 fixes only the session-lost and resume/switch cases C27 names. S3 runs before S2 (S3 OR1; at `b79be6f2` S3 T1, T2 and T8 have landed: `domain/problem`, `libs.Fail`, `ApiError.problem`, `report()`): S2 writes `problem` codes, never `apperr` (gone). Shared files with S3 and who splits them (pre-flight F3, F13): S2 T0 splits `agent-api.ts` (S3 T5 follows its parts, S3 A9 as amended), `agent-chats-slice.ts` and `agent-chat-pane.tsx` (S3 T7's OR3 sites follow the parts, whichever lands second); S3 T5 splits `endpoints/chat/handlers/chats.go` into `chats_create.go`/`chats_read.go`/`chats_mutate.go` (S2 T11 follows those parts).

## Refresh notes (2026-10-28, at `d8404d2e`)

What changed under this plan since it was written at `94d001ddd`, and is folded into the task text below:

- S1 T10 shipped `GET /v0/chats/:chatId/feed?since=N` with R3's exact delta shape (`api/internal/app/usecases/chat/internal/ingress/live.go` `FeedDelta`), the post-`ready` `Replace` snapshot (`Recorder.Snapshot`) and the frames `row`/`ready{epoch,seq}`/`delta`/`resync{seq}`/`deleted`. A `since` beyond the head answers `resync{seq: head}` then closes 4000 (S1 ledger ruling); an attach failure answers `resync{seq: since}`; a client over 1,024 unread frames gets `resync{seq: last sent}`; a purge ends with `deleted` and 4001. A plain GET is a 400. A long tool output streams a 64 KiB tail window (`toolOutputWindow`), cut by a `Replace`.
- S1 T7/T8b wrote `answerable` on ask rows (`api/internal/domain/ledger/ask.go`), and `ask_resolved` gained `refused` (an ask nobody answered in time; web `afa856c5`). `TurnRetracted` was never added (P3).
- S1 T11 retired the relay: HTTP hooks are the only ingress (`POST …/chats/runners/:segid/hooks/:provider/:event`, header `X-Crowbar-Runner-Token`). S1 T12 (migration step 6) converts legacy chats into rows, and `cmd/crowbar-seed-chat` already writes ledger rows (spec issue 10 is closed).
- The web chat surface is unchanged in substance: `web.chat-surface` 21,131 (was 21,130), the same 16 timers. `agent-api.ts`'s reads are named `listChatMessages`/`listChatActivity` (not `getChat*`); `streaming-message-batcher.ts` lives in `web/src/features/workspace/stores/hooks/lib/`.
- `wsManager`'s transport `onclose` carries no close code (`web/src/lib/ws/manager.ts`, `tauri-transport.ts`), so the client acts on the `resync`/`deleted` frame, never on 4000/4001 (R15, R28).
- The chat transcript renders mermaid fences as plain code today (`ChatCodeBlockElement` handles only excalidraw and text attachments; `MermaidDiagram` lives in `features/editor/markdown/plate/`). R26 adds diagram rendering with the streaming skeleton.
- The S2 ledger's rulings P1–P36 (adopted, user-accepted 2026-10-18) were never folded into this text; the ones that change a task are folded below (P1 → R1, P2/P13 → task order, P3 → T1, P7–P12 → T3/T5, P26 → T13, P31 → disk). Carries C7 and C27 and the M8 fix (already landed in S1 T5: `3f4561ba`, `spawn-error.ts` maps `chat_busy`) are folded into T1, T5 and T10.

---

## Global Constraints

Every task's requirements include these. Quoted from the specs, `CLAUDE.md` and the user's standing rules.

- "no feature flags, no parallel old/new code paths, no compatibility shims between stages. Each stage deletes what it replaces in the same change." Every task deletes what it replaces in its own commit.
- "Same features as today, fewer lines." Nothing a user can do today is lost by the end of S2. A task may show a simpler presentation that a later S2 task finishes (T3's tool row before T8), never a missing capability.
- "A fix starts with a failing test that reproduces it. Never fix a symptom with a timer, poll, retry, extra flag, or resync effect." Symptoms 4–8 each get a failing regression test first, in the task that closes it (names in each task).
- "No timer-gated UI." The only `setTimeout`/`setInterval` left in `web/src/features/agent` at the end are the allowlist in R14.
- "Plate is the only markdown renderer, never react-markdown." Rendering goes onto `@/components/ui/*` with CSS-variable tokens, phosphor icons, shiki for code.
- Preserve every animation: the per-word `chat-fresh-text` fade (`transcript/plate/streaming-value-patch.ts`, `chat-fresh-text-plugin.tsx`), the eased follow (`hooks/lib/follow-scroll.ts`), the arrival slide (`chat/lib/arrival-animation.ts`, `data-arriving`), the working-line verb rotation, shimmer on live verbs. "A snap is a defect." Never `content-visibility` (it caused a CPU melt).
- Store patterns: `useChatStore((s) => s.narrow.field)` with a narrow selector, never a bare `useChatStore()`; `chatStore.getState()` only inside event handlers and `useEffect` bodies; stores never import from `components/` or any `*.tsx`; a selector never returns a fresh array or object (keep indexes in the store, or use `useShallow`).
- Files: kebab-case component files; every file a task creates or edits is ≤500 lines when the task commits (R19). Comments explain *why* in ≤3 lines (R18).
- Tests: in `web/src/__tests__/` mirroring `src`, `@/` imports, extend the unit's test file, ≤3 `vi.mock` per file, no Tailwind-class assertions, no reading `src/` text, no sleeps (`waitFor`, fake timers, resolved promises). Go: one test file per source file, `require.Eventually`/channels, no `time.Sleep`, every backend bug gets a black-box `TestRegression_*`.
- Running tests: targeted only, never the full suite. Web: `cd web && bunx vitest run <paths>`. Types: `cd web && bun tsc --noEmit` (never `bunx tsc`: that resolves a different package). Before each web commit also: `bunx eslint <touched paths> --max-warnings 0`, `bunx prettier --check <touched paths>`, `bun run knip`, `bun run lint:baselines`, `bun run lint:doctor`. Go, always with an isolated home: `T=$(mktemp -d) && cd api && HOME=$T/home CROWBAR_HOME= go test -tags noEmbed ./internal/<pkg>/...`; black box (bbx) `HOME=$T/home CROWBAR_HOME= go test -tags 'integration noEmbed' -count=1 -run '<regex>' ./tests/` (`-timeout 40m` for a wide regex; `-race` on targeted tests only); then `golangci-lint run --build-tags noEmbed ./...`, `scripts/deadcode-check.sh`, `go vet -tags noEmbed ./...` from `api/`. Never `go test ./...`, `make pr-checks`/`test-integration`/`test-coverage`, or `-tags integration` on `./tests/integration/...` or `./tests/kit/...` (vet only).
- Baselines only shrink. A task that fixes a file listed in `web/knip.json` `ignoreIssues`, a `*_BASELINE` list in `web/eslint.config.js` or `api/deadcode-baseline.txt` deletes that entry in the same commit (`check-knip.mjs` and `check-eslint-baselines.mjs` fail on a stale entry). Never add an entry.
- Safety: never connect to `~/.crowbar/crowbar.sock` or open anything under `~/.crowbar/state`; live checks run in the isolated noVNC container with a `CROWBAR_HOME` under the worktree's git-ignored `/.crowbar/` (S3 OR6, S4 OR4), started by the direct commands the handoff names, never make targets; the user re-runs the WebKitGTK height-animation check (O3) on the desktop. Never read or write `~/.claude` or `~/.codex`. No real provider CLI runs anywhere in S2 (handoff hard rule); the live check uses the fake CLI (R25). Check free disk on the worktree's filesystem (`df -h .`; on macOS `df -h /System/Volumes/Data`) before any build or `bun install`; under 8 GB free, stop and report (P31: before T3's `bun install` too). No provider names in non-test Go code or comments. Never bare `git stash`, never `git rebase -i`. Never push. Stage by explicit path (`git add <paths>`, never `git add web`/`api`/`-A`). No scratch files inside the worktree; scratch goes in the session scratchpad. Commit messages end with the trailer the handoff names (`Co-Authored-By: …` and `Claude-Session: …`).

## Rulings (open decisions resolved for S2)

| # | Decision | Ruling |
|---|---|---|
| R1 | Ratchets per task (amended by P1, CLAUDE.md and the red loc-budgets base) | Every task runs `node scripts/ratchet/file-size.mjs` and `node scripts/ratchet/counters.mjs` (both green at base). Any `grew`/`added` fails the task. `runGate` also exits 1 on an unwritten shrink, so a task that deleted code runs the same script with `--update` (it refuses growth and writes only shrinks) and commits the lowered `file-size-baseline.json`/`counters-baseline.json` with the task, as S1 did (e.g. `cae02572`). `loc-budgets` is red at base (handoff §4, S1 R7: at `d8404d2e` web.core, web.sidebar, web.editor, web.tests, web.total and most go.* keys are over), so `--update` refuses it: a task runs `node scripts/ratchet/loc-budgets.mjs` read-only and states, in its commit body, the delta against its base for every key it touched; it never makes a key it touched rise. Each task checks the 500-line cap with `wc -l` on what it touched. T13 makes the final write-back of every gate. |
| R2 | Where the ChatStore lives | `web/src/features/agent/stores/` (chat-surface). `CLAUDE.md` says server-adjacent "conversations" go in `lib/store/`, but `features/agent/stores/chat-ledger-store.ts` already sets this precedent, and moving chat code into the sidebar module would hide it from the chat-surface budget. |
| R3 | Delta frames | **Shipped by S1 T10** (P3): `delta{id, turnId, scope, stream, n, text, replace}`. `n` is the frame's 1-based position in its stream; `text` is only the new suffix, or everything so far when `replace` is true (a non-prefix rewrite, a tool-output window cut, or the snapshot every live stream gets right after `ready`). The client appends when `n == last+1`, replaces on `replace`, ignores `n <= last` (and, with no `replace`, an `n > last+1` gap: the next `replace` or the item's row repairs it). `id` is the message id for `answer`/`reasoning` and the tool id for `tool_output`/`diff`. S2 changes nothing on the daemon side. |
| R4 | Whether Crowbar can answer an ask | **Shipped by S1** (`ledger.Ask.Answerable`, `json:"answerable"`). It never changes on the row: a CLI that hangs up, the bound tool's post, an expiry (`refused`) or the turn ending each write `ask_resolved`. |
| R5 | Tool input/output after `/activity/:toolId/payload` goes | T1: `GET /v0/chats/:chatId/blobs/:ref` on the flat `chats` group beside `/:chatId/feed` (no worktree needed). `ref` is `sha256:` followed by 64 lowercase hex, exactly as rows carry it in `inputRef`/`resultRef`; anything else → 400 (`problem.ErrInvalidArgument`), checked by `ledgerstore.ValidRef(ref) bool`, exported from the check `Blobs.pathFor` already does (`adapter/store/ledger/blobs.go`), never a second one; unknown chat or swept blob → 404 (`problem.ErrNotFound`). The web `readBlob` URL-encodes the ref. It reads `adapter/store/ledger.Blobs.Get(ref)` after checking the chat exists. Blobs are content-addressed and local to this user's daemon, so cross-chat reads are not a privilege boundary; the chat id only scopes the URL. |
| R6 | `retract` (report §4.2, additions H) | T2: `POST …/chats/:id/prompts/:requestId/retract`. Pending prompt → `prompt_settled{consumed:false, reason: retracted}`, and a runner started with that prompt (`delivery: restart`) is terminated (`runner_exited{reason: retracted}`); response `{state: "retracted", text}`. Delivered with its turn running → `turn_ended{interrupted, reason: retracted}`, then the provider is signalled exactly as Stop does (S1 R10); response `{state: "interrupted", text}`. Settled prompt or closed turn → 409 (`problem.ErrConflict`). Unknown request → 404 (`problem.ErrNotFound`). A provider acknowledgement that lands after a retract is recorded as what it is: a provider-origin `user_message` and its `turn_started`. `TurnRetracted` does not exist (P3); the reason field carries it. |
| R7 | Who owns the outbox | The client, in the ChatStore, per architecture §5 and additions §I: `queued` and `sending` exist only in memory; from the `user_message` row on, the prompt's state is read from rows (`pending → delivered → settled | refused | retracted`). Persistence is deleted: an unsent prompt does not survive an app reload (decision D1, accepted only with P11: a steerable prompt is sent at once with `delivery: steer` and so persists as its `user_message` row; only a prompt held by the outbox, i.e. not steerable, is memory-only). P11 also keeps the compaction hold, the image-upload hold and the staged model/effort selection that `use-prompt-queue.ts` has today. |
| R8 | Item identity and order | Item ids: `p:<requestId>` (Crowbar prompt; the same key from the optimistic echo through settle), `u:<seq>` (provider-origin prompt), `m:<turnId>:<messageId>` (assistant text), `r:<turnId>:<messageId>` (reasoning; `r:<seq>` when the row has no `messageId`, which is optional, so two reasoning rows of one turn never collide), `t:<toolId>` (tool), `s:<spawnToolId>` (subagent card, keyed by the call that spawned it so binding never changes the key; it takes that call's place, and the spawning `tool_call` makes no tool item; a subagent with no spawning call: `s:<scope>`), `a:<askId>` (ask), `k:<seq>` (marker, turn end, failure, lost session). Order is the `seq` of the item's first row and never changes. An item known only from live deltas sits in a live tail after the ordered list and moves into the order, same key, when its first row lands. Rows in a subagent scope are the card's children, never top-level items. |
| R9 | Busy | The pane reads turn facts from its own fold of rows (running turn, pending prompts, open asks, live subagents). Sidebar and tabs, which have no feed, read the entity's server-folded `working` from lifecycle frames. No client code writes `working`. |
| R10 | Telemetry and providers | Telemetry moves to `ChatStore.telemetry[chatId]`: the lifecycle `telemetry` frame, plus one GET when a feed attaches; a value with an older `observedAt` never replaces a newer one. Providers are read from the window-level `useAgentProvidersStore`; the per-workspace copy and its 3× retry + toast are deleted (D5). |
| R11 | Remount on settle | A text row picks its renderer once, at mount: an item that is not settled when its row mounts keeps the live Plate editor (with the fresh-word fade) until that row unmounts; an item settled at mount renders static. Settling changes the item's data, never the component. Virtualizer unmount/remount on scroll is not a settle. |
| R12 | Scroll anchoring: report §4.4 says "CSS bottom anchoring", the user's rule says preserve the eased follow | Keep `hooks/lib/follow-scroll.ts` for follow-at-bottom (a rAF animation, not a timer). Delete the reveal/visibility settle gate (`SETTLE_QUIET_FRAMES`), the measured tail-room padding, `ARM_QUIET_MS`, `READER_INPUT_MS`, `PIN_SETTLE_GRACE_MS`, the arm timer and `use-scroll-frame-span`. Pin-to-top becomes a spacer computed from the virtualizer's own offsets (never measured DOM padding). `overflow-anchor` keeps a reader's position while rows above change size. |
| R13 | Lost session (S1 R5 leaves the affordance to S2) | A `session_lost` row renders as a marker with one action, "Start a new conversation", which calls the existing `resumeChat` (the next prompt starts fresh, as S1 R5 does). Rebinding to another session id is S8's (decision D3). A send, resume or switch the daemon refuses because the session is lost answers the typed code `session_lost` (R27); the web never shows the "may already have this message" wording or raw daemon text for it. |
| R14 | "Zero polling loops" | At T13 the only `setTimeout`/`setInterval` matches under `web/src/features/agent` are: the shared 1 s text clock (`activity/lib/text-clock.ts`, writes `textContent`, no re-render), the working-line verb rotation, and `use-slash-catalog`'s input debounce. Feed reconnects use `wsManager`'s existing backoff (R15); no new timer anywhere. |
| R15 | Resuming a feed with `since=seq` through `wsManager` (its URL is fixed at subscribe) | T3: `wsManager.subscribe(endpoint, cb, url?)` takes an optional `() => string` evaluated at every (re)open; the endpoint string stays the channel key. The feed passes `() => /v0/chats/<id>/feed?since=<feed.seq>`. The manager's `onclose` carries no close code, so the client acts on the frame the daemon sends before closing, never on 4000/4001: `resync{seq}` with `seq == feed.seq` reconnects through the manager's existing backoff with `since=feed.seq`; `resync{seq}` with `seq < feed.seq` (a `since` beyond the daemon's head: the client is ahead of a reset ledger) drops the feed state and reattaches with `since=0`; `deleted` drops the feed and unsubscribes, so the manager does not reopen it. While rows are folding into the unpublished pre-`ready` draft, a `resync` is compared with the draft's seq: a resync before `ready` (an attach that failed mid-replay answers `resync{seq: last replayed}`, `feed.go` `pump`) drops the draft and reconnects with `since=<published feed.seq>`; `seq > feed.seq` never happens after `ready`. The manager's `{ reconnected: true }` callback is ignored by the feed (its replay is the `since`). S4b later replaces this resolver with `holdScope` (S4 A2). |
| R16 | Lifecycle subscriptions | T4: one `wsManager` subscription per lifecycle endpoint (`${chatBase(wsId)}/ws`), refcounted by the workspace views that need it, applied by one reducer into `ChatStore.chats`. Today every mounted workspace applies every frame of its repo into its own copy (K-fold). After T11 the lifecycle feed carries entity frames only. |
| R17 | Feed lifetime | A chat's feed is attached while a view of it is mounted or its outbox is non-empty. Detaching drops its feed state; re-attaching replays from `since=0`. No LRU, no cache. |
| R18 | Comments (33% of `features/agent` is comment lines) | A task that edits a file brings that file's comments to the ≤3-line *why* rule: narratives, incident history and restated code are deleted. T12 applies the rule to the files no earlier task touched. Comment trimming never changes code. |
| R19 | 500-line cap | CSS is exempt (`file-size.mjs` does not count it; `styles/transcript.css` is 878 lines and T8 only adds to it). A task that edits a production or test file over 500 lines brings it under 500 in that task, split by responsibility and named for what each part exports. An oversize test file whose source is rewritten is replaced, not split: its cases for deleted behaviour go, the remaining ones move to the test file of the source they exercise. Outside `features/agent` (`review-thread-item.tsx` in T4), the split is offset by R18 so the module does not grow. |
| R20 | The 6 s "terminal may be waiting" hint in `use-prompt-queue.ts` | Deleted: it is a timer guessing a fact the daemon reports. The banner shows when the chat entity carries `terminalWait` (S1 keeps termwait's detection). Decision D4. |
| R21 | Command URLs | Commands (send, stop, retract, answer, resume, settings) keep today's repo/home-scoped routes, built from the chat entity's own `workspaceId`, never from an ambient workspace. The new reads (feed, blobs) are flat. Unifying the URLs is S4's. |
| R22 | Runner moves | S1 R6 removed runner moves between chats; at `d8404d2e` no lifecycle kind `moved` exists (`grep -n 'AgentChatKind' api/internal/api/v0/dto/*.go` lists none; `domain.AgentExitMoved` is an exit reason, not a frame kind). T4 deletes `followRunner`, the `moved` frame case and `'moved'` in `web/src/lib/ws/chat-frame-kinds.ts`; if that dto grep finds a `moved` kind at T4, stop and report. |
| R23 | Activity components (`/activity` `components`, `lib/activity-components.ts`, `activity-component-row.tsx`, `endpoints/chat/handlers/activity_components.go`) | A second projection of the same facts. T3 deletes the client half, T11 the server half. One renderer per row kind replaces it. |
| R24 | Optional turn fold ("Worked for 42s · 9 tool calls", additions A) | Not in S2 (decision D6). |
| R25 | The live check's provider | The S1 T1 fake CLI (`api/tests/kit/fakecli`), built into the session scratchpad; its descriptor is written into the dev home (`<worktree>/.crowbar/descriptors/`, git-ignored) from the shipped `claude.yaml` by the same edits `kit.InstallFakeProvider` (`api/tests/kit/fakeprovider.go`) makes: the fake as `spawn.cmd` and `runtime.spawn.cmd`, the dropped keys, and `hooks_injection` = the four `set_env` steps `FAKECLI_DIR`, `FAKECLI_SEG={segid}`, `FAKECLI_TOKEN={runner_token}`, `FAKECLI_HOOK={hook_curl} {hook_url}`. Hooks are posted over HTTP (the only ingress since S1 T11) with `curl --unix-socket` to the dev daemon's socket in that home. Nothing of the driver is committed (T13). |
| R26 | Diagram fences while a message streams (user requirement 2026-10-18, S2 ledger; amended by the 2026-10-28 pre-flight F7–F10) | T3. Diagram routing applies only inside a `StreamingFenceContext` provider, which only the transcript's `MarkdownMessage`/`MarkdownMessageStatic` mount: the default context keeps today's behaviour, so the composer (which shares `ChatCodeBlockElement` through `chatComposerPlugins`) is unchanged and excalidraw keeps its draggable attachment shell there. Inside the transcript, a `mermaid` or `excalidraw` fence that is still open in a text item that is not settled renders as a skeleton placeholder: never partial code, never a parse or render attempt. An opening line with no newline yet whose info string is a prefix of `mermaid`/`excalidraw` already counts as a diagram fence (skeleton), so a half-typed ```` ```merm ```` never flashes as code. When the fence closes, the diagram renders once (one mermaid render call, after `MermaidDiagram`'s debounce; no excalidraw scene parse while the fence is open). If the item settles with the fence still open, the block renders as plain code; a skeleton never outlives its stream. "Open" is derived from the item's own text (the last fence of the source has no closing line), never stored: the owner is the text, and settled-ness is the item's. Both providers compute `openFence` (static: `{ live: false, openFence }` → `code`, since remark parses an unclosed fence as a code block running to EOF), and the context value is memoized on `[live, openFence]`. A block is the open fence only if it is the document's last top-level block (`editor.api.findPath`); a fence nested in a list or quote is out of scope. The open/close fence rule (`FENCE` and the close test in `streaming-markdown-parse.ts` `scanFrom`) is extracted into one non-exported helper in that file, called by both `scanFrom` and `openDiagramFence`, which therefore lives in `streaming-markdown-parse.ts` too (shared, not copied, not exported). `openDiagramFence` scans from the stream cursor offset (the settled prefix is never inside a fence). The skeleton and the diagram/code share one wrapper element that reserves the skeleton's height. The skeleton stays visible, with `MermaidDiagram` mounted hidden under it, until `onStateChange('success' \| 'error')`; only then does the block switch `data-state` and, in a layout effect, animate height from the reserved box to the measured one with the Web Animations API (`element.animate`), so neither transition snaps; `animate` is skipped when `matchMedia('(prefers-reduced-motion: reduce)').matches` (the CSS kill-switch in `index.css` does not reach WAAPI); no `content-visibility`, no timer. Mermaid in chat reuses `MermaidDiagram` (`features/editor/markdown/plate/mermaid-diagram.tsx`, already error-contained by `04e0373fc`); excalidraw keeps `ExcalidrawPreview`. |
| R27 | Typed session-lost send error (carry C27; S1 C24 re-review 2 Q3, re-review 3; amended by pre-flight F1, F14) | T1 (daemon): `session_lost` is a problem code: `SessionLost Code = "session_lost"` in `api/internal/domain/problem/code.go` (a specific code; `Status()` → 409 beside `ChatBusy`) and `'session_lost'` in `ProblemCode` (`web/src/lib/errors/problem.ts`). `ErrPromptSessionLost = &problem.Error{Code: problem.SessionLost, …, Err: problem.ErrConflict}` is new in `runner/errors.go`, and `ErrSpawnSessionRefused` becomes a `*problem.Error` carrying the same code, so resume and switch carry it through `libs.Fail` with no handler change (`handlers/switch.go` is not modified). `spawnFailure` (`runner/prompts.go`) returns `ErrPromptSessionLost` when `errors.Is(err, ErrSpawnSessionRefused)` instead of `uncertainPromptOutcome(…)`; a send whose outcome truly is unknown keeps `ErrPromptOutcomeUnknown`. T5 (web outbox): codes are read from `ApiError.problem.code`. `request_id_conflict`, `request_outcome_uncertain` and `session_lost` → failed, and Retry mints a fresh request id (a new `p:<id>` item; the old row stays as history); so does Retry on a row whose prompt fact is `settled`, `refused` or `retracted` (an abandoned prompt). A network error or 5xx → failed, and Retry keeps the same request id (the daemon dedupes by request id). A refusal carrying `session_lost`, or a prompt whose settle reason is `session_lost`, reads "The session was lost. Start a new conversation to send this." never "The provider may already have this message…". T10 (toasts): `describeSpawnError` maps `session_lost` to its own words, and no resume/switch toast shows `problem.message` for `session_lost` (after S3 T1 a daemon message ends in its category text, e.g. `: conflict`). |
| R28 | One source of message state | The per-chat feed is the only input to transcript content, live text and prompt state on the client. After T3 nothing reads `/messages`, `/activity`, payload, `/choices` (GET) or lifecycle `message_delta`/`plan`/`choice`/`compaction_*` frames; after T5 nothing reads `/pending-prompt`; after T11 the daemon serves none of them. T3's acceptance greps for every old store and hook (`chatLedgerStore`, `useChatMessages`, `useAgentActivity`, `streamingMessages`, `notifyAgentChatMessages`, `prefetchVisibleChats`, `listChatMessages`, `listChatActivity`, `getToolPayload`) and finds none outside deleted files. The outbox holds only what has no row yet (`queued`/`sending`/`failed`); an item with a `user_message` row is never also an outbox item. |
| R29 | Test mocks | ≤3 `vi.mock` per file (CLAUDE.md). Files that exceed it today and that S2 rewrites or folds into must come under it in that task, not grow: `components/agent-chat-pane.test.tsx` (7, T4), `composer/agent-composer.test.tsx` (4, T7), `chat/agent-empty-document.test.tsx` (5, T7). Prefer seeding `chatStore` and pushing frames through `applyFeedFrame` over mocking modules. |

## Decisions that change what the user sees

Each is ruled above so the plan can proceed. The user accepted all of them on 2026-10-18 ("Agreed with everything"), with the ledger's amendments: D1 only with P11 (R7), D5 → a Retry action on the failed provider state (no timer).

| # | Change | Task | Alternative |
|---|---|---|---|
| D1 | A prompt queued but not yet sent is lost if the app reloads (today it survives in localStorage). | T5 | (b) a daemon-held queue with a delivery driver: survives reloads, costs a new daemon state machine. |
| D2 | Esc in an empty composer acts on your newest prompt: still queued → back into the composer; sent but not delivered → retracted, back into the composer; delivered with no output yet → the turn is interrupted and the text comes back; once output has arrived → Esc stops the turn and the text stays in the transcript. | T5 | Esc only ever stops, never restores. |
| D3 | A lost session shows "Start a new conversation" and nothing else; picking another session to rebind waits for S8. | T10 | Ship a session picker in S2. |
| D4 | The "terminal may be waiting" hint no longer appears after 6 s on its own; the banner shows only when the daemon detects a terminal prompt. | T5 | Keep a client hint (needs a timer, which the rules forbid). |
| D5 | A failed provider-list load no longer retries 3× and toasts; the provider picker shows the store's failed state with a Retry action (ledger ruling). | T4 | — |
| D9 | A mermaid fence in a chat reply renders as a diagram (today: plain code); while it streams, mermaid and excalidraw fences show a skeleton (R26). | T3 | — (user requirement) |
| D6 | No "Worked for Ns · N tool calls" fold of a finished turn. | — | Add it in S2 (T8, ~80 lines). |
| D7 | Tool rows by family with run grouping and edit diffs open by default when ≤12 lines changed; subagent cards replace the Agent tool row; a todo strip docks above the composer; reasoning persists as a collapsed "Thought for Ns" row; approvals become a card stack docked over the composer; a failed turn is a row with Retry and Open terminal; an interrupted turn shows "Interrupted · you stopped this turn". | T8–T10 | These are the additions design as written; listed so they are not a surprise. |
| D8 | The first prompt of an empty chat is typed into the same composer as every other prompt, placed where the empty-chat document is today. The layout and the move to the dock on send stay; the separate first-prompt editor goes. | T7 | Keep two editors. |

## Shared vocabulary

**Wire, `web/src/features/agent/api/chat-rows.ts`** (TypeScript mirror of S1's `ledger.Row`, `api/internal/domain/ledger/*.go` bodies and `usecases/chat/feed.go` frames as shipped at `d8404d2e`; T3 checks each field against those structs' JSON tags before writing it. Not to be confused with the dead `features/agent/tree/lib/chat-rows.ts`, which T4 deletes):

```ts
export interface LedgerRow<K extends RowKind = RowKind> {
  chatId: string
  seq: number
  kind: K
  turnId?: string
  scope?: string // '' or absent = main agent
  at: string
  body: RowBodies[K]
}
export type RowKind = keyof RowBodies
export interface RowBodies {
  user_message: { requestId?: string; text: string; origin: 'crowbar' | 'provider' | 'harness' | 'note'; delivery?: 'restart' | 'steer' | 'api'; runnerId?: string; dispatch?: string }
  turn_started: { requestId?: string; providerTurnId?: string; runnerId?: string; provider?: string; sessionId?: string; persists?: 'turn_start' | 'turn_end' }
  prompt_settled: { requestId: string; consumed: boolean; reason: string } // reasons: provider_builtin, not_acknowledged, runner_exited, daemon_restart, delivery_failed, session_lost, + retracted (T2)
  turn_ended: { status: 'done' | 'interrupted'; reason?: string; runnerId?: string; effort?: string; diff?: string }
  turn_failed: { reason: string; detail?: string }
  session_bound: { provider: string; sessionId: string; cause: 'assigned' | 'announced' | 'fresh'; previous?: string }
  session_lost: { provider: string; sessionId: string; reason: string }
  runner_started: { runnerId: string; provider: string; terminalSession: string; launchModel: string; launchEffort: string; launchPermissionLevel: string; resumeSessionId: string }
  runner_exited: { runnerId: string; reason: string; exitCode: number }
  assistant_text: { messageId: string; index: number; final?: boolean; text: string; phase?: '' | 'commentary'; reply?: string } // index -1 = complete; reply: the reply this message joins
  reasoning: { messageId?: string; text: string }
  marker: { kind: MarkerKind; detail?: string; providerTurnId?: string } // MarkerKind: ledger/marker.go (session_changed, session_conflict, dead_letter, compaction_started/_ended, stopped, provider_switched, model_changed, effort_changed, notification)
  tool_call: ToolCallBody // ledger/tool.go: toolId, phase (pre|post|fail), name, kind, title, target, locations, inputRef, resultRef, status, error, durationMs, diff, mcpServer, mcpTool, exitCode, interrupted, inputKey, spawnsScope, addressesScope
  ask: AskBody // ledger/ask.go: askId, event, kind, promptId, toolId, toolName, title, question, mode, multi, options, questions, schema, detail, toolTarget, toolInput (JSON; a JSON *string* of the leading text when toolInputTruncated), toolInputTruncated, mcpServer, inputKey, answerable
  ask_resolved: { askId: string; resolution: 'answered' | 'proceeded' | 'refused' | 'abandoned'; optionIds?: string[] }
  subagent: { phase: 'start' | 'stop' | 'bound' | 'result'; spawnToolId?: string; type?: string; title?: string; status?: string; result?: string; usage?: { tokens?: number; toolUses?: number; durationMs?: number } }
  plan: { op: 'replace' | 'upsert'; items: PlanItem[] } // PlanItem: { id?, text, detail?, status? }
}
export type FeedFrame =
  | { type: 'row'; row: LedgerRow }
  | { type: 'ready'; epoch: string; seq?: number }
  | { type: 'delta'; delta: FeedDelta }
  | { type: 'resync'; seq?: number } // Go `omitempty`: an absent seq is 0 (same for ready on an empty chat)
  | { type: 'deleted' }
export interface FeedDelta {
  id: string
  turnId: string
  scope?: string
  stream: 'answer' | 'reasoning' | 'tool_output' | 'diff'
  n: number
  text: string
  replace?: boolean
}
```

**Store, `web/src/features/agent/stores/chat-store.ts`** (T3 adds `feeds`, T4 `chats`/`byWorkspace`/`scopes`/`telemetry`, T5 `outbox`):

```ts
export interface ChatFeed {
  status: 'connecting' | 'live' | 'error'
  error?: string
  epoch: string
  seq: number // last applied row
  order: string[] // item ids, by first seq
  liveTail: string[] // ids known only from deltas
  items: Record<string, TranscriptItem>
  live: Record<string, string> // `<stream>:<delta id>` → live text; a tool's tool_output and diff are two streams with their own n
  liveN: Record<string, number> // same key
  facts: TurnFacts
}
export interface TurnFacts {
  running: { turnId: string; requestId?: string; startedAt: string; seq: number } | null
  prompts: Record<string, PromptFact> // requestId → state from rows
  compacting: boolean
  openAsks: string[]
  liveSubagents: string[] // scopes
  plan: Record<string, PlanItem[]> // by scope, latest
  lastExit: { runnerId: string; reason: string; exitCode?: number; seq: number } | null
  sessionLost: { provider: string; reason: string; seq: number } | null
  output: Record<string, true> // turn ids that produced text or a tool
}
export type PromptFact =
  | { state: 'pending'; seq: number }
  | { state: 'delivered'; turnId: string }
  | { state: 'settled' }
  | { state: 'refused'; reason: string }
  | { state: 'retracted' }
export interface OutboxItem {
  requestId: string
  text: string
  state: 'queued' | 'sending' | 'failed'
  error?: string
  retractOnAck?: boolean
}
export const chatStore: StoreApi<ChatStoreState>
export function useChatStore<T>(selector: (s: ChatStoreState) => T): T
```

`TranscriptItem` is a union by `kind` (`user | text | reasoning | tool | subagent | ask | marker`) with the id scheme of R8; each kind's fields are defined in the task that first renders it (T3 for user/text/tool/ask/marker, T9 for subagent/reasoning). The fold (`stores/lib/fold-rows.ts`) is pure: `applyRow(feed, row): ChatFeed` and `applyDelta(feed, delta): ChatFeed` return a new feed whose unchanged items, `order` and `facts` keep their identity (structural sharing is what makes per-row subscriptions cheap). Text follows S1's `Messages` rule: the contiguous chunks `0..k` when chunk `k` is final, else the complete row (`index: -1`), else the chunks present in index order; settled when a final chunk is contiguous, a complete row exists, or its turn ended.

## Ownership after S2

| Fact | Owner after S2 | Owner today (deleted) |
|---|---|---|
| Transcript content (text, tools, asks, markers, subagents, plan) | `ChatStore.feeds[chat].items`, folded from feed rows | `stores/chat-ledger-store.ts` REST pages + `use-chat-messages` poll; `use-agent-activity` poll + `/activity`; `lib/activity-components.ts`; workspace slice `choices`, `streamingPlan` |
| Streaming text | `feeds[chat].live`, superseded by the item's row | slice `streamingMessages/Reasoning/ToolOutput/Diff`, `streaming-message-batcher.ts`, lifecycle `message_delta` |
| Turn running, pending prompt, compaction, last exit (pane) | `feeds[chat].facts` | slice `working`, `compacting`, `turnRevision`, `settledPrompts`, `abandonedPrompts`; `use-pane-session`, `session-status.ts` |
| Busy (sidebar, tabs) | entity `working` in `ChatStore.chats`, server-folded | one copy per mounted workspace (`agentChats.working`), `readChatWorking`/`isChatWorking` scans |
| Chat entities | `ChatStore.chats` + `byWorkspace` index, one lifecycle subscription per endpoint | `agent-chats-slice.ts` per workspace, `use-workspace-agent-chats-stream.ts` per workspace |
| Unsent prompts | `ChatStore.outbox` (memory) | `use-prompt-queue.ts` + `lib/prompt-queue-persistence.ts` (localStorage) |
| Prompt state after send | rows (`user_message`, `turn_started`, `prompt_settled`) | lifecycle `prompt_settled` frames + 1 s busy recheck |
| Telemetry | `ChatStore.telemetry` | slice `telemetry` + `use-agent-telemetry.ts` |
| Providers | `useAgentProvidersStore` (window) | slice `providers` per workspace + seed retry |
| Excalidraw edit requests (composer ↔ preview) | `composer/lib/excalidraw-edit-requests.ts` (tiny store keyed by chat) | slice `excalidrawEditRequests` |
| Old read shapes | none | daemon `transcript` projector (S1 R13), `/messages`, `/activity`, payload, `/choices`, `/pending-prompt`, lifecycle event frames |

## Deletion table (S2 "Deletes" → task → files)

| Deleted | Task | Files (sources and their tests) |
|---|---|---|
| Ledger poll | T3 | `web/src/features/agent/hooks/use-chat-messages.ts`, `stores/chat-ledger-store.ts`, `lib/prefetch-visible-chats.ts` (+ its call in `web/src/lib/boot.ts`) |
| Activity poll | T3 | `hooks/use-agent-activity.ts`, `lib/activity-components.ts` (its test sits at `web/src/__tests__/features/agent/activity-components.test.ts`, not in the mirror), `transcript/activity-component-row.tsx`, `transcript/lib/flatten-transcript-rows.ts`, the `listChatActivity`/`getToolPayload`/`listChatMessages`/pending-choice reads in `api/agent-api.ts` (`getPendingPrompt` goes in T5, P12) |
| Busy poll | T5 | `hooks/use-prompt-queue.ts` (`BUSY_RECHECK_MS` interval, `AWAITING_TERMINAL_HINT_MS` timeout) |
| Streaming copy | T3 | slice fields `streamingMessages`, `streamingReasoning`, `streamingToolOutput`, `streamingPlan`, `streamingDiff` and their setters; `web/src/features/workspace/stores/hooks/lib/streaming-message-batcher.ts`; the `message_delta`/`plan`/`choice`/`compaction_*`/`prompt_settled` cases in `use-workspace-agent-chats-stream.ts` |
| Client prompt persistence | T5 | `lib/prompt-queue-persistence.ts`, `transcript/queued-row.tsx` (its rendering moves into the one user row) |
| Chat slice in workspace stores | T3 (event half), T4 (rest) | `web/src/features/workspace/stores/slices/agent-chats-slice.ts`, `stores/hooks/use-workspace-agent-chats-stream.ts`, `readChatWorking`/`isChatWorking`/`subscribeChatWorking` in `workspace-store-registry.ts`, the `agentChats` key in `workspace-store.ts`/`.types.ts`; `hooks/use-pane-session.ts`, `lib/session-status.ts` (with the dead `describeRung`, carry C7), `hooks/use-agent-telemetry.ts`; dead `tree/lib/chat-rows.ts`, `tree/lib/chat-search.ts` |
| Remount on settle | T3 | the `streaming ? <MarkdownMessage> : <MarkdownMessageStatic>` swap in `transcript/message-row.tsx` (R11); index keys |
| Scroll feedback loop | T6 | the settle gate, tail-room measurement and timers in `hooks/use-transcript-anchor.ts`, `hooks/use-scroll-frame-span.ts` |
| Duplicate first-prompt composer | T7 | `chat/agent-empty-document.tsx` editor half |
| R13 read shapes + LRU projection | T3/T5 (client), T11 (daemon) | `api/internal/app/usecases/chat/internal/transcript/` (whole package); handlers for `/messages`, `/activity`, `/activity/:toolId/payload`, `/choices` GET, `/pending-prompt` in both the repo mount (`endpoints/chat/routes.go`) and the home mount (`endpoints/home/routes.go`); `handlers/activity_components.go`; their DTOs in `api/internal/api/v0/dto/` (`agent.go`, `agent_activity.go`, `agent_choice_frame.go`, the event kinds in `agent_event.go`); lifecycle event frames; chat DTO `session` |

## Module keys (`scripts/ratchet/modules.json`) and counters

| Key | Direction | Why |
|---|---|---|
| `web.chat-surface` | falls 21,131 → ≤ 12,000 | the gate (R26 adds the diagram fence path; it is paid for inside the module) |
| `web.workspace-panes` | falls (≈ −1,300) | slice, workspace stream hook, batcher, registry scans deleted |
| `web.sidebar` | falls slightly | `readChatWorking` subscriptions become one selector |
| `web.git-review` | must not rise | `review-thread-item.tsx` split (R19) offset by comment trimming |
| `web.core` | ≈ −4 | `wsManager` URL resolver (+4, T3), `boot.ts` prefetch (−2, T3), `lib/ws/chat-frame-kinds.ts` event kinds (−6, T11) |
| `web.settings` | must not rise | `agent-providers-store.ts` gains no code; readers only |
| `web.total` | falls | sum |
| `web.tests` | falls | old suites of deleted modules (≈ 16k lines) go; new suites are smaller |
| `go.chat` | falls | `internal/transcript/`, read handlers |
| `go.dto` | falls | activity, choice, pending-prompt, component, `session` DTOs |
| `go.http` | ≈ 0 | routes deleted (−) vs retract and blobs routes (+) |
| `go.tests` | falls | the projector's and read handlers' tests |
| `web.timers` (counter) | falls by ≥ 12 (84 → ≤ 72) | 16 matches in `features/agent` today; 3 remain (R14) |
| `web.empty-catch`, `web.swallowed-promise` | must not rise | the feed has no swallowed errors: a failed attach is `status: 'error'` |

## Gate baseline (re-measured at `d8404d2e`, 2026-10-28)

Committed ratchet baselines (`scripts/ratchet/*-baseline.json`) still hold the S0 numbers: `web.chat-surface` 21,130 · `web.workspace-panes` 14,211 · `web.sidebar` 18,562 · `web.core` 5,979 · `web.git-review` 5,890 · `web.total` 105,828 · `web.tests` 113,678 · `go.chat` 38,317 · `go.dto` 2,165 · `go.http` 10,606 · `web.timers` 84 (counters).

Measured now (`node scripts/ratchet/loc-budgets.mjs`, read-only; it exits red): `web.chat-surface` 21,131 (+1) · `web.workspace-panes` 14,018 · `web.sidebar` 18,566 (+4) · `web.core` 6,058 (+79) · `web.git-review` 5,890 · `web.total` 106,191 (+363) · `web.tests` 116,743 (+3,065) · `go.chat` 32,230 · `go.dto` 2,171 (+6) · `go.http` 10,830 (+224) · `go.total` 129,700 (+1,738). Counters are green (`web.timers` 84, `go.timers` 21). The red keys are S1/S5/S6 growth waiting on the user's R7 ruling; S2 never raises them and reports its delta per R1.

`web/src/features/agent` now (physical lines, as the ratchet counts them; comment = lines starting `//`, `*` or `/*`):

| Directory | Lines | Comment | Code (no blank/comment) | T13 target |
|---|---|---|---|---|
| `activity` | 647 | 158 | 455 | ≤ 560 |
| `api` | 1,490 | 624 | 772 | ≤ 600 |
| `chat` | 1,944 | 654 | 1,209 | ≤ 700 |
| `components` | 1,480 | 580 | 833 | ≤ 700 |
| `composer` | 5,387 | 1,630 | 3,433 | ≤ 3,900 |
| `controls` | 877 | 187 | 649 | ≤ 720 |
| `hooks` | 3,122 | 1,249 | 1,704 | ≤ 600 |
| `lib` | 1,162 | 305 | 769 | ≤ 150 |
| `shared` | 241 | 31 | 183 | ≤ 230 |
| `stores` | 160 | 18 | 132 | ≤ 1,050 |
| `terminal` | 228 | 59 | 161 | ≤ 210 |
| `transcript` | 3,915 | 1,329 | 2,427 | ≤ 2,450 |
| `tree` | 478 | 173 | 272 | 0 |
| **total** | **21,131** | **6,997** | **12,999** | **≤ 12,000** (the targets sum to 11,870) |

Timers in `features/agent` today (16 matches of `\b(setTimeout|setInterval)\b`): `use-agent-activity.ts` 4, `use-chat-messages.ts` 2, `use-prompt-queue.ts` 2, `use-transcript-anchor.ts` 2, `use-scroll-frame-span.ts` 2, `working-line.tsx` 2, `subagent-shelf.tsx` 1, `use-slash-catalog.ts` 1.

## Review Focus

The inputs the spec implies but no feature test exercises, most likely to bite first. Each has a test in the owning task.

1. **A row committed while the feed attaches.** S1's replay dedupes on the server; the client must still apply each `seq` once and in order across a reconnect with `since`. Tests: T3 `applies a row once when it arrives in replay and live`, `resumes with since=seq after a resync frame naming the feed seq`.
2. **The settle that used to remount.** A text item streams, its complete row lands, the turn ends: the same DOM node must stay mounted with the live editor (R11). Test: T3 `keeps the live renderer mounted when a streamed reply settles` (asserts the row element is the same node before and after).
3. **Esc racing delivery.** Esc while the POST is in flight, and Esc at the moment `turn_started` lands. Tests: T5 `retracts after the send resolves when Esc came first`, `restores the text when the retract answers retracted even though turn_started arrived`; T2 `TestRegression_Symptom5_ALateAckAfterRetractIsRecordedAsAProviderTurn`.
4. **Daemon restart under a live chat.** A new `epoch` on `ready`: rows continue by `seq`, but live text is gone and must be dropped, not left frozen. Test: T3 `drops live text when ready carries a new epoch`.
5. **A long chat re-attached** (R17 drops feed state on detach). 5,000 rows replayed must fold without per-row re-sorting or re-rendering the list per row. Test: T3 `folds 5000 replayed rows into one order without re-rendering per row` (counts list commits with `React.Profiler`).
6. **Scoped rows before their card.** A child tool row whose scope's card has not been created yet (S1 R15 buffers on the server, but a `subagent{bound}` can follow the child rows by `seq` on replay). Test: T9 `attaches child rows that precede their card's bind`.
7. **A diagram fence split across deltas.** The opening fence, the body and the closing fence arrive in different frames, sometimes mid-line (` `` ` then `` `mermaid ``), and a reply can end with the fence open. Tests: T3 `shows a skeleton while a mermaid fence is open and renders the diagram once when it closes`, `falls back to plain code when the stream ends inside the fence`, `keeps the block element and animates its height from skeleton to diagram`.
8. **A client ahead of the daemon.** After a daemon reset the feed answers `resync{seq: head}` with `head < feed.seq`; reconnecting with the same `since` would loop. Test: T3 `rebuilds from since=0 when resync names a seq behind the feed`.

## Spec issues found while planning

1. S1 has no row for a retracted prompt and `TurnRetracted` has no writer. Settled by R6 (a `reason`, not a status).
2. S1's `FeedDelta.text` is everything so far, while the report and the additions doc require append-only deltas. Settled by R3.
3. S1's ask rows lack `answerable`, which the `/choices` read carried. Settled by R4.
4. Architecture §5 draws the outbox in the client, and "Prompts are ledger rows" removes the localStorage queue: together they mean unsent prompts die with the app. Settled by R7, surfaced as D1.
5. Report §4.4 asks for CSS bottom anchoring; the user's standing rule forbids losing the eased follow. Settled by R12.
6. Telemetry is not a ledger row in S1, so the ChatStore keeps one non-row field. Settled by R10.
7. `CLAUDE.md` says "conversations" live in `lib/store/`; the existing chat ledger store lives in `features/agent/stores/`. Settled by R2.
8. `CLAUDE.md` lists `bunx tsc --noEmit` as a CI check; `bunx tsc` resolves a different npm package. Use `bun tsc --noEmit`.
9. `features/agent ≤ 12k` is reachable only if rewritten and remaining files follow the comment rule: today's code without comments and blanks is already 12,996 lines. T12 exists for that; if T13 still misses, it reports the per-directory gap and never moves code out of `features/agent`.
10. ~~S1 T7 deletes the activity aggregate but does not list `api/cmd/crowbar-seed-chat`.~~ Closed: the command writes ledger rows (`cmd/crowbar-seed-chat/fixture_test.go` checks the kinds). T13's long-chat check uses it.
11. Additions §4 A–J name third-party products as references; S2's tasks describe the behaviour only.
12. The feed's frames use Go `omitempty` on `seq`: a `ready` of an empty chat and a `resync` at 0 carry no `seq`. The client reads absent as 0 (vocabulary).
13. `wsManager` cannot see close codes; R15 acts on frames instead.
14. Chat never rendered mermaid; R26/D9 make it render, with the skeleton rule.
15. Three test files S2 folds into hold more than 3 `vi.mock` today (R29).

## Open rulings (from the 2026-10-28 refresh; none blocks dispatch)

| # | Question | Recommendation |
|---|---|---|
| O1 | The pre-flight file (`.superpowers/sdd/2026-10-17-s2-chatstore-plan/preflight.md`, P1–P36) did not survive the move to the cloud; only the ledger's one-line summaries of P1–P3, P7–P13, P26, P31 did. R1, R7, T0, T3 and T5 fold those summaries as read here (e.g. P11 "steerable prompts persist as ledger rows" read as: a steerable prompt goes out at once with `delivery: steer`). | Dispatch on this text. If the controller still holds `preflight.md`, diff its P4–P6, P14–P25, P27–P30, P32–P36 against the task text before T3. |
| O2 | T4 builds `ChatStore.chats/byWorkspace/scopes/telemetry` and R15's `url` resolver; S4 (b, c) deletes both. | Keep T4 and R15 as planned: S4 waits on S3 T8 and its own refresh, and leaving the K-fold workspace slice in place until then keeps symptom-adjacent bugs alive. Keep T4's entity half minimal (no features beyond today's), so S4c's delete is mechanical. |
| O3 | R26's "no snap" is checked in jsdom only through `element.animate` being called with from/to heights on the same node; the visual result needs the live check. | Add the fence to T13 step 9 (done below) and accept jsdom's structural assertion as the unit test. If the WebKitGTK webview lacks the Web Animations API height interpolation, fall back to a CSS `transition: height` on an explicit pixel height set from the measured skeleton (no timer). |
| O4 | C27's "no raw daemon text" is wider than chat: after S3 T1 every problem message ends in its category text (e.g. `: conflict`). | S2 fixes the cases C27 names (session-lost send/resume/switch, via the `session_lost` code: never `problem.message` for it). The generic envelope is S3's (`report()`, landed in S3 T2). |
| O5 | Should the client cap a `tool_output` live text? The daemon windows it (64 KiB tail; once it passes 2× the window it sends a `replace` with the tail). | No client cap: the daemon's `replace` bounds it. T3's fold test covers a `replace` after appends. |
| O6 | S1's gate missed (S1 ledger, T13 Step 3) and a "post-S2/S4 shrink pass" is queued; `loc-budgets` is red for keys S2 touches (`web.core` +79, `web.sidebar` +4, `web.tests`). | S2 must not grow them (R1) and T13 reports them; it does not try to repay S1/S5 growth. |

## Pre-flight log (2026-10-28, against `b79be6f2`)

Source: an opus pre-flight of this refreshed plan, 25 findings (blockers F1–F4; should-fix F5–F17; nits F18–F25). Each finding's evidence was re-checked against the code at `b79be6f2` before it was applied. Only line numbers had drifted: `streaming-markdown-parse.ts` is 128 lines (`FENCE` `:22`, close test `:57-61`, open `:79`, BACKWARD return `:66`), and `use-prompt-queue.ts`'s busy recheck calls `onRefreshChat` at `:271`.

- Applied (25): F1 (R6, R27, s2a T1/T2), F2 (R5, s2a T1), F3 (s2b T0; S3 A9, s3c T5), F4 (s2e T11), F5 (R15, s2b T3), F6 (vocabulary, s2b T3), F7–F10 (R26, s2b T3), F11 (s2b T3, s2c T5, deletion table), F12 (s2b T3), F13 a–f (s2b T0/T4, s2c T5, s2e T11, Dependencies; S3 A3), F14 (R27, s2c T5), F15 (s2d T8/T9), F16 (Global Constraints, s2e T13), F17 (s2c T7), F18 (R19, s2d T8), F19 (s2b T4), F20 (s2b T3 sizes, R27, O4, s2d T10), F21 and F22 (s2d T10, s2a T1/T2), F23 (R8), F24 (R26), F25 (s2d T9).
- Rejected: none.
- Found while verifying (applied): F3's four A9 parts cannot all be under 500 by moves alone: chat reads plus the activity read is ~760 lines. T0 adds a fifth part, `agent-chat-activity.ts`, which T3 mostly deletes; S3 A9 and s3c T5 name it.

Rulings:

- Ruling: S2 T0 splits `agent-api.ts` now with A9's part names (`agent-chat-reads.ts`, `agent-chat-commands.ts`, `agent-chat-creates.ts`, `agent-providers-api.ts`, plus `agent-chat-activity.ts`), and S3 T5 follows those parts (S3 A9 amended; S3 T3's Go work is in flight, and S3's web tasks T5–T7 come later). Why: S2 T3 must edit the file and R19 forces it under 500; gating T3 on S3 T5 would block S2 behind S3 lane A. Cost if wrong: S3 T5 renames imports in the files it touches.
- Ruling: `session_lost` is a problem code (`domain/problem/code.go`, 409; web `ProblemCode`). `ErrSpawnSessionRefused` and the new `ErrPromptSessionLost` carry it, `spawnFailure` maps the first to the second, and `handlers/switch.go` is unchanged. Why: `libs.Fail` already answers resume and switch from the error's code, so the code is the one owner; no handler mapping is needed. Cost if wrong: one more specific code in the enum.
- Ruling: Retry mints a fresh request id after `request_id_conflict`, `request_outcome_uncertain` or `session_lost`, and keeps the same id after a network error or 5xx. Why: the daemon refuses a reused id whose outcome it already decided, and dedupes a resend of one it never saw. Cost if wrong: a network-failure retry lands as a duplicate 409 the user retries again.
- Ruling: no hand-built rows fixture. The recorded subagent session's rows are generated by a black-box test that replays the real hook payloads through the real HTTP ingress and translation, reads `kit.FeedRows`, normalizes run-specific values in code, and writes `web/src/__tests__/__fixtures__/claude-subagent-session.rows.json` only under `-update-rows-fixture` (it fails on drift otherwise; s2d T8). Why: the handoff forbids fabricated or edited fixtures, and the web tests need the session's real rows. Cost if wrong: one bbx test and one committed JSON file to refresh when the translation changes.
- Ruling (R26, F7–F10): reduced motion is checked with `matchMedia` before `element.animate`; the skeleton stays until `MermaidDiagram` reports success or error; diagram routing applies only inside the transcript's `StreamingFenceContext`; the static renderer is told the fence is open; the shared open/close fence rule is extracted, not exported; a half-typed diagram info string counts as a diagram. Why: the CSS kill-switch does not reach WAAPI; switching on fence close would snap twice through mermaid's debounce placeholder; the composer shares `ChatCodeBlockElement`; remark cannot see an unclosed fence; and one rule must decide fences for both the stream cursor and the skeleton. Cost if wrong: a few lines in `diagram-block.tsx` and `streaming-markdown-parse.ts`.
- Ruling: T11 moves the open-ask fold to `ledger.State.OpenAsks()` and keeps `domain.ActivityChoice` for the answer desk, termwait and `AnswerChoice`; `broadcastChoices`/`seam.Choices` go with the `choice` frame (F4). Why: the smallest change that deletes the projector without rewriting three kept consumers. Cost if wrong: a later stage swaps `ActivityChoice` for `ledger.Ask`.
- Ruling: T13's live check runs in the noVNC container, seeds with `go run ./cmd/crowbar-seed-chat`, and the user re-runs the WebKitGTK height-animation check on the desktop (F16). Why: no Tauri MCP here, and the handoff forbids make targets for these runs. Cost if wrong: a desktop-only difference shows up only in the user's run.
