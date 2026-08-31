# Unified Sidebar — Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewire the sidebar and pane surface to the closed 2026-08-28 design:
one sidebar (one tree per project-space, one Recents band), one row component,
one drag arm, a pane holding exactly one chat plus an editor view.

**Architecture:** Reuse existing components (`ROW_BASE`, `ui/tabs.tsx`
underline variant, `sidebar-carousel.tsx`'s carousel pattern, `pane-sash.tsx`,
`pane-drop-zones.ts`, `pane-border.ts`) under new wiring. Three places the
*data model* itself must change, not just the wiring, found during
reconnaissance and not visible from the spec text alone — flagged up front so
no task below treats them as a bigger rewiring than they are:

1. **`PaneGroup.bufferIds: string[]` holds a mixed tab strip today**
   (`editor|terminal|newTab|commitDiff|agentChat|...`, ten variants,
   `features/panes/types/pane-content.ts`). Law 3 ("a pane holds exactly one
   chat") and the two-views model (chat view + editor view containing
   files/terminals/branch-review as tabs) require an actual type change here,
   not a restyle.
2. **Two independent drag systems exist** — `workspace-tree-context.tsx`
   (sidebar) and `use-agent-chats-drag.ts` (chats) — structurally similar
   (same 5px threshold, same refs-only-in-one-`useEffect` pattern) but
   separately built, on separate policy modules (`drop-rules.ts` vs.
   `chat-drop.ts`) over the same generic core (`tree-dnd/drop-core.ts`,
   `drop-dom.ts`). Law 2 ("one row, one drag") is a real merge.
3. **A drag-to-pane gesture already exists and means the opposite of what
   the new spec wants.** `editor-removal-overlay.tsx` (`PANE_ARM_MS=150`,
   shared by both trees) arms a dwell-gated *removal* when a row is dragged
   onto a pane. The new spec's law 6 ("every drop adds") and §8 rules (pane
   middle = open/merge) need this deleted, not extended.

**Tech Stack:** React, Zustand (workspace-store-registry + a new window-level
store this plan introduces), IndexedDB (`idb`) for layout persistence,
Vitest + Testing Library.

**Spec:** [`docs/superpowers/specs/2026-08-28-sidebar-and-pane-surface-design.md`](../specs/2026-08-28-sidebar-and-pane-surface-design.md)
(read first — this plan does not repeat its rationale, only its
implementation) + [`docs/superpowers/specs/2026-08-23-unified-sidebar-design.md`](../specs/2026-08-23-unified-sidebar-design.md)
§5.1 for the wire contract this plan's data layer consumes.

**Companion plan:** [`docs/superpowers/plans/2026-08-28-sidebar-backend-implementation.md`](./2026-08-28-sidebar-backend-implementation.md).
Task 8.1 below (route consumption) depends on that plan's Stage 7. Every other
task in this plan is independent of it — build the surface first against the
existing routes, swap the data layer last, per both specs' "no migration"
stance (there's nothing to keep working during the swap, so sequence for
engineering convenience, not compatibility).

## Global Constraints

- **Component files are kebab-case**: `my-component.tsx`, not
  `MyComponent.tsx`. Exported component name stays PascalCase.
- **Tests live in `web/src/__tests__/...` mirroring `web/src/...`.** A test
  for `web/src/features/foo/bar.tsx` goes in
  `web/src/__tests__/features/foo/bar.test.tsx`. Use `@/` imports in test
  files, never relative `../../`.
- **Store selectors are narrow.** `useXxxStore((s) => s.specificField)`,
  never `useXxxStore()` with no selector. `useXxxStore.getState()` only
  inside event handlers and `useEffect` bodies, never in render.
- **Stores do not import from `components/`.** Side effects (toasts, DOM
  writes) live in components watching store state via `useEffect`.
- **No migration, no legacy path, no compatibility shim.** Build as though
  Crowbar ships for the first time. Delete superseded code outright — do not
  leave it behind a flag.
- **Reuse over manufacture.** Every task below names the existing component
  it rewires. If a task's implementation needs a genuinely new component,
  that's a signal to re-read the relevant spec section — the spec's own law
  10 treats this as the exception, not the default.
