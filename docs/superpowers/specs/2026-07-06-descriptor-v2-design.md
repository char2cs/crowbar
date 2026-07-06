# Descriptor v2 — Total Provider Abstraction & Hook-Derived Conversation Log

**Date:** 2026-07-06
**Status:** design, pending approval
**Supersedes / amends:** [`2026-07-05-crowbar-agentic-engine-design.md`](./2026-07-05-crowbar-agentic-engine-design.md) §2 (opaque-transcript-blob), §5 (adapter read side), §6 (ledger). The 2026-07-05 doc's product thesis, global constraints (§3), reducer (§7), and switch lifecycle (§8) stand unchanged.

---

## 0. In plain terms

Each agentic CLI (Claude Code, Codex) is described by one YAML file. The whole idea: **to support a new CLI, you write a new YAML file — you never touch Go code.**

Right now that promise is broken in two places, because two per-CLI facts are still hardcoded in Go instead of living in the YAML:

1. **The hook-config file we hand the CLI.** When Crowbar launches a CLI, it writes a small config telling the CLI "run this Crowbar command when a turn ends." The exact shape of that config file is different for every CLI — and today Go builds it. So a new CLI with a differently-shaped config can't be added by YAML alone.
2. **We read the CLI's own conversation-history file off its disk.** To carry context across a provider switch, Crowbar opens the vendor's transcript file and copies it. That file's format and location differ per CLI, and it's full of noise (tool calls, sub-agent chatter) we don't want.

**What v2 changes:**

1. **Move that config file's contents into the YAML, written out word-for-word.** Go stops knowing its shape — it just fills in a few `{…}` blanks and writes whatever the YAML author put there. New CLI, new shape → new YAML, no Go change.
2. **Stop reading the CLI's history file entirely.** The CLI's hooks already hand us the user's message and the agent's reply as they happen. We build our *own* clean conversation record from those two signals. Crowbar never reads another program's files, and the record is just user + assistant messages — none of the tool-call noise.

**Result:** a new CLI is genuinely just a new YAML file, and the conversation Crowbar carries across a switch is a clean chat log we built ourselves. The rest of this document is the detailed engineering of exactly that.

---

## 1. Why v2 exists

The v1 descriptor claims "new provider = new YAML, zero engine changes." It does not hold. Two provider-specific facts are still baked into Go:

1. **The hook-config wire shape is hardcoded.** `inject.go:renderHooks` emits the literal `{"hooks":{"<Event>":[{"hooks":[{"type":"command","command":...}]}]}}` nesting. That shape is Claude/Codex-specific. A CLI whose hook config is a TOML table, a flat array, or a different JSON nesting cannot be onboarded by YAML alone — it needs a new Go renderer. The `provider_event` field and the `render_hooks` verb are the leak.

2. **Crowbar reads the vendor's transcript file.** `handleTurnStop` does `os.ReadFile(ev.Transcript)` and stores the vendor's raw transcript as an opaque ledger blob. This couples Crowbar to each CLI's on-disk transcript format and location, drags in every CLI's transcript bloat (tool calls, sub-agent spawns, intermediate narration), and means the "conversation" Crowbar hands off is a foreign file it does not understand.

v2 removes both. After v2, **every provider-specific fact — write side and read side — is declarative YAML data**, and Crowbar never reads a file another process wrote.

## 2. Principles (unchanged, now actually enforced)

- **Descriptor purity.** All per-CLI knowledge lives in one YAML descriptor. Engine Go is provider-agnostic: no `if provider == …`, no provider-specific literal shapes, no provider-specific field names.
- **Observe, never proxy.** Crowbar spawns the real interactive CLI in a PTY and never writes to its product stdin. It learns what happened only through the CLI's own side channels (hooks). It reads no file the CLI wrote.
- **The conversation is built from hooks.** Crowbar's record of a chat is assembled from two hook signals — the user's submitted prompt and the agent's final turn message — not by parsing a transcript. This is deliberately the *clean* conversation (final message per turn), free of tool guts.

