# The agent chat surface — decomposition and redesign

**Date:** 2026-08-23

**Status:** Design spec. Companion to
[`2026-08-23-agents-chat-rearchitecture-design.md`](./2026-08-23-agents-chat-rearchitecture-design.md),
which restructures the backend. That spec declares the frontend out of scope except for
one route rename; this one is that frontend, and the two are designed to merge.

**Scope:** `web/src/features/agent/` — 9,024 lines across 32 files. The chat surface is
redecomposed and reskinned to the design canvas; the sidebar chats tree is relocated
unchanged.

**Not in scope:** the API client (`api/agent-api.ts`) — it is the wire and it does not
move. The backend. The terminal pane's own internals.

**The design canvas** is the visual authority: ten artboards covering empty, conversation,
working, slash, permission, question, halted, compacting, and both themes.

---

## 1. What is wrong today

| file | LOC | what it is |
|---|---|---|
| `components/agent-chat-view.tsx` | 1,430 | four concerns interleaved |
| `components/agent-chat-pane.tsx` | 1,264 | five more |
| `components/agent-chats-panel.tsx` | 705 | the sidebar tree |
| `components/agent-choice-prompt.tsx` | 644 | questions and permissions |
| `components/use-agent-chats-drag.ts` | 532 | a hook, in `components/` |

### 1.1 The folder has nowhere to put a hook

`features/agent/` has three directories: `api/`, `components/`, `lib/`. Every comparable
sibling has more — `git/` has six, `terminal/` has seven, `editor/` has twenty. With no
`hooks/`, four hooks were filed under `components/`, one of them 532 lines. The folder
shape caused the file shape.

`editor/` is the precedent for the fix: it is the largest feature in the app and it is
sliced by domain, not by file kind.

### 1.2 Two unrelated surfaces share one folder

| surface | LOC | what it is |
|---|---|---|
| the chats tree | 3,499 | a sidebar panel: rows, folders, drag, search, removal |
| the chat surface | 4,610 | a pane: transcript, composer, activity, controls |
| the wire | 915 | `agent-api.ts`, used by both |

They share the API client and nothing else. No component of one imports a component of
the other.

### 1.3 The view is four concerns, not one

Measured by its own state:

| concern | state it owns |
|---|---|
| prompt queue | `queue`, `persistenceLost`, `idleEpoch`, `updateQueue`, `reconcileQueue`, `releaseBusyBarrier`, `dispatch`, `mark` |
| message ledger | `messages`, `hasOlder`, `messagesLoading`, `messagesError`, `applyMessages`, `loadInitialMessages`, `refreshMessages` |
| slash catalogue | `catalogState`, `slashOpen`, `slashSelected` |
| composer | `draft`, `composerError`, `submitUnavailable` |

Each is a hook wearing a component's clothes. None can be tested without mounting the
other three.

### 1.4 The pane is five more

`attachedState` (PTY adoption), `chosenPresentation` + `splitFocus` (which surface is
shown), `splitSizes` + `splitStacked` + `gridSlack` (split geometry), the four prompt
counters, and `revive`/`adopt`/`handleSessionGone` (lifecycle).

### 1.5 The blocked states stack instead of replacing

`AgentChoicePrompts` renders *above* the composer, so a chat waiting on a permission
shows a dead input box under a live question. The design's rule is the opposite, and it
is the one structural change in this spec.

---

## 2. Principles

1. **Slice by domain, not by file kind.** `composer/`, `transcript/`, `activity/`,
   `controls/` — each holding its own components, and its own `lib/` for the pure parts.
2. **A component renders; a hook holds state.** If a component owns a `useEffect` that
   fetches, the fetch belongs in `hooks/`.
3. **The design's numbers exist once.** Values are lifted from the canvas stylesheet into
   `styles/`, not retyped as utility classes. See §6.
4. **Capability is presence.** A control the provider does not offer is absent, never
   disabled. Carried verbatim from `agent-model-picker.tsx`.

---

## 3. The composer is a state machine

> It is an input when you can talk, and it is the question, the permission, or the reason
> you cannot, when you cannot.

One 38px slot. Exactly one occupant. Never two stacked, and never an input box rendered
dead beneath something else. Resolved by a pure function, `composer/lib/composer-state.ts`:

| precedence | state | occupant | source |
|---|---|---|---|
| 1 | `terminal_wait` | signpost → Terminal | `AgentChat.terminalWait` |
| 2 | `unanswerable` | signpost → Terminal | `AgentChoice.answerable === false` |
| 3 | `choice` | the question, in the bar | `pendingChoices()` |
| 4 | `halted` | the provider's stop reason | `notice` message, `AgentRateLimit.resetsAt` |
| 5 | `compacting` | busy notice, prompts queue | the chat's `compacting` work-state |
| 6 | `input` | the pill | otherwise |

