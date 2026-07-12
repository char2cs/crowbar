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

#### Persist placement. Never persist liveness.

This distinction is the heart of the spec, and it is easy to get backwards.

The disease was never *persistence*. It was persisting **liveness** — `Status: active|ended`
as durable truth, which reality contradicts the instant a CLI dies (or fails to). Two
authorities, guaranteed drift.

**Placement** — which chat, which conversation a runner is on — is different in kind:
Crowbar is its *only* writer. Nothing external can contradict it. It cannot drift.

So the Runner **is** an event-sourced asynx aggregate (`Started` / `Moved` / `Exited`), and
it carries **no status field**:

* **One atomic write per move.** `Moved` is a single event on a single aggregate. The torn
  cross-aggregate write (bug 1) has nowhere to happen.
* **Everything else is a projection of it** — the chat's conversation history (§2 Chat), the
  session index (§5), the hub's WS frames. One write; the rest derives. This is the pattern
  the codebase already uses, and it is what makes the second write that could fail simply
  not exist.
* **An audit trail.** "R1 moved A→B at 15:04" is recorded. The bug that prompted this spec
  had to be reconstructed from `ps` output and inference; it would have been one log line.

**Liveness stays the PTY's, and only the PTY's.** At boot, reconcile **once** against the
terminal engine's live session set: any runner whose PTY is gone is dead, and gets its
`Exited`. That is one reconciliation, at one moment, against the one authority — not two
truths coexisting and diverging over time.

*(An in-memory placement registry was considered and rejected. It is the shape of the
`Registry` that caused bug 2; it would not even have bought atomicity, since appending the
conversation to the destination chat is a durable write the mutex cannot cover; and it
discards the projections and the audit trail for nothing.)*

### Chat
A Crowbar thread. It owns the **ledger** — one file per turn, on disk — which is the only
thing in this system Crowbar uniquely owns. It does **not** own the process, and after this
refactor it knows nothing whatsoever about processes.

```
AgentChat {
  ID, WorkspaceID, Title, TitleLocked, CreatedAt
  Working             bool        // turn state
  CurrentTurnStarted  *time.Time
  LastActivityAt      time.Time
  LedgerCursor        int
}
```

`Segments []AgentSegment` and `ActiveSegmentID` are **removed, and nothing replaces them on
the aggregate.** The chat aggregate no longer contains a single field describing a process.

Everything the system needs to know about which conversations a chat has hosted — the
reducer's "is this id known?", and Resume's "where do I pick up?" — is a **projection of
Runner events** (§5), not chat state. The chat never writes it, so there is no second write
to fail.

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

This is the one flow that is still two writes, and the plan does not pretend otherwise. They
are ordered so that the failure mode is bounded: the first is a **create** (which cannot
destroy anything), the second is the atomic `Moved`. If the `Moved` fails, the result is a
**stray empty chat** in the sidebar and the runner still on A — annoying, self-evident, and
recoverable by deleting the chat. Nothing is lost.

That bound — *a failure can never destroy a chat* — is the guarantee this design makes.
"Every operation is a single write" is **not** a guarantee it makes, and claiming so would be
comfortable and false.

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

**Runner events are the only write. Everything below derives from them** — the chat
aggregate writes none of it, which is why no second write exists to fail (§2).

* **`chat_conversations`** — `(chatID, providerID, sessionID, firstSeenAt)`, **append-only**.
  Built from `Started` / `Moved`. This is what `Segments` really was, minus everything that
  described a *process*. It answers the system's two durable questions:
  * *"Is this conversation id one I know?"* — the reducer's second question (§3). Indexed by
    `sessionID`, so it also **replaces `Registry.sessionToChat`**, and being persisted, the
    boot-reseed path (`Registry.Seed`) disappears.
  * *"Where does this chat pick up?"* — Resume reads the chat's newest row for its provider
    and session.

  Append-only history **cannot drift from reality** — only live state can. That is exactly
  why this is safe to persist while liveness is not (§2).

* **`chat_liveness`** — `chatID → runnerID | none`. Built from `Started` / `Moved` /
  `Exited`. Powers "is this chat live", the sidebar indicator, and the pane's attach.
  Replaces `ActiveSegmentID`. **Derived, never stored on the chat.**

* **hub frames** — the existing `agentchat.*` WS fan-out gains the runner's lifecycle, so the
  frontend learns of a move from the same stream it already listens to.

`Registry`'s in-memory `segToChat` / `sessionToChat` / `segToSession` maps are **deleted** —
they are precisely the durable-state shadow that caused bug 2. `segToInjected` (the
injected-context echo guard) **survives**: it is genuinely ephemeral, per-spawn state with no
durable meaning and nothing to drift from.

**Boot reconciliation.** Once, at startup, against the terminal engine's live session set:
any runner whose PTY is gone gets an `Exited`. One reconciliation, at one moment, against
the single authority on liveness (§2). This replaces today's boot reactor that ends segments.

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
