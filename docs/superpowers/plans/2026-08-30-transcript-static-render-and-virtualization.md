# Transcript Static Render & Virtualization — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Chat transcript performance stays flat regardless of loaded turn count (thousands+), with Plate remaining the sole rendering engine everywhere.

**Architecture:** Settled messages render through `PlateStatic` (from `platejs/static`) fed by the *same* `chatComposerPlugins` array the interactive editor uses (derived via a key-swap map, not a hand-duplicated list) — verified live to produce byte-identical output for 8/10 node families, with two small exceptions handled in Task 1. Only the actively-streaming message keeps the full interactive editor. DOM mounting is further capped with TanStack Virtual (already a dependency, already used for `git-history-list.tsx` and `changed-files-tree.tsx`). `use-transcript-anchor.ts`/`follow-scroll.ts` need no changes — confirmed their scroll math is virtualizer-agnostic.

**Tech Stack:** React 19, Platejs 53, `@tanstack/react-virtual@^3.14.9`, Vitest + Testing Library.

**Spec:** `docs/superpowers/specs/2026-08-30-chat-scale-and-glass-perf-design.md`

## Global Constraints

- Plate stays the only markdown/rendering engine. No `react-markdown`, no hand-duplicated node components — see Task 1/2 for the derivation pattern that keeps the static plugin set structurally tied to the interactive one.
- `mergeMessages` keeps turnId-keying (`use-chat-messages.ts:27-35`) — do not touch; a test pins this invariant.
- Web tests mirror `web/src/` under `web/src/__tests__/`, use `@/` imports, kebab-case component files (project CLAUDE.md).
- Web package manager: `~/.bun/bin/bun` (not on default PATH). Never `bunx tsc`; use `~/.bun/bin/bun tsc --noEmit`. Run tests with `~/.bun/bin/bun vitest run <path>`.
- Transcript rows are confirmed read-only/presentational today (no `contentEditable`, no onChange in `message-row.tsx`) — the static swap assumes this holds. If a future change makes rows editable in place, this design needs revisiting.
- Leave changes uncommitted in the working tree per task; the coordinating session reviews and commits.

---

### Task 1: Static-safe plugin variants for Link and Callout

**Files:**
- Modify: `web/src/components/editor/plugins/link-kit.tsx`
- Create: `web/src/components/ui/callout-content.tsx`
- Modify: `web/src/components/ui/callout-node.tsx`
- Create: `web/src/components/ui/callout-node-static.tsx`
- Create: `web/src/components/editor/plugins/callout-kit-static.ts`
- Test: `web/src/__tests__/components/editor/plugins/callout-kit-static.test.tsx`

**Interfaces:**
- Produces: `LinkKitStatic: SlatePlugin[]` (named export from `link-kit.tsx`), `CalloutKitStatic: SlatePlugin[]` (named export from `callout-kit-static.ts`).
- Consumed by: Task 2 (`chatComposerPluginsStatic`).

**Why:** Verified live — every node type in `chatComposerPlugins` renders byte-identical under `PlateStatic` except two plugin-level exceptions, neither a content difference: `LinkFloatingToolbar` calls `useEditorRef()` unconditionally (only valid in an interactive editor), and `CalloutElement`'s emoji-picker button needs Plate context. Both fixes reuse the existing rendering logic — nothing about what a link or callout *looks like* is duplicated.

- [ ] **Step 1: Add `LinkKitStatic` to `link-kit.tsx`**

Add alongside the existing `LinkKit` export (keep `LinkKit` exactly as-is):

```tsx
export const LinkKitStatic = [
  LinkPlugin.configure({
    inputRules: [
      LinkRules.markdown(),
      LinkRules.autolink({ variant: 'paste' }),
      LinkRules.autolink({ variant: 'space' }),
      LinkRules.autolink({ variant: 'break' }),
    ],
    render: { node: LinkElement },
  }),
]
```

Same `LinkElement`, same `LinkRules` as `LinkKit` — only `render.afterEditable` (the toolbar) is dropped.

- [ ] **Step 2: Extract shared callout content into `callout-content.tsx`**