Precedence is the whole contract: a chat blocked on a trust dialog *and* holding a
pending choice shows the trust dialog, because that is the one a person can act on.

States 1 and 2 are the same occupant from two different causes, and they are kept
separate deliberately: they are different sentences to say, and conflating them would
tell a codex user their permission is unanswerable when the real problem is an
unaccepted workspace.

---

## 4. What the API actually backs

Audited field by field against `descriptor/internal/schema/vocabulary.yaml`, the three
shipped descriptors, and the TS wire shapes.

### 4.1 Backed today

| designed element | source |
|---|---|
| user / assistant messages, per-turn effort | `AgentChatMessage` |
| tool rows, durations | `AgentToolCall.name` / `.target` / `.durationMs` |
| subagent shelf — count, type, elapsed | `AgentSubagent` |
| permission bar | `permission` ask |
| multiple choice | `AgentChoiceQuestion` |
| model / effort pickers, and their absence | `modelSelect` / `effortSelect` / `models` / `efforts` |
| slash picker | `SlashCatalog` |
| context gauge | `AgentContextUsage` |
| rate-limit chips, "resets at 19:00" | `AgentRateLimit.resetsAt` |
| halted notice | `turn_failed` (claude), `terminal_notices` (codex) |
| streaming reply | `message_delta` |
| terminal signpost | `AgentTerminalWait` |
| compacting state | the chat's `compacting` work-state, beside `working` |
| compaction divider | the `compact_post` ledger marker, carrying `manual` \| `auto` |
| compact control | `compact_start` key presence |
| spinner verb | **frontend-owned** — needs only `working` |
| queue, empty state, pane chrome | client-side |

### 4.2 Provider skew

The design draws claude-on-hooks. It is not what every provider reports.

| element | claude | codex (pty) | codex-api |
|---|---|---|---|
| streaming reply | ✓ | **✗** | ✓ |
| context gauge | ✓ | **✗** | ✓ |
| rate-limit chips | ✓ | **✗** | ✓ |
| subagent shelf | ✓ | ✓ | **✗** |
| halted notice | ✓ | ✓ | **✗** |
| answerable permission | ✓ | **✗** | ✓ |
| multiple choice | ✓ | **✗** | **✗** |
| compact control | ✓ | **✗** | ✓ |

Every absence must read as *this provider does not report that*, never as *Crowbar is
broken*. Absence is already the design's idiom; §2.4 makes it the rule.

**codex's permission is `answerable: false`** — *"a decision made in Crowbar reached
nobody — the human answers in the terminal."* The design draws allow/deny
unconditionally, which is a lie a user can click. Resolved by state 2 in §3.

### 4.3 Not backed, and what happens

| element | status |
|---|---|
| per-turn token counter | **removed from the design.** Not on `AgentTelemetry`; where it exists it is cumulative |
| switcher "cannot hand over mid-turn" | no descriptor key. Backend work in flight; until then the state does not render |
| slash catalogue completeness | `SlashCatalog.completeness` is `plugin_only` on claude — structurally partial, and the design says nothing. See §8 |

---

## 5. The tree

```
features/agent/
  api/agent-api.ts                     unchanged
  types/agent-chat.ts                  shared view types

  hooks/
    use-chat-messages.ts               paging, streaming, refresh
    use-prompt-queue.ts                queue, persistence, barrier, reconcile
    use-slash-catalog.ts               fetch, open, selection
    use-agent-activity.ts              moved from components/
    use-agent-telemetry.ts             context + rate limits
    use-chat-presentation.ts           which surface, split geometry

  composer/
    agent-composer.tsx                 the shell — resolves and renders one state
    composer-field.tsx                 growth, handle placement
    composer-handle.tsx                send ↔ stop
    composer-halted.tsx                stop notice
    composer-choice.tsx                question / permission in the bar
    composer-signpost.tsx              terminal_wait and unanswerable
    composer-slash-picker.tsx
    lib/composer-state.ts              §3, pure
    lib/handle-geometry.ts             (36 − d) / 2, pure

  transcript/
    agent-transcript.tsx
    message-bubble.tsx                 user, rounded to the pill's radius
    message-prose.tsx                  assistant markdown
    queued-row.tsx
    turn-tools.tsx
    compaction-divider.tsx             the compact_post boundary

  activity/
    working-line.tsx                   verb + elapsed
    subagent-shelf.tsx
    lib/verbs.ts                       ours
    lib/shelf-fit.ts                   shedding order, pure

  controls/
    provider-bar.tsx                   the footer cluster
    model-picker.tsx                   moved
    effort-picker.tsx
    view-switcher.tsx                  restyled from pane:1225
    context-gauge.tsx                  moved

  chat/
    agent-chat-view.tsx                assembly — target ≤ 200 LOC
    agent-chat-pane.tsx                pane chrome, surface switch — target ≤ 400 LOC
    agent-empty-document.tsx           Plate markdown empty state

  styles/
    composer.css  transcript.css  activity.css      §6

  tree/                                relocated unchanged
    agent-chats-panel.tsx  agent-chat-row.tsx  agent-chat-folder-row.tsx
    agent-chats-search.tsx  agent-chat-context-menu.tsx  agent-chat-glyph.tsx
    hooks/  lib/
```

