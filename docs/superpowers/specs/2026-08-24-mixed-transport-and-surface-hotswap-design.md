# Mixed transport, and surfaces a provider declares

**Date:** 2026-08-24

**Status:** Design spec, for a separate session. Nothing here is built.

**Scope:** two capabilities that share one seam in the agent engine.

1. **Mixed transport.** A provider speaks over the API for some events and hooks for
   others, on ONE session. Concretely: codex over `codex app-server`, with a TUI
   attached to the same conversation.
2. **Surface hotswap.** The descriptor declares whether its two faces — Crowbar's chat
   and the provider's own terminal — can be live at the same instant. That answer
   decides whether the surface switcher is free, gated, or absent, and whether split
   exists at all.

They are one piece of work because `api.attach` is what makes hotswap true for codex,
so both land in the same descriptor change.

**Not in scope:** claude's descriptor (it already gets both faces and both capabilities
resolve to their permissive values). The chat surface's components — every capability
below is already key-presence in the UI and needs wiring, not design.

---

## 1. What is true today

Everything in this section was read out of the tree on 2026-08-24. It matters because
the descriptor format is well ahead of the engine, and a reader who assumes otherwise
will look for code that does not exist.

### 1.1 The schema already supports mixed transport

`api/internal/engine/agents/internal/spec/v3.go`:

- `RuntimeSpec.Transport` (:21) — "the default for every event that declares none:
  hooks | api | oneshot".
- `RuntimeSpec` carries `API APISpec` and `Hooks HooksWire` **side by side** (:22–23).
- `EventSpec.Transport` (:53) overrides the runtime default per event. Its own comment
  says what it is for: *"This is the whole mechanism behind a MIXED provider — API for
  turns, hooks for permissions."*
- `Descriptor.TransportFor(event)` (:104) resolves the pair.

`APISpec` (:27) declares how the process is started and how the terminal is obtained:

- `Serve []string` (:30) — "starts the server Crowbar speaks to".
- `Attach []string` (:33) — *"points a TUI at the SAME conversation, so one session
  yields both the structured protocol and the terminal pane with no screen scraping."*

**`attach` is the load-bearing field for capability 2.** It is why adopting the API does
not cost codex its terminal, and why codex on the API would need no `terminal_prompts`
needles at all — screen scraping stops being how Crowbar learns anything.

### 1.2 None of it is implemented

- `TransportFor` has **zero callers** anywhere in `api/`.
- `API.Serve` and `API.Attach` are **never read** anywhere in `api/`.
- The only runtime use of transport is `return d.Runtime.Transport` (v3.go:108).

The ingestion path assumes one provider, one wire.

### 1.3 The API descriptor is inert

`descriptors-v3/experimental/codex-api.yaml` exists, declares `transport: api`,
`protocol: jsonrpc2`, `serve: [codex, app-server, --listen, "unix://{socket}"]`,
`attach: [codex, --remote, "unix://{socket}"]`, `handshake: { call: initialize }`.

It is not reachable: the embed is `//go:embed descriptors-v3/*.yaml`
(`descriptor.go:18`), a single-star glob that does not descend into `experimental/`.
The live daemon's provider list is exactly `["claude", "codex"]`.

**Its 13 recorded fixtures** live at
`api/internal/engine/agents/internal/protocol/testdata/fixtures/codex-api/`, captured by
`scripts/capture-codex-fixtures.sh`. **No Go file references them.** The descriptor's
header says its paths were "verified against RECORDED traffic", and that was presumably
true when written, but there is no standing test asserting it. Treat the leaf paths as
*captured*, not as *proven*, and make replaying those fixtures the first task.

### 1.4 What each transport actually carries

Counted from the `events:` block of each descriptor.

