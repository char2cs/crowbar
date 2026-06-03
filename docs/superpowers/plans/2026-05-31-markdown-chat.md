# Markdown-Chat Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the bubble-style chat with a shared CM6 markdown document where humans and agents write to the same canvas, with live preview, turn tinting, streaming, tool call pills, a sticky todo checklist, Excalidraw/Mermaid widgets, and a slash command palette.

**Architecture:** Turn store (Zustand, per-workspace) is source of truth; CM6 is a windowed view assembled from the last 100 turns. Extensions handle tinting, read-only enforcement, live preview, streaming, widgets, and slash commands. The replacement chain: `pane-container.tsx` → `FlowContent` → `ChatView` (workflow) → the new `MarkdownChatView`. We replace `features/workflow/components/chat-view.tsx` with a wrapper around the new CM6-based view and delete `components/chat/`.

**Tech Stack:** `@codemirror/state` `@codemirror/view` `@codemirror/lang-markdown` `@codemirror/commands` `@uiw/react-codemirror`; `@excalidraw/excalidraw`; `mermaid`; `nanoid` (already installed); existing Zustand + WS infrastructure.

---

## File Map

### New files

| Path | Responsibility |
|---|---|
| `web/src/features/markdown-chat/types.ts` | `MarkdownTurn`, `WidgetData`, `TurnRole`, `ToolCallData` |
| `web/src/features/markdown-chat/stores/conversation-store.ts` | Zustand per-workspace registry; owns `MarkdownTurn[]` |
| `web/src/features/markdown-chat/extensions/turn-boundaries.ts` | Parse `<!-- turn:ID role:ROLE -->` markers; tinting; read-only filter; streaming bypass annotation |
| `web/src/features/markdown-chat/extensions/live-preview.ts` | Hide markdown syntax tokens; render styled decorations (headings, bold, italic, inline code) outside cursor line |
| `web/src/features/markdown-chat/extensions/streaming-ext.ts` | `appendStreamChunk(view, text)` — insert at document end; blinking cursor decoration |
| `web/src/features/markdown-chat/extensions/todo-sticky.ts` | Detect `- [ ]` / `- [x]` blocks in streaming turn; apply `sticky-active` class while streaming |
| `web/src/features/markdown-chat/extensions/widget-ext.ts` | Detect fenced blocks with `widget-id:` and `<!-- tool-call:{...} -->`; mount React widget decorations |
| `web/src/features/markdown-chat/extensions/slash-command-ext.ts` | Intercept `/` keypress; open command palette overlay |
| `web/src/features/markdown-chat/extensions/widget-registry.ts` | `WIDGET_REGISTRY: Record<string, WidgetComponent>` |
| `web/src/features/markdown-chat/components/markdown-chat-view.tsx` | Root component — wires store, WS, CM6 editor, toolbar |
| `web/src/features/markdown-chat/components/markdown-chat-editor.tsx` | CM6 host — mounts all extensions, exposes `EditorView` ref |
| `web/src/features/markdown-chat/components/markdown-chat-toolbar.tsx` | Docked toolbar: formatting group + Insert dropdown |
| `web/src/features/markdown-chat/components/slash-command-palette.tsx` | Floating command palette overlay |
| `web/src/features/markdown-chat/components/tool-call-pill.tsx` | Collapsible tool call pill widget |
| `web/src/features/markdown-chat/components/mermaid-widget.tsx` | Read-only Mermaid SVG renderer |
| `web/src/features/markdown-chat/components/excalidraw-widget.tsx` | Editable Excalidraw canvas widget |
| `web/src/lib/mock/markdown-chat.ts` | Mock `MarkdownTurn[]` + streaming simulation |
| `web/src/mocks/handlers/markdown-chat.ts` | MSW REST + WS handlers for chat endpoints |
| `web/src/__tests__/features/markdown-chat/stores/conversation-store.test.ts` | Store unit tests |
| `web/src/__tests__/features/markdown-chat/extensions/turn-boundaries.test.ts` | Extension unit tests |
| `web/src/__tests__/features/markdown-chat/extensions/streaming-ext.test.ts` | Extension unit tests |
| `web/src/__tests__/features/markdown-chat/extensions/live-preview.test.ts` | Extension unit tests |
| `web/src/__tests__/features/markdown-chat/extensions/todo-sticky.test.ts` | Extension unit tests |
| `web/src/__tests__/features/markdown-chat/components/markdown-chat-view.test.tsx` | Integration smoke test |

### Modified files

| Path | Change |
|---|---|
| `web/src/features/workflow/components/flow-content.tsx` | Import `MarkdownChatView` instead of `ChatView` |
| `web/src/features/panes/components/pane-container.tsx` | No change — already routes to `FlowContent` |

### Deleted files

| Path |
|---|
| `web/src/features/workflow/components/chat-view.tsx` |
| `web/src/components/chat/ChatView.tsx` |
| `web/src/components/chat/MessageBubble.tsx` |
| `web/src/components/chat/ChatInput.tsx` |
| `web/src/components/chat/ChatEmptyState.tsx` |
| `web/src/components/chat/ToolCallSeparator.tsx` |
| `web/src/lib/mock/conversations.ts` |

> `web/src/components/chat/MarkdownContent.tsx` is kept — check for other usages before deleting.

---

## Task 1: Install CM6 and Excalidraw dependencies

**Files:** `web/package.json`

- [ ] **Step 1: Install packages**

```bash
cd web
npm install @codemirror/state @codemirror/view @codemirror/lang-markdown @codemirror/commands @uiw/react-codemirror @excalidraw/excalidraw mermaid
```

Expected: no peer-dep warnings; `node_modules/@codemirror/state` exists.

- [ ] **Step 2: Verify TypeScript resolves the new packages**

```bash
cd web
npx tsc --noEmit 2>&1 | grep -i codemirror | head -5
```

Expected: no output (no TS errors from new packages).

- [ ] **Step 3: Commit**

```bash
cd web
git add package.json package-lock.json
git commit -m "chore: install CM6, Excalidraw, and Mermaid deps"
```

---

## Task 2: Types

**Files:**
- Create: `web/src/features/markdown-chat/types.ts`

- [ ] **Step 1: Write types**

```typescript
// web/src/features/markdown-chat/types.ts

export type TurnRole = 'user' | 'agent'

export interface WidgetData {
  id: string
  type: string      // frontend registry key: 'excalidraw' | 'mermaid'
  payload: unknown  // opaque — backend never inspects
}

export interface ToolCallData {
  name: string
  args: Record<string, unknown>
  status: 'pending' | 'done' | 'error'
  output: string
}

export interface MarkdownTurn {
  id: string
  role: TurnRole
  content: string        // raw markdown; fenced blocks reference widget IDs
  timestamp: string      // ISO 8601
  authorName: string
  widgets: WidgetData[]
  streaming?: boolean    // true while agent is actively writing
}

// Turn boundary marker embedded in CM6 document text:
// <!-- turn:ID role:ROLE -->
// These lines are hidden by turn-boundary-ext decorations.
export const TURN_MARKER_RE = /^<!-- turn:([a-zA-Z0-9_-]+) role:(user|agent) -->$/

// Widget fenced block info string format: "excalidraw widget-id:abc123"
export const WIDGET_ID_RE = /widget-id:([a-zA-Z0-9_-]+)/

// Tool call embedded comment format:
// <!-- tool-call:{...JSON...} -->
export const TOOL_CALL_RE = /^<!-- tool-call:(.+) -->$/
```

- [ ] **Step 2: Verify TypeScript**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "markdown-chat/types" | head -5
```

Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/markdown-chat/types.ts
git commit -m "feat(markdown-chat): add MarkdownTurn, WidgetData, ToolCallData types"
```

---

## Task 3: Conversation Store

**Files:**
- Create: `web/src/features/markdown-chat/stores/conversation-store.ts`
- Create: `web/src/__tests__/features/markdown-chat/stores/conversation-store.test.ts`

- [ ] **Step 1: Write the failing tests**

```typescript
// web/src/__tests__/features/markdown-chat/stores/conversation-store.test.ts
import { getOrCreateConversationStore, destroyConversationStore } from '@/features/markdown-chat/stores/conversation-store'

const TURN = {
  id: 't1',
  role: 'user' as const,
  content: 'Hello',
  timestamp: '2026-05-31T00:00:00Z',
  authorName: 'Mateo',
  widgets: [],
}

afterEach(() => {
  destroyConversationStore('ws1')
})

test('appendTurn adds turn to store', () => {
  const store = getOrCreateConversationStore('ws1')
  store.getState().appendTurn(TURN)
  expect(store.getState().turns).toHaveLength(1)
  expect(store.getState().turns[0].id).toBe('t1')
})

test('updateStreamingTurn appends content delta', () => {
  const store = getOrCreateConversationStore('ws1')
  store.getState().appendTurn({ ...TURN, id: 'a1', role: 'agent', streaming: true, content: '' })
  store.getState().updateStreamingTurn('a1', 'Hello ')
  store.getState().updateStreamingTurn('a1', 'world')
  expect(store.getState().turns[0].content).toBe('Hello world')
})

test('finalizeStreamingTurn clears streaming flag', () => {
  const store = getOrCreateConversationStore('ws1')
  store.getState().appendTurn({ ...TURN, id: 'a1', role: 'agent', streaming: true, content: '' })
  store.getState().finalizeStreamingTurn('a1')
  expect(store.getState().turns[0].streaming).toBe(false)
})

test('updateWidgetPayload updates widget in turn', () => {
  const store = getOrCreateConversationStore('ws1')
  store.getState().appendTurn({
    ...TURN,
    id: 'a1',
    widgets: [{ id: 'w1', type: 'excalidraw', payload: null }],
  })
  store.getState().updateWidgetPayload('a1', 'w1', { elements: [] })
  const widget = store.getState().turns[0].widgets[0]
  expect((widget.payload as { elements: unknown[] }).elements).toEqual([])
})

test('getOrCreateConversationStore returns same instance for same wsId', () => {
  const a = getOrCreateConversationStore('ws1')
  const b = getOrCreateConversationStore('ws1')
  expect(a).toBe(b)
})

test('destroyConversationStore removes the store', () => {
  const a = getOrCreateConversationStore('ws1')
  destroyConversationStore('ws1')
  const b = getOrCreateConversationStore('ws1')
  expect(a).not.toBe(b)
})
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd web && npx vitest run src/__tests__/features/markdown-chat/stores/conversation-store.test.ts
```

