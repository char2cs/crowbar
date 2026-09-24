# Descriptor channel split — one shape per channel, no cross-shape fallbacks

Status: SPEC + PLAN. Not started.
Date: 2026-09-22

## 1. The problem, precisely

A descriptor maps a vendor CLI's wire payloads onto Crowbar's canonical events.
codex delivers those payloads over **two different channels**:

- the app-server **api** connection (jsonrpc2), and
- the **hook relay** (`crowbar hook <event>`, JSON on stdin, HTTP to the daemon).

Today an event declares ONE static `transport:` and ONE `map:`. Because a single
canonical event can arrive over either channel, the map has to cope with both
wire shapes at once, spelled as `||` alternation:

```yaml
tool_pre:
  in: item/started
  map:
    session_id: "threadId || session_id"     # api name || hooks name
    tool_id:    "item.id || tool_use_id"
```

### 1.1 This is not hypothetical and not dead code

`translate/inbound/hooks.go:42` records the live consequence:

> this used to be skipped whenever `TransportFor(canonical) == "api"` … That
> reasoning breaks for a DUAL-SHAPE event (codex's session_start/user_prompt/
> turn_stop, which inherit the api default but are still ALSO fired hooks-shaped
> by codex's own internal memory-consolidation session) — the skip is keyed on
> the event's static declared transport, not on whether THIS delivery is
> actually hooks-shaped

That mismatch reintroduced the **chat-theft** bug (a foreign codex session's
payload accepted as this chat's conversation). The workaround was to
presence-gate the ownership guard per field instead of consulting transport.

**Root flaw: transport is declared statically per event; shape arrives per
message.** Every `||` cross-shape fallback, and the guard workaround, descend
from that single mismatch.

### 1.2 Census (measured, 2026-09-22)

| file | lines with `||` | cross-shape | field-selection | enum in `when:` |
|---|---|---|---|---|
| codex.yaml | 37 | 28 | ~5 | ~4 |
| claude.yaml | 7 | 0 | 7 | 0 |

All 28 cross-shape alternations sit on events that inherit the `api` default.
The three events explicitly on hooks (`subagent_pre`, `subagent_post`,
`session_end`) carry none. claude has a single channel, so it has no cross-shape
alternation at all — its 7 are legitimate field-selection.

`||` is currently doing **four unrelated jobs** with one glyph:

1. **cross-shape fallback** — `"threadId || session_id"`. Accidental. Delete.
2. **field-selection within one shape** — `tool_input.file_path || tool_input.command || …`.
   Legitimate: one payload shape, the meaningful key differs per tool. Keep.
3. **enum matching in `when:`** — `when: { item.type: commandExecution || fileChange }`.
   Not a fallback at all. Keep, rename.
4. **multiple wire names for one event** — `ask: item/commandExecution/requestApproval
   || item/fileChange/requestApproval` (codex.yaml:323). One canonical event that
   answers to two wire methods. A structural key, not a mapping. Keep, respell.

Worst case today mixes (1) and (2) in ONE expression, with nothing marking the
boundary:

```yaml
tool_target: "item.command || item.query || … || tool_input.file_path"
                └────────── api field-selection ──────────┘   └ hooks fallback ┘
```

This is why a wrong edit there is invisible, and how the `permission` event
shipped with `tool_name: "tool || tool_name"` mapping to empty — no card raised,
turn hung until the budget expired.

## 2. Design

### 2.1 Route by DELIVERY CHANNEL, not by static declaration

Crowbar always knows, at receive time, which channel a payload came in on: the
hook relay posts to the daemon's hook ingress; the api connection delivers on
the jsonrpc2 conn. That fact — not a config field — selects the map.

```yaml
tool_pre:
  required: [session_id, tool_id, tool_name]

  api:
    in: item/started
    when:
      item.type: { any_of: [commandExecution, fileChange, mcpToolCall, webSearch] }
    map:
      session_id:  threadId
      tool_id:     item.id
      tool_name:   { first_present: [item.tool, item.type] }
      tool_target: { first_present: [item.command, item.query, item.server] }
    fixtures: [item-started.commandExecution.json]

  hooks:
    in: PreToolUse
    map:
      session_id:  session_id
      tool_id:     tool_use_id
      tool_name:   tool_name
      tool_target: { first_present: [tool_input.file_path, tool_input.command] }
    fixtures: [pre-tool-use.bash.json]
```

Consequences:

- `threadId` and `session_id` can never appear in one expression again — not by
  convention, but because they live in different blocks.
- The ownership guard belongs to the `hooks:` block by construction. The
  presence-gating workaround in `hooks.go` can revert to an explicit rule.
- A channel a provider does not use is simply absent; its payloads are refused
  with a named error rather than silently half-parsed.

### 2.2 Three operators, three jobs

| today | becomes | meaning |
|---|---|---|
| `"a \|\| b"` across shapes | (removed) | expressed by the channel block |
| `a \|\| b \|\| c` one shape | `{ first_present: [a, b, c] }` | first key present wins |
| `when: { k: a \|\| b }` | `when: { k: { any_of: [a, b] } }` | value-set match |
| `ask: a \|\| b` / `in: a \|\| b` | `ask: [a, b]` / `in: [a, b]` | one event, several wire names |

Each is separately type-checkable by the schema validator.

### 2.3 `required:` — the highest-value rule

Every map entry is best-effort today: a mapping that resolves to nothing
silently yields an empty field. `required:` names the fields whose absence is a
**hard, surfaced error** naming event + channel + descriptor.

This alone would have caught the `tool || tool_name` permission bug.

**Expect this to surface existing latent mismatches on first run.** That is the
point, but it means the migration lands real bugs rather than a clean no-op.

### 2.4 `fixtures:` — no undeclared-untested branches

Each channel block must name ≥1 recorded real payload. The conformance suite
fails when a declared block has no fixture. Fixtures are captured from the real
CLI with its version recorded (`codex app-server generate-json-schema` is the
precedent), so a vendor bump is re-validated mechanically.

### 2.5 `surfaces:` — declare, don't infer

Surface capability is scattered today across `hasTerminal`, `hotswap`, `attach`,
`transport`. Declare it:

```yaml
surfaces:
  chat:     { channel: api }
  terminal: { channel: hooks, start_here: true }
```

`start_here` is the missing fact behind "can't start a chat on the CLI": which
surfaces may be *launched into*, not merely switched to afterwards. A chat-only
provider omits `terminal` and every terminal affordance disappears by absence —
the existing house rule.

### 2.6 Explicit non-support

An absent event means both "provider doesn't emit it" and "we forgot". Add:

```yaml
not_emitted: [plan_update, reasoning_delta]
```

Validator requires every canonical event to be either mapped or listed. Gaps
become countable instead of invisible.

## 2.7 Findings surfaced BY the migration (keep — these are the point)

**F1. codex's api-side permission payload cannot name its tool.** P2b built
fixtures from real captures (`dispatch_test.go`'s verbatim codex-cli 0.149.1
approval payloads) and the conformance suite failed: `tool_name: tool` and
`tool_input: params` map to fields that exist in NEITHER real payload. The api
approval frame carries `threadId/turnId/itemId/reason/command/cwd/...` — the tool
identity is namespaced by the WIRE METHOD (`item/commandExecution/requestApproval`
vs `item/fileChange/requestApproval`), not by any payload field.

So on the api transport a codex permission card renders with an EMPTY tool name.
This is the same defect class as the `tool || tool_name` bug and it was invisible
because `permission` had no fixtures at all before this work. Not fixed in P2b
(field semantics, outside an equivalence-preserving migration). Needs a generic
primitive — deriving a mapped value from the matched wire name, or per-wire-name
sub-blocks — and must NOT be a provider-specific special case in Go.

**F2. A wrong channel now drops an event silently.** The `||` fallback used to
mask a mis-marked delivery by resolving the other shape; channel blocks turn that
into "no block for this channel" and a debug-level drop. That is better than
resolving wrong fields, but it must be LOUD. See P5.

## 3. Non-goals

- Not a lifecycle fix. The stuck-spinner bug (`CloseStalledTurn` not salvaging
  streamed text) was a missing call, not a parsing fault. Parsing rigour and
  lifecycle correctness are separate; do not sell one as the other.
- No new provider vocabulary in Go. This pushes MORE into the descriptor, so the
  validator must get correspondingly stricter.
- claude.yaml barely moves: one channel, keep its 7 `first_present` conversions.

## 4. Plan

Phases are sequential; each must be green before the next starts.

**P1 — schema + operators.** DONE 2026-09-22. Added `any_of` / `first_present`
(`spec/expr.go`) and the job-4 list form on `WireRef` (`spec/wire_ref.go`, covers
`ask:`/`in:`/`out:` since they share the type), alongside the existing `||`. Both
spellings parse; `||` not yet removed. Equivalence proven over all 41 real grammar
lines / 21 distinct expressions from the shipped descriptors (20 subtests).

  P1 IMPLEMENTATION NOTE, load-bearing for P5: the new operators are currently
  COMPILED DOWN to a `||`-joined string via `mapping.Join`. That guaranteed
  byte-identical equivalence for this phase, but it leaves the glyph as the
  INTERNAL encoding. See P5.

**P2 — channel-scoped events.** Add `api:` / `hooks:` blocks to the event schema,
with `required:` and `fixtures:`. Resolution picks the block by the channel the
delivery arrived on. Legacy single-map events keep working.

**P3 — migrate codex.yaml.** YAML AUTHORING TRAP (hit during P1): any path
containing a `[selector]` — `item.content[type=text].text`, `item.changes[0].path`
— MUST be double-quoted inside a `first_present:`/`any_of:` flow sequence, or
YAML misparses `[` as a nested sequence. codex.yaml has several such paths in the
chains that become `first_present:` lists. Split all 10 dual-shape events into channel blocks;
convert the ~5 field-selection and ~4 enum uses. Capture fixtures for BOTH
channels per event — the hooks side is the one currently untested in practice.

**P4 — migrate claude.yaml.** Convert 7 `first_present`; single `hooks:` block
per event. Fixtures.

**P5 — enforce.** Remove `||` from the grammar AND from the internal
representation. Specifically:

  - delete `mapping.Join`; `first_present`/`any_of`/list form must carry a real
    `[]string` through `spec` into the resolver, never a joined string. Leaving
    the compile-to-`||` bridge in place would make the grammar rejection
    cosmetic — the alternation concept would still be a string encoding one
    edit away from returning;
  - the parser ERRORS on `||` in any expression rather than ignoring it;
  - a repo-level test scans every shipped descriptor and fails on any `||`
    outside a comment;
  - turn on `required:`, `fixtures:` and `not_emitted:` as hard validator rules;
  - revert the `hooks.go` presence-gating workaround to an explicit channel rule;
  - make an undeclared-channel delivery a SURFACED error, not a debug drop (F2);
  - resolve F1: a generic way for a mapped field to come from the matched wire
    name, so codex's api permission card can name its tool;
  - fix whatever this surfaces.

**REGRESSION LESSON (2026-09-22).** P2/P3 left the `internal/app/usecases/chat`
test harness feeding api-shaped codex payloads on an unmarked ctx. Those
deliveries began resolving to the hooks channel and dropping, turning that whole
package red. Each agent ran only its own package's tests, so nobody saw it.
**Every remaining phase must run `go build ./internal/... && go vet ./internal/...
&& go test ./internal/...` — the whole internal tree — before reporting done.**

**P6 — surfaces.** `surfaces:` block; wire `start_here` to the new-chat paths.

**P7 — verification.** Full targeted suites, then live Tauri: real claude and
real codex, both surfaces, tool calls, permission prompt, compaction, stop,
and a codex internal-memory-session payload (the chat-theft regression).

## 5. Acceptance

### 5.0 ZERO `||` — non-negotiable, explicitly confirmed by the user

By the end of P5 the `||` glyph must not appear in ANY descriptor, in ANY
position — mapping values, `when:` blocks, `ask:`, `in:`, or anywhere else — and
the grammar must REJECT it rather than merely no longer emitting it. Two
enforcing gates, both required:

- the parser errors on `||` in any expression (not silently ignores it); and
- a repo-level test scans every shipped descriptor and FAILS on any occurrence
  of `||` outside a comment, so a reintroduction cannot land quietly.

All four jobs must therefore have a replacement spelling before `||` is removed:
channel blocks (1), `first_present:` (2), `any_of:` (3), list form (4). A phase
that removes `||` while any job lacks a spelling is not done.

- Zero cross-shape alternations remain in any descriptor.
- Every canonical event is mapped or in `not_emitted:` for both providers.
- Every declared channel block has ≥1 fixture; suite fails otherwise.
- A missing `required:` field raises a named error, proven by a regression test.
- `TestRegression_*` covering the chat-theft payload passes with the guard
  expressed as a channel rule rather than presence-gating.
- Live: claude and codex both drive a full turn with tools on both surfaces.


---

## P6b — per-event `owner:` and `surfaces:` (BIG BANG, replaces the old logic)

User-confirmed 2026-09-22. Crowbar must STOP GUESSING where an event belongs;
the descriptor declares it. Replace the old mechanism outright — no parallel
paths, no compatibility shim.

### The principle (user's words, and the repo's existing law)
Crowbar provides the MECHANISM; the descriptor sets the POLICY. A Crowbar-side
veto on what a descriptor may declare is the same mistake as provider vocabulary
in Go. There is therefore NO validator rule forbidding a descriptor from gating
any event on any surface — the descriptor author knows their provider best.

### Tag 1 — `owner:` (which channel is authoritative)
```yaml
tool_pre:
  owner: api          # api|hooks|either. Absent => either.
  api:   { ... }
  hooks: { ... }
```
DELETE `apiOwnsThisEvent` (`turn/ingest.go`). It guesses with three runtime
conditions, one of which — `descriptor.TransportFor(canonical) == "api"` — is the
STATIC per-event transport that section 1.1 proved is the wrong axis and that
caused the chat-theft bug.

`owner:` replaces that static guess ONLY. The liveness pair
(`HasLiveAPIConnection` + `HasDispatchedOverAPI`) MUST REMAIN: the production
incident recorded above `apiOwnsThisEvent` shows why — a spawn handed its opening
prompt to the companion PTY, the api side "covered" a turn it never carried, and
the turn's ONLY record was dropped as a presumed duplicate. Declaring an owner
says who is authoritative; it does not say whether that owner actually carried
this turn.

Rule: drop a delivery iff `owner` names the OTHER channel AND that channel is
live AND has been dispatched to. Never drop when that would leave zero writers.

### Tag 2 — per-event `surfaces:` (where the event is worth listening to)
```yaml
message_delta:
  surfaces: [chat]            # skip when the CLI's own UI is in front of the user
turn_stop:
  surfaces: [chat, terminal]  # absent => all surfaces; nothing changes unless opted in
```
The surface signal already exists: `Runners.ShowingNativeView(runnerID)`
(`runner/attach.go:93`), whose own doc calls it "the generic signal for 'the CLI's
own UI is the one in front of the user', rather than a provider name". No new
plumbing.

KNOWN LIMIT, must be stated in the descriptor comments, not hidden: it is only
accurate for NON-HOTSWAP providers. A hotswap provider's terminal is a second
window onto a session Crowbar still drives, so it never calls SwitchToTerminal
and this reads false even while the TUI is on screen. So per-event `surfaces:`
is live for codex and inert for claude until that signal covers hotswap.

### Visibility, not veto
The conformance layer should be able to report what a gating implies — e.g. an
event gated off a surface that is the only writer of some ledger fact. Loud and
greppable, like `unverified: true`. NOT a rejection.

### Acceptance
- `apiOwnsThisEvent` no longer exists.
- Both descriptors declare `owner:` on every dual-channel event.
- Duplicate-suppression is driven by `owner:` + liveness, proven by a regression
  test reproducing the companion-PTY echo.
- A regression test proves the zero-writer case CANNOT happen (the production
  data-loss incident, encoded).
- Per-event `surfaces:` gating works, proven against `ShowingNativeView`.
- Whole `./internal/...` tree green; integration regressions green; providers
  verified usable live through the UI (P7).

---

## P8 — Model catalogs: discovered, or manifested. Never declared in Go.

### The defect this closes
Both descriptors carried a hand-typed `model.available` snapshot that drifts
silently. Measured 2026-09-22: codex's list was wrong three ways — missing
`gpt-6-astra` (the CLI's own priority-1 model) and still offering `gpt-5.4`
and `gpt-5.4-mini`, which `codex debug models` no longer returns at all.
Claude's `[sonnet, opus, haiku]` was missing `fable` and `opusplan`, both
live-verified accepted by the real binary.

The second-order defect: the frontend fell back to `models[0]` for a chat with
no stored selection, so a stale list did not merely omit a model — it asserted
a wrong one. That is why the switcher snapped to "sonnet" when a turn ended.

### Two sources, one vocabulary
A provider's models come from exactly one of:

- **`model.discover`** — fork the CLI and map its output. Authoritative.
  Codex: `codex debug models`.
- **`model.manifest`** — read a JSON manifest. For a CLI with no enumerable
  model surface. Claude.

Both feed the SAME item-mapping vocabulary (`items_path`, `keep_when`,
`order_by`, `item.{id,label,efforts,default_effort}`). A descriptor declares
one or the other; declaring both, or either alongside the retired
`model.available`, is a validation error.

### Why claude needs a manifest
Swept 2026-09-22 against the real binary — every outside surface, no internals
read:

- No `models` subcommand. The subcommand set is agents, attach, auth,
  auto-mode, doctor, gateway, import, install, logs, mcp, plugin, project,
  respawn, rm, setup-token, stop, ultrareview, update.
- `--help` names three aliases by example only.
- An invalid `--model` returns prose naming no valid value: "isn't described
  by this version's model catalog … map it with behavesAs on a modelPicker
  row". `modelPicker` is a settings concept, not an enumerable list.
- Bare `--model` opens an interactive TUI picker; it needs a PTY and would
  register as a real turn.
- `GET https://api.anthropic.com/v1/models` is 401 without `x-api-key`.
  Subscription users have no key, the key would have to come from `~/.claude`
  (forbidden), and it answers in API ids, not the CLI aliases `--model` takes.

So claude's list is hardcoded either way. The manifest only moves the
hardcoding OUT of the binary, where a new model is a JSON edit on `develop`
rather than a nightly release. T3Code reached the same split independently —
its manifest carries `claudeAgent` and `antigravity` and deliberately omits
codex, because codex is discoverable.

### Efforts are PER MODEL, and claude's live in the manifest
Effort support varies by model, so a single global list is the wrong shape
whatever its source. Measured on codex: `gpt-6-astra` offers
`[low medium high xhigh max ultra]`, `gpt-5.6-luna` drops `ultra`, and
`gpt-5.5` drops `max` too. The catalog is therefore `model -> [efforts]`
everywhere, and codex's `item.efforts: "supported_reasoning_levels[].effort"`
already maps it that way.

Claude's come from the same manifest rows (`item.efforts: "efforts[]"`), NOT
from parsing `claude --help`. A `--help` regex was prototyped and rejected: it
can only ever yield ONE global enum, which is the shape this section exists to
abolish, and it bought a bespoke text adapter to do it. The manifest needs no
new adapter at all — `adapter: json` plus the shared `pathselect` walk covers
both providers.

Recorded so it is not re-derived: `claude --help` does document an exhaustive
effort enum, `(low, medium, high, xhigh, max)`, and `--effort` itself validates
NOTHING at the CLI boundary (`none` and `ultra` are both accepted silently).
So no per-model difference is measurable for claude today, and every manifest
row carries the full enum. When one turns out to differ, correcting it is a
one-line edit to that row — which is the entire point of the file.

The model flag reads differently and the difference is load-bearing:

    --model <model>    Provide an alias for the latest model
                       (e.g. 'fable', 'opus', or 'sonnet')

`e.g.` — examples, not an enum. Parsing it would yield three models and
silently miss `haiku` and `opusplan`, both confirmed accepted by the real
binary. That asymmetry is the ENTIRE justification for the manifest: model ids
are the one claude fact nothing enumerates.

Also measured: `claude -p --output-format json` reports the resolved model in
`modelUsage` (it named `claude-opus-5-5[1m]` here). A true source, but it runs
a real turn on the user's quota — ~$0.20 for the word "hi" — so it is ruled out
as a probe. It did establish that claude's default is ACCOUNT-DEPENDENT, which
is why no shared manifest may declare one.

### The default is RECEIVED, never computed
Measured 2026-09-22: `codex debug models` carries NO model-level default flag.
Its per-model keys include `priority`, `visibility`, `default_reasoning_level`,
`default_reasoning_summary`, `default_verbosity` — nothing states which model
is the default. (`default_reasoning_level` is the EFFORT default. Do not
confuse the two.) The jsonrpc `model/list` route DOES carry `isDefault`;
`debug models` does not.

So "the catalog's first entry is the default" is a positional convention
standing in for a fact nobody states, and it silently asserts a wrong model the
day ordering changes. It is banned.

Instead: `AgentProvider` carries an explicit `DefaultModel`, empty meaning
UNKNOWN, and unknown is a renderable terminal state rather than a cue to guess.
A source populates it only when it genuinely states it, declared in the
descriptor (`default_when: { field: "isDefault", equals: true }`). A
`debug models` discovery leaves it empty, and that is correct.

This costs nothing, because the default never needed computing:
`selection.Steps` already gates on `if sel.Model != ""`, so an unset selection
omits `--model` entirely, the CLI picks its own default, and the launch report
reports what it picked. The only window without an answer is a chat that has
never run, and "Default" is the honest label for that window — it self-corrects
on the first report.

Corollary: catalog ORDER is cosmetic. It sets display order and which row wears
a default badge. It can never produce a wrong assertion.

Explicitly rejected: a hand-maintained preferred-default list to compensate for
codex not stating one. T3Code does this (`PREFERRED_DEFAULT_CODEX_MODELS`
overrides codex's own flag) and it is precisely the drift being removed.

### Non-negotiable: never assert an unconfirmed model
With discovery, the model list is legitimately EMPTY for a while — before the
first probe returns, and whenever it fails. Therefore:

- The three selection gates (`runner/promptswitch.go`, `runner/selection.go`,
  `conversation/selection.go`) must not reject a non-empty model just because
  the catalog is empty. An empty catalog means UNKNOWN, never DENY.
- The UI renders the last CONFIRMED model, or the literal label "Default". It
  must never fall back to `models[0]`, in ANY window — including a live turn
  whose launch report has not landed yet, and a chat that has never run.

### Acceptance
- Zero occurrences in Go of any provider's model wire vocabulary.
- Codex's `model.available` and its whole per-model `effort.available` map are
  deleted; the probe supplies both.
- Claude resolves from the manifest, with the embedded copy proving offline.
- A test proves the codex filter drops `visibility: hide` and orders by
  `priority`, using a real trimmed capture as testdata.
- A test proves an empty/failed catalog does not reject a non-empty model.
- A test proves the UI renders no model rather than a guessed one.
