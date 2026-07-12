# The Runner Model — making the running CLI a first-class citizen

**Status:** approved, not yet implemented
**Date:** 2026-07-12
**Supersedes:** the segment-inside-the-chat model introduced by
`2026-07-09-agentic-engine-asynx-aggregate-design.md` (the asynx conversion itself stands;
only the placement of the *runner* inside the chat aggregate is replaced).

---

## 1. Why

Four bugs, all live-reproduced on `frontend/scaffolding`, all the same root cause.

1. **A torn switch destroys a chat.** `/resume` from chat A into chat B, where B already
   has its own live CLI: `moveToKnownChat` runs `EndSegment(A)` (commits), then
   `OpenSegment(B)` — which *fails*, because `OpenSegment.Validate` rejects a chat that
   already has an active segment. There is no rollback. Chat A is left with no active
   segment and is unusable, and B is never joined. **Reproduced by the user; state
   captured live.**
2. **Split brain.** `Registry.OnSessionStart` mutates the in-memory `segToChat` map
   *before* the aggregate commands run. When the commands fail (bug 1), the registry still
   believes the runner moved — so the orphaned CLI's turn hooks are routed into the target
   chat's ledger, polluting it.
3. **The pane does not follow the process.** `AgentChatPane` is pinned to a `chatId` for
   its whole life. When the CLI moves to another chat, the open pane shows
   *"This agent has exited"* while the CLI is alive and well in a chat the user must find
   and click. Its **Resume** button is actively harmful there: it spawns a *second* CLI on
   the old session while the first keeps running.
4. **The title-rename command renames the wrong chat.** The chat id is baked into the
   `--append-system-prompt` at spawn, so after any conversation move the agent renames the
   chat it *used* to be in.

The common cause: **the running CLI is not a modelled thing.** It is smeared across
`AgentSegment` rows *inside* chat aggregates. So "the CLI moved from chat A to chat B" must
be expressed as *delete a row from aggregate A, insert a row into aggregate B* — two writes,
two aggregates, no transaction. That is why it can tear in half, and why a parallel
in-memory registry exists to shadow it.

We are not patching these four. We are removing the shape that produces them.

---

## 2. The model

Three entities. `AgentSegment` is **not** one of them — it is deleted.

### Conversation
A provider session: a `claude` session id, a `codex` rollout id. **Owned by the provider.**
Crowbar observes its id and nothing else. It never creates, moves, or deletes one. (This is
the standing rule from `project_agentchat_workspace_scoping` / the descriptor-v2 work:
*a provider owns its sessions; Crowbar only observes.*)

### Runner
One live CLI process in one PTY. Its identity is the **`crowbarSegmentID`** we already mint
and already pass to every hook — it is *already* stable across conversation switches; we
simply never modelled it.

```
Runner {
  ID              string   // the crowbarSegmentID; stable for the process's life
  WorkspaceID     string
  ProviderID      string   // claude | codex | ...
  TerminalSession string   // its PTY — its identity AND its heartbeat
  CurrentChatID   string   // exactly one, always
  CurrentSession  string   // the provider conversation it currently holds
}
```

**A Runner has no status field.** The PTY is the *sole* authority on whether it is alive.
A runner exists exactly as long as its process does. This is not a simplification — it
deletes a class of drift we are currently living with: right now a segment can read `ended`
while its CLI is demonstrably still running (observed: PTY `f6fab4b1`, Claude alive,
segment `ended`). Two authorities on liveness always drift. There will be one.

Consequence: `attachAgentSegment`'s current double-check ("segment is active" **and**
"daemon lists the PTY as live") collapses into the single question it was really asking.

### Chat
A Crowbar thread. It owns the **ledger** — one file per turn, on disk — which is the only
thing in this system Crowbar uniquely owns. It does **not** own the process.

```
AgentChat {
  ID, WorkspaceID, Title, TitleLocked, CreatedAt
  Working             bool        // turn state
  CurrentTurnStarted  *time.Time
  LastActivityAt      time.Time
  LedgerCursor        int
  LastProviderID      string      // for Resume: who was here last
  LastSessionID       string      // for Resume: which conversation to reopen
}
```