- **Test command:** `bun test` (repo's Vitest wrapper — confirmed
  `package.json`'s `"test": "vitest run"`, `"test:coverage": "vitest run
  --coverage"`). Use `bun`, not `bunx`, for `tsc` — `bunx tsc` resolves a
  different, wrong package in this repo.
- **Run before every commit:** `bun test` and `bun tsc --noEmit` (or the
  package.json equivalent script if one wraps it) both clean.

---

## Part A — Data layer: the mixed pane model becomes one-chat-per-pane

*This is the task the rest of the plan depends on. Do it first.*

### Task 1: `PaneGroup` narrows to one chat plus an editor-view tab strip

**Files:**
- Modify: `web/src/features/panes/types/pane.ts` — `PaneGroup`'s shape
  changes.
- Modify: `web/src/features/panes/types/pane-content.ts` — the ten-variant
  `PaneContentType` union splits into two: the chat itself (not a member of
  the tab strip any more) and everything else (the editor view's tabs).
- Modify: `web/src/features/workspace/stores/slices/pane-slice.ts` — every
  action reading/writing `bufferIds` updates to the new shape.
- Test: `web/src/__tests__/features/panes/types/pane.test.ts` (new)

**Interfaces:**
- Consumes: nothing (this is the foundational type change).
- Produces the shape every later task in this plan builds on:
  ```ts
  interface PaneGroup {
    id: string
    type: 'group'
    chatId: string | null       // the pane's one chat; null = the empty stage
    editorTabIds: string[]      // everything the editor view holds: files, terminals, branch review
    activeEditorTabId: string | null
    editorOpen: boolean         // split toggle state — chat-only vs. chat+editor
    locked?: boolean
  }

  type EditorTabContentType =
    | 'editor' | 'terminal' | 'commitDiff' | 'markdownPreview'
    | 'htmlPreview' | 'csvPreview' | 'externalEditor' | 'branchReview'

  interface EditorTabBase {
    id: string
    type: EditorTabContentType
    path?: string
    name: string
    isPinned?: boolean
    isPreview?: boolean
  }
  ```
  `agentChat` and `newTab` leave the content-type union entirely: a pane's
  chat is `PaneGroup.chatId`, never a tab; the empty stage is `chatId === null`,
  never a `newTab` buffer. `AgentChatContent`'s re-pointing behavior
  (`chatId`/`runnerId` both mutable post-creation, confirmed by reconnaissance
  as load-bearing for `/clear`/`/resume`/provider-switch) becomes `PaneGroup`
  fields directly: add `runnerId: string | null` alongside `chatId`.

- [ ] **Step 1: Write the failing test**

```ts
import { describe, expect, it } from 'vitest'
import type { PaneGroup } from '@/features/panes/types/pane'

describe('PaneGroup', () => {
  it('holds exactly one chat, never a list', () => {
    const pane: PaneGroup = {
      id: 'pane-1',
      type: 'group',
      chatId: 'chat-1',
      runnerId: 'runner-1',
      editorTabIds: [],
      activeEditorTabId: null,
      editorOpen: false,
    }
    expect(pane.chatId).toBe('chat-1')
    expect(pane).not.toHaveProperty('bufferIds')
  })

  it('an empty pane has chatId null, not a newTab buffer', () => {
    const pane: PaneGroup = {
      id: 'pane-1',
      type: 'group',
      chatId: null,
      runnerId: null,
      editorTabIds: [],
      activeEditorTabId: null,
      editorOpen: false,
    }
    expect(pane.chatId).toBeNull()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bun test web/src/__tests__/features/panes/types/pane.test.ts`
Expected: FAIL — `chatId`/`runnerId` don't exist on today's `PaneGroup`,
`bufferIds` does.

- [ ] **Step 3: Implement the type change**

Replace `PaneGroup`'s `bufferIds: string[], activeBufferId: string | null,
mruBufferIds?, previewBufferId?, pinnedBufferIds?` with the shape above. Split
`pane-content.ts`'s union: delete `AgentChatContent` and `NewTabContent` from
it; the remaining eight variants become `EditorTabContentType`. Rename the
file's exported union from `PaneContentType`/`OpenContentSpec` to
`EditorTabContentType`/`OpenEditorTabSpec` — grep every consumer
(`grep -rl "PaneContentType\|OpenContentSpec" web/src`) and update each to the
new names; this is a rename, not a new type, so every existing call site keeps
its meaning, just under the new name.

- [ ] **Step 4: Run test to verify it passes**

Run: `bun test web/src/__tests__/features/panes/types/pane.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/panes/types/pane.ts web/src/features/panes/types/pane-content.ts web/src/__tests__/features/panes/types/pane.test.ts
git commit -m "feat(panes): PaneGroup holds one chat, not a mixed buffer list"
```

### Task 2: `pane-slice.ts` actions migrate to the new shape

**Files:**
- Modify: `web/src/features/workspace/stores/slices/pane-slice.ts` — every
  one of the 20 `PaneActions` methods that referenced `bufferIds` changes.
  `splitPane`, `closePane`, `setActivePane`, `resizePaneSplit`,
  `distributePaneSplit`, `togglePaneFullscreen`, `exitPaneFullscreen`,
  `getAllPaneGroups`, `getPaneById`, `navigateToPane` are unaffected in shape
  (they operate on the tree/geometry, not buffer membership) — leave them as
  is. `activatePaneBuffer`, `addBufferToPane`, `removeBufferFromPane`,
  `moveBufferToPane`, `setPanePreviewBuffer`, `setPaneBufferPinned`,
  `reorderPaneBuffers`, `getPaneByBufferId`, `clearPreviewBufferEverywhere`,
  `switchToNextBufferInPane`, `switchToPreviousBufferInPane` are the ten that
  change: each becomes an *editor-tab* operation (renamed with an `EditorTab`
  suffix, e.g. `addEditorTabToPane`) operating on `editorTabIds`, since a
  chat is no longer a member of that list to add/remove/reorder.
- New action: `setPaneChat(paneId: string, chatId: string | null, runnerId: string | null): void`
  — the one write path for what chat a pane holds; every place that used to
  call `addBufferToPane` with an `agentChat` content spec now calls this
  instead.
- Test: `web/src/__tests__/features/workspace/stores/slices/pane-slice.test.ts`
  (extend the existing file — find it first, since one likely already exists
  given the slice's age; if none exists, create it fresh following the house
  pattern from a sibling slice test).

**Interfaces:**
- Consumes: `PaneGroup` (Task 1).
- Produces: `setPaneChat`, and the renamed `EditorTab`-suffixed action set —
  consumed by every component task in Parts B-E below. The exact renamed
  names, so later tasks cite them consistently:
  ```ts
  activateEditorTabInPane(paneId: string, tabId: string): void
  addEditorTabToPane(paneId: string, tab: EditorTabBase): void
  removeEditorTabFromPane(paneId: string, tabId: string): void
  moveEditorTabToPane(tabId: string, fromPaneId: string, toPaneId: string): void
  setEditorTabPreview(paneId: string, tabId: string): void
  setEditorTabPinned(paneId: string, tabId: string, pinned: boolean): void
  reorderEditorTabs(paneId: string, tabId: string, targetIndex: number): void
  getPaneByEditorTabId(tabId: string): PaneGroup | null
  clearEditorTabPreviewEverywhere(): void
  switchToNextEditorTab(paneId: string): void
  switchToPreviousEditorTab(paneId: string): void
  setPaneChat(paneId: string, chatId: string | null, runnerId: string | null): void
  ```

- [ ] **Step 1: Write the failing test**

```ts
import { describe, expect, it, beforeEach } from 'vitest'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

describe('pane-slice: setPaneChat', () => {
  it('sets exactly one chat on a pane, replacing any prior one', () => {
    const store = createWorkspaceStore('ws-test')
    const paneId = store.getState().panes['root-pane']?.id ?? 'root-pane'
    store.getState().setPaneChat(paneId, 'chat-1', 'runner-1')
    expect(store.getState().getPaneById(paneId)?.chatId).toBe('chat-1')
    store.getState().setPaneChat(paneId, 'chat-2', 'runner-2')
    expect(store.getState().getPaneById(paneId)?.chatId).toBe('chat-2')
  })

  it('editor tabs are independent of the chat', () => {
    const store = createWorkspaceStore('ws-test')
    const paneId = store.getState().panes['root-pane']?.id ?? 'root-pane'
    store.getState().setPaneChat(paneId, 'chat-1', 'runner-1')
    store.getState().addEditorTabToPane(paneId, { id: 'file-1', type: 'editor', name: 'foo.ts' })
    expect(store.getState().getPaneById(paneId)?.editorTabIds).toContain('file-1')
    expect(store.getState().getPaneById(paneId)?.chatId).toBe('chat-1')
  })
})
```

(Match `createWorkspaceStore`'s real constructor/import path — confirm the
exact factory name in `workspace-store-registry.ts` before writing this file
for real; it may be `getOrCreateWorkspaceStore` rather than a bare
`createWorkspaceStore`.)

- [ ] **Step 2: Run, verify fail**

Run: `bun test web/src/__tests__/features/workspace/stores/slices/pane-slice.test.ts`

- [ ] **Step 3: Implement** the renamed actions and `setPaneChat`, each a
  small, direct `set()` mutation on `state.panes[paneId]`, following the
  slice's existing immer-draft convention (check the top of `pane-slice.ts`
  for whether it uses `immer` middleware or manual spread — match whichever
  it already uses, don't introduce a second pattern).

- [ ] **Step 4: Run, verify pass**

Run: `bun test web/src/__tests__/features/workspace/stores/slices/pane-slice.test.ts`

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/stores/slices/pane-slice.ts web/src/__tests__/features/workspace/stores/slices/pane-slice.test.ts
git commit -m "feat(panes): pane-slice actions operate on one chat plus an editor tab strip"
```

### Task 3: `buffer-slice.ts`'s `isUncloseable` mechanic carries over to Recents' working row

**Files:**
- Modify: `web/src/features/workspace/stores/slices/buffer-slice.ts` — confirm
  `isUncloseable` still applies to editor tabs (a lone unpinned tab in an
  editor view, mirroring the sole-New-Tab rule) after Task 2's rename; no
  behavior change needed here, just confirm the invariant survives the type
  change (`buf.isUncloseable = pane.bufferIds.length === 1` becomes
  `tab.isUncloseable = pane.editorTabIds.length === 1`, keyed on
  `editorTabIds` not the old field).
- Test: extend whatever test already covers `syncNewTabCloseability` (grep
  for it first — `grep -rl syncNewTabCloseability web/src/__tests__`).

- [ ] **Step 1: Write the failing test**

```ts
it('the sole editor tab in a pane is uncloseable', () => {
  const store = createWorkspaceStore('ws-test')
  const paneId = 'pane-1'
  store.getState().addEditorTabToPane(paneId, { id: 'file-1', type: 'editor', name: 'foo.ts' })
  expect(store.getState().getPaneById(paneId)?.editorTabIds).toHaveLength(1)
  const tab = store.getState().getBufferById?.('file-1') // or wherever isUncloseable now lives after A.2
  expect(tab?.isUncloseable).toBe(true)
})
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Update `syncNewTabCloseability`'s condition** from
  `pane.bufferIds.length === 1` to `pane.editorTabIds.length === 1`, and
  rename the function if its name still says "NewTab" (the concept it guards
  is now "sole editor tab," not specifically a new-tab placeholder — rename to
  `syncSoleEditorTabCloseability` if the old name would now mislead a reader;
  keep the old name if the file's own doc comment already generalizes it).

- [ ] **Step 4: Run**

Run: `bun test web/src/__tests__/features/workspace/stores/slices/buffer-slice.test.ts`

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/stores/slices/buffer-slice.ts
git commit -m "fix(panes): sole-editor-tab closeability keys on editorTabIds after the pane-model migration"
```

---

## Part B — One tree

*Merges the two sidebar trees (`components/layout/` workspace tree,
`features/agent/tree/` chats tree) into one, per law 1 and law 2. Depends on
Part A only for type names it references; the tree's own row data does not
depend on the pane model.*

### Task 4: One row-kind model — `SidebarRow` union

**Files:**
- Create: `web/src/components/sidebar/types/sidebar-row.ts`
- Test: `web/src/__tests__/components/sidebar/types/sidebar-row.test.ts`

**Interfaces:**
- Produces the type every row-rendering task below consumes:
  ```ts
  type SidebarRowKind = 'chat' | 'branch' | 'folder' | 'workflow'

  interface SidebarRow {
    id: string
    kind: SidebarRowKind
    parentId: string | null
    order: number
    label: string
    labelProvisional?: boolean
    ownsWorktree: boolean
    workspaceId: string | null
    working: boolean
    hasView: boolean          // grey label — a chat with a live or dormant view
    branchName?: string       // only when ownsWorktree
  }
  ```
  This is the frontend mirror of the backend plan's `domain.Chat` +
  pre-resolved walks (backend plan Task 3.1, Stage 7's "each row ships its
  walks already resolved") — until the backend plan's Stage 7 lands, Task
  B.5 below adapts today's `Repo`/`Workspace`/`ChatFolder` shapes into this
  union client-side as a bridge; once the new routes exist, that bridge
  collapses to a direct API mapping (Task 15).

- [ ] **Step 1: Write the failing test**

```ts
import { describe, expect, it } from 'vitest'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

describe('SidebarRow', () => {
  it('the four kinds are the whole taxonomy', () => {
    const kinds: SidebarRow['kind'][] = ['chat', 'branch', 'folder', 'workflow']
    expect(kinds).toHaveLength(4)
  })
})
```

- [ ] **Step 2: Run, verify fail** (file doesn't exist)

- [ ] **Step 3: Implement** the type as above.

- [ ] **Step 4: Run, verify pass**

- [ ] **Step 5: Commit**

```bash
git add web/src/components/sidebar/types/sidebar-row.ts web/src/__tests__/components/sidebar/types/sidebar-row.test.ts
git commit -m "feat(sidebar): SidebarRow — the one row-kind union both trees converge on"
```

### Task 5: One `<SidebarRow>` component, built on `ROW_BASE`

**Files:**
- Create: `web/src/components/sidebar/sidebar-row.tsx`
- Test: `web/src/__tests__/components/sidebar/sidebar-row.test.tsx`

**Interfaces:**
- Consumes: `SidebarRow` (Task 4), `ROW_BASE`/`ROW_ACTIVE`/`ROW_SUB_ACTION`
  etc. (`web/src/components/layout/workspace-row-base.ts`, confirmed to be a
  class-string module, not a component — this task is the first place a real
  `<SidebarRow>` component gets built from those tokens, consumed by both the
  tree and Recents rather than each row type hand-rolling its own markup).
- Produces:
  ```tsx
  interface SidebarRowProps {
    row: SidebarRow
    depth: number            // 0 for a Recents entry — no indent there (spec §5.1)
    onOpen: (id: string) => void
    onTrash?: (id: string) => void
    onCreate?: (id: string, kind: 'workspace' | 'thread') => void
    onToggleFold?: (id: string) => void
    folded?: boolean
    dragProps?: Record<string, string>  // spread from drop-dom's createDropRowDom, Part D
  }
  function SidebarRow(props: SidebarRowProps): JSX.Element
  ```

- [ ] **Step 1: Write the failing test**

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { SidebarRow } from '@/components/sidebar/sidebar-row'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'

const baseRow: SidebarRowType = {
  id: 'row-1', kind: 'chat', parentId: null, order: 0, label: 'Fix the thing',
  ownsWorktree: false, workspaceId: null, working: false, hasView: false,
}

describe('SidebarRow', () => {
  it('renders the label', () => {
    render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
    expect(screen.getByText('Fix the thing')).toBeInTheDocument()
  })

  it('a row with a view greys its label, focused or not', () => {
    render(<SidebarRow row={{ ...baseRow, hasView: true }} depth={0} onOpen={vi.fn()} />)
    const label = screen.getByText('Fix the thing')
    expect(label.className).toMatch(/text-muted-foreground|opacity/)
  })

  it('a working row shows the spinner glyph, not the static mark', () => {
    render(<SidebarRow row={{ ...baseRow, working: true }} depth={0} onOpen={vi.fn()} />)
    expect(screen.getByTestId('flip-dot-spinner')).toBeInTheDocument()
  })

  it('trailing controls are trash, +, chevron in that order, revealed on hover', () => {
    render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} onTrash={vi.fn()} onCreate={vi.fn()} onToggleFold={vi.fn()} />)
    const controls = screen.getAllByRole('button')
    expect(controls.map((c) => c.getAttribute('data-control'))).toEqual(['trash', 'create', 'fold'])
  })
})
```

- [ ] **Step 2: Run, verify fail** (component doesn't exist)

- [ ] **Step 3: Implement**, anatomy exactly per spec §3.1:
  `[glyph][label(+sub-line)] [trash][+][chevron]`, `h-9 px-1.5 mx-1.5 my-0.5
  gap-1.5 text-[13px]` from `ROW_BASE`, right inset 10px (spec's stated
  deviation from `ROW_BASE`'s own 6px — apply as an override class on this
  component, not a change to the shared token), leading glyph 6px inset. Every
  hand-rolled mark (`chevron`, `+`, `trash`) `size-3`; leading glyph 16px
  (20px when `row.kind === 'branch' && row.parentId === null`, i.e. the
  project-home case). No second line ever (spec §3.3 — this is a hard
  simplification versus every existing row implementation, which all still
  draw one). `working` swaps the glyph for the flip-dot spinner in place
  (find and reuse the existing spinner component — grep
  `flip-dot|FlipDot` in `web/src/components` — do not build a new one). `hasView`
  applies a muted-foreground/opacity treatment to the label only, mark stays
  full strength.

- [ ] **Step 4: Run, verify pass**

Run: `bun test web/src/__tests__/components/sidebar/sidebar-row.test.tsx`

- [ ] **Step 5: Commit**

```bash
git add web/src/components/sidebar/sidebar-row.tsx web/src/__tests__/components/sidebar/sidebar-row.test.tsx
git commit -m "feat(sidebar): one SidebarRow component, built on ROW_BASE tokens"
```

### Task 6: One tree component consuming `SidebarRow[]`

**Files:**
- Create: `web/src/components/sidebar/sidebar-tree.tsx`
- Test: `web/src/__tests__/components/sidebar/sidebar-tree.test.tsx`
- Do not modify or delete `workspace-tree.tsx` / `agent-chats-panel.tsx` yet —
  Task 8 retires them once this component is proven and wired.

**Interfaces:**
- Consumes: `SidebarRow` (B.1), `SidebarRow` component (B.2).
- Produces:
  ```tsx
  interface SidebarTreeProps {
    rows: SidebarRow[]     // one project's rows, flat, parentId-linked
    onOpen: (id: string) => void
    onTrash: (id: string) => void
    onCreate: (parentId: string, kind: 'workspace' | 'thread') => void
  }
  function SidebarTree(props: SidebarTreeProps): JSX.Element
  ```
  No rules between rows (spec §4 — "no rules between rows," the horizontal
  separators that divided projects are gone since a space holds one project).
  Fold state is local component state keyed by row id, not a store field —
  matches the existing `collapsedChatRows` pattern living in the always-alive
  `useSidebarStore`, reused here rather than reinvented (import that specific
  slice, don't add a second fold-state store).

- [ ] **Step 1: Write the failing test**

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { SidebarTree } from '@/components/sidebar/sidebar-tree'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

const rows: SidebarRow[] = [
  { id: 'folder-1', kind: 'folder', parentId: null, order: 0, label: 'Bugs', ownsWorktree: false, workspaceId: null, working: false, hasView: false },
  { id: 'chat-1', kind: 'chat', parentId: 'folder-1', order: 0, label: 'Fix the thing', ownsWorktree: false, workspaceId: null, working: false, hasView: false },
]

describe('SidebarTree', () => {
  it('renders a row per entry, nested under its parent', () => {
    render(<SidebarTree rows={rows} onOpen={vi.fn()} onTrash={vi.fn()} onCreate={vi.fn()} />)
    expect(screen.getByText('Bugs')).toBeInTheDocument()
    expect(screen.getByText('Fix the thing')).toBeInTheDocument()
  })

  it('folding a container hides its descendants', () => {
    render(<SidebarTree rows={rows} onOpen={vi.fn()} onTrash={vi.fn()} onCreate={vi.fn()} />)
    screen.getByTestId('fold-folder-1').click()
    expect(screen.queryByText('Fix the thing')).not.toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement** — walk `rows` by `parentId` into a tree, render
  recursively with `depth` incrementing per level, `ROW_INDENT_STEP=14` per
  level (from `workspace-row-base.ts`, confirmed constant — reuse it, don't
  hardcode 14 again). An empty container renders the one affordance row (spec
  §3.5) instead of nothing.

- [ ] **Step 4: Run, verify pass**

- [ ] **Step 5: Commit**

```bash
git add web/src/components/sidebar/sidebar-tree.tsx web/src/__tests__/components/sidebar/sidebar-tree.test.tsx
git commit -m "feat(sidebar): SidebarTree — one tree over SidebarRow[], no rules between rows"
```

### Task 7: The affordance row (empty-container create control)

**Files:**
- Create: `web/src/components/sidebar/affordance-row.tsx`
- Test: `web/src/__tests__/components/sidebar/affordance-row.test.tsx`

**Interfaces:**
- Consumes: nothing beyond React/the design tokens.
- Produces:
  ```tsx
  interface AffordanceRowProps {
    onCreateThread: () => void
    onCreateWorkspace?: () => void   // present only when the parent is git-capable
  }
  function AffordanceRow(props: AffordanceRowProps): JSX.Element
  ```

- [ ] **Step 1: Write the failing test**

```tsx
it('shows only the bubble icon when the parent is not git-capable', () => {
  render(<AffordanceRow onCreateThread={vi.fn()} />)
  expect(screen.queryByTestId('affordance-dropdown')).not.toBeInTheDocument()
})

it('shows a split-control dropdown when a workspace is also legal', () => {
  render(<AffordanceRow onCreateThread={vi.fn()} onCreateWorkspace={vi.fn()} />)
  expect(screen.getByTestId('affordance-dropdown')).toBeInTheDocument()
})
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement** — icon-only, no subtitle, no visible dropdown
  chrome at rest; the menu appears on click only when `onCreateWorkspace` is
  provided.

- [ ] **Step 4: Run, verify pass**

- [ ] **Step 5: Commit**

```bash
git add web/src/components/sidebar/affordance-row.tsx web/src/__tests__/components/sidebar/affordance-row.test.tsx
git commit -m "feat(sidebar): the affordance row — the only way to fill an empty container"
```

### Task 8: Bridge — adapt `Repo`/`Workspace`/`ChatFolder` into `SidebarRow[]`, retire the two old trees

**Files:**
- Create: `web/src/components/sidebar/lib/rows-from-repo.ts`
- Delete: `web/src/components/layout/workspace-tree.tsx`,
  `workspace-tree-item.tsx`, `repo-section.tsx`, `project-home-row.tsx`,
  `folder-row.tsx` and every other file in that tree exclusively used by it
  (confirm each is dead via `grep -rl` before deleting — some, like
  `sidebar-project-header.tsx`, are reused by Part C and must stay).
- Delete: `features/agent/tree/agent-chats-panel.tsx`, `agent-chat-row.tsx`,
  `agent-chat-folder-row.tsx`, `lib/chat-rows.ts`, `lib/shown-chats.ts` (its
  "what's shown" concept is superseded by Recents, Part C).
- Test: `web/src/__tests__/components/sidebar/lib/rows-from-repo.test.ts`

**Interfaces:**
- Consumes: today's `Repo`/`Workspace`/`Folder`/`ChatFolder` store shapes
  (`lib/store/sidebar.ts`), `SidebarRow` (B.1).
- Produces: `rowsFromRepo(repo: Repo): SidebarRow[]` — this is the bridge
  named in Task 4; it is deleted in Task 15 once the backend plan's Stage 7
  ships rows pre-shaped as `SidebarRow` over the wire.

- [ ] **Step 1: Write the failing test**

```ts
it('a locked branch becomes a branch-kind row', () => {
  const repo = makeTestRepo({ workspaces: [{ id: 'ws-1', branch: 'develop', status: 'locked' }] })
  const rows = rowsFromRepo(repo)
  const row = rows.find((r) => r.workspaceId === 'ws-1')
  expect(row?.kind).toBe('branch')
  expect(row?.ownsWorktree).toBe(true)
})

it('a chat folder becomes a folder-kind row', () => {
  const repo = makeTestRepo({ folders: [{ id: 'f-1', name: 'Bugs' }] })
  const rows = rowsFromRepo(repo)
  expect(rows.find((r) => r.id === 'f-1')?.kind).toBe('folder')
})
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement** the mapping, then wire `SidebarTree` (B.3) into
  whatever currently mounts `WorkspaceTree`/`AgentChatsPanel`, and delete the
  old files. Run a full grep for every deleted export
  (`grep -rl "WorkspaceTree\|AgentChatsPanel" web/src`) before deleting to
  confirm nothing outside the two trees imports them.

- [ ] **Step 4: Run**

Run: `bun test web/src/__tests__/components/sidebar/`
Run: `bun tsc --noEmit` (catches any surviving import of a deleted file)

- [ ] **Step 5: Commit**

```bash
git add -A web/src/components/sidebar/ web/src/components/layout/ web/src/features/agent/tree/
git commit -m "feat(sidebar): one tree replaces the workspace tree and the chats panel"
```

---

## Part C — Spaces: one project per sidebar

*Spec §4. Depends on Part B (spaces render `SidebarTree` per project).*

### Task 9: `Space` scroller — x-mandatory snap over one panel per project

**Files:**
- Create: `web/src/components/sidebar/space-scroller.tsx`
- Test: `web/src/__tests__/components/sidebar/space-scroller.test.tsx`

**Interfaces:**
- Consumes: `Project` (`web/src/lib/types.ts:21`, confirmed:
  `{id, name, path, lastActivity, order?, avatarUrl?, avatarEmoji?}`),
  `SidebarTree` (B.3).
- Produces:
  ```tsx
  interface SpaceScrollerProps {
    projects: Project[]
    activeProjectId: string
    onActiveProjectChange: (id: string) => void
    rowsForProject: (projectId: string) => SidebarRow[]
  }
  function SpaceScroller(props: SpaceScrollerProps): JSX.Element
  ```
  Reuse `sidebar-carousel.tsx`'s exact mechanics (`isUserGesture` ref armed
  only by `wheel`/`touchstart`, `ResizeObserver` with the `clientWidth === 0`
  guard, `Math.round(scrollLeft / clientWidth)` for the active-panel read) —
  this is a new component, not a repurposed `SidebarCarousel` instance,
  because `SidebarCarousel`'s scope is shrinking to Files/Git only (Task
  D.1) and mixing "which project" with "which card tab" into one carousel
  component would conflate two different numbers. Copy the pattern; don't
  import the component.

- [ ] **Step 1: Write the failing test**

```tsx
it('renders one panel per project, min-width 100%', () => {
  const projects = [makeProject('p1'), makeProject('p2')]
  render(<SpaceScroller projects={projects} activeProjectId="p1" onActiveProjectChange={vi.fn()} rowsForProject={() => []} />)
  const panels = screen.getAllByTestId('space-panel')
  expect(panels).toHaveLength(2)
  expect(panels[0]).toHaveClass('min-w-full')
})

it('clicking a mark scrolls to that space', () => {
  const onChange = vi.fn()
  const projects = [makeProject('p1'), makeProject('p2')]
  render(<SpaceScroller projects={projects} activeProjectId="p1" onActiveProjectChange={onChange} rowsForProject={() => []} />)
  fireEvent.wheel(screen.getByTestId('space-scroll-region'), { deltaX: 100 })
  expect(onChange).toHaveBeenCalled()
})
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement**

- [ ] **Step 4: Run, verify pass**

- [ ] **Step 5: Commit**

```bash
git add web/src/components/sidebar/space-scroller.tsx web/src/__tests__/components/sidebar/space-scroller.test.tsx
git commit -m "feat(sidebar): SpaceScroller — one project per sidebar, x-mandatory snap"
```

### Task 10: Space header — the project row, promoted

**Files:**
- Create: `web/src/components/sidebar/space-header.tsx`
- Modify: `web/src/components/layout/sidebar-project-header.tsx` — this file
  stays (Task 8 did not delete it) and this task adds the fold behavior;
  the row-rendering itself moves to reuse `SidebarRow` (B.2) rather than its
  own markup.
- Test: `web/src/__tests__/components/sidebar/space-header.test.tsx`

**Interfaces:**
- Consumes: `Project`, `SidebarRow` (B.2).
- Produces:
  ```tsx
  interface SpaceHeaderProps {
    project: Project
    folded: boolean
    onToggleFold: () => void
    onOverflow: () => void
  }
  function SpaceHeader(props: SpaceHeaderProps): JSX.Element
  ```

- [ ] **Step 1: Write the failing test**

```tsx
it('at rest shows the project mark and name, no controls', () => {
  render(<SpaceHeader project={makeProject('p1')} folded={false} onToggleFold={vi.fn()} onOverflow={vi.fn()} />)
  expect(screen.queryByTestId('chevron')).not.toBeInTheDocument()
})

it('on hover the mark slot becomes a chevron, overflow appears', () => {
  render(<SpaceHeader project={makeProject('p1')} folded={false} onToggleFold={vi.fn()} onOverflow={vi.fn()} />)
  fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
  expect(screen.getByTestId('chevron')).toBeInTheDocument()
  expect(screen.getByTestId('overflow')).toBeInTheDocument()
})

it('clicking folds: chevron stays, rotated', () => {
  const onToggle = vi.fn()
  render(<SpaceHeader project={makeProject('p1')} folded={true} onToggleFold={onToggle} onOverflow={vi.fn()} />)
  expect(screen.getByTestId('chevron')).toHaveClass('rotate-180')
})
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement**, built on `SidebarRow` (B.2) with the controls
  swapped: no trash, no `+`; mark↔chevron swap on hover per spec §4; no
  background at rest, `--accent` under the pointer, same as any other row.

- [ ] **Step 4: Run, verify pass**

- [ ] **Step 5: Commit**

```bash
git add web/src/components/sidebar/space-header.tsx web/src/__tests__/components/sidebar/space-header.test.tsx
git commit -m "feat(sidebar): SpaceHeader — the project row, promoted, folds tree not Recents"
```

### Task 11: Space marks in the window chrome's dead middle

**Files:**
- Modify: `web/src/components/layout/sidebar-project-header.tsx` — the
  window-chrome row (confirmed: traffic-light reserve `w-[72px]` on Mac,
  toggle leading, back/forward/settings trailing via `useJumpNavigation()`,
  `<div className="flex-1" />` already the dead middle this task fills).
- Test: `web/src/__tests__/components/layout/sidebar-project-header.test.tsx`
  (extend existing).

**Interfaces:**
- Consumes: `Project[]`, `activeProjectId` (from C.1's scroller state, lifted
  to whatever ancestor already owns `sidebar-project-header.tsx` and the
  scroller — likely the sidebar's own root component; find it before writing
  this task's code).

- [ ] **Step 1: Write the failing test**

```tsx
it('renders one icon-only mark per project in the chrome middle', () => {
  const projects = [makeProject('p1'), makeProject('p2')]
  render(<SidebarProjectHeader projects={projects} activeProjectId="p1" onSelectProject={vi.fn()} />)
  expect(screen.getAllByTestId('space-mark')).toHaveLength(2)
})

it('the current space mark is full strength, others muted', () => {
  const projects = [makeProject('p1'), makeProject('p2')]
  render(<SidebarProjectHeader projects={projects} activeProjectId="p1" onSelectProject={vi.fn()} />)
  const marks = screen.getAllByTestId('space-mark')
  expect(marks[0]).not.toHaveClass('opacity-60')
  expect(marks[1]).toHaveClass('opacity-60')
})
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement** — icon-only marks in the existing `flex-1`
  spacer, no labels/counts/close, click drives the same scroller C.1 built
  (mark and panel are two views of one number, per spec §4.1 — wire this
  through the same `activeProjectId`/`onActiveProjectChange` pair, don't
  maintain a second index).

- [ ] **Step 4: Run**

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/sidebar-project-header.tsx web/src/__tests__/components/layout/sidebar-project-header.test.tsx
git commit -m "feat(chrome): space marks fill the window chrome's dead middle"
```

---

## Part D — Recents

*Spec §5. Depends on Part B (reuses `SidebarRow`) and the existing
`agentChats.working` slice (confirmed:
`s.agentChats.working[chatId] ?? false`, `features/workspace/stores/slices/agent-chats-slice.ts:78`).*

### Task 12: `RecentsBand` — four states, no indent, no parentage

**Files:**
- Create: `web/src/components/sidebar/recents-band.tsx`
- Test: `web/src/__tests__/components/sidebar/recents-band.test.tsx`

**Interfaces:**
- Consumes: `SidebarRow` (B.2, `depth={0}` always), `PaneGroup[]` (Part A, to
  derive LIVE/SET), `agentChats.working` (existing slice, per-row selector
  pattern copied verbatim: `useWorkspaceStore((s) => s.agentChats.working[chatId] ?? false)`).
- Produces:
  ```tsx
  type RecentsEntryState = 'live' | 'working' | 'set' | 'dormant'

  interface RecentsEntry {
    id: string             // keyed by the view's identity, not by state — §5.6
    chatIds: string[]      // one for a lone entry, 2+ for a set
    state: RecentsEntryState
  }

  interface RecentsBandProps {
    entries: RecentsEntry[]
    onFocus: (entry: RecentsEntry) => void
    onClose: (entry: RecentsEntry) => void   // absent control for 'working' — no onClose call possible
  }
  function RecentsBand(props: RecentsBandProps): JSX.Element
  ```

- [ ] **Step 1: Write the failing test**

```tsx
it('a working entry has no close control', () => {
  const entries: RecentsEntry[] = [{ id: 'e1', chatIds: ['chat-1'], state: 'working' }]
  render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} />)
  expect(screen.queryByTestId('recents-close-e1')).not.toBeInTheDocument()
})

