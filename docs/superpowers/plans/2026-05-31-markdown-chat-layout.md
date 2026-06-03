# Markdown Chat Layout Redesign — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current single-CM6-instance chat view with a split architecture: a read-only history viewer (full-width user-turn tinting, no hacks) + a separate editable input CM6 (rich widget support) docked at the bottom inside a warm-tinted user zone.

**Architecture:** The history viewer (`markdown-chat-history.tsx`) is a read-only CM6 that receives turns imperatively via dispatched transactions — no transaction filter, no `<!-- input -->` marker. The input editor (`markdown-chat-input.tsx`) is a fully-editable CM6 with the complete widget stack (Excalidraw, Mermaid, live-preview, slash commands). On `⌘↵` the input CM6 content is appended to the history viewer and the input clears.

**Tech Stack:** CodeMirror 6, React 18, Zustand (vanilla stores), `@codemirror/state` `Prec`, `@codemirror/view` `Decoration.line`, TailwindCSS v4, CrossUI Toolbar from `@/components/ui/toolbar`

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `extensions/turn-boundaries.ts` | Remove INPUT_MARKER/filter; user tint via `Decoration.line`; column padding CSS; add `appendTurnToHistory()` |
| Rename+simplify | `components/markdown-chat-editor.tsx` → `components/markdown-chat-history.tsx` | Read-only history display; custom scrollbar; no editable region |
| Create | `components/markdown-chat-input.tsx` | Editable CM6; full widget stack; auto-expand; submit on `⌘↵` |
| Modify | `components/markdown-chat-view.tsx` | New layout: history + tinted input zone + toolbar; new submit flow |
| Modify | `components/markdown-chat-toolbar.tsx` | Render inside input zone; amber-tinted buttons |
| Modify | `__tests__/.../turn-boundaries.test.ts` | Remove INPUT_MARKER assertions |
| Create | `__tests__/.../markdown-chat-input.test.tsx` | Submit flow, clear-on-submit, widget insertion |

---

## Task 1: Simplify turn-boundaries.ts

**Files:**
- Modify: `web/src/features/markdown-chat/extensions/turn-boundaries.ts`
- Modify: `web/src/__tests__/features/markdown-chat/extensions/turn-boundaries.test.ts`

The goal: remove all `<!-- input -->` machinery, switch user-turn tinting from `Decoration.mark` (inline) to `Decoration.line` (per-line, enables full-width CSS background), add `appendTurnToHistory()` for the new submit flow.

### CSS approach for full-width user-turn bands

The key insight: `.cm-line` is a block element that spans 100% of `.cm-content`. If we give `.cm-content` no horizontal padding and instead put padding on each `.cm-line`, then user-turn lines can have `background: <warm>` that fills the full line width — naturally bleeding to viewport edges.

Column formula: `padding: 0 max(48px, calc((100% - 680px) / 2 + 48px))`
- At 1200px viewport: `max(48px, (1200-680)/2 + 48) = max(48px, 308px)` → 308px per side → text column = 1200-616 = 584px ✓
- At 500px viewport: `max(48px, (500-680)/2 + 48) = max(48px, -42px) = 48px` per side → text = 404px ✓

- [ ] **Step 1: Write the failing tests first**

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
  // No INPUT_MARKER in the new design
  expect(doc).not.toContain('<!-- input -->')
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
  expect(ranges[0].from).toBe(0)
  expect(ranges[1].from).toBeGreaterThan(ranges[0].from)
  expect(ranges[1].to).toBeGreaterThan(ranges[0].to)
})
```

- [ ] **Step 2: Run tests to confirm which pass/fail**

```bash
cd web && npx vitest run src/__tests__/features/markdown-chat/extensions/turn-boundaries.test.ts -t "INPUT" 2>&1 | tail -20
```

Expected: the `not.toContain('<!-- input -->')` test FAILS (current code adds INPUT_MARKER).

- [ ] **Step 3: Rewrite turn-boundaries.ts**

Replace the entire file:

```typescript
import {
  Annotation,
  EditorState,
  RangeSetBuilder,
  StateField,
  Text,
} from '@codemirror/state'
import {
  Decoration,
  DecorationSet,
  EditorView,
} from '@codemirror/view'
import type { MarkdownTurn, TurnRole } from '../types'
import { TURN_MARKER_RE } from '../types'

export interface TurnRange {
  id: string
  role: TurnRole
  from: number
  to: number
}

// Used by appendTurnToHistory and appendStreamChunk to bypass any future filters.
export const streamingAnnotation = Annotation.define<boolean>()

export function turnsToDocument(turns: MarkdownTurn[]): string {
  if (turns.length === 0) return ''
  return turns
    .map((t) => `<!-- turn:${t.id} role:${t.role} -->\n${t.content}`)
    .join('\n\n')
}

