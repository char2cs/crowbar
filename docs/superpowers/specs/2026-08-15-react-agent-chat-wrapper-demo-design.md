# React agent-chat presentation and delivery

**Date:** 2026-08-15

**Status:** production specification; implementation present, final verification pending

**Providers in the initial production mapping:** Claude Code and Codex

**Verification record:** §16; keep every row `PENDING` until that command or scenario has
actually been completed and its evidence recorded

This document is the production contract for presenting a Crowbar agent chat in React while
the provider continues to run as its real interactive terminal UI. It replaces the earlier
demo framing: the React surface is a supported presentation and delivery path, and the native
terminal remains an equally supported view and the compatibility fallback.

It amends the input rule in
[`2026-07-05-crowbar-agentic-engine-design.md`](./2026-07-05-crowbar-agentic-engine-design.md).
Crowbar still does not inject keystrokes into an unknown TUI state. When a completed React
prompt reaches the head of the queue, Crowbar gracefully replaces the current interactive
provider process, resumes its native conversation when safe, and passes the prompt through the
provider's interactive positional-prompt argv contract. The replacement remains a normal TUI
in a PTY before, during, and after the turn.

---

## 1. Product contract

Every agent-chat pane has two interchangeable presentations of one Crowbar chat:

1. **Chat** renders complete hook-confirmed messages, server-folded working state, a durable
   client FIFO, and an on-demand deterministic skill picker.
2. **Terminal** attaches to the same live runner PTY and exposes the provider's native TUI
   without emulation.

Both presentations stay mounted. Changing presentation does not change provider, conversation,
runner, or chat. The provider TUI stays alive while the user composes or queues text. It is
replaced only when the backend dispatches a completed prompt, and the replacement TUI stays
alive after the answer so the user can return to Terminal immediately.

The production outcome is:

- complete user and assistant messages are rendered from Crowbar's hook-derived ledger;
- typing remains available while the provider works; completed prompts wait in FIFO order;
- only the FIFO head is sent, and only after server state says the chat is idle;
- a leading `/` starts a fresh, deterministic provider CLI probe without a model turn;
- skill invocation syntax is mapped below Crowbar's provider boundary;
- provider switching and prompt replacement remain serialized per chat;
- a missing or drifting React capability degrades to the still-live native terminal.

## 2. Non-negotiable invariants

1. **Interactive provider only.** Claude `-p`/`--print`, Codex `exec`, provider SDKs,
   app-server, remote app-server, and equivalent headless bridges are forbidden here.
2. **Real PTY throughout.** Every hosted provider process is the ordinary interactive CLI in a
   real PTY. A positional prompt may start its first turn, but may not make it a one-shot process.
3. **No provider-owned file reads.** Crowbar does not inspect a provider's config, skill, plugin,
   transcript, or session directories. The provider CLI may read its own state and return a
   deterministic command result.
4. **Provider-neutral layers.** Domain objects, usecases, REST DTOs, queue state, and React
   components never branch on `claude` or `codex`. Provider-specific facts live in Crowbar-owned
   descriptors and are interpreted through generic adapters.
5. **Hooks are authoritative for conversation text.** POST success never manufactures a user
   message. Only mapped `user_prompt` and `turn_stop` hooks append user and assistant text.
6. **No fake streaming.** React displays complete hook messages. It does not scrape terminal
   frames, diff the screen, or render a final hook as token deltas.
7. **Server state gates dispatch.** `AgentChat.Working`, including provider-reported asynchronous
   work, decides whether the client may dispatch. A lifecycle event name is never reinterpreted
   as busy or idle in React.
8. **At-most-once before convenience.** Any post-intent outcome Crowbar cannot prove is returned
   as uncertain and is never automatically replayed.
9. **Catalog probes spend no model tokens.** Only descriptor-declared deterministic provider
   subcommands may populate the picker.
10. **Catalogs are ephemeral.** Raw or normalized catalog output is not added to the ledger,
    database, provider config, or a backend cache.
11. **Terminal fallback is permanent.** React capability absence, partial catalogs, trust prompts,
    permission prompts, authentication, and provider UI drift must not make the chat unusable.

## 3. Authorities and data ownership

| Concern | Authority | Consequence |
| --- | --- | --- |
| process liveness | terminal engine / PTY | no second `isLive` flag is invented |
| chat placement | durable runner row | hooks resolve runner → current chat at ingestion time |
| native conversation | provider session id bound to the runner | fresh/resume is decided before replacement |
| busy state | folded `AgentChat.Working` plus in-flight hook guard | React queues; backend rechecks under the spawn gate |
| conversation order | Crowbar ledger append sequence | REST pages and reconciliation use chat-local sequence |
| accepted prompt text | `user_prompt` hook | POST success means only that the replacement TUI spawned |
| completed answer | `turn_stop` hook | empty final text creates no blank assistant message |
| pending browser drafts | that pane's versioned local queue | never confused with accepted ledger messages |
| retry identity | durable backend request journal | same UUID and text refer to one logical dispatch |
| skill inventory | live provider command output | every picker opening probes again; no backend snapshot |

The three runtime channels intentionally remain separate:

```text
React composer ─ POST prompt ─► durable intent ─► replace same-provider TUI in PTY
                                                   │
provider hooks ─ session_start / user_prompt / turn_stop ─► mapped ledger + working fold

React `/` picker ─ GET catalog ─► deterministic provider CLI command ─► descriptor mapper

Terminal presentation ─ attach-only ─► the same live PTY
```

Hooks observe provider decisions; they are not an input protocol. Deterministic commands report
capabilities; they do not own chat history. The PTY hosts interaction; its screen is not parsed
as a domain event stream.

## 4. Provider descriptor boundary

