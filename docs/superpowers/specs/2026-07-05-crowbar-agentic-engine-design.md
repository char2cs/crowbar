# Crowbar — Agentic Engine & Provider-Switching Core (Iteration 1)

**Status:** design approved, pre-implementation
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
| Hook events | `SessionStart` / `UserPromptSubmit` / `Stop` / … | **same event names** (+ `PermissionRequest`, `SubagentStart`) |
| Hook wire (stdin JSON) | `session_id`, `transcript_path`, `hook_event_name`, `source` | convergent (`hook_event_name`, `hookSpecificOutput`, `stop_hook_active`) |
| Session id | hook payload; **`--session-id <uuid>` to assign**; `--resume <id>` | rollout filename + `session_meta`; `resume <id>` / `--last` |
| Transcript | `~/.claude/projects/<slug>/<uuid>.jsonl` | `~/.codex/sessions/YYYY/MM/DD/rollout-*-<uuid>.jsonl` |
| Sys-prompt inject | `--append-system-prompt` / `--system-prompt-file` | `-c developer_instructions=…` / `AGENTS.md` |

**Headline finding:** Codex's hook system is a near-clone of Claude's (same event names,
same wire shape). The doc's #1 fear — Codex lifecycle granularity — is largely unfounded.

**Two cells still unverified** (confirmed *while building the Codex connector*; they do not
change any interface, each carries a documented fallback):
- Codex `SessionStart.source` granularity (its `move_signal`).
- Codex's hook-injection mechanism (relocate `CODEX_HOME` vs `-c hooks=<path>`) + trust-seeding.

---

## 2. Interaction model (read this before the architecture)

The human talks to the agent by **typing directly into the terminal** — the real CLI TUI in
the PTY, exactly like using it in iTerm2. **There is no "send a message" API, and Crowbar
never writes to the PTY at all.**