export function parseTurnBoundaries(doc: Text): TurnRange[] {
  const ranges: TurnRange[] = []
  for (let i = 1; i <= doc.lines; i++) {
    const line = doc.line(i)
    const match = line.text.match(TURN_MARKER_RE)
    if (match) {
      if (ranges.length > 0) ranges[ranges.length - 1].to = line.from - 1
      ranges.push({ id: match[1], role: match[2] as TurnRole, from: line.from, to: doc.length })
    }
  }
  return ranges
}

// Append a completed turn (user or agent) to the history viewer.
// Used by handleSubmit in the view after input CM6 submits.
export function appendTurnToHistory(
  view: EditorView,
  id: string,
  role: TurnRole,
  content: string,
): void {
  const sep = view.state.doc.length === 0 ? '' : '\n\n'
  const insert = `${sep}<!-- turn:${id} role:${role} -->\n${content}`
  view.dispatch({
    changes: { from: view.state.doc.length, insert },
    annotations: streamingAnnotation.of(true),
  })
}

const turnRangesField = StateField.define<TurnRange[]>({
  create: (state) => parseTurnBoundaries(state.doc),
  update: (ranges, tr) => tr.docChanged ? parseTurnBoundaries(tr.newDoc) : ranges,
})

function buildDecorations(state: EditorState): DecorationSet {
  const ranges = state.field(turnRangesField)
  const builder = new RangeSetBuilder<Decoration>()

  for (const range of ranges) {
    const markerLine = state.doc.lineAt(range.from)
    // Hide the boundary marker line (including trailing newline)
    const markerEnd = Math.min(markerLine.to + 1, state.doc.length)
    builder.add(markerLine.from, markerEnd, Decoration.replace({}))

    if (range.role === 'user') {
      // Apply warm-tint class to every line of the user turn.
      // Decoration.line adds to the .cm-line element, which spans the full
      // content width — CSS can then give it a full-viewport background.
      const contentStart = markerLine.to + 1
      if (contentStart < range.to) {
        const firstLine = state.doc.lineAt(contentStart).number
        const lastLine = state.doc.lineAt(range.to).number
        for (let ln = firstLine; ln <= lastLine; ln++) {
          const l = state.doc.line(ln)
          builder.add(l.from, l.from, Decoration.line({ class: 'cm-turn-user' }))
        }
      }
    }
    // Agent turns: no tint (plain document background)
  }

  return builder.finish()
}

const turnDecorationsField = StateField.define<DecorationSet>({
  create: (state) => buildDecorations(state),
  update: (deco, tr) => tr.docChanged ? buildDecorations(tr.state) : deco,
  provide: (f) => EditorView.decorations.from(f),
})

// Full-width user-turn tinting via CSS.
// .cm-content has no horizontal padding; .cm-line carries the column padding.
// User lines get background that fills the full .cm-line block width.
const turnTheme = EditorView.theme({
  '&': { height: '100%', width: '100%' },
  '.cm-scroller': {
    overflow: 'auto',
    fontFamily: 'var(--font-sans, system-ui)',
    // Custom scrollbar
    scrollbarWidth: 'thin',
    scrollbarColor: 'oklch(0.4 0 0 / 35%) transparent',
  },
  '.cm-scroller::-webkit-scrollbar': { width: '6px' },
  '.cm-scroller::-webkit-scrollbar-track': { background: 'transparent' },
  '.cm-scroller::-webkit-scrollbar-thumb': {
    background: 'oklch(0.4 0 0 / 35%)',
    borderRadius: '99px',
  },
  '.cm-content': {
    padding: '40px 0 32px',
    minWidth: '100%',
  },
  // Column layout: padding pushes text to center column.
  // Background on user lines fills the full .cm-line block (= full viewport width).
  '.cm-line': {
    padding: '0 max(48px, calc((100% - 680px) / 2 + 48px))',
    lineHeight: '1.75',
  },
  '.cm-turn-user': {
    background: 'rgba(255, 215, 80, 0.055)',
  },
})

export function turnBoundaries() {
  return [turnRangesField, turnDecorationsField, turnTheme]
}

export function getTurnRanges(state: EditorState): TurnRange[] {
  return state.field(turnRangesField)
}
```

- [ ] **Step 4: Run turn-boundaries tests**

```bash
cd web && npx vitest run src/__tests__/features/markdown-chat/extensions/turn-boundaries.test.ts 2>&1 | tail -15
```

Expected: all 3 pass.

- [ ] **Step 5: Run full test suite to check regressions**

```bash
cd web && npx vitest run 2>&1 | tail -10
```

Expected: same passing count as before (one pre-existing failure is normal).

- [ ] **Step 6: Commit**

```bash
cd web && git add src/features/markdown-chat/extensions/turn-boundaries.ts \
  src/__tests__/features/markdown-chat/extensions/turn-boundaries.test.ts