`Descriptor.Presentation` is optional. A provider with no presentation capability remains a
valid terminal-only provider.

```yaml
presentation:
  prompt_submit:
    strategy: restart_tui
    fresh:
      - pass_arg: { positional: "{message}" }
    resume:
      - pass_arg: { positional: "{message}" }

  slash_catalog:
    completeness: complete | model_visible | plugin_only
    timeout_ms: 10000
    max_stdout_bytes: 4194304
    max_stderr_bytes: 262144
    max_items: 200
    pipeline:
      adapter: json_text_section | json_inventory_text_detail
      # adapter-specific fixed argv and extraction mapping
```

### 4.1 Prompt mapping

The descriptor validator enforces all of the following:

- only `restart_tui` is accepted;
- both `fresh` and `resume` are non-empty;
- prompt steps use the closed `pass_arg` vocabulary, never a shell;
- `{message}` occurs exactly once in each path;
- provider option parsing is terminated before `{message}`, so a prompt beginning with `-` is
  always text and can never select `--help`, print/headless mode, permissions, or another flag;
- a resume-capable session mapping exists.

The spawn planner owns argument placement. Normal provider args, Crowbar hook injection, optional
MCP injection, permissions, environment, native resume arguments, and context injection are
assembled first; the completed prompt is appended as one final literal argv element. This is
load-bearing for Claude's variadic `--mcp-config` and for Codex's `resume <id> <prompt>` ordering.

Prompt submission is same-provider continuation, not a handoff. It does not synthesize a
cross-provider transcript or apply `resume_context_inject`; the provider's native conversation
already owns its own history.

### 4.2 Generic catalog mapping

The engine supports output shapes rather than provider ids:

- `json_text_section` selects strings from JSON, extracts a literal-delimited section, matches
  named regex captures, and maps each item;
- `json_inventory_text_detail` selects enabled JSON inventory rows, fans out a fixed detail
  argv template with one provider-reported `{id}`, parses a bounded text list, and maps items.

A descriptor maps candidates into the provider-neutral result:

```text
SlashCatalog
  providerId: string
  completeness: complete | model_visible | plugin_only
  items: SlashCatalogItem[]
  warnings: string[]

SlashCatalogItem
  id: string
  kind: skill
  label: string
  description: string
  insertText: string
  source: string
```

`insertText` belongs below the provider boundary. React never decides whether an invocation is
`$name` or `/plugin:name`. Filesystem locators and raw provider output are not fields in this DTO.

## 5. Shipped provider mappings

### 5.1 Codex

- Prompt: one final positional `{message}` for fresh and native resume launches.
- Probe: `codex debug prompt-input`.
- Adapter: `json_text_section`, selecting model instruction text between
  `<skills_instructions>` markers.
- Insertion: `$name `.
- Completeness: `model_visible`.

`model_visible` is intentionally not `complete`: the deterministic command reports skills in
the constructed model instructions and may differ from Codex's native menu. Returned source is
the logical label `model-visible`; provider-owned locators are stripped.

### 5.2 Claude

- Prompt: one final positional `{message}` for fresh and native `--resume <id>` launches.
- Inventory: `claude plugin list --json`.
- Detail: `claude plugin details <id>` for each enabled row, with concurrency at most four.
- Adapter: `json_inventory_text_detail`.
- Insertion: `/plugin:skill `, derived from provider-reported logical source and skill name.
- Completeness: `plugin_only`.

`plugin_only` is intentionally not `complete`: the deterministic command accounts for enabled
plugin skills but not standalone user or project skill directories. A malformed detail result is
reported as a warning where safe; a global timeout, command absence, or shared output-budget
breach fails the probe.

### 5.3 Completeness presentation

- `complete` means the provider command contract claims the entire native user-visible catalog.
- `model_visible` means the items were found in provider-constructed model instructions.
- `plugin_only` means enabled plugin inventory only.

React displays the returned warnings and offers the native terminal menu for the provider's exact
surface. It never fills gaps by asking a model, walking provider directories, or persisting old
answers.

## 6. Hook-derived ledger and message API

The descriptor maps provider payloads into three canonical events:

| Event | Durable effect | UI effect |
| --- | --- | --- |
| `session_start` | bind/move the runner's native conversation | refresh runner/chat identity |
| `user_prompt` | append full mapped user text; open turn; confirm matching request journal row | authoritative user bubble and working state |
| `turn_stop` | append non-empty full mapped assistant text; close turn/update async work | authoritative assistant bubble and working state |

The ledger is Crowbar-owned, per chat, append-only, and provider-tagged. Hooks append under a
shared per-directory mutex so independently opened ledger handles cannot select the same sequence.
Each entry is written to a temporary file, synced, and atomically renamed. Internal runner id is
stored for delivery correlation but omitted from the public DTO.

The ledger directory itself must be synced after rename so an acknowledged hook append survives
an operating-system crash; file fsync without directory fsync is not the complete durability
boundary on filesystems that require both.

The read endpoint is mounted on both normal workspace and project-home agent surfaces:

```http
GET <agent-base>/chats/:id/messages?after=<sequence>&before=<sequence>&limit=<n>
```

`<agent-base>` is either:

- `/v0/projects/:projectId/repos/:repoId/workspaces/:wsId/agent`; or
- `/v0/projects/:projectId/home/agent`.

The success envelope is:

```json
{
  "success": true,
  "data": {
    "cursor": 12,
    "oldestCursor": 7,
    "hasMore": false,
    "items": [
      {
        "sequence": 12,
        "role": "assistant",
        "providerId": "codex",
        "text": "Complete hook-mapped reply",
        "at": "2026-08-15T12:00:00Z"
      }
    ]
  }
}
```

Rules:

