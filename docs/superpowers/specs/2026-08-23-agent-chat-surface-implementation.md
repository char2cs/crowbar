# The agent chat surface — implementation plan

**Date:** 2026-08-23

**Implements:** [`2026-08-23-agent-chat-surface-design.md`](./2026-08-23-agent-chat-surface-design.md)

**Shape:** eleven stages. Stages 0–2 are behaviour-preserving and land first, because they
pay for themselves whether or not the redesign follows. Stages 3–9 are the redesign, each
one a mergeable slice. Stage 10 is the parity gate.

**The safety net:** 37 tests under `web/src/__tests__/features/agent/`, including
`agent-chat-view.test.tsx`, `agent-chat-pane.test.tsx` and `agent-choice-prompt.test.tsx`.
For every behaviour-preserving stage the gate is the same and it is strict: **those tests
pass with no edit to any assertion.** Import paths may move. Expectations may not.

**Per-stage gate, unless stated otherwise:**

```
cd web && bun tsc --noEmit && bun run test && bun run lint
```

`bun`, not `bunx` — `bunx tsc` resolves a different package.

---

## Stage 0 — relocate the chats tree

Pure move, no edits inside the files.

| from `components/` | to |
|---|---|
| `agent-chats-panel.tsx`, `agent-chat-row.tsx`, `agent-chat-folder-row.tsx`, `agent-chats-search.tsx`, `agent-chat-context-menu.tsx`, `agent-chat-glyph.tsx` | `tree/` |
| `use-agent-chats-drag.ts`, `use-agent-chat-folders.ts`, `use-agent-chat-list-virtualizer.ts` | `tree/hooks/` |
| `lib/chat-drop.ts`, `chat-rows.ts`, `chat-search.ts`, `chat-tree-commit.ts`, `chat-removal.ts`, `shown-chats.ts`, `chat-menu-model.ts`, `chat-label.ts` | `tree/lib/` |

Tests move to mirror. External importers to update: `components/layout/sidebar-carousel.tsx`,
`components/layout/removal-commit.ts`, `features/workspace/stores/slices/agent-chats-slice.ts`,
and the four `features/panes` / `features/tabs` call sites.

**Gate:** the 37 tests, assertions untouched.

---

## Stage 1 — the view's four hooks

`agent-chat-view.tsx`: **1,430 → ~300**. No JSX changes beyond replacing local state with
hook returns.

| new hook | absorbs |
|---|---|
| `hooks/use-prompt-queue.ts` | `queue`, `persistenceLost`, `idleEpoch`, `updateQueue`, `reconcileQueue`, `releaseBusyBarrier`, `dispatch`, `mark`, `showAwaitingTerminalHint` |
| `hooks/use-chat-messages.ts` | `messages`, `hasOlder`, `messagesLoading`, `messagesError`, `applyMessages`, `loadInitialMessages`, `refreshMessages`, `loadOlder`, the streaming bubble |
| `hooks/use-slash-catalog.ts` | `catalogState`, `slashOpen`, `slashSelected`, `slashItems`, `selectSlashItem` |
| `hooks/use-agent-activity.ts` | moved from `components/`, unchanged |

`MessageRow`, `QueueRow` and `SlashPicker` move out of the file to their §5 homes as-is —
renamed, not rewritten. That is stages 8 and 4's starting point, not new work here.

The ordering invariant the queue hook must preserve: the busy barrier releases *before*
the queue reconciles, or a prompt dispatches twice. It is currently implicit in statement
order inside one component; extracting it makes it explicit, and it gets its own test.

**Gate:** the 37 tests, assertions untouched. Plus a new
`__tests__/features/agent/hooks/use-prompt-queue.test.ts` pinning the barrier ordering.

---

## Stage 2 — the pane's hooks

`agent-chat-pane.tsx`: **1,264 → ~400**.

| new hook | absorbs |
|---|---|
| `hooks/use-chat-presentation.ts` | `chosenPresentation`, `splitFocus`, `splitSizes`, `splitStacked`, `gridSlack` |
| `hooks/use-runner-attachment.ts` | `attachedState`, `seededFor`, `adopt`, `revive`, `fail`, `handleSessionGone` |

The four prompt counters stay on the pane — they are wiring between two children, not
state of their own.

**Gate:** the 37 tests, assertions untouched.

---

## Stage 3 — the design's stylesheet

Lift `composer.css`, `transcript.css` and `activity.css` from the canvas sources into
`features/agent/styles/`, class names and comments intact, imported by the components that
use them. Nothing renders differently yet — no component references them until stage 4.

Two rules that survive the move because losing them breaks the design:

- the send button's geometry is one number — `(pill height − diameter) / 2 = inset`, so
  28px inside a 36px box insets 4px on every side;
- `.pill` is a stadium at one line and `18px` past it, and the transition between them is
  `.18s ease` — not a snap.

**Gate:** typecheck and lint. `bun run build` to confirm the stylesheets are bundled.

---

## Stage 4 — the composer

| file | what it is |
|---|---|
| `composer/lib/composer-state.ts` | §3's resolver — pure, no React |
| `composer/lib/handle-geometry.ts` | handle offset from field height |
| `composer/composer-field.tsx` | the growing field, `multi` past one line |
| `composer/composer-handle.tsx` | send ↔ stop; stop when empty and working |
| `composer/composer-slash-picker.tsx` | from stage 1 |
| `composer/agent-composer.tsx` | the shell: resolve, render one occupant |