```tsx
import * as React from 'react'

import { cn } from '@/lib/utils'

export function calloutClassName(className?: string) {
  return cn('my-1 flex rounded-sm bg-muted p-4 pl-3', className)
}

const EMOJI_FONT_FAMILY =
  '"Apple Color Emoji", "Segoe UI Emoji", NotoColorEmoji, "Noto Color Emoji", "Segoe UI Symbol", "Android Emoji", EmojiSymbols'

export function CalloutBody({
  icon,
  iconSlot,
  children,
}: {
  icon?: string
  iconSlot?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <div className="flex w-full gap-2 rounded-md">
      {iconSlot ?? (
        <span
          className="flex size-6 select-none items-center justify-center p-1 text-[18px]"
          style={{ fontFamily: EMOJI_FONT_FAMILY }}
        >
          {icon || '💡'}
        </span>
      )}
      <div className="w-full">{children}</div>
    </div>
  )
}

export { EMOJI_FONT_FAMILY }
```

- [ ] **Step 3: Refactor `callout-node.tsx` to use the shared content**

Replace the body of `CalloutElement` so its JSX comes from `CalloutBody`, passing the interactive emoji-picker button as `iconSlot`:

```tsx
'use client'

import * as React from 'react'

import { useCalloutEmojiPicker } from '@platejs/callout/react'
import { useEmojiDropdownMenuState } from '@platejs/emoji/react'
import { PlateElement } from 'platejs/react'

import { Button } from '@/components/ui/button'

import { CalloutBody, EMOJI_FONT_FAMILY, calloutClassName } from './callout-content'
import { EmojiPicker, EmojiPopover } from './emoji-toolbar-button'

export function CalloutElement({
  attributes,
  children,
  className,
  ...props
}: React.ComponentProps<typeof PlateElement>) {
  const { emojiPickerState, isOpen, setIsOpen } = useEmojiDropdownMenuState({
    closeOnSelect: true,
  })

  const { emojiToolbarDropdownProps, props: calloutProps } = useCalloutEmojiPicker({
    isOpen,
    setIsOpen,
  })

  const icon = props.element.icon as string | undefined

  return (
    <PlateElement
      className={calloutClassName(className)}
      style={{
        backgroundColor: props.element.backgroundColor as React.CSSProperties['backgroundColor'],
      }}
      attributes={{
        ...attributes,
        'data-plate-open-context-menu': true,
      }}
      {...props}
    >
      <CalloutBody
        icon={icon}
        iconSlot={
          <EmojiPopover
            {...emojiToolbarDropdownProps}
            control={
              <Button
                variant="ghost"
                className="size-6 select-none p-1 text-[18px] hover:bg-muted-foreground/15"
                style={{ fontFamily: EMOJI_FONT_FAMILY }}
                contentEditable={false}
              >
                {icon || '💡'}
              </Button>
            }
          >
            <EmojiPicker {...emojiPickerState} {...calloutProps} />
          </EmojiPopover>
        }
      >
        {children}
      </CalloutBody>
    </PlateElement>
  )
}
```

- [ ] **Step 4: Create `callout-node-static.tsx`**

```tsx
import * as React from 'react'

import { SlateElement } from 'platejs/static'

import { CalloutBody, calloutClassName } from './callout-content'

export function CalloutElementStatic({
  attributes,
  children,
  className,
  ...props
}: React.ComponentProps<typeof SlateElement>) {
  return (
    <SlateElement
      className={calloutClassName(className)}
      style={{
        backgroundColor: props.element.backgroundColor as React.CSSProperties['backgroundColor'],
      }}
      attributes={attributes}
      {...props}
    >
      <CalloutBody icon={props.element.icon as string | undefined}>{children}</CalloutBody>
    </SlateElement>
  )
}
```

- [ ] **Step 5: Create `callout-kit-static.ts`**

```ts
'use client'

import { CalloutPlugin } from '@platejs/callout/react'

import { CalloutElementStatic } from '@/components/ui/callout-node-static'

export const CalloutKitStatic = [CalloutPlugin.withComponent(CalloutElementStatic)]
```

- [ ] **Step 6: Write the parity test**

