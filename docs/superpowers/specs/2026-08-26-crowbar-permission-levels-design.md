# Crowbar Permission Levels — a generic auto-approve policy in front of the answer desk

**Date:** 2026-08-26
**Status:** design, pending approval

## 0. In plain terms

Right now, whenever Claude or Codex wants to do something that needs a nod — edit a file, run a
command, anything either CLI's own gating flags for approval — Crowbar always stops and waits for
a human to click Allow or Deny in the chat UI. There is no setting anywhere to change that. That's
the "manual mode for everything" complaint this spec answers.

The fix is **not** to hand Claude or Codex a more permissive native mode (`bypassPermissions`,
`--dangerously-bypass-approvals-and-sandbox`, etc.) — Crowbar deliberately avoids those today,
because they make the CLI stop telling Crowbar about the request at all: no chat log entry, no
audit trail, nothing to show the user afterward. That was a considered decision, and this design
keeps it.

Instead: give each chat a **Permission Level** — `guarded` (today's behavior), `trusted`, or
`full-auto` — and classify every provider tool call into a **Risk Tier** the descriptor already
knows how to compute from that provider's own tool vocabulary. When the chat's level clears the
tier, Crowbar answers "allow" itself, instantly, using the exact same rendering and ledger-write
path a human's click uses today — so the CLI still only ever hears from Crowbar, and the chat
transcript still shows exactly what happened, just without the click.

**The shipped default is `full-auto`**, set globally in Crowbar's own Settings and editable there.
This is called out deliberately: it means a brand-new chat asks the user nothing at all unless
they've gone into Settings and turned it down. That's a real, visible product decision — not an
incidental default — and it's recorded as one in §4.

One structural fix rides along: Crowbar's own background tool calls (the MCP tools it injects into
a session, not anything the user asked for) run inside a pane with nobody present to answer a
modal. Codex already special-cases this with a CLI flag; Claude has no equivalent today and may be
silently exposed to the same stall. This spec closes that gap generically, for both providers, as
part of the same mechanism — not as a session-level trust setting, but as an unconditional
exemption underneath it.

## 1. Current state and why it's stuck (verified, not assumed)

- Claude Code fires its `PreToolUse`/`PermissionRequest` hook on every gated tool call
  **regardless of `--permission-mode`** — the mode only changes how much the CLI itself decides
  needs asking. A hook may return an unconditional `allow` and Claude proceeds with no prompt,
  while remaining subject to Claude's own non-overridable safety net for `rm`/`rmdir` on critical
  paths, hook or no hook.
- `claude.yaml:184-186` is explicit about why Crowbar runs `--permission-mode auto`, not
  `bypassPermissions`/`dontAsk`: those skip the hook entirely, so Crowbar would lose visibility
  into what the agent did. `auto` was chosen specifically to keep the hook firing.
- Once the hook *does* fire, there is **no code path anywhere that auto-decides it**. Both
  providers' `permission`/`elicitation` canonical events funnel through the same generic engine
  plumbing: `Turns.handleObservation` → `openChoice` (`api/internal/app/usecases/chat/internal/turn/observation.go:52-58,67-99`)
  unconditionally calls `holdForAnswer` (line 98) whenever `ev.Choice != nil`, which parks the
  relay on `answerdesk.Desk.Hold` (`answerdesk.go:86`). The only way out is a human calling
  `AnswerChoice` (`api/internal/app/usecases/chat/answers.go:129-175`) or a timeout that releases
  the relay back to the CLI's own terminal prompt. Confirmed by reading every function in that
  chain — there is no auto-allow branch, no per-tool exception, nothing keyed on provider.
- Codex is wired identically (`item/permissions/requestApproval` → the same `permission` event →
  the same `openChoice`/`holdForAnswer`), so this is one fix for both providers, not two.
- **The Crowbar-internal-tools exemption exists for Codex only, and it's a CLI-config hack, not an
  engine feature.** `codex.yaml:461-471` sets `mcp_servers.crowbar.default_tools_approval_mode=
  "approve"` server-wide, with the comment recording *why*: "an agent that acts without a per-call
  modal is the point of this surface" — Crowbar's own tool calls happen inside a pane with no
  human present, so holding them for approval wouldn't make them safer, it would just stall them
  forever. Claude's descriptor (`claude.yaml:247-296`) registers `PermissionRequest`/`Elicitation`
  generically with no equivalent carve-out — if Claude ever calls one of Crowbar's own MCP tools
  autonomously, it likely hits the exact stall the Codex comment warns about.

## 2. Concepts (Crowbar-owned, provider-blind)