Wired into the view in place of the `Textarea` block. Only states `input` and `halted` are
reachable this stage; the rest resolve and render nothing until stage 5.

**Gate:** unit tests for both `lib/` files — every row of the §3 table plus precedence,
and the geometry at 1, 2 and 4 lines. The 37 stay green.

---

## Stage 5 — blocked states move into the bar

**The one structural change.** `AgentChoicePrompts` stops rendering above the composer.

| new file | state |
|---|---|
| `composer/composer-choice.tsx` | `choice` — question and permission |
| `composer/composer-signpost.tsx` | `terminal_wait` and `unanswerable` |

`agent-choice-prompt.tsx` (644 lines) is the source: its answer submission, option
handling and multi-select logic are reused; its stacked layout is not.

This is where `answerable: false` gets handled. A codex permission resolves to
`unanswerable` and draws a signpost, never allow/deny.

**Gate:** `agent-choice-prompt.test.tsx` assertions rewritten — this stage changes
behaviour on purpose, and it is the only stage that may touch them. New: a test that
an unanswerable choice renders no allow/deny control, and one that precedence puts a
trust dialog ahead of a pending choice.

---

## Stage 6 — the controls

| file | what it is |
|---|---|
| `controls/view-switcher.tsx` | `ui/tabs` pill variant; chat bubble + terminal |
| `controls/model-picker.tsx` | moved |
| `controls/effort-picker.tsx` | split out of the picker |
| `controls/context-gauge.tsx` | moved |
| `controls/provider-bar.tsx` | the cluster: controls left, gauge right, aligned to the pill |

The switcher replaces the two `aria-pressed` buttons at `agent-chat-pane.tsx:1225`. It
takes a `handover` prop it is not yet given, and — pending §8.1 — a third `split` segment
in dev builds only.

**Gate:** a test that a provider declaring no catalogue renders no picker at all, not a
disabled one.

---

## Stage 7 — activity

| file | what it is |
|---|---|
| `activity/lib/verbs.ts` | the rotation, ours |
| `activity/lib/shelf-fit.ts` | shedding order — names at 5, tokens into `+N` when full |
| `activity/working-line.tsx` | verb + elapsed + tool rows |
| `activity/subagent-shelf.tsx` | the shelf, above the pill |

`agent-activity-strip.tsx` is replaced by these two.

**Gate:** `shelf-fit` unit-tested at 3, 5 and 9 subagents; the count and the clocks never
shed, because they are all `AgentSubagent` carries.

---

## Stage 8 — transcript

`transcript/agent-transcript.tsx`, `message-bubble.tsx`, `message-prose.tsx`,
`queued-row.tsx`, `turn-tools.tsx`, `compaction-divider.tsx`.

User bubbles take the pill's radius. Assistant text uses `MARKDOWN_PROSE_CLASS` — the same
prose styling as a rendered markdown file, which is the point of the empty state.

`compaction-divider.tsx` renders the `compact_post` marker and its `manual` | `auto`
trigger. Everything above the boundary is out of the model's context and the transcript
still holds it — the divider is the only thing that says so.

**Gate:** the divider renders from a marker fixture shaped as backend §4.5 specifies, and
both triggers are covered.

---

## Stage 9 — the empty state

`chat/agent-empty-document.tsx` — a blank Plate markdown document, with the handle riding
the line below the caret and the inverted control bar.

Round-tripping is not byte-stable, so the composer reads the document as markdown source,
not as a serialised Plate value.

**Gate:** typing `# ` produces a heading and consumes the marker; the handle sits on the
line below the caret at every caret position; the control bar's height matches the pill's.

---

## Stage 10 — parity

Not headless. `make dev-desktop`, driven over the Tauri bridge, one state at a time.

| # | state | reached by |
|---|---|---|
| 1 | empty, dark + light | new chat |
| 2 | conversation, dark + light | two turns with tool calls |
| 3 | working | prompt in flight, subagents fanned out |
| 4 | slash | `/` typed |
| 5 | permission | a gated tool call |
| 6 | question | `AskUserQuestion` |
| 7 | halted | rate limit reached |
| 8 | compacting | a compaction, manual or auto |

Each captured at rest against its artboard. Then the transitions, which a resting capture
cannot see and which are half the design: the handle sliding to the last line, the pill's
radius easing from stadium to 18px, the subagent pulse, the send-to-stop swap.

**Gate:** `make pr-checks`, plus every state above reproduced and compared. A state that
cannot be reached live is reported as unverified — not assumed.

---

## Sequencing and merge

Stages 0–2 are safe to land immediately: they are behaviour-preserving, they are gated by
tests written against the current design, and they do not touch anything the backend
re-architecture is moving.

Stages 3–9 depend only on `agent-api.ts`, which the backend spec leaves alone. The one
coupling is the route rename in that spec's stage 8 — it breaks every agent URL the web
client calls, and it lands in `api/agent-api.ts` alone, which this plan never edits.

Compaction is assumed present — `compact.go` and `translate/outbound/` are already in the
tree — so the `compacting` state, the divider and the compact control are built live
against backend §4.5, not behind a flag. The switcher's `handover` prop is the one thing
that ships dark.

**Blocked on a decision before stage 6:** whether `split` is a third switcher segment in
dev builds. Design spec §8.1.