```tsx
import { render } from '@testing-library/react'
import { createStaticEditor, PlateStatic } from 'platejs/static'
import { Plate, PlateContent, usePlateEditor } from 'platejs/react'
import { describe, expect, it } from 'vitest'
import type { Value } from 'platejs'

import { CalloutKit } from '@/components/editor/plugins/callout-kit'
import { CalloutKitStatic } from '@/components/editor/plugins/callout-kit-static'
import { LinkKit, LinkKitStatic } from '@/components/editor/plugins/link-kit'

function CalloutFixture(): Value {
  return [
    { type: 'callout', icon: '⚠️', children: [{ text: 'Careful with this one.' }] },
  ]
}

describe('CalloutKitStatic', () => {
  it('renders the same visible content as the interactive callout', () => {
    const value = CalloutFixture()

    const interactive = render(
      <InteractiveHarness plugins={CalloutKit} value={value} />,
    )
    const staticEditor = createStaticEditor({ plugins: CalloutKitStatic, value })
    const staticRender = render(<PlateStatic editor={staticEditor} />)

    expect(staticRender.container.textContent).toBe(interactive.container.textContent)
    expect(staticRender.container.querySelector('[data-slate-node]')?.textContent).toContain(
      'Careful with this one.',
    )
  })
})

function InteractiveHarness({ plugins, value }: { plugins: typeof CalloutKit; value: Value }) {
  const editor = usePlateEditor({ plugins, value }, [])
  return (
    <Plate editor={editor} readOnly>
      <PlateContent readOnly tabIndex={-1} />
    </Plate>
  )
}
```

Adjust the harness to match whatever setup `markdown-message.test.tsx` already uses for jsdom/Slate polyfills — read that file first and reuse its pattern rather than reinventing it.

- [ ] **Step 7: Run it**

```
~/.bun/bin/bun vitest run web/src/__tests__/components/editor/plugins/callout-kit-static.test.tsx
```

Expected: PASS. If the interactive and static text content differ, fix `CalloutBody`/`calloutClassName` until they match — do not special-case the test.

- [ ] **Step 8: Type-check**

```
~/.bun/bin/bun tsc --noEmit
```

---

### Task 2: `chatComposerPluginsStatic`, derived not duplicated

**Files:**
- Modify: `web/src/features/agent/composer/plate/chat-composer-plugins.ts`
- Test: `web/src/__tests__/features/agent/composer/plate/chat-composer-plugins-static.test.ts`

**Interfaces:**
- Consumes: `LinkKitStatic`, `CalloutKitStatic` (Task 1).
- Produces: `chatComposerPluginsStatic: SlatePlugin[]` (named export).
- Consumed by: Task 3.

**Why:** The static plugin set must never require separate maintenance — it should be *impossible* to add a plugin to `chatComposerPlugins` and forget the static side by accident, short of it actually needing a static override (in which case a missing override shows up immediately as a content diff in Task 1-style parity tests, not a silent omission).

- [ ] **Step 1: Add the derivation to `chat-composer-plugins.ts`**

Add these imports alongside the existing ones:

```ts
import { LinkKit, LinkKitStatic } from '@/components/editor/plugins/link-kit'
import { LinkPlugin } from '@platejs/link/react'
import { CalloutKitStatic } from '@/components/editor/plugins/callout-kit-static'
import { CalloutPlugin } from '@platejs/callout/react'
```

(`LinkKit`'s existing import line just gains `LinkKitStatic` alongside it; `CalloutKit`'s import is untouched, `CalloutKitStatic` is new.)

After the existing `chatComposerPlugins` array (leave it byte-for-byte as-is), add:

```ts
const STATIC_NODE_OVERRIDES: Record<string, (typeof chatComposerPlugins)[number]> = {
  [LinkPlugin.key]: LinkKitStatic[0],
  [CalloutPlugin.key]: CalloutKitStatic[0],
}

/**
 * `chatComposerPlugins`, derived — not hand-duplicated. A plugin added above
 * flows through automatically; only registered exceptions (Link's toolbar,
 * Callout's icon picker — both need an interactive editor, neither is a
 * content difference, see callout-content.tsx/link-kit.tsx) get swapped.
 */
export const chatComposerPluginsStatic = chatComposerPlugins.map(
  (plugin) => STATIC_NODE_OVERRIDES[plugin.key] ?? plugin,
)
```

- [ ] **Step 2: Write the derivation test**

```ts
import { describe, expect, it } from 'vitest'

import {
  chatComposerPlugins,
  chatComposerPluginsStatic,
} from '@/features/agent/composer/plate/chat-composer-plugins'

describe('chatComposerPluginsStatic', () => {
  it('has the same plugin keys, in the same order, as chatComposerPlugins', () => {
    expect(chatComposerPluginsStatic.map((p) => p.key)).toEqual(
      chatComposerPlugins.map((p) => p.key),
    )
  })

  it('swaps only link and callout', () => {
    const changed = chatComposerPluginsStatic.filter(
      (plugin, i) => plugin !== chatComposerPlugins[i],
    )
    expect(changed.map((p) => p.key).sort()).toEqual(['a', 'callout'].sort())
  })
})
```