Two new enums, neither aware that "claude" or "codex" exist:

```go
type RiskTier string

const (
    RiskReadOnly  RiskTier = "read-only" // inspects, changes nothing
    RiskStandard  RiskTier = "standard"  // ordinary edits/commands inside the workspace
    RiskSensitive RiskTier = "sensitive" // destructive, external-facing, or unclassified
    RiskInternal  RiskTier = "internal"  // Crowbar's own injected tool calls — see §5
)

type PermissionLevel string

const (
    LevelGuarded  PermissionLevel = "guarded"   // today's behavior: nothing auto-approves
    LevelTrusted  PermissionLevel = "trusted"   // read-only + standard auto-approve
    LevelFullAuto PermissionLevel = "full-auto" // everything auto-approves
)
```

Policy is one small, pure function, independent of both provider and UI:

```go
func autoApprove(level PermissionLevel, risk RiskTier) bool {
    if risk == RiskInternal {
        return true // unconditional — not part of the level dial at all, see §5
    }
    switch level {
    case LevelFullAuto:
        return true
    case LevelTrusted:
        return risk == RiskReadOnly || risk == RiskStandard
    default: // guarded
        return false
    }
}
```

`guarded` never auto-approves anything (other than `internal`) — it is exactly today's behavior,
preserved on purpose so nothing changes for a chat that doesn't opt in.

An **unclassified tool name defaults to `sensitive`.** The descriptor's classification table is a
safe allowlist, not a denylist: a new tool version Crowbar hasn't been taught about is the most
conservative case, never the most permissive.

**Elicitation events are out of scope for this policy and always hold for a human**, regardless of
level. Unlike a tool-permission allow/deny, an elicitation's `accept` reply must carry actual
content matching a schema — there's no safe way for Crowbar to synthesize that automatically, and
the user's original complaint was about tool-call approval, not answering forms on the agent's
behalf. Flagged for your review in §8; this can be revisited later without touching the
tool-permission design at all.

**Implementation status (as shipped):** the `internal` carve-out described above for an
elicitation whose `mcp_server` is Crowbar's own was NOT built — `autoApproveIfPolicy`'s gate is
`Kind != ChoiceToolPermission`, which excludes every elicitation unconditionally, including one
from Crowbar's own server. This is currently latent, not a live bug: Crowbar's own injected MCP
server issues only plain tool calls today, never an elicitation, so the excluded case cannot
actually occur. Recorded here as a known, deliberately deferred gap rather than silently dropped —
if Crowbar's own server ever gains an elicitation-issuing tool, this carve-out needs to be built
before that tool can work at any level, including `full-auto`.

## 3. Descriptor changes — a classification table, not a mode switch

Each provider's `permission` event mapping gains a `risk` classification, keyed off the same
`tool_name` value it already extracts today. No changes to `spawn.args` — Claude keeps
`--permission-mode auto`, Codex keeps `--sandbox workspace-write --ask-for-approval on-request`,
exactly as today, so the hook keeps firing for everything regardless of level.

Illustrative shape (exact key names are an implementation-plan detail):

```yaml
# claude.yaml, inside the existing `permission:` event
permission:
  ask: PermissionRequest
  map: { ... unchanged ... }
  risk:
    read-only: [Read, Grep, Glob, NotebookRead]
    standard:  [Edit, Write, MultiEdit, NotebookEdit, Bash]
    internal:  ["mcp__crowbar__*"]     # Crowbar's own injected MCP server
    # anything else → sensitive (the default)
```

```yaml
# codex.yaml, inside the existing `permission:` event
permission:
  ask: item/permissions/requestApproval
  map: { tool_name: tool, tool_input: params }
  risk:
    standard: [shell, apply_patch]
    internal: ["mcp__crowbar__*"]
    # anything else → sensitive
```