`Segments []AgentSegment` and `ActiveSegmentID` are **removed**. There is no tenancy table
to replace them: **the ledger *is* the history** (one file per turn, provider in the
filename), and the handoff cut is already computed from it.

### Relationships

```
Runner ──CurrentChatID──▶ Chat           exactly one; the runner points, never the reverse
Runner ──CurrentSession──▶ Conversation  exactly one
Chat   ◀──── 0 or 1 ───── Runner         "live" = some runner points at me. NOT a stored flag.
Chat   ──over its life──▶ many Conversations   (a provider handoff swaps the conversation,
                                                keeps the chat and its ledger)
Runner ──────────────────▶ 1 PTY         its identity and its heartbeat
```

The arrow only ever points **runner → chat**. That is the whole trick: the runner is the
only thing that moves, so **a move is one write to one aggregate.**

### Invariants

* **I1** — A runner points at exactly one chat and one conversation.
* **I2** — A chat is hosted by at most one runner. Liveness is a *query*, not a flag.
* **I3** — A conversation is held by at most one live runner. This is a **provider-level**
  constraint, not ours: two Claude processes on one session id both write the same session
  file. It is what forces eviction (§4.3).

---

## 3. The backbone principle: reconcile, never transact

**By the time a hook fires, the CLI has already done the thing.** `SessionStart` is a
*notification*, not a request. Crowbar cannot refuse it and cannot push the CLI back.

Therefore the daemon's hook path **reconciles**: it records what already happened, and its
write must always succeed. Conflicts are resolved by acting on the *other* party (evicting
an incumbent runner), never by rejecting reality.

Today's code violates this. It validates a fait accompli, and when the validation fails it
has already destroyed the source chat. That is bug 1, exactly.

**Corollary — the reducer branches only on facts.** It asks two questions and no others:

1. Did the conversation id under this runner change?
2. Is the new id one we know?

It must **never** read the hook's `source` field. Verified against the real binaries
(§7): Claude reports `source: clear`, Codex reports `source: startup` for the *same* event.
Any code branching on that vocabulary is provider-specific and will break. The existing
reducer already branches only on facts, and that is precisely why it survives the
difference. This is now a written rule, not an accident.

---

## 4. Flows

### 4.1 Spawn
Create the chat, start a runner on it. `SessionStart` binds the conversation
(`runner.CurrentSession`).

### 4.2 `/clear` (or `/new`) — an unknown conversation id appears
Create chat B, then `runner.Move(B, newSession)`. **Chat A is never written to.** It simply
stops being pointed at, and is therefore dormant. Same runner, same PTY, same terminal.

Two writes — but ordered so the only possible failure is a **stray empty chat**, never a
destroyed one. In practice even that is unreachable: both CLIs announce *lazily* (§7), so
the new chat is minted at the moment we learn the new conversation id, which is when there
is already a turn to put in it.

### 4.3 `/resume` into a **known** conversation — the eviction case
`runner.Move(B, sessionB)`.

If another live runner already holds `sessionB`, it is **evicted**: its PTY is killed. This
is forced by I3 — two CLIs on one session id corrupt the provider's session file. Crowbar
has no third option, because the CLI has *already* switched (§3).

* Order: **Move first** (record reality), **then** evict. If the kill fails, Crowbar's
  record is still *accurate* — two runners really do hold the conversation — and only the
  cleanup needs retrying.
* **Residual, unavoidable:** the moment the user picks the conversation in the CLI's own
  picker, two processes hold it, before any hook reaches us. That window is the provider's,
  not ours. We minimise it; we cannot close it.
* **UX (decided with the user):** a toast — *"Codex was closed — that conversation moved
  to this terminal."* The evicted runner's tab **closes**, and focus moves to the tab that
  took the conversation over, preserving "one tab per live conversation". A terminal tab
  has no unsaved state, so closing costs nothing.