| event | claude | codex (hooks, shipped) | codex-api |
|---|---|---|---|
| session_start | ✔ | ✔ | ✔ |
| user_prompt | ✔ | ✔ | ✔ |
| turn_stop | ✔ | ✔ | ✔ |
| tool_pre / tool_post | ✔ | ✔ | ✔ |
| permission | ✔ | ✔ **`answerable: false`** | ✔ **answerable** |
| message_delta | ✔ | ✘ | ✔ |
| elicitation | ✔ | ✘ | ✔ |
| telemetry | ✘ | ✘ | ✔ |
| interrupt | ✘ | ✘ | ✔ |
| compact_start | ✔ | ✘ | ✔ |
| prompt (out) | ✘ | ✘ | ✔ |
| subagent_pre / subagent_post | ✔ | ✔ | ✘ |
| compact_pre / compact_post | ✔ | ✔ | ✘ |
| session_end | ✔ | ✔ | ✘ |
| turn_failed / tool_fail / notification | ✔ | ✘ | ✘ |

Totals: claude 17, codex-hooks 11, codex-api 12.

The API additionally declares catalogs it can call — `model/list`, `skills/list`,
`account/rateLimits/read` — and an `inject` path (`config/mcpServer/reload`,
`thread/inject_items`).

**Read this as a trade, not a count.** The API gains the whole interactive core
(streaming, answerable permissions, telemetry, interrupt, compact) and loses five
observational events. `codex.yaml`'s header frames the choice as "fewer events", which
is numerically true and materially misleading; that framing is what this spec revises.

### 1.5 The frontend is already built for both capabilities

- `ViewSwitcher` accepts `handoverBlocked` and renders the terminal segment `disabled`
  with the tooltip *"This provider cannot hand a live turn over — finish or stop it
  first"* (`controls/view-switcher.tsx:17, 35, 56`). It threads through
  `SelectionCluster` and `ProviderBar`. **Nothing ever passes it.** It is a dead prop.
- Split is gated on `SPLIT_PRESENTATION_AVAILABLE = import.meta.env.DEV` plus the
  `chatSplitPresentationEnabled` setting (`features/settings/lib/chat-presentation.ts:55,
  70`) — a dev flag standing in for a provider fact.
- Provider capabilities already ride the wire as key-presence booleans:
  `mcpEnabled`, `compaction`, `modelSelect`, `effortSelect` (`dto/agent.go:528–531`),
  each defaulting in the safe direction in `mapProvider`.

So both capabilities are frontend-complete. The work is descriptor, engine, and wire.

---

## 2. Capability 1 — mixed transport

### 2.1 The shape: one codex, not two

Do **not** ship `codex-api` as a second provider. Two entries would make the user choose
between halves of one capability, and the halves are complementary rather than
alternative.

Merge into a single `descriptors-v3/codex.yaml`:

- `runtime.transport: api` — the default for every event.
- `runtime.api` — `serve`, `attach`, `handshake`, `protocol` from the experimental file.
- `runtime.hooks` — retained, because the attached TUI still fires them.
- Per-event `transport: hooks` on exactly the five the API does not carry:
  `subagent_pre`, `subagent_post`, `compact_pre`, `compact_post`, `session_end`.

The hook injection codex.yaml already performs (`-c hooks.SessionStart=[…]` and
siblings) stays: it is what makes the attached TUI report those five. `terminal_prompts`
and `terminal_notices` can be **deleted** once the API path is live — a structured
protocol reports the trust modal and the usage-limit banner directly, and screen needles
are a fallback for a transport that cannot.

`experimental/codex-api.yaml` is deleted by this work, not kept alongside.

### 2.2 Engine work

Three pieces, in dependency order.

**(a) Replay the fixtures first.** Before any runtime change, add a test that drives the
13 captured payloads through the API descriptor's `map:` paths and asserts the canonical
events that come out. This is the standing proof §1.3 says does not exist. If a leaf
path is wrong, it is far cheaper to learn it here than against a live process.

