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

**Files:**
- Modify: whichever component hosts `useChatPresentation` today (confirmed:
  `use-chat-presentation.ts`, currently the chat⇄**terminal** split — this
  task generalizes it to chat⇄**editor view**, reusing its four constants
  verbatim: `SPLIT_SIDE_BY_SIDE_MIN_PX=780`, `SPLIT_MIN_HALF_PX=340`,
  `SPLIT_MIN_STACKED_PX=160`, `SPLIT_DEFAULT_SIZES=[45,55]`).
- Test: extend that hook's existing test file.

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