git commit -m "refactor(turn-boundaries): remove INPUT_MARKER machinery; user tint via Decoration.line for full-width CSS bands"
```

---

## Task 2: Create markdown-chat-history.tsx (read-only history viewer)

**Files:**
- Create: `web/src/features/markdown-chat/components/markdown-chat-history.tsx`
- Keep (do not delete yet): `web/src/features/markdown-chat/components/markdown-chat-editor.tsx` (will be deleted in Task 4)

This is the read-only CM6 instance. No transaction filter. Receives the initial document on mount and subsequent turns via `appendTurnToHistory` + `appendStreamChunk` dispatched from the parent.

- [ ] **Step 1: Write the new component**

```typescript
// web/src/features/markdown-chat/components/markdown-chat-history.tsx
import { useEffect, useRef } from 'react'
import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { markdown } from '@codemirror/lang-markdown'
import { history, defaultKeymap, historyKeymap } from '@codemirror/commands'
import { keymap } from '@codemirror/view'
import type { MarkdownTurn } from '../types'
import { turnsToDocument } from '../extensions/turn-boundaries'
import { turnBoundaries } from '../extensions/turn-boundaries'
import { livePreview } from '../extensions/live-preview'
import { streamingExt } from '../extensions/streaming-ext'
import { todoStickyExt } from '../extensions/todo-sticky'
import { widgetExt } from '../extensions/widget-ext'
import './excalidraw-widget'
import './mermaid-widget'

interface MarkdownChatHistoryProps {
  turns: MarkdownTurn[]
  getTurns: () => MarkdownTurn[]
  onWidgetChange: (widgetId: string, payload: unknown) => void
  onReady: (view: EditorView) => void
}

export function MarkdownChatHistory({
  turns,
  getTurns,
  onWidgetChange,
  onReady,
}: MarkdownChatHistoryProps) {
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!containerRef.current) return

    const state = EditorState.create({
      doc: turnsToDocument(turns),
      extensions: [
        markdown(),
        history(),
        keymap.of([...defaultKeymap, ...historyKeymap]),
        // Read-only: no editable keymap, no transaction filter — dispatcher controls updates
        EditorView.editable.of(false),
        turnBoundaries(),
        livePreview(),
        streamingExt(),
        todoStickyExt(),
        widgetExt(getTurns, onWidgetChange),
      ],
    })

    const view = new EditorView({ state, parent: containerRef.current })
    onReady(view)

    return () => view.destroy()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <div
      ref={containerRef}
      className="min-h-0 w-full flex-1"
    />
  )
}
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -30
```

Expected: no errors related to the new file.

- [ ] **Step 3: Commit**

```bash
cd web && git add src/features/markdown-chat/components/markdown-chat-history.tsx
git commit -m "feat(markdown-chat): add read-only history viewer CM6 component"
```

---

## Task 3: Create markdown-chat-input.tsx (editable rich input)

**Files:**
- Create: `web/src/features/markdown-chat/components/markdown-chat-input.tsx`
- Create: `web/src/__tests__/features/markdown-chat/components/markdown-chat-input.test.tsx`

The input CM6 is empty on mount, supports the full widget stack (Excalidraw, Mermaid, live-preview, slash commands), auto-expands, and submits on `⌘↵`. The parent calls `clear()` (exposed via ref) after the submit is processed.

- [ ] **Step 1: Write the failing test**

```typescript
// web/src/__tests__/features/markdown-chat/components/markdown-chat-input.test.tsx
import { render, screen } from '@testing-library/react'
import { vi } from 'vitest'

// Mock heavy deps
vi.mock('@excalidraw/excalidraw', () => ({ Excalidraw: () => null }))
vi.mock('mermaid', () => ({ default: { initialize: vi.fn(), render: vi.fn().mockResolvedValue({ svg: '' }) } }))
vi.mock('@codemirror/view', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@codemirror/view')>()
  class MockEditorView {
    static editable = actual.EditorView.editable
    static domEventHandlers = actual.EditorView.domEventHandlers
    static theme = actual.EditorView.theme
    static decorations = actual.EditorView.decorations
    static lineWrapping = actual.EditorView.lineWrapping
    static updateListener = actual.EditorView.updateListener
    state = { doc: { toString: () => '', length: 0 }, field: () => [], selection: { main: { head: 0 } } }
    dispatch = vi.fn()
    destroy = vi.fn()
    constructor(_config: unknown) {}
  }
  return { ...actual, EditorView: MockEditorView }
})

import { MarkdownChatInput } from '@/features/markdown-chat/components/markdown-chat-input'