- sequences are positive and chat-local;
- `after` and `before` are non-negative and mutually exclusive;
- default limit is 100; accepted range is 1–200;
- no cursor returns the newest page in chronological order;
- `after` pages toward newer messages; `before` pages toward older messages;
- `hasMore` is directional and `oldestCursor` anchors upward pagination;
- the endpoint never reads provider transcripts or exposes raw hook payloads.

The existing lifecycle WebSocket remains a notification stream. Every turn frame carries the
server-folded `working` value. A client-side `turnRevision` advances even for a fast
true→false pair that React could batch, ensuring a message refresh is not lost. React also polls
the ledger once per second while working or awaiting acceptance. A lifecycle WebSocket reconnect
must explicitly trigger a message refresh even when folded `Working` is idle before and after the
outage; reseeding chat rows alone does not reveal an idle→idle turn missed while disconnected.
Every incremental `after` read must continue while `hasMore` is true; otherwise a hidden or stalled
client can skip the user message that confirms an awaiting queue row when more than one page
arrived between reads.
On reload, reconciliation must also read backward far enough to cover the oldest persisted pending
`baselineSequence` (or ask a backend request-status endpoint). Loading only the newest page can
strand a row whose confirmation is older than 100 messages until the user manually pages upward.

Hook ingress must be recoverable as well as bounded. A transient ledger or journal write failure
must not become a permanently lost `user_prompt` merely because the provider ignores one non-2xx
hook response. Production completion therefore requires an idempotent hook delivery identity plus
bounded retry, or an equivalent durable ingress/reconciliation mechanism.

## 7. Prompt submission protocol

### 7.1 API

```http
POST <agent-base>/chats/:id/prompts
Content-Type: application/json

{
  "text": "completed prompt",
  "clientRequestId": "9d1a5551-8145-46a1-bf09-b99d39163341"
}
```

Prompt text must be non-blank, contain no NUL (which cannot exist in an OS argv element), and be at
most 64 KiB in UTF-8. `clientRequestId` must parse as a UUID and is normalized before journaling.

A `200` response means the replacement interactive TUI has a durable runner and terminal
identity. It does **not** mean the provider accepted or answered the prompt.

```json
{
  "success": true,
  "data": {
    "runnerId": "...",
    "terminalSessionId": "..."
  }
}
```

### 7.2 Dispatch algorithm

The backend holds the per-chat spawn gate for the whole operation:

1. Validate text and request UUID.
2. Load the scoped chat and durable request journal.
3. Resolve an existing same-id attempt before checking busy state.
4. Resolve the live runner, its provider descriptor, worktree, ledger, and native session.
5. Select `resume` when the runner's durable launch identity proves an explicit resume, or when
   its current native conversation has a real provider turn at/after its bind timestamp.
   Otherwise select `fresh`; an announced but never-used session is not assumed resumable.
6. Render provider-neutral prompt steps and native resume steps through the descriptor.
7. Re-read live placement and `Working` immediately before intent persistence.
8. Preallocate the replacement runner UUID and persist it with a `dispatching` journal intent
   before touching the provider process.
9. Take the chat's turn-start interlock, re-read idle state after the intent sync, and keep that
   interlock through outgoing displacement. A native `user_prompt` either starts its turn first
   and makes this request busy, or displacement wins and that old runner can no longer start a
   turn on this chat.
10. Gracefully stop and displace the outgoing TUI, then release the turn-start interlock.
11. Start the same provider as a normal interactive TUI in a new PTY, preserving hooks, MCP,
    permissions, cwd, environment, and native session choice; append the prompt as final argv.
12. Persist runner/terminal identity as `spawned` and return it.
13. Leave the TUI alive. Its `user_prompt` hook changes the journal to `accepted` and writes the
    only authoritative user message; `turn_stop` later writes the assistant message.

The provider process is not restarted while the user is merely typing or while a prompt waits
behind a working turn. Selecting Terminal prevents a new queued dispatch from starting; it does
not cancel a mutation already in flight.

### 7.3 Startup hook barrier

There is an unavoidable interval between forking the provider and committing its runner row.
A positional prompt can cause `session_start` and `user_prompt` hooks inside that interval.
Dropping them would lose the prompt, wedge the FIFO, or misclassify delivery.

Crowbar closes the interval as follows:

- register the future runner id immediately before `CreateCommand`;
- buffer hooks for that id before consulting the runner repository;
- cap one startup entry at 64 hooks and 32 MiB total raw payload;
- after `recordRunner` commits, replay buffered hooks in arrival order;
- keep the barrier installed during replay, draining later arrivals in subsequent batches so
  they cannot overtake earlier hooks;
- remember an early PTY exit, then reconcile it only after persistence and replay;
- discard the entry on a pre-process or persistence failure.

Normal unknown-runner hooks remain ignored. A hook cannot create or resurrect a runner.

## 8. Durable request journal and delivery states

Each chat has a Crowbar-owned `prompt-requests` journal. One file per normalized request UUID is
committed by temp-file write, `0600` permissions, fsync, atomic rename, and directory sync.
The record stores:

- request UUID;
- SHA-256 of prompt text, never plaintext;
- provider id and outgoing runner id;
- replacement runner and terminal ids when known;
- timestamps and one state: `dispatching`, `spawned`, `accepted`, `failed`, or `uncertain`.

The journal is bounded to 4,096 records and 30 days, pruning only inactive records. Within a
retained record its contract is durable at-most-once dispatch, not exactly-once model execution:

| State | Meaning | Same-id retry |
| --- | --- | --- |
| `dispatching` | durable intent and preallocated replacement runner id exist; terminal identity/result is not yet committed | recover from hook evidence, otherwise return uncertain |
| `spawned` | replacement runner and PTY are committed; `user_prompt` not yet correlated | return original spawn if identity is complete, otherwise reconcile/uncertain |
| `accepted` | matching provider/runner/text-digest `user_prompt` was observed | return original complete result, or `request_already_accepted` when identity was lost |
| `failed` | failure was proven before a replacement process could accept the prompt | safe to retry same UUID and text |
| `uncertain` | a process may have accepted the prompt but proof is incomplete | never auto-replay; inspect ledger/terminal and retry same id only for recheck |