it('a set draws as one shell around its member rows', () => {
  const entries: RecentsEntry[] = [{ id: 'e1', chatIds: ['chat-1', 'chat-2'], state: 'set' }]
  render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} />)
  const shell = screen.getByTestId('recents-set-e1')
  expect(within(shell).getAllByTestId(/^recents-row-/)).toHaveLength(2)
})

it('entries render flat, no indent', () => {
  const entries: RecentsEntry[] = [{ id: 'e1', chatIds: ['chat-1'], state: 'dormant' }]
  render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} />)
  expect(screen.getByTestId('recents-row-chat-1')).not.toHaveAttribute('data-depth')
})
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement** — header is a hairline rule with "Recents" at its
  far end (spec §5.1, 22px per the spec's own numbers table); every entry
  uses `SidebarRow` at `depth={0}`; a `set` entry wraps N `SidebarRow`s in a
  shell div carrying its own ground/radius/padding per spec §5.3, and per
  §5.3 "the shell carries the state, the members carry the pointer" — the
  shell itself, not each member, takes `ROW_ACTIVE` when the set is live.

- [ ] **Step 4: Run, verify pass**

- [ ] **Step 5: Commit**

```bash
git add web/src/components/sidebar/recents-band.tsx web/src/__tests__/components/sidebar/recents-band.test.tsx
git commit -m "feat(sidebar): RecentsBand — four states, one shell per set"
```

### Task 13: Deriving `RecentsEntry[]` from panes + working + a remembered-arrangements list

**Files:**
- Create: `web/src/components/sidebar/lib/recents-entries.ts`
- Modify: `web/src/features/workspace/stores/slices/pane-slice.ts` — add
  `dormantArrangements: RecentsEntry[]` state (populated on close, per spec
  §5.5: "the view dies, the row does not... idle, and the view it was is
  remembered so the close is undoable").
- Test: `web/src/__tests__/components/sidebar/lib/recents-entries.test.ts`

**Interfaces:**
- Consumes: `PaneGroup[]` (Task 1/A.2), `agentChats.working`,
  `dormantArrangements` (new).
- Produces: `deriveRecentsEntries(panes, working, dormantArrangements):
  RecentsEntry[]` — pure, per spec §5.6's population rule: a chat appears at
  most once, in the highest band that claims it (live, then working, then
  dormant); order is the user's, keyed by the entry's chat-set identity so a
  view keeps its slot as it changes kind.

- [ ] **Step 1: Write the failing test**

```ts
it('a chat appears once, in the highest band that claims it', () => {
  const panes = [makePane({ chatId: 'chat-1' })]
  const working = { 'chat-1': true, 'chat-2': true }
  const dormant: RecentsEntry[] = []
  const entries = deriveRecentsEntries(panes, working, dormant)
  const chat1Entries = entries.filter((e) => e.chatIds.includes('chat-1'))
  expect(chat1Entries).toHaveLength(1)
  expect(chat1Entries[0].state).toBe('live')
})

it('closing a working view keeps it in the band as "working", not dormant', () => {
  const panes: PaneGroup[] = []  // view closed
  const working = { 'chat-1': true }
  const dormant: RecentsEntry[] = []
  const entries = deriveRecentsEntries(panes, working, dormant)
  expect(entries.find((e) => e.chatIds.includes('chat-1'))?.state).toBe('working')
})

it('an arrangement keeps its slot as it gains a pane', () => {
  const dormant: RecentsEntry[] = [{ id: 'view-a', chatIds: ['chat-1'], state: 'dormant' }]
  const panes = [makePane({ id: 'view-a', chatId: 'chat-1' })]
  const entries = deriveRecentsEntries(panes, {}, dormant)
  const idx = entries.findIndex((e) => e.id === 'view-a')
  expect(idx).toBe(0) // same slot dormant held, not re-sorted to the end
})
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement**

- [ ] **Step 4: Run, verify pass**

- [ ] **Step 5: Commit**

```bash
git add web/src/components/sidebar/lib/recents-entries.ts web/src/features/workspace/stores/slices/pane-slice.ts web/src/__tests__/components/sidebar/lib/recents-entries.test.ts
git commit -m "feat(sidebar): derive Recents entries — one band, one slot per view, order is the user's"
```

### Task 14: Closing empties the last pane instead of refusing; the × always means "end this view"

**Files:**
- Modify: `web/src/features/workspace/stores/slices/pane-slice.ts` —
  `closePane`'s behavior when it's the last pane holding this chat: instead
  of refusing or destroying the pane, set `chatId: null` (the empty-stage
  pane, per spec §5.4 "closing the last pane empties it rather than
  refusing").
- Test: extend `pane-slice.test.ts`.

- [ ] **Step 1: Write the failing test**

```ts
it('closing the only pane leaves an empty-stage pane, not zero panes', () => {
  const store = createWorkspaceStore('ws-test')
  const paneId = 'root-pane'
  store.getState().setPaneChat(paneId, 'chat-1', 'runner-1')
  store.getState().closePane(paneId)
  expect(store.getState().getPaneById(paneId)).not.toBeNull()
  expect(store.getState().getPaneById(paneId)?.chatId).toBeNull()
})
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement**

- [ ] **Step 4: Run, verify pass**

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/stores/slices/pane-slice.ts web/src/__tests__/features/workspace/stores/slices/pane-slice.test.ts
git commit -m "fix(panes): closing the last pane empties it to the New Tab stage, never refuses"
```

---

## Part E — The file explorer card: 4-panel carousel shrinks to 2

*Spec §6. `sidebar-carousel.tsx` already implements exactly this mechanic
today for 4 tabs (`workspaces|chats|files|git`) — this part narrows its scope,
it does not build a carousel from scratch.*

### Task 15: `SidebarCarousel` narrows to Files/Git; head becomes the underline tabs variant

**Files:**
- Modify: `web/src/components/layout/sidebar-carousel.tsx` — `TABS` shrinks
  from `['workspaces','chats','files','git']` to `['files','git']`; the
  `workspaces`/`chats` panels' content is deleted (Part B's `SidebarTree` +
  Part D's `RecentsBand` now own that surface directly, not as carousel
  panels).
- Modify: wherever the card's head renders tabs — swap to `ui/tabs.tsx`'s
  confirmed `variant="underline"` (`TabsList` prop, ships today, unused
  elsewhere per reconnaissance), `justify-start` with the fold control on
  `ml-auto`, per spec §6.1.
- Test: `web/src/__tests__/components/layout/sidebar-carousel.test.tsx`
  (extend existing).

**Interfaces:**
- Consumes: `TabsList variant="underline"` (`ui/tabs.tsx`, confirmed prop).
- Produces: the card's two-panel carousel, unchanged mechanics
  (`isUserGesture` ref, `ResizeObserver` + `clientWidth === 0` guard,
  `Math.round` panel resolution — all confirmed already correct, keep them
  verbatim) over the narrowed `TABS`.

- [ ] **Step 1: Write the failing test**

```tsx
it('the card has exactly two panels: Files and Git', () => {
  render(<SidebarCarousel activeWorkspaceRepoPath="/repo" />)
  expect(screen.getAllByTestId('carousel-panel')).toHaveLength(2)
})

it('the head uses the underline tabs variant, icon only', () => {
  render(<SidebarCarousel activeWorkspaceRepoPath="/repo" />)
  expect(screen.queryByText('Files')).not.toBeInTheDocument() // icon only, no label
  expect(screen.getByTestId('tabs-underline')).toBeInTheDocument()
})
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement** — delete the `workspaces`/`chats` panel branches
  from the component's render; swap the head's tab markup for
  `<TabsList variant="underline">`; move the fold control onto the head's own
  line, `ml-auto` (spec §6.1 — today it likely sits in the tree above the
  card; relocate it).

- [ ] **Step 4: Run, verify pass**

Run: `bun test web/src/__tests__/components/layout/sidebar-carousel.test.tsx`

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/sidebar-carousel.tsx web/src/__tests__/components/layout/sidebar-carousel.test.tsx
git commit -m "feat(card): SidebarCarousel narrows to Files/Git, head becomes the underline variant"
```

### Task 16: Floating position, resize handle, proportion-of-rail height

**Files:**
- Modify: `web/src/components/layout/sidebar-carousel.tsx` (or its container,
  if the floating/positioning logic lives one level up — confirm before
  editing).
- Test: extend the same test file.

- [ ] **Step 1: Write the failing test**

```tsx
it('opens at one third of the sidebar height', () => {
  render(<SidebarCarousel activeWorkspaceRepoPath="/repo" sidebarHeight={900} />)
  expect(screen.getByTestId('carousel-card')).toHaveStyle({ height: '300px' })
})

it('the resize handle is the top 6px', () => {
  render(<SidebarCarousel activeWorkspaceRepoPath="/repo" />)
  const handle = screen.getByTestId('carousel-resize-handle')
  expect(handle).toHaveClass('h-1.5') // matches pane-sash.tsx's confirmed w-1.5/h-1.5 = 6px
})
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement** — `inset-8px` on three sides, `--radius: 12px`,
  `--pane` ground, opens at ⅓ of sidebar height, height stored as a
  proportion of rail height (survives window resize), top 6px is the resize
  hot zone (matches `pane-sash.tsx`'s `w-1.5`/`h-1.5` literally, per
  reconnaissance — reuse the same Tailwind class, don't hand-roll a new `6px`
  value). Tree keeps a bottom inset equal to the card's height so the last row
  scrolls clear.

- [ ] **Step 4: Run, verify pass**

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/sidebar-carousel.tsx
git commit -m "feat(card): floating position, ⅓-height open, top-6px resize handle"
```

---

## Part F — The pane: one row across the top, two views, recursive layout

*Spec §7. Depends on Part A (the pane data model).*

### Task 17: One row across the top — split toggle, chat head, editor tab strip

**Files:**
- Modify: `web/src/components/layout/tab-bar.tsx` — today draws the editor's
  own tab strip; this task makes it the *whole* pane-top row: split toggle
  leading (before the chat name, outside the tab scroller), the chat name
  itself (no close, no reordering, outside the scroller, sourced from
  `pane.chatId`), then the editor tab strip in a scroller, `+` as the last
  child inside it.
- Test: `web/src/__tests__/components/layout/tab-bar.test.tsx` (extend).

**Interfaces:**
- Consumes: `PaneGroup` (Task 1/A.2).

- [ ] **Step 1: Write the failing test**

```tsx
it('the split toggle leads, before the chat name, outside the tab scroller', () => {
  const pane = makePane({ chatId: 'chat-1', editorTabIds: ['file-1'] })
  render(<TabBar pane={pane} />)
  const row = screen.getByTestId('pane-top-row')
  const children = Array.from(row.children).map((c) => c.getAttribute('data-role'))
  expect(children[0]).toBe('split-toggle')
  expect(children[1]).toBe('chat-head')
})

it('a pane with only its chat draws no bar at all', () => {
  const pane = makePane({ chatId: 'chat-1', editorTabIds: [] })
  render(<TabBar pane={pane} />)
  expect(screen.queryByTestId('editor-tab-scroller')).not.toBeInTheDocument()
})

it('the chat head has no close affordance', () => {
  const pane = makePane({ chatId: 'chat-1' })
  render(<TabBar pane={pane} />)
  expect(within(screen.getByTestId('chat-head')).queryByRole('button', { name: /close/i })).not.toBeInTheDocument()
})
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement**. Delete any "Editor" tab concept if one exists
  today (spec explicitly rejects it — the second view is reached through open
  files or `+`, never a tab standing for the idea of a view).

- [ ] **Step 4: Run, verify pass**

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/tab-bar.tsx web/src/__tests__/components/layout/tab-bar.test.tsx
git commit -m "feat(pane): one row across the top — split toggle, chat head, editor tabs"
```

### Task 18: Two views — landscape side-by-side, portrait stacked, both mounted

**Scope extended post-hoc by the controller, after Task 31/32 landed.**
The brief below only generalizes `use-chat-presentation.ts` itself — its
current real host is `agent-chat-pane.tsx`'s OWN internal chat⇄terminal
split (a different concern: that component's own embedded tool output,
not the app-wide pane-level chat-view/editor-view split this redesign
introduces). Verified independently: no task anywhere in this plan wires
the generalized hook into `pane-container.tsx`, which currently renders
chat-view and editor-view as Task 31's own disclosed "placeholder
sequential stack... until [Task 18] lands." Lower severity than the
Task 29-32 gaps (nothing crashes or is unreachable without this — the
placeholder stack works, it just isn't the responsive side-by-side/
stacked layout the spec describes), so this is folded into Task 18's own
scope rather than spawned as a separate task: **this task must ALSO wire
the generalized hook into `pane-container.tsx`**, replacing the
placeholder stack with the real side-by-side (landscape)/stacked
(portrait)/tabs (too small or split off) arrangement, per spec §7.2.

**Files:**
- Modify: whichever component hosts `useChatPresentation` today (confirmed:
  `use-chat-presentation.ts`, currently the chat⇄**terminal** split — this
  task generalizes it to chat⇄**editor view**, reusing its four constants
  verbatim: `SPLIT_SIDE_BY_SIDE_MIN_PX=780`, `SPLIT_MIN_HALF_PX=340`,
  `SPLIT_MIN_STACKED_PX=160`, `SPLIT_DEFAULT_SIZES=[45,55]`).
- Modify: `web/src/features/panes/components/pane-container.tsx` — wire the
  generalized hook in, replacing the placeholder sequential stack (find its
  exact current location by searching for "placeholder sequential stack"
  in that file's comments) with the real arrangement: side-by-side on
  landscape, stacked (tab strip moves down between chat and editor, per
  spec §7.2) on portrait, tabs when too small or the split is off (driven
  by `pane.editorOpen`, per Task 1). Both regions stay mounted always
  (`hidden` attribute per Task 31's established pattern, never unmounted)
  regardless of which presentation is chosen — confirm this survives
  whatever you build.
- Test: extend that hook's existing test file, plus
  `pane-container.test.tsx` for the new wiring.

**Interfaces:**
- Consumes: `PaneGroup.editorOpen` (Task 1), the four constants above
  (unchanged).

- [ ] **Step 1: Write the failing test**

```ts
it('below SPLIT_SIDE_BY_SIDE_MIN_PX, tabs are the only presentation regardless of editorOpen', () => {
  const { result } = renderHook(() => useChatPresentation('chat-1', { current: { clientWidth: 600 } } as any))
  expect(result.current.presentation).toBe('tabs')
})

it('landscape width with editorOpen true presents side by side', () => {
  const { result } = renderHook(() => useChatPresentation('chat-1', { current: { clientWidth: 900 } } as any))
  act(() => result.current.setPresentation('split'))
  expect(result.current.chosen).toBe('side-by-side')
})
```

- [ ] **Step 2: Run, verify fail** (fails only if the hook's semantics
  changed — if today's tests already cover this shape for the
  chat/terminal split, this step may find they pass unmodified, which is
  fine: it confirms the generalization is a rename, not a behavior change.
  If they fail, the hook's internals need the actual generalization, not
  just a rename.)

- [ ] **Step 3: Implement** — both views stay mounted always (`display: none`
  dormancy, not `content-visibility` — confirmed load-bearing by the spec's
  own perf note). No swipe gesture between them (explicitly rejected). Measure
  stacked-vs-side-by-side via `ResizeObserver` on the pane, never the window.

- [ ] **Step 4: Run, verify pass**

- [ ] **Step 5: Commit**

```bash
git add -A web/src/components/layout/use-chat-presentation.ts
git commit -m "feat(pane): generalize the chat split hook to chat-view / editor-view"
```

### Task 19: Gutters, corners, sash insets — confirm unchanged, add the regression test

**Files:**
- No production change expected — `pane-border.ts`'s `isWindowEdge` already
  returns `false` for `top` unconditionally (confirmed literal in code) and
  `pane-drop-zones.ts`'s `getPaneDropZoneFromRect` already implements
  threshold=0.25 with diagonal corner tests (confirmed). This task exists to
  write the regression tests the model spec's own invariant convention
  requires, not to change behavior.
- Test: `web/src/__tests__/components/layout/pane-border.test.ts`,
  `web/src/__tests__/components/layout/pane-drop-zones.test.ts` (create if
  missing, extend if present).

- [ ] **Step 1: Write the tests**

```ts
it('top is never a window edge, regardless of sidebar state', () => {
  expect(isWindowEdge('top', { atTop: true }, 'left', true)).toBe(false)
  expect(isWindowEdge('top', { atTop: true }, 'left', false)).toBe(false)
})

it('corners resolve to the nearer edge via the diagonal test', () => {
  const rect = { left: 0, top: 0, width: 100, height: 100 } as DOMRect
  const zone = getPaneDropZoneFromRect({ x: 5, y: 5 }, rect) // top-left corner
  expect(['left', 'top']).toContain(zone)
})
```

- [ ] **Step 2: Run, verify pass immediately** (this task adds coverage over
  already-correct behavior — if either test fails, that's a real regression
  to fix, not an expected red step).

- [ ] **Step 3: Commit**

```bash
git add web/src/__tests__/components/layout/pane-border.test.ts web/src/__tests__/components/layout/pane-drop-zones.test.ts
git commit -m "test(pane): lock in top-never-a-window-edge and the corner diagonal test"
```

---

## Part G — One drag, and deleting the removal gesture it replaces

*Spec §8, law 2, law 6. The largest architectural task in this plan — merges
`workspace-tree-context.tsx` and `use-agent-chats-drag.ts` into one arm, and
deletes `editor-removal-overlay.tsx`'s dwell-to-remove mechanism outright,
replacing it with the four-target open/merge table from spec §8.1.*

### Task 20: One `DropPolicy` over `SidebarRow`, built on the existing generic core

**Files:**
- Create: `web/src/components/sidebar/lib/sidebar-drop-policy.ts`
- Test: `web/src/__tests__/components/sidebar/lib/sidebar-drop-policy.test.ts`

**Interfaces:**
- Consumes: `tree-dnd/drop-core.ts` (confirmed generic, unchanged:
  `DropMode`, `AllowedModes`, `NO_MODES/REORDER_MODES/INTO_MODES/ALL_MODES`,
  `EDGE_BAND_CONTAINER=0.2`, `EDGE_BAND_HEAVY=0.3`, `dropModeAt`,
  `DropPolicy<S,T>`), `SidebarRow` (B.1).
- Produces: `SIDEBAR_DROP_POLICY: DropPolicy<SidebarRow, SidebarRow>` — one
  policy replacing both `SIDEBAR_DROP_POLICY` (old, `drop-rules.ts`) and
  `CHAT_DROP_POLICY` (old, `chat-drop.ts`).

Same-repo rule carries over (`drop-rules.ts`'s confirmed `allowedModes`
check, ~line 80) generalized to a same-project rule, since rows now span one
project's whole forest rather than one repo's workspaces. Working-row refusal
(spec §8.3) reads `SidebarRow.working`, refusing every mode when true — the
UI mirror of the backend plan's `guardNotWorking` (backend plan Task 4.1),
duplicated here deliberately since a drag needs to refuse *before* a network
round trip, not after.

- [ ] **Step 1: Write the failing test**

```ts
it('a working row allows no drop mode', () => {
  const subject: SidebarRow = { ...baseRow, working: true }
  const modes = SIDEBAR_DROP_POLICY.allowedModes([subject], targetRow)
  expect(modes).toEqual(NO_MODES)
})

it('cross-project drag is refused', () => {
  const subject: SidebarRow = { ...baseRow, projectId: 'p1' } as any
  const target: SidebarRow = { ...baseRow, projectId: 'p2' } as any
  const modes = SIDEBAR_DROP_POLICY.allowedModes([subject], target)
  expect(modes).toEqual(NO_MODES)
})

it('cross-repo is legal only for a row owning no worktree', () => {
  const subjectNoWorktree: SidebarRow = { ...baseRow, ownsWorktree: false, repoId: 'r1' } as any
  const subjectWithWorktree: SidebarRow = { ...baseRow, ownsWorktree: true, repoId: 'r1' } as any
  const target: SidebarRow = { ...baseRow, repoId: 'r2' } as any
  expect(SIDEBAR_DROP_POLICY.allowedModes([subjectNoWorktree], target)).not.toEqual(NO_MODES)
  expect(SIDEBAR_DROP_POLICY.allowedModes([subjectWithWorktree], target)).toEqual(NO_MODES)
})
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement**, built directly on `drop-core.ts`'s exports —
  do not reimplement `dropModeAt` or the edge-band math, only the
  `allowedModes`/`edgeBandFor` policy functions are new here.

- [ ] **Step 4: Run, verify pass**

- [ ] **Step 5: Commit**

```bash
git add web/src/components/sidebar/lib/sidebar-drop-policy.ts web/src/__tests__/components/sidebar/lib/sidebar-drop-policy.test.ts
git commit -m "feat(sidebar): one DropPolicy over SidebarRow, replacing the two tree-specific ones"
```

### Task 21: One drag hook — `useSidebarDrag`, replacing both tree-specific hooks

**Files:**
- Create: `web/src/components/sidebar/hooks/use-sidebar-drag.ts`
- Delete: `web/src/components/layout/workspace-tree-context.tsx`,
  `features/agent/tree/hooks/use-agent-chats-drag.ts` — once every consumer
  is moved onto the new hook (Task 6/D.1's tree and band both take it).
- Test: `web/src/__tests__/components/sidebar/hooks/use-sidebar-drag.test.ts`

**Interfaces:**
- Consumes: `SIDEBAR_DROP_POLICY` (G.1), `tree-dnd/drop-dom.ts`'s
  `createDropRowDom`/`createDropHitTest` (confirmed generic, both trees
  already bind through this factory today — this task binds it once, over
  `SidebarRow`, instead of twice).
- Produces:
  ```ts
  const SIDEBAR_DRAG_THRESHOLD_PX = 5   // matches both existing hooks' confirmed value

  function useSidebarDrag(options: {
    scrollRef: React.RefObject<HTMLElement>
    subjectsFor: (rowId: string) => SidebarRow[]
    onDrop: (subjects: SidebarRow[], target: SidebarRow, mode: DropMode) => void
    onPaneDrop: (subjects: SidebarRow[], paneId: string, zone: 'center' | 'left' | 'right' | 'top' | 'bottom') => void
  }): SidebarDrag
  ```
  `onPaneDrop` replaces the old `onPaneRemove` callback shape (confirmed
  present on both old hooks) — its four zone values map to spec §8.1's table
  (`center` → into this view, you choose where; edge → into this view, on
  that side) instead of arming a removal.

- [ ] **Step 1: Write the failing test**

```ts
it('a press under 5px does not start a drag', () => {
  const onDrop = vi.fn()
  const { result } = renderHook(() => useSidebarDrag({ scrollRef: mockRef, subjectsFor: () => [baseRow], onDrop, onPaneDrop: vi.fn() }))
  simulatePointerDrag(result.current, { dx: 3, dy: 0 })
  expect(result.current.dragging).toBe(false)
})

it('dropping on the middle third of a pane calls onPaneDrop with zone center', () => {
  const onPaneDrop = vi.fn()
  const { result } = renderHook(() => useSidebarDrag({ scrollRef: mockRef, subjectsFor: () => [baseRow], onDrop: vi.fn(), onPaneDrop }))
  simulatePointerDragOntoPane(result.current, paneRectMiddle)
  expect(onPaneDrop).toHaveBeenCalledWith([baseRow], expect.any(String), 'center')
})
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement**, following the confirmed architecture of both
  existing hooks (refs-only inside one `useEffect`, window-level
  `pointermove`/`pointerup`/`pointercancel`, direct `style.transform` writes
  for the ghost, React state only when the resolved target visibly changes) —
  this is a proven pattern, keep it, just make it the only implementation.

- [ ] **Step 4: Migrate consumers** — `SidebarTree` (B.3) and `RecentsBand`
  (D.1) both take `useSidebarDrag` with the same `dragProps` spread onto
  `SidebarRow` (per `SidebarRowProps.dragProps`, already declared in Task
  B.2). Delete the two old hooks and their now-unused policy files
  (`drop-rules.ts`, `chat-drop.ts`) once nothing imports them
  (`grep -rl "drop-rules\|chat-drop" web/src` must return only this task's
  own deletions).

- [ ] **Step 5: Run**

Run: `bun test web/src/__tests__/components/sidebar/`
Run: `bun tsc --noEmit`

- [ ] **Step 6: Commit**

```bash
git add -A web/src/components/sidebar/ web/src/components/layout/workspace-tree-context.tsx web/src/features/agent/tree/
git commit -m "feat(sidebar): one drag arm — useSidebarDrag replaces the two tree-specific hooks"
```

### Task 22: Delete the dwell-to-remove overlay; every pane drop adds

**Files:**
- Delete: `web/src/components/layout/editor-removal-overlay.tsx` and its
  `PANE_ARM_MS`/`PANE_DROP_ATTR` constants (confirmed: dwell-gated, two
  visual states, imperative DOM attribute toggling — this whole mechanism is
  superseded, not adapted).
- Modify: wherever the pane mounts a drop target for rows (found via
  `grep -rl PANE_DROP_ATTR web/src`) — replace the removal-overlay wiring
  with `onPaneDrop` from Task 21, per spec §8's four-target table:
  | target | outcome |
  |---|---|
  | middle of a pane | into this view, caller chooses where |
  | edge of a pane | into this view, on that side |
  | middle of a Recents entry | into that view, opened |
  | above/below a Recents entry | reorder |
- Test: `web/src/__tests__/components/layout/pane-drop-target.test.tsx`
  (new — the old overlay's tests, if any, get deleted with it, not migrated,
  since the behavior they tested no longer exists).

- [ ] **Step 1: Write the failing test**

```tsx
it('dropping a chat on a pane middle opens it into that view, never removes the pane', () => {
  const onOpenInto = vi.fn()
  render(<PaneDropTarget paneId="pane-1" onOpenInto={onOpenInto} />)
  simulateDrop(screen.getByTestId('pane-drop-pane-1'), { zone: 'center', subjects: [baseRow] })
  expect(onOpenInto).toHaveBeenCalledWith('pane-1', [baseRow])
})

it('no removal overlay exists anywhere in the tree', () => {
  const { container } = render(<PaneDropTarget paneId="pane-1" onOpenInto={vi.fn()} />)
  expect(container.querySelector('[data-pane-removal]')).toBeNull()
})
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement** — dropping a chat that's already up goes *to* it
  (a neutral indicator, never opens twice, spec §8.2); a target already on
  screen grows instead of reopening; whatever goes up leaves every
  arrangement that was remembering it, and the arrangement left behind keeps
  its survivors as a set (spec §8.2's "pull one chat out of a live three-up,
  the other two are kept as a set").

- [ ] **Step 4: Run**

- [ ] **Step 5: Commit**

```bash
git add -A web/src/components/layout/ web/src/__tests__/components/layout/pane-drop-target.test.tsx
git commit -m "feat(pane): every drop adds — delete the dwell-to-remove overlay"
```

### Task 23: Clicking a chat makes its own view; nothing you click costs the current one

**Files:**
- Modify: wherever `SidebarRow.onOpen` (B.2) is wired to pane actions —
  clicking an unopened chat must not evict whatever the focused pane
  currently holds; instead it puts that arrangement into Recents (dormant)
  and opens the clicked chat into a *new* view, per spec §8.4.
- Test: `web/src/__tests__/components/sidebar/lib/click-to-open.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
it('clicking a new chat never evicts the currently focused pane', () => {
  const store = createWorkspaceStore('ws-test')
  store.getState().setPaneChat('pane-1', 'chat-1', 'runner-1')
  openChatFromSidebar(store, 'chat-2')
  const dormant = store.getState().dormantArrangements
  expect(dormant.some((e) => e.chatIds.includes('chat-1'))).toBe(true)
})

it('clicking a chat already up goes to it, never opens a second pane on it', () => {
  const store = createWorkspaceStore('ws-test')
  store.getState().setPaneChat('pane-1', 'chat-1', 'runner-1')
  openChatFromSidebar(store, 'chat-1')
  const panesWithChat1 = Object.values(store.getState().panes).filter((p) => p.chatId === 'chat-1')
  expect(panesWithChat1).toHaveLength(1)
})
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement**

- [ ] **Step 4: Run**

- [ ] **Step 5: Commit**

```bash
git add -A web/src/components/sidebar/ web/src/__tests__/components/sidebar/lib/click-to-open.test.ts
git commit -m "feat(sidebar): clicking a chat opens its own view, never costs the one on screen"
```

---

## Part H — Deleting

*Spec §9. Depends on the backend plan's Stage 7 (delete-preview, working-guard)
for the counts and the server-side refusal; this part's UI can be built and
tested against a mocked preview response before that lands, then wired for
real in Task 25.*

### Task 24: Trash on every owning row; deny tint on hover; leading the trailing cluster

**Files:**
- Modify: `web/src/components/sidebar/sidebar-row.tsx` (B.2) — the trash
  control is already declared in `SidebarRowProps.onTrash`; this task adds
  the population rule: every row backed by a chat/workspace/folder/repo, and
  the space header for a project, carries one; a protected branch (locked
  `branch`-kind row with no `parentId`) and the affordance row never do.
- Test: extend `sidebar-row.test.tsx`.

- [ ] **Step 1: Write the failing test**

```tsx
it('a protected branch row has no trash', () => {
  render(<SidebarRow row={{ ...baseRow, kind: 'branch', branchName: 'develop', ownsWorktree: true }} depth={0} onOpen={vi.fn()} onTrash={vi.fn()} />)
  expect(screen.queryByTestId('trash-control')).not.toBeInTheDocument()
})

it('trash takes the deny tint on hover', () => {
  render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} onTrash={vi.fn()} />)
  fireEvent.mouseEnter(screen.getByTestId('trash-control'))
  expect(screen.getByTestId('trash-control')).toHaveClass('text-destructive') // or the repo's real --deny token class
})
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement**

- [ ] **Step 4: Run, verify pass**

- [ ] **Step 5: Commit**

```bash
git add web/src/components/sidebar/sidebar-row.tsx web/src/__tests__/components/sidebar/sidebar-row.test.tsx
git commit -m "feat(sidebar): trash on every owning row, refused only by locked branches and the affordance row"
```

### Task 25: Delete confirm names what goes; working refuses unconditionally

**Files:**
- Create: `web/src/components/sidebar/delete-confirm-dialog.tsx`
- Create: `web/src/components/sidebar/lib/delete-preview-client.ts` — calls
  the backend plan's `GET /repos/:rid/chats/:id/delete-preview` (Task 7.2 of
  the backend plan).
- Test: `web/src/__tests__/components/sidebar/delete-confirm-dialog.test.tsx`

**Interfaces:**
- Consumes: `{chatCount, fileCount}` from `delete-preview-client.ts`.

- [ ] **Step 1: Write the failing test**

```tsx
it('a working row is refused, not confirmed — no dialog opens', async () => {
  const onTrashClick = renderTrashClick({ ...baseRow, working: true })
  await onTrashClick()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  expect(screen.getByText(/refuses|working/i)).toBeInTheDocument()
})

it('an idle delete names what goes', async () => {
  mockDeletePreview({ chatCount: 3, fileCount: 6 })
  const onTrashClick = renderTrashClick({ ...baseRow, working: false })
  await onTrashClick()
  expect(screen.getByText(/6 uncommitted files/)).toBeInTheDocument()
  expect(screen.getByText(/3 chats/)).toBeInTheDocument()
})
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement** — for a working row, the trash control itself
  declines and shows a short inline reason (spec §9: "REFUSED, not
  confirmed... it does not offer to kill the agent"); for an idle row, fetch
  the preview and render the counts in the confirm copy.

- [ ] **Step 4: Run, verify pass**

- [ ] **Step 5: Commit**

```bash
git add web/src/components/sidebar/delete-confirm-dialog.tsx web/src/components/sidebar/lib/delete-preview-client.ts web/src/__tests__/components/sidebar/delete-confirm-dialog.test.tsx
git commit -m "feat(sidebar): delete confirm names what goes; a working row is refused outright"
```

---

## Part I — Window-level pane layout, and wiring the real backend routes

*Depends on the backend plan's Stage 7. Do this part last.*

### Task 26: Hoist `pane`/`buffer` state out of the per-workspace store registry

**Files:**
- Create: `web/src/features/panes/stores/window-pane-store.ts`
- Modify: `web/src/features/workspace/stores/workspace-store-registry.ts` —
  `getOrCreateWorkspaceStore`/`destroyWorkspaceStore` stop owning
  `PaneSlice`/`BufferSlice`; those move to the new window-level store, created
  once per window, never destroyed on workspace switch (confirmed trap: today
  `destroyWorkspaceStore` kills a live pane layout on switch — this is the
  fix).
- Modify: `web/src/lib/persistence/workspace-layout.ts` — the IndexedDB key
  changes from workspace id to a window/session id, since layout is no longer
  workspace-scoped (confirmed: today's `saveWorkspaceLayout` is keyed by
  workspace — this task re-keys it, keeping the same `idb` mechanism and
  `crowbar` database, per the "zero backend calls in this path" finding —
  this migration is entirely client-side, no API surface involved).
- Test: `web/src/__tests__/features/panes/stores/window-pane-store.test.ts`

**Interfaces:**
- Consumes: `PaneGroup`, `EditorTabBase` (Part A).
- Produces: a store surviving workspace switch, keyed by chat id (per the
  model spec's own framing: "each group's content keys off the chat id and
  resolves its workspace through the row" — this is stage 8 of the *backend*
  spec's own sequencing table, executed here because it's frontend work).

- [ ] **Step 1: Write the failing test**

```ts
it('pane layout survives a workspace switch', () => {
  const store = createWindowPaneStore()
  store.getState().setPaneChat('pane-1', 'chat-1', 'runner-1')
  setActiveWorkspaceId('ws-2') // simulate switching workspaces
  expect(store.getState().getPaneById('pane-1')?.chatId).toBe('chat-1')
})
```

- [ ] **Step 2: Run, verify fail** (today's store dies on switch — this test
  is the direct regression test for the trap the model spec names)

- [ ] **Step 3: Implement** — move the slice definitions, update every
  `usePaneStore`/`useBufferStore`-shaped selector across the codebase
  (`grep -rl "useWorkspaceStore.*pane\|useWorkspaceStore.*buffer" web/src`)
  to read from the new window store instead.

- [ ] **Step 4: Run**

Run: `bun test web/src/__tests__/features/panes/`
Run: `bun tsc --noEmit`

- [ ] **Step 5: Commit**

```bash
git add -A web/src/features/panes/ web/src/features/workspace/stores/workspace-store-registry.ts web/src/lib/persistence/
git commit -m "fix(panes): hoist pane/buffer state to a window-level store, off the per-workspace registry"
```

### Task 27: Replace the 22 `useActiveWorkspaceState` reads with row-resolved workspace ids

**Files:**
- Modify: the ~22 call sites reconnaissance found for
  `useActiveWorkspace|activeWorkspaceId` — for each, determine whether it
  needs "the workspace of the focused pane's chat" (most cases — replace with
  a read off the focused `PaneGroup.chatId` resolved through the row's
  pre-fetched `workspaceId`, per the backend plan's confirmed
  "each row ships its walks already resolved") or genuinely needs "the
  window's single active workspace" (rare, and only legitimate for global
  chrome like the space header — keep those on the existing global).
- Test: one regression test per call site category, not per file — group by
  the pattern found (e.g. "the Files/Git card scopes to the focused pane's
  chat" gets one test covering `sidebar-carousel.tsx`'s new
  `activeWorkspaceRepoPath` derivation).

- [ ] **Step 1: Enumerate the call sites**

Run: `grep -rn "useActiveWorkspace\|activeWorkspaceId" web/src --include="*.tsx" --include="*.ts" | grep -v __tests__`

Categorize each as "follows the focused pane" or "genuinely global"; this
determines which of the two write-ups below applies to it.

- [ ] **Step 2: Write the failing test for the card's case** (the concrete
  example the spec calls out: "the card acts on the current worktree, and the
  current worktree is the row lit in the tree behind it")

```tsx
it('the card scopes to the focused pane chat, not a single window-wide workspace', () => {
  const store = createWindowPaneStore()
  store.getState().setPaneChat('pane-1', 'chat-1', 'runner-1')
  store.getState().setFocusedPane('pane-1')
  render(<SidebarCarouselContainer />)
  expect(screen.getByTestId('carousel-repo-path')).toHaveTextContent(repoPathForChat('chat-1'))
})
```

- [ ] **Step 3: Migrate each call site** per its category from Step 1.

- [ ] **Step 4: Run**

Run: `bun test`
Run: `bun tsc --noEmit`

- [ ] **Step 5: Commit**

```bash
git add -A web/src
git commit -m "fix(workspace): resolve workspace scope from the focused pane's chat, not one global"
```

### Task 28: Consume the real `/repos/:rid/tree` route; delete the `rows-from-repo.ts` bridge

**Files:**
- Delete: `web/src/components/sidebar/lib/rows-from-repo.ts` (Task 8's
  bridge).
- Create: `web/src/components/sidebar/lib/sidebar-api-client.ts` — calls
  `GET /repos/:rid/tree`, `PATCH /repos/:rid/tree`, `POST /repos/:rid/chats`,
  `POST /repos/:rid/chats/:id/promote`, `DELETE /repos/:rid/chats/:id`, `GET
  /repos/:rid/chats/:id/delete-preview` (backend plan Stage 7).
- Test: `web/src/__tests__/components/sidebar/lib/sidebar-api-client.test.ts`

**Interfaces:**
- Consumes: the backend plan's wire contract (its Stage 7, requires that
  plan fully merged first).
- Produces: `SidebarRow[]` sourced directly from the API response shape (each
  row already carrying `ownsWorktree`/`workspaceId`/`forkParentId`
  pre-resolved, per the model spec's own wire-contract note — this task
  deletes the client-side derivation Task 8 stood up as a bridge, since the
  server now does it).

- [ ] **Step 1: Write the failing test**

```ts
it('fetches the tree and maps directly to SidebarRow, no client-side derivation', async () => {
  mockFetch('/api/v0/repos/repo-1/tree', { rows: [{ id: 'chat-1', type: 'chat', workspaceId: 'ws-1', ownsWorktree: true, /* ... */ }] })
  const rows = await fetchSidebarTree('repo-1')
  expect(rows[0].ownsWorktree).toBe(true)
})
```

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement**, delete `rows-from-repo.ts`, wire
  `sidebar-api-client.ts` into whatever mounts `SpaceScroller` (C.1).

- [ ] **Step 4: Run the full suite**

Run: `bun test`
Run: `bun tsc --noEmit`

- [ ] **Step 5: Commit**

```bash
git add -A web/src/components/sidebar/
git commit -m "feat(sidebar): consume /repos/:rid/tree directly, delete the client-side row bridge"
```

---

### Task 29: Row actions — rename, lock/unlock, and branch import via a right-click menu

**Added post-hoc, by the controller, after Task 8 landed.** Task 8 retired
`workspace-tree.tsx`/`agent-chats-panel.tsx` and disclosed (per this plan's
own "Known gap left to the executor" note above) that rename, lock/unlock,
and branch import have no home on `SidebarRow`'s 4-prop surface any more.
Verified by the controller: no task 9-28 reintroduces any of these three
verbs. Multiselect and drag-driven "group into folder" are correctly left
to Part G (Tasks 20-23) — this task covers only the single-row, non-drag
verbs a right-click menu offers.

**Files:**
- Create: `web/src/components/sidebar/lib/row-actions.ts`
- Create: `web/src/components/sidebar/row-context-menu.tsx`
- Create: `web/src/components/sidebar/rename-dialog.tsx`
- Restore verbatim: `web/src/components/layout/repo-import-dialog.tsx` —
  `git show 9ad89156:web/src/components/layout/repo-import-dialog.tsx > web/src/components/layout/repo-import-dialog.tsx`
  (it was deleted by Task 8 only because its one caller, `workspace-tree-context.tsx`,
  was deleted — the dialog itself is self-contained: props are
  `projectId`/`repoId`/`defaultBranch`/`open`/`onOpenChange`/`onImport`, no
  internal dependency on anything else Task 8 removed. Confirm this via
  `git show 9ad89156:web/src/components/layout/repo-import-dialog.tsx | head -40`
  before restoring — if its actual dependencies differ from this plan's
  belief, treat that as a real finding and adjust.)
- Modify: `web/src/components/sidebar/sidebar-row.tsx` — add
  `data-sidebar-row-id={row.id}` to the root `role="treeitem"` div (the only
  change; every existing prop/behavior stays exactly as Task 5 shipped it).
- Modify: `web/src/components/layout/sidebar-tree-panel.tsx` — mount the
  context menu and both dialogs, wire their callbacks.
- Test: `web/src/__tests__/components/sidebar/lib/row-actions.test.ts`
- Test: `web/src/__tests__/components/sidebar/row-context-menu.test.tsx`
- Test: `web/src/__tests__/components/sidebar/rename-dialog.test.tsx`

**Interfaces:**
- Consumes: `SidebarRow` type (Task 4), `SidebarRow` component (Task 5,
  extended additively), `SidebarTreePanel` (Task 8), `@/components/ui/context-menu`'s
  `ContextMenu`/`useContextMenu`/`ContextMenuItem`, `@/components/ui/dialog`,
  `@/lib/api`'s `setWorkspaceLock`/`postWorkspace`/`importBranches`/
  `renameWorkspaceBranch`, `@/lib/store/sidebar`'s `useSidebarStore`.
- Produces:
  ```ts
  // row-actions.ts — restored from the deleted workspace-tree-actions.ts,
  // trimmed to single-row use (no PendingRowHooks/optimistic spinner rows —
  // the batch import fires and relies on the existing WS-driven cache to
  // surface new rows, same as create/rename/delete already do with no
  // optimistic write, per that file's own documented rationale).
  export function projectIdForRepo(repoId: string): string | undefined
  export function performRenameWorkspaceBranch(wsId: string, branch: string): Promise<void>
  export function performRenameFolder(folderId: string, name: string): Promise<void>
  export function performRenameRow(rowId: string, name: string): Promise<void>
  export function performSetWorkspaceLock(wsId: string, locked: boolean | null): Promise<void>
  export function performImportBranches(repoId: string, branches: string[]): Promise<void>
  export function performCreateFolder(parentId: string): Promise<void>
  ```
  ```tsx
  // row-context-menu.tsx
  interface SidebarRowContextMenuProps {
    treeRef: React.RefObject<HTMLElement | null>
    rows: SidebarRow[]           // to look up kind/ownsWorktree/parentId by id
    onRename: (rowId: string) => void      // opens rename-dialog.tsx
    onImport: (repoRowId: string) => void  // opens the restored RepoImportDialog
  }
  function SidebarRowContextMenu(props: SidebarRowContextMenuProps): JSX.Element | null
  ```
  ```tsx
  // rename-dialog.tsx
  interface RenameDialogProps {
    open: boolean
    initialValue: string
    onOpenChange: (open: boolean) => void
    onConfirm: (name: string) => void
  }
  function RenameDialog(props: RenameDialogProps): JSX.Element
  ```

- [ ] **Step 1: Write the failing test for the pure logic**

```ts
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { performRenameRow, performSetWorkspaceLock } from '@/components/sidebar/lib/row-actions'
import { useSidebarStore } from '@/lib/store/sidebar'
import * as api from '@/lib/api'

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof api>()),
  renameWorkspaceBranch: vi.fn().mockResolvedValue(undefined),
  setWorkspaceLock: vi.fn().mockResolvedValue(undefined),
}))

