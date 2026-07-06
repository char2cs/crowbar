# Crowbar — Agentic Engine & Provider-Switching Core (Iteration 1)

**Status:** design validated by a Phase-0 spike — every load-bearing claim proven live on the
real binaries (§1 scorecard). Ready for implementation planning.
**Date:** 2026-07-05
**Scope:** backend only. No GUI, no input overlay, no viewer. Interface left bare;
exercised via HTTP + integration tests against the real CLIs.

---

## 1. What we're building

A provider-agnostic engine that drives real, interactive agentic CLIs (Claude Code
and Codex first) as a thin shell around them, plus the chat domain that lets a user:

1. Run an agent CLI under Crowbar (spawned in a real PTY, interactive TUI, never headless).
2. Have Crowbar own the notion of a **chat** and track it across provider segments.
3. Switch the provider backing a chat mid-conversation, with a crude context handoff.
4. Detect when the conversation underneath a running CLI moved to a different chat.

**Crowbar's core knows nothing about Claude or Codex.** All per-CLI knowledge lives in a
single YAML **descriptor** per provider. Adding a third provider is a third YAML file with
zero engine changes — that is the thesis this iteration exists to prove.

### Grounding (Phase-0 spike, real installed binaries)

| Concern | Claude Code `2.1.201` | Codex `0.139.0` |
|---|---|---|
| Spawn (interactive default) | `claude` | `codex` |
| Hook inject | `--settings <json\|file>` (additive merge) | `$CODEX_HOME/hooks.json` + trust seed |
| Hook events (all fired live) | `SessionStart` / `UserPromptSubmit` / `Stop` | **same event names**, same nested `{hooks:[{type,command}]}` shape |
| Hook wire (stdin JSON) | `session_id`, `transcript_path`, `hook_event_name`, `source` | convergent (`hook_event_name`, `hookSpecificOutput`, `stop_hook_active`) |
| Session id | hook payload; `--session-id` can assign, but engine **records the native id** (uniform w/ Codex); `--resume <id>` | rollout filename + hook; **no `--session-id`**; `resume <id>` / `--last` |
| Transcript — path **read from the hook**, never computed | `~/.claude/projects/<slug>/<uuid>.jsonl` (slug truncated + hash-suffixed) | `~/.codex/sessions/YYYY/MM/DD/rollout-*-<uuid>.jsonl` |
| Handoff inject | `--append-system-prompt` (per-invocation, **non-polluting** — proven) | initial-prompt arg (proven); `-c developer_instructions` (alt, untested) |

**Headline finding:** Codex's hook system is a near-clone of Claude's (same event names,
same wire shape). The doc's #1 fear — Codex lifecycle granularity — is unfounded.

### Phase-0 spike — RESULTS (all PROVEN live, before any engine code)

A throwaway PTY harness drove the two real binaries through every risky path. Scorecard:

| Claim | Claude | Codex |
|---|---|---|
| `SessionStart` hook → `session_id` + `transcript_path` (+ `source`) | ✅ | ✅ |
| One generic stdin-reading hook drives both, same field names | ✅ | ✅ |
| Detection: a conversation move re-fires `SessionStart` with a **changed** id | ✅ `/clear` | ✅ `/new` |
| Session-scoped injection, no mutation of the user's real config | ✅ `--settings` | ✅ relocated `CODEX_HOME` |
| Forward handoff = cross-provider **opaque** transcript read | ✅ Codex read Claude's raw 38 KB `.jsonl` and extracted the codeword | — |
| Native resume / Case-1 (`--resume <id>` → `source=resume`) | ✅ | ✅ `resume` |
| Backward handoff (`--append-system-prompt` delta) lands | ✅ | — |
| **Pollution:** does the appended delta persist into the vendor session file? | ✅ **No — per-invocation, clean** | — |

**Vindication of facts-not-labels (§7):** the *same kind* of move is labelled `source:"clear"`
by Claude but `source:"startup"` by Codex — the labels disagree, the id-change does not. Any
reducer that branched on the label would already be broken across these two CLIs.