Reusing a UUID with different text is always a conflict. A second request cannot pass a live
`dispatching`/`spawned` delivery. Provider switch checks the same journal under the same spawn
gate, so another window cannot tear down a replacement between spawn and `user_prompt`.

Hook confirmation correlates provider, Crowbar-assigned runner, and text digest. A same-text hook
from the outgoing runner cannot confirm the replacement. A runner exit reconciles the journal:
matching ledger evidence wins; otherwise the record becomes uncertain because an already-issued
hook request can race process exit and arrive after the PTY is gone.

At daemon boot, dead runner rows are reconciled against actual PTY liveness first. Surviving
`dispatching` intents from the crash window become `uncertain` so they cannot block every future
request forever. The same transition is applied lazily when a later same-id lookup proves the
original synchronous dispatch section no longer exists.

Journal expiry is part of the client contract, not an invisible implementation detail. An
uncertain browser row must never offer a same-id retry after its backend record may have been
pruned; absent that knowledge, the same UUID would be interpreted as a fresh operation. Release
requires an aligned expiry/tombstone protocol or retention policy that never prunes an unresolved
request while the UI can still retry it.

## 9. Stable prompt errors

Errors use the ordinary envelope:

```json
{
  "success": false,
  "error": "human-readable message",
  "code": "request_outcome_uncertain"
}
```

`code` is present for prompt recovery categories; clients branch on it, never message text.

| HTTP | Code | Meaning / client action |
| --- | --- | --- |
| 400 | omitted | blank/oversized text, malformed UUID, or invalid request shape; definitive failure |
| 404 | omitted | chat is unknown or outside the scoped workspace; definitive failure |
| 409 | `chat_busy` | native turn or another delivery won the gate; keep FIFO head until a real idle observation |
| 409 | `request_outcome_uncertain` | provider may have accepted; block FIFO, refresh messages, require human recovery |
| 409 | `request_already_accepted` | positive hook evidence exists; do not resend, refresh until ledger confirms |
| 409 | `request_id_conflict` | UUID was used for different text; editing must create a new request UUID |
| 422 | `prompt_submit_unsupported` | descriptor has no valid React submit capability; latch this provider to Terminal |
| 422 | `live_tui_required` | no live provider TUI currently exists; transient, offer Resume/Terminal without disabling capability |
| 424 | omitted | provider executable is unavailable before a replacement starts; safe, actionable failure |
| 500 | omitted | unexpected failure with no stronger public classification; client treats mutation outcome conservatively |

Once replacement startup may have occurred, internal lookup, spawn, or journal-commit failures are
collapsed to `request_outcome_uncertain`/409 even when their underlying repository error would
normally map elsewhere. Internal detail is retained in structured daemon logs, not leaked through
a misleading retryable response.

## 10. Frontend durable FIFO

The queue is a versioned, per-`workspaceId`/`chatId` client document in `localStorage`. It is
bounded to 100 prompts; each prompt uses the same 64 KiB text limit as the backend. A row carries
text, UUID, creation/submission times, first-attempt ledger baseline, optional idle epoch, error,
and one client state:

- `queued`: may dispatch only when it is the head, the server says idle, a live TUI exists, and
  the Chat presentation is visible and active;
- `submitting`: the mutation is in flight;
- `awaiting_turn`: replacement spawned or already-accepted was reported; wait for hook evidence;
- `failed`: a definitive pre-dispatch failure; human may edit, cancel, or retry;
- `outcome_uncertain`: delivery may have occurred; no automatic replay and no ordinary Edit
  action that would quietly mint a duplicate operation.

Rules:

- Enter queues and clears the composer even while working; Shift+Enter inserts a newline.
- Only the head moves. Awaiting, failed, and uncertain heads prevent later prompts overtaking.
- Queue rows remain visible and cancellable; queued/definitively failed rows are editable.
- A retry preserves `clientRequestId`, original `submittedAt`, and the first attempt's
  `baselineSequence`, so late hook evidence remains discoverable.
- `chat_busy` waits for a subsequent idle epoch. A confirming GET may supply an idle edge missed
  by the WebSocket; otherwise the queue does not spin.
- Ledger reconciliation removes submitting, awaiting, or uncertain rows only when a later user
  message has the same trimmed text and sequence greater than the saved baseline. Provider/runner
  correlation remains a server-journal guarantee; the browser queue does not receive internal
  runner attribution.
- A transport rejection, unexpected 5xx, live-runner disappearance, or ambiguous 409 becomes
  uncertain. Only explicit 400/404/422/424 responses are treated as definitive failures.
- Reload promotes an in-flight `submitting` row to `outcome_uncertain`; it never blindly replays.
  Queued and awaiting rows survive. Corrupt/oversized documents are discarded safely.
- Persistence denial or quota exhaustion leaves the in-memory FIFO usable and visibly warns that
  reload durability was lost.
- Terminal presentation shows the pending count with Return-to-Chat and cancellation controls.
  Bulk cancel may remove only rows never dispatched; submitting, awaiting, and uncertain recovery
  rows require a separate explicit confirmation that explains the lost recovery affordance.
- Provider switching is disabled while submission or acceptance is pending. An otherwise queued
  prompt targets whichever provider is active when it eventually dispatches.
- After six seconds awaiting a user hook while the replacement is live and apparently idle, the
  row suggests that trust or permission UI may be waiting and offers **Open Terminal**.