describe('row-actions', () => {
  beforeEach(() => {
    useSidebarStore.setState({
      repos: [{
        id: 'repo-1', projectId: 'proj-1', name: 'repo', defaultWorkspaceId: 'ws-home',
        defaultBranch: 'main', defaultWorkspaceStatus: 'clean', defaultWorking: false,
        workspaces: [{ id: 'ws-1', branch: 'feature-x', status: 'clean', age: '' }],
        folders: [],
      }],
    })
  })

  it('renaming a workspace row calls renameWorkspaceBranch with its repo/project ids', async () => {
    await performRenameRow('ws-1', 'feature-y')
    expect(api.renameWorkspaceBranch).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-1', 'feature-y')
  })

  it('locking a workspace calls setWorkspaceLock with locked: true', async () => {
    await performSetWorkspaceLock('ws-1', true)
    expect(api.setWorkspaceLock).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-1', true)
  })
})
```

Verify the real current shape of `Repo`/`Workspace` in `web/src/lib/store/sidebar.ts`
before finalizing this fixture — Task 8's report already found `age` is
required and there is no separate `ChatFolder`; confirm nothing else has
drifted since.

- [ ] **Step 2: Run, verify fail**

- [ ] **Step 3: Implement `row-actions.ts`** by restoring the four
  `export function` bodies (`projectIdForRepo`, `performRenameWorkspaceBranch`,
  `performRenameFolder`, `performRenameRow`) verbatim from
  `git show 9ad89156:web/src/components/layout/workspace-tree-actions.ts`
  (they are pure — depend only on `useSidebarStore` and `@/lib/api`, nothing
  from the deleted React context) and add two new, small functions:
  ```ts
  export async function performSetWorkspaceLock(
    wsId: string,
    locked: boolean | null,
  ): Promise<void> {
    const repo = useSidebarStore.getState().repos.find((r) => r.workspaces.some((w) => w.id === wsId))
    const projectId = repo?.projectId
    if (!repo || !projectId) return
    try {
      await setWorkspaceLock(projectId, repo.id, wsId, locked)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update lock')
    }
  }

  export async function performImportBranches(repoId: string, branches: string[]): Promise<void> {
    if (branches.length === 0) return
    const projectId = projectIdForRepo(repoId)
    if (!projectId) return
    try {
      await importBranches(projectId, repoId, branches)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to import branches')
    }
  }
  ```

- [ ] **Step 4: Run, verify pass**

- [ ] **Step 5: Write the failing test for the context menu's item list**

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { SidebarRowContextMenu } from '@/components/sidebar/row-context-menu'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

const rows: SidebarRow[] = [
  { id: 'ws-1', kind: 'branch', parentId: null, order: 0, label: 'repo', ownsWorktree: true, workspaceId: 'ws-1', working: false, hasView: false },
  { id: 'folder-1', kind: 'folder', parentId: 'ws-1', order: 0, label: 'Bugs', ownsWorktree: false, workspaceId: null, working: false, hasView: false },
]

function renderMenu() {
  const treeRef = { current: document.createElement('div') }
  document.body.appendChild(treeRef.current)
  const onRename = vi.fn()
  const onImport = vi.fn()
  render(<SidebarRowContextMenu treeRef={treeRef} rows={rows} onRename={onRename} onImport={onImport} />)
  return { treeRef, onRename, onImport }
}

describe('SidebarRowContextMenu', () => {
  it('right-clicking the project-home row offers Rename, Lock, and Import', () => {
    const { treeRef } = renderMenu()
    const target = document.createElement('div')
    target.setAttribute('role', 'treeitem')
    target.setAttribute('data-sidebar-row-id', 'ws-1')
    treeRef.current.appendChild(target)
    fireEvent.contextMenu(target)
    expect(screen.getByText('Rename')).toBeInTheDocument()
    expect(screen.getByText('Lock')).toBeInTheDocument()
    expect(screen.getByText('Import branches')).toBeInTheDocument()
  })

  it('right-clicking a folder row offers only Rename', () => {
    const { treeRef } = renderMenu()
    const target = document.createElement('div')
    target.setAttribute('role', 'treeitem')
    target.setAttribute('data-sidebar-row-id', 'folder-1')
    treeRef.current.appendChild(target)
    fireEvent.contextMenu(target)
    expect(screen.getByText('Rename')).toBeInTheDocument()
    expect(screen.queryByText('Lock')).not.toBeInTheDocument()
    expect(screen.queryByText('Import branches')).not.toBeInTheDocument()
  })
})
```