This is evaluated the same place the existing `map:` fields are (`choice.go`'s `permissionChoice`),
producing a `Risk RiskTier` field on `models.ChoicePrompt` alongside `ToolName`/`Title`/`Options`.
Go's classifier code pattern-matches against **descriptor-declared strings**, never a hardcoded
`"Read"`/`"Bash"` literal — the tool vocabulary stays entirely inside the YAML, same as every other
field in this file.

## 4. Engine change — one insertion point, both providers

`openChoice` (`observation.go:67-99`) already does the one thing that matters: it opens the choice
in the activity ledger and then holds it. The auto-approve path doesn't bypass `holdForAnswer` —
it still needs the slot `Hold` creates — it immediately resolves it afterward, reusing exactly the
sequence `AnswerChoice` runs for a human's click: `answerdesk.Decide` → `agent.RenderAnswer` →
write the ledger entry → `answers.Resolve(slot, stdout)`. Concretely, right after `t.holdForAnswer`
in `openChoice`, a policy check compares the chat's current `PermissionLevel` against
`ev.Choice.Risk`; when `autoApprove` returns true, it calls the same resolution path with the
fixed "Allow" option (`domain.ChoiceOptionAllow` — the option ID `permissionChoice` already always
creates), tagging the ledger write so the transcript can show it was policy-approved, not
human-approved.

This is the property that survives from the original design intent: even at `full-auto`, every
decision is still visible in the chat afterward — Crowbar is answering faster, not going dark.

No new cross-package plumbing is needed to do this: `openChoice` already receives `agent
engineagents.Agent` as a parameter and already has `t.answers` (`*answerdesk.Desk`) and
`t.activity` in scope inside the `turn` package — the exact three things `AnswerChoice`
(`chat` package) uses for a human's click. The auto-resolve logic can be added directly in
`turn/observation.go` without reaching into the `chat.Usecase` at all. The one thing worth a
quick check at implementation time, not a redesign risk: confirming `t.activity` exposes the
same answer-recording method `u.activity.AnswerChoice(...)` calls, so the ledger write looks
identical whether a human or the policy made the call.

**Session state, in two layers.** The *default* level is a single global, persisted setting —
`full-auto` out of the box — editable from Crowbar's Settings, per project convention living in
`features/settings/` on the frontend with whatever small backend store already backs other global
settings. A chat's `PermissionLevel` is seeded from that default the moment the chat is created.

From there, the level is per-chat, in-memory state on `Turns` — the same shape as the existing
`telemetry` map (`t.telemetry.Set(chat.ID, report)` / `t.Telemetry(chatID)` in
`observation.go:229-235`) — no new durable store beyond the one global default value. A new
use-case method sets the level for a chat; the frontend calls it when the user flips the per-chat
switcher (§6), which starts at whatever the global default currently is and can be overridden for
that chat only, without touching the global setting.

## 5. The `internal` exemption is not part of the level dial

`RiskInternal` always auto-approves, at every level including `guarded` — it is a structural fix,
not a trust setting. The reasoning is unrelated to how much a user trusts the agent's work: nobody
is present in a Crowbar-driven pane to click a modal for Crowbar's own background tool calls, so
holding them for approval doesn't add safety, it just stalls them. This generic, engine-level
exemption covers Claude and Codex identically, closing the Claude-side gap in §1 for free and
giving Codex a real fix instead of the CLI-config workaround it has today (`codex.yaml:461-471`
can stay as defense-in-depth, or be removed once the engine exemption ships — noted in §8, not
required for this feature).

## 6. Frontend

Two surfaces:

- **Settings → Agents tab** (`web/src/features/settings/components/tabs/providers-settings.tsx`,
  the `ProvidersSettings` component — internally still `id: 'providers'` in
  `settings-tab-items.ts:32`, only the label was renamed to "Agents"). A new "Default permission
  level" `Select` (Guarded / Trusted / Full Auto), persisted globally, shipping with `Full Auto`
  selected out of the box. Follows the existing select-in-a-`SettingRow` pattern already used
  elsewhere in settings (e.g. `editor-settings.tsx:122-146`'s "Render Whitespace" control) rather
  than introducing a new control style — either as its own row inside the tab's existing
  `Section title="Agents"` (line 185), or as a small sibling component the way
  `ChatPresentationSetting` (`chat-presentation-setting.tsx`) is its own file for the tab's
  separate "Chat" section.
- **Per-chat switcher**: near the existing approve/deny UI, scoped to the active chat, initialized
  from the current global default and independently changeable for that chat only.

Exact component placement for the per-chat switcher (the settings-side placement is now settled
above) is left to the implementation plan.

## 7. Safety floor underneath all of this

- Claude Code's own non-overridable `rm`/`rmdir` critical-path circuit breaker still applies even
  under a policy-rendered `allow` — Crowbar cannot loosen it, hook or no hook.
- Codex's sandbox (`workspace-write`) still bounds what's technically reachable regardless of
  `PermissionLevel` — `full-auto` widens what Crowbar answers "yes" to, not what the sandbox
  permits.
- `full-auto` is a per-chat, explicit, reversible opt-in — never a default, never persisted past
  the chat's lifetime.

## 8. Decisions

1. **Elicitation stays always-held, out of scope for this policy** (§2). Confirmed — users still
   decide these themselves regardless of `PermissionLevel`, since answering one means
   synthesizing content, not just allow/deny.
2. **Default permission level is `full-auto`, set in Settings** (§4, §6). Confirmed, as a
   deliberate and visible default rather than an incidental one.

Still open, for review:

3. **Codex's existing `default_tools_approval_mode="approve"` CLI flag**: leave it in place as
   redundant defense-in-depth once the engine-level `internal` exemption ships, or remove it as
   dead weight. Either is safe; recommend leaving it for now and revisiting in a later cleanup.
4. **Unclassified tool names default to `sensitive`** (§2) — the conservative choice, and the more
   consequential one now that new chats start at `full-auto`: a misclassified or unrecognized tool
   still gets auto-approved at `full-auto` regardless of tier, since `full-auto` clears every tier.
   The `sensitive` default only matters at `trusted` or below. Confirm this failure mode is still
   what you want.

## 9. Testing plan

- **Policy unit test**: the full `(PermissionLevel × RiskTier)` matrix in §2, table-driven,
  confirming exactly which combinations `autoApprove` returns true for — including `RiskInternal`
  returning true under `guarded`.
- **Descriptor classification test**, per provider: known tool names from `risk:` map to the
  expected tier; an unlisted name defaults to `sensitive`.
- **Engine integration test**: a `permission` event at `trusted` with a `standard`-tier tool
  resolves without ever reaching `answerdesk.Desk.Hold`'s human-wait path — i.e., the relay gets
  its stdout back without a UI call to `AnswerChoice`. A `sensitive`-tier tool at `trusted` still
  holds and requires the existing human path, unchanged.
- **Regression test for the Claude-side gap**: a `permission` event whose `tool_name` matches
  Crowbar's own MCP-server pattern resolves automatically under Claude at `guarded` — this is the
  parity fix, and it needs its own named regression test per project convention, not just coverage
  incidental to the matrix test above.
- **Default-seeding test**: a freshly opened chat's `PermissionLevel` matches the current global
  Settings default (out of the box, `full-auto`) before any per-chat switcher interaction; changing
  the global default does not retroactively change an already-open chat's level.
- Live verification per project convention: after green tests, exercise `guarded` → `trusted` →
  `full-auto` in a real chat via `make dev-desktop` against both a Claude and a Codex session.
  Confirm the ledger records `AutoApproved: true`/`false` correctly for each decision (a final
  whole-branch review found the two answer paths originally shared one code path with no way to
  tell them apart at all; a fix wave added the `AutoApproved` field to the activity ledger and its
  API/DTO — visually surfacing that distinction in the transcript UI is a separate, deferred
  follow-up, so this check is at the ledger/API layer, not yet the rendered chat).
- **Codex-specific checks**, added after the final review found Codex's own risk classification
  has no passing test today (its one direct test predates this plan and is already failing for an
  unrelated, pre-existing reason): a Codex session at `trusted` with a real `shell` tool call
  auto-approves (proves `tool_name` extraction from Codex's real payload shape actually feeds the
  classifier, not just the hand-built test fixture); a Crowbar-own MCP tool call on Codex at
  `guarded` (the strictest level) still auto-approves (proves the `RiskInternal` parity fix
  genuinely fires from Codex's real payload, not only masked by the pre-existing
  `default_tools_approval_mode="approve"` CLI flag doing the work underneath it).

## 10. Out of scope / deferred

- **Per-rule "always allow this" learning** (mentioned during design discussion as a natural
  follow-on): grows an allowlist from individual human clicks instead of a blanket level.
  Not built here; the same `openChoice` insertion point this spec adds is what it would plug into.
- **Workspace-level persistence of a per-chat override**: rejected — only the global default
  (§4) persists; a chat's own override is in-memory and dies with the chat.
- **Elicitation auto-answering**: decided against, see §8.1.
- **Frontend surfacing of `AutoApproved`**: the activity ledger and its API/DTO now carry a real
  `AutoApproved` fact per resolved choice (added in the final-review fix wave — see §9), but no
  chat-transcript UI reads or displays it yet. The backend truth existing was the substantive part
  of the original ask; deciding where and how to show it visually is a scoped follow-up.
- **Per-chat switcher reachability at `full-auto`**: the switcher (§6) only renders next to a
  *pending* permission prompt, but at the shipped `full-auto` default, tool-permission prompts
  auto-resolve server-side before the client ever sees them — so the dial is effectively invisible
  under the very default the product ships with, reachable only for the elicitation/question
  prompts it doesn't govern. This is a real product placement question (a persistent, always-visible
  home for the control — e.g. a chat header or chat settings surface — versus the current
  transient, prompt-adjacent one), not a code defect, and is left for a follow-up design decision
  rather than resolved here.