- **The terminal's input belongs entirely to the human.** Crowbar injects no keystrokes — not
  messages, not `/compact`, not `/clear`. Writing into a live Ink TUI is fragile (it depends on
  the TUI's current mode) and would race with the user typing. It is also unnecessary (below).
- **All Crowbar orchestration is out-of-band**, three tools only: (a) **spawn-time arguments**
  (`--session-id`, `--resume`, handoff via `--append-system-prompt` / `-c developer_instructions`);
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
id: claude
version:
  pinned: "2.1.201"
  compat_check: "claude --version"      # engine warns on drift

spawn:
  cmd: claude
  interactive_required: true            # engine refuses to launch headless
  forbid_flags: ["-p", "--print"]       # §3 hard guard, enforced in engine
  env: {}                               # cwd supplied by the chat at runtime

session:                                # all spawn-time ARGS — Crowbar never writes to the PTY
  assign:  { arg: "--session-id {uuid}" }   # Crowbar owns the id
  resume:  { arg: "--resume {id}" }

# Injection is expressed as ordered declarative STEPS from a closed generic vocabulary.
# Engine implements the step verbs; it knows nothing about "claude".
config_injection:
  - render_hooks: { format: claude_settings_json, into: "{tmp}/settings.json" }
  - pass_arg:     { arg: "--settings", value: "{tmp}/settings.json" }

hooks:                                  # events to register + field-maps (JSONPath into the payload)
  session_start:
    provider_event: SessionStart
    fields: { session_id: $.session_id, move_signal: $.source, transcript: $.transcript_path }
  turn_stop:
    provider_event: Stop
    fields: { session_id: $.session_id, transcript: $.transcript_path }

move_signal_map:                        # provider raw value → canonical enum
  startup: fresh
  resume:  resumed
  clear:   cleared
  compact: compacted

transcript:                             # for ADOPTION when no hook handed us a path (foreign session)
  locate:  "~/.claude/projects/{cwd_slug}/{session_id}.jsonl"
  content: opaque                       # stored & forwarded, never parsed (POC)

handoff_inject:                         # how a handoff blob enters a fresh spawn
  - pass_arg: { arg: "--append-system-prompt", value: "{handoff}" }

super_harness: {}                       # RESERVED, unused (MCP/skills/subagents — invariant 7)
```

### 4.2 `codex.yaml` (delta only — same shape, this is the abstraction proof)

```yaml
id: codex
version: { pinned: "0.139.0", compat_check: "codex --version" }

spawn: { cmd: codex, interactive_required: true, forbid_flags: ["exec"] }

session:
  # Codex mints its own id on fresh start (no --session-id equivalent confirmed);
  # Crowbar learns it from the first SessionStart hook (correlated by CROWBAR_SEGMENT_ID).
  resume:  { arg: "resume {id}" }

config_injection:                       # the genuine divergence: a procedure, not a flag
  # PENDING VERIFY (Phase 0) — the exact steps below are the leading hypothesis, not confirmed.
  # Open question: relocate CODEX_HOME (isolates the user's real config) vs `-c hooks=<path>`
  # (lighter, but trust-seeding still mutates config). Whichever wins, it stays pure YAML data.
  - set_env:    { name: CODEX_HOME, value: "{tmp}/codex-home" }
  - copy_tree:  { from: "~/.codex", into: "{tmp}/codex-home", except: ["sessions", "logs*"] }
  - render_hooks: { format: codex_hooks_json, into: "{tmp}/codex-home/hooks.json" }
  - seed_trust: { file: "{tmp}/codex-home/hooks.json" }   # writes trusted_hash into config.toml

hooks:                                  # SAME canonical events; move_signal PENDING VERIFY
  session_start: { provider_event: SessionStart, fields: { session_id: $.session_id, move_signal: $.source, transcript: $.transcript_path } }
  turn_stop:     { provider_event: Stop,         fields: { session_id: $.session_id, transcript: $.transcript_path } }

transcript:
  locate:  "~/.codex/sessions/{yyyy}/{mm}/{dd}/rollout-*-{session_id}.jsonl"
  content: opaque

handoff_inject:
  - pass_arg: { arg: "-c", value: "developer_instructions={handoff}" }
```

**Injection step vocabulary (closed set, engine-implemented, zero provider knowledge):**
`render_hooks`, `pass_arg`, `set_env`, `copy_tree`, `write_file`, `seed_trust`.
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
  events (`session_start{session_id, move_signal, transcript}`, `turn_stop{session_id, transcript}`).

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

**The only signal is the `SessionStart` hook** (`{session_id, move_signal}`), attributed to a
segment via `CROWBAR_SEGMENT_ID`. No input-watching — reacting to the hook loses nothing,
because the outgoing context is already in the ledger (snapshotted on every `turn_stop`) and
the vendor's old transcript file persists on disk after a `/clear`.

**One serialized reducer** (single writer → registry never corrupts, an acceptance criterion):

```
on session_start{session_id S, move_signal m}  (segment seg):
    prev = registry.session_for(seg)
    if S == prev:              → no-op (confirmation of a Crowbar-initiated action)
    elif m == cleared:         → close chat-assoc; OPEN NEW chat on this segment
    elif registry.knows(S):    → CASE 1: move focus to chat(S)          (emit chat.focus)
    else:                      → CASE 2: register NEW chat for S; adopt transcript (may lag 1 turn)
    registry.bind(seg → S)

on turn_stop{transcript}:      → append opaque snapshot to that chat's ledger (§6.1)
```

- **"Known"** is well-defined: Crowbar records every session id it spawns/sees. Foreign ids
  (raw-terminal `/resume` of a pre-existing conversation) are the unknowns → adopt.
- **Codex fallback:** if `SessionStart.source` is absent, `move_signal` is unknown but a *move*
  is still detected (session id changed under a known segment). Known→focus, unknown→adopt
  still hold; only clear-vs-foreign flavor is blurred, and both actions converge on "track it".

---

## 8. Provider switch (Component 3)

User-initiated, orchestrated by Crowbar (not a user "send"):

```
POST /chats/{id}/switch { provider: codex }
  1. read the outgoing session's transcript from disk → append an opaque snapshot to the ledger
     (the ledger already holds prior turns from turn_stop; this captures anything since the last Stop)
  2. assemble the handoff blob from the ledger
  3. gracefully terminate the outgoing CLI process  # kill-on-switch (a PID signal, NOT a PTY write)
  4. spawn the target CLI in the same chat as a NEW segment, injecting the handoff at spawn time
     (descriptor handoff_inject) + a fresh CROWBAR_SEGMENT_ID
  5. new segment bound to the chat; active_segment updated
  # No keystrokes are ever written to any PTY.
```

- **Switch-back** reuses `session.resume`: returning to a prior provider resumes its old
  session id, restoring **native** context (higher fidelity than a re-injected summary) — so
  we never keep idle processes alive.
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

- **Phase 0 (spike, carried into the plan):** confirm the pending cells against the real
  binaries: (a) Codex `SessionStart.source` granularity; (b) Codex's hook-injection mechanism
  (relocate `CODEX_HOME` vs `-c hooks=<path>`) and trust-seeding. Each has a documented fallback
  so a negative result reshapes a descriptor cell, not the architecture.
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