- [ ] **Step 6: Run, verify fail**

- [ ] **Step 7: Implement `row-context-menu.tsx`**, mirroring the deleted
  `row-context-menu.tsx`'s architecture (a sibling of the tree, native
  `contextmenu` listener on `treeRef.current`, `e.target.closest('[role="treeitem"]')`
  then read `data-sidebar-row-id` — NOT the old `readDropRow`/`dragSubjectsFor`,
  which belong to the drag system Part G owns) rather than a hook inside the
  tree, for the same re-render reason the original documented: opening the
  popup must not re-render every row. Item rules, decided from the looked-up
  `SidebarRow` by id (no multiselect — always exactly one row, since this
  task does not touch the drag/selection system):
  - **Rename** — always offered.
  - **Lock** / **Unlock** — offered only when `row.ownsWorktree` is true
    (a chat/folder row never owns a worktree to lock). Label reads the
    row's current lock state — this task does not have a `locked` field on
    `SidebarRow` yet, so read it from `useSidebarStore`'s matching
    `Workspace.status === 'locked'` directly inside the menu's item-builder,
    not from the `SidebarRow` prop.
  - **Import branches** — offered only for the project-home row
    (`row.kind === 'branch' && row.parentId === null`).
  - **New folder** — offered on any `branch`/`folder` kind row (a row that
    can itself hold children per §3.1's "containers are always expandable").
    Task 8's review found `createFolder` (`@/lib/api/sidebar-placement`) has
    zero live callers post-deletion — this item is how it gets one again.
    Calls `performCreateFolder(rowId)` directly (no dialog): creates a
    folder named `'New folder'` under `rowId` via `createFolder`, matching
    the deleted `workspace-tree-context.tsx`'s `NEW_FOLDER_NAME` constant
    and default-name behavior; the user renames it afterward via this same
    menu's Rename item. Add `performCreateFolder(parentId: string): Promise<void>`
    to `row-actions.ts`'s exports, restored/adapted from the deleted
    `confirmCreate`'s folder branch (`git show 9ad89156:web/src/components/layout/workspace-tree-context.tsx`,
    search `createFolder(`).
  Calls `onRename(rowId)` / `onImport(repoRowId)` for the Rename/Import
  items; Lock/Unlock and New folder call `performSetWorkspaceLock`/
  `performCreateFolder` directly (no dialog needed for either).