### 4.4 `/resume` into a conversation Crowbar has never seen
(e.g. one the user started in a plain terminal outside Crowbar.) Mint a chat for it,
`runner.Move(newChat, session)`. It joins the sidebar. This is the user's stated
requirement: *"If the conversation isn't known, Crowbar should add it again."*

### 4.5 `/exit`, crash, or daemon restart
The PTY dies, so the runner is gone (§2 — no status flag to update). The chat is dormant.
`LastProviderID` / `LastSessionID` let **Resume** start a fresh runner exactly where it left
off.

### 4.6 Provider switch (handoff)
Unchanged from today. The chat stays; a new runner on the other provider takes it over,
pointed at the ledger via `context_inject` / `resume_context_inject`. The descriptor work
(descriptor v2, the ledger pointer) is untouched by this spec.

### 4.7 Turn hooks
A hook carries its `segid`. Resolve `segid → runner → CurrentChatID` **from durable state**
and append to that chat's ledger. There is no in-memory map to disagree with. This alone
kills bug 2.

### 4.8 Title rename
`crowbar chat rename --segment <segid>` — resolve the chat the same way, **at call time**.
The chat id is no longer baked into the spawn args, so bug 4 becomes unrepresentable. The
descriptor's `title_instruction` prompt template uses `{segid}` instead of `{chatid}`.

---

## 5. Projections

Runner events feed read models; nothing else writes them.

* **`chat_liveness`** — `chatId → runnerId | none`. Powers "is this chat live", the sidebar
  indicator, and the pane's attach. Replaces `ActiveSegmentID`.
* **`session_index`** — `sessionId → chatId`. Replaces `Registry.sessionToChat`. Being
  **persisted**, the boot-reseed path (`Registry.Seed`) disappears.

`Registry`'s in-memory `segToChat` / `sessionToChat` / `segToSession` maps are **deleted**.
`segToInjected` (the injected-context echo guard) survives — it is genuinely ephemeral,
per-spawn state and has no durable meaning.

---

## 6. Frontend

A pane's buffer becomes `{ type: 'agentChat', runnerId, chatId }` — **both mutable**.

* The **runner moves to another chat** → update `buffer.chatId`. The tab **relabels**. The
  terminal is keyed by the runner's PTY, which has not changed, so **xterm never remounts**.
  This is the user's requirement — *"without changing the Terminal"* — falling out of the
  model rather than being engineered.
* A dormant chat is **Resumed** → a new runner → update `buffer.runnerId`. The tab
  **reattaches** to the new PTY.
* Opening a chat from the sidebar: if it is live, focus/open its runner's tab; if dormant,
  spawn a runner resuming `LastSessionID` and open that.
* Eviction: close the evicted runner's tab, focus the taker (§4.3).

The auto-revive machinery in `agent-chat-pane.tsx` (`canAutoRevive`, `MAX_REVIVE_ATTEMPTS`)
exists to paper over the fact that a pane cannot tell "my CLI died" from "my CLI moved". Under
this model it *can*, so that machinery is **removed**, not ported. Revive becomes: dormant
chat + explicit open = spawn a runner. Nothing implicit.

---

## 7. Provider-agnosticism — verified, not assumed

Spiked against the real binaries on 2026-07-12 (`codex-cli 0.139.0`, `claude 2.1.207`).

Codex has the same conversation vocabulary as Claude — `/new`, `/clear`, `/resume`,
`/compact` — and announces every change. The *timing and vocabulary differ*:

| | Claude | Codex |
|---|---|---|
| Announces on switch | eagerly, at the keystroke | **lazily**, at the first turn of the new conversation |
| `source` reports | `clear` / `resume` | `startup` / `resume` |
| Order vs the turn | `session_start` **before** `user_prompt` | **same** |

Observed trace (Codex): `/new` fired **nothing**; the first prompt afterwards produced
`session_start` with a **new** id (`source: startup`) *before* that prompt's `user_prompt`.
`/resume` back was likewise silent at the keystroke, then fired `session_start` with the
**original** id (`source: resume`) — and Codex correctly recalled the earlier conversation.

**What this settles:**