Confirm the actual plugin key Platejs assigns to `LinkPlugin` (commonly `'a'` for the anchor node type) by running the test once and reading the failure — correct the expected key list to match reality rather than guessing further; do not hardcode a wrong assumption.

- [ ] **Step 3: Run it**

```
~/.bun/bin/bun vitest run web/src/__tests__/features/agent/composer/plate/chat-composer-plugins-static.test.ts
```

---

### Task 3: `MarkdownMessageStatic` and wiring into `MessageRow`

**Files:**
- Create: `web/src/features/agent/transcript/plate/markdown-message-static.tsx`
- Modify: `web/src/features/agent/transcript/message-row.tsx`
- Test: `web/src/__tests__/features/agent/transcript/plate/markdown-message-static.test.tsx`
- Test: extend `web/src/__tests__/features/agent/transcript/message-row.test.tsx`

**Interfaces:**
- Consumes: `chatComposerPluginsStatic` (Task 2), `chatMarkdownToValue` (existing, `chat-composer-serialization.ts`).
- Produces: `MarkdownMessageStatic({ children, className }: { children: string; className?: string })` — same prop signature as the existing `MarkdownMessage`, drop-in compatible.
- Consumed by: `message-row.tsx`'s render of user/assistant message text.

**Why:** Only the actively-streaming message needs `applyStreamedValue`'s patch-in-place and the full interactive editor. Every settled message is static content — render it through `PlateStatic` instead.

- [ ] **Step 1: Write `markdown-message-static.tsx`**

```tsx
import { useMemo } from 'react'
import { createStaticEditor, PlateStatic } from 'platejs/static'

import { chatComposerPluginsStatic } from '@/features/agent/composer/plate/chat-composer-plugins'
import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'
import { cn } from '@/lib/utils'

interface MarkdownMessageStaticProps {
  children: string
  className?: string
}

/**
 * A settled message, rendered without an interactive editor. Same
 * `chatComposerPluginsStatic` set MarkdownMessage's interactive editor uses —
 * see chat-composer-plugins.ts for why that set can't drift from this one.
 */
export function MarkdownMessageStatic({ children, className }: MarkdownMessageStaticProps) {
  const editor = useMemo(
    () =>
      createStaticEditor({
        plugins: chatComposerPluginsStatic,
        value: chatMarkdownToValue(children),
      }),
    [children],
  )

  return <PlateStatic editor={editor} className={cn('agent-prose', className)} />
}
```

- [ ] **Step 2: Wire `MessageRow` to pick interactive vs. static**

Read `message-row.tsx` around line 162 first (the existing `<MarkdownMessage className="break-words">{message.text}</MarkdownMessage>` call) to confirm the exact prop the row already has for "is this the live streaming message" — likely something derived from `message.id` matching the chat's current streaming id, already threaded through since `agent-transcript.tsx` renders `streamingBubbles` separately from `messages`. If `MessageRow` doesn't yet know whether its own message is the streaming one, add an `isStreaming?: boolean` prop, defaulting to `false`, threaded from `agent-transcript.tsx`'s `messages.map` (which does have access to whichever id is currently streaming, e.g. via `props.streamingMessageId` or equivalent — check `agent-chat-view.tsx` for how the streaming id is tracked and pass it down).

Replace the render with:

```tsx
{isStreaming ? (
  <MarkdownMessage className="break-words">{message.text}</MarkdownMessage>
) : (
  <MarkdownMessageStatic className="break-words">{message.text}</MarkdownMessageStatic>
)}
```

Import `MarkdownMessageStatic` alongside the existing `MarkdownMessage` import.

- [ ] **Step 3: Write the parity test for the full component (not just callout)**