- [ ] **Step 8: Run, verify pass**

- [ ] **Step 9: Add `data-sidebar-row-id={row.id}` to `sidebar-row.tsx`**

One-line addition to the existing root div's props (alongside
`role="treeitem"` `tabIndex={0}` `{...dragProps}`) — no other change to
that file. Re-run Task 5's existing test file
(`web/src/__tests__/components/sidebar/sidebar-row.test.tsx`) to confirm
all 10 of its tests still pass unmodified.

- [ ] **Step 10: Write the failing test for the rename dialog**

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { RenameDialog } from '@/components/sidebar/rename-dialog'

describe('RenameDialog', () => {
  it('confirms with the edited value', () => {
    const onConfirm = vi.fn()
    render(<RenameDialog open initialValue="feature-x" onOpenChange={vi.fn()} onConfirm={onConfirm} />)
    const input = screen.getByRole('textbox')
    fireEvent.change(input, { target: { value: 'feature-y' } })
    fireEvent.click(screen.getByRole('button', { name: /rename/i }))
    expect(onConfirm).toHaveBeenCalledWith('feature-y')
  })
})
```

- [ ] **Step 11: Run, verify fail**

- [ ] **Step 12: Implement `rename-dialog.tsx`** using `@/components/ui/dialog`
  (`Dialog`, `DialogPopup`, `DialogHeader`, `DialogTitle`) and
  `@/components/ui/input`'s `Input`, seeded with `initialValue`, a Rename
  button calling `onConfirm(value)` then `onOpenChange(false)`.

- [ ] **Step 13: Run, verify pass**

- [ ] **Step 14: Restore `repo-import-dialog.tsx`** per the file list above,
  then wire everything into `sidebar-tree-panel.tsx`: local state for
  `renamingRowId: string | null` and `importRepoRowId: string | null`; mount
  `<SidebarRowContextMenu treeRef={treeRef} rows={rows} onRename={setRenamingRowId} onImport={setImportRepoRowId} />`,
  `<RenameDialog open={renamingRowId != null} initialValue={...} onConfirm={(name) => performRenameRow(renamingRowId!, name)} .../>`,
  and `<RepoImportDialog open={importRepoRowId != null} onImport={(branches) => performImportBranches(repoIdFor(importRepoRowId!), branches)} .../>`
  (resolve `projectId`/`repoId`/`defaultBranch` for the dialog from the same
  `useSidebarStore` repo lookup `row-actions.ts` uses). Give the tree
  container a `ref` if `sidebar-tree-panel.tsx` doesn't already hold one.

- [ ] **Step 15: Run the full scoped suite**

Run: `./node_modules/.bin/vitest run src/__tests__/components/sidebar/`
Run: `bun tsc --noEmit` — confirm zero new errors versus this task's own
starting baseline (record the exact before/after count, same methodology
Task 8 used).

- [ ] **Step 16: Commit**

```bash
git add web/src/components/sidebar/ web/src/components/layout/repo-import-dialog.tsx web/src/components/layout/sidebar-tree-panel.tsx
git commit -m "feat(sidebar): rename, lock/unlock, and branch import return via a right-click menu"
```

---

## Self-review notes (from writing this plan)

- **Spec coverage:** every row of the surface spec's own Handoff table (§13)
  has a Part above: rail → Parts B/C, window chrome → C.3, space header → C.2,
  tree → Part B, Recents → Part D, card → Part E, pane → Part F, drag → Part
  G. Deleting (§9) is Part H, not in that table but load-bearing. Part A and
  Part I are plan-internal (the data-layer migration the spec's prose implies
  but doesn't itself decompose into tasks) and Part I is this plan's version
  of the model spec's own stage 8.
- **Type consistency check:** `SidebarRow` (B.1) is used with the same field
  names in every later part (C, D, G, H) — confirmed. `PaneGroup.chatId`/
  `editorTabIds` (A.1) match across A.2, D.2/D.3, F.1/F.2, I.1 — confirmed.
- **Known gap left to the executor:** `workspace-tree-context.tsx`'s split
  between `WorkspaceTreeActionsContext` (create/rename/import/lock) and
  `WorkspaceTreeDragContext` has no direct equivalent named above — Task
  B.5/G.2's executor must find where create/rename/import/lock actions land
  once the old context is deleted (most likely as plain callback props into
  `SidebarTree`, matching `SidebarTreeProps` in Task 6) and should treat
  that as part of B.5's/G.2's own scope rather than opening a new task for it.

---

### Task 30: Mount Spaces for real — `SpaceScroller` + `RecentsBand` replace `SidebarTreePanel`

**Added post-hoc, by the controller, after Task 15 landed.** Task 15 deleted
the carousel's `workspaces` panel (which rendered `SidebarTreePanel`, the
app's only reachable mount point for the sidebar tree since Task 8), on the
stated premise that "Part B's `SidebarTree` + Part D's `RecentsBand` now own
that surface directly, not as carousel panels." Verified by the controller:
**no task anywhere in this plan actually performs that mount.** `SpaceScroller`
(Task 9), `SpaceHeader` (Task 10), the window-chrome space marks (Task 11),
and `RecentsBand` (Task 12) are all built and reviewed clean, but zero of
them have a live consumer — confirmed by grep before writing this task. Left
as-is, the app ships with NO way to see or navigate the workspace tree at
all. This is not a disclosed-and-deferred gap like Task 29's (secondary
actions) — this is the sidebar's primary navigation surface. Execute
immediately after Task 15, before Task 16, so the app never carries this
regression forward.

**Files:**
- Modify: `web/src/components/layout/ide-shell.tsx` — mount `SpaceScroller`
  between `SidebarProjectHeader` and `SidebarCarousel` (its real current
  layout, confirmed: `<SidebarProjectHeader .../> <ContextPill /> <SidebarCarousel .../>`
  at line ~131-144), matching the design spec's own layout diagram (§2's
  ASCII sketch: `SPACES` region `flex: 1` above, `CARD` — the carousel —
  floating and last).
- Modify: `web/src/components/sidebar/space-scroller.tsx` — each panel
  currently renders only `<SidebarTree>` (Task 9); per spec's own layout
  diagram, "the tree and Recents are one scrolling group... below it, the
  entries" — extend each panel to also render `<RecentsBand>` below its
  `<SidebarTree>`, inside the SAME scroll region (one `ScrollArea` per
  panel, not two). Add `recentsForProject: (projectId: string) => RecentsEntry[]`
  and `onFocusRecent`/`onCloseRecent` props alongside the existing
  `rowsForProject`/`onOpen`/`onTrash`/`onCreate`, matching the established
  per-project-callback pattern this component already uses.
- Create: `web/src/components/layout/space-content-actions.ts` (or fold into
  wherever fits best on investigation) — `resolveRow`, `handleOpen`,
  `handleTrash`, `handleCreate` extracted from `sidebar-tree-panel.tsx`
  (read that file in full first: it is fully self-contained, already
  correct, and already reviewed clean across Tasks 8/29 — these handlers
  resolve a row's owning repo by searching ALL of `useSidebarStore`'s repos
  regardless of which subset is currently rendered, so they need NO change
  to work per-project; only the RENDERED `rows` need to be filtered by
  project). Confirm this claim by reading the file before assuming it.
- Create: a project-scoped rows/entries deriver — `rowsForProject(projectId)`
  = `repos.filter(r => r.projectId === projectId).flatMap(rowsFromRepo)`
  (repos already carry `projectId`, confirmed in `lib/store/sidebar.ts`);
  `recentsForProject(projectId)` = `deriveRecentsEntries` (Task 13) fed only
  the panes/working-chats belonging to workspaces under that project's
  repos — investigate the real, current way to resolve "which workspace does
  this pane's chat belong to" (likely via the workspace store registry or
  the chat's own `workspaceId`) before implementing; do not assume a
  mechanism that doesn't exist.
- Modify: hoist `RemovalTray`, `RenameDialog`, `RepoImportDialog`,
  `SidebarRowContextMenu`, and the "New Project" button (all currently
  rendered ONCE inside `sidebar-tree-panel.tsx`) to mount ONCE at the
  `ide-shell.tsx` level (or one level above `SpaceScroller`, your call) —
  NOT once per `SpaceScroller` panel, which would duplicate trays/dialogs
  per project.
- Delete: `web/src/components/layout/sidebar-tree-panel.tsx` and its test,
  once its logic is fully absorbed above — confirm via `grep -rl "SidebarTreePanel"`
  that nothing else references it before deleting (Task 15's own diff
  already stopped rendering it, so this should show zero remaining
  importers).
- Test: extend/create tests under `web/src/__tests__/components/layout/`
  and `web/src/__tests__/components/sidebar/` mirroring whatever new files
  you create, plus updated tests for `ide-shell.tsx` and `space-scroller.tsx`.

**Interfaces:**
- Consumes: `SpaceScroller` (Task 9, extend per above), `RecentsBand` (Task
  12), `deriveRecentsEntries` (Task 13), `rowsFromRepo` (Task 8),
  `SidebarTreePanel`'s existing handler logic (Task 8/29, to extract/reuse,
  not rewrite from scratch), `ide-shell.tsx`'s existing `allProjects`/
  `activeProjectIdFromRoute`/`handleSelectProject` (already live, used by
  `SidebarProjectHeader` per Task 11 — reuse the SAME values for
  `SpaceScroller`'s `projects`/`activeProjectId`/`onActiveProjectChange`,
  do not derive a second, possibly-inconsistent copy).

- [ ] **Step 1: Investigate and confirm the plan above against real code**

Read `ide-shell.tsx`, `sidebar-tree-panel.tsx`, `space-scroller.tsx`,
`recents-band.tsx`, `recents-entries.ts` in full. Confirm or correct every
factual claim above (the real layout order, the real handler
extractability, the real way to map a pane/chat to its owning project)
before writing code. This session has repeatedly found brief-vs-reality
drift on tasks of this size — treat that as the default expectation here,
not the exception.

- [ ] **Step 2: Write failing tests** for the project-scoped derivation
  functions (`rowsForProject`/`recentsForProject` — real fixtures, real
  assertions that a project's rows/entries genuinely exclude another
  project's) and for `SpaceScroller`'s extended per-panel rendering
  (`RecentsBand` renders below `SidebarTree` in the same panel).

- [ ] **Step 3: Run, verify fail**

- [ ] **Step 4: Implement** the extraction, the derivers, the `SpaceScroller`
  extension, and the `ide-shell.tsx` mount. Verify manually (read the
  resulting JSX tree, or use the Tauri MCP dev-desktop verification this
  session's mandate requires) that a real multi-project setup shows each
  project's own tree+Recents when scrolled to, and that trash/rename/lock/
  import/create/New-Project all still work exactly as they did through
  `SidebarTreePanel` before this task.

- [ ] **Step 5: Run, verify pass** — full scoped suite plus a tsc before/after
  count (known pre-existing baseline: 457 errors as of Task 15).

- [ ] **Step 6: Delete `sidebar-tree-panel.tsx`** once confirmed dead, per
  the grep check above.

- [ ] **Step 7: Commit**

```bash
git add web/src/components/layout/ web/src/components/sidebar/
git commit -m "feat(sidebar): mount Spaces for real — SpaceScroller+RecentsBand replace SidebarTreePanel"
```

---

### Task 31: `PaneContainer` migrates off the old buffer shape — and chat becomes a first-class pane concern, not a buffer type

**Added post-hoc, by the controller, after Task 17 landed.** Task 2's original
"confirmed consumer-regression window" ruling named 8 files still reading the
OLD `PaneGroup` shape (`bufferIds`/`activeBufferId`/`previewBufferId` +
`activatePaneBuffer`/`addBufferToPane`/`moveBufferToPane`), explicitly
disclosed as "not fixed until Part F (Tasks 17-19)... and whichever tasks
touch tab-bar.tsx/pane-container.tsx directly." Task 17 fixed `tab-bar.tsx`
(dropping tsc errors 457→433) but explicitly flagged that **no task in this
entire plan claims `pane-container.tsx`** — verified independently by the
controller: it is the LIVE, mounted primary renderer for every editor/
terminal/agent-chat surface in the app (reachable via
`ide-shell.tsx → workspace-host.tsx → workspace-view.tsx →
workspace-layout-root.tsx → pane-node-renderer.tsx/split-view-root.tsx →
PaneContainer`), and it still references the removed fields/actions
directly — 23 of the project's 433 remaining tsc errors are in this one
file. **This is not a secondary feature gap like Task 29's, and not a
navigation gap like Task 30's — this is the core pane-rendering surface,
and it does not currently compile.** Execute immediately after Task 17,
before Task 18.

**Files:**
- Modify: `web/src/features/panes/components/pane-container.tsx` (full
  content read by the controller — 655 lines, reproduced findings below).
- Test: `web/src/__tests__/features/panes/components/pane-container.test.tsx`
  (create if missing, extend if present — confirm which on investigation).

**The structural change beneath the field renames — read this before touching anything**

`PaneGroup` no longer has an `'agentChat'` buffer type in its content union
at all (Task 1, already committed) — `pane.chatId`/`pane.runnerId` are now
their OWN top-level fields on `PaneGroup`, entirely separate from
`editorTabIds`. Today's `pane-container.tsx` still treats the chat as ONE
MORE FILTERED BUFFER TYPE inside `paneBuffers` (lines 611-635: a `.filter(b
=> b.type === 'agentChat')` keep-alive block, mirroring the terminal
keep-alive block above it at lines 577-600). That is now structurally
wrong, not just a field-name mismatch: **a pane's chat must render
whenever `pane.chatId` is set, independent of `editorTabIds`/
`activeEditorTabId` entirely** (a pane can hold a chat with zero editor
tabs, matching Task 17's own "a pane with only its chat draws no [tab]
bar at all" rule) — it does not compete with the editor tabs for "which
one is active," because it isn't in that list any more.

**Files/interfaces already built that this task must consume, not
reinvent** (all already committed and reviewed clean):
- `PaneGroup` (`web/src/features/panes/types/pane.ts`, Task 1):
  `{id, type:'group', chatId, runnerId, editorTabIds, activeEditorTabId,
  editorOpen, locked?}`.
- `EditorTabBase` (`web/src/features/panes/types/pane-content.ts`, Task 1):
  carries `isPreview`/`isPinned`/`isUncloseable` per tab — replaces the old
  pane-level `previewBufferId` (line 506's `pane.previewBufferId ===
  buffer.id` check becomes reading the ACTIVE TAB's own `isPreview` field).
- `pane-slice.ts` actions (Tasks 1-3): `activateEditorTabInPane`,
  `addEditorTabToPane`, `removeEditorTabFromPane`, `moveEditorTabToPane`,
  `setEditorTabPreview`, `setEditorTabPinned`, `reorderEditorTabs`,
  `getPaneByEditorTabId`, `clearEditorTabPreviewEverywhere`,
  `switchToNextEditorTab`, `switchToPreviousEditorTab`, `setPaneChat` —
  verify these are the REAL, current names/signatures by reading
  `pane-slice.ts` directly (do not assume from this list alone; Task 17's
  report may have touched some of these too — read its diff).
- `ChatHead`/`SplitToggleButton` (Task 17, `tab-bar.tsx`) — the pane-top row
  already resolves and displays the chat's identity; `PaneContainer` does
  NOT need to re-derive chat metadata for the top row, only for its own
  content-area rendering (mounting `AgentChatPane`).

**What to change, concretely, in `pane-container.tsx`:**
1. `useBuffersByIds(pane.bufferIds)` → `useBuffersByIds(pane.editorTabIds)`
   (confirm `useBuffersByIds`'s real signature still fits — it may already
   be generic over "an array of content ids," in which case only the field
   read changes).
2. `pane.activeBufferId` → `pane.activeEditorTabId` throughout.
3. `activatePaneBuffer` → `activateEditorTabInPane`; `addBufferToPane` →
   `addEditorTabToPane`; `moveBufferToPane` → `moveEditorTabToPane`,
   everywhere they appear (the drag/drop handlers, `handleTabClick`,
   `openFileTreeDropInPane`, `handleSplitDrop` — read every call site,
   there are several).
4. Delete the `'agentChat'` filter/keep-alive block (lines 611-635)
   entirely. Replace it with a keep-alive block gated on `pane.chatId`
   directly (mounted whenever `pane.chatId` is set, `isVisible` always
   true for it since it doesn't compete with editor tabs for "active" —
   confirm this against spec §7's actual rendering rule for a pane holding
   both a chat and open files, since this is the one genuinely new design
   question this migration surfaces: read `docs/superpowers/specs/2026-08-28-sidebar-and-pane-surface-design.md`
   §7 in full for how a pane's chat and its editor view coexist — likely
   "Two views" per Task 18's own title, meaning the chat and the editor
   tab strip are the SxS/stacked halves per `use-chat-presentation.ts`,
   NOT alternatives on the same content stack. If so, `PaneContainer`'s own
   job shrinks to hosting the editor-tab content area ALONE, and the chat
   half is a sibling this component (or whatever wraps it) renders
   alongside — investigate and adapt rather than forcing the chat into the
   same Suspense/switch-statement machinery the editor tab types use).
5. `pane.previewBufferId === buffer.id` → read the active editor tab's own
   `isPreview` field instead (find it via `editorTabIds`/whatever the tab
   objects are, not a separate pane-level id).
6. `EditorBufferShell`/`PaneRenderBuffer`/`AgentChatContent` type imports —
   `AgentChatContent` no longer exists (Task 1 deleted it) — remove the
   import and every type reference to it.
7. `NewTabView` fallback (line 570, `{!activeBuffer && <NewTabView
   paneId={pane.id} />}`) — confirm `NewTabView` itself still exists post
   Task 1-3 (it was NOT deleted, only the `'newTab'` BUFFER TYPE was) and
   still makes sense as the fallback for "this pane has no chat and no
   editor tabs" — read its current real props/behavior before assuming.

**Interfaces:**
- Consumes: everything listed above.
- Produces: no new public interface — this is an internal migration of an
  already-mounted component; its own `PaneContainerProps` (`pane`,
  `position`) are unchanged.

- [ ] **Step 1: Investigate thoroughly before writing any code**

Read `pane-container.tsx` in full (already reproduced above, but re-read
the live file — it may have drifted further since this task was written).
Read `pane-slice.ts`'s real current action signatures. Read spec §7 in
full for the actual chat/editor-tabs coexistence model. Read `EditorPane`,
`AgentChatPane`, `NewTabView`'s real current prop signatures (all
consumed here) before wiring anything.

- [ ] **Step 2: Write failing tests** covering at minimum: a pane with a
  chat and zero editor tabs renders the chat, not `NewTabView`; a pane
  with editor tabs and no chat renders the active tab's content, no chat
  surface; a pane with both renders both (per whatever the real §7
  coexistence model turns out to be); the active tab's preview styling
  reads from the tab's own `isPreview`, not a removed pane-level field;
  drag-drop of a file/tab into this pane calls the renamed editor-tab
  actions, not the old buffer actions.

- [ ] **Step 3: Run, verify fail**

- [ ] **Step 4: Implement** per the concrete change list above, adapted to
  whatever Step 1's investigation actually finds.

- [ ] **Step 5: Run, verify pass.** Also run a project-wide `tsc --noEmit`
  before/after count — this task should measurably shrink the remaining
  error count (433 baseline as of Task 17), not just avoid growing it,
  since it closes the single largest remaining chunk of Task 2's original
  regression window.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/panes/components/pane-container.tsx web/src/__tests__/features/panes/components/pane-container.test.tsx
git commit -m "fix(panes): PaneContainer migrates to editorTabIds/chatId — chat is a first-class pane concern, not a buffer type"
```