## 3. The descriptor, end to end

Two provider descriptors, fully annotated. Everything provider-specific is in these files.

### 3.1 `claude.yaml`

```yaml
id: claude
spawn:
  cmd: claude
  interactive_required: true
  forbid_flags: ["-p", "--print"]
  env:
    clear: ["CLAUDE_CODE_CHILD_SESSION", "CLAUDECODE"]

session:
  resume: { arg: "--resume {id}" }

# ---- WRITE SIDE ----------------------------------------------------------
# The hook-config file is authored LITERALLY. Native event names
# ("SessionStart"/"UserPromptSubmit"/"Stop") and the file's JSON shape are the
# author's, never Go's. Crowbar only substitutes {…} variables.
config_injection:
  - write_file:
      path: "{tmp}/settings.json"
      content: |
        {"hooks":{
          "SessionStart":[{"hooks":[{"type":"command",
            "command":"{crowbar_hook} hook session_start --segment {segid} --provider {provider}"}]}],
          "UserPromptSubmit":[{"hooks":[{"type":"command",
            "command":"{crowbar_hook} hook user_prompt --segment {segid} --provider {provider}"}]}],
          "Stop":[{"hooks":[{"type":"command",
            "command":"{crowbar_hook} hook turn_stop --segment {segid} --provider {provider}"}]}]
        }}
  - pass_arg: { arg: "--settings", value: "{tmp}/settings.json" }

# ---- READ SIDE -----------------------------------------------------------
# Left column = Crowbar's FIXED vocabulary. Right column = where THIS CLI puts
# that value in its hook payload (dotted path for nesting). The daemon parses
# the raw payload per `format`, then extracts these fields.
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    user_prompt:   { message: prompt }
    turn_stop:     { session_id: session_id, message: last_assistant_message }

handoff_inject:
  - pass_arg: { arg: "--append-system-prompt", value: "{handoff}" }
```

### 3.2 `codex.yaml`

```yaml
id: codex
spawn:
  cmd: codex
  interactive_required: true
  forbid_flags: ["exec"]
  args: ["--dangerously-bypass-hook-trust"]

session:
  resume: { arg: "resume {id}" }

config_injection:
  - set_env:    { name: CODEX_HOME, value: "{tmp}/codex-home" }
  - write_file: { path: "{tmp}/codex-home/auth.json", from: "~/.codex/auth.json" }
  - write_file:
      path: "{tmp}/codex-home/config.toml"
      content: |
        [projects."{cwd}"]
        trust_level = "trusted"
  - write_file:
      path: "{tmp}/codex-home/hooks.json"
      content: |
        {"hooks":{
          "SessionStart":[{"hooks":[{"type":"command",
            "command":"{crowbar_hook} hook session_start --segment {segid} --provider {provider}"}]}],
          "UserPromptSubmit":[{"hooks":[{"type":"command",
            "command":"{crowbar_hook} hook user_prompt --segment {segid} --provider {provider}"}]}],
          "Stop":[{"hooks":[{"type":"command",
            "command":"{crowbar_hook} hook turn_stop --segment {segid} --provider {provider}"}]}]
        }}

hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    user_prompt:   { message: prompt }
    turn_stop:     { session_id: session_id, message: last_assistant_message }

handoff_inject:
  - pass_arg: { positional: "{handoff}" }
```

> Note: Claude and Codex share identical read-side field names (`session_id`, `prompt`, `last_assistant_message`) and identical hook-config JSON shapes — Codex evidently modeled its hooks on Claude's. The two descriptors looking near-identical is therefore *weak* evidence of generality; see §9.

### 3.3 The four contracts a descriptor spans

| Contract | Who owns it | Where it lives |
|---|---|---|
| **Guaranteed variables** | Crowbar (fixed) | `{tmp}`, `{cwd}`, `{crowbar_hook}`, `{segid}`, `{provider}`, `{id}`, `{handoff}` |
| **Verb set** | Crowbar (closed) | `set_env`, `write_file` (with `content` or `from`), `pass_arg` (with `arg`+`value`, or `positional`) |
| **Canonical events** | Crowbar (closed) | `session_start`, `user_prompt`, `turn_stop` |
| **Payload format** | Crowbar (closed, extensible) | `hooks.format` ∈ {`json`} today |