**(b) Spawn: serve, then attach.** The runner currently assumes one process and one PTY.
It needs to start `serve` on a socket, complete the `handshake`, and then start `attach`
in the PTY it already knows how to manage. The socket path is descriptor-templated
(`{socket}`) and must live where the existing socket rules put it — see
[[project_dev_home_isolation]] for why a socket cannot sit inside a Crowbar worktree
(macOS `sun_path` is 104 bytes).

Failure of `attach` must **not** fail the session. If the server is up and the TUI is
not, the chat works and the terminal surface is absent — which is exactly the third
state in §3.1, reached at runtime rather than by declaration.

**(c) Route by `TransportFor`, not by provider.** Ingestion currently assumes one wire.
It needs to accept canonical events from both the JSON-RPC client and the hook endpoint,
for the same runner, and reconcile them onto one chat aggregate. Two constraints:

- **Session identity.** The hook payload's `session_id` and the API's `threadId` must
  resolve to the same Crowbar chat, or the same turn arrives twice under two identities.
  `require_payload_fields: [transcript_path]` exists to reject another CLI's traffic;
  the API side needs the equivalent.
- **No double-counting.** `turn_stop` arrives on both wires. `TransportFor` decides
  which one is authoritative for a given event, and the other must be dropped rather
  than merged.

A JSON-RPC client speaking `jsonrpc2` with the `initialize` handshake is new code:
inbound notifications, outbound calls (`turn/start`, `turn/interrupt`,
`thread/compact/start`), and `ask` replies (`item/permissions/requestApproval` with the
declared `allow`/`deny` templates).

### 2.3 What the user gets

No frontend work. Every one of these is already key-presence:

- Compact button appears (`compact_start` declared → `compaction: true`).
- Context gauge fills and rate limits appear (`telemetry`).
- Answers stream instead of landing whole (`message_delta`).
- **Permission requests become answerable from the chat** — the read-only choice card
  with *"Answer this in the terminal"* becomes a real one.
- Structured questions surface (`elicitation`).
- Stop sends a real interrupt instead of killing the runner.

Losses to state plainly in the changelog: no subagent shelf for codex, and compaction
boundaries only for compactions Crowbar itself started.

---

## 3. Capability 2 — surface hotswap

### 3.1 Three states, not two

| terminal surface | switcher | split |
|---|---|---|
| exists, both faces live at once | free at any time | offered |
| exists, not simultaneously | terminal segment **disabled** while a turn is open | not offered |
| does not exist | **no switcher at all** | not offered |

The third row is the house rule and is the reason this is not a single boolean: a
provider served with no `attach` has no terminal, and a disabled control would claim one
exists. Absence is for "not ever"; disabled is for "not right now".

### 3.2 Derive existence, declare simultaneity

**Existence is structural** and must be derived, not declared, because it is already
knowable from what Crowbar was told to run: a `spawn.cmd` under the hooks transport, or
`api.attach` under the API transport. Deriving it is reading Crowbar's own configuration,
not inferring provider behaviour — and a separate boolean could contradict it.

**Simultaneity is behavioural** and must be declared:

```yaml
runtime:
  hotswap: true    # both faces are live on one session
```

Defaulting **false** on absence, the same direction as every other capability key: a
descriptor that has not thought about it gets the conservative answer, and a user is
told to finish the turn rather than being allowed a swap nobody verified.

Resist splitting this into two bits (swap-mid-turn vs. split) until a real provider needs
the distinction. `codex.yaml`'s own rule applies: *"A needle gets a kind when a specific
one has been captured, never before."*

### 3.3 Gate on the turn being OPEN, not on `working`

This is the subtle one and the easiest to get wrong.

A chat sitting on a permission is not streaming, so `working` is false — but the CLI is
mid-turn, and that is precisely the worst moment to hand the conversation over. The gate
must be "a turn is open", which includes waiting on a decision, not "the agent is
producing output".

### 3.4 Wire and frontend