---

### Task 32: `buffer-slice.ts` migrates `openContent`/`openNewTab` — the app's actual "open a file" path

**Added post-hoc, by the controller, after Task 31 landed.** Task 31's
implementer disclosed (correctly) that `buffer-slice.ts` — the store
action every "open a file," "open a terminal," "start a chat" call in the
app ultimately routes through — is still built entirely around the OLD
pre-Tasks-1-3 model: a `'newTab'` PLACEHOLDER BUFFER object minted into
`state.buffers` and referenced by id from `pane.bufferIds`, and an
`'agentChat'` buffer type competing for a slot in that same list. Both
concepts are gone from the real `PaneContent`/`EditorTabContentType` union
(Task 1) and the real `PaneGroup` shape (`editorTabIds`, and `chatId` as
its own top-level field, established Tasks 1-3 and confirmed live by Task
31 — a pane's chat is not a tab any more, it is set via `setPaneChat`).
Verified independently by the controller: `npx tsc --noEmit -p .` shows
~30 real errors rooted in this one file (calls to `addBufferToPane`,
`activatePaneBuffer`, `removeBufferFromPane`, `getPaneByBufferId`,
`setPanePreviewBuffer` — none of which exist on `PaneActions` any more;
reads of `pane.bufferIds`, which doesn't exist on `PaneGroup`). **This is
the function every "open a file" gesture in the live app calls — it
currently throws at runtime.** No task anywhere in this plan claims this
file's `openContent`/`openNewTab` migration (Task 3 touched this file, but
only added `syncSoleEditorTabCloseability`, already correctly built
against the new shape — the OLDER, much larger `openContent`/`openNewTab`
functions were untouched by Task 3 and remain fully on the old model).
Execute immediately after Task 31, before Task 18.

**Files:**
- Modify: `web/src/features/workspace/stores/slices/buffer-slice.ts` (full
  content already read by the controller — reproduced findings below).
- Modify: `web/src/features/panes/types/pane-content.ts`'s
  `OpenEditorTabSpec` union, if `'newTab'`/`'agentChat'` variants still
  exist there (verify — they may already be gone, in which case
  `buffer-slice.ts`'s own `spec.type === 'newTab'`/`'agentChat'` branches
  are dead code the compiler is already flagging as unreachable, not a
  live spec shape).
- Modify: every real call site that currently calls
  `openContent({type:'agentChat', ...})` to start/open a chat (grep for
  it — likely wherever a chat gets created/re-focused, e.g. whatever
  replaced the old `createChat`/`newChat` flow this session's earlier
  tasks touched) — these must call `setPaneChat(paneId, chatId, runnerId)`
  directly instead; opening a chat is no longer "adding a tab."
- Delete: `makeNewTabBuffer`, `findNewTabInPane`,
  `AUTO_EVICTION_PROTECTED`'s `'newTab'` member (the whole New-Tab-as-a-
  buffer machinery) once confirmed dead.