Everything else — native event names, config-file shape, field paths, flags, env keys, resume syntax — is free-form author data.

## 4. The round trip

```
                    ┌──────────────────────── author-controlled (YAML) ──────────────────────┐
spawn: write_file renders the literal hook-config with {segid}/{provider}/{crowbar_hook} filled in
                    └────────────────────────────────────────────────────────────────────────┘
CLI runs a turn ──▶ fires its native "Stop" hook ──▶ runs the authored command:
    crowbar hook turn_stop --segment S1 --provider claude   (payload piped on stdin)
                    │
                    ▼
crowbar hook: read raw payload bytes (stdin) → POST /v0/agent/hooks
    { segment_id:"S1", provider:"claude", event:"turn_stop", payload_raw:"{…}" }
                    │
                    ▼   ┌──────────────────────── daemon (provider-agnostic) ────────────────┐
IngestHook: resolve active segment by S1 → descriptor(seg.ProviderID)
    parse payload_raw per descriptor.hooks.format (json) → map
    MapHook(turn_stop, map) using events.turn_stop field paths →
        CanonicalEvent{ session_id:"3501…", message:"acknowledged" }
    reducer (session_start only) / ledger append (user_prompt, turn_stop)
                    └────────────────────────────────────────────────────────────────────────┘
```

**Key move:** the native event name (`Stop`) is converted to the canonical name (`turn_stop`) at *authoring time* — the author wrote `crowbar hook turn_stop` inside the `"Stop"` block. The daemon receives the canonical name directly and never sees, nor maps, native event names. That is why `provider_event` disappears from Go.

## 5. `crowbar hook` — dumb byte forwarder

`api/cmd/crowbar/hook.go` becomes provider- and format-agnostic:

- **Args/flags:** `crowbar hook <canonical-event> --segment <id> --provider <id>`. `<canonical-event>` is `args[0]`; `--segment` and `--provider` are required flags. Payload source defaults to **stdin**; `--payload-file <path>` and `--payload <inline>` are accepted for CLIs that deliver otherwise (the author selects in the literal command). This replaces reading `$CROWBAR_SEGMENT_ID`.
- **No JSON decode.** It reads the raw payload bytes (capped, as today) and forwards them **verbatim** as `payload_raw` (string). Parsing is the daemon's job (it holds the descriptor and thus the `format`). This kills the current `{_raw:…}` fallback and makes `hooks.format` genuinely load-bearing.
- **Still never breaks the CLI:** all errors swallowed to exit-0, surfaced on stderr only.

Wire body: `{ segment_id, provider, event, payload_raw }`.

## 6. Read-side extraction (engine/agent)

- **`Descriptor.Hooks`** changes from `map[string]HookMap` to:
  ```go
  type HookSpec struct {
      Format string                       `yaml:"format"` // "json"
      Events map[string]map[string]string `yaml:"events"` // canonical -> (vocab -> path)
  }
  ```
  `HookMap` and its `ProviderEvent`/`Fields` shape are deleted.
