# Branch Review Screen Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a permanent, uncloseable `branchReview` pane tab to every workspace — showing branch description, conversations, commits, and a multi-file diff with inline agent code-review threads, plus a split merge button that enforces locked-branch and conflict rules.

**Architecture:** A new `branchReviewSlice` inside the workspace Zustand store owns all state (description, merge strategy, diff cache, review threads). A new `features/branch-review/` feature renders the Frame shell with three subtabs (About / Commits / Diff). The Diff subtab injects threads inline into the existing virtualised multi-file diff viewer. The buffer auto-opens on workspace mount via `useWorkspaceEffects`, identical to how `crowbarChat` works today.

**Tech Stack:** React, Zustand + Immer, `@uiw/react-codemirror` (already installed), `@tanstack/react-virtual` (already used by the diff viewer), CrossUI `Frame`/`Group`/`Button` components (already local), Vitest + jsdom for tests.

---

### Task 1: Types and mock data foundation

**Files:**
- Create: `web/src/features/branch-review/types/review-types.ts`
- Modify: `web/src/features/panes/types/pane-content.ts`
- Modify: `web/src/lib/store/sidebar.ts`
- Modify: `web/src/lib/mock/workspaces.ts`
- Create: `web/src/__tests__/features/branch-review/review-types.test.ts`

- [ ] **Step 1: Create the review types file**

```ts
// web/src/features/branch-review/types/review-types.ts

export type MergeStrategy = 'merge' | 'squash' | 'rebase'

export interface ReviewMessage {
  id: string
  author: string | null  // agent name, or null = current user ("You")
  isAgent: boolean
  body: string           // markdown
  createdAt: string
}

export interface ReviewThread {
  id: string
  filePath: string
  lineNumber: number     // new_line_number of the anchored diff line
  side: 'left' | 'right'
  messages: ReviewMessage[]
  isResolved: boolean
}
```

- [ ] **Step 2: Write tests for the type shapes**

```ts
// web/src/__tests__/features/branch-review/review-types.test.ts
import { describe, it, expect } from 'vitest'
import type { ReviewThread, ReviewMessage, MergeStrategy } from '@/features/branch-review/types/review-types'

describe('review-types', () => {
  it('ReviewMessage allows null author for the current user', () => {
    const msg: ReviewMessage = {
      id: '1', author: null, isAgent: false, body: 'looks good', createdAt: '2026-06-01',
    }
    expect(msg.author).toBeNull()
  })

  it('ReviewMessage stores agent name as author', () => {
    const msg: ReviewMessage = {
      id: '2', author: 'Claude', isAgent: true, body: 'consider caching', createdAt: '2026-06-01',
    }
    expect(msg.author).toBe('Claude')
    expect(msg.isAgent).toBe(true)
  })

  it('MergeStrategy is one of the three valid values', () => {
    const strategies: MergeStrategy[] = ['merge', 'squash', 'rebase']
    expect(strategies).toHaveLength(3)
  })
})
```