Extend `markdown-message.test.tsx`'s existing fixtures (or add a sibling `markdown-message-static.test.tsx`) to render the SAME representative markdown (headings, list, table, code block, link, image, callout, raw HTML — reuse whatever fixture the Task 1 verification agent already built, check its throwaway test's fixture before it was deleted is gone, so recreate it here as the permanent version) through both `MarkdownMessage` and `MarkdownMessageStatic`, asserting `container.textContent` and the block-level tag structure (`container.querySelectorAll('h1,h2,h3,p,ul,ol,table,pre,a,img').length` and each one's `tagName`) match.

- [ ] **Step 4: Run it**

```
~/.bun/bin/bun vitest run web/src/__tests__/features/agent/transcript/plate/markdown-message-static.test.tsx web/src/__tests__/features/agent/transcript/message-row.test.tsx
```

- [ ] **Step 5: Type-check**

```
~/.bun/bin/bun tsc --noEmit
```

---

### Task 4: Flatten transcript rows into a pure function

**Files:**
- Create: `web/src/features/agent/transcript/lib/flatten-transcript-rows.ts`
- Test: `web/src/__tests__/features/agent/transcript/lib/flatten-transcript-rows.test.ts`

**Interfaces:**
- Consumes: the same inputs `agent-transcript.tsx` already computes — `messages: AgentChatMessage[]`, `compactionBefore`, `interruptedBefore` (Records keyed by sequence, per `agent-transcript.tsx:145-165`), `firstTurnSequence`, `firstReplySequence` (`agent-transcript.tsx:93-101`).
- Produces: `TranscriptRow` (discriminated union) and `flattenTranscriptRows(...)`.
- Consumed by: Task 5.

Read `agent-transcript.tsx` fully first (lines 61-165 especially) to get the exact current prop names for `compactionBefore`/`interruptedBefore`/`firstTurnSequence`/`firstReplySequence` before writing the signature below — the names here are from the grounding investigation and may need adjusting to match the file verbatim.

- [ ] **Step 1: Write the failing test**

```ts
import { describe, expect, it } from 'vitest'

import { flattenTranscriptRows } from '@/features/agent/transcript/lib/flatten-transcript-rows'
import type { AgentChatMessage } from '@/features/agent/hooks/use-chat-messages'

function msg(sequence: number): AgentChatMessage {
  return { sequence, turnId: `t${sequence}`, text: `msg ${sequence}` } as AgentChatMessage
}

describe('flattenTranscriptRows', () => {
  it('emits one row per message, in sequence order', () => {
    const rows = flattenTranscriptRows({
      messages: [msg(1), msg(2), msg(3)],
      compactionBefore: {},
      interruptedBefore: {},
      firstTurnSequence: 1,
      firstReplySequence: undefined,
    })
    expect(rows.filter((r) => r.kind === 'message')).toHaveLength(3)
  })

  it('inserts a compaction-divider row before the message it precedes', () => {
    const rows = flattenTranscriptRows({
      messages: [msg(1), msg(2)],
      compactionBefore: { 2: { summary: 'compacted' } as never },
      interruptedBefore: {},
      firstTurnSequence: 1,
      firstReplySequence: undefined,
    })
    const idx = rows.findIndex((r) => r.kind === 'compaction-divider')
    expect(idx).toBeGreaterThanOrEqual(0)
    expect(rows[idx + 1]).toMatchObject({ kind: 'message', message: { sequence: 2 } })
  })

  it('inserts a first-turn-divider row after the message matching firstTurnSequence', () => {
    const rows = flattenTranscriptRows({
      messages: [msg(1), msg(2)],
      compactionBefore: {},
      interruptedBefore: {},
      firstTurnSequence: 1,
      firstReplySequence: undefined,
    })
    const msgIdx = rows.findIndex((r) => r.kind === 'message' && r.message.sequence === 1)
    expect(rows[msgIdx + 1]?.kind).toBe('first-turn-divider')
  })
})
```

- [ ] **Step 2: Run it, confirm it fails**

```
~/.bun/bin/bun vitest run web/src/__tests__/features/agent/transcript/lib/flatten-transcript-rows.test.ts
```

Expected: FAIL — module doesn't exist yet.

- [ ] **Step 3: Implement**

```ts
import type { AgentChatMessage } from '@/features/agent/hooks/use-chat-messages'

export type TranscriptRow =
  | { kind: 'message'; key: string; message: AgentChatMessage }
  | { kind: 'compaction-divider'; key: string; sequence: number; info: unknown }
  | { kind: 'interrupted-divider'; key: string; sequence: number; info: unknown }
  | { kind: 'first-turn-divider'; key: string }

export function flattenTranscriptRows({
  messages,
  compactionBefore,
  interruptedBefore,
  firstTurnSequence,
  firstReplySequence,
}: {
  messages: AgentChatMessage[]
  compactionBefore: Record<number, unknown>
  interruptedBefore: Record<number, unknown>
  firstTurnSequence: number | undefined
  firstReplySequence: number | undefined
}): TranscriptRow[] {
  const rows: TranscriptRow[] = []

  for (const message of messages) {
    const compaction = compactionBefore[message.sequence]
    if (compaction !== undefined) {
      rows.push({
        kind: 'compaction-divider',
        key: `compaction-${message.sequence}`,
        sequence: message.sequence,
        info: compaction,
      })
    }
    const interrupted = interruptedBefore[message.sequence]
    if (interrupted !== undefined) {
      rows.push({
        kind: 'interrupted-divider',
        key: `interrupted-${message.sequence}`,
        sequence: message.sequence,
        info: interrupted,
      })
    }

    rows.push({ kind: 'message', key: `message-${message.sequence}`, message })

    if (message.sequence === firstTurnSequence) {
      rows.push({ kind: 'first-turn-divider', key: 'first-turn-divider' })
    }
  }

  return rows
}
```

Reconcile this against `agent-transcript.tsx`'s actual current render order for dividers (compaction vs. interrupted vs. first-turn — confirm which renders *before* vs. *after* the message it's attached to for each divider type; the sketch above assumes compaction/interrupted precede their message and first-turn follows, matching the grounding investigation's read of lines 145-165, but verify against the literal file before trusting it) and adjust. `firstReplySequence` may turn out to only gate which divider variant renders, not its position — read the actual JSX before finalizing this function's shape.

- [ ] **Step 4: Run it, confirm it passes**

```
~/.bun/bin/bun vitest run web/src/__tests__/features/agent/transcript/lib/flatten-transcript-rows.test.ts
```

---

### Task 5: Virtualize `AgentTranscript`'s historical rows

**Files:**
- Modify: `web/src/features/agent/transcript/agent-transcript.tsx`
- Modify: `web/src/features/agent/styles/transcript.css`
- Test: extend `web/src/__tests__/features/agent/transcript/agent-transcript.test.tsx`

**Interfaces:**
- Consumes: `flattenTranscriptRows` (Task 4), `useVirtualizer` from `@tanstack/react-virtual` (already a dependency).
- Produces: no new exports — internal to `AgentTranscript`.

**Why:** Model directly on `git-history-list.tsx:74-80` (dynamic row height, already proven in this codebase) and `changed-files-tree.tsx`'s flattened-row pattern (already proven for heterogeneous row types).

- [ ] **Step 1: Read the precedents first**

Read `web/src/features/git/components/git-history-list.tsx` (the virtualizer config + render) and `web/src/features/git/tree/hooks/use-agent-chat-list-virtualizer.ts` (the pane-resize-suppression `observeElementRect` trick) in full before writing this task's code — copy their patterns exactly rather than reinventing.

- [ ] **Step 2: Wire the virtualizer into `AgentTranscript`**

Replace the `messages.map(...)` block (`agent-transcript.tsx:145-165`) with a flattened-row virtualizer. Keep everything *after* that block (`trailingInterruption`, `streamingBubbles.map`, `queue.map`, `WorkingLine`, currently lines 166-208) exactly as normal flex children, unchanged — they're small-count, always-visible tail items with no reason to virtualize, and leaving them alone preserves `.stream`'s `margin-top:auto` bottom-anchor behavior with zero changes to `use-transcript-anchor.ts`.

```tsx
const rows = useMemo(
  () =>
    flattenTranscriptRows({
      messages,
      compactionBefore: props.compactionBefore,
      interruptedBefore: props.interruptedBefore,
      firstTurnSequence,
      firstReplySequence,
    }),
  [messages, props.compactionBefore, props.interruptedBefore, firstTurnSequence, firstReplySequence],
)

const rowVirtualizer = useVirtualizer({
  count: rows.length,
  getScrollElement: () => anchor.scrollRef.current,
  estimateSize: () => 64,
  overscan: 12,
  measureElement: (el) => el.getBoundingClientRect().height,
  getItemKey: (index) => rows[index].key,
})
```

Render:

```tsx
<div style={{ position: 'relative', height: rowVirtualizer.getTotalSize(), width: '100%' }}>
  {rowVirtualizer.getVirtualItems().map((virtualRow) => {
    const row = rows[virtualRow.index]
    const isLast = virtualRow.index === rows.length - 1
    return (
      <div
        key={row.key}
        ref={rowVirtualizer.measureElement}
        data-index={virtualRow.index}
        style={{
          position: 'absolute',
          top: 0,
          left: 0,
          width: '100%',
          transform: `translateY(${virtualRow.start}px)`,
          paddingBottom: isLast ? 0 : 18,
        }}
      >
        <TranscriptRowView row={row} providers={props.providers} toolCallsByTurn={props.toolCallsByTurn} precedingUserAt={precedingUserAtByAssistantSequence} />
      </div>
    )
  })}
</div>
```

Where `TranscriptRowView` is a small local switch component (in the same file, or a sibling file if it grows) mapping `row.kind` to `<MessageRow>` / `<CompactionDivider>` / `<InterruptedDivider>` / `<FirstTurnDivider>` — port the exact prop-passing from the current `messages.map` block, don't drop any prop.

`paddingBottom: 18` on every row but the last reproduces `.stream`'s `gap: 18px` (which only applied between flex children, never after the last one) — `measureElement` measures the row's full outer height *including* this padding, so `rowVirtualizer`'s offsets stay correct.

- [ ] **Step 3: CSS — give the virtualized block an explicit positioning context**

In `transcript.css`, add a rule for the new wrapper (name it to match whatever class/data-attribute you give it in Step 2, e.g. `.agent-chat .stream .virtual-rows`):

```css
.agent-chat .stream .virtual-rows {
  position: relative;
  width: 100%;
}
```

No changes to `.agent-chat .scroll` or `.agent-chat .stream` themselves — the virtualized block is just their first flex child, same as the old `messages.map` output was.

- [ ] **Step 4: Run the existing transcript tests, confirm they still pass**

```
~/.bun/bin/bun vitest run web/src/__tests__/features/agent/transcript/agent-transcript.test.tsx
```

Fix any prop-threading gaps this surfaces (a divider or `MessageRow` prop dropped in the `TranscriptRowView` switch) rather than editing the test to match broken output.

- [ ] **Step 5: Type-check**

```
~/.bun/bin/bun tsc --noEmit
```

---

### Task 6: Scale test — mounted count stays flat

**Files:**
- Create: `web/src/__tests__/features/agent/transcript/agent-transcript.scale.test.tsx`

**Interfaces:**
- Consumes: `AgentTranscript` (Task 5), same rendering harness `agent-transcript.test.tsx` already uses.

**Why:** This is the test that actually proves the goal — model on `web/src/__tests__/features/git/components/changed-files-tree.scale.test.tsx`, which already solves the jsdom-has-no-layout-engine problem (mock `HTMLElement.prototype.getBoundingClientRect` to a fixed viewport rect) for exactly this kind of test.

- [ ] **Step 1: Write it**

Read `changed-files-tree.scale.test.tsx` first and copy its `getBoundingClientRect` mock verbatim. Then:

```tsx
import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { AgentTranscript } from '@/features/agent/transcript/agent-transcript'
// ... plus whatever harness/props agent-transcript.test.tsx uses to mount it

function messagesOfLength(n: number) {
  return Array.from({ length: n }, (_, i) => ({
    sequence: i,
    turnId: `t${i}`,
    text: `message number ${i}`,
    // ...fill in whatever other fields AgentChatMessage requires, matching
    // the fixture builder already used in agent-transcript.test.tsx
  }))
}

describe('AgentTranscript scale', () => {
  it('keeps mounted row count roughly constant as loaded message count grows', () => {
    const small = render(<AgentTranscript /* ...props with messages: messagesOfLength(100) */ />)
    const smallCount = small.container.querySelectorAll('[data-index]').length

    const large = render(<AgentTranscript /* ...props with messages: messagesOfLength(2000) */ />)
    const largeCount = large.container.querySelectorAll('[data-index]').length

    expect(largeCount / smallCount).toBeLessThan(1.5)
  })
})
```

- [ ] **Step 2: Run it**

```
~/.bun/bin/bun vitest run web/src/__tests__/features/agent/transcript/agent-transcript.scale.test.tsx
```

Expected: PASS. If it fails because the virtualizer isn't shrinking mounted count, that's a real bug in Task 5 — go fix Task 5, don't loosen this test's threshold.

---

### Task 7: Yield during the evidence-recovery burst load

**Files:**
- Modify: `web/src/features/agent/hooks/use-chat-messages.ts`
- Test: extend `web/src/__tests__/features/agent/hooks/use-chat-messages.test.ts`

**Interfaces:**
- Modifies: `loadInitial`'s internal page loop (`use-chat-messages.ts:141-173`). No signature change — `loadInitial`'s external contract is unchanged.

**Why:** Confirmed: `loadInitial` can apply up to `EVIDENCE_RECOVERY_MAX_PAGES(100) × MESSAGE_PAGE_SIZE(100)` = 10,000 messages via `applyMessages` in one synchronous loop, only `await`-ing the network fetch between pages — no yield to the render pipeline. A chat opened with deep queued-evidence history can block the main thread for one long task.

- [ ] **Step 1: Read the exact current loop first**

Read `use-chat-messages.ts:123-183` in full — the forward-from-baseline branch and the backward-for-new-chat branch both need this change; get their exact current shape before editing.

- [ ] **Step 2: Add a yield between pages**

Add a small helper near the top of the file:

```ts
function yieldToRenderer(): Promise<void> {
  return new Promise((resolve) => requestAnimationFrame(() => resolve()))
}
```

In each page-loop branch of `loadInitial`, after `applyMessages(page.items)` (or equivalent) and before fetching the next page, add:

```ts
await yieldToRenderer()
```

Do not add this inside `refresh`'s loop (`use-chat-messages.ts:185-219`) — that path already runs on a 1s poll cadence with typically-small pages; only `loadInitial`'s burst-load path needs it.

- [ ] **Step 3: Write the test**

```ts
it('yields to the renderer between evidence-recovery pages instead of blocking synchronously', async () => {
  // Set up a mock fetcher returning EVIDENCE_RECOVERY_MAX_PAGES pages of
  // MESSAGE_PAGE_SIZE messages each, matching whatever mock shape the
  // existing "ordering: displayOrder over sequence" test in this file uses.
  const rafSpy = vi.spyOn(window, 'requestAnimationFrame')
  const { result } = renderHook(() => useChatMessages(/* ...same setup as existing tests */))

  await act(async () => {
    await result.current.loadInitial()
  })

  expect(rafSpy).toHaveBeenCalled()
})
```

Match this to whatever mocking pattern the existing describe blocks (`applyMessages empty-page guard`, etc.) already use in this file — read them first, don't introduce a second mocking style.

- [ ] **Step 4: Run it**

```
~/.bun/bin/bun vitest run web/src/__tests__/features/agent/hooks/use-chat-messages.test.ts
```

Confirm the existing `mergeMessages: keyed by turnId, not sequence` and `ordering: displayOrder over sequence` tests still pass unmodified — this task must not touch merge/ordering behavior, only add a yield point.

---

### Task 8: Confirm scroll-anchor code needs no changes

**Files:**
- None modified. Verification-only.

**Why:** The design in the spec claims `use-transcript-anchor.ts` and `follow-scroll.ts` need zero changes because their bottom-anchor logic works entirely off `el.scrollTop`/`el.scrollHeight`/`el.clientHeight` — virtualizer-agnostic. This task proves that claim after Task 5 lands, rather than leaving it asserted.

- [ ] **Step 1: Run the existing tests unmodified, after Task 5 is merged**

```
~/.bun/bin/bun vitest run web/src/__tests__/features/agent/hooks/use-transcript-anchor.test.tsx web/src/__tests__/features/agent/hooks/lib/follow-scroll.test.ts
```

Expected: PASS, with zero source changes to either file. If either fails, that disproves the spec's assumption — stop and re-open the design (do not patch the test to match broken scroll behavior).

- [ ] **Step 2: Manual confirmation on live app**

On the running dev-desktop app (see Task 9), scroll to the middle of a long virtualized transcript, then send a new prompt while scrolled away from the bottom — confirm the transcript does NOT auto-scroll (the `stuck` state correctly detects "not at bottom" through the virtualizer's total-height wrapper). Then scroll to the bottom and send another prompt — confirm it DOES follow the new content.

---

### Task 9: Live verification on the running app

**Files:** None. Verification-only, run last, after Tasks 1-8 are all green.

- [ ] **Step 1: Build and launch the dev-desktop app** (per this project's existing dev workflow — check for a `make dev-desktop` target or equivalent; do not start a second daemon instance if one is already running for this worktree).

- [ ] **Step 2: Using the Tauri MCP tools, open a chat with a large loaded history** (seed one with hundreds of turns if none exists) and confirm via `mcp__tauri__webview_dom_snapshot` or `mcp__tauri__webview_screenshot` that scrolling through it stays smooth and that `document.querySelectorAll('[data-index]').length` (via `mcp__tauri__webview_execute_js`) stays roughly constant while scrolling from top to bottom.

- [ ] **Step 3: Confirm a live streaming reply still renders correctly** — start a new prompt, watch the streaming message render via the interactive editor path (not static) while it's in flight, and confirm it swaps to the static path once settled with no visible flash/layout jump.

- [ ] **Step 4: Confirm no regressions in dividers** — a chat with a compaction or interrupted-turn marker still shows the correct divider at the correct position post-virtualization.