- Test: `web/src/__tests__/features/workspace/stores/slices/buffer-slice.test.ts`
  (extend/fix — many of its existing cases likely assert the old
  mint-a-newTab-buffer/agentChat-buffer behavior and need rewriting, not
  just field renames).

**What the controller already found, concretely:**
- `openContent`'s dedup-and-jump logic for an existing `'agentChat'`/
  `'terminal'` buffer calls `getPaneByBufferId`/`activatePaneBuffer` (both
  gone) — needs `getPaneByEditorTabId`/`activateEditorTabInPane` for the
  terminal case; the `'agentChat'` case shouldn't be reached via
  `openContent` at all any more (see below).
- The auto-eviction block reads `pane.bufferIds` and calls
  `removeBufferFromPane` (gone) — needs `pane.editorTabIds`/
  `removeEditorTabFromPane`.
- The final "push into the active pane" step calls `addBufferToPane`/
  `setPanePreviewBuffer` (gone) — needs `addEditorTabToPane`/
  `setEditorTabPreview`, and (per Task 31's own finding)
  `addEditorTabToPane` takes the tab's own object, not a bare id — confirm
  this signature and adapt.
- `spec.type === 'newTab'` inside `openContent`'s buffer-construction
  branch, and the entire `openNewTab` action, mint/manage a `NewTabContent`
  object that can no longer type-check (`NewTabContent` was deleted from
  the union in Task 1). Given `PaneContainer` (Task 31) already falls back
  to rendering `NewTabView` directly whenever `pane.editorTabIds.length
  === 0` (no buffer object needed at all for that to happen), `openNewTab`
  is very likely now OBSOLETE — investigate every real call site
  (`grep -rl "openNewTab"`) and determine whether each one can simply be
  deleted (if the pane already renders its empty state for free) or needs
  a different, much smaller replacement (e.g., just ensuring the target
  pane exists and has empty `editorTabIds`, with no buffer object to
  manage at all).
- `spec.type === 'agentChat'` inside `openContent`'s buffer-construction
  branch: per Task 31's finding, a chat is set via `setPaneChat`, not
  opened as a tab. Investigate every real call site of
  `openContent({type:'agentChat', ...})` and redirect each one to call
  `setPaneChat` instead. This is a genuine behavior change, not a rename
  — confirm with each call site's surrounding logic (e.g. does it need to
  find/create a pane first?) rather than doing a blind find-replace.

**Interfaces:**
- Consumes: `pane-slice.ts`'s real current action signatures (Tasks 1-3,
  extended by Task 31 — re-verify against the file as it now stands, not
  this task's own guesses).
- Produces: no new public interface — `BufferActions`'s own signature
  (`openContent`, `closeBuffer`, etc.) stays the same shape unless
  `openNewTab` is found to be genuinely obsolete and removed, in which
  case update every real caller and this interface together.

- [ ] **Step 1: Investigate thoroughly before writing any code**

Read `buffer-slice.ts` in full (already substantially read by the
controller above, but re-read the live file — it may have drifted).
Read `pane-slice.ts`'s real, current action signatures in full. Grep for
every real call site of `openContent`, `openNewTab`, and
`openContent({type:'agentChat'...})` specifically, across the whole
`web/src` tree, and read each one before deciding how to migrate it.

- [ ] **Step 2: Write failing tests** covering: opening a real file
  creates an editor tab via `addEditorTabToPane` (not the old action);
  opening an already-open terminal jumps to its existing pane via the
  renamed actions; auto-eviction at the tab cap correctly removes the
  evictee via `removeEditorTabFromPane`; whatever `openNewTab`'s
  resolution turns out to be (obsolete-and-removed, or a smaller
  replacement) is correctly tested; a chat-opening call site correctly
  calls `setPaneChat`, not `openContent`.

- [ ] **Step 3: Run, verify fail**

- [ ] **Step 4: Implement** per the findings above, adapted to whatever
  Step 1's investigation actually finds.

- [ ] **Step 5: Run, verify pass.** Run a project-wide `tsc --noEmit`
  before/after count (402 baseline as of Task 31) — this task should
  measurably shrink the remaining error count, since it closes the
  largest remaining chunk of the original regression window.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/workspace/stores/slices/buffer-slice.ts web/src/features/panes/types/pane-content.ts web/src/__tests__/features/workspace/stores/slices/buffer-slice.test.ts
git commit -m "fix(panes): buffer-slice.ts's openContent/openNewTab migrate off the deleted newTab/agentChat buffer model"
```

---

### Task 33: `onDrop` gets real persistence — placing a row is not just a gesture

**Added post-hoc, by the controller, after Task 21 landed.** Task 21 built
`useSidebarDrag`'s full mechanism (hit-test, ghost, hairline, edge-scroll,
subtree cycle guard, pane-zone geometry) correctly and completely, but its
own report disclosed: `onDrop`/`onPaneDrop` are wired end-to-end through
`SidebarTree`/`RecentsBand` but resolve to a safe placeholder (a toast, no
real API call) in `web/src/components/sidebar/lib/drop-actions.ts`.
`onPaneDrop`'s real wiring is correctly Task 22's job (verified — its brief
explicitly covers the four-target pane-drop table). **`onDrop`'s real
persistence — actually reordering/reparenting a row when a tree-to-tree
drag completes — has no task anywhere in this plan.** Verified: grepped
the whole plan for `reparent|placeWorkspace|placeFolder`, zero hits outside
Task 21's own interface declaration. Without this, "One drag" (Part G,
this task's whole reason to exist) produces a convincing, fully-tested drag
gesture that changes nothing when released — worse than no drag UI at all,
since it invites the action and silently (toast-only) refuses it. Execute
immediately after Task 21/22, before Task 23.

**Files:**
- Modify: `web/src/components/sidebar/lib/drop-actions.ts` (Task 21's own
  placeholder file — replace the toast-only `onDrop` implementation with
  real API calls).
- Test: `web/src/__tests__/components/sidebar/lib/drop-actions.test.ts`
  (extend/rewrite).

**Reference: the OLD system's full placement-planning logic**, deleted by
Task 8 but recoverable via `git show 9ad89156:web/src/components/layout/drop-plan.ts`
— read it in full before starting; it is genuinely sophisticated (multi-row
moves, fork-parent lineage tracking via `workspaceAnchor`, folder-vs-workspace
container resolution, `resolvesToFirstChild`'s "after an expanded row" rule)
and gets the ordering/container math right in ways worth preserving. **Do
NOT port it verbatim** — it solves a strictly HARDER problem than this task
needs, because:
- It handles `project`/`repo`-kind drag subjects (reordering projects,
  moving repos between projects) — the NEW unified system doesn't expose
  that through this drag mechanism at all any more (`SpaceScroller`, Part
  C, already committed, handles "which project" as a horizontal scroll
  between full-width panels, not a tree drag; `rowsFromRepo` makes each
  repo its own root row within a project's forest). `SIDEBAR_DROP_POLICY`
  (Task 20, already committed) confirms the new system's real scope: rows
  of kind `branch`/`folder`/`chat` reordering/reparenting within one
  project's forest (crossing repos only for a `chat`-kind row with no
  worktree, per spec §8.3's exception) — narrower than the old 5-kind
  matrix.
- It builds `DropPlan.writes`/`capturePlacement` for OPTIMISTIC local
  painting before the daemon confirms. This entire plan has established,
  repeatedly and deliberately (Task 8's `performRenameWorkspaceBranch`,
  `performCreateFolder`, etc. — read their doc comments), a **no-optimistic-write**
  convention: fire the real API call, let the WS-driven cache apply the
  daemon's own confirmed state. Follow that same convention here — do NOT
  build a new optimistic-paint/undo mechanism just for drops when nothing
  else in this codebase does that any more.

**What to actually build, concretely:**
- A pure function (name your own, e.g. `planRowDrop`) that takes the
  drag's `subjects: SidebarRow[]`, `target: SidebarRow`, `mode: DropMode`
  (Task 20/21's real types), and the live `useSidebarStore` snapshot, and
  returns the ORDERED list of real API calls needed — adapting
  `drop-plan.ts`'s `planRowDrop`/`insertIndex`/`spliced`/`findNode`/
  `membersOf`/`workspaceAnchor`/`resolvesToFirstChild` logic (the
  ordering/container/lineage math is real and worth keeping) to the new
  `SidebarRow`/`SIDEBAR_DROP_POLICY` model, WITHOUT the `writes`/optimistic
  half.
- Fire each call via the real, already-existing API functions:
  `placeWorkspace(projectId, repoId, wsId, {folderId?, order})` and
  `placeFolder(projectId, repoId, folderId, {parentId?, order})`
  (`@/lib/api/sidebar-placement`, confirmed real signatures — read the
  file directly to verify), and `reparentWorkspace(projectId, repoId,
  wsId, ...)` (`@/lib/api/workspace`, confirmed real, for the fork-parent-
  lineage-changing case, mirroring `drop-plan.ts`'s `'reparent'` call
  kind) — verify each function's exact current parameter shape before
  calling it, don't assume from this list.
- Calls fire IN ORDER (a multi-row move's `order` values are relative to
  the list as it stands after the previous call lands — same reasoning
  `drop-plan.ts`'s own comment gives) — `await` each one sequentially, not
  `Promise.all`.
- A `'chat'`-kind row (no worktree) crossing repos: per
  `SIDEBAR_DROP_POLICY`'s already-verified rule, this is the one case a
  cross-repo drop is legal — investigate how a `chat` row's placement
  should actually be represented server-side (this may need a different
  API than `placeWorkspace`/`placeFolder`, since a chat row isn't
  necessarily a `Workspace` — read the backend spec / grep for how chat
  placement/parenting already works elsewhere in this plan, e.g. Task 29's
  `performCreateFolder`/row-actions.ts patterns, before assuming).
- Errors: `toast.error(...)` on failure, matching every other row-action's
  established pattern (Task 8/29's `performRenameWorkspaceBranch` etc.) —
  do not add a new error-handling convention.

**Interfaces:**
- Consumes: `SidebarRow`, `DropMode` (Task 4/20), `useSidebarStore`
  (`Repo`/`Workspace`/`Folder` real shapes), `placeWorkspace`/`placeFolder`
  (`@/lib/api/sidebar-placement`), `reparentWorkspace` (`@/lib/api/workspace`).
- Produces: `onDrop`'s real implementation, replacing the toast placeholder
  in `drop-actions.ts` — its own signature (`(subjects, target, mode) =>
  void`, per Task 21) is unchanged.

- [ ] **Step 1: Investigate thoroughly before writing any code**

Read `drop-plan.ts`'s full old logic (above). Read `SIDEBAR_DROP_POLICY`'s
real current implementation (Task 20) to confirm exactly which
subject/target/mode combinations can reach `onDrop` at all (the policy
already refuses everything else, so `onDrop`'s implementation only needs
to handle what the policy allows through). Read `drop-actions.ts`'s
current placeholder to see the real shape you're replacing. Read the real
`placeWorkspace`/`placeFolder`/`reparentWorkspace` signatures.

- [ ] **Step 2: Write failing tests** for at minimum: reordering a
  workspace among its current siblings (no reparent call fired, only a
  placement/order call); dropping a workspace `into` a folder (folder
  edge written, no lineage change); dropping a workspace onto a row under
  a DIFFERENT fork-parent (a `reparentWorkspace` call fires before the
  placement call, matching `drop-plan.ts`'s documented ordering
  rationale); a multi-row move firing its calls in order; a failed API
  call producing a `toast.error`, not a thrown/unhandled exception.

- [ ] **Step 3: Run, verify fail**

- [ ] **Step 4: Implement** per the findings above.

- [ ] **Step 5: Run, verify pass.** Project-wide `tsc --noEmit`
  before/after count (286 baseline as of Task 21).

- [ ] **Step 6: Commit**

```bash
git add web/src/components/sidebar/lib/drop-actions.ts web/src/__tests__/components/sidebar/lib/drop-actions.test.ts
git commit -m "feat(sidebar): onDrop gets real persistence — placing a row calls the real placement API"
```