- **`ParsePayload(raw []byte) (map[string]any, error)`** on the descriptor dispatches on `Format`. Today: `case "json"`. An unknown format is an explicit error (documented boundary, §9).
- **`CanonicalEvent`** replaces `Transcript string` with `Message string`.
- **`MapHook(canonical, payload)`** looks up `Events[canonical]` and extracts `session_id` + `message` via dotted paths.
- **`extract`** gains dotted-path descent (`a.b.c`) and drops the `$.` prefix convention (bare paths now).
- **`Validate`** requires: `hooks.format` non-empty; `events.session_start.session_id` mapped; and — new — `events.turn_stop.message` mapped (a provider that can't report the agent's turn message can't feed the ledger).

## 7. Conversation ledger — hook-derived turns

`api/internal/app/ledger/ledger.go` stops storing opaque transcript snapshots and stores **conversation turns**:

- **`AppendTurn(role, provider string, at time.Time, text string) (string, error)`** — `role` ∈ {`user`, `assistant`}. Entry filename keeps the `%08d-<utc>-…` chronological prefix; content is a small JSON record `{role, provider, text, at}`. Empty `text` is skipped (never write a blank turn).
- **`RenderConversation() ([]byte, error)`** replaces `ReadAll`: reads every entry in order and renders a legible plain-text conversation (`user: …` / `assistant (claude): …`) for the receiving model. This is the clean, bloat-free log the product wants.

Usecase (`agent.go`) read-side rewiring:
- `IngestHook` switch gains **`case "user_prompt"`** → `AppendTurn("user", seg.ProviderID, now, ev.Message)`; broadcast a lifecycle event.
- **`handleTurnStop`** drops `os.ReadFile(ev.Transcript)`; it does `AppendTurn("assistant", seg.ProviderID, now, ev.Message)`. Empty message → no-op (skip).
- `persistBound` / `persistRegistered` drop `seg.TranscriptPath = ev.Transcript`.
- **`domain.AgentSegment.TranscriptPath` is deleted** (no migration — pre-production, per project convention). `agent_gaps_test.go` and `agent_test.go` assertions on it are rewritten (§10).
- `AssembleHandoff` calls `RenderConversation()` instead of `ReadAll()`; the preamble/footer wrapper is unchanged.

## 8. Spawn-side rewiring (engine/agent + usecase)

- **`render_hooks` verb deleted** from `inject.go` (`case "render_hooks"` + `renderHooks`). Hook configs are now plain `write_file` steps.
- **`TemplateCtx`** gains `Segid` and `Provider`; `Expand` adds `{segid}` and `{provider}`. (The unused `{uuid}`/`UUID` and `{cwd_slug}`/`CwdSlug` are removed if no descriptor references them — verified none do.)
- **`spawnSegment`** sets `tctx.Segid = segID`, `tctx.Provider = providerID`, and stops appending `CROWBAR_SEGMENT_ID=` to the env (attribution now rides in the hook command via `{segid}`).
- The engine still hard-guards `forbid_flags` on the assembled argv (unchanged, load-bearing global constraint).

## 9. Compatibility contract (the "does it scale?" answer)

v2 does **not** scale to *any* CLI. It scales to CLIs that meet an explicit contract. A CLI is Crowbar-compatible iff:

1. It fires **shell-command hooks** on lifecycle events (not webhooks/sockets only).
2. Those events include a **session-start**, a **user-prompt**, and a **turn-end**.
3. The turn-end payload carries the **agent's final message**; user-prompt carries the **user's text**; both carry a **session id** that **changes on a new conversation** (the reducer's detection substrate).
4. It allows **session-scoped config injection** (a flag or relocatable config home) without mutating the user's global config.
5. Its payload is in a **supported `format`** (JSON today), obtainable by a supported delivery (`stdin`/`--payload-file`/`--payload`).

**Within the contract:** onboarding = one new YAML, zero Go.
**Outside it:** engine work — a new `format` parser, a new verb (e.g. a `run` verb for CLIs that register hooks via a subcommand rather than a config file), a new canonical event, or richer path extraction. These are the closed "instruction set"; extending it is a Crowbar release, not a descriptor.
**Hard floor:** a CLI with no shell-command hooks *and* no readable side channel is unsupportable by any descriptor — a structural consequence of "observe, never proxy," not a YAML gap.

**Flagged risk — generality is unproven.** Claude and Codex share one hook schema family (§3.2). Two near-duplicate proofs are weak evidence. The contract above is a hypothesis we designed *for*; it is validated only by onboarding a genuinely different third CLI (OpenCode / Gemini CLI / Cursor). This spec does not do that; it is called out as the top open risk.

## 10. Testing plan — parity plus the new behavior

Bar: the unit suite and the `integration`-tagged suite pass, green as before, plus the v2 behavior.

**Unit (engine/agent):**
- `descriptor_test` / `descriptor_error_test`: drop `ProviderEvent`; assert new `HookSpec` (`format`, `events`), new `Validate` rules (format required, turn_stop.message required).
- New `parse_test`: `ParsePayload` json + unknown-format error.
- `hooks_test`: `MapHook` extracts `session_id` + `message`; dotted-path descent; missing field → "".
- `inject_test` / `inject_error_test`: remove `render_hooks` cases; assert `write_file` renders literal `content` with `{segid}`/`{provider}`/`{crowbar_hook}` substituted; assert the deleted verb is now an "unknown verb" error.
- `template_test`: `{segid}`/`{provider}` expand; removed vars gone.

**Unit (ledger):** rewrite for `AppendTurn`/`RenderConversation` — ordering, role/provider rendering, empty-text skip.

**Unit (usecase agent):** `agent_test` — `user_prompt` appends a user turn; `turn_stop` appends an assistant turn from `ev.Message` (no file read); handoff renders the conversation; drop `TranscriptPath` assertions; assert the hook command in the spawn plan carries `--segment <segID> --provider <provider>` and env no longer carries `CROWBAR_SEGMENT_ID`.

**Unit (cmd):** `hook_test` — `--segment`/`--provider` flags parsed; raw payload forwarded verbatim as `payload_raw`; stdin default + `--payload-file`.

**Integration (`api/tests/integration/agent/agent_gaps_test.go`):** the cross-provider handoff tests currently *assert on `TranscriptPath`* and read vendor transcript files as their oracle. Rewire:
- Verification shifts to Crowbar's **ledger/`RenderConversation`** — assert the handoff Crowbar assembled contains the prior turns.
- Vendor transcript files may still be read *as a test oracle* to confirm the injected handoff reached the next CLI (that is the test peeking, not Crowbar depending on it), but assertions that Crowbar *recorded* a transcript path are deleted.
- Native `--resume` / switch-back tests keep asserting session-id continuity (reducer behavior, unchanged); they drop transcript-path-equality assertions.

**Live verification (per project rule):** after green tests, sample a real Claude↔Codex switch in the running app via `make dev-desktop` and confirm the handoff conversation is the clean turn log, not a transcript slurp. Tests ≠ live proof.

## 11. Out of scope / deferred

- **Storage-layout reorg** (`spawns/<segid>/`, `chats/<chatid>/ledger/`): keep today's `agent-tmp/<segID>` + `AgentLedgerDir`; `{tmp}` semantics (per-segment scratch) unchanged. Renaming risks `sweepStaleAgentTmp`; not worth it now.
- **`run` verb** for subcommand-registration CLIs: named in the contract as an example extension, not built.
- **Multi-format parser** (TOML/keyval/XML payloads): `format` dispatch exists; only `json` implemented.
- **Per-provider transcript parser / markdown viewer:** still deferred to when Crowbar's *own code* (not a model) must read transcript internals — now even further off, since v2 removes all transcript reading.
- **Third-CLI onboarding** (§9 risk): the real generality test; separate effort.

## 12. Flagged decisions for review

1. **Raw-forward vs. decode-in-hook.** Spec chooses `crowbar hook` forwards raw bytes and the daemon parses per `format` (makes `format` load-bearing, kills `{_raw}`). Lighter alternative: keep JSON decode in `crowbar hook`, treat `format` as contract documentation. Recommend raw-forward.
2. **`--provider` role.** It selects the descriptor and makes the hook self-describing; the active segment's `ProviderID` remains the source of truth, and `IngestHook` asserts they match (warns on mismatch) so `--provider` is a guard, not dead config. Alternative: drop `--provider`, derive solely from the segment.
3. **Read-side `via` key dropped.** Delivery (stdin/file/arg) is encoded in the authored command via `crowbar hook` flags, not a separate YAML key — one source of truth. `format` stays on the read side. Confirm you're fine collapsing `via` this way.