test('renders without crashing', () => {
  const { container } = render(
    <MarkdownChatInput
      getTurns={() => []}
      onSubmit={vi.fn()}
      onWidgetChange={vi.fn()}
      onEditorReady={vi.fn()}
    />
  )
  expect(container.firstChild).toBeTruthy()
})
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd web && npx vitest run src/__tests__/features/markdown-chat/components/markdown-chat-input.test.tsx 2>&1 | tail -15
```

Expected: FAIL — module not found.

- [ ] **Step 3: Create the component**

```typescript
// web/src/features/markdown-chat/components/markdown-chat-input.tsx
import { useEffect, useRef, useState, useCallback } from 'react'
import { EditorState, Prec } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { markdown } from '@codemirror/lang-markdown'
import { history, defaultKeymap, historyKeymap } from '@codemirror/commands'
import type { MarkdownTurn } from '../types'
import { livePreview } from '../extensions/live-preview'
import { widgetExt } from '../extensions/widget-ext'
import { slashCommandExt, type SlashCommandState } from '../extensions/slash-command-ext'
import { SlashCommandPalette, type SlashCommand } from './slash-command-palette'
import './excalidraw-widget'
import './mermaid-widget'

interface MarkdownChatInputProps {
  getTurns: () => MarkdownTurn[]
  onSubmit: (content: string) => void
  onWidgetChange: (widgetId: string, payload: unknown) => void
  onEditorReady: (view: EditorView) => void
  onSlashCommand?: (cmd: SlashCommand) => void
}