**Gotchas the spike forced into this design (each folded into the sections below):**
- **`CLAUDE_CODE_CHILD_SESSION=1` / `CLAUDECODE=1`** (inherited when a `claude` child is spawned
  from inside Claude Code) **suppress the child's transcript persistence** → `--resume` breaks.
  A spike artifact (Crowbar's Go daemon spawns clean), but `spawn.env` clears them defensively (§4.1).
- **Transcript path is READ from the hook payload, never computed** — Claude's slug is
  truncated + hash-suffixed; Codex's is a date-partitioned rollout (§4).
- **Codex hooks fire interactively only** (not under `codex exec`) — which is exactly where the
  product runs. Injection = relocated `CODEX_HOME` + `--dangerously-bypass-hook-trust` (§4.2).
- **Switch = gracefully quit the outgoing CLI** (Claude flushes on a clean exit; Codex tolerates
  a hard kill) — never SIGKILL mid-flight (§8).

---

## 2. Interaction model (read this before the architecture)

The human talks to the agent by **typing directly into the terminal** — the real CLI TUI in
the PTY, exactly like using it in iTerm2. **There is no "send a message" API, and Crowbar
never writes to the PTY at all.**

- **The terminal's input belongs entirely to the human.** Crowbar injects no keystrokes — not
  messages, not `/compact`, not `/clear`. Writing into a live Ink TUI is fragile (it depends on
  the TUI's current mode) and would race with the user typing. It is also unnecessary (below).
- **All Crowbar orchestration is out-of-band**, three tools only: (a) **spawn-time arguments**
  (`--resume`, handoff via Claude's `--append-system-prompt` or Codex's initial-prompt arg);
  (b) **read-only side channels** (hooks + transcript); (c) **process lifecycle** (spawn /
  terminate a PID — a signal, not a PTY write).
- Crowbar **observes** via hooks + transcript, **owns** the chat + ledger, **detects** moves
  via `SessionStart`, and **orchestrates** switches by spawning/terminating processes — never
  by typing into them.
- The **single dispatch chokepoint** is therefore the one **spawn** function every provider
  process is born through (it applies the descriptor: hooks + handoff + `CROWBAR_SEGMENT_ID`) —
  the switch-injection, logging, and replay point.
- The HTTP API is **lifecycle/management only**: spawn an agent session, switch provider,
  dump handoff, list/observe chats, plus a WS event stream. No message endpoint.

This keeps sessions indistinguishable from a human at the vendor CLI (the compliance posture)
and keeps Terminal-only a real fallback: nothing Crowbar adds is load-bearing for the session.

---

## 3. Architecture

Fits the existing layered Go daemon (`api/`). Reuses, does not rebuild.

```
api/
  cmd/crowbar/                 # existing cobra binary (serve, version)
    + hook subcommand          # NEW: thin socket client, forwards hook payloads
  internal/
    engine/
      terminal/                # EXISTING — PTY + screen-model + serializer + resize + state
        + spawn(argv,env)      # EXTEND: today spawns only s.shell; generalize to arbitrary argv+env
      agent/                   # NEW — provider-agnostic agentic engine
        descriptor/            #   YAML load + validate + template resolution
        inject/                #   generic step runner (write_file/set_env/pass_arg/seed_trust)
        hooks/                 #   ingest → canonical events (field-map from descriptor)
        transcript/            #   locate + read + forward opaque
        descriptors/           #   claude.yaml, codex.yaml  (go:embed, on-disk override)
    app/
      usecases/chat/           # NEW — chat aggregate, segment lifecycle, switch, detection reducer
      repositories/            # NEW — gorm Chat/Segment; ledger writer (workspace .crowbar)
    api/v0/
      endpoints/               # NEW — chat lifecycle endpoints + hook-ingest endpoint
      ws/                      # EXISTING broadcaster/hub — add chat event channel
```

**Why reuse `engine/terminal`:** it already solves the hardest doc requirement ("the PTY host
must be a competent terminal" — DA1/DA2/DSR/XTVERSION, resize/SIGWINCH, Unicode, alt-screen)
via `creack/pty` + a VT screen model. Claude/Codex are just processes to spawn in it. The one
real change is generalizing session spawn from shell-only (`exec.Command(s.shell)`) to an
arbitrary `argv + env`.

**Naming:** the new package is `engine/agent` — `engine/provider` is already the *git*
provider (GitHub/GitLab); we do not overload it.

---

## 4. The provider descriptor (single YAML per provider)

One file tells the generic engine everything about a CLI. Provider-specific reality lives
**entirely** in the YAML; the engine is a generic interpreter of it.

### 4.1 `claude.yaml`

```yaml
# Validated against claude 2.1.201 (Phase-0 spike, §1). The pinned CLI version
# is documentation only — there is no wired compat/version-drift check (a
# version.compat_check field existed early on but was parsed and never
# consumed by anything; removed during the daemon-hardening pass rather than
# wired, per YAGNI — see that pass's report for the rationale).
id: claude

spawn:
  cmd: claude
  interactive_required: true            # engine refuses to launch headless
  forbid_flags: ["-p", "--print"]       # §3 hard guard, enforced in engine
  env:                                  # cwd supplied by the chat at runtime
    clear: ["CLAUDE_CODE_CHILD_SESSION", "CLAUDECODE"]  # else a nested spawn won't persist its
                                        #   transcript → --resume breaks (Phase-0 finding)

session:                                # all spawn-time ARGS — Crowbar never writes to the PTY
  # Claude's --session-id COULD assign an id at spawn time (a session.assign
  # field existed for this), but the engine always records the native id
  # instead (uniform with Codex, which has no such flag) — assign was never
  # read anywhere, so it was removed as dead config rather than wired.
  resume:  { arg: "--resume {id}" }

# Injection is expressed as ordered declarative STEPS from a closed generic vocabulary.
# Engine implements the step verbs; it knows nothing about "claude".
config_injection:
  - render_hooks: { into: "{tmp}/settings.json" }
  - pass_arg:     { arg: "--settings", value: "{tmp}/settings.json" }

hooks:                                  # events to register + field-maps (JSONPath into the payload)
  session_start:
    provider_event: SessionStart
    fields:
      session_id: $.session_id          # REQUIRED — the only field the reducer branches on
      transcript: $.transcript_path
  turn_stop:
    provider_event: Stop
    fields: { session_id: $.session_id, transcript: $.transcript_path }

# NOTE: an earlier revision of this descriptor also had a top-level
# `transcript: { from_hook, locate, content }` block and a
# `hooks.session_start.fields.move_signal: $.source` entry. Neither was ever
# read by any engine code (the transcript PATH the engine actually uses comes
# from hooks.*.fields.transcript above, and the reducer branches purely on
# session-id-changed / session-id-known — §7 — never on a lifecycle label).
# Both were removed as dead config during the daemon-hardening pass rather
# than wired, so every field left in this file is genuinely consumed.

handoff_inject:                         # how a handoff blob enters a fresh spawn
  # Spike-proven: on a --resume spawn the appended prompt is applied PER-INVOCATION — it is NOT
  # written into the vendor session file, so switch-back injects a delta without polluting it.
  - pass_arg: { arg: "--append-system-prompt", value: "{handoff}" }

super_harness: {}                       # RESERVED, unused (MCP/skills/subagents — invariant 7)
```

### 4.2 `codex.yaml` (delta only — same shape, this is the abstraction proof)

```yaml
# Validated against codex 0.139.0 (Phase-0 spike, §1) — documentation only, see
# the claude.yaml comment above for why there is no version field here.
id: codex

spawn:
  cmd: codex
  interactive_required: true
  forbid_flags: ["exec"]
  # Spike-proven: Codex hooks fire in the interactive TUI ONLY (never under `codex exec`) — which is
  # exactly where Crowbar runs. --dangerously-bypass-hook-trust lets our injected hooks run without
  # per-hash trust seeding (intended for vetted automation).
  args: ["--dangerously-bypass-hook-trust"]

session:
  # Codex mints its own id (no --session-id) — Crowbar learns it from the first SessionStart hook,
  # correlated by CROWBAR_SEGMENT_ID. This is the record-not-assign model, now uniform with Claude.
  resume:  { arg: "resume {id}" }

config_injection:                       # the genuine divergence: a procedure, not a flag
  # Spike-PROVEN: relocate CODEX_HOME (isolates the user's real ~/.codex), carry auth + project
  # trust, drop in our hooks. Hooks fired live; trust is bypassed by the spawn flag above.
  - set_env:    { name: CODEX_HOME, value: "{tmp}/codex-home" }
  - write_file: { path: "{tmp}/codex-home/auth.json", from: "~/.codex/auth.json" }   # carry login
  - write_file: { path: "{tmp}/codex-home/config.toml", content: "<project trust = trusted>" }
  - render_hooks: { into: "{tmp}/codex-home/hooks.json" }
  # A seed_trust verb was considered here — the spawn flag makes hooks run without it; Codex's
  # trusted_hash was not reverse-engineered (7 serializations missed) and is not needed for the
  # POC — so it was never added to the vocabulary.

hooks:                                  # SAME canonical events, same field-map shape as claude.yaml
  session_start: { provider_event: SessionStart, fields: { session_id: $.session_id, transcript: $.transcript_path } }
  turn_stop:     { provider_event: Stop,         fields: { session_id: $.session_id, transcript: $.transcript_path } }

# (No transcript: block or move_signal field — see the NOTE under claude.yaml
# above; both were removed as dead config, never wired, during the
# daemon-hardening pass.)

handoff_inject:                         # Spike-proven: the handoff rides as Codex's INITIAL PROMPT
  # (a spawn-time positional arg — Crowbar still never writes to the PTY). Codex read a raw Claude
  # transcript injected this way and correctly extracted its content. `-c developer_instructions=`
  # is a plausible system-level alternative, untested.
  - pass_arg: { positional: "{handoff}" }
```

**Injection step vocabulary (closed set, engine-implemented, zero provider knowledge):**
`render_hooks`, `pass_arg`, `set_env`, `write_file`.
A new provider composes these differently; only if a provider needs a genuinely new mechanism
does a new verb get added.

---

## 5. Hooks — mechanism

A hook is a config entry: "when event X fires, run command Y." The CLI spawns Y and pipes a
**JSON payload to its stdin** at the moment of the event. We only observe (never block).

- **The command is `crowbar hook <canonical-event>`** — a new subcommand on the existing
  `crowbar` binary, invoked by **absolute path** `$CROWBAR_HOME/bin/crowbar`. It is a **thin
  socket client**: reads stdin, reads `$CROWBAR_SEGMENT_ID` from its env, POSTs both to the
  daemon's unix socket. Zero domain logic (invariant 1). Lean code path — no daemon init.
  On startup the daemon ensures `$CROWBAR_HOME/bin/crowbar` exists (symlink/copy from
  `os.Executable()`).
- **Attribution — `CROWBAR_SEGMENT_ID`:** at spawn, the engine injects `CROWBAR_SEGMENT_ID=<uuid>`
  into the CLI's env. The hook subcommand stamps every forwarded payload with it. This ties
  each hook unambiguously to the segment (process) Crowbar spawned — even with multiple CLIs
  running, even for Codex where we don't control the session id.
- **Canonical events:** the ingest endpoint maps the provider's raw event + fields to a
  canonical event via the descriptor's `hooks` field-map. Engine code reads only canonical
  events (`session_start{session_id, transcript}`, `turn_stop{session_id, transcript}`).

```
claude/codex ─(event)→ runs `$CROWBAR_HOME/bin/crowbar hook <event>`
                          │  stdin: {provider JSON}  env: CROWBAR_SEGMENT_ID
                          ▼
                 POST unix://…/crowbar.sock  {segment_id, event, payload}
                          ▼
       hook-ingest endpoint → field-map (descriptor) → canonical event → reducer
```

---

## 6. Chat domain — Crowbar owns the saving

### 6.1 Model

- **Chat** = an ordered list of **Segments** + a **Ledger**. Identity is Crowbar's, stable
  across providers. `{ id, title, created_at, active_segment_id, segments[] }`.
- **Segment** = one provider stint: `{ id, chat_id, provider_id, provider_session_id,
  crowbar_segment_id, started_at, ended_at, status }`.
- **Ledger** = the append-only, provider-agnostic record, **owned by Crowbar in the workspace**
  (not left to the vendors' scattered transcript files):

```
<workspace>/.crowbar/chats/<chat-id>/
  chat.yaml                                   # metadata + segment list
  ledger/
    0001-2026-07-05T20-14-claude.blob         # opaque snapshot, timestamped + provider-tagged
    0002-2026-07-05T20-31-codex.blob          # appended when Codex takes over
  handoff.yaml                                # assembled view `crowbar handoff dump` prints
```

**How it fills:** on every `turn_stop`, Crowbar copies that turn's transcript content into a
new timestamped, provider-tagged ledger entry — **opaque, never parsed** (the reader is always
an agent; a model is a universal parser). The ledger therefore accumulates *every provider
that has ever touched the chat, in one place, in order*.

- **Durable store is cumulative** (nothing lost). **Injected handoff is assembled** from it.
  POC injects the concatenated ledger verbatim (crude, opaque) and accepts the token ceiling —
  the "crude handoff on purpose". Smarter assembly (summarize old segments) is deferred.

### 6.2 Persistence

- Chat/Segment metadata → gorm (existing sqlite, single-conn). Serialize writes (see §7).
- Ledger blobs → files under `<workspace>/.crowbar/chats/…` (large, append-only, opaque).

---

## 7. Context-move detection (pure `SessionStart`)

"Moving into another chat" physically means: one PTY runs one CLI process, but the *conversation
underneath it* changes — the user typed `/clear` (new session id) or `/resume <x>` (different
existing session id). Same process, different conversation.

**The only signal is the `SessionStart` hook**, attributed to a segment via `CROWBAR_SEGMENT_ID`.
Spike-verified live on both CLIs: Claude `/clear` and Codex `/new` each re-fired `SessionStart`
with a changed session id. No input-watching — reacting to the hook loses nothing, because the
outgoing context is already in the ledger (snapshotted on every `turn_stop`) and the vendor's old
transcript file persists on disk after a `/clear`.

**The reducer branches on FACTS, never on the meaning of a lifecycle label.** There are only two
observations — no CLI can assign them a "weird meaning" because they are observations, not
semantics: (1) *did the session id under this segment change?* (2) *is the new id one we know?*

**One serialized reducer** (single writer → registry never corrupts, an acceptance criterion):

```
on session_start{session_id S}  (segment seg):
    prev = registry.session_for(seg)      # last id seen for this segment, or none
    if   S == prev:      → no-op
    elif prev == none:   → first id for this segment → bind its chat → S   (spawn / switch-continuation)
    elif registry.knows(S): → CASE 1: move focus to chat(S)     (emit chat.focus)
    else:                → CASE 2: register NEW chat for S; adopt transcript (may lag 1 turn)
    registry.bind(seg → S)

on turn_stop{transcript}:  → append opaque snapshot to that chat's ledger (§6.1)
```

- Walk it through: `/clear` → a new id appears → unknown → new chat (we never encode "clear means
  new chat"; it simply *is* a new id). `/resume` → known→focus, unknown→adopt. `/compact` → if it
  keeps the id → no-op; if some CLI's compact mints a new id → handled generically as a move.
  **Each is correct without the engine knowing what the command does.**
- **"Known"** is well-defined: Crowbar records every session id it spawns/sees. Foreign ids
  (raw-terminal `/resume` of a pre-existing conversation) are the unknowns → adopt.
- **The `source`/`move_signal` label was NEVER load-bearing** — the reducer branches purely on
  session-id-changed / session-id-known, never on a lifecycle label's meaning. A descriptor
  `move_signal` field existed early on to capture it as optional metadata, but nothing ever read
  the captured value, so it was removed as dead config during the daemon-hardening pass rather
  than wired. A provider that omits the label, or one with unusual lifecycle semantics, still
  cannot break or corrupt the registry — "does Codex emit `source`?" remains moot for correctness.

---

## 8. Provider switch (Component 3)

User-initiated, orchestrated by Crowbar (not a user "send"). **Spike-proven end-to-end** — a full
Claude→Codex→Claude round-trip carried a codeword across both switches (§1 scorecard).

```
POST /chats/{id}/switch { provider: codex }
  1. read the outgoing session's transcript from disk → append an opaque snapshot to the ledger
     (the ledger already holds prior turns from turn_stop; this captures anything since the last Stop)
  2. assemble the handoff blob from the ledger
  3. gracefully quit the outgoing CLI  # clean exit, NOT SIGKILL: Claude flushes its transcript on a
     #   clean exit; Codex tolerates a hard kill (spike). A PID-level action, never a PTY write.
  4. spawn the target CLI in the same chat as a NEW segment, injecting the handoff at spawn time
     (descriptor handoff_inject) + a fresh CROWBAR_SEGMENT_ID
  5. new segment bound to the chat; active_segment updated
  # No keystrokes are ever written to any PTY.
```

- **Switch-back** reuses `session.resume`: returning to a prior provider resumes its old
  session id, restoring **native** context (higher fidelity than a re-injected summary) — so
  we never keep idle processes alive. **Spike-proven:** the returning CLI restored its native
  session (`source=resume`) *and* absorbed the appended delta, and the appended delta did **not**
  pollute the vendor session file (per-invocation — §4.1). So switch-back is both lossless and clean.
- **`crowbar handoff dump <chat>`** prints `handoff.yaml` from day one. The switch is the
  feature most likely to be silently wrong and cannot be verified from the terminal; its
  internal state must be legible.

**Known ceiling (stated, not fought):** cross-provider context is a *reconstruction from a
summary*, not a lossless port. Switch fidelity ≤ handoff fidelity. The *mechanism* is
model-agnostic; *fidelity* is per-pair, tuned later.

---

## 9. HTTP + WS surface (bare, lifecycle only)

- `POST   /v0/chats`                     — spawn provider P in workspace W → creates chat + segment
- `GET    /v0/chats` / `GET /v0/chats/{id}` — registry / chat detail
- `POST   /v0/chats/{id}/switch`         — provider switch (§8)
- `GET    /v0/chats/{id}/handoff`        — dump assembled handoff (backs `crowbar handoff dump`)
- `POST   /v0/hooks`                      — hook ingest (called by `crowbar hook`; §5)
- WS event channel (existing broadcaster): `segment.opened`, `chat.focus`, `chat.registered`,
  `turn.stopped`, `switch.completed`.

No message endpoint. The terminal PTY stream itself rides the existing terminal WS.

---

## 10. Invariants (held across the iteration)

1. Engine is truth; CLI/GUI/agents are clients. No domain logic in `cmd/crowbar` or endpoints.
2. One code path per operation; provider differences live only in the descriptor.
3. All CLI-specific knowledge is isolated in the YAML. No `if provider == "claude"` in the engine.
4. Crowbar never writes to the PTY; every provider **spawn** routes through the single dispatch
   chokepoint (the switch-injection / logging point). Orchestration is spawn-args + PID lifecycle.
5. Every Crowbar feature is additive/removable without breaking the underlying session
   (Terminal-only stays a real fallback).
6. Uniform surface, honest internals (the engine absorbs provider non-uniformity;
   `config_injection` is honestly per-provider *data*, not forced uniformity).
7. The descriptor reserves `super_harness:` though unused.

---

## 11. Verification (no UI, so this is how we know it works)

- **Phase-0 already proved the mechanisms** end-to-end with a throwaway Python PTY harness
  driving the real binaries (§1 scorecard). Implementation re-verifies the same assertions
  **through Crowbar's Go `engine/terminal`** — the one integration surface the harness did not
  exercise. The harness scripts are retained as executable reference for the test authors.
- **Integration tests drive the real pinned CLIs** in a PTY (tagged, per the black-box
  regression convention), asserting on: single engine path spawning both CLIs; hook payloads
  arriving and mapping to canonical events; ledger accumulation; the detection reducer across
  both "known" and "unknown" session moves; a full switch round-trip + switch-back.
- **`crowbar handoff dump`** for switch legibility.
- **Tauri MCP** reserved for later live exercise once a surface exists; not required this
  iteration.
- **Abstraction litmus:** grep the engine for provider names — zero hits outside `descriptors/`.

---

## 12. Build order

`1 → 2 → 3`, session record is substrate for switching.

- **Phase 0 (spike) — ✅ COMPLETE (2026-07-05).** Every load-bearing claim proven live on the
  real binaries before any engine code (§1 scorecard): dual-provider hook convergence, detection
  by id-change, session-scoped injection, and the full Claude→Codex→Claude switch incl.
  non-polluting resume. The one formerly-open cell — Codex's hook-injection mechanism — is settled:
  relocated `CODEX_HOME` + `--dangerously-bypass-hook-trust`. No open architectural risk remains;
  the rest is Tier-3 plumbing (below) on a proven foundation.
- **Component 1 — Adapter contract + dual PTY control:** extend `engine/terminal` spawn to
  argv+env; `engine/agent` descriptor load + generic inject step-runner; `crowbar hook`
  subcommand + ingest endpoint; canonical events. *Acceptance:* one engine path spawns, and
  receives hooks from, both CLIs; no provider branches in the engine.
- **Component 2 — Session record + detection:** chat/segment model + ledger writer; the
  serialized reducer; `CROWBAR_SEGMENT_ID` attribution. *Acceptance:* accurate registry across
  both CLIs; known-move → focus, unknown-move → adopt; never corrupts the registry.
- **Component 3 — One provider switch:** read-transcript → snapshot → kill → spawn-with-handoff →
  new segment; switch-back via resume; `handoff dump`. *Acceptance:* live switch continues
  coherently; switch-back correct; inspector shows exactly what was captured and injected.

---

## 13. Out of scope (designed for, not built)

Full agent-facing `crowbar` CLI surface (only the `hook` subcommand + install scaffolding now);
super-harness injection (MCP/skills/subagents — `super_harness:` reserved); LSP-over-MCP;
workflows; handoff fidelity tuning (incl. `/compact`-based condensation — the *only* flow that
would ever write to the PTY, out of scope here); all GUI/visual work; per-provider transcript **parsers**
(agents are the only readers of transcript content this iteration); auth handling (the CLI
authenticates itself with the user's existing login — Crowbar never touches credentials);
additional providers (OpenCode, Cursor).