Tests mirror into `web/src/__tests__/features/agent/` per CLAUDE.md.

---

## 6. Parity, and how it is proven

"Matches the design" is not a judgement call. Three mechanisms, each blind where the next
one sees.

**1. The stylesheet is the design's, lifted.** The canvas already styles everything in
Crowbar's own tokens — `var(--input)`, `var(--primary)`, `var(--radius-sm)`,
`var(--ui-xs)`. Those rules move into `features/agent/styles/` with their class names
and their comments intact. Parity then is a diff of two stylesheets, not an opinion, and
a value cannot drift by being retyped as a utility class that rounds 18px to `rounded-2xl`.

This is a deliberate exception to reaching for Tailwind utilities first, and it is
narrow: the composer, transcript and activity surfaces only. Everything else uses
`@/components/ui/*` as usual — the view switcher is `ui/tabs` in its pill variant, the
menus are `ui/dropdown-menu`.

**2. The pure parts are unit-tested.** `composer-state.ts` (every row of §3's table, and
its precedence), `handle-geometry.ts`, `shelf-fit.ts` (the shedding order at 3, 5 and 9
subagents). These are the invariants a restyle can silently break.

**3. Each artboard is reproduced live and compared.** Not headless — `make dev-desktop`,
driven over the Tauri bridge, because vibrancy and the real font stack change what every
translucent surface looks like. Ten states, each captured at rest and against its
artboard:

| # | artboard | how the state is reached |
|---|---|---|
| 1 | Empty (dark / light) | new chat, no messages |
| 2 | Conversation (dark / light) | two turns, tools rendered |
| 3 | Working | prompt in flight, subagents fanned out |
| 4 | Slash | `/` typed |
| 5 | Permission | a gated tool call |
| 6 | Question | `AskUserQuestion` |
| 7 | Halted | rate limit reached |
| 8 | Compacting | a compaction, manual or auto |

Animation is in scope: the handle's slide, the pill's radius transition, the subagent
pulse. A state change that snaps is a defect, and a resting screenshot cannot see it —
each transition is watched, not measured.

---

## 7. Merging with the backend work

**Compaction is assumed present.** The backend spec's §4.5 is landing alongside this
work — `compact.go` and `translate/outbound/` are already in the tree — so `compacting`
is a first-class state here, not a flagged one: `composer-state.ts` resolves it, the
divider renders the `compact_post` marker, and the compact control appears wherever
`compact_start` is declared. It is built against the shape §4.5 specifies; if that shape
moves, this moves with it.

Two designed elements still depend on work in flight, and each ships dark rather than
absent so the merge is a wiring change and not a second frontend pass:

- **The switcher's third state** — `view-switcher.tsx` accepts a `handover` prop it never
  currently receives. When a descriptor key declares it, the prop is wired.
- **`split`** — `ChatPresentation` has three members, the design has two icons. Split is a
  development diagnostic, so it is a third segment present only in dev builds, and it is
  the one place the tab group is not two-up. This needs a decision (§8).

---

## 8. Open questions

1. **Split in the switcher.** Third segment in dev builds, or left on the existing
   keybinding and out of the tab group entirely? The design shows two icons and split
   would make it three, asymmetrically, in one build flavour.
2. **Slash completeness.** `plugin_only` is claude's permanent state — the user's own
   skills are not machine-readable. Saying nothing means a user sees a list missing their
   skills with no explanation. One dimmed word, or continued silence?
3. **The chats tree.** Relocated unchanged here. It has its own 705- and 532-line files
   and wants the same treatment, but that is a separate spec.