export function MarkdownChatInput({
  getTurns,
  onSubmit,
  onWidgetChange,
  onEditorReady,
  onSlashCommand,
}: MarkdownChatInputProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const onSubmitRef = useRef(onSubmit)
  const onSlashCommandRef = useRef(onSlashCommand)
  const [slashState, setSlashState] = useState<SlashCommandState>({ open: false, query: '', anchorRect: null })

  useEffect(() => { onSubmitRef.current = onSubmit }, [onSubmit])
  useEffect(() => { onSlashCommandRef.current = onSlashCommand }, [onSlashCommand])

  const handleSlashCommand = useCallback((cmd: SlashCommand) => {
    const view = viewRef.current
    if (view) {
      const { from } = view.state.selection.main
      const line = view.state.doc.lineAt(from)
      const textBefore = line.text.slice(0, from - line.from)
      const slashIdx = textBefore.lastIndexOf('/')
      if (slashIdx !== -1) {
        view.dispatch({ changes: { from: line.from + slashIdx, to: from, insert: cmd.id } })
      }
    }
    setSlashState({ open: false, query: '', anchorRect: null })
    onSlashCommandRef.current?.(cmd)
  }, [])

  useEffect(() => {
    if (!containerRef.current) return

    const submitKeymap = Prec.highest(keymap.of([{
      key: 'Mod-Enter',
      run(view) {
        const content = view.state.doc.toString().trim()
        if (content) {
          onSubmitRef.current(content)
          // Clear the input
          view.dispatch({
            changes: { from: 0, to: view.state.doc.length, insert: '' },
          })
        }
        return true
      },
    }]))

    const state = EditorState.create({
      doc: '',
      extensions: [
        markdown(),
        history(),
        keymap.of([...defaultKeymap, ...historyKeymap]),
        submitKeymap,
        livePreview(),
        widgetExt(getTurns, onWidgetChange),
        slashCommandExt(setSlashState),
        EditorView.lineWrapping,
        EditorView.theme({
          '&': {
            fontSize: '14px',
            // Auto-height: grows with content up to max
            height: 'auto',
            maxHeight: '240px',
          },
          '.cm-scroller': {
            overflow: 'auto',
            fontFamily: 'var(--font-sans, system-ui)',
            scrollbarWidth: 'thin',
            scrollbarColor: 'oklch(0.4 0 0 / 35%) transparent',
          },
          '.cm-content': {
            padding: '8px max(48px, calc((100% - 680px) / 2 + 48px))',
            minHeight: '52px',
          },
          '.cm-line': { lineHeight: '1.7' },
        }),
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
  }, [])

  return (
    <div className="relative">
      <div ref={containerRef} className="w-full" />
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

- [ ] **Step 4: Run the test**

```bash
cd web && npx vitest run src/__tests__/features/markdown-chat/components/markdown-chat-input.test.tsx 2>&1 | tail -15
```

Expected: PASS.

- [ ] **Step 5: Type-check**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
cd web && git add src/features/markdown-chat/components/markdown-chat-input.tsx \
  src/__tests__/features/markdown-chat/components/markdown-chat-input.test.tsx
git commit -m "feat(markdown-chat): add editable input CM6 component with full widget stack"
```

---

## Task 4: Update markdown-chat-toolbar.tsx for the input zone

**Files:**
- Modify: `web/src/features/markdown-chat/components/markdown-chat-toolbar.tsx`

The toolbar moves inside the warm-tinted input zone. Buttons get an amber tint matching the user zone. `insertBlock` in the toolbar always appends to `doc.length` of the input CM6 (already does this — no change needed there). The only change is visual: amber button colors and the toolbar wrapper class is removed (parent supplies the zone background).

- [ ] **Step 1: Update toolbar styles**

In `markdown-chat-toolbar.tsx`, replace the outer wrapper div className and the `ToolbarButton` className to use amber-tinted colors:

```typescript
// Replace the MarkdownChatToolbar return JSX:
export function MarkdownChatToolbar({ editorView, onInsertWidget }: ToolbarProps) {
  const v = editorView

  return (
    <Toolbar className="rounded-none border-x-0 border-b-0 border-t border-[rgba(255,215,80,0.09)] bg-transparent px-2 py-1.5">
      <ToolbarGroup>
        <ToolbarButton
          aria-label="Bold"
          onClick={() => v && wrapSelection(v, '**')}
          className="flex h-6 min-w-6 items-center justify-center rounded px-1.5 text-xs font-bold text-[rgba(255,215,80,0.35)] hover:bg-[rgba(255,215,80,0.08)] hover:text-[rgba(255,215,80,0.7)]"
        >
          B
        </ToolbarButton>
        <ToolbarButton
          aria-label="Italic"
          onClick={() => v && wrapSelection(v, '*')}
          className="flex h-6 min-w-6 items-center justify-center rounded px-1.5 text-xs italic text-[rgba(255,215,80,0.35)] hover:bg-[rgba(255,215,80,0.08)] hover:text-[rgba(255,215,80,0.7)]"
        >
          I
        </ToolbarButton>
        <ToolbarButton
          aria-label="Inline code"
          onClick={() => v && wrapSelection(v, '`')}
          className="flex h-6 min-w-6 items-center justify-center rounded px-1.5 font-mono text-[10px] text-[rgba(255,215,80,0.35)] hover:bg-[rgba(255,215,80,0.08)] hover:text-[rgba(255,215,80,0.7)]"
        >
          {"`x`"}
        </ToolbarButton>
      </ToolbarGroup>

      <ToolbarSeparator className="bg-[rgba(255,215,80,0.1)]" />

      <ToolbarGroup>
        <ToolbarButton
          aria-label="Heading 1"
          onClick={() => v && prependLine(v, '# ')}
          className="flex h-6 min-w-6 items-center justify-center rounded px-1.5 text-xs font-semibold text-[rgba(255,215,80,0.35)] hover:bg-[rgba(255,215,80,0.08)] hover:text-[rgba(255,215,80,0.7)]"
        >
          H1
        </ToolbarButton>
        <ToolbarButton
          aria-label="Heading 2"
          onClick={() => v && prependLine(v, '## ')}
          className="flex h-6 min-w-6 items-center justify-center rounded px-1.5 text-xs font-semibold text-[rgba(255,215,80,0.35)] hover:bg-[rgba(255,215,80,0.08)] hover:text-[rgba(255,215,80,0.7)]"
        >
          H2
        </ToolbarButton>
        <ToolbarButton
          aria-label="Heading 3"
          onClick={() => v && prependLine(v, '### ')}
          className="flex h-6 min-w-6 items-center justify-center rounded px-1.5 text-xs font-semibold text-[rgba(255,215,80,0.35)] hover:bg-[rgba(255,215,80,0.08)] hover:text-[rgba(255,215,80,0.7)]"
        >
          H3
        </ToolbarButton>
      </ToolbarGroup>

      <ToolbarSeparator className="bg-[rgba(255,215,80,0.1)]" />

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

      {/* Spacer + send hint */}
      <div className="ml-auto flex items-center gap-2">
        <span className="font-mono text-[10px] text-[rgba(255,215,80,0.2)]">⌘↵</span>
        <button
          onClick={() => {
            if (!v) return
            const content = v.state.doc.toString().trim()
            if (content) {
              // Trigger the Mod-Enter keymap imperatively
              v.dispatch({ 
                effects: [],
                // The keymap handles submit; we just need to focus and dispatch an event
              })
              // Call the keymap handler via a synthetic key event is complex.
              // Instead, re-use onInsertWidget channel to signal submit—or better,
              // expose a submit prop on the toolbar.
            }
          }}
          className="flex h-7 w-7 items-center justify-center rounded-lg bg-[rgba(80,140,255,0.8)] text-white transition-colors hover:bg-[rgba(80,140,255,1)]"
          title="Send (⌘↵)"
        >
          <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
            <path d="M2 8L14 2L8 14L6.5 9.5Z"/>
          </svg>
        </button>
      </div>
    </Toolbar>
  )
}
```

Wait — the send button needs to call `onSubmit` directly. Add `onSubmit` to `ToolbarProps`:

```typescript
interface ToolbarProps {
  editorView: EditorView | null
  onInsertWidget: (widgetType: string, widgetId: string) => void
  onSubmit: () => void  // new
}
```

And in the send button's `onClick`:
```typescript
onClick={() => onSubmit()}
```

- [ ] **Step 2: Type-check**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: no errors. If `onSubmit` prop errors appear in callers, fix in Task 5.

- [ ] **Step 3: Commit**

```bash
cd web && git add src/features/markdown-chat/components/markdown-chat-toolbar.tsx
git commit -m "feat(markdown-chat): amber toolbar styles for user input zone; add onSubmit prop"
```

---

## Task 5: Rework markdown-chat-view.tsx — new layout + submit flow

**Files:**
- Modify: `web/src/features/markdown-chat/components/markdown-chat-view.tsx`
- Delete: `web/src/features/markdown-chat/components/markdown-chat-editor.tsx`

This is the main wiring task. New layout:
```
[div.flex.h-full.flex-col]
  [div.flex-1.min-h-0]         ← history viewer (scrollable)
    <MarkdownChatHistory>
  [div.input-zone]              ← warm-tinted user zone
    <MarkdownChatInput>
    <MarkdownChatToolbar>
```

New submit flow (no more `<!-- input -->` markers, no `insertTurnAndStartStreaming`):
1. `handleSubmit(content)` called from input CM6's `⌘↵`
2. Append user turn to store
3. `appendTurnToHistory(historyView, userId, 'user', content)` — dispatches to history CM6
4. Append empty streaming agent turn to store
5. `appendTurnToHistory(historyView, agentId, 'agent', '')` — adds agent marker
6. `simulateMarkdownStream(MOCK_RESPONSE, chunk => appendStreamChunk(historyView, chunk), () => { finalizeStreaming(historyView); state.finalizeStreamingTurn(agentId) })`

- [ ] **Step 1: Write the new view**

```typescript
// web/src/features/markdown-chat/components/markdown-chat-view.tsx
import { useEffect, useRef, useCallback, useState } from 'react'
import { useStore } from 'zustand'
import { nanoid } from 'nanoid'
import type { EditorView } from '@codemirror/view'
import { appendStreamChunk, finalizeStreaming } from '../extensions/streaming-ext'
import { appendTurnToHistory } from '../extensions/turn-boundaries'
import { getOrCreateConversationStore } from '../stores/conversation-store'
import { getMockMarkdownTurns, simulateMarkdownStream } from '@/lib/mock/markdown-chat'
import { MarkdownChatHistory } from './markdown-chat-history'
import { MarkdownChatInput } from './markdown-chat-input'
import { MarkdownChatToolbar } from './markdown-chat-toolbar'
import type { SlashCommand } from './slash-command-palette'

interface MarkdownChatViewProps {
  workspaceId: string
  stepId: string
}

const STEP_GREETINGS: Record<string, string> = {
  brainstorm: "I'm ready to brainstorm. What do you want to build?",
  spec: "Let's refine the spec. What would you like to discuss?",
  build: 'Ready to implement. What should we tackle first?',
  ai_review: "I've reviewed the diff. Here's what I found.",
  human_review: 'Waiting for your review comments.',
}

const MOCK_RESPONSE =
  'Great point. Let me think through this carefully.\n\n' +
  'There are several considerations here:\n\n' +
  '1. **Performance** — the current approach has O(n²) complexity\n' +
  '2. **Correctness** — edge cases around empty inputs\n' +
  '3. **Maintainability** — the code is hard to follow\n\n' +
  'My recommendation is to refactor the core loop first.'

export function MarkdownChatView({ workspaceId, stepId }: MarkdownChatViewProps) {
  const store = getOrCreateConversationStore(workspaceId)
  const turns = useStore(store, (s) => s.turns)
  const historyViewRef = useRef<EditorView | null>(null)
  const [inputEditorView, setInputEditorView] = useState<EditorView | null>(null)
  const cancelStreamRef = useRef<(() => void) | null>(null)

  const getTurns = useCallback(() => store.getState().turns, [store])

  // Seed turns on mount
  useEffect(() => {
    const state = store.getState()
    if (state.turns.length > 0) return
    const mockTurns = getMockMarkdownTurns(workspaceId, stepId)
    if (mockTurns.length > 0) {
      mockTurns.forEach((t) => state.appendTurn(t))
    } else {
      const greeting = STEP_GREETINGS[stepId]
      if (greeting) {
        state.appendTurn({
          id: nanoid(), role: 'agent', content: greeting,
          timestamp: new Date().toISOString(), authorName: 'Claude', widgets: [],
        })
      }
    }
    return () => { cancelStreamRef.current?.() }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workspaceId, stepId])

  const handleSubmit = useCallback((content: string) => {
    cancelStreamRef.current?.()
    const historyView = historyViewRef.current
    if (!historyView) return

    const state = store.getState()
    const userId = nanoid()
    const agentId = nanoid()

    // Update store
    state.appendTurn({ id: userId, role: 'user', content, timestamp: new Date().toISOString(), authorName: 'You', widgets: [] })
    state.appendTurn({ id: agentId, role: 'agent', content: '', timestamp: new Date().toISOString(), authorName: 'Claude', widgets: [], streaming: true })

    // Update history CM6 document imperatively
    appendTurnToHistory(historyView, userId, 'user', content)
    appendTurnToHistory(historyView, agentId, 'agent', '')

    // Stream agent response
    cancelStreamRef.current = simulateMarkdownStream(
      MOCK_RESPONSE,
      (chunk) => {
        state.updateStreamingTurn(agentId, chunk)
        appendStreamChunk(historyView, chunk)
      },
      () => {
        state.finalizeStreamingTurn(agentId)
        finalizeStreaming(historyView)
        cancelStreamRef.current = null
      },
    )
  }, [store])

  const handleWidgetChange = useCallback((widgetId: string, payload: unknown) => {
    const { turns: currentTurns } = store.getState()
    const turn = currentTurns.find((t) => t.widgets.some((w) => w.id === widgetId))
    if (turn) store.getState().updateWidgetPayload(turn.id, widgetId, payload)
  }, [store])

  const handleInsertWidget = useCallback((widgetType: string, widgetId: string) => {
    const { turns: currentTurns } = store.getState()
    const lastTurn = currentTurns.at(-1)
    if (lastTurn) {
      store.getState().appendWidget(lastTurn.id, { id: widgetId, type: widgetType, payload: null })
    }
  }, [store])

  const handleSlashCommand = useCallback((cmd: SlashCommand) => {
    handleSubmit(cmd.id)
  }, [handleSubmit])

  const handleHistoryReady = useCallback((view: EditorView) => {
    historyViewRef.current = view
  }, [])

  const handleInputReady = useCallback((view: EditorView) => {
    setInputEditorView(view)
  }, [])

  const handleSendClick = useCallback(() => {
    const view = inputEditorView
    if (!view) return
    const content = view.state.doc.toString().trim()
    if (content) {
      handleSubmit(content)
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: '' } })
    }
  }, [inputEditorView, handleSubmit])

  if (turns.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        <p className="text-sm">Loading conversation…</p>
      </div>
    )
  }

  return (
    <div className="flex h-full w-full flex-col overflow-hidden">
      {/* History: flex-1 scrollable document */}
      <div className="min-h-0 flex-1 overflow-hidden">
        <MarkdownChatHistory
          turns={turns}
          getTurns={getTurns}
          onWidgetChange={handleWidgetChange}
          onReady={handleHistoryReady}
        />
      </div>

      {/* Input zone: warm tint, same as user turns */}
      <div
        className="flex-shrink-0"
        style={{
          background: 'rgba(255, 215, 80, 0.055)',
          borderTop: '1px solid rgba(255, 215, 80, 0.09)',
        }}
      >
        <MarkdownChatInput
          getTurns={getTurns}
          onSubmit={(content) => {
            handleSubmit(content)
          }}
          onWidgetChange={handleWidgetChange}
          onEditorReady={handleInputReady}
          onSlashCommand={handleSlashCommand}
        />
        <MarkdownChatToolbar
          editorView={inputEditorView}
          onInsertWidget={handleInsertWidget}
          onSubmit={handleSendClick}
        />
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Delete the old editor file**

```bash
cd web && rm src/features/markdown-chat/components/markdown-chat-editor.tsx
```

- [ ] **Step 3: Check for any remaining imports of the old file**

```bash
cd web && grep -r "markdown-chat-editor" src/ 2>&1
```

Expected: no results. If any found, update the import to use `markdown-chat-history`.

- [ ] **Step 4: Type-check**

```bash
cd web && npx tsc --noEmit 2>&1 | head -30
```

Fix any errors. Common ones:
- `markdown-chat-editor` still imported somewhere → change to `markdown-chat-history`
- `onSubmit` prop not passed to `MarkdownChatToolbar` → already added in Task 4

- [ ] **Step 5: Run full test suite**

```bash
cd web && npx vitest run 2>&1 | tail -10
```

Expected: same or better passing count. Tests that relied on `markdown-chat-editor.tsx` directly may need updating (see Task 6).

- [ ] **Step 6: Commit**

```bash
cd web && git add src/features/markdown-chat/components/markdown-chat-view.tsx \
  src/features/markdown-chat/components/markdown-chat-history.tsx \
  src/features/markdown-chat/components/markdown-chat-toolbar.tsx
git rm src/features/markdown-chat/components/markdown-chat-editor.tsx
git commit -m "feat(markdown-chat): split into history viewer + input editor; warm-tint input zone; full-width user turn bands"
```

---

## Task 6: Fix and update tests

**Files:**
- Modify: `web/src/__tests__/features/markdown-chat/components/markdown-chat-view.test.tsx`

The `markdown-chat-view.test.tsx` may reference the old editor component. Update it to match the new architecture.

- [ ] **Step 1: Run existing view tests to see failures**

```bash
cd web && npx vitest run src/__tests__/features/markdown-chat/components/markdown-chat-view.test.tsx 2>&1 | tail -30
```

- [ ] **Step 2: Update the test file**

The test needs to mock `MarkdownChatHistory` and `MarkdownChatInput` instead of the old `MarkdownChatEditor`. Replace the mocks at the top:

```typescript
// In markdown-chat-view.test.tsx — replace the @codemirror/view mock and editor import
vi.mock('@/features/markdown-chat/components/markdown-chat-history', () => ({
  MarkdownChatHistory: vi.fn(({ onReady }) => {
    // Simulate onReady being called with a mock view
    const mockView = {
      state: { doc: { toString: () => '', length: 0 }, field: () => [] },
      dispatch: vi.fn(),
      destroy: vi.fn(),
    }
    onReady?.(mockView)
    return <div data-testid="mock-history" />
  }),
}))

vi.mock('@/features/markdown-chat/components/markdown-chat-input', () => ({
  MarkdownChatInput: vi.fn(({ onEditorReady }) => {
    const mockView = {
      state: { doc: { toString: () => 'test content', length: 12 }, field: () => [] },
      dispatch: vi.fn(),
      destroy: vi.fn(),
    }
    onEditorReady?.(mockView)
    return <div data-testid="mock-input" />
  }),
}))
```

Update assertions to use `data-testid="mock-history"` and `data-testid="mock-input"` where the old test checked for `mock-editor`.

- [ ] **Step 3: Run all tests**

```bash
cd web && npx vitest run 2>&1 | tail -10
```

Expected: 660/661 passing (one pre-existing unrelated failure is normal).

- [ ] **Step 4: Commit**

```bash
cd web && git add src/__tests__/features/markdown-chat/components/markdown-chat-view.test.tsx
git commit -m "test(markdown-chat): update view tests for split history/input architecture"
```

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Task |
|-----------------|------|
| Agent turns: no tint | Task 1 — `buildDecorations` only adds `Decoration.line` for `role === 'user'` |
| User turns: full-width warm band edge-to-edge | Task 1 — `.cm-line` full-width + `.cm-turn-user { background }` |
| Custom scrollbar | Task 1 — `.cm-scroller` CSS in `turnTheme` |
| Google-doc column margins | Task 1 — `.cm-line { padding: 0 max(48px, calc(...)) }` |
| Input zone: same warm tint | Task 5 — `style={{ background: 'rgba(255,215,80,0.055)' }}` wrapper |
| CrossUI toolbar inside tinted zone | Task 5 — toolbar is a child of the input-zone div |
| Send button in toolbar | Task 4 — `onSubmit` prop + send button in toolbar |
| Rich content (Excalidraw, Mermaid) in input | Task 3 — input CM6 has `widgetExt`, imports widget registrations |
| Rich content in history viewer | Task 2 — history CM6 has `widgetExt` |
| Input clears on submit | Task 3 — `view.dispatch({ changes: { from:0, to:length, insert:'' } })` in submit keymap |
| Two CM6 instances | Tasks 2+3 — separate `markdown-chat-history.tsx` and `markdown-chat-input.tsx` |
| No INPUT_MARKER hacks | Task 1 — removed entirely |
| No transaction filter | Task 2 — history uses `EditorView.editable.of(false)` instead |

**Placeholder scan:** No TBD/TODO found. All code blocks are complete.

**Type consistency:** `appendTurnToHistory(view, id, role, content)` defined in Task 1, used in Task 5 — signatures match. `MarkdownChatToolbar` gains `onSubmit: () => void` in Task 4, consumed in Task 5.

**One gap identified and addressed:** The send button in the toolbar needs `onSubmit` as a prop (added in Task 4 spec). The `handleSendClick` in Task 5 reads from `inputEditorView` and calls `handleSubmit` + clears the input CM6. This is correct since `⌘↵` in the input CM6 only calls `onSubmit(content)` without clearing — the view's `handleSubmit` does not clear the input either. Clearing happens in two places: (a) the `⌘↵` keymap in `markdown-chat-input.tsx` clears after calling `onSubmitRef.current`, and (b) `handleSendClick` clears after calling `handleSubmit`. This is correct.