- Add `hotswap` and `hasTerminal` to `AgentProviderDTO` beside the existing capability
  booleans (`dto/agent.go:528–531`), and map both through `mapProvider` in
  `web/src/features/agent/api/agent-api.ts` with `?? false` — a daemon that does not
  send them must degrade to the conservative answer. (There is a regression test for
  exactly this class of bug: `mapProvider` rebuilds field by field, and a key it forgets
  is dropped in silence.)
- Feed the dead prop: `AgentChatView` passes
  `handoverBlocked={!provider.hotswap && turnIsOpen}` into `SelectionCluster`.
- Hide the switcher entirely when `!provider.hasTerminal`.
- Offer split only when `provider.hotswap`. The dev flag may remain as a second gate —
  capability says *may*, the setting says *shown* — but it must stop being the only one.

### 3.5 What both shipped providers declare today

Both claude and codex-on-hooks keep the PTY attached for the whole session with hooks
reporting alongside, so **both declare `hotswap: true`** and both have a terminal. The
`false` case has no provider yet. Declare it anyway: it is what stops the switcher from
being a lie the first time a provider cannot do it, and it retires a dev flag that is
standing in for a provider fact.

---

## 4. Order of work

1. Replay the 13 captured fixtures against the API descriptor's paths. Fix what the
   replay disproves. **Do not skip this.**
2. Land capability 2 alone, against the current hooks transport: `hotswap` +
   `hasTerminal` on the descriptor and the wire, the dead prop fed, split gated on the
   capability. Ships value immediately and is independently verifiable.
3. JSON-RPC client + handshake, exercised against the fixtures.
4. Spawn serve+attach.
5. Route ingestion by `TransportFor`.
6. Merge codex into one descriptor, delete `experimental/codex-api.yaml`, delete
   `terminal_prompts` / `terminal_notices` once the structured path reports them.

Steps 2 and 3 are independent and can proceed in parallel.

---

## 5. Verification

A green suite is not evidence for any of this. The failure modes here are all live ones.

- **Live-verify in the desktop app**, driving the Tauri MCP bridge — not headless. See
  [[feedback_verify_via_dev_desktop_not_headless]]. The dev daemon must run under the
  worktree's isolated `CROWBAR_HOME`; never the production socket.
- **Both transports at once is the thing to prove.** Start a codex chat, run a turn, and
  assert that a `subagent_pre` (hooks) and a `message_delta` (API) both reach the same
  chat aggregate, in order, with no duplicate turn.
- **Answer a permission from the chat.** This is the headline capability and the one
  that has never worked for codex. Assert the CLI actually proceeds.
- **Hotswap under load:** switch chat → terminal mid-turn and confirm the TUI shows the
  same conversation, not a second one.
- **The negative case needs a fixture provider.** With no real `hotswap: false` provider,
  write a test descriptor that declares it and assert the switcher greys and split
  disappears — otherwise the capability is only ever exercised on its permissive value
  and the gate is untested. See [[project_vacuous_guard_tests]].
- Standing gates: `go build ./...` (with `-tags noEmbed` for a dev binary), `go vet`,
  `golangci-lint run`, the full web suite, and `bun tsc`.

---

## 6. Risks

- **The API's leaf paths are captured, not proven** (§1.3). Step 1 exists to convert one
  into the other; expect corrections.
- **Two wires, one aggregate** is where the real complexity is, not in the JSON-RPC
  client. Session identity and turn de-duplication are the parts to design carefully.
- **`attach` is a claim this spec has not tested.** It is declared in the descriptor and
  its comment is unambiguous, but no Crowbar code has ever run it. If attaching a TUI to
  a served thread does not work as declared, capability 2 for codex collapses to the
  third state (no terminal) and the split view goes with it. **Verify `attach` early** —
  it is the assumption the whole shape rests on, and it is cheap to test by hand before
  any of the engine work.