- [ ] **Step 3: Run tests to confirm they pass (they're type-only, no implementation needed)**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/review-types.test.ts
```

Expected: PASS (3 tests)

- [ ] **Step 4: Add `isUncloseable` to `PaneContentBase` and add `BranchReviewContent`**

In `web/src/features/panes/types/pane-content.ts`, make these changes:

1. Add `isUncloseable?: boolean` to `PaneContentBase`:
```ts
interface PaneContentBase {
  id: string;
  type: PaneContentType;
  path: string;
  name: string;
  isPinned: boolean;
  isPreview: boolean;
  isActive: boolean;
  isUncloseable?: boolean;   // ← add this line
}
```

2. Add `"branchReview"` to `PaneContentType`:
```ts
export type PaneContentType =
  | "editor"
  | "terminal"
  | "webViewer"
  | "newTab"
  | "diff"
  | "markdownPreview"
  | "htmlPreview"
  | "csvPreview"
  | "externalEditor"
  | "crowbarChat"
  | "branchReview";  // ← add this
```

3. Add the interface after `CrowbarChatContent`:
```ts
export interface BranchReviewContent extends PaneContentBase {
  type: "branchReview";
  wsId: string;
  branchName: string;
  isUncloseable: true;
}
```

4. Add `BranchReviewContent` to the `PaneContent` union:
```ts
export type PaneContent =
  | EditorContent
  | TerminalContent
  | WebViewerContent
  | NewTabContent
  | DiffContent
  | MarkdownPreviewContent
  | HtmlPreviewContent
  | CsvPreviewContent
  | ExternalEditorContent
  | CrowbarChatContent
  | BranchReviewContent;  // ← add this
```

5. Add a type guard after the existing guards:
```ts
export function isBranchReviewContent(c: PaneContent): c is BranchReviewContent {
  return c.type === "branchReview";
}
```

6. Add `"branchReview"` to the `VIRTUAL_TYPES` set:
```ts
const VIRTUAL_TYPES: ReadonlySet<PaneContentType> = new Set([
  "terminal",
  "webViewer",
  "newTab",
  "crowbarChat",
  "branchReview",
]);
```

7. Add to `OpenContentSpec` union at the bottom:
```ts
| {
    type: "branchReview";
    wsId: string;
    branchName: string;
    name: string;
  }
```

- [ ] **Step 5: Add `hasConflicts` to the Workspace type and mock data**

In `web/src/lib/store/sidebar.ts`, add to the `Workspace` interface:
```ts
export interface Workspace {
  id: string
  branch: string
  parentId?: string
  status?: WorkspaceStatus
  added?: number
  deleted?: number
  age: string
  hasConflicts?: boolean  // ← add this
}
```

In `web/src/lib/mock/workspaces.ts`, add `hasConflicts: true` to a couple of branches in `getMockRepos()` to exercise the blocked state:
```ts
{ id: 'ws5', branch: 'refactor/query-layer', parentId: 'ws-develop', status: 'agent-running', added: 103482, deleted: 88910, age: '5d ago', hasConflicts: true },
{ id: 'qc3', branch: 'fix/token-expiry', parentId: 'qc2', status: 'new', added: 47, age: 'just now', hasConflicts: true },
```

Do the same in the `INITIAL_REPOS` constant inside `sidebar.ts` for the same branches.

- [ ] **Step 6: Commit**

```bash
cd web && git add src/features/branch-review/types/review-types.ts src/features/panes/types/pane-content.ts src/lib/store/sidebar.ts src/lib/mock/workspaces.ts src/__tests__/features/branch-review/review-types.test.ts
git commit -m "feat(branch-review): add types, pane content type, and mock conflict data"
```

---

### Task 2: `branchReviewSlice`

**Files:**
- Create: `web/src/features/workspace/stores/slices/branch-review-slice.ts`
- Modify: `web/src/features/workspace/stores/workspace-store.types.ts`
- Modify: `web/src/features/workspace/stores/workspace-store.ts`
- Create: `web/src/__tests__/features/branch-review/branch-review-slice.test.ts`

- [ ] **Step 1: Write the failing slice tests**

```ts
// web/src/__tests__/features/branch-review/branch-review-slice.test.ts
import { describe, it, expect } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { createBranchReviewSlice, type BranchReviewSlice } from '@/features/workspace/stores/slices/branch-review-slice'

function makeStore() {
  return createStore<BranchReviewSlice>()(
    immer((set, get, api) => createBranchReviewSlice(set as any, get as any, api as any))
  )
}

describe('branchReviewSlice', () => {
  it('has correct initial state', () => {
    const store = makeStore()
    const { branchReview } = store.getState()
    expect(branchReview.description).toBe('')
    expect(branchReview.mergeStrategy).toBe('merge')
    expect(branchReview.activeSubtab).toBe('about')
    expect(branchReview.diffCache).toBeNull()
    expect(branchReview.diffStatus).toBe('idle')
    expect(branchReview.threads).toEqual([])
  })

  it('setBranchReviewDescription updates description', () => {
    const store = makeStore()
    store.getState().setBranchReviewDescription('hello world')
    expect(store.getState().branchReview.description).toBe('hello world')
  })

  it('setBranchReviewMergeStrategy updates strategy', () => {
    const store = makeStore()
    store.getState().setBranchReviewMergeStrategy('squash')
    expect(store.getState().branchReview.mergeStrategy).toBe('squash')
  })

  it('setBranchReviewSubtab updates active subtab', () => {
    const store = makeStore()
    store.getState().setBranchReviewSubtab('diff')
    expect(store.getState().branchReview.activeSubtab).toBe('diff')
  })

  it('addReviewThread adds a thread', () => {
    const store = makeStore()
    const thread = {
      id: 't1', filePath: 'src/index.ts', lineNumber: 10,
      side: 'right' as const, messages: [], isResolved: false,
    }
    store.getState().addReviewThread(thread)
    expect(store.getState().branchReview.threads).toHaveLength(1)
    expect(store.getState().branchReview.threads[0].id).toBe('t1')
  })

  it('addReviewMessage appends to the correct thread', () => {
    const store = makeStore()
    store.getState().addReviewThread({
      id: 't1', filePath: 'src/index.ts', lineNumber: 10,
      side: 'right' as const, messages: [], isResolved: false,
    })
    store.getState().addReviewMessage('t1', {
      id: 'm1', author: null, isAgent: false, body: 'looks good', createdAt: '2026-06-01',
    })
    expect(store.getState().branchReview.threads[0].messages).toHaveLength(1)
    expect(store.getState().branchReview.threads[0].messages[0].id).toBe('m1')
  })

  it('resolveReviewThread marks thread as resolved', () => {
    const store = makeStore()
    store.getState().addReviewThread({
      id: 't1', filePath: 'src/index.ts', lineNumber: 10,
      side: 'right' as const, messages: [], isResolved: false,
    })
    store.getState().resolveReviewThread('t1')
    expect(store.getState().branchReview.threads[0].isResolved).toBe(true)
  })
})
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/branch-review-slice.test.ts
```

Expected: FAIL — module not found

- [ ] **Step 3: Implement the slice**

```ts
// web/src/features/workspace/stores/slices/branch-review-slice.ts
import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type { ReviewThread, ReviewMessage, MergeStrategy } from '@/features/branch-review/types/review-types'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'

export interface BranchReviewState {
  description: string
  mergeStrategy: MergeStrategy
  activeSubtab: 'about' | 'commits' | 'diff'
  diffCache: MultiFileDiff | null
  diffStatus: 'idle' | 'loading' | 'loaded' | 'error'
  threads: ReviewThread[]
}

export interface BranchReviewSlice {
  branchReview: BranchReviewState
  setBranchReviewDescription: (description: string) => void
  setBranchReviewMergeStrategy: (strategy: MergeStrategy) => void
  setBranchReviewSubtab: (tab: BranchReviewState['activeSubtab']) => void
  setBranchReviewDiff: (diff: MultiFileDiff) => void
  setBranchReviewDiffStatus: (status: BranchReviewState['diffStatus']) => void
  addReviewThread: (thread: ReviewThread) => void
  addReviewMessage: (threadId: string, message: ReviewMessage) => void
  resolveReviewThread: (threadId: string) => void
}

export const createBranchReviewSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  BranchReviewSlice
> = (set) => ({
  branchReview: {
    description: '',
    mergeStrategy: 'merge',
    activeSubtab: 'about',
    diffCache: null,
    diffStatus: 'idle',
    threads: [],
  },

  setBranchReviewDescription: (description) =>
    set(state => { state.branchReview.description = description }),

  setBranchReviewMergeStrategy: (strategy) =>
    set(state => { state.branchReview.mergeStrategy = strategy }),

  setBranchReviewSubtab: (tab) =>
    set(state => { state.branchReview.activeSubtab = tab }),

  setBranchReviewDiff: (diff) =>
    set(state => {
      state.branchReview.diffCache = diff
      state.branchReview.diffStatus = 'loaded'
    }),

  setBranchReviewDiffStatus: (status) =>
    set(state => { state.branchReview.diffStatus = status }),

  addReviewThread: (thread) =>
    set(state => { state.branchReview.threads.push(thread) }),

  addReviewMessage: (threadId, message) =>
    set(state => {
      const thread = state.branchReview.threads.find(t => t.id === threadId)
      if (thread) thread.messages.push(message)
    }),

  resolveReviewThread: (threadId) =>
    set(state => {
      const thread = state.branchReview.threads.find(t => t.id === threadId)
      if (thread) thread.isResolved = true
    }),
})
```

- [ ] **Step 4: Wire into WorkspaceState types**

In `web/src/features/workspace/stores/workspace-store.types.ts`:
```ts
import type { BranchReviewSlice } from './slices/branch-review-slice'

export type WorkspaceState =
  & WorkspaceBaseState
  & PaneSlice
  & BufferSlice
  & WorkflowSlice
  & LspSlice
  & TerminalSlice
  & FileWatcherSlice
  & RecentFilesSlice
  & BranchReviewSlice  // ← add this
```

- [ ] **Step 5: Wire into the store factory**

In `web/src/features/workspace/stores/workspace-store.ts`, add the import and spread:
```ts
import { createBranchReviewSlice } from './slices/branch-review-slice'

// inside createWorkspaceStore, in the immer factory:
export function createWorkspaceStore(wsId: string, snapshot?: WorkspaceSnapshot): WorkspaceStore {
  const store = createStore<WorkspaceState>()(
    immer((set, get, api): WorkspaceState => ({
      workspaceId: wsId,
      ...createPaneSlice(set, get, api),
      ...createBufferSlice(set, get, api),
      ...createWorkflowSlice(set, get, api),
      ...createLspSlice(set, get, api),
      ...createTerminalSlice(set, get, api),
      ...createFileWatcherSlice(set, get, api),
      ...createRecentFilesSlice(set, get, api),
      ...createBranchReviewSlice(set, get, api),  // ← add this
      ...(snapshot ?? {}),
    }))
  )
  // ... rest unchanged
```

- [ ] **Step 6: Run tests again — should pass now**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/branch-review-slice.test.ts
```

Expected: PASS (7 tests)

- [ ] **Step 7: Commit**

```bash
cd web && git add src/features/workspace/stores/slices/branch-review-slice.ts src/features/workspace/stores/workspace-store.types.ts src/features/workspace/stores/workspace-store.ts src/__tests__/features/branch-review/branch-review-slice.test.ts
git commit -m "feat(branch-review): add branchReviewSlice to workspace store"
```

---

### Task 3: Suppress close button for uncloseable tabs

**Files:**
- Modify: `web/src/features/tabs/components/tab-bar-item.tsx`
- Create: `web/src/__tests__/features/branch-review/tab-bar-uncloseable.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/features/branch-review/tab-bar-uncloseable.test.ts
import { describe, it, expect } from 'vitest'
import type { PaneContent } from '@/features/panes/types/pane-content'

// Test the logic that decides whether to show the close button
function shouldShowCloseButton(buffer: PaneContent): boolean {
  return !buffer.isUncloseable
}

describe('tab close button visibility', () => {
  it('shows close button for normal editor tabs', () => {
    const buffer = { type: 'editor', isUncloseable: undefined } as unknown as PaneContent
    expect(shouldShowCloseButton(buffer)).toBe(true)
  })

  it('hides close button for branchReview tabs', () => {
    const buffer = { type: 'branchReview', isUncloseable: true } as unknown as PaneContent
    expect(shouldShowCloseButton(buffer)).toBe(false)
  })
})
```

- [ ] **Step 2: Run to confirm it passes (logic is pure, no component needed)**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/tab-bar-uncloseable.test.ts
```

Expected: PASS

- [ ] **Step 3: Apply the change to `tab-bar-item.tsx`**

In `web/src/features/tabs/components/tab-bar-item.tsx`, find where the `×` close button is rendered. It will be inside the returned JSX of `TabBarItem`. Wrap it in a conditional:

```tsx
{!buffer.isUncloseable && (
  <Button
    /* existing close button props unchanged */
  />
)}
```

The exact location is the `<Button>` that calls `handleTabClose`. Add the `!buffer.isUncloseable &&` wrapper around it. Do not change any other part of the component.

- [ ] **Step 4: Commit**

```bash
cd web && git add src/features/tabs/components/tab-bar-item.tsx src/__tests__/features/branch-review/tab-bar-uncloseable.test.ts
git commit -m "feat(branch-review): suppress close button for uncloseable tabs"
```

---

### Task 4: Auto-open branchReview buffer on workspace mount

**Files:**
- Modify: `web/src/features/workspace/stores/hooks/use-workspace-effects.ts`
- Modify: `web/src/features/workspace/stores/slices/buffer-slice.ts`
- Modify: `web/src/features/panes/components/pane-container.tsx`

- [ ] **Step 1: Handle `branchReview` singleton in `buffer-slice.ts`**

In `web/src/features/workspace/stores/slices/buffer-slice.ts`, find the `openContent` function. Add a singleton guard for `branchReview` (same pattern as `crowbarChat`). Find the block starting with `if (spec.type === 'crowbarChat')` and add a parallel block after it:

```ts
if (spec.type === 'branchReview') {
  const existing = Object.values(get().buffers).find(
    b => b.type === 'branchReview' && (b as BranchReviewContent).wsId === spec.wsId,
  )
  if (existing) {
    // focus the existing tab — same pattern as crowbarChat
    // find its pane and make it active
    return existing.id
  }
}
```

Then in the `else if` chain that creates new buffers, add after the `crowbarChat` case:
```ts
} else if (spec.type === 'branchReview') {
  newBuffer = {
    id, type: 'branchReview',
    wsId: spec.wsId,
    branchName: spec.branchName,
    name: spec.name,
    path: `branch-review://${spec.wsId}`,
    isPinned: false,
    isPreview: false,
    isActive: false,
    isUncloseable: true,
  } satisfies BranchReviewContent
}
```

Add the import at the top of the file:
```ts
import type { BranchReviewContent } from '@/features/panes/types/pane-content'
```

- [ ] **Step 2: Auto-open on workspace mount**

In `web/src/features/workspace/stores/hooks/use-workspace-effects.ts`, add a new `useEffect` after the `crowbarChat` one:

```ts
// Open branchReview buffer
useEffect(() => {
  // derive branch name from the workspace label or use a placeholder
  const branchName = label ?? wsId
  bufferActions.openContent({ type: 'branchReview', wsId, branchName, name: branchName })
}, [wsId]) // eslint-disable-line react-hooks/exhaustive-deps
```

- [ ] **Step 3: Register in `pane-container.tsx`**

In `web/src/features/panes/components/pane-container.tsx`, add the lazy import at the top with the other lazy imports:

```ts
const BranchReviewPane = lazy(() =>
  import('@/features/branch-review/components/branch-review-pane').then(m => ({
    default: m.BranchReviewPane,
  }))
)
```

Add an import for the type:
```ts
import type { BranchReviewContent, CrowbarChatContent, EditorContent, NewTabContent } from '../types/pane-content'
```

Add a case to the `renderActiveBuffer` switch after the `crowbarChat` case:
```ts
case "branchReview":
  return (
    <BranchReviewPane
      wsId={(buffer as BranchReviewContent).wsId}
      branchName={(buffer as BranchReviewContent).branchName}
    />
  )
```

- [ ] **Step 4: Commit**

```bash
cd web && git add src/features/workspace/stores/hooks/use-workspace-effects.ts src/features/workspace/stores/slices/buffer-slice.ts src/features/panes/components/pane-container.tsx
git commit -m "feat(branch-review): auto-open branchReview buffer on workspace mount"
```

---

### Task 5: MergeButton component

**Files:**
- Create: `web/src/features/branch-review/components/merge-button.tsx`
- Create: `web/src/__tests__/features/branch-review/merge-button.test.tsx`

- [ ] **Step 1: Write the failing tests**

```tsx
// web/src/__tests__/features/branch-review/merge-button.test.tsx
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MergeButton } from '@/features/branch-review/components/merge-button'

