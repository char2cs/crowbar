# Plate Markdown Editor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Plate a Notion-style rich editor the default surface for `.md`/`.markdown` files, with Monaco as a per-file Rich⇄Source escape hatch, keeping the markdown text on disk as the single source of truth.

**Architecture:** Markdown files stay ordinary `editor` buffers (`EditorContent`). A surface selector inside `EditorPane` renders the new lazy `MarkdownEditorPane` (Plate) when the buffer is markdown and its per-buffer view mode is `rich`; otherwise it renders the existing Monaco `EditorSurface`. Plate deserializes `buffer.content` on mount and, on edit (debounced), serializes back to markdown and writes it through the *same* store seam Monaco uses (`editor-app-store`'s `handleContentChange`). Save is unchanged.

**Tech Stack:** React + Vite, Zustand stores, Vitest + Testing Library (jsdom), `platejs` + `@platejs/markdown` (MIT), `remark-gfm` (already a dependency).

## Global Constraints

- Component files use **kebab-case**; exported React components stay PascalCase. (CLAUDE.md)
- All tests live under `web/src/__tests__/` mirroring `web/src/`; use `@/` imports, never relative `../../`. (CLAUDE.md)
- Stores use **narrow selectors** (`useXxxStore((s) => s.field)`); `getState()` only in handlers/effects; stores must **not** import from `components/`. (CLAUDE.md)
- Always use `@/components/ui/*` and CSS-variable tokens; never hardcode colors. (memory: component-tokens)
- Heavy editor code must be **lazy/code-split**: the entry chunk must gain **zero** Plate/Slate bytes (same discipline as Monaco and the CodeMirror comment-composer chunk).
- **No PR / no push.** Commit locally per task and stop. (memory: no-unrequested-prs)
- Live verification runs against the **one** already-running dev Tauri instance — never spawn a second. (memory: one-dev-instance)
- Test command (run from `web/`): `bunx vitest run <path>`. Typecheck: `bunx tsc --noEmit`. Lint: `bun run lint`.
- Branch: work continues on the current `enhancement/restyling` branch. Do not touch the unrelated uncommitted agent-chat virtualization files already in the tree.

**Reference:** design spec at `docs/superpowers/specs/2026-07-22-plate-markdown-editor-design.md`.

**Plate API facts (verified against platejs.org, July 2026):**
- Install: `platejs @platejs/markdown`; UI via shadcn registry (`npx shadcn@latest add ...`).
- Editor: `import { Plate, PlateContent, usePlateEditor, createPlateEditor } from 'platejs/react'`.
- Markdown plugin: `import { MarkdownPlugin } from '@platejs/markdown'`, registered as `MarkdownPlugin.configure({ options: { remarkPlugins: [remarkGfm, ...] } })`.
- Deserialize (md → value): `editor.getApi(MarkdownPlugin).markdown.deserialize(md)` (also `editor.api.markdown.deserialize(md)` once mounted).
- Serialize (value → md): `editor.api.markdown.serialize()`.
- The `<Plate>` provider accepts an `onChange={({ value, editor }) => {}}` prop that fires on every change. **Implementers: confirm this exact callback signature against https://platejs.org/docs/editor before Task 4; adjust the destructure if the current version differs.**

---

## Scope: this plan = Slice 1 (foundation) only

The spec's rollout is five staged slices. This plan delivers **Slice 1** — a working rich editor for the GFM core plus the toggle — because it is the foundation the other four depend on, and because slices 2–5 (frontmatter node, extended plugins, image display, preview retirement) are cheaply layered onto a proven bridge. Slices 2–5 each get their own plan once Slice 1 lands and is live-verified. The staged roadmap is at the end of this document.

**Slice 1 delivers:** open a `.md` file → Notion-style Plate editor (headings, bold/italic/strike, lists, task-lists, links, blockquotes, GFM tables, fenced code with highlighting, inline code, HR, slash menu, drag handle, block toolbar) → edits serialize to markdown and mark the buffer dirty → Cmd+S saves through the existing pipeline → a Rich⇄Source toggle flips to Monaco on the same text and back.

## File Structure (Slice 1)

**Create:**
- `web/src/features/editor/markdown/plate/is-markdown-path.ts` — `isMarkdownPath(path): boolean` (`.md`/`.markdown`; `.mdx` excluded, stays on Monaco per spec Risk).
- `web/src/features/editor/markdown/plate/markdown-view-store.ts` — Zustand store: per-buffer `'rich' | 'source'` view mode.
- `web/src/features/editor/markdown/plate/markdown-plugins.ts` — the shared Plate plugin list (GFM core kit + `MarkdownPlugin.configure`), imported by both the component and the round-trip tests so they exercise identical serialization.
- `web/src/features/editor/markdown/plate/markdown-serialization.ts` — pure helpers `markdownToPlateValue(md)` and `plateValueToMarkdown(value)` built on a headless `createPlateEditor` using `markdown-plugins.ts`, so serialization is unit-testable without React.
- `web/src/features/editor/markdown/plate/markdown-editor-pane.tsx` — the Plate surface component (`MarkdownEditorPane`).
- `web/src/features/editor/markdown/plate/markdown-view-toggle.tsx` — the `MarkdownViewToggle` button (Rich⇄Source).
- Shadcn-registry-generated UI kit files under `web/src/components/editor/**` (Task 1; generated, not hand-written).

**Modify:**
- `web/src/features/panes/components/editor-pane.tsx` — surface selector (markdown + rich → `MarkdownEditorPane`, else existing Monaco path).
- `web/src/features/editor/components/toolbar/editor-status-actions.tsx` — render `MarkdownViewToggle` for markdown buffers (Source-mode toolbar).
- `web/package.json` / lockfile — new deps (Task 1).

**Test (mirror):**
- `web/src/__tests__/features/editor/markdown/plate/is-markdown-path.test.ts`
- `web/src/__tests__/features/editor/markdown/plate/markdown-view-store.test.ts`
- `web/src/__tests__/features/editor/markdown/plate/markdown-serialization.test.ts`
- `web/src/__tests__/features/editor/markdown/plate/markdown-editor-pane.test.tsx`
- `web/src/__tests__/features/panes/components/editor-pane-surface-select.test.tsx`

---

### Task 1: Add Plate dependencies and UI kit

**Files:**
- Modify: `web/package.json` (+ lockfile)
- Create: `web/src/components/editor/**` (shadcn-generated)

**Interfaces:**
- Produces: installed `platejs`, `@platejs/markdown`; a registry-generated editor UI kit under `@/components/editor/`. Later tasks import `Plate, PlateContent, usePlateEditor, createPlateEditor` from `platejs/react` and `MarkdownPlugin` from `@platejs/markdown`.

- [ ] **Step 1: Install core packages**

Run (from `web/`):
```bash
bun add platejs @platejs/markdown
```
Expected: both resolve and appear under `dependencies`. `remark-gfm` is already present — do not re-add.

- [ ] **Step 2: Scaffold the Plate UI kit from the shadcn registry**

Run (from `web/`):
```bash
npx shadcn@latest add https://platejs.org/r/editor-basic
```
This writes editor primitives (Editor container, toolbar, block components) under `web/src/components/editor/`. If the registry command prompts, accept defaults; if it cannot resolve, fall back to `npx shadcn@latest add @platejs/basic-nodes-kit @platejs/basic-marks-kit` and consult https://platejs.org/docs/installation/react. Review generated files for hardcoded colors and replace any with app tokens per Global Constraints.

- [ ] **Step 3: Verify install + typecheck**

Run (from `web/`):
```bash
bunx tsc --noEmit
```
Expected: PASS (no missing-module errors for `platejs` / `@platejs/markdown`).

- [ ] **Step 4: Verify the entry chunk is still Plate-free**

Run (from `web/`):
```bash
bunx vite build >/dev/null 2>&1 && grep -rl "platejs" dist/assets/*.js | head
```
Expected: nothing yet (no code imports Plate). This establishes the baseline for the Task 7 code-split check.

- [ ] **Step 5: Commit**

```bash
git add web/package.json web/bun.lock web/src/components/editor
git commit -m "feat(md): add Plate deps and editor UI kit"
```

---

### Task 2: `isMarkdownPath` helper

**Files:**
- Create: `web/src/features/editor/markdown/plate/is-markdown-path.ts`
- Test: `web/src/__tests__/features/editor/markdown/plate/is-markdown-path.test.ts`

**Interfaces:**
- Produces: `export function isMarkdownPath(path: string): boolean`

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/features/editor/markdown/plate/is-markdown-path.test.ts
import { describe, expect, it } from 'vitest'
import { isMarkdownPath } from '@/features/editor/markdown/plate/is-markdown-path'

describe('isMarkdownPath', () => {
  it('matches .md and .markdown case-insensitively', () => {
    expect(isMarkdownPath('/a/README.md')).toBe(true)
    expect(isMarkdownPath('/a/notes.MARKDOWN')).toBe(true)
  })
  it('excludes .mdx and non-markdown', () => {
    expect(isMarkdownPath('/a/page.mdx')).toBe(false)
    expect(isMarkdownPath('/a/main.ts')).toBe(false)
    expect(isMarkdownPath('/a/markdown')).toBe(false) // no extension
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bunx vitest run src/__tests__/features/editor/markdown/plate/is-markdown-path.test.ts`
Expected: FAIL — cannot find module `is-markdown-path`.

- [ ] **Step 3: Write minimal implementation**

```ts
// web/src/features/editor/markdown/plate/is-markdown-path.ts

/** True for files the Plate rich editor owns. `.mdx` is deliberately excluded —
 *  MDX (JSX in markdown) does not round-trip through a markdown serializer, so
 *  those files stay on Monaco (see design spec, Risks). */
export function isMarkdownPath(path: string): boolean {
  const ext = path.split('.').pop()?.toLowerCase()
  return ext === 'md' || ext === 'markdown'
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bunx vitest run src/__tests__/features/editor/markdown/plate/is-markdown-path.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/editor/markdown/plate/is-markdown-path.ts web/src/__tests__/features/editor/markdown/plate/is-markdown-path.test.ts
git commit -m "feat(md): isMarkdownPath helper"
```

---

### Task 3: Per-buffer view-mode store

**Files:**
- Create: `web/src/features/editor/markdown/plate/markdown-view-store.ts`
- Test: `web/src/__tests__/features/editor/markdown/plate/markdown-view-store.test.ts`

**Interfaces:**
- Produces:
  - `type MarkdownView = 'rich' | 'source'`
  - `useMarkdownViewStore` — Zustand store with state `{ views: Record<string, MarkdownView> }` and actions `setView(bufferId, view)`, `toggleView(bufferId)`, `clearView(bufferId)`.
  - `selectMarkdownView(bufferId)` — a selector factory: `(state) => state.views[bufferId] ?? 'rich'` (default `rich`).

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/features/editor/markdown/plate/markdown-view-store.test.ts
import { beforeEach, describe, expect, it } from 'vitest'
import {
  useMarkdownViewStore,
  selectMarkdownView,
} from '@/features/editor/markdown/plate/markdown-view-store'

describe('markdown view store', () => {
  beforeEach(() => useMarkdownViewStore.setState({ views: {} }))

  it('defaults to rich for an unknown buffer', () => {
    expect(selectMarkdownView('b1')(useMarkdownViewStore.getState())).toBe('rich')
  })

  it('setView records the mode', () => {
    useMarkdownViewStore.getState().setView('b1', 'source')
    expect(selectMarkdownView('b1')(useMarkdownViewStore.getState())).toBe('source')
  })

  it('toggleView flips rich<->source from the default', () => {
    useMarkdownViewStore.getState().toggleView('b1') // rich -> source
    expect(selectMarkdownView('b1')(useMarkdownViewStore.getState())).toBe('source')
    useMarkdownViewStore.getState().toggleView('b1') // source -> rich
    expect(selectMarkdownView('b1')(useMarkdownViewStore.getState())).toBe('rich')
  })

  it('clearView removes the entry', () => {
    useMarkdownViewStore.getState().setView('b1', 'source')
    useMarkdownViewStore.getState().clearView('b1')
    expect(useMarkdownViewStore.getState().views.b1).toBeUndefined()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bunx vitest run src/__tests__/features/editor/markdown/plate/markdown-view-store.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Write minimal implementation**

```ts
// web/src/features/editor/markdown/plate/markdown-view-store.ts
import { create } from 'zustand'

export type MarkdownView = 'rich' | 'source'

interface MarkdownViewState {
  views: Record<string, MarkdownView>
  setView: (bufferId: string, view: MarkdownView) => void
  toggleView: (bufferId: string) => void
  clearView: (bufferId: string) => void
}

export const useMarkdownViewStore = create<MarkdownViewState>((set) => ({
  views: {},
  setView: (bufferId, view) =>
    set((s) => ({ views: { ...s.views, [bufferId]: view } })),
  toggleView: (bufferId) =>
    set((s) => ({
      views: {
        ...s.views,
        [bufferId]: (s.views[bufferId] ?? 'rich') === 'rich' ? 'source' : 'rich',
      },
    })),
  clearView: (bufferId) =>
    set((s) => {
      const next = { ...s.views }
      delete next[bufferId]
      return { views: next }
    }),
}))

/** Selector: the view mode for a buffer, defaulting to `rich`. */
export const selectMarkdownView =
  (bufferId: string) =>
  (state: MarkdownViewState): MarkdownView =>
    state.views[bufferId] ?? 'rich'
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bunx vitest run src/__tests__/features/editor/markdown/plate/markdown-view-store.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/editor/markdown/plate/markdown-view-store.ts web/src/__tests__/features/editor/markdown/plate/markdown-view-store.test.ts
git commit -m "feat(md): per-buffer rich/source view store"
```

---

### Task 4: Serialization bridge (markdown ⇄ Plate value) + shared plugins

**Files:**
- Create: `web/src/features/editor/markdown/plate/markdown-plugins.ts`
- Create: `web/src/features/editor/markdown/plate/markdown-serialization.ts`
- Test: `web/src/__tests__/features/editor/markdown/plate/markdown-serialization.test.ts`

**Interfaces:**
- Consumes: `platejs/react` (`createPlateEditor`), `@platejs/markdown` (`MarkdownPlugin`), `remark-gfm`.
- Produces:
  - `markdownPlugins` — the shared plugin array (GFM core kit + `MarkdownPlugin.configure({ options: { remarkPlugins: [remarkGfm] } })`).
  - `markdownToPlateValue(md: string): Value` — deserialize markdown → Plate value.
  - `plateValueToMarkdown(value: Value): string` — serialize a Plate value → markdown.

- [ ] **Step 1: Write the shared plugin list**

```ts
// web/src/features/editor/markdown/plate/markdown-plugins.ts
import { MarkdownPlugin } from '@platejs/markdown'
import remarkGfm from 'remark-gfm'
// GFM-core node/mark kits generated by the shadcn registry in Task 1.
// Adjust these import paths to the actual generated files under
// `@/components/editor/plugins/*` if the registry named them differently.
import { BasicNodesKit } from '@/components/editor/plugins/basic-nodes-kit'
import { BasicMarksKit } from '@/components/editor/plugins/basic-marks-kit'
import { ListKit } from '@/components/editor/plugins/list-kit'
import { TableKit } from '@/components/editor/plugins/table-kit'
import { CodeBlockKit } from '@/components/editor/plugins/code-block-kit'
import { LinkKit } from '@/components/editor/plugins/link-kit'

/** GFM-core plugin set shared by the live editor and the round-trip tests, so
 *  both exercise identical serialization. Extended plugins (math, mermaid,
 *  callouts, footnotes) are added in later slices. */
export const markdownPlugins = [
  ...BasicNodesKit,
  ...BasicMarksKit,
  ...ListKit,
  ...TableKit,
  ...CodeBlockKit,
  ...LinkKit,
  MarkdownPlugin.configure({ options: { remarkPlugins: [remarkGfm] } }),
]
```

*(Implementer note: the exact kit export names come from Task 1's generated files. Open `web/src/components/editor/plugins/` and use the real names; the set must cover headings, marks, lists+tasklists, tables, code blocks, and links.)*

- [ ] **Step 2: Write the serialization helpers**

```ts
// web/src/features/editor/markdown/plate/markdown-serialization.ts
import { createPlateEditor, type Value } from 'platejs/react'
import { MarkdownPlugin } from '@platejs/markdown'
import { markdownPlugins } from './markdown-plugins'

/** A fresh headless editor carrying exactly the live plugin set. */
function makeEditor() {
  return createPlateEditor({ plugins: markdownPlugins })
}

/** Markdown text -> Plate value. */
export function markdownToPlateValue(md: string): Value {
  return makeEditor().getApi(MarkdownPlugin).markdown.deserialize(md)
}

/** Plate value -> markdown text. */
export function plateValueToMarkdown(value: Value): string {
  const editor = makeEditor()
  editor.children = value
  return editor.getApi(MarkdownPlugin).markdown.serialize()
}
```

- [ ] **Step 3: Write the failing round-trip test**

```ts
// web/src/__tests__/features/editor/markdown/plate/markdown-serialization.test.ts
import { describe, expect, it } from 'vitest'
import {
  markdownToPlateValue,
  plateValueToMarkdown,
} from '@/features/editor/markdown/plate/markdown-serialization'

const roundTrip = (md: string) => plateValueToMarkdown(markdownToPlateValue(md))

describe('markdown round-trip (GFM core)', () => {
  it('preserves a heading and emphasis', () => {
    const md = '# Title\n\nThis is **bold** and *italic*.\n'
    expect(roundTrip(md).trim()).toBe(md.trim())
  })

  it('preserves an unordered list', () => {
    const md = '- one\n- two\n- three\n'
    expect(roundTrip(md).trim()).toBe(md.trim())
  })

  it('preserves a task list', () => {
    const md = '- [ ] todo\n- [x] done\n'
    expect(roundTrip(md).trim()).toBe(md.trim())
  })

  it('preserves a GFM table', () => {
    const md = '| a | b |\n| --- | --- |\n| 1 | 2 |\n'
    // Table cell padding may be re-canonicalized; assert structure survives.
    const out = roundTrip(md)
    expect(out).toMatch(/\| a\s*\| b\s*\|/)
    expect(out).toMatch(/\| 1\s*\| 2\s*\|/)
  })

  it('preserves a fenced code block with language', () => {
    const md = '```ts\nconst x = 1\n```\n'
    expect(roundTrip(md)).toContain('```ts')
    expect(roundTrip(md)).toContain('const x = 1')
  })

  it('preserves a link', () => {
    const md = '[Plate](https://platejs.org)\n'
    expect(roundTrip(md)).toContain('[Plate](https://platejs.org)')
  })
})
```

- [ ] **Step 4: Run the test**

Run: `bunx vitest run src/__tests__/features/editor/markdown/plate/markdown-serialization.test.ts`
Expected: PASS. **If a specific construct fails**, the fix is in `markdown-plugins.ts` (a missing kit) or the assertion's canonicalization tolerance — not in disk/save logic. Do not loosen an assertion to hide dropped content; a construct that survives structurally but re-canonicalizes (e.g. table padding) may use a `toMatch`, but content that *disappears* is a real bug to fix in the plugin set.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/editor/markdown/plate/markdown-plugins.ts web/src/features/editor/markdown/plate/markdown-serialization.ts web/src/__tests__/features/editor/markdown/plate/markdown-serialization.test.ts
git commit -m "feat(md): markdown<->Plate serialization bridge with round-trip tests"
```

---

### Task 5: `MarkdownEditorPane` (Plate surface + write seam)

**Files:**
- Create: `web/src/features/editor/markdown/plate/markdown-editor-pane.tsx`
- Test: `web/src/__tests__/features/editor/markdown/plate/markdown-editor-pane.test.tsx`

**Interfaces:**
- Consumes: `markdownToPlateValue`, `plateValueToMarkdown`, `markdownPlugins`; the workspace buffer (`useBufferById`); the editor write seam `handleContentChange` from `useEditorAppStore.use.actions()` (same action Monaco's ContentSink calls — updates `buffer.content`, recomputes `isDirty = content !== savedContent`).
- Produces: `export function MarkdownEditorPane({ paneId, bufferId }: { paneId: string; bufferId: string })`.

Behavior:
1. On mount, read `buffer.content` and deserialize once into the Plate editor's initial value.
2. On Plate change, debounce (150 ms, matching the ContentSink cadence), serialize to markdown, and call `handleContentChange(md, …)` so the buffer goes dirty and Cmd+S persists it.
3. Expose a `flush` for the toggle/tab-switch path (Task 6) via the `flush-editor-content` window event — listen for it and flush the pending serialize immediately, mirroring Monaco.
4. Wrap the editor in the existing `ErrorBoundary`; on throw, the fallback sets this buffer's view to `source` (spec: fall back to Monaco) and shows a toast.

- [ ] **Step 1: Write the failing test**

```tsx
// web/src/__tests__/features/editor/markdown/plate/markdown-editor-pane.test.tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MarkdownEditorPane } from '@/features/editor/markdown/plate/markdown-editor-pane'

// Buffer + write-seam doubles. Adjust mock paths to the real module specifiers
// if they differ in the repo.
const handleContentChange = vi.fn()
vi.mock('@/features/workspace/stores/hooks/use-buffer-store', () => ({
  useBufferById: () => ({
    id: 'b1',
    type: 'editor',
    path: '/repo/README.md',
    name: 'README.md',
    content: '# Hello\n\nWorld **bold**.\n',
    savedContent: '# Hello\n\nWorld **bold**.\n',
    isDirty: false,
  }),
}))
vi.mock('@/features/editor/stores/editor-app-store', () => ({
  useEditorAppStore: { use: { actions: () => ({ handleContentChange }) } },
}))

describe('MarkdownEditorPane', () => {
  it('renders the markdown content as rich text', async () => {
    render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
    // The heading text is present in the rendered rich output.
    expect(await screen.findByText('Hello')).toBeInTheDocument()
    expect(screen.getByText('bold')).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bunx vitest run src/__tests__/features/editor/markdown/plate/markdown-editor-pane.test.tsx`
Expected: FAIL — module not found.

- [ ] **Step 3: Write the implementation**

```tsx
// web/src/features/editor/markdown/plate/markdown-editor-pane.tsx
import { useCallback, useEffect, useMemo, useRef } from 'react'
import { Plate, PlateContent, usePlateEditor, type Value } from 'platejs/react'
import { ErrorBoundary } from '@/components/error-boundary'
import { useBufferById } from '@/features/workspace/stores/hooks/use-buffer-store'
import { useEditorAppStore } from '@/features/editor/stores/editor-app-store'
import { hasTextContent } from '@/features/panes/types/pane-content'
import { useMarkdownViewStore } from './markdown-view-store'
import { markdownPlugins } from './markdown-plugins'
import { markdownToPlateValue, plateValueToMarkdown } from './markdown-serialization'

const SINK_DELAY_MS = 150

export function MarkdownEditorPane({
  paneId: _paneId,
  bufferId,
}: {
  paneId: string
  bufferId: string
}) {
  const buffer = useBufferById(bufferId)
  const { handleContentChange } = useEditorAppStore.use.actions()

  const initialContent = buffer && hasTextContent(buffer) ? buffer.content : ''
  // Deserialize ONCE for the initial value. Re-parsing on every keystroke would
  // fight the user's cursor; external reloads are handled by remounting via key.
  const initialValue = useMemo<Value>(
    () => markdownToPlateValue(initialContent),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- initial value only
    [],
  )

  const editor = usePlateEditor({ plugins: markdownPlugins, value: initialValue })

  // Trailing-debounce serialize -> buffer store, mirroring Monaco's ContentSink.
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const flush = useCallback(() => {
    if (timer.current) {
      clearTimeout(timer.current)
      timer.current = null
    }
    const md = plateValueToMarkdown(editor.children as Value)
    handleContentChange(md)
  }, [editor, handleContentChange])

  const onChange = useCallback(() => {
    if (timer.current) clearTimeout(timer.current)
    timer.current = setTimeout(flush, SINK_DELAY_MS)
  }, [flush])

  // Flush on the app-wide flush event (save / switch-away) and on unmount, so a
  // Cmd+S or a tab switch never races a stale in-memory value.
  useEffect(() => {
    const onFlush = () => flush()
    window.addEventListener('flush-editor-content', onFlush)
    return () => {
      window.removeEventListener('flush-editor-content', onFlush)
      flush()
    }
  }, [flush])

  const fallbackToSource = useCallback(() => {
    useMarkdownViewStore.getState().setView(bufferId, 'source')
  }, [bufferId])

  return (
    <ErrorBoundary
      fallback={
        <FallbackToSource onMount={fallbackToSource} />
      }
    >
      <Plate editor={editor} onChange={onChange}>
        <PlateContent
          className="markdown-content h-full overflow-auto p-6 outline-none"
          placeholder="Write…"
        />
      </Plate>
    </ErrorBoundary>
  )
}

// Rendered by the ErrorBoundary when Plate throws: switch this buffer to the
// Monaco source view and surface a toast (toast wired in Task 6's toolbar host,
// or replace with the app's toast API here).
function FallbackToSource({ onMount }: { onMount: () => void }) {
  useEffect(() => {
    onMount()
  }, [onMount])
  return (
    <div className="flex h-full items-center justify-center p-8 text-sm text-muted-foreground">
      Rich markdown failed to load — switching to source view.
    </div>
  )
}
```

*(Implementer notes: (a) confirm the `<Plate onChange>` signature per the Plate API note at the top; if it passes `({ value })`, you may serialize `value` directly instead of `editor.children`. (b) `useBufferById` and `useEditorAppStore.use.actions()` are the real hooks used by `code-editor.tsx` — verify the import specifiers. (c) if `handleContentChange` requires more args in this repo, pass them as `code-editor.tsx` does.)*

- [ ] **Step 4: Run test to verify it passes**

Run: `bunx vitest run src/__tests__/features/editor/markdown/plate/markdown-editor-pane.test.tsx`
Expected: PASS. If Plate needs a DOM API jsdom lacks (e.g. `getBoundingClientRect` returning zeros is fine; `Range`/`Selection` gaps may need a setup shim), add the shim to `web/src/__tests__/setup.ts` rather than weakening the test.

- [ ] **Step 5: Add a dirty-marking test**

Append to the same test file:
```tsx
import { fireEvent } from '@testing-library/react'
// ...
it('serializes edits to the buffer store (debounced)', async () => {
  vi.useFakeTimers()
  handleContentChange.mockClear()
  render(<MarkdownEditorPane paneId="p1" bufferId="b1" />)
  window.dispatchEvent(new Event('flush-editor-content'))
  expect(handleContentChange).toHaveBeenCalled()
  const md = handleContentChange.mock.calls[0][0] as string
  expect(md).toContain('# Hello')
  vi.useRealTimers()
})
```
Run: `bunx vitest run src/__tests__/features/editor/markdown/plate/markdown-editor-pane.test.tsx`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/editor/markdown/plate/markdown-editor-pane.tsx web/src/__tests__/features/editor/markdown/plate/markdown-editor-pane.test.tsx
git commit -m "feat(md): MarkdownEditorPane Plate surface wired to the buffer write seam"
```

---

### Task 6: Surface selector in `EditorPane` + Rich/Source toggle

**Files:**
- Modify: `web/src/features/panes/components/editor-pane.tsx`
- Create: `web/src/features/editor/markdown/plate/markdown-view-toggle.tsx`
- Modify: `web/src/features/editor/components/toolbar/editor-status-actions.tsx`
- Test: `web/src/__tests__/features/panes/components/editor-pane-surface-select.test.tsx`

**Interfaces:**
- Consumes: `isMarkdownPath`, `useMarkdownViewStore` + `selectMarkdownView`, `MarkdownEditorPane`.
- Produces: `EditorPane` renders `MarkdownEditorPane` for markdown buffers in `rich` view; `MarkdownViewToggle` flips the mode and flushes.

Rationale for putting the selector in `EditorPane` (not `pane-container.tsx`): the default `editor` case in `pane-container` already routes to `EditorPane`, and `EditorPane` holds the full buffer (`useBufferById`) and the Monaco-arming logic. Branching here keeps the delicate keep-alive/routing block in `pane-container` untouched (see its comments about Suspense boundaries), a smaller blast radius than the spec's original placement. This is an intentional refinement of the spec.

- [ ] **Step 1: Write the failing routing test**

```tsx
// web/src/__tests__/features/panes/components/editor-pane-surface-select.test.tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { useMarkdownViewStore } from '@/features/editor/markdown/plate/markdown-view-store'

vi.mock('@/features/editor/markdown/plate/markdown-editor-pane', () => ({
  MarkdownEditorPane: () => <div data-testid="plate-surface" />,
}))
vi.mock('@/features/editor/components/editor-surface', () => ({
  EditorSurface: () => <div data-testid="monaco-surface" />,
}))

let currentBuffer: { id: string; type: string; path: string; name: string; fileMissing?: boolean }
vi.mock('@/features/workspace/stores/hooks/use-buffer-store', () => ({
  useBufferById: () => currentBuffer,
}))
vi.mock('@/features/workspace/stores/workspace-context', () => ({
  useWorkspaceStore: () => ({ editorManager: {}, armEditor: async () => {} }),
}))

import { EditorPane } from '@/features/panes/components/editor-pane'

const baseProps = {
  paneId: 'p1',
  isActiveSurface: true,
  isPreview: false,
  onPromote: () => {},
}

describe('EditorPane surface selection', () => {
  it('renders Plate for a markdown buffer in rich view', () => {
    currentBuffer = { id: 'b1', type: 'editor', path: '/r/README.md', name: 'README.md' }
    useMarkdownViewStore.setState({ views: {} }) // default rich
    render(<EditorPane {...baseProps} bufferId="b1" />)
    expect(screen.getByTestId('plate-surface')).toBeInTheDocument()
    expect(screen.queryByTestId('monaco-surface')).toBeNull()
  })

  it('renders Monaco for a markdown buffer in source view', () => {
    currentBuffer = { id: 'b2', type: 'editor', path: '/r/README.md', name: 'README.md' }
    useMarkdownViewStore.setState({ views: { b2: 'source' } })
    render(<EditorPane {...baseProps} bufferId="b2" />)
    expect(screen.getByTestId('monaco-surface')).toBeInTheDocument()
  })

  it('renders Monaco for a non-markdown buffer', () => {
    currentBuffer = { id: 'b3', type: 'editor', path: '/r/main.ts', name: 'main.ts' }
    useMarkdownViewStore.setState({ views: {} })
    render(<EditorPane {...baseProps} bufferId="b3" />)
    expect(screen.getByTestId('monaco-surface')).toBeInTheDocument()
    expect(screen.queryByTestId('plate-surface')).toBeNull()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bunx vitest run src/__tests__/features/panes/components/editor-pane-surface-select.test.tsx`
Expected: FAIL — `EditorPane` still always renders Monaco (`monaco-surface`) / Plate branch missing.

- [ ] **Step 3: Add the selector branch to `EditorPane`**

In `web/src/features/panes/components/editor-pane.tsx`, add imports and a branch **before** the Monaco-arming block (after the `fileMissing` guard at line ~58–67). Use a lazy import so Plate stays out of the base pane chunk:

```tsx
// add near the top imports
import { lazy, Suspense } from 'react'
import { useMarkdownViewStore, selectMarkdownView } from '@/features/editor/markdown/plate/markdown-view-store'
import { isMarkdownPath } from '@/features/editor/markdown/plate/is-markdown-path'

const MarkdownEditorPane = lazy(() =>
  import('@/features/editor/markdown/plate/markdown-editor-pane').then((m) => ({
    default: m.MarkdownEditorPane,
  })),
)
```

Then, inside the component after `const buffer = useBufferById(bufferId)` and the `fileMissing` guard:

```tsx
const markdownView = useMarkdownViewStore(
  buffer ? selectMarkdownView(buffer.id) : () => 'rich' as const,
)
const isMarkdown = !!buffer && isMarkdownPath(buffer.path)

if (isMarkdown && markdownView === 'rich') {
  return (
    <Suspense fallback={null}>
      <MarkdownEditorPane paneId={paneId} bufferId={bufferId} />
    </Suspense>
  )
}
```

Leave the existing Monaco arm + `EditorSurface` path as the fall-through for everything else.

- [ ] **Step 4: Run the routing test**

Run: `bunx vitest run src/__tests__/features/panes/components/editor-pane-surface-select.test.tsx`
Expected: PASS (all three cases).

- [ ] **Step 5: Create the toggle component**

```tsx
// web/src/features/editor/markdown/plate/markdown-view-toggle.tsx
import { Eye, Code } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { useMarkdownViewStore, selectMarkdownView } from './markdown-view-store'

/** Rich <-> Source toggle for a markdown buffer. Flushes any pending rich-editor
 *  serialize before switching so the source view opens on the latest text. */
export function MarkdownViewToggle({ bufferId }: { bufferId: string }) {
  const view = useMarkdownViewStore(selectMarkdownView(bufferId))
  const toggleView = useMarkdownViewStore((s) => s.toggleView)

  const onClick = () => {
    // Flush the rich editor's debounced write before leaving it.
    window.dispatchEvent(new Event('flush-editor-content'))
    toggleView(bufferId)
  }

  return (
    <Button
      variant="ghost"
      size="sm"
      onClick={onClick}
      title={view === 'rich' ? 'View markdown source' : 'View rich editor'}
    >
      {view === 'rich' ? <Code /> : <Eye />}
    </Button>
  )
}
```

- [ ] **Step 6: Render the toggle for markdown buffers in the editor toolbar**

In `web/src/features/editor/components/toolbar/editor-status-actions.tsx`, import `MarkdownViewToggle` and `isMarkdownPath`, and render `<MarkdownViewToggle bufferId={activeBuffer.id} />` when `activeBuffer` is an editor buffer whose path `isMarkdownPath`. (In `rich` mode this toolbar is not shown — the rich surface bypasses `EditorSurface` — so in `rich` mode also surface the toggle inside `MarkdownEditorPane`'s own top-right corner: add the same `<MarkdownViewToggle bufferId={bufferId} />` to `markdown-editor-pane.tsx` as an absolutely-positioned control. This guarantees the toggle is reachable in both views.)

Add to `markdown-editor-pane.tsx` inside the returned `<Plate>` wrapper, above `<PlateContent>`:
```tsx
<div className="absolute right-2 top-2 z-10">
  <MarkdownViewToggle bufferId={bufferId} />
</div>
```
and `import { MarkdownViewToggle } from './markdown-view-toggle'`, and make the pane's root `relative`.

- [ ] **Step 7: Full typecheck + targeted tests + lint**

Run (from `web/`):
```bash
bunx tsc --noEmit
bunx vitest run src/__tests__/features/editor/markdown src/__tests__/features/panes/components/editor-pane-surface-select.test.tsx
bun run lint
```
Expected: all PASS/clean.

- [ ] **Step 8: Commit**

```bash
git add web/src/features/panes/components/editor-pane.tsx web/src/features/editor/markdown/plate/markdown-view-toggle.tsx web/src/features/editor/components/toolbar/editor-status-actions.tsx web/src/__tests__/features/panes/components/editor-pane-surface-select.test.tsx
git commit -m "feat(md): route markdown buffers to Plate with a Rich/Source toggle"
```

---

### Task 7: Code-split verification + full suite + live Tauri verification

**Files:** none (verification only). Fix-ups land in the relevant task's files if a check fails.

- [ ] **Step 1: Confirm Plate is code-split out of the entry chunk**

Run (from `web/`):
```bash
bunx vite build >/dev/null 2>&1
echo "entry chunks referencing platejs (expect NONE):"
grep -rl "platejs" dist/assets/*.js | xargs -I{} basename {} | grep -iE "index|entry|main" || echo "clean"
```
Expected: `clean` — Plate appears only in a lazily-loaded chunk, never the entry. If it leaks, ensure `MarkdownEditorPane` is imported only via the `lazy(() => import(...))` in `editor-pane.tsx` and nowhere eagerly.

- [ ] **Step 2: Run the full web test suite**

Run (from `web/`): `bunx vitest run`
Expected: PASS. Investigate any regression in editor/pane suites caused by the new branch.

- [ ] **Step 3: Live Tauri verification (serialized, on the single running dev instance)**

Drive the already-running dev Tauri app (never spawn a second — memory: one-dev-instance). Via the Tauri MCP, verify in order:
1. Open a real repo `.md` (e.g. this plan or a README) → it renders as a **rich Plate editor**, not Monaco.
2. Type/edit: heading, bold, a bullet list, a task checkbox, a table, a fenced code block, a link → all behave as rich blocks; the slash menu and drag handle work.
3. The tab shows a **dirty** indicator after editing.
4. **Cmd+S** → `git diff` on the file shows a sane, localized change (accepting canonicalization); no content loss.
5. Click the **toggle** → the same file opens in **Monaco** showing the current markdown text; edit one char; toggle back → Plate reflects it.
6. Open a `.ts` file → still Monaco (no regression). Open a `.mdx` file → still Monaco (excluded by design).
7. Confirm light/dark theme and UI font apply to the rich editor.

Capture a screenshot/GIF of the rich editor for the handoff. Record any defect as a follow-up task; do not mark Slice 1 done until 1–6 pass.

- [ ] **Step 4: Commit any verification fix-ups**

Only if Step 1–3 required code changes:
```bash
git add -A web/src
git commit -m "fix(md): address Slice 1 verification findings"
```

---

## Self-Review (performed against the spec)

- **Spec coverage (Slice 1 subset):** surface selector → Task 6; markdown-as-source-of-truth + write seam → Tasks 4–5; Rich⇄Source toggle → Task 6; GFM feature set → Tasks 1,4 (kits); lazy/code-split → Tasks 1,6,7; theming/perf/live verify → Task 7; error fallback to source → Task 5. Frontmatter, extended plugins, image display, and preview retirement are **explicitly deferred** to Slices 2–5 (below), consistent with the spec's staged rollout.
- **Placeholder scan:** no TBD/TODO in our logic. Two flagged *external-API confirmation* notes (the `<Plate onChange>` signature; the exact generated kit export names) are legitimate verify-against-docs steps, not deferred work — each has a concrete default and a check.
- **Type consistency:** `MarkdownView`, `selectMarkdownView`, `useMarkdownViewStore`, `markdownPlugins`, `markdownToPlateValue`/`plateValueToMarkdown`, `MarkdownEditorPane`, `MarkdownViewToggle`, `isMarkdownPath` are used with identical names/signatures across Tasks 2–6.

## Staged roadmap (Slices 2–5 — each its own plan after Slice 1 is live-verified)

- **Slice 2 — Frontmatter verbatim:** `remark-frontmatter` + a dedicated Plate node that carries the raw `--- … ---` bytes and re-emits them unchanged. Round-trip test asserts **byte-exact** frontmatter. Depends on: the Task 4 plugin set.
- **Slice 3 — Extended plugins (one task each):** math (`remark-math` + KaTeX, lazy), footnotes, callouts/admonitions (`> [!NOTE]` syntax), Mermaid (fenced ```` ```mermaid ````, lazy, themed). Each ships its own round-trip fixtures and can land independently.
- **Slice 4 — Local image display:** resolve relative/root paths against the file dir and load via Tauri's asset protocol (port `resolvePath` from the retiring `markdown-preview.tsx`). Display-only; no upload.
- **Slice 5 — Retire the old preview path:** remove `markdownPreview` from `PaneContentType` / `MarkdownPreviewContent` / `OpenContentSpec` / `pane-container.tsx` case / `hasTextContent`; delete `markdown-preview.tsx` and `parser.ts`; drop the markdown branch of `breadcrumb.tsx` `handlePreviewClick` (keep html/csv). Do this **last**, once Plate is proven, so the fallback isn't removed before its replacement is trusted.