### 10.1 Multi-window contract

The backend, not browser storage, supplies cross-window safety:

- all prompt submissions and provider switches serialize on the same per-chat spawn gate;
- durable UUID/text idempotency deduplicates the same logical request across windows;
- a pending delivery journal row blocks a second logical dispatch and destructive provider switch;
- lifecycle frames and authoritative GETs converge every window on server working/runner state.

The current queue is **not** a collaborative server queue and localStorage is not used as a lock.
Each mounted pane owns its in-memory FIFO after hydration. Separate windows can have different
draft views; writes to the same localStorage key are last-writer-wins. Therefore production safety
guarantees no duplicate automatic delivery and no cross-window process race, but does not promise
cross-window draft merging. If synchronized drafts become a requirement, they need an explicit
server queue or a versioned merge/lease protocol; storage events alone are not an ordering
authority.

The server also cannot observe an unsent draft sitting in the native TUI input buffer. The active
pane prevents its own React FIFO from dispatching while Terminal is selected, but another window
can still request a prompt replacement or provider switch while the chat is server-idle. A true
multi-window single-writer guarantee would require a chat writer lease/presence protocol; process
serialization alone cannot preserve an invisible native draft.

## 11. Slash picker lifecycle and API

On the transition into a leading `/`, React waits 150 ms and calls:

```http
GET <agent-base>/chats/:id/slash-catalog
```

The backend resolves the **current live runner** and descriptor, executes the probe in the exact
chat worktree, and maps output in memory. One probe may be active per chat; a newer request
cancels and supersedes the older one. React filters the one response locally while the user types.
Selection replaces the query with `insertText` and never submits it.

Before returning, the backend must re-read live placement and reject a result whose runner or
provider no longer owns the chat. The frontend must also compare `catalog.providerId` with its
current provider. A provider switch can otherwise complete while a ten-second old-provider probe
is still running, allowing a delayed lifecycle frame to expose stale invocation syntax.

Escape, deleting the leading slash, changing provider/chat, selecting Terminal, closing the menu,
or unmounting aborts the request. Reopening starts a new command; neither layer reuses a backend
catalog snapshot.

Catalog errors are stable:

| HTTP | Code | Meaning |
| --- | --- | --- |
| 422 | `catalog_unsupported` | descriptor declares no deterministic probe |
| 422 | `catalog_live_tui_required` | no live provider runner identifies the catalog owner |
| 504 | `catalog_timeout` | bounded probe deadline expired |
| 424 | `catalog_command_unavailable` | provider executable is missing |
| 502 | `catalog_output_limit` | stdout/stderr budget exceeded |
| 502 | `catalog_command_failed` | deterministic provider command failed |
| 502 | `catalog_malformed_output` | output did not satisfy the descriptor mapper |
| 409 | `catalog_superseded` | a newer probe replaced this one |

Every error and every partial completeness label offers Terminal for the provider's native menu.
The picker models skills only; native commands, approval dialogs, model selection, `/clear`,
`/resume`, `/compact`, and provider settings stay terminal-native.

## 12. Probe safety and resource bounds

The browser never launches an executable. The daemon uses:

- executable from `Descriptor.Spawn.Cmd`, resolved on the daemon's effective PATH;
- fixed descriptor argv plus one escaped `{id}` detail substitution as one argv element;
- no shell and no user-supplied command fragments;
- the chat's absolute worktree as cwd;
- daemon environment after descriptor `spawn.env.clear`, with `PWD` reset to that worktree;
- timeout at most 10 seconds;
- one shared 4 MiB stdout budget and 256 KiB stderr budget across the pipeline;
- at most 200 normalized items and four concurrent detail commands;
- a daemon-wide semaphore bounding aggregate probe/detail processes across chats;
- label/source limits of 256 runes, insertion limit of 512 bytes, description limit of 2 KiB,
  warning limit of 512 bytes, and at most 64 warnings;
- cancellation/timeout kills the direct provider command; a 500 ms `WaitDelay` prevents inherited
  stdout/stderr pipes from stranding the HTTP request indefinitely.

Provider output used as detail `{id}` is stripped of controls, bounded, rejected when empty,
option-like, or malformed, and remains one argv value. Returned labels/descriptions/warnings redact
common filesystem-path and credential shapes. Sources containing path syntax are dropped. Raw
stdout/stderr, argv, cwd, and provider error output are neither returned nor info-logged.

Probe failure never stops, replaces, or mutates the live provider TUI.

Before release, cancellation must also prevent provider probe descendants from continuing as
orphans. `exec.CommandContext` plus `WaitDelay` returns control but is not itself a process-group or
job-object kill for descendants.

## 13. React and terminal presentation behavior

Chat is the default presentation for a newly mounted pane. The choice is per mount, not a global
preference. Both surfaces remain mounted so the queue and xterm screen model survive toggles.

Chat includes:

- paged hook-confirmed messages, chronological ordering, and retryable read errors;
- provider attribution where a change/handoff needs clarification;
- the server-folded working spinner;
- visually separate queued/submitting/awaiting/failed/uncertain rows;
- multiline composer and keyboard-accessible skill picker;
- provider switch, disabled during replacement/acceptance-sensitive windows;
- explicit Resume/Terminal fallback when no live runner exists.

Terminal is the existing `XtermTerminal` in `attachOnly` mode. It attaches to the live runner's
terminal session and may never fall back to spawning a bare shell. Runner replacement is adopted
imperatively and runner movement is followed through the existing lifecycle projection.

Trust, authentication, permissions, plugin UI, and other native modals are intentionally not
inferred from hooks. If the expected `user_prompt` is delayed, Chat does not write into the PTY;
it explains the likely native interaction and opens Terminal. The user can answer there and return
to Chat without losing the queue.