1. The contract holds for both CLIs: every change is announced with the correct id, always
   **before** the turn that follows it. **No turn is ever misfiled.**
2. The reducer must not read `source` (§3). Codex says `startup` where Claude says `clear`.
3. Lazy announcement means `/new` with no follow-up creates **no empty chat** — the chat is
   minted when we learn the id, i.e. when there is already a turn for it.

**Crowbar intercepts nothing.** Verified: no code anywhere inspects terminal input for
`/clear`, `/new`, `/resume`; `terminalEngine.Write` hands raw bytes to the PTY. Crowbar
learns of a conversation change *only* from the provider's own hook. This is what makes the
system provider-agnostic (we know no CLI's command vocabulary), robust (a switch by a route
we've never seen still works), and honest (a `/resume` picker the user escapes out of fires
no hook, so Crowbar never records an intent that didn't happen).

**Optional hardening, recommended:** Codex's `user_prompt` payload *also* carries
`session_id`, which our descriptor does not currently map. Mapping it costs nothing and
makes the reducer robust to a hypothetical third CLI that never fires `session_start` at
all — the switch would then be caught on the next turn instead of never.

---

## 8. What becomes impossible

Not "fixed" — **unrepresentable**:

* A chat cannot be bricked by a failed move. Nothing writes to the chat you are leaving.
* A hook cannot land in the wrong chat's ledger. It resolves through durable state.
* A rename cannot hit the wrong chat. Same resolution; no id baked at spawn.
* A dead segment cannot shadow a live CLI. Liveness has exactly one source.
* Two runners cannot silently share a conversation. I3 is enforced at the move.

---

## 9. Testing

Per `feedback_blackbox_regression_tests` and `feedback_no_timing_in_tests` (**no sleeps, no
`Eventually`, no poll intervals — block on real signals**: asynx `WaitIdle`/`Drain`, channels).

**Regression tests — one per bug above, each must fail on `HEAD` before the refactor:**

* `TestRegression_ResumeIntoOccupiedChat_DoesNotBrickSource` — the user's bug. Runner on A
  reports B's known session while B has a live runner. Assert: A is *not* destroyed, B is
  hosted by the mover, the incumbent is evicted.
* `TestRegression_HookAfterFailedMove_DoesNotPolluteLedger` — split brain.
* `TestRegression_ClearMintsChat_KeepsSamePTY` — the moved-to chat carries the same runner
  and PTY.
* `TestRegression_RenameResolvesChatAtCallTime` — rename after a move hits the *current* chat.
* `TestRegression_DeadPTY_MeansDeadRunner` — no runner can outlive its PTY.

**Reducer:** table-driven over (id changed?, id known?, target occupied?) — and an explicit
test that the reducer produces identical outcomes for `source: clear` and `source: startup`,
locking in §3.

**Provider-agnosticism:** the existing real-CLI integration suite must stay green for both
providers, and gains a case for the in-CLI `/clear` → `/resume` round trip.

**Live verification (mandatory, per `feedback_verify_in_tauri_before_claiming` and
`feedback_manual_tauri_in_loop`):** driven through the Tauri MCP against a real
`make dev-desktop` build — never the production instance
(`feedback_dev_verification_isolation`). Scenarios:

1. `/clear` in Claude → tab relabels, terminal does not flicker, new chat in sidebar.
2. `/resume` into a dormant chat → tab follows, conversation recalled.
3. `/resume` into a chat that is **open and live in another tab** → incumbent evicted, its
   tab closes, focus lands on the taker, toast shown, source chat still healthy.
4. Same three against Codex (lazy announcement path).
5. `/exit` → chat dormant, Resume brings it back; no double-spawn.
6. Delete the active chat → no blank pane.

---

## 10. Out of scope

* The descriptor / handoff / ledger-pointer work (shipped, untouched).
* Terminal session persistence across daemon restart (Phase 2/3 of
  `project_terminal_session_persistence`).
* `TMUX`/`TMUX_PANE` scrubbing in `ptyEnv()` — a real but unrelated bug, tracked separately.