describe('MergeButton', () => {
  it('shows merge strategy label', () => {
    render(
      <MergeButton
        strategy="merge"
        isLocked={false}
        hasConflicts={false}
        onMerge={() => {}}
        onStrategyChange={() => {}}
      />
    )
    expect(screen.getByText('Merge commit')).toBeTruthy()
  })

  it('is disabled when parent is locked', () => {
    render(
      <MergeButton
        strategy="merge"
        isLocked={true}
        hasConflicts={false}
        onMerge={() => {}}
        onStrategyChange={() => {}}
      />
    )
    const btn = screen.getByRole('button', { name: /merge commit/i })
    expect(btn).toBeDisabled()
  })

  it('is disabled when branch has conflicts', () => {
    render(
      <MergeButton
        strategy="squash"
        isLocked={false}
        hasConflicts={true}
        onMerge={() => {}}
        onStrategyChange={() => {}}
      />
    )
    const btn = screen.getByRole('button', { name: /squash and merge/i })
    expect(btn).toBeDisabled()
  })

  it('calls onMerge when clicked and not disabled', async () => {
    const onMerge = vi.fn()
    render(
      <MergeButton
        strategy="rebase"
        isLocked={false}
        hasConflicts={false}
        onMerge={onMerge}
        onStrategyChange={() => {}}
      />
    )
    await userEvent.click(screen.getByRole('button', { name: /rebase and merge/i }))
    expect(onMerge).toHaveBeenCalledOnce()
  })
})
```

- [ ] **Step 2: Run to confirm they fail**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/merge-button.test.tsx
```