Expected: FAIL — `getOrCreateConversationStore` is not defined.

- [ ] **Step 3: Implement the store**

```typescript
// web/src/features/markdown-chat/stores/conversation-store.ts
import { createStore } from 'zustand'
import type { MarkdownTurn, WidgetData } from '../types'

interface ConversationState {
  turns: MarkdownTurn[]
  appendTurn: (turn: MarkdownTurn) => void
  updateStreamingTurn: (id: string, contentDelta: string) => void
  finalizeStreamingTurn: (id: string) => void
  updateWidgetPayload: (turnId: string, widgetId: string, payload: unknown) => void
}

type ConversationStore = ReturnType<typeof createConversationStore>

function createConversationStore() {
  return createStore<ConversationState>((set) => ({
    turns: [],
    appendTurn: (turn) =>
      set((s) => ({ turns: [...s.turns, turn] })),
    updateStreamingTurn: (id, delta) =>
      set((s) => ({
        turns: s.turns.map((t) =>
          t.id === id ? { ...t, content: t.content + delta } : t
        ),
      })),
    finalizeStreamingTurn: (id) =>
      set((s) => ({
        turns: s.turns.map((t) =>
          t.id === id ? { ...t, streaming: false } : t
        ),
      })),
    updateWidgetPayload: (turnId, widgetId, payload) =>
      set((s) => ({
        turns: s.turns.map((t) =>
          t.id !== turnId
            ? t
            : {
                ...t,
                widgets: t.widgets.map((w: WidgetData) =>
                  w.id === widgetId ? { ...w, payload } : w
                ),
              }
        ),
      })),
  }))
}

const registry = new Map<string, ConversationStore>()

export function getOrCreateConversationStore(wsId: string): ConversationStore {
  if (!registry.has(wsId)) {
    registry.set(wsId, createConversationStore())
  }
  return registry.get(wsId)!
}

export function destroyConversationStore(wsId: string): void {
  registry.delete(wsId)
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd web && npx vitest run src/__tests__/features/markdown-chat/stores/conversation-store.test.ts
```

Expected: all 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/markdown-chat/stores/conversation-store.ts \
        web/src/__tests__/features/markdown-chat/stores/conversation-store.test.ts
git commit -m "feat(markdown-chat): add per-workspace conversation store"
```

---

## Task 4: Mock Data

**Files:**
- Create: `web/src/lib/mock/markdown-chat.ts`
- Delete: `web/src/lib/mock/conversations.ts`

- [ ] **Step 1: Create mock turns**

```typescript
// web/src/lib/mock/markdown-chat.ts
import type { MarkdownTurn } from '@/features/markdown-chat/types'

const MOCK_TURNS: Record<string, MarkdownTurn[]> = {
  'ws3:brainstorm': [
    {
      id: 'mt1',
      role: 'user',
      content: 'How should we handle auth across crowbar, quiver.core and quiver.desktop?',
      timestamp: '2026-05-31T10:00:00Z',
      authorName: 'Mateo',
      widgets: [],
    },
    {
      id: 'mt2',
      role: 'agent',
      content: `Given all three share a user identity, a **shared auth service** makes the most sense.

Here are the three options I'd consider:

## Option A — Shared Auth Microservice
<!-- tool-call:{"name":"read_file","args":{"path":"src/auth/token.ts"},"status":"done","output":"export async function refreshToken(..."} -->

Each app delegates auth to a central service. Single source of truth for tokens.

## Option B — Auth SDK
Ship a shared \`@quiver/auth\` package that each app installs. Keeps network hops low.

## Option C — OAuth + PKCE per app
Each app runs its own OAuth flow. Simpler to deploy, harder to revoke globally.

I'd recommend **Option A** for Crowbar specifically since you already have a Go backend planned.`,
      timestamp: '2026-05-31T10:01:00Z',
      authorName: 'Claude',
      widgets: [],
    },
    {
      id: 'mt3',
      role: 'user',
      content: "Makes sense. Let's go with Option A.",
      timestamp: '2026-05-31T10:02:00Z',
      authorName: 'Mateo',
      widgets: [],
    },
  ],
  'ws2:build': [
    {
      id: 'mt4',
      role: 'agent',
      content: `Starting implementation. Here's my plan:

- [ ] Read existing auth files
- [ ] Create token-service skeleton
- [ ] Wire up refresh logic
- [ ] Update error handling
- [ ] Write tests`,
      timestamp: '2026-05-31T09:00:00Z',
      authorName: 'Claude',
      widgets: [],
    },
  ],
}

export function getMockMarkdownTurns(wsId: string, stepId: string): MarkdownTurn[] {
  return MOCK_TURNS[`${wsId}:${stepId}`] ?? []
}

// Simulate streaming a response in chunks every 30ms.
// Returns a cancel function.
export function simulateMarkdownStream(
  text: string,
  onChunk: (chunk: string) => void,
  onDone: () => void,
): () => void {
  const words = text.split(' ')
  let i = 0
  let cancelled = false

  function tick() {
    if (cancelled || i >= words.length) {
      if (!cancelled) onDone()
      return
    }
    onChunk((i === 0 ? '' : ' ') + words[i])
    i++
    setTimeout(tick, 30)
  }

  setTimeout(tick, 30)
  return () => { cancelled = true }
}
```

- [ ] **Step 2: Check if `conversations.ts` is referenced anywhere other than the old ChatView**

```bash
grep -r "from '@/lib/mock/conversations'" web/src --include="*.ts" --include="*.tsx" | grep -v "chat-view.tsx"
```

Expected: no output. If output appears, update those imports to use `getMockMarkdownTurns` before deleting.

- [ ] **Step 3: Delete the old mock file**

```bash
rm web/src/lib/mock/conversations.ts
```

- [ ] **Step 4: Verify TypeScript**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "conversations" | head -10
```

Expected: no errors (or only errors in files that were already importing `ChatMessage` types — fix those if found).

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/mock/markdown-chat.ts
git rm web/src/lib/mock/conversations.ts
git commit -m "feat(markdown-chat): add mock turns and streaming simulator, remove old mock"
```

---

## Task 5: Turn Boundaries CM6 Extension

**Files:**
- Create: `web/src/features/markdown-chat/extensions/turn-boundaries.ts`
- Create: `web/src/__tests__/features/markdown-chat/extensions/turn-boundaries.test.ts`

- [ ] **Step 1: Write the failing tests**

```typescript
// web/src/__tests__/features/markdown-chat/extensions/turn-boundaries.test.ts
import { Text } from '@codemirror/state'
import { parseTurnBoundaries, turnsToDocument } from '@/features/markdown-chat/extensions/turn-boundaries'
import type { MarkdownTurn } from '@/features/markdown-chat/types'

const TURNS: MarkdownTurn[] = [
  { id: 'a1', role: 'agent', content: 'Hello', timestamp: '', authorName: 'Claude', widgets: [] },
  { id: 'u1', role: 'user', content: 'World', timestamp: '', authorName: 'Mateo', widgets: [] },
]

test('turnsToDocument produces boundary markers followed by content', () => {
  const doc = turnsToDocument(TURNS)
  expect(doc).toContain('<!-- turn:a1 role:agent -->')
  expect(doc).toContain('Hello')
  expect(doc).toContain('<!-- turn:u1 role:user -->')
  expect(doc).toContain('World')
})

test('parseTurnBoundaries finds turn ranges in document text', () => {
  const doc = turnsToDocument(TURNS)
  const text = Text.of(doc.split('\n'))
  const ranges = parseTurnBoundaries(text)
  expect(ranges).toHaveLength(2)
  expect(ranges[0].id).toBe('a1')
  expect(ranges[0].role).toBe('agent')
  expect(ranges[1].id).toBe('u1')
  expect(ranges[1].role).toBe('user')
})

test('parseTurnBoundaries returns correct from/to for each turn', () => {
  const doc = turnsToDocument(TURNS)
  const text = Text.of(doc.split('\n'))
  const ranges = parseTurnBoundaries(text)
  // The first turn starts at position 0
  expect(ranges[0].from).toBe(0)
  // The second turn starts after the first turn's content
  expect(ranges[1].from).toBeGreaterThan(ranges[0].from)
  // to positions are monotonically increasing
  expect(ranges[1].to).toBeGreaterThan(ranges[0].to)
})
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd web && npx vitest run src/__tests__/features/markdown-chat/extensions/turn-boundaries.test.ts
```

Expected: FAIL — `parseTurnBoundaries` is not defined.

- [ ] **Step 3: Implement the extension**

```typescript
// web/src/features/markdown-chat/extensions/turn-boundaries.ts
import {
  Annotation,
  EditorState,
  StateField,
  Transaction,
} from '@codemirror/state'
import {
  Decoration,
  DecorationSet,
  EditorView,
} from '@codemirror/view'
import { Text, RangeSetBuilder } from '@codemirror/state'
import type { MarkdownTurn, TurnRole } from '../types'
import { TURN_MARKER_RE } from '../types'

export interface TurnRange {
  id: string
  role: TurnRole
  from: number
  to: number
}

// Annotation used by streaming-ext to bypass the read-only filter.
export const streamingAnnotation = Annotation.define<boolean>()

// Convert turns array to a single CM6 document string.
export function turnsToDocument(turns: MarkdownTurn[]): string {
  return turns
    .map((t) => `<!-- turn:${t.id} role:${t.role} -->\n${t.content}`)
    .join('\n\n')
}

// Parse turn boundary markers from a CM6 Text object.
// Returns an array of TurnRange with character positions.
export function parseTurnBoundaries(doc: Text): TurnRange[] {
  const ranges: TurnRange[] = []
  let lineStart = 0

  for (let i = 1; i <= doc.lines; i++) {
    const line = doc.line(i)
    const match = line.text.match(TURN_MARKER_RE)
    if (match) {
      // Close the previous range
      if (ranges.length > 0) {
        ranges[ranges.length - 1].to = line.from - 1
      }
      ranges.push({
        id: match[1],
        role: match[2] as TurnRole,
        from: line.from,
        to: doc.length, // will be updated when next marker found or at end
      })
    }
    lineStart = line.to + 1
  }

  return ranges
}