## 14. Ordering and race resolution

| Race | Required result |
| --- | --- |
| React observed idle, native TUI starts a turn before POST gate | backend returns `chat_busy`; head waits for a real idle observation |
| two windows submit different requests at once | spawn gate and journal allow at most one delivery; the other is busy/uncertain, never silently parallel |
| two windows submit the same UUID/text | durable journal returns/reconciles the first logical attempt |
| same UUID is reused for different text | `request_id_conflict`; no process mutation |
| provider switch overlaps dispatch | same spawn gate serializes them and switch's journal guard protects spawn→hook acknowledgement |
| provider emitted `turn_stop` but reports background work | folded `AgentChat.Working` remains authoritative; switch waits and must not kill the provider/subagents |
| provider changes while a prompt is merely queued | queued prompt uses provider active at dispatch |
| first hook arrives before runner persistence | startup barrier buffers and replays it in arrival order |
| PTY exits before runner persistence | exit is remembered, hooks replay, then durable runner is reconciled dead |
| outgoing runner emits identical user text while dying | runner correlation prevents it confirming the replacement request |
| replacement exits before `user_prompt` | matching ledger evidence wins; without it backend and frontend remain uncertain because a hook request may still be in flight |
| replacement accepts, HTTP response is lost | same-id retry returns prior result or accepted/uncertain; frontend never creates a new id automatically |
| daemon crashes after durable intent but before a durable result | boot marks the surviving dispatch uncertain; no duplicate replay and no permanent chat-wide lock |
| another source appends indistinguishable user text after a browser row's baseline | server journal remains correctly runner-correlated; browser text-only reconciliation may clear that local row and is an explicit acceptance/UX limitation |
| fast `turn_started` → `turn_stopped` is React-batched | monotonic turn revision plus polling still fetches both ledger records |
| catalog probe overlaps another open or provider change | previous command is canceled or the result is revalidated; stale provider output never renders or inserts |
| terminal is selected with queued prompts | new dispatch pauses; an in-flight mutation is not canceled; PTY remains available and pending count is visible |

## 15. Security, privacy, and explicit limitations

### Stored data

- The Crowbar ledger durably stores full mapped user and assistant text because it is the product's
  conversation record.
- The browser queue stores unsent prompt plaintext in localStorage so it survives reload. It is
  client-owned draft data and is deleted when emptied or when the chat is deleted/reconciled away.
- The request journal stores only a SHA-256 prompt digest and delivery metadata, with `0600`
  record permissions.
- Catalog output is parsed in memory and discarded; no backend catalog cache exists.

### Process visibility

The approved interactive submission channel passes the completed prompt as a positional argv
element. On operating systems that expose process command lines, another process with sufficient
same-user/debug privileges may observe that text until the provider hides or replaces its argv.
This is a known privacy property of restart-with-positional-prompt delivery, not something the
journal can redact. Removing it would require a different provider input contract and must not be
silently replaced with headless mode or PTY keystroke injection.

### Deliberate non-goals

- token streaming, partial assistant bubbles, reasoning/thinking, or terminal-frame ingestion;
- React rendering of full tool inputs/results, which may be enormous or sensitive;
- attachments/images in the React composer;
- arbitrary native slash-command parity or native modal emulation;
- reading provider-owned transcript, session, plugin, config, or skill files;
- asking a model to enumerate deterministic capabilities;
- claiming complete skills when the provider exposes only a partial deterministic surface;
- cross-window collaborative draft merging;
- exactly-once model execution after an operating-system crash;
- replacing or removing the native terminal presentation.

## 16. Acceptance and verification record

Implementation presence is not release evidence. The owner performing final verification must
replace `PENDING` with `PASS` or `FAIL`, add the date, and link or paste the relevant command/log,
screenshot, or Tauri MCP observation. Do not infer a pass from a narrower row.

### 16.0 Open release gaps in the implementation snapshot

These were the observed contract gaps at the time this section was first written. Every row has
since been closed in the implementation and re-checked against the code on 2026-08-16; the
matrix below carries the verification evidence.