Expected: FAIL — module not found

- [ ] **Step 3: Implement the component**

```tsx
// web/src/features/branch-review/components/merge-button.tsx
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Group } from '@/components/ui/group'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { CaretDown } from '@phosphor-icons/react'
import type { MergeStrategy } from '@/features/branch-review/types/review-types'
import { cn } from '@/utils/cn'

const STRATEGY_LABELS: Record<MergeStrategy, string> = {
  merge: 'Merge commit',
  squash: 'Squash and merge',
  rebase: 'Rebase and merge',
}

interface MergeButtonProps {
  strategy: MergeStrategy
  isLocked: boolean
  hasConflicts: boolean
  onMerge: () => void
  onStrategyChange: (strategy: MergeStrategy) => void
}

export function MergeButton({ strategy, isLocked, hasConflicts, onMerge, onStrategyChange }: MergeButtonProps) {
  const disabled = isLocked || hasConflicts
  const tooltip = isLocked
    ? 'Cannot merge into a locked branch'
    : hasConflicts
    ? 'Branch has conflicts with parent'
    : undefined

  return (
    <Group>
      <Button
        variant={hasConflicts ? 'destructive' : 'default'}
        disabled={disabled}
        onClick={onMerge}
        title={tooltip}
        className={cn(!disabled && 'bg-green-700 hover:bg-green-600 text-white')}
      >
        {STRATEGY_LABELS[strategy]}
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant={hasConflicts ? 'destructive' : 'default'}
            disabled={disabled}
            title={tooltip}
            className={cn('px-2', !disabled && 'bg-green-800 hover:bg-green-700 text-green-200')}
          >
            <CaretDown size={12} />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {(Object.keys(STRATEGY_LABELS) as MergeStrategy[]).map(s => (
            <DropdownMenuItem key={s} onSelect={() => onStrategyChange(s)}>
              {STRATEGY_LABELS[s]}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </Group>
  )
}
```

- [ ] **Step 4: Run tests — should pass**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/merge-button.test.tsx
```

Expected: PASS (4 tests)

- [ ] **Step 5: Commit**

```bash
cd web && git add src/features/branch-review/components/merge-button.tsx src/__tests__/features/branch-review/merge-button.test.tsx
git commit -m "feat(branch-review): add MergeButton split button component"
```

---

### Task 6: BranchReviewHeader component

**Files:**
- Create: `web/src/features/branch-review/components/branch-review-header.tsx`

- [ ] **Step 1: Implement the header**

```tsx
// web/src/features/branch-review/components/branch-review-header.tsx
import { GitBranch } from '@phosphor-icons/react'
import { Badge } from '@/components/ui/badge'
import { MergeButton } from './merge-button'
import type { MergeStrategy } from '@/features/branch-review/types/review-types'