// StateField that tracks turn ranges in the current document.
const turnRangesField = StateField.define<TurnRange[]>({
  create(state) {
    return parseTurnBoundaries(state.doc)
  },
  update(ranges, tr) {
    if (!tr.docChanged) return ranges
    return parseTurnBoundaries(tr.newDoc)
  },
})

// Build decorations for turn tinting and hiding boundary markers.
function buildDecorations(state: EditorState): DecorationSet {
  const ranges = state.field(turnRangesField)
  const builder = new RangeSetBuilder<Decoration>()

  for (const range of ranges) {
    const markerLine = state.doc.lineAt(range.from)
    // Hide the marker line itself
    builder.add(
      markerLine.from,
      markerLine.to,
      Decoration.mark({ class: 'cm-turn-marker-hidden' }),
    )
    // Tint the entire turn range
    const contentFrom = markerLine.to + 1
    if (contentFrom < range.to) {
      builder.add(
        contentFrom,
        range.to,
        Decoration.mark({ class: `cm-turn-${range.role}` }),
      )
    }
  }

  return builder.finish()
}

// StateField for decorations.
const turnDecorationsField = StateField.define<DecorationSet>({
  create(state) {
    return buildDecorations(state)
  },
  update(deco, tr) {
    if (!tr.docChanged) return deco
    return buildDecorations(tr.state)
  },
  provide: (f) => EditorView.decorations.from(f),
})

// Transaction filter: reject edits to completed (non-streaming) turns.
// Agent streaming bypasses this via streamingAnnotation.
function makeReadOnlyFilter(streamingTurnId: string | null) {
  return EditorState.transactionFilter.of((tr) => {
    if (!tr.docChanged) return tr
    if (tr.annotation(streamingAnnotation)) return tr

    const ranges = tr.startState.field(turnRangesField)
    const lastRange = ranges[ranges.length - 1]
    if (!lastRange) return tr

    // Allow edits only in the last turn's range (current user input)
    let blocked = false
    tr.changes.iterChanges((fromA) => {
      if (fromA < lastRange.from) blocked = true
    })

    return blocked ? [] : tr
  })
}

// CSS for turn tinting — injected via EditorView.theme.
const turnTheme = EditorView.theme({
  '.cm-turn-marker-hidden': { display: 'none' },
  '.cm-turn-user': { backgroundColor: 'hsl(var(--color-muted) / 0.4)' },
  '.cm-turn-agent': { backgroundColor: 'hsl(var(--color-accent) / 0.15)' },
})

export function turnBoundaries(streamingTurnId: string | null = null) {
  return [
    turnRangesField,
    turnDecorationsField,
    makeReadOnlyFilter(streamingTurnId),
    turnTheme,
  ]
}