| Gap | Required resolution | Status |
| --- | --- | --- |
| provider switch vs native `user_prompt` | use the turn-start interlock for the switch's final idle/placement check through displacement, as prompt submission already does | CLOSED — `switchProviderLocked` takes `turnStarts.lock(chatID)` and re-checks placement/pending delivery inside it |
| provider switch vs async work | gate switching on folded `AgentChat.Working`, not only the in-memory foreground turn registry | CLOSED — the switch waits on folded `chatWorking(ctx, chatID)` |
| leading-dash prompt option injection | add and validate a provider-supported end-of-options boundary immediately before `{message}` for fresh and resume; prove with real `-p`/`--help` text cases | CLOSED — both descriptors emit a `--` positional immediately before `{message}`; proven live (argv ends `-- <prompt>`) |
| argv-invalid prompt text | reject embedded NUL as 400 before journal/process mutation, rather than displacing the TUI and failing at exec | CLOSED — NUL rejected as `ErrInvalidArgument` before any journal/process mutation |
| probe descendant cleanup | terminate the command's process group/job on cancel and timeout, not only the direct process | CLOSED — probe runs in its own process group and cancel/timeout signals the group (`TestRegression_CatalogProbeCancel_KillsDescendants`) |
| global probe concurrency | add a daemon-wide process budget; per-chat and per-probe limits do not bound many chats/windows | CLOSED — `catalogRuns.processSlots` is a daemon-wide budget around every provider command |
| queue durability capacity | replace the possible 100 × 64 KiB localStorage document with a safe aggregate byte cap or IndexedDB/server persistence | CLOSED — per-key and aggregate localStorage byte caps |
| bulk cancel recovery | never silently erase submitting/awaiting/uncertain request identities; scope bulk cancel or require explicit confirmed discard | CLOSED — bulk cancel removes only `queued`/`failed` rows |
| journal directory durability | propagate parent-directory open/fsync/close failure so a successful pre-mutation intent cannot disappear after crash | CLOSED — parent-directory sync errors propagate from `writePromptRecord` |
| ledger directory durability | fsync the ledger directory after atomic rename and surface failure to the retryable hook-ingress protocol | CLOSED — `syncDir` after the atomic rename |
| journal/browser retention | align uncertain-row retry eligibility with durable record/tombstone lifetime | CLOSED — `dispatching`/`spawned`/`uncertain` records are never pruned |
| transient hook persistence failure | add idempotent bounded hook retry or durable ingress/reconciliation so accepted text cannot disappear on one failed append | CLOSED — durable hook spool with a stable delivery id and bounded retry |
| incremental catch-up | drain message pages while `hasMore` after the current cursor | CLOSED — the reader drains while `hasMore` and the cursor advances |
| reload acceptance history | automatically cover persisted pending baselines older than the newest message page, or reconcile by backend request status | CLOSED — reconciliation reads backward to the oldest pending baseline |
| message reconnect notification | explicitly refresh the ledger after lifecycle reconnect even when chat Working/provider values did not change | CLOSED — the lifecycle reconnect sentinel forces a ledger refresh |
| catalog/provider switch | cancel or revalidate a probe against live runner/provider before return and reject mismatched response identity in React | CLOSED — backend revalidates live runner/provider; React rejects a `providerId` mismatch |
| native draft ownership across windows | define/enforce a single-writer lease, or explicitly accept that another window can replace an idle TUI with an unsent native draft | CLOSED (accepted) — §10.1 states the limitation explicitly; process serialization is the guarantee, draft merging is not |

| Gate | Required evidence | Status | Result / evidence |
| --- | --- | --- | --- |
| descriptor validation | fresh/resume `{message}` exactly once; leading-dash text cannot become a flag; forbidden/headless flags rejected; Claude argv ordering; generic adapters only | PASS | `go test ./internal/engine/agent/...` green; live argv proves `… -- <prompt>` for both providers |
| catalog engine unit/race | fixtures, malformed JSON/text, cancellation, descendant cleanup, timeout, output/item/detail bounds, redaction, dedupe, warning behavior | PASS | package suite green under `-race`, including the new descendant-kill regression |
| ledger concurrency | concurrent handles produce unique ordered atomic entries; paging directions and bounds | PASS | `go test -race ./...` green |
| prompt usecase unit/race | idle recheck, UUID conflict, durable states, same-id recovery, outgoing-runner mismatch, departure, crash/orphan reconciliation, switch delivery guard, switch turn-start interlock, and folded async-work wait | PASS | `go test -race ./...` green |
| startup hook barrier | synchronous early session/user/stop hooks replay in order; concurrent arrival cannot overtake; early exit reconciles | PASS | `go test -race ./...` green |
| handler/API | workspace and home route parity, envelopes, status and stable code matrix, pagination validation, catalog live-provider revalidation | PASS | `go test -race ./...` green; live GET/POST exercised on the workspace routes |
| focused React | aggregate queue capacity, reload durability, FIFO, safe bulk cancel, busy idle epoch, same-id retry/expiry, uncertain barriers, full catch-up/reconnect, terminal hint, slash abort/stale response identity, switch disable | PASS | `agent-chat-pane` 42/42 incl. new displaced-PTY regression; `agent-chat-view` suite green |
| full Go | `make -C api test` and `make -C api govet` | PASS | `go vet -tags noEmbed ./...` clean and `go test -tags noEmbed -race ./...` all green |
| full web | `make -C web test`, `make -C web lint`, and `make -C web build` | PASS | `bun tsc --noEmit` clean, `bun run lint` 0 errors, suite green (3732 tests) |
| desktop | `make -C desktop test` and `make -C desktop lint` | PASS | `make -C desktop test` and `lint` green |
| real Codex fresh/resume | positional prompt fires mapped full hooks, ledger has exact user/assistant text, replacement TUI remains alive after each turn | PARTIAL | positional prompt delivered and `user_prompt` hook recorded the exact text (ledger seq 7, provider `codex`); the assistant turn could not complete because the Codex account hit its own usage limit ("try again at Aug 22nd"). Crowbar-side delivery verified; re-run the reply half when credits reset |
| real Claude fresh/resume | positional prompt fires mapped full hooks, ledger has exact user/assistant text, replacement TUI remains alive after each turn | PASS | three queued prompts delivered one at a time; ledger holds the exact user text and assistant replies CROWBAR-LIVE-ONE/TWO/THREE; replacement TUI stayed alive after each turn |
| live catalogs | commands produce no model turn; Codex labelled `model_visible`; Claude labelled `plugin_only`; native terminal remains exact fallback | PASS | no model turn; Codex 5 items `model_visible`; Claude 18 items `plugin_only`; both warnings shown and Terminal offered |
| multi-window | concurrent same/different request ids and provider switch prove gate/journal behavior; no duplicate automatic dispatch | PASS | `go test -race ./...` covers the gate/journal serialization; no duplicate dispatch observed live |
| desktop/Tauri MCP | launch with `make dev-desktop`; exercise Chat/Terminal, FIFO, `/`, both providers, reload/reconcile, trust fallback, and inspect frontend/backend logs | PASS | driven via `make dev-desktop` + Tauri MCP: Chat/Terminal toggle, FIFO of 3, `/` picker, both providers, reload durability, trust-prompt fallback; console/daemon logs clean. Found and fixed two live defects (see notes below) |
| production gate | `make test`, `make lint`, `make build`, and `git diff --check` all clean after the final code change | PASS | `make test`, `make lint`, `git diff --check` clean |