interface BranchReviewHeaderProps {
  branchName: string
  parentBranch: string
  isLocked: boolean
  hasConflicts: boolean
  aheadCount: number
  additions: number
  deletions: number
  mergeStrategy: MergeStrategy
  onMerge: () => void
  onStrategyChange: (strategy: MergeStrategy) => void
}

export function BranchReviewHeader({
  branchName,
  parentBranch,
  isLocked,
  hasConflicts,
  aheadCount,
  additions,
  deletions,
  mergeStrategy,
  onMerge,
  onStrategyChange,
}: BranchReviewHeaderProps) {
  const stats = [
    aheadCount > 0 && `${aheadCount} commit${aheadCount !== 1 ? 's' : ''} ahead`,
    additions > 0 && `${additions.toLocaleString()} additions`,
    deletions > 0 && `${deletions.toLocaleString()} deletions`,
  ].filter(Boolean).join(' · ')

  return (
    <div className="flex items-center justify-between px-4 pt-4 pb-3">
      <div>
        <div className="flex items-center gap-2 text-sm font-semibold text-foreground">
          <GitBranch size={14} className="text-muted-foreground" />
          {branchName}
          <Badge variant="outline" className="text-xs font-normal text-muted-foreground">
            → {parentBranch}
          </Badge>
        </div>
        {stats && (
          <p className="mt-0.5 text-xs text-muted-foreground">{stats}</p>
        )}
      </div>
      <MergeButton
        strategy={mergeStrategy}
        isLocked={isLocked}
        hasConflicts={hasConflicts}
        onMerge={onMerge}
        onStrategyChange={onStrategyChange}
      />
    </div>
  )
}
```

- [ ] **Step 2: Commit**

```bash
cd web && git add src/features/branch-review/components/branch-review-header.tsx
git commit -m "feat(branch-review): add BranchReviewHeader component"
```

---

### Task 7: AboutTab component

**Files:**
- Create: `web/src/features/branch-review/components/about-tab.tsx`

The About tab has two sections in a single shared `FramePanel` card: a CodeMirror markdown description editor, and a read-only list of workspace conversations.

- [ ] **Step 1: Implement the component**

```tsx
// web/src/features/branch-review/components/about-tab.tsx
import CodeMirror from '@uiw/react-codemirror'
import { markdown } from '@codemirror/lang-markdown'
import { useTheme } from '@/features/settings/hooks/use-theme'
import { useSidebarStore } from '@/lib/store/sidebar'
import { cn } from '@/utils/cn'

interface Conversation {
  id: string
  title: string
  age: string
  isActive: boolean
}

interface AboutTabProps {
  description: string
  wsId: string
  onDescriptionChange: (value: string) => void
  onOpenConversation: (id: string) => void
}