// Selector to get current turn ranges from state.
export function getTurnRanges(state: EditorState): TurnRange[] {
  return state.field(turnRangesField)
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd web && npx vitest run src/__tests__/features/markdown-chat/extensions/turn-boundaries.test.ts
```

Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/markdown-chat/extensions/turn-boundaries.ts \
        web/src/__tests__/features/markdown-chat/extensions/turn-boundaries.test.ts
git commit -m "feat(markdown-chat): add turn-boundaries CM6 extension"
```

---

## Task 6: Live Preview CM6 Extension

**Files:**
- Create: `web/src/features/markdown-chat/extensions/live-preview.ts`
- Create: `web/src/__tests__/features/markdown-chat/extensions/live-preview.test.ts`

- [ ] **Step 1: Write the failing tests**

```typescript
// web/src/__tests__/features/markdown-chat/extensions/live-preview.test.ts
import { EditorState } from '@codemirror/state'
import { markdown } from '@codemirror/lang-markdown'
import { livePreview, hasLivePreviewDecoration } from '@/features/markdown-chat/extensions/live-preview'

function makeState(content: string) {
  return EditorState.create({
    doc: content,
    extensions: [markdown(), livePreview()],
  })
}

test('heading line gets heading decoration when cursor is not on it', () => {
  const state = makeState('# Hello World\nSome text')
  // cursor on line 2 (position 15), not on the heading
  const stateWithCursor = state.update({ selection: { anchor: 15 } }).state
  expect(hasLivePreviewDecoration(stateWithCursor, 0, 'cm-live-heading-1')).toBe(true)
})

test('heading line does not get decoration when cursor is on it', () => {
  const state = makeState('# Hello World\nSome text')
  // cursor on position 2 (inside the heading line)
  const stateWithCursor = state.update({ selection: { anchor: 2 } }).state
  expect(hasLivePreviewDecoration(stateWithCursor, 0, 'cm-live-heading-1')).toBe(false)
})

test('bold syntax gets decoration outside cursor line', () => {
  const state = makeState('Some **bold** text\nAnother line')
  const stateWithCursor = state.update({ selection: { anchor: 20 } }).state
  expect(hasLivePreviewDecoration(stateWithCursor, 5, 'cm-live-bold')).toBe(true)
})
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd web && npx vitest run src/__tests__/features/markdown-chat/extensions/live-preview.test.ts
```

Expected: FAIL.

- [ ] **Step 3: Implement the extension**

```typescript
// web/src/features/markdown-chat/extensions/live-preview.ts
import { EditorState, StateField, RangeSetBuilder } from '@codemirror/state'
import { Decoration, DecorationSet, EditorView } from '@codemirror/view'
import { syntaxTree } from '@codemirror/language'

// Exposed for testing — checks if a decoration of a given class exists at position.
export function hasLivePreviewDecoration(
  state: EditorState,
  pos: number,
  cls: string,
): boolean {
  const deco = state.field(livePreviewField, false)
  if (!deco) return false
  let found = false
  deco.between(pos, pos + 1, (_from, _to, d) => {
    if ((d.spec as { class?: string }).class === cls) found = true
  })
  return found
}

function cursorLine(state: EditorState): number {
  return state.doc.lineAt(state.selection.main.head).number
}

function buildLivePreviewDecorations(state: EditorState): DecorationSet {
  const builder = new RangeSetBuilder<Decoration>()
  const activeLine = cursorLine(state)
  const tree = syntaxTree(state)

  tree.cursor().iterate((node) => {
    const lineNum = state.doc.lineAt(node.from).number
    if (lineNum === activeLine) return // show raw syntax on cursor line

    switch (node.name) {
      case 'ATXHeading1':
        builder.add(node.from, node.to, Decoration.mark({ class: 'cm-live-heading-1' }))
        break
      case 'ATXHeading2':
        builder.add(node.from, node.to, Decoration.mark({ class: 'cm-live-heading-2' }))
        break
      case 'ATXHeading3':
        builder.add(node.from, node.to, Decoration.mark({ class: 'cm-live-heading-3' }))
        break
      case 'StrongEmphasis':
        builder.add(node.from, node.to, Decoration.mark({ class: 'cm-live-bold' }))
        break
      case 'Emphasis':
        builder.add(node.from, node.to, Decoration.mark({ class: 'cm-live-italic' }))
        break
      case 'InlineCode':
        builder.add(node.from, node.to, Decoration.mark({ class: 'cm-live-inline-code' }))
        break
    }
  })

  return builder.finish()
}

const livePreviewField = StateField.define<DecorationSet>({
  create: (state) => buildLivePreviewDecorations(state),
  update(deco, tr) {
    if (!tr.docChanged && !tr.selection) return deco
    return buildLivePreviewDecorations(tr.state)
  },
  provide: (f) => EditorView.decorations.from(f),
})

const livePreviewTheme = EditorView.theme({
  '.cm-live-heading-1': {
    fontSize: '1.5em',
    fontWeight: '700',
    '& .cm-md-syntax': { display: 'none' },
  },
  '.cm-live-heading-2': {
    fontSize: '1.25em',
    fontWeight: '700',
    '& .cm-md-syntax': { display: 'none' },
  },
  '.cm-live-heading-3': {
    fontSize: '1.1em',
    fontWeight: '700',
    '& .cm-md-syntax': { display: 'none' },
  },
  '.cm-live-bold': { fontWeight: '700' },
  '.cm-live-italic': { fontStyle: 'italic' },
  '.cm-live-inline-code': {
    fontFamily: 'var(--font-editor)',
    backgroundColor: 'hsl(var(--color-code) / 0.15)',
    borderRadius: '3px',
    padding: '0 3px',
  },
})

export function livePreview() {
  return [livePreviewField, livePreviewTheme]
}
```

- [ ] **Step 4: Run tests**

```bash
cd web && npx vitest run src/__tests__/features/markdown-chat/extensions/live-preview.test.ts
```

Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/markdown-chat/extensions/live-preview.ts \
        web/src/__tests__/features/markdown-chat/extensions/live-preview.test.ts
git commit -m "feat(markdown-chat): add Obsidian-style live preview CM6 extension"
```

---

## Task 7: Streaming Extension

**Files:**
- Create: `web/src/features/markdown-chat/extensions/streaming-ext.ts`
- Create: `web/src/__tests__/features/markdown-chat/extensions/streaming-ext.test.ts`

- [ ] **Step 1: Write the failing tests**

```typescript
// web/src/__tests__/features/markdown-chat/extensions/streaming-ext.test.ts
import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { streamingExt, appendStreamChunk, hasBlinkingCursor } from '@/features/markdown-chat/extensions/streaming-ext'

function makeView(content: string) {
  const state = EditorState.create({
    doc: content,
    extensions: [streamingExt()],
  })
  // EditorView requires a DOM node — use a detached div in jsdom
  const dom = document.createElement('div')
  return new EditorView({ state, parent: dom })
}

test('appendStreamChunk appends text at end of document', () => {
  const view = makeView('Hello')
  appendStreamChunk(view, ' world')
  expect(view.state.doc.toString()).toBe('Hello world')
  view.destroy()
})

test('appendStreamChunk can be called multiple times', () => {
  const view = makeView('')
  appendStreamChunk(view, 'a')
  appendStreamChunk(view, 'b')
  appendStreamChunk(view, 'c')
  expect(view.state.doc.toString()).toBe('abc')
  view.destroy()
})

test('hasBlinkingCursor returns true after appendStreamChunk', () => {
  const view = makeView('Hello')
  appendStreamChunk(view, ' world')
  expect(hasBlinkingCursor(view.state)).toBe(true)
  view.destroy()
})
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd web && npx vitest run src/__tests__/features/markdown-chat/extensions/streaming-ext.test.ts
```

Expected: FAIL.

- [ ] **Step 3: Implement**

```typescript
// web/src/features/markdown-chat/extensions/streaming-ext.ts
import { Annotation, EditorState, StateField } from '@codemirror/state'
import { Decoration, DecorationSet, EditorView, WidgetType } from '@codemirror/view'
import { streamingAnnotation } from './turn-boundaries'

class BlinkingCursorWidget extends WidgetType {
  toDOM() {
    const span = document.createElement('span')
    span.className = 'cm-streaming-cursor'
    return span
  }
  eq() { return true }
}

const blinkingCursorDecoration = Decoration.widget({
  widget: new BlinkingCursorWidget(),
  side: 1,
})

const cursorActiveField = StateField.define<boolean>({
  create: () => false,
  update(active, tr) {
    if (tr.annotation(streamingAnnotation)) return true
    if (tr.annotation(streamingDoneAnnotation)) return false
    return active
  },
})

export const streamingDoneAnnotation = Annotation.define<boolean>()

function buildCursorDeco(state: EditorState): DecorationSet {
  if (!state.field(cursorActiveField)) return Decoration.none
  const pos = state.doc.length
  return Decoration.set([blinkingCursorDecoration.range(pos)])
}

const cursorDecoField = StateField.define<DecorationSet>({
  create: (state) => buildCursorDeco(state),
  update(_deco, tr) {
    return buildCursorDeco(tr.state)
  },
  provide: (f) => EditorView.decorations.from(f),
})

const streamingTheme = EditorView.theme({
  '.cm-streaming-cursor': {
    display: 'inline-block',
    width: '2px',
    height: '1em',
    backgroundColor: 'currentColor',
    verticalAlign: 'text-bottom',
    animation: 'cm-blink 1s step-start infinite',
  },
  '@keyframes cm-blink': {
    '0%, 100%': { opacity: 1 },
    '50%': { opacity: 0 },
  },
})

export function appendStreamChunk(view: EditorView, text: string): void {
  const pos = view.state.doc.length
  view.dispatch({
    changes: { from: pos, insert: text },
    annotations: streamingAnnotation.of(true),
  })
}

export function finalizeStreaming(view: EditorView): void {
  view.dispatch({ annotations: streamingDoneAnnotation.of(true) })
}

export function hasBlinkingCursor(state: EditorState): boolean {
  return state.field(cursorActiveField)
}

export function streamingExt() {
  return [cursorActiveField, cursorDecoField, streamingTheme]
}
```

- [ ] **Step 4: Run tests**

```bash
cd web && npx vitest run src/__tests__/features/markdown-chat/extensions/streaming-ext.test.ts
```

Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/markdown-chat/extensions/streaming-ext.ts \
        web/src/__tests__/features/markdown-chat/extensions/streaming-ext.test.ts
git commit -m "feat(markdown-chat): add streaming extension with blinking cursor"
```

---

## Task 8: Todo Sticky Extension

**Files:**
- Create: `web/src/features/markdown-chat/extensions/todo-sticky.ts`
- Create: `web/src/__tests__/features/markdown-chat/extensions/todo-sticky.test.ts`

- [ ] **Step 1: Write the failing tests**

```typescript
// web/src/__tests__/features/markdown-chat/extensions/todo-sticky.test.ts
import { EditorState } from '@codemirror/state'
import { markdown } from '@codemirror/lang-markdown'
import { todoStickyExt, findTodoBlockRange } from '@/features/markdown-chat/extensions/todo-sticky'

function makeState(content: string) {
  return EditorState.create({
    doc: content,
    extensions: [markdown(), todoStickyExt()],
  })
}

test('findTodoBlockRange returns null when no checklist present', () => {
  const state = makeState('Just some text\nNo checklist here')
  expect(findTodoBlockRange(state)).toBeNull()
})

test('findTodoBlockRange returns range when checklist is present', () => {
  const state = makeState('- [ ] Task one\n- [x] Task two\n- [ ] Task three')
  const range = findTodoBlockRange(state)
  expect(range).not.toBeNull()
  expect(range!.from).toBe(0)
  expect(range!.to).toBeGreaterThan(0)
})

test('findTodoBlockRange finds checklist in middle of content', () => {
  const state = makeState('Preamble text\n\n- [ ] Task one\n- [x] Task two\n\nTrailing text')
  const range = findTodoBlockRange(state)
  expect(range).not.toBeNull()
  // Range should start at the first checklist item
  const line = state.doc.lineAt(range!.from)
  expect(line.text.trim()).toBe('- [ ] Task one')
})
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd web && npx vitest run src/__tests__/features/markdown-chat/extensions/todo-sticky.test.ts
```

Expected: FAIL.

- [ ] **Step 3: Implement**

```typescript
// web/src/features/markdown-chat/extensions/todo-sticky.ts
import { EditorState, StateField, RangeSetBuilder } from '@codemirror/state'
import { Decoration, DecorationSet, EditorView } from '@codemirror/view'

const TODO_ITEM_RE = /^- \[[ x]\] /

export interface TodoRange { from: number; to: number }

export function findTodoBlockRange(state: EditorState): TodoRange | null {
  let from: number | null = null
  let to: number | null = null

  for (let i = 1; i <= state.doc.lines; i++) {
    const line = state.doc.line(i)
    if (TODO_ITEM_RE.test(line.text)) {
      if (from === null) from = line.from
      to = line.to
    } else if (from !== null && line.text.trim() === '') {
      // allow a blank line before trailing text
      continue
    } else if (from !== null) {
      break
    }
  }

  return from !== null && to !== null ? { from, to } : null
}

// The sticky class is applied externally based on streaming state.
// This extension only provides the decoration infrastructure.
const stickyDecoField = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(_deco, tr) {
    if (!tr.docChanged) return _deco
    return Decoration.none // rebuilt by component via updateStickyDeco
  },
  provide: (f) => EditorView.decorations.from(f),
})

export function applyTodoStickyDeco(view: EditorView, isStreaming: boolean): void {
  const range = findTodoBlockRange(view.state)
  if (!range || !isStreaming) {
    // Remove sticky class — nothing to do at CM6 level;
    // the wrapping div in markdown-chat-editor handles the CSS class.
    return
  }
}

export function todoStickyExt() {
  return [stickyDecoField]
}
```

> **Note:** The actual sticky behavior is implemented at the React component level in `markdown-chat-editor.tsx` — a `sticky-active` CSS class is toggled on the todo block's wrapper element based on `turn.streaming`. The CM6 extension provides `findTodoBlockRange` so the editor component can locate the block and apply the class via a portal or DOM manipulation.

- [ ] **Step 4: Run tests**

```bash
cd web && npx vitest run src/__tests__/features/markdown-chat/extensions/todo-sticky.test.ts
```

Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/markdown-chat/extensions/todo-sticky.ts \
        web/src/__tests__/features/markdown-chat/extensions/todo-sticky.test.ts
git commit -m "feat(markdown-chat): add todo sticky extension"
```

---

## Task 9: Widget Registry and Widget Extension

**Files:**
- Create: `web/src/features/markdown-chat/extensions/widget-registry.ts`
- Create: `web/src/features/markdown-chat/extensions/widget-ext.ts`

- [ ] **Step 1: Create widget registry**

```typescript
// web/src/features/markdown-chat/extensions/widget-registry.ts
import type { ReactNode } from 'react'
import type { WidgetData } from '../types'

export interface WidgetComponentProps {
  data: WidgetData
  onChange: (payload: unknown) => void
}

export type WidgetComponent = (props: WidgetComponentProps) => ReactNode

const registry = new Map<string, WidgetComponent>()

export function registerWidget(type: string, component: WidgetComponent): void {
  registry.set(type, component)
}

export function getWidget(type: string): WidgetComponent | undefined {
  return registry.get(type)
}

// Populated by excalidraw-widget.tsx and mermaid-widget.tsx at module load time.
export const WIDGET_REGISTRY: Record<string, WidgetComponent> = {}
```

- [ ] **Step 2: Create widget extension**

```typescript
// web/src/features/markdown-chat/extensions/widget-ext.ts
import { EditorState, StateField, RangeSetBuilder } from '@codemirror/state'
import { Decoration, DecorationSet, EditorView, WidgetType } from '@codemirror/view'
import { createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { syntaxTree } from '@codemirror/language'
import type { MarkdownTurn, ToolCallData } from '../types'
import { WIDGET_ID_RE, TOOL_CALL_RE } from '../types'
import { getWidget } from './widget-registry'
import { ToolCallPill } from '../components/tool-call-pill'

// -- Tool call pill widget --

class ToolCallWidget extends WidgetType {
  constructor(private data: ToolCallData) { super() }
  eq(other: ToolCallWidget) {
    return JSON.stringify(this.data) === JSON.stringify(other.data)
  }
  toDOM() {
    const div = document.createElement('div')
    const root = createRoot(div)
    root.render(createElement(ToolCallPill, { data: this.data }))
    return div
  }
}

// -- Generic (Excalidraw / Mermaid) widget --

class FencedWidget extends WidgetType {
  constructor(
    private widgetId: string,
    private widgetType: string,
    private turns: MarkdownTurn[],
    private onWidgetChange: (widgetId: string, payload: unknown) => void,
  ) { super() }

  eq(other: FencedWidget) {
    return this.widgetId === other.widgetId && this.widgetType === other.widgetType
  }

  toDOM() {
    const div = document.createElement('div')
    div.className = 'cm-widget-container'

    const Component = getWidget(this.widgetType)
    if (!Component) {
      div.textContent = `[unknown widget type: ${this.widgetType}]`
      return div
    }

    const turn = this.turns.find((t) => t.widgets.some((w) => w.id === this.widgetId))
    const widgetData = turn?.widgets.find((w) => w.id === this.widgetId)
    if (!widgetData) return div

    const root = createRoot(div)
    root.render(
      createElement(Component, {
        data: widgetData,
        onChange: (payload) => this.onWidgetChange(this.widgetId, payload),
      }),
    )
    return div
  }
}

function buildWidgetDecorations(
  state: EditorState,
  turns: MarkdownTurn[],
  onWidgetChange: (widgetId: string, payload: unknown) => void,
): DecorationSet {
  const builder = new RangeSetBuilder<Decoration>()

  // Tool call comments
  for (let i = 1; i <= state.doc.lines; i++) {
    const line = state.doc.line(i)
    const toolMatch = line.text.match(TOOL_CALL_RE)
    if (toolMatch) {
      try {
        const data: ToolCallData = JSON.parse(toolMatch[1])
        builder.add(
          line.from,
          line.to,
          Decoration.replace({ widget: new ToolCallWidget(data) }),
        )
      } catch { /* malformed JSON — skip */ }
    }
  }

  // Fenced widget blocks
  syntaxTree(state).cursor().iterate((node) => {
    if (node.name !== 'FencedCode') return
    const infoLine = state.doc.lineAt(node.from).text
    const widgetIdMatch = infoLine.match(WIDGET_ID_RE)
    if (!widgetIdMatch) return

    const widgetId = widgetIdMatch[1]
    const widgetType = infoLine.split(' ')[0].replace('```', '').trim()

    builder.add(
      node.from,
      node.to,
      Decoration.replace({
        widget: new FencedWidget(widgetId, widgetType, turns, onWidgetChange),
        inclusive: true,
      }),
    )
  })

  return builder.finish()
}

// Factory that creates the StateField with access to turns and callback.
export function widgetExt(
  turns: MarkdownTurn[],
  onWidgetChange: (widgetId: string, payload: unknown) => void,
) {
  const widgetDecoField = StateField.define<DecorationSet>({
    create: (state) => buildWidgetDecorations(state, turns, onWidgetChange),
    update(_deco, tr) {
      if (!tr.docChanged) return _deco
      return buildWidgetDecorations(tr.state, turns, onWidgetChange)
    },
    provide: (f) => EditorView.decorations.from(f),
  })

  return [widgetDecoField]
}
```

- [ ] **Step 3: Verify TypeScript**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "widget" | head -10
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add web/src/features/markdown-chat/extensions/widget-registry.ts \
        web/src/features/markdown-chat/extensions/widget-ext.ts
git commit -m "feat(markdown-chat): add widget registry and CM6 widget extension"
```

---

## Task 10: Tool Call Pill Component

**Files:**
- Create: `web/src/features/markdown-chat/components/tool-call-pill.tsx`

- [ ] **Step 1: Implement the component**

```typescript
// web/src/features/markdown-chat/components/tool-call-pill.tsx
import { useState } from 'react'
import type { ToolCallData } from '../types'

interface ToolCallPillProps {
  data: ToolCallData
}

const STATUS_COLORS: Record<ToolCallData['status'], string> = {
  pending: 'bg-yellow-100 text-yellow-800',
  done: 'bg-blue-100 text-blue-800',
  error: 'bg-red-100 text-red-800',
}

export function ToolCallPill({ data }: ToolCallPillProps) {
  const [expanded, setExpanded] = useState(false)

  return (
    <div className="my-1 inline-flex flex-col gap-1">
      <button
        onClick={() => setExpanded((v) => !v)}
        className="inline-flex items-center gap-1.5 rounded-full border border-border bg-muted px-2.5 py-0.5 text-xs text-muted-foreground transition-colors hover:bg-muted/80"
      >
        <span className="text-muted-foreground/60">{expanded ? '▾' : '▸'}</span>
        <span className="font-mono font-medium">{data.name}</span>
        {data.args.path && (
          <span className="text-muted-foreground/70 truncate max-w-48">
            {String(data.args.path)}
          </span>
        )}
        <span
          className={`rounded px-1 py-0.5 text-[10px] font-semibold ${STATUS_COLORS[data.status]}`}
        >
          {data.status}
        </span>
      </button>

      {expanded && (
        <div className="ml-3 rounded border border-border bg-card p-2 text-xs font-mono">
          <div className="mb-1 text-muted-foreground/60">args</div>
          <pre className="whitespace-pre-wrap text-foreground">
            {JSON.stringify(data.args, null, 2)}
          </pre>
          {data.output && (
            <>
              <div className="mb-1 mt-2 text-muted-foreground/60">output</div>
              <pre className="max-h-40 overflow-auto whitespace-pre-wrap text-foreground">
                {data.output}
              </pre>
            </>
          )}
        </div>
      )}
    </div>
  )
}
```

- [ ] **Step 2: Verify TypeScript**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "tool-call-pill" | head -5
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/markdown-chat/components/tool-call-pill.tsx
git commit -m "feat(markdown-chat): add collapsible tool call pill component"
```

---

## Task 11: Mermaid Widget

**Files:**
- Create: `web/src/features/markdown-chat/components/mermaid-widget.tsx`

- [ ] **Step 1: Implement**

```typescript
// web/src/features/markdown-chat/components/mermaid-widget.tsx
import { useEffect, useRef, useState } from 'react'
import mermaid from 'mermaid'
import type { WidgetComponentProps } from '../extensions/widget-registry'

mermaid.initialize({ startOnLoad: false, theme: 'neutral' })

let mermaidId = 0

export function MermaidWidget({ data }: WidgetComponentProps) {
  const source = typeof data.payload === 'string' ? data.payload : ''
  const [svg, setSvg] = useState<string>('')
  const [error, setError] = useState<string>('')
  const idRef = useRef(`mermaid-${++mermaidId}`)

  useEffect(() => {
    if (!source.trim()) return
    mermaid
      .render(idRef.current, source)
      .then(({ svg: rendered }) => {
        setSvg(rendered)
        setError('')
      })
      .catch((err: Error) => {
        setError(err.message)
      })
  }, [source])

  if (error) {
    return (
      <div className="rounded border border-destructive/30 bg-destructive/10 p-2 text-xs text-destructive">
        Mermaid error: {error}
      </div>
    )
  }

  if (!svg) {
    return (
      <div className="h-16 animate-pulse rounded border border-border bg-muted" />
    )
  }

  return (
    <div
      className="overflow-auto rounded border border-border bg-card p-2"
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  )
}
```

- [ ] **Step 2: Register in widget registry**

Add to the bottom of `widget-registry.ts`:

```typescript
// At the end of web/src/features/markdown-chat/extensions/widget-registry.ts
// Registration happens at import time.
import { MermaidWidget } from '../components/mermaid-widget'
WIDGET_REGISTRY['mermaid'] = MermaidWidget
```

Wait — circular imports. Instead, register widgets in `markdown-chat-editor.tsx` on mount (Task 14). Leave `WIDGET_REGISTRY` as an empty object for now; Task 14 populates it.

- [ ] **Step 3: Verify TypeScript**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "mermaid-widget" | head -5
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add web/src/features/markdown-chat/components/mermaid-widget.tsx
git commit -m "feat(markdown-chat): add read-only Mermaid SVG widget"
```

---

## Task 12: Excalidraw Widget

**Files:**
- Create: `web/src/features/markdown-chat/components/excalidraw-widget.tsx`

- [ ] **Step 1: Implement**

```typescript
// web/src/features/markdown-chat/components/excalidraw-widget.tsx
import { useCallback } from 'react'
import { Excalidraw } from '@excalidraw/excalidraw'
import type { ExcalidrawElement } from '@excalidraw/excalidraw/types/element/types'
import type { AppState } from '@excalidraw/excalidraw/types/types'
import type { WidgetComponentProps } from '../extensions/widget-registry'

interface ExcalidrawPayload {
  elements: readonly ExcalidrawElement[]
  appState: Partial<AppState>
}

export function ExcalidrawWidget({ data, onChange }: WidgetComponentProps) {
  const payload = data.payload as ExcalidrawPayload | null

  const handleChange = useCallback(
    (elements: readonly ExcalidrawElement[], appState: AppState) => {
      onChange({ elements, appState } satisfies ExcalidrawPayload)
    },
    [onChange],
  )

  return (
    <div className="relative h-80 w-full overflow-hidden rounded border border-border">
      <Excalidraw
        initialData={
          payload
            ? { elements: payload.elements, appState: payload.appState }
            : undefined
        }
        onChange={handleChange}
        UIOptions={{ canvasActions: { export: false, loadScene: false } }}
      />
    </div>
  )
}
```

- [ ] **Step 2: Verify TypeScript**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "excalidraw-widget" | head -5
```

Expected: no errors. If Excalidraw type imports fail, check `@excalidraw/excalidraw` version and adjust import paths accordingly.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/markdown-chat/components/excalidraw-widget.tsx
git commit -m "feat(markdown-chat): add interactive Excalidraw widget"
```

---

## Task 13: Slash Command Extension + Palette

**Files:**
- Create: `web/src/features/markdown-chat/extensions/slash-command-ext.ts`
- Create: `web/src/features/markdown-chat/components/slash-command-palette.tsx`

- [ ] **Step 1: Create the palette component**

```typescript
// web/src/features/markdown-chat/components/slash-command-palette.tsx
import { useState, useEffect, useRef } from 'react'

export interface SlashCommand {
  id: string
  label: string
  description: string
  icon?: string
}

// The real list — extend as new skills are added.
export const SLASH_COMMANDS: SlashCommand[] = [
  { id: '/tdd', label: '/tdd', description: 'Test-driven development workflow', icon: '🧪' },
  { id: '/code-review', label: '/code-review', description: 'Review current branch', icon: '🔍' },
  { id: '/plan', label: '/plan', description: 'Write an implementation plan', icon: '📋' },
  { id: '/debug', label: '/debug', description: 'Systematic debugging', icon: '🐛' },
  { id: '/explain', label: '/explain', description: 'Explain selected code', icon: '💬' },
]

interface SlashCommandPaletteProps {
  query: string
  onSelect: (command: SlashCommand) => void
  onClose: () => void
  anchorRect: DOMRect
}

export function SlashCommandPalette({ query, onSelect, onClose, anchorRect }: SlashCommandPaletteProps) {
  const [activeIdx, setActiveIdx] = useState(0)
  const filtered = SLASH_COMMANDS.filter(
    (c) => c.label.toLowerCase().includes(query.toLowerCase()),
  )

  const ref = useRef<HTMLDivElement>(null)

  // Reset active index when filter changes
  useEffect(() => { setActiveIdx(0) }, [query])

  // Keyboard navigation
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'ArrowDown') { e.preventDefault(); setActiveIdx((i) => Math.min(i + 1, filtered.length - 1)) }
      if (e.key === 'ArrowUp') { e.preventDefault(); setActiveIdx((i) => Math.max(i - 1, 0)) }
      if (e.key === 'Enter') { e.preventDefault(); if (filtered[activeIdx]) onSelect(filtered[activeIdx]) }
      if (e.key === 'Escape') { e.preventDefault(); onClose() }
    }
    window.addEventListener('keydown', handler, true)
    return () => window.removeEventListener('keydown', handler, true)
  }, [filtered, activeIdx, onSelect, onClose])

  if (filtered.length === 0) return null

  return (
    <div
      ref={ref}
      className="fixed z-50 w-72 rounded-lg border border-border bg-popover shadow-lg"
      style={{ top: anchorRect.top - 8, left: anchorRect.left, transform: 'translateY(-100%)' }}
    >
      {filtered.map((cmd, i) => (
        <button
          key={cmd.id}
          className={`flex w-full items-start gap-2 px-3 py-2 text-left text-sm transition-colors hover:bg-muted ${i === activeIdx ? 'bg-muted' : ''}`}
          onClick={() => onSelect(cmd)}
        >
          {cmd.icon && <span className="mt-0.5 text-base leading-none">{cmd.icon}</span>}
          <div>
            <div className="font-mono font-medium text-foreground">{cmd.label}</div>
            <div className="text-xs text-muted-foreground">{cmd.description}</div>
          </div>
        </button>
      ))}
    </div>
  )
}
```

- [ ] **Step 2: Create slash command extension**

```typescript
// web/src/features/markdown-chat/extensions/slash-command-ext.ts
import { EditorView, keymap } from '@codemirror/view'
import { Prec } from '@codemirror/state'

export interface SlashCommandState {
  open: boolean
  query: string
  anchorRect: DOMRect | null
}

export type SlashCommandListener = (state: SlashCommandState) => void

export function slashCommandExt(onStateChange: SlashCommandListener) {
  let currentQuery = ''
  let paletteOpen = false

  function close(view: EditorView) {
    paletteOpen = false
    currentQuery = ''
    onStateChange({ open: false, query: '', anchorRect: null })
    return true
  }

  function updateQuery(view: EditorView, ch: string) {
    if (ch === '/') {
      // Check if at start of line or after whitespace
      const { from } = view.state.selection.main
      const lineText = view.state.doc.lineAt(from).text
      const col = from - view.state.doc.lineAt(from).from
      const prevChar = lineText[col - 1] ?? ''
      if (col === 0 || prevChar === ' ') {
        paletteOpen = true
        currentQuery = ''
        const coords = view.coordsAtPos(from) ?? { top: 0, left: 0, bottom: 0, right: 0 }
        const rect = new DOMRect(coords.left, coords.top, 0, coords.bottom - coords.top)
        onStateChange({ open: true, query: '', anchorRect: rect })
        return false // let the character through
      }
    }

    if (paletteOpen) {
      if (ch === ' ' || ch === '\n') {
        close(view)
      } else {
        currentQuery += ch
        const { from } = view.state.selection.main
        const coords = view.coordsAtPos(from) ?? { top: 0, left: 0, bottom: 0, right: 0 }
        const rect = new DOMRect(coords.left, coords.top, 0, coords.bottom - coords.top)
        onStateChange({ open: true, query: currentQuery, anchorRect: rect })
      }
    }

    return false // never consume the character
  }

  return [
    EditorView.domEventHandlers({
      keydown(e, view) {
        if (e.key === 'Escape' && paletteOpen) {
          close(view)
          return true
        }
        return false
      },
    }),
    Prec.high(
      keymap.of([
        {
          key: '/',
          run(view) { updateQuery(view, '/'); return false },
        },
      ]),
    ),
    // Listen to input events for query building
    EditorView.updateListener.of((update) => {
      if (!paletteOpen || !update.docChanged) return
      // Detect backspace wiping the /
      const { from } = update.state.selection.main
      const line = update.state.doc.lineAt(from)
      const col = from - line.from
      const text = line.text.slice(0, col)
      const slashIdx = text.lastIndexOf('/')
      if (slashIdx === -1) {
        close(update.view)
      } else {
        currentQuery = text.slice(slashIdx + 1)
        const coords = update.view.coordsAtPos(from) ?? { top: 0, left: 0, bottom: 0, right: 0 }
        const rect = new DOMRect(coords.left, coords.top, 0, coords.bottom - coords.top)
        onStateChange({ open: true, query: currentQuery, anchorRect: rect })
      }
    }),
  ]
}
```

- [ ] **Step 3: Verify TypeScript**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "slash-command" | head -5
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add web/src/features/markdown-chat/extensions/slash-command-ext.ts \
        web/src/features/markdown-chat/components/slash-command-palette.tsx
git commit -m "feat(markdown-chat): add slash command extension and palette"
```

---

## Task 14: Toolbar Component

**Files:**
- Create: `web/src/features/markdown-chat/components/markdown-chat-toolbar.tsx`

- [ ] **Step 1: Implement**

```typescript
// web/src/features/markdown-chat/components/markdown-chat-toolbar.tsx
import type { EditorView } from '@codemirror/view'
import { nanoid } from 'nanoid'
import type { MarkdownTurn } from '../types'

const CODE_LANGUAGES = [
  'typescript', 'javascript', 'python', 'go', 'shell', 'json', 'plain',
] as const

type CodeLanguage = (typeof CODE_LANGUAGES)[number]

interface ToolbarProps {
  editorView: EditorView | null
  onInsertWidget: (widgetType: string, widgetId: string) => void
}

function wrapSelection(view: EditorView, syntax: string) {
  const { from, to } = view.state.selection.main
  const selected = view.state.sliceDoc(from, to)
  view.dispatch({
    changes: { from, to, insert: `${syntax}${selected}${syntax}` },
    selection: { anchor: from + syntax.length, head: to + syntax.length },
  })
  view.focus()
}

function prependLine(view: EditorView, prefix: string) {
  const { from } = view.state.selection.main
  const line = view.state.doc.lineAt(from)
  const already = line.text.startsWith(prefix)
  view.dispatch({
    changes: already
      ? { from: line.from, to: line.from + prefix.length, insert: '' }
      : { from: line.from, insert: prefix },
  })
  view.focus()
}

function insertBlock(view: EditorView, content: string) {
  const { from } = view.state.selection.main
  view.dispatch({ changes: { from, insert: `\n${content}\n` } })
  view.focus()
}

function insertExcalidraw(view: EditorView, widgetId: string) {
  insertBlock(view, `\`\`\`excalidraw widget-id:${widgetId}\n\`\`\``)
}

function insertCodeBlock(view: EditorView, lang: CodeLanguage) {
  insertBlock(view, `\`\`\`${lang}\n\n\`\`\``)
}

function insertMermaid(view: EditorView) {
  insertBlock(view, '```mermaid\nflowchart LR\n    A --> B\n```')
}

export function MarkdownChatToolbar({ editorView, onInsertWidget }: ToolbarProps) {
  const v = editorView

  return (
    <div className="flex items-center gap-0.5 border-b border-border px-2 py-1">
      {/* Formatting group */}
      <ToolbarButton title="Bold (Cmd+B)" onClick={() => v && wrapSelection(v, '**')}>
        <b>B</b>
      </ToolbarButton>
      <ToolbarButton title="Italic (Cmd+I)" onClick={() => v && wrapSelection(v, '*')}>
        <i>I</i>
      </ToolbarButton>
      <ToolbarButton title="Inline code" onClick={() => v && wrapSelection(v, '`')}>
        <span className="font-mono text-xs">`·`</span>
      </ToolbarButton>

      <div className="mx-1 h-4 w-px bg-border" />

      <ToolbarButton title="Heading 1" onClick={() => v && prependLine(v, '# ')}>
        H1
      </ToolbarButton>
      <ToolbarButton title="Heading 2" onClick={() => v && prependLine(v, '## ')}>
        H2
      </ToolbarButton>
      <ToolbarButton title="Heading 3" onClick={() => v && prependLine(v, '### ')}>
        H3
      </ToolbarButton>

      <div className="mx-1 h-4 w-px bg-border" />

      {/* Insert dropdown */}
      <div className="relative">
        <InsertDropdown
          onInsertExcalidraw={() => {
            if (!v) return
            const id = nanoid()
            onInsertWidget('excalidraw', id)
            insertExcalidraw(v, id)
          }}
          onInsertCodeBlock={(lang) => v && insertCodeBlock(v, lang)}
          onInsertMermaid={() => v && insertMermaid(v)}
        />
      </div>
    </div>
  )
}

function ToolbarButton({
  children,
  onClick,
  title,
}: {
  children: React.ReactNode
  onClick: () => void
  title: string
}) {
  return (
    <button
      title={title}
      onClick={onClick}
      className="flex h-6 min-w-6 items-center justify-center rounded px-1 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
    >
      {children}
    </button>
  )
}

function InsertDropdown({
  onInsertExcalidraw,
  onInsertCodeBlock,
  onInsertMermaid,
}: {
  onInsertExcalidraw: () => void
  onInsertCodeBlock: (lang: CodeLanguage) => void
  onInsertMermaid: () => void
}) {
  const [open, setOpen] = useState(false)
  const [codeOpen, setCodeOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
        setCodeOpen(false)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [])

  return (
    <div ref={ref}>
      <ToolbarButton title="Insert block" onClick={() => setOpen((v) => !v)}>
        + Insert
      </ToolbarButton>
      {open && (
        <div className="absolute left-0 top-full z-50 mt-1 w-44 rounded-md border border-border bg-popover shadow-md">
          <MenuItem
            onClick={() => { onInsertExcalidraw(); setOpen(false) }}
          >
            ✏️ Excalidraw drawing
          </MenuItem>
          <div className="relative">
            <MenuItem onClick={() => setCodeOpen((v) => !v)}>
              {'<>'} Code block ▸
            </MenuItem>
            {codeOpen && (
              <div className="absolute left-full top-0 w-36 rounded-md border border-border bg-popover shadow-md">
                {CODE_LANGUAGES.map((lang) => (
                  <MenuItem
                    key={lang}
                    onClick={() => { onInsertCodeBlock(lang); setOpen(false); setCodeOpen(false) }}
                  >
                    {lang}
                  </MenuItem>
                ))}
              </div>
            )}
          </div>
          <MenuItem onClick={() => { onInsertMermaid(); setOpen(false) }}>
            📊 Mermaid diagram
          </MenuItem>
        </div>
      )}
    </div>
  )
}

function MenuItem({ children, onClick }: { children: React.ReactNode; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm text-foreground transition-colors hover:bg-muted"
    >
      {children}
    </button>
  )
}
```

Add the missing React imports at the top of the file:

```typescript
import { useState, useRef, useEffect } from 'react'
```

- [ ] **Step 2: Verify TypeScript**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "markdown-chat-toolbar" | head -5
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/markdown-chat/components/markdown-chat-toolbar.tsx
git commit -m "feat(markdown-chat): add docked toolbar with formatting and insert dropdown"
```

---

## Task 15: CM6 Editor Host Component

**Files:**
- Create: `web/src/features/markdown-chat/components/markdown-chat-editor.tsx`

- [ ] **Step 1: Implement**

```typescript
// web/src/features/markdown-chat/components/markdown-chat-editor.tsx
import { useEffect, useRef, useState, useCallback } from 'react'
import { EditorState, Compartment } from '@codemirror/state'
import { EditorView, keymap, lineNumbers } from '@codemirror/view'
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
import { markdown } from '@codemirror/lang-markdown'
import { ExcalidrawWidget } from './excalidraw-widget'
import { MermaidWidget } from './mermaid-widget'
import { WIDGET_REGISTRY } from '../extensions/widget-registry'
import { turnBoundaries, turnsToDocument } from '../extensions/turn-boundaries'
import { livePreview } from '../extensions/live-preview'
import { streamingExt, appendStreamChunk, finalizeStreaming } from '../extensions/streaming-ext'
import { todoStickyExt } from '../extensions/todo-sticky'
import { widgetExt } from '../extensions/widget-ext'
import { slashCommandExt } from '../extensions/slash-command-ext'
import { SlashCommandPalette, type SlashCommand } from './slash-command-palette'
import type { MarkdownTurn } from '../types'
import type { SlashCommandState } from '../extensions/slash-command-ext'

// Register widgets once at module load time.
WIDGET_REGISTRY['excalidraw'] = ExcalidrawWidget
WIDGET_REGISTRY['mermaid'] = MermaidWidget

interface MarkdownChatEditorProps {
  turns: MarkdownTurn[]
  streamingTurnId: string | null
  onSubmit: (content: string) => void
  onWidgetChange: (widgetId: string, payload: unknown) => void
  onSlashCommand: (command: SlashCommand) => void
  onEditorReady: (view: EditorView) => void
}

export function MarkdownChatEditor({
  turns,
  streamingTurnId,
  onSubmit,
  onWidgetChange,
  onSlashCommand,
  onEditorReady,
}: MarkdownChatEditorProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const [slashState, setSlashState] = useState<SlashCommandState>({
    open: false,
    query: '',
    anchorRect: null,
  })

  const handleSlashCommand = useCallback(
    (cmd: SlashCommand) => {
      // Remove the /query from the input before dispatching
      const view = viewRef.current
      if (view) {
        const { from } = view.state.selection.main
        const line = view.state.doc.lineAt(from)
        const slashIdx = line.text.lastIndexOf('/')
        if (slashIdx !== -1) {
          view.dispatch({
            changes: {
              from: line.from + slashIdx,
              to: from,
              insert: cmd.id,
            },
          })
        }
      }
      setSlashState({ open: false, query: '', anchorRect: null })
      onSlashCommand(cmd)
    },
    [onSlashCommand],
  )

  useEffect(() => {
    if (!containerRef.current) return

    const docContent = turns.length > 0 ? turnsToDocument(turns) : ''

    const submitKeymap = keymap.of([
      {
        key: 'Mod-Enter',
        run(view) {
          const content = view.state.doc.toString()
          // Find the last turn boundary to extract user input
          const lines = content.split('\n')
          const lastMarkerIdx = lines.map((l, i) => ({ l, i }))
            .filter(({ l }) => /^<!-- turn:/.test(l))
            .at(-1)?.i ?? -1
          const userInput = lines.slice(lastMarkerIdx + 1).join('\n').trim()
          if (userInput) onSubmit(userInput)
          return true
        },
      },
    ])

    const state = EditorState.create({
      doc: docContent,
      extensions: [
        markdown(),
        history(),
        keymap.of([...defaultKeymap, ...historyKeymap]),
        submitKeymap,
        turnBoundaries(streamingTurnId),
        livePreview(),
        streamingExt(),
        todoStickyExt(),
        widgetExt(turns, onWidgetChange),
        slashCommandExt(setSlashState),
        EditorView.theme({
          '&': { height: '100%', fontSize: '14px' },
          '.cm-scroller': { overflow: 'auto', fontFamily: 'var(--font-sans)' },
          '.cm-content': { padding: '16px', minHeight: '100%' },
          '.cm-line': { lineHeight: '1.7' },
        }),
        EditorView.lineWrapping,
      ],
    })

    const view = new EditorView({ state, parent: containerRef.current })
    viewRef.current = view
    onEditorReady(view)

    return () => {
      view.destroy()
      viewRef.current = null
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []) // Mount once — turns updates handled imperatively via appendStreamChunk

  return (
    <div className="relative flex h-full flex-col">
      <div ref={containerRef} className="flex-1 overflow-hidden" />
      {slashState.open && slashState.anchorRect && (
        <SlashCommandPalette
          query={slashState.query}
          anchorRect={slashState.anchorRect}
          onSelect={handleSlashCommand}
          onClose={() => setSlashState({ open: false, query: '', anchorRect: null })}
        />
      )}
    </div>
  )
}
```

- [ ] **Step 2: Verify TypeScript**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "markdown-chat-editor" | head -10
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/markdown-chat/components/markdown-chat-editor.tsx
git commit -m "feat(markdown-chat): add CM6 editor host with all extensions wired"
```

---

## Task 16: Root View + MSW Handler

**Files:**
- Create: `web/src/features/markdown-chat/components/markdown-chat-view.tsx`
- Create: `web/src/mocks/handlers/markdown-chat.ts`

- [ ] **Step 1: Implement the root view**

```typescript
// web/src/features/markdown-chat/components/markdown-chat-view.tsx
import { useEffect, useRef, useCallback } from 'react'
import { nanoid } from 'nanoid'
import type { EditorView } from '@codemirror/view'
import { appendStreamChunk, finalizeStreaming } from '../extensions/streaming-ext'
import { getOrCreateConversationStore } from '../stores/conversation-store'
import { getMockMarkdownTurns, simulateMarkdownStream } from '@/lib/mock/markdown-chat'
import { MarkdownChatEditor } from './markdown-chat-editor'
import { MarkdownChatToolbar } from './markdown-chat-toolbar'
import type { SlashCommand } from './slash-command-palette'

interface MarkdownChatViewProps {
  workspaceId: string
  stepId: string
}

const STEP_GREETINGS: Record<string, string> = {
  brainstorm: "I'm ready to brainstorm. What do you want to build?",
  spec: 'Let\'s refine the spec. What would you like to discuss?',
  build: 'Ready to implement. What should we tackle first?',
  ai_review: 'I\'ve reviewed the diff. Here\'s what I found.',
  human_review: 'Waiting for your review comments.',
}

export function MarkdownChatView({ workspaceId, stepId }: MarkdownChatViewProps) {
  const store = getOrCreateConversationStore(workspaceId)
  const turns = store((s) => s.turns)
  const editorViewRef = useRef<EditorView | null>(null)
  const cancelStreamRef = useRef<(() => void) | null>(null)

  // Seed mock turns on mount
  useEffect(() => {
    const existing = store.getState().turns
    if (existing.length === 0) {
      const mockTurns = getMockMarkdownTurns(workspaceId, stepId)
      if (mockTurns.length > 0) {
        mockTurns.forEach((t) => store.getState().appendTurn(t))
      } else {
        const greeting = STEP_GREETINGS[stepId]
        if (greeting) {
          store.getState().appendTurn({
            id: nanoid(),
            role: 'agent',
            content: greeting,
            timestamp: new Date().toISOString(),
            authorName: 'Claude',
            widgets: [],
          })
        }
      }
    }
    return () => { cancelStreamRef.current?.() }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspaceId, stepId])

  const handleSubmit = useCallback(
    (content: string) => {
      cancelStreamRef.current?.()

      // Append user turn
      store.getState().appendTurn({
        id: nanoid(),
        role: 'user',
        content,
        timestamp: new Date().toISOString(),
        authorName: 'You',
        widgets: [],
      })

      // Start streaming agent response
      const agentId = nanoid()
      store.getState().appendTurn({
        id: agentId,
        role: 'agent',
        content: '',
        timestamp: new Date().toISOString(),
        authorName: 'Claude',
        widgets: [],
        streaming: true,
      })

      const MOCK_RESPONSE =
        'Great point. Let me think through this carefully. ' +
        'There are several considerations here that we should address:\n\n' +
        '1. **Performance** — the current approach has O(n²) complexity\n' +
        '2. **Correctness** — edge cases around empty inputs\n' +
        '3. **Maintainability** — the code is hard to follow\n\n' +
        'My recommendation is to refactor the core loop first.'

      cancelStreamRef.current = simulateMarkdownStream(
        MOCK_RESPONSE,
        (chunk) => {
          store.getState().updateStreamingTurn(agentId, chunk)
          if (editorViewRef.current) appendStreamChunk(editorViewRef.current, chunk)
        },
        () => {
          store.getState().finalizeStreamingTurn(agentId)
          if (editorViewRef.current) finalizeStreaming(editorViewRef.current)
          cancelStreamRef.current = null
        },
      )
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [store],
  )

  const handleWidgetChange = useCallback(
    (widgetId: string, payload: unknown) => {
      const { turns: currentTurns } = store.getState()
      const turn = currentTurns.find((t) => t.widgets.some((w) => w.id === widgetId))
      if (turn) store.getState().updateWidgetPayload(turn.id, widgetId, payload)
    },
    [store],
  )

  const handleInsertWidget = useCallback(
    (widgetType: string, widgetId: string) => {
      // Add widget data to the pending input turn (last turn or a new one)
      const state = store.getState()
      const lastTurn = state.turns.at(-1)
      if (lastTurn?.role === 'user') {
        state.updateWidgetPayload(lastTurn.id, widgetId, null)
      }
    },
    [store],
  )

  const handleSlashCommand = useCallback((cmd: SlashCommand) => {
    // For now, append the command as a user message and trigger stream
    handleSubmit(cmd.id)
  }, [handleSubmit])

  const streamingTurnId = turns.find((t) => t.streaming)?.id ?? null

  if (turns.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <p className="text-sm">Loading conversation…</p>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <MarkdownChatEditor
        turns={turns}
        streamingTurnId={streamingTurnId}
        onSubmit={handleSubmit}
        onWidgetChange={handleWidgetChange}
        onSlashCommand={handleSlashCommand}
        onEditorReady={(view) => { editorViewRef.current = view }}
      />
      <MarkdownChatToolbar
        editorView={editorViewRef.current}
        onInsertWidget={handleInsertWidget}
      />
    </div>
  )
}
```

- [ ] **Step 2: Create MSW handler**

First, check the MSW handlers directory:

```bash
ls web/src/mocks/handlers/
```

Then create the handler:

```typescript
// web/src/mocks/handlers/markdown-chat.ts
import { http, HttpResponse } from 'msw'
import { getMockMarkdownTurns } from '@/lib/mock/markdown-chat'

export const markdownChatHandlers = [
  http.get('/api/v0/conversations/:wsId/:stepId', ({ params }) => {
    const turns = getMockMarkdownTurns(
      String(params.wsId),
      String(params.stepId),
    )
    return HttpResponse.json(turns)
  }),
]
```

- [ ] **Step 3: Register handler in MSW browser entry**

Find the MSW browser entry and add the new handler:

```bash
grep -rn "handlers" web/src/mocks/browser.ts 2>/dev/null || grep -rn "setupWorker\|handlers" web/src/mocks/ | head -10
```

Add `markdownChatHandlers` to the handlers array in the browser setup file (exact path may vary — follow what you find in the grep above).

- [ ] **Step 4: Verify TypeScript**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "markdown-chat-view\|markdown-chat.ts" | head -10
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/markdown-chat/components/markdown-chat-view.tsx \
        web/src/mocks/handlers/markdown-chat.ts
git commit -m "feat(markdown-chat): add root MarkdownChatView and MSW handler"
```

---

## Task 17: Integration — Swap ChatView, Delete Old Components

**Files:**
- Modify: `web/src/features/workflow/components/flow-content.tsx`
- Delete: `web/src/features/workflow/components/chat-view.tsx`
- Delete: `web/src/components/chat/ChatView.tsx`
- Delete: `web/src/components/chat/MessageBubble.tsx`
- Delete: `web/src/components/chat/ChatInput.tsx`
- Delete: `web/src/components/chat/ChatEmptyState.tsx`
- Delete: `web/src/components/chat/ToolCallSeparator.tsx`

- [ ] **Step 1: Update flow-content.tsx**

Open `web/src/features/workflow/components/flow-content.tsx` and replace the `ChatView` import and usage:

```typescript
// web/src/features/workflow/components/flow-content.tsx
import { useCurrentStep } from '@/features/workspace/stores/hooks/use-workflow'
import { MarkdownChatView } from '@/features/markdown-chat/components/markdown-chat-view'
import { DiffView } from './diff-view'
import { SplitView } from './split-view'

interface FlowContentProps {
  workspaceId: string
}

export function FlowContent({ workspaceId }: FlowContentProps) {
  const currentStep = useCurrentStep()

  if (!currentStep) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <p className="text-sm">No active step</p>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      {currentStep.contentType === 'chat' && (
        <MarkdownChatView workspaceId={workspaceId} stepId={currentStep.id} />
      )}
      {currentStep.contentType === 'diff' && (
        <DiffView workspaceId={workspaceId} stepId={currentStep.id} />
      )}
      {currentStep.contentType === 'split' && (
        <SplitView workspaceId={workspaceId} stepId={currentStep.id} />
      )}
    </div>
  )
}
```

- [ ] **Step 2: Delete old chat files**

```bash
git rm web/src/features/workflow/components/chat-view.tsx \
       web/src/components/chat/ChatView.tsx \
       web/src/components/chat/MessageBubble.tsx \
       web/src/components/chat/ChatInput.tsx \
       web/src/components/chat/ChatEmptyState.tsx \
       web/src/components/chat/ToolCallSeparator.tsx
```

- [ ] **Step 3: Check for remaining references to deleted files**

```bash
cd web && grep -r "from '@/components/chat\|from '../chat\|from './chat" src --include="*.ts" --include="*.tsx" | grep -v "MarkdownContent"
```

Expected: no output. If output appears, update those imports.

- [ ] **Step 4: TypeScript clean build**

```bash
cd web && npx tsc --noEmit 2>&1 | head -30
```

Expected: no errors. Fix any type errors before proceeding.

- [ ] **Step 5: Run full test suite**

```bash
cd web && npx vitest run
```

Expected: all tests pass (the old `ChatView.test.tsx` will need to be deleted or updated):

```bash
# Delete the old chat component tests
git rm web/src/__tests__/components/chat/ChatView.test.tsx 2>/dev/null || true
```

Run tests again:

```bash
cd web && npx vitest run
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/workflow/components/flow-content.tsx
git commit -m "feat(markdown-chat): swap ChatView for MarkdownChatView, delete old chat components"
```

---

## Task 18: Smoke Test + Demo Verification

**Files:**
- Create: `web/src/__tests__/features/markdown-chat/components/markdown-chat-view.test.tsx`

- [ ] **Step 1: Write smoke test**

```typescript
// web/src/__tests__/features/markdown-chat/components/markdown-chat-view.test.tsx
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { vi } from 'vitest'
import { MarkdownChatView } from '@/features/markdown-chat/components/markdown-chat-view'
import { destroyConversationStore } from '@/features/markdown-chat/stores/conversation-store'

// CM6 requires layout — mock EditorView
vi.mock('@codemirror/view', async (importOriginal) => {
  const mod = await importOriginal<typeof import('@codemirror/view')>()
  return {
    ...mod,
    EditorView: class {
      state = { doc: { toString: () => '', length: 0, lines: 0, line: () => ({ text: '', from: 0, to: 0 }) }, selection: { main: { head: 0, from: 0, to: 0 } }, field: () => [] }
      dispatch = vi.fn()
      destroy = vi.fn()
      coordsAtPos = vi.fn(() => ({ top: 0, left: 0, bottom: 0, right: 0 }))
      constructor({ parent }: { parent: Element }) {
        parent.innerHTML = '<div class="cm-editor"><div class="cm-content" contenteditable="true"></div></div>'
      }
    },
  }
})

afterEach(() => {
  destroyConversationStore('ws-test')
})

test('renders without crashing', () => {
  render(<MarkdownChatView workspaceId="ws-test" stepId="brainstorm" />)
  // The editor mounts; loading state may show briefly
  expect(document.querySelector('.cm-editor') ?? screen.queryByText('Loading')).toBeTruthy()
})

test('shows greeting for brainstorm step when no mock data', async () => {
  render(<MarkdownChatView workspaceId="ws-unknown" stepId="brainstorm" />)
  await waitFor(() => {
    expect(screen.queryByText('Loading conversation…')).toBeNull()
  })
  // Greeting turn is appended to store
  const { getOrCreateConversationStore } = await import('@/features/markdown-chat/stores/conversation-store')
  const store = getOrCreateConversationStore('ws-unknown')
  expect(store.getState().turns[0]?.role).toBe('agent')
})
```

- [ ] **Step 2: Run the smoke test**

```bash
cd web && npx vitest run src/__tests__/features/markdown-chat/components/markdown-chat-view.test.tsx
```

Expected: both tests pass.

- [ ] **Step 3: Start dev server and verify the demo**

```bash
cd web && npm run dev
```

Open `http://localhost:5173` in a browser. Navigate to a workspace with a chat pane. Verify:

- [ ] The document renders with existing turns (mock data for `ws3:brainstorm`)
- [ ] User turns have a slightly different tint from agent turns
- [ ] Heading syntax (`# Heading`) live-previews as styled text when cursor moves away
- [ ] Bold syntax (`**text**`) live-previews
- [ ] Typing `/` in the input opens the slash command palette
- [ ] Pressing `Cmd+Enter` submits the input and triggers a mock streaming response
- [ ] Streaming shows a blinking cursor at the end
- [ ] After streaming, tool call pills are visible and expand on click
- [ ] The Insert dropdown in the toolbar allows inserting a code block

- [ ] **Step 4: Final commit**

```bash
git add web/src/__tests__/features/markdown-chat/components/markdown-chat-view.test.tsx
git commit -m "test(markdown-chat): add smoke test for MarkdownChatView"
```

---

## Self-Review Checklist

**Spec coverage:**

| Spec requirement | Covered by |
|---|---|
| CM6 live preview (Obsidian-style) | Task 6 (`live-preview.ts`) |
| Turn-based tinting (tint only, no borders) | Task 5 (`turn-boundaries.ts`) |
| Append-only, read-only past turns | Task 5 — transaction filter |
| Agent streaming — blinking cursor | Task 7 (`streaming-ext.ts`) |
| Sticky todo checklist while streaming | Task 8 (`todo-sticky.ts`) + Task 16 |
| Tool call collapsible pills | Task 9 (`widget-ext.ts`) + Task 10 (`tool-call-pill.tsx`) |
| Excalidraw inline widget | Task 9 (`widget-ext.ts`) + Task 12 (`excalidraw-widget.tsx`) |
| Mermaid read-only widget | Task 9 (`widget-ext.ts`) + Task 11 (`mermaid-widget.tsx`) |
| Slash command palette | Task 13 |
| Toolbar — formatting + insert dropdown | Task 14 |
| Widget registry (frontend-owned, opaque backend) | Task 9 (`widget-registry.ts`) |
| Per-workspace conversation store | Task 3 |
| Windowed CM6 (turn store as source of truth) | Task 15 + Task 16 |
| Mock data + streaming simulation | Task 4 |
| MSW handler | Task 16 |
| Delete old chat components | Task 17 |
| `Cmd+Enter` submits | Task 15 (submitKeymap) |
| Demo testable in browser | Task 18 |

All spec requirements are covered. ✓