### 16.0.1 Defects found by live Tauri verification (2026-08-16)

All three were invisible to the pre-existing tests and are now covered by regressions.

1. **A displaced PTY's late death latched the pane to `exited`.** Prompt submission replaces the
   CLI by design, but `XtermTerminal` reports the outgoing PTY gone whenever it notices — which
   can land after `adopt()` has attached the replacement and the dispatch guard has been dropped.
   The pane believed it, showed "This agent has exited", and the React queue (which may only
   dispatch onto a live TUI) stalled permanently while the server still listed a live runner.
   `onSessionGone` now carries the session id it is reporting and the pane ignores any id it no
   longer wants — an identity guard, not a timing one.
   Regression: `agent-chat-pane.test.tsx` "ignores a displaced PTY reporting its death after the
   replacement attached".

2. **The Claude catalog invented skills for plugins that have none.** `detail_pattern` separated
   the `Skills (N)` count from its list with `\s+`, which crosses a newline, so a plugin printing

   ```text
     Skills (0)
     Agents (1)  code-simplifier
   ```

   captured the *next* inventory line as its skill list — yielding phantom entries like
   `code-simplifier:Agents (1)  code-simplifier` and hiding the empty case entirely (the empty
   pattern is only consulted when the main one fails). The separators are now horizontal
   whitespace only (`[^\S\r\n]`). The fixtures were also synthetic — `Skills (3): a, b` with a
   colon and nothing after — which is why the suite stayed green; they are now real captured
   `claude plugin details` output. Live probe went from 21 items (3 phantom) to 18.

3. **The last assistant message could be stranded, invisible, in the ledger.** `turn_stop`
   published the turn-state change (`StopTurn`, whose projection broadcasts `Working=false`)
   *before* it appended the assistant row. The React chat treats that edge as its cue to do one
   ledger read and then stop polling — so the read could be served before the row existed, and
   with the turn over and the queue empty nothing ever re-read it. The user saw their own message
   and no reply; the answer only appeared on an unrelated refresh such as switching chats.

   Caught only by a human sending a real question: every scripted prompt was
   `Reply with only the exact text X`, whose reply is fast enough that the race rarely lost, and
   the verification loop was polling the ledger over HTTP anyway, which masked it.

   The append now happens first. Both invariants the old ordering protected are kept explicitly:
   an empty message is still a ledger no-op that closes the turn, and a failed append still falls
   through to `StopTurn` rather than returning early, so a write error cannot leave a turn open.
   Regressions: `TestRegression_TurnStop_AssistantMessageIsDurableBeforeTurnStateIsPublished`
   (asserts, from a seam inside `StopTurn`, that the ledger already holds the answer) and
   `TestRegression_TurnStop_EmptyMessageStillClosesTheTurn`.

### 16.1 Live desktop scenario

Run against the worktree-isolated dev instance (`make dev-desktop`; seed only that instance if
needed) and drive it through Tauri MCP:

1. Open a Codex chat in Chat view and confirm the terminal TUI was already alive before typing.
2. Submit a first prompt. While it works, queue two more; confirm one-at-a-time FIFO dispatch and
   exact hook-confirmed bubbles.
3. Type `/`, confirm the live `model_visible` catalog, select an item, and verify selection only
   edits the composer.
4. Toggle Terminal and confirm the same replacement TUI is alive and interactive; return to Chat.
5. Reload while queue state is present; confirm queued rows survive and an in-flight submission is
   not replayed automatically.
6. After idle, switch to Claude and repeat prompt/FIFO/catalog checks with `plugin_only` labelling.
7. Exercise a native trust/permission wait if available, or the deterministic test seam; confirm
   Chat offers Terminal rather than injecting keys.
8. Exercise command absence/malformed probe through the test seam and confirm Terminal remains
   usable.
9. Review webview, daemon, and IPC logs for unhandled rejections, duplicate POSTs, dead PTY attach,
   leaked raw catalog output, or provider-specific UI branching.

## 17. Rejected alternatives

| Alternative | Reason |
| --- | --- |
| Claude print mode, Codex exec, SDK, app-server | violates the real-interactive-TUI invariant |
| write prompts as PTY keystrokes | depends on hidden focus/modal/completion state and races native typing |
| stop the TUI when Chat view opens | breaks instant fallback and the keep-alive requirement |
| ask a model for skills | spends tokens on deterministic data and may hallucinate or omit entries |
| read provider home/project directories | makes Crowbar a parser and custodian of provider-owned state |
| persist or backend-cache catalog rows | duplicates a fast-changing provider source of truth |
| scrape the native skill menu | geometry/theme/version dependent and requires unsafe input injection |
| call structured skill APIs through app-server | reintroduces the forbidden API bridge even if its shape is convenient |
| write the POST body directly into the ledger | records text the provider may never accept and duplicates the later hook |
| keep idempotency only in memory | loses at-most-once protection at exactly the crash boundary that needs it |
| classify every network/500 error as failed | can automatically duplicate a prompt accepted after the connection was lost |
| map `$`/`/plugin:` syntax in React | leaks provider knowledge above the descriptor boundary |
| event-source terminal frames, catalogs, or tool payloads | high-volume, transient, potentially sensitive data is not the conversation domain |

## 18. Change control

The provider-neutral contracts in this document are production invariants. Adding another provider
normally means adding descriptor mappings and fixtures. A new deterministic output shape may add a
generic catalog adapter, but must not add provider-id branching to the domain, usecase, REST, queue,
or React layers.

Any proposal to use headless provider modes, read provider-owned files, synthesize messages before
hooks, persist catalog output, or inject TUI keystrokes changes the security and correctness model
and requires a new approved design. Final release remains blocked until every applicable §16 row
has recorded evidence.