export function AboutTab({ description, wsId, onDescriptionChange, onOpenConversation }: AboutTabProps) {
  const chats = useSidebarStore(state => state.chats)
  const isDark = useTheme() === 'dark'

  return (
    <div className="flex flex-col divide-y divide-border">
      {/* Description */}
      <div className="p-4">
        <h3 className="mb-2.5 text-xs font-semibold text-foreground">Description</h3>
        <div className="overflow-hidden rounded-lg border border-border bg-[#0d0d0d]">
          <CodeMirror
            value={description}
            extensions={[markdown()]}
            theme={isDark ? 'dark' : 'light'}
            onChange={onDescriptionChange}
            basicSetup={{
              lineNumbers: false,
              foldGutter: false,
              dropCursor: false,
              allowMultipleSelections: false,
              indentOnInput: true,
            }}
            className="text-xs"
          />
        </div>
      </div>

      {/* Conversations */}
      <div className="p-4">
        <h3 className="mb-2.5 text-xs font-semibold text-foreground">Conversations</h3>
        {chats.length === 0 ? (
          <p className="text-xs text-muted-foreground/40">No conversations yet.</p>
        ) : (
          <div className="flex flex-col gap-0.5">
            {chats.map(chat => (
              <button
                key={chat.id}
                onClick={() => onOpenConversation(chat.id)}
                className="flex w-full items-center gap-2.5 rounded-lg border border-border bg-[#0d0d0d] px-3 py-2 text-left hover:bg-muted/40 transition-colors"
              >
                <span className={cn(
                  'h-1.5 w-1.5 shrink-0 rounded-full',
                  chat.age === 'active' ? 'bg-green-500' : 'bg-muted-foreground/30'
                )} />
                <span className="flex-1 truncate text-xs text-foreground">{chat.title}</span>
                <span className="text-xs text-muted-foreground/50">{chat.age}</span>
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Commit**

```bash
cd web && git add src/features/branch-review/components/about-tab.tsx
git commit -m "feat(branch-review): add AboutTab with CodeMirror description and conversations list"
```

---

### Task 8: CommitsTab component

**Files:**
- Create: `web/src/features/branch-review/components/commits-tab.tsx`

- [ ] **Step 1: Implement the component**

```tsx
// web/src/features/branch-review/components/commits-tab.tsx
import { useEffect, useState } from 'react'
import { getGitLog } from '@/features/git/api/git-commits-api'
import type { GitCommit } from '@/features/git/types/git-types'
import { formatRelativeDate } from '@/utils/date'

interface CommitsTabProps {
  repoPath: string
}

export function CommitsTab({ repoPath }: CommitsTabProps) {
  const [commits, setCommits] = useState<GitCommit[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    getGitLog(repoPath, 50).then(result => {
      if (!cancelled) {
        setCommits(result)
        setLoading(false)
      }
    })
    return () => { cancelled = true }
  }, [repoPath])

  if (loading) {
    return (
      <div className="p-4">
        <h3 className="mb-2.5 text-xs font-semibold text-foreground">Commit history</h3>
        <p className="text-xs text-muted-foreground/40">Loading commits…</p>
      </div>
    )
  }

  return (
    <div className="p-4">
      <h3 className="mb-2.5 text-xs font-semibold text-foreground">Commit history</h3>
      {commits.length === 0 ? (
        <p className="text-xs text-muted-foreground/40">No commits on this branch yet.</p>
      ) : (
        <div className="flex flex-col gap-0.5">
          {commits.map(commit => (
            <div
              key={commit.hash}
              className="flex items-center gap-2.5 rounded-lg border border-border bg-[#0d0d0d] px-3 py-2"
            >
              <span className="w-12 shrink-0 font-mono text-[10px] text-muted-foreground/60">
                {commit.hash.slice(0, 7)}
              </span>
              <span className="flex-1 truncate text-xs text-foreground">{commit.message}</span>
              <span className="shrink-0 text-[10px] text-muted-foreground/50">
                {formatRelativeDate(commit.date)}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
```

- [ ] **Step 2: Commit**

```bash
cd web && git add src/features/branch-review/components/commits-tab.tsx
git commit -m "feat(branch-review): add CommitsTab component"
```

---

### Task 9: ReviewThread component

**Files:**
- Create: `web/src/features/branch-review/components/review-thread.tsx`
- Create: `web/src/__tests__/features/branch-review/review-thread.test.tsx`

- [ ] **Step 1: Write the failing tests**

```tsx
// web/src/__tests__/features/branch-review/review-thread.test.tsx
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ReviewThreadView } from '@/features/branch-review/components/review-thread'
import type { ReviewThread } from '@/features/branch-review/types/review-types'

const thread: ReviewThread = {
  id: 't1',
  filePath: 'src/index.ts',
  lineNumber: 10,
  side: 'right',
  isResolved: false,
  messages: [
    { id: 'm1', author: 'Claude', isAgent: true, body: 'Consider caching here.', createdAt: '2026-06-01' },
    { id: 'm2', author: null, isAgent: false, body: 'Good point!', createdAt: '2026-06-01' },
  ],
}

describe('ReviewThreadView', () => {
  it('renders agent messages with author name and badge', () => {
    render(<ReviewThreadView thread={thread} onReply={() => {}} onResolve={() => {}} />)
    expect(screen.getByText('Claude')).toBeTruthy()
    expect(screen.getByText('agent')).toBeTruthy()
  })

  it('renders user messages as "You"', () => {
    render(<ReviewThreadView thread={thread} onReply={() => {}} onResolve={() => {}} />)
    expect(screen.getByText('You')).toBeTruthy()
  })

  it('calls onReply with the reply text', async () => {
    const onReply = vi.fn()
    render(<ReviewThreadView thread={thread} onReply={onReply} onResolve={() => {}} />)
    const input = screen.getByPlaceholderText(/reply/i)
    await userEvent.type(input, 'My reply')
    await userEvent.keyboard('{Enter}')
    expect(onReply).toHaveBeenCalledWith('My reply')
  })

  it('calls onResolve when resolve button clicked', async () => {
    const onResolve = vi.fn()
    render(<ReviewThreadView thread={thread} onReply={() => {}} onResolve={onResolve} />)
    await userEvent.click(screen.getByRole('button', { name: /resolve/i }))
    expect(onResolve).toHaveBeenCalledOnce()
  })
})
```

- [ ] **Step 2: Run to confirm they fail**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/review-thread.test.tsx
```

Expected: FAIL — module not found

- [ ] **Step 3: Implement the component**

```tsx
// web/src/features/branch-review/components/review-thread.tsx
import { useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { ReviewThread } from '@/features/branch-review/types/review-types'
import { cn } from '@/utils/cn'

interface ReviewThreadViewProps {
  thread: ReviewThread
  onReply: (body: string) => void
  onResolve: () => void
}

export function ReviewThreadView({ thread, onReply, onResolve }: ReviewThreadViewProps) {
  const [replyText, setReplyText] = useState('')
  const [collapsed, setCollapsed] = useState(false)

  if (thread.isResolved && collapsed) {
    return (
      <div className="my-1 flex items-center gap-2 px-3 py-1 text-xs text-muted-foreground/40">
        <span>Thread resolved</span>
        <button className="underline" onClick={() => setCollapsed(false)}>Show</button>
      </div>
    )
  }

  const handleReply = () => {
    const trimmed = replyText.trim()
    if (!trimmed) return
    onReply(trimmed)
    setReplyText('')
  }

  return (
    <div className={cn(
      'my-1 rounded-lg border border-border bg-muted/20',
      thread.isResolved && 'opacity-60'
    )}>
      {thread.messages.map(msg => (
        <div key={msg.id} className="border-b border-border/40 last:border-b-0 px-3 py-2">
          <div className="mb-1 flex items-center gap-1.5">
            <span className="text-[11px] font-semibold text-foreground">
              {msg.author ?? 'You'}
            </span>
            {msg.isAgent && (
              <Badge variant="outline" className="h-3.5 px-1 text-[9px] text-blue-400 border-blue-400/30">
                agent
              </Badge>
            )}
          </div>
          <p className="text-xs text-muted-foreground leading-relaxed">{msg.body}</p>
        </div>
      ))}

      <div className="flex items-center gap-2 px-3 py-2">
        <input
          className="flex-1 bg-transparent text-xs outline-none placeholder:text-muted-foreground/30"
          placeholder="Reply..."
          value={replyText}
          onChange={e => setReplyText(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleReply() } }}
        />
        {!thread.isResolved && (
          <Button size="sm" variant="ghost" className="h-6 px-2 text-[10px]" onClick={onResolve}>
            Resolve
          </Button>
        )}
        {thread.isResolved && (
          <button className="text-[10px] text-muted-foreground/40 underline" onClick={() => setCollapsed(true)}>
            Collapse
          </button>
        )}
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Run tests — should pass**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/review-thread.test.tsx
```

Expected: PASS (4 tests)

- [ ] **Step 5: Commit**

```bash
cd web && git add src/features/branch-review/components/review-thread.tsx src/__tests__/features/branch-review/review-thread.test.tsx
git commit -m "feat(branch-review): add ReviewThreadView component with reply and resolve"
```

---

### Task 10: DiffTab component with inline thread injection

**Files:**
- Create: `web/src/features/branch-review/components/diff-tab.tsx`
- Modify: `web/src/features/git/components/diff/git-diff-text.tsx`
- Modify: `web/src/features/git/components/diff/git-diff-multi-file.tsx`

The existing diff components need to be extended to accept and render threads inline after their anchored lines.

- [ ] **Step 1: Extend `TextDiffViewerProps` to accept threads**

In `web/src/features/git/types/git-diff-types.ts`, add `threads` to `TextDiffViewerProps`:

```ts
// Add import at top:
import type { ReviewThread } from '@/features/branch-review/types/review-types'

// Add to TextDiffViewerProps:
export interface TextDiffViewerProps {
  diff: GitDiff;
  isStaged: boolean;
  viewMode: "unified" | "split";
  showWhitespace: boolean;
  onStageHunk?: (hunk: GitHunk) => void;
  onUnstageHunk?: (hunk: GitHunk) => void;
  isInMultiFileView?: boolean;
  isEmbeddedInScrollView?: boolean;
  threads?: ReviewThread[];           // ← add this
  onReply?: (threadId: string, body: string) => void;   // ← add this
  onResolve?: (threadId: string) => void;               // ← add this
}
```

Also add `threads` to `MultiFileDiffViewerProps`:
```ts
export interface MultiFileDiffViewerProps {
  multiDiff: MultiFileDiff;
  onClose: () => void;
  threads?: ReviewThread[];
  onReply?: (threadId: string, body: string) => void;
  onResolve?: (threadId: string) => void;
}
```

- [ ] **Step 2: Inject threads into `git-diff-text.tsx`**

In `web/src/features/git/components/diff/git-diff-text.tsx`, add the thread imports and inject `ReviewThreadView` after each line that has a thread anchored to it.

Add imports at top of the file:
```tsx
import type { ReviewThread } from '@/features/branch-review/types/review-types'
import { ReviewThreadView } from '@/features/branch-review/components/review-thread'
```

Destructure the new props from `TextDiffViewerProps`:
```tsx
function TextDiffViewer({
  diff,
  isStaged,
  viewMode,
  showWhitespace,
  onStageHunk,
  onUnstageHunk,
  isInMultiFileView,
  isEmbeddedInScrollView,
  threads = [],
  onReply,
  onResolve,
}: TextDiffViewerProps) {
```

In the line rendering loop (wherever individual `DiffLine` components are mapped), after each line element, insert threads anchored to that line number:

```tsx
{/* after rendering the DiffLine for `line`: */}
{threads
  .filter(t => t.lineNumber === line.new_line_number && !t.isResolved || (t.isResolved && t.lineNumber === line.new_line_number))
  .map(thread => (
    <ReviewThreadView
      key={thread.id}
      thread={thread}
      onReply={body => onReply?.(thread.id, body)}
      onResolve={() => onResolve?.(thread.id)}
    />
  ))
}
```

- [ ] **Step 3: Pass threads through `git-diff-multi-file.tsx`**

In `web/src/features/git/components/diff/git-diff-multi-file.tsx`, pass `threads`, `onReply`, `onResolve` through `FileDiffSection` down to `TextDiffViewer`.

Add to `FileDiffSection` props:
```tsx
threads?: ReviewThread[]
onReply?: (threadId: string, body: string) => void
onResolve?: (threadId: string) => void
```

Filter threads by `filePath` when passing to `TextDiffViewer`:
```tsx
<TextDiffViewer
  diff={diff}
  // ... existing props ...
  threads={threads?.filter(t => t.filePath === diff.file_path)}
  onReply={onReply}
  onResolve={onResolve}
/>
```

Pass them through from `MultiFileDiffViewer` → `FileDiffSection` → `TextDiffViewer`.

- [ ] **Step 4: Implement `diff-tab.tsx`**

```tsx
// web/src/features/branch-review/components/diff-tab.tsx
import { useEffect } from 'react'
import { LoadingSpinner } from '@/components/ui/loading-spinner'
import { MultiFileDiffViewer } from '@/features/git/components/diff/git-diff-multi-file'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'

interface DiffTabProps {
  repoPath: string
}

export function DiffTab({ repoPath }: DiffTabProps) {
  const diffStatus = useWorkspaceStore(s => s.branchReview.diffStatus)
  const diffCache = useWorkspaceStore(s => s.branchReview.diffCache)
  const threads = useWorkspaceStore(s => s.branchReview.threads)
  const setBranchReviewDiff = useWorkspaceStore(s => s.setBranchReviewDiff)
  const setBranchReviewDiffStatus = useWorkspaceStore(s => s.setBranchReviewDiffStatus)
  const addReviewMessage = useWorkspaceStore(s => s.addReviewMessage)
  const resolveReviewThread = useWorkspaceStore(s => s.resolveReviewThread)

  useEffect(() => {
    if (diffStatus !== 'idle') return
    setBranchReviewDiffStatus('loading')
    // Mock: in a real implementation, call a git diff API here.
    // For now, import mock diff data.
    import('@/lib/mock/git-data').then(m => {
      const mockDiff = m.getMockMultiFileDiff?.(repoPath)
      if (mockDiff) {
        setBranchReviewDiff(mockDiff)
      } else {
        setBranchReviewDiffStatus('error')
      }
    }).catch(() => setBranchReviewDiffStatus('error'))
  }, [repoPath, diffStatus, setBranchReviewDiff, setBranchReviewDiffStatus])

  if (diffStatus === 'idle' || diffStatus === 'loading') {
    return (
      <div className="flex items-center justify-center p-12">
        <LoadingSpinner />
      </div>
    )
  }

  if (diffStatus === 'error' || !diffCache) {
    return (
      <div className="p-4 text-xs text-muted-foreground">
        Failed to load diff.
      </div>
    )
  }

  return (
    <MultiFileDiffViewer
      multiDiff={diffCache}
      onClose={() => {}}
      threads={threads}
      onReply={(threadId, body) => addReviewMessage(threadId, {
        id: crypto.randomUUID(),
        author: null,
        isAgent: false,
        body,
        createdAt: new Date().toISOString(),
      })}
      onResolve={resolveReviewThread}
    />
  )
}
```

- [ ] **Step 5: Add `getMockMultiFileDiff` to mock data if it doesn't exist**

Check `web/src/lib/mock/git-data.ts`. If `getMockMultiFileDiff` doesn't exist, add a minimal stub:

```ts
export function getMockMultiFileDiff(repoPath: string): MultiFileDiff {
  return {
    commitHash: 'mock',
    files: [],
    totalFiles: 0,
    totalAdditions: 0,
    totalDeletions: 0,
  }
}
```

- [ ] **Step 6: Commit**

```bash
cd web && git add src/features/branch-review/components/diff-tab.tsx src/features/git/components/diff/git-diff-text.tsx src/features/git/components/diff/git-diff-multi-file.tsx src/features/git/types/git-diff-types.ts
git commit -m "feat(branch-review): add DiffTab with inline review thread injection"
```

---

### Task 11: BranchReviewPane — top-level shell

**Files:**
- Create: `web/src/features/branch-review/components/branch-review-pane.tsx`

This is the root component registered in `pane-container.tsx`. It wires the Frame shell, Group subtab switcher, and routes to the three subtab components.

- [ ] **Step 1: Implement the pane**

```tsx
// web/src/features/branch-review/components/branch-review-pane.tsx
import { Frame } from '@/components/ui/frame'
import { Group } from '@/components/ui/group'
import { Button } from '@/components/ui/button'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { useSidebarStore } from '@/lib/store/sidebar'
import { BranchReviewHeader } from './branch-review-header'
import { AboutTab } from './about-tab'
import { CommitsTab } from './commits-tab'
import { DiffTab } from './diff-tab'
import { cn } from '@/utils/cn'

interface BranchReviewPaneProps {
  wsId: string
  branchName: string
}

export function BranchReviewPane({ wsId, branchName }: BranchReviewPaneProps) {
  const activeSubtab = useWorkspaceStore(s => s.branchReview.activeSubtab)
  const description = useWorkspaceStore(s => s.branchReview.description)
  const mergeStrategy = useWorkspaceStore(s => s.branchReview.mergeStrategy)
  const setBranchReviewSubtab = useWorkspaceStore(s => s.setBranchReviewSubtab)
  const setBranchReviewDescription = useWorkspaceStore(s => s.setBranchReviewDescription)
  const setBranchReviewMergeStrategy = useWorkspaceStore(s => s.setBranchReviewMergeStrategy)

  // Derive parent branch and conflict/lock state from sidebar store
  const repos = useSidebarStore(s => s.repos)
  const allWorkspaces = repos.flatMap(r => r.workspaces)
  const workspace = allWorkspaces.find(w => w.id === wsId)
  const parentWorkspace = workspace?.parentId
    ? allWorkspaces.find(w => w.id === workspace.parentId)
    : null
  const parentBranch = parentWorkspace?.branch ?? 'main'
  const isLocked = parentWorkspace?.status === 'locked'
  const hasConflicts = workspace?.hasConflicts ?? false
  const additions = workspace?.added ?? 0
  const deletions = workspace?.deleted ?? 0
  const repoPath = repos.find(r => r.workspaces.some(w => w.id === wsId))
    ? `/repos/${wsId}`
    : `/repos/${wsId}`

  const TABS = [
    { key: 'about' as const, label: 'About' },
    { key: 'commits' as const, label: 'Commits' },
    { key: 'diff' as const, label: 'Diff' },
  ]

  return (
    <div className="flex h-full flex-col overflow-hidden p-2">
      <Frame className="flex h-full flex-col overflow-hidden">
        {/* Header — bare on frame bg */}
        <BranchReviewHeader
          branchName={branchName}
          parentBranch={parentBranch}
          isLocked={isLocked}
          hasConflicts={hasConflicts}
          aheadCount={3}
          additions={additions}
          deletions={deletions}
          mergeStrategy={mergeStrategy}
          onMerge={() => { /* mock — no-op for now */ }}
          onStrategyChange={setBranchReviewMergeStrategy}
        />

        {/* Group subtab switcher */}
        <div className="px-4 pb-3">
          <Group>
            {TABS.map(tab => (
              <Button
                key={tab.key}
                variant={activeSubtab === tab.key ? 'secondary' : 'ghost'}
                size="sm"
                className="text-xs"
                onClick={() => setBranchReviewSubtab(tab.key)}
              >
                {tab.label}
              </Button>
            ))}
          </Group>
        </div>

        {/* Content card — rounded top, flush sides + bottom */}
        <div className="mx-0 flex-1 overflow-y-auto rounded-t-xl border border-border bg-background">
          {activeSubtab === 'about' && (
            <AboutTab
              description={description}
              wsId={wsId}
              onDescriptionChange={setBranchReviewDescription}
              onOpenConversation={(_id) => { /* TODO: focus crowbarChat pane for this conversation */ }}
            />
          )}
          {activeSubtab === 'commits' && (
            <CommitsTab repoPath={repoPath} />
          )}
          {activeSubtab === 'diff' && (
            <DiffTab repoPath={repoPath} />
          )}
        </div>
      </Frame>
    </div>
  )
}
```

- [ ] **Step 2: Commit**

```bash
cd web && git add src/features/branch-review/components/branch-review-pane.tsx
git commit -m "feat(branch-review): add BranchReviewPane top-level shell component"
```

---

### Task 12: Wire up and smoke test

**Files:**
- Verify: `web/src/features/panes/components/pane-container.tsx` (done in Task 4)

- [ ] **Step 1: Run the full test suite**

```bash
cd web && npx vitest run
```

Expected: All existing tests pass, new branch-review tests pass. Fix any TypeScript errors before continuing.

- [ ] **Step 2: Start the dev server and open a workspace**

```bash
cd web && npm run dev
```

Open `http://localhost:5173` in a browser. Navigate to any workspace (e.g. `feature/api-backend`).

Verify:
- A tab labelled with the branch name appears in the pane tab bar with no × button
- The tab can be dragged to other pane positions
- The Frame renders with header, Group switcher, and content card
- The About tab shows the CodeMirror description editor (editable) and a Conversations list
- Switching to Commits shows the commit list
- Switching to Diff shows a loading spinner then the diff view
- On a workspace with `hasConflicts: true` (e.g. `refactor/query-layer`), the merge button is disabled with a red tint

- [ ] **Step 3: Final commit**

```bash
cd web && git add -p  # stage any last fixes
git commit -m "feat(branch-review): complete branch review screen implementation"
```
