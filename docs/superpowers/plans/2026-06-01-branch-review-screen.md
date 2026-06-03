# Branch Review Screen Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a permanent, uncloseable `branchReview` pane tab to every workspace — showing branch description, conversations, commits, and a bespoke diff viewer with inline agent code-review threads, plus a split merge button that enforces locked-branch and conflict rules.

**Architecture:** A new `branchReviewSlice` inside the workspace Zustand store owns all state. A new `features/branch-review/` feature renders the Frame shell with three subtabs (About / Commits / Diff) using CrossUI `Tabs`. The Diff subtab uses a bespoke hybrid-virtualised diff viewer (file-level always; line-level for files > 200 lines). `description`, `mergeStrategy`, `threads`, and `activeSubtab` are persisted to IndexDB via a new `branch-review` object store (db version 4). The buffer auto-opens on workspace mount, identical to how `crowbarChat` works today.

**Tech Stack:** React, Zustand + Immer, `@uiw/react-codemirror` (already installed), `@tanstack/react-virtual` (already used), `idb` (already used), CrossUI `Frame`/`Tabs`/`Button`/`Group` components (already local), Vitest + jsdom.

---

### Task 1: Types and mock data foundation

**Files:**
- Create: `web/src/features/branch-review/types/review-types.ts`
- Modify: `web/src/features/panes/types/pane-content.ts`
- Modify: `web/src/lib/store/sidebar.ts`
- Modify: `web/src/lib/mock/workspaces.ts`
- Create: `web/src/lib/mock/branch-diff.ts`
- Create: `web/src/__tests__/features/branch-review/review-types.test.ts`

- [ ] **Step 1: Create the review types file**

```ts
// web/src/features/branch-review/types/review-types.ts

export type MergeStrategy = 'merge' | 'squash' | 'rebase'

export interface ReviewMessage {
  id: string
  author: string | null  // agent name, or null = current user ("You")
  isAgent: boolean
  body: string
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

- [ ] **Step 2: Create the mock branch diff data**

```ts
// web/src/lib/mock/branch-diff.ts
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'

export function getMockBranchDiff(_wsId: string): MultiFileDiff {
  return {
    commitHash: 'mock-branch-diff',
    commitMessage: 'Mock branch changes',
    files: [
      {
        file_path: 'src/api/routes.ts',
        is_new: false,
        is_deleted: false,
        is_renamed: false,
        additions: 42,
        deletions: 8,
        lines: [
          { line_type: 'header', content: '@@ -1,8 +1,42 @@' },
          { line_type: 'context', content: 'import { Router } from "express"', old_line_number: 1, new_line_number: 1 },
          { line_type: 'removed', content: 'const router = Router()', old_line_number: 2 },
          { line_type: 'added', content: 'const router = Router({ strict: true })', new_line_number: 2 },
          { line_type: 'added', content: '', new_line_number: 3 },
          { line_type: 'added', content: 'router.use(rateLimit({ windowMs: 60_000, max: 100 }))', new_line_number: 4 },
          { line_type: 'context', content: '', old_line_number: 3, new_line_number: 5 },
          { line_type: 'context', content: 'export default router', old_line_number: 4, new_line_number: 6 },
        ],
      },
      {
        file_path: 'src/middleware/auth.ts',
        is_new: true,
        is_deleted: false,
        is_renamed: false,
        additions: 28,
        deletions: 0,
        lines: [
          { line_type: 'header', content: '@@ -0,0 +1,28 @@' },
          { line_type: 'added', content: 'import { verifyToken } from "../lib/jwt"', new_line_number: 1 },
          { line_type: 'added', content: '', new_line_number: 2 },
          { line_type: 'added', content: 'export function authMiddleware(req, res, next) {', new_line_number: 3 },
          { line_type: 'added', content: '  const token = req.headers.authorization?.split(" ")[1]', new_line_number: 4 },
          { line_type: 'added', content: '  if (!token) return res.status(401).json({ error: "Unauthorized" })', new_line_number: 5 },
          { line_type: 'added', content: '  req.user = verifyToken(token)', new_line_number: 6 },
          { line_type: 'added', content: '  next()', new_line_number: 7 },
          { line_type: 'added', content: '}', new_line_number: 8 },
        ],
      },
    ],
    totalFiles: 2,
    totalAdditions: 70,
    totalDeletions: 8,
  }
}
```

- [ ] **Step 3: Write tests for the type shapes**

```ts
// web/src/__tests__/features/branch-review/review-types.test.ts
import { describe, it, expect } from 'vitest'
import type { ReviewThread, ReviewMessage, MergeStrategy } from '@/features/branch-review/types/review-types'
import { getMockBranchDiff } from '@/lib/mock/branch-diff'

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

  it('MergeStrategy covers all three values', () => {
    const strategies: MergeStrategy[] = ['merge', 'squash', 'rebase']
    expect(strategies).toHaveLength(3)
  })

  it('getMockBranchDiff returns a valid MultiFileDiff', () => {
    const diff = getMockBranchDiff('ws1')
    expect(diff.files.length).toBeGreaterThan(0)
    expect(diff.totalFiles).toBe(diff.files.length)
  })
})
```

- [ ] **Step 4: Run tests**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/review-types.test.ts
```

Expected: PASS (4 tests)

- [ ] **Step 5: Add `isUncloseable` to `PaneContentBase` and add `BranchReviewContent`**

In `web/src/features/panes/types/pane-content.ts`:

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
  isUncloseable?: boolean;
}
```

2. Add `"branchReview"` to `PaneContentType` union.

3. Add the interface after `CrowbarChatContent`:
```ts
export interface BranchReviewContent extends PaneContentBase {
  type: "branchReview";
  wsId: string;
  branchName: string;
  isUncloseable: true;
}
```

4. Add `BranchReviewContent` to the `PaneContent` union.

5. Add type guard:
```ts
export function isBranchReviewContent(c: PaneContent): c is BranchReviewContent {
  return c.type === "branchReview";
}
```

6. Add `"branchReview"` to `VIRTUAL_TYPES`.

7. Add to `OpenContentSpec`:
```ts
| { type: "branchReview"; wsId: string; branchName: string; name: string; }
```

- [ ] **Step 6: Add `hasConflicts` to `Workspace` and mock data**

In `web/src/lib/store/sidebar.ts`, add to `Workspace` interface:
```ts
hasConflicts?: boolean
```

In `web/src/lib/mock/workspaces.ts`, add `hasConflicts: true` to `ws5` and `qc3` in `getMockRepos()`.

In `web/src/lib/store/sidebar.ts`, add `hasConflicts: true` to `ws5` and `qc3` in `INITIAL_REPOS`.

- [ ] **Step 7: Commit**

```bash
cd web && git add src/features/branch-review/types/review-types.ts src/lib/mock/branch-diff.ts src/features/panes/types/pane-content.ts src/lib/store/sidebar.ts src/lib/mock/workspaces.ts src/__tests__/features/branch-review/review-types.test.ts
git commit -m "feat(branch-review): add types, mock diff data, pane content type, conflict flags"
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
    const { branchReview } = makeStore().getState()
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
    store.getState().addReviewThread({
      id: 't1', filePath: 'src/index.ts', lineNumber: 10,
      side: 'right', messages: [], isResolved: false,
    })
    expect(store.getState().branchReview.threads).toHaveLength(1)
    expect(store.getState().branchReview.threads[0].id).toBe('t1')
  })

  it('addReviewMessage appends to the correct thread', () => {
    const store = makeStore()
    store.getState().addReviewThread({ id: 't1', filePath: 'src/index.ts', lineNumber: 10, side: 'right', messages: [], isResolved: false })
    store.getState().addReviewMessage('t1', { id: 'm1', author: null, isAgent: false, body: 'looks good', createdAt: '2026-06-01' })
    expect(store.getState().branchReview.threads[0].messages).toHaveLength(1)
    expect(store.getState().branchReview.threads[0].messages[0].id).toBe('m1')
  })

  it('resolveReviewThread marks thread as resolved', () => {
    const store = makeStore()
    store.getState().addReviewThread({ id: 't1', filePath: 'src/index.ts', lineNumber: 10, side: 'right', messages: [], isResolved: false })
    store.getState().resolveReviewThread('t1')
    expect(store.getState().branchReview.threads[0].isResolved).toBe(true)
  })
})
```

- [ ] **Step 2: Run — expect FAIL (module not found)**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/branch-review-slice.test.ts
```

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

export const INITIAL_BRANCH_REVIEW_STATE: BranchReviewState = {
  description: '',
  mergeStrategy: 'merge',
  activeSubtab: 'about',
  diffCache: null,
  diffStatus: 'idle',
  threads: [],
}

export const createBranchReviewSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  BranchReviewSlice
> = (set) => ({
  branchReview: { ...INITIAL_BRANCH_REVIEW_STATE },

  setBranchReviewDescription: (description) =>
    set(s => { s.branchReview.description = description }),

  setBranchReviewMergeStrategy: (strategy) =>
    set(s => { s.branchReview.mergeStrategy = strategy }),

  setBranchReviewSubtab: (tab) =>
    set(s => { s.branchReview.activeSubtab = tab }),

  setBranchReviewDiff: (diff) =>
    set(s => { s.branchReview.diffCache = diff; s.branchReview.diffStatus = 'loaded' }),

  setBranchReviewDiffStatus: (status) =>
    set(s => { s.branchReview.diffStatus = status }),

  addReviewThread: (thread) =>
    set(s => { s.branchReview.threads.push(thread) }),

  addReviewMessage: (threadId, message) =>
    set(s => {
      const t = s.branchReview.threads.find(t => t.id === threadId)
      if (t) t.messages.push(message)
    }),

  resolveReviewThread: (threadId) =>
    set(s => {
      const t = s.branchReview.threads.find(t => t.id === threadId)
      if (t) t.isResolved = true
    }),
})
```

- [ ] **Step 4: Wire into `workspace-store.types.ts`**

Add to imports and the `WorkspaceState` intersection:
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
  & BranchReviewSlice
```

- [ ] **Step 5: Wire into `workspace-store.ts`**

```ts
import { createBranchReviewSlice } from './slices/branch-review-slice'

// inside the immer factory, add:
...createBranchReviewSlice(set, get, api),
```

- [ ] **Step 6: Run tests — expect PASS**

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

### Task 3: IndexDB persistence for branch review state

**Files:**
- Modify: `web/src/lib/persistence/schemas.ts`
- Modify: `web/src/lib/persistence/idb.ts`
- Create: `web/src/lib/persistence/branch-review.ts`
- Modify: `web/src/features/workspace/stores/workspace-store.ts`
- Create: `web/src/__tests__/lib/persistence/branch-review-persistence.test.ts`

The `crowbar` IndexDB is currently at version 3. We bump to version 4 and add a `branch-review` object store keyed by `wsId`.

- [ ] **Step 1: Add schema type**

In `web/src/lib/persistence/schemas.ts`, add:

```ts
import type { ReviewThread, MergeStrategy } from '@/features/branch-review/types/review-types'

export interface BranchReviewPersistedState {
  wsId: string
  description: string
  mergeStrategy: MergeStrategy
  activeSubtab: 'about' | 'commits' | 'diff'
  threads: ReviewThread[]
  updatedAt: number
}
```

Add to `CrowbarDB`:
```ts
'branch-review': {
  key: string
  value: BranchReviewPersistedState
}
```

- [ ] **Step 2: Write the failing persistence tests**

```ts
// web/src/__tests__/lib/persistence/branch-review-persistence.test.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { saveBranchReview, loadBranchReview } from '@/lib/persistence/branch-review'
import { resetDB } from '@/lib/persistence/idb'

beforeEach(() => resetDB())

describe('branch-review persistence', () => {
  it('returns null when nothing has been saved', async () => {
    const result = await loadBranchReview('ws-test')
    expect(result).toBeNull()
  })

  it('saves and loads branch review state', async () => {
    await saveBranchReview('ws-test', {
      description: '# My branch',
      mergeStrategy: 'squash',
      activeSubtab: 'diff',
      threads: [],
    })
    const loaded = await loadBranchReview('ws-test')
    expect(loaded?.description).toBe('# My branch')
    expect(loaded?.mergeStrategy).toBe('squash')
    expect(loaded?.activeSubtab).toBe('diff')
    expect(loaded?.threads).toEqual([])
  })

  it('overwrites previous state on save', async () => {
    await saveBranchReview('ws-test', { description: 'v1', mergeStrategy: 'merge', activeSubtab: 'about', threads: [] })
    await saveBranchReview('ws-test', { description: 'v2', mergeStrategy: 'rebase', activeSubtab: 'commits', threads: [] })
    const loaded = await loadBranchReview('ws-test')
    expect(loaded?.description).toBe('v2')
    expect(loaded?.mergeStrategy).toBe('rebase')
  })

  it('persists threads across save/load', async () => {
    const threads = [{
      id: 't1', filePath: 'src/index.ts', lineNumber: 5,
      side: 'right' as const, isResolved: false,
      messages: [{ id: 'm1', author: 'Claude', isAgent: true, body: 'Nice!', createdAt: '2026-06-01' }],
    }]
    await saveBranchReview('ws-test', { description: '', mergeStrategy: 'merge', activeSubtab: 'about', threads })
    const loaded = await loadBranchReview('ws-test')
    expect(loaded?.threads).toHaveLength(1)
    expect(loaded?.threads[0].messages[0].author).toBe('Claude')
  })
})
```

- [ ] **Step 3: Run — expect FAIL**

```bash
cd web && npx vitest run src/__tests__/lib/persistence/branch-review-persistence.test.ts
```

- [ ] **Step 4: Bump DB to version 4 in `idb.ts`**

In `web/src/lib/persistence/idb.ts`, change `openDB<CrowbarDB>('crowbar', 3, {` to `openDB<CrowbarDB>('crowbar', 4, {` and add the v4 upgrade block:

```ts
if (oldVersion < 4) {
  db.createObjectStore('branch-review', { keyPath: 'wsId' })
}
```

- [ ] **Step 5: Implement the persistence module**

```ts
// web/src/lib/persistence/branch-review.ts
import { getDB } from './idb'
import type { BranchReviewPersistedState } from './schemas'
import type { ReviewThread, MergeStrategy } from '@/features/branch-review/types/review-types'

interface SavePayload {
  description: string
  mergeStrategy: MergeStrategy
  activeSubtab: 'about' | 'commits' | 'diff'
  threads: ReviewThread[]
}

export async function saveBranchReview(wsId: string, payload: SavePayload): Promise<void> {
  try {
    const db = await getDB()
    const record: BranchReviewPersistedState = {
      wsId,
      ...payload,
      updatedAt: Date.now(),
    }
    await db.put('branch-review', record)
  } catch {
    // storage unavailable — silently skip
  }
}

export async function loadBranchReview(wsId: string): Promise<Omit<BranchReviewPersistedState, 'wsId' | 'updatedAt'> | null> {
  try {
    const db = await getDB()
    const record = await db.get('branch-review', wsId)
    if (!record) return null
    const { wsId: _wsId, updatedAt: _updatedAt, ...rest } = record
    return rest
  } catch {
    return null
  }
}
```

- [ ] **Step 6: Run tests — expect PASS**

```bash
cd web && npx vitest run src/__tests__/lib/persistence/branch-review-persistence.test.ts
```

Expected: PASS (4 tests)

- [ ] **Step 7: Wire save into the workspace store**

In `web/src/features/workspace/stores/workspace-store.ts`, add a subscription that saves `branchReview` to IndexDB whenever the persisted fields change:

```ts
import { saveBranchReview } from '@/lib/persistence/branch-review'

// inside createWorkspaceStore, after the existing subscribe call:
store.subscribe((state, prev) => {
  const br = state.branchReview
  const prevBr = prev.branchReview
  if (
    br.description === prevBr.description &&
    br.mergeStrategy === prevBr.mergeStrategy &&
    br.activeSubtab === prevBr.activeSubtab &&
    br.threads === prevBr.threads
  ) return
  saveBranchReview(wsId, {
    description: br.description,
    mergeStrategy: br.mergeStrategy,
    activeSubtab: br.activeSubtab,
    threads: br.threads,
  })
})
```

- [ ] **Step 8: Wire load into workspace hydration**

Find where the workspace store is hydrated (where `loadFromLocalStorage` or similar is called, likely in `workspace-persistence.ts` or workspace route). Add a call to `loadBranchReview` and apply the result to the initial slice state via the snapshot mechanism.

In `web/src/features/workspace/stores/workspace-persistence.ts`, add:

```ts
import { loadBranchReview } from '@/lib/persistence/branch-review'
import { INITIAL_BRANCH_REVIEW_STATE } from './slices/branch-review-slice'

export async function loadBranchReviewSnapshot(wsId: string): Promise<{ branchReview: typeof INITIAL_BRANCH_REVIEW_STATE }> {
  const persisted = await loadBranchReview(wsId)
  if (!persisted) return { branchReview: { ...INITIAL_BRANCH_REVIEW_STATE } }
  return {
    branchReview: {
      ...INITIAL_BRANCH_REVIEW_STATE,
      description: persisted.description,
      mergeStrategy: persisted.mergeStrategy,
      activeSubtab: persisted.activeSubtab,
      threads: persisted.threads,
      // diffCache and diffStatus intentionally reset to defaults
    },
  }
}
```

Then find where `createWorkspaceStore` is called (likely in the workspace route or `WorkspaceView.tsx`) and await `loadBranchReviewSnapshot(wsId)` before calling `createWorkspaceStore`, merging the result into the snapshot argument.

- [ ] **Step 9: Commit**

```bash
cd web && git add src/lib/persistence/schemas.ts src/lib/persistence/idb.ts src/lib/persistence/branch-review.ts src/features/workspace/stores/workspace-store.ts src/features/workspace/stores/workspace-persistence.ts src/__tests__/lib/persistence/branch-review-persistence.test.ts
git commit -m "feat(branch-review): persist description, strategy, subtab, and threads to IndexDB"
```

---

### Task 4: Tab close suppression + buffer auto-open

**Files:**
- Modify: `web/src/features/tabs/components/tab-bar-item.tsx`
- Modify: `web/src/features/workspace/stores/slices/buffer-slice.ts`
- Modify: `web/src/features/workspace/stores/hooks/use-workspace-effects.ts`
- Modify: `web/src/features/panes/components/pane-container.tsx`
- Create: `web/src/__tests__/features/branch-review/tab-bar-uncloseable.test.ts`

- [ ] **Step 1: Write the tab suppression test**

```ts
// web/src/__tests__/features/branch-review/tab-bar-uncloseable.test.ts
import { describe, it, expect } from 'vitest'
import type { PaneContent } from '@/features/panes/types/pane-content'

function shouldShowCloseButton(buffer: PaneContent): boolean {
  return !buffer.isUncloseable
}

describe('tab close button visibility', () => {
  it('shows close button for normal editor tabs', () => {
    expect(shouldShowCloseButton({ type: 'editor', isUncloseable: undefined } as unknown as PaneContent)).toBe(true)
  })

  it('hides close button for branchReview tabs', () => {
    expect(shouldShowCloseButton({ type: 'branchReview', isUncloseable: true } as unknown as PaneContent)).toBe(false)
  })
})
```

- [ ] **Step 2: Run — expect PASS (pure logic, no component needed)**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/tab-bar-uncloseable.test.ts
```

- [ ] **Step 3: Suppress × in `tab-bar-item.tsx`**

In `web/src/features/tabs/components/tab-bar-item.tsx`, find the JSX that renders the close `<Button>` (the one calling `handleTabClose`). Wrap it:

```tsx
{!buffer.isUncloseable && (
  <Button /* existing props unchanged */ />
)}
```

- [ ] **Step 4: Handle `branchReview` singleton in `buffer-slice.ts`**

In `web/src/features/workspace/stores/slices/buffer-slice.ts`:

Add import: `import type { BranchReviewContent } from '@/features/panes/types/pane-content'`

In `openContent`, after the `crowbarChat` singleton check, add:
```ts
if (spec.type === 'branchReview') {
  const existing = Object.values(get().buffers).find(
    b => b.type === 'branchReview' && (b as BranchReviewContent).wsId === spec.wsId,
  )
  if (existing) return existing.id
}
```

In the buffer creation chain, after the `crowbarChat` case, add:
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

- [ ] **Step 5: Auto-open in `use-workspace-effects.ts`**

After the `crowbarChat` `useEffect`, add:
```ts
useEffect(() => {
  const branchName = label ?? wsId
  bufferActions.openContent({ type: 'branchReview', wsId, branchName, name: branchName })
}, [wsId]) // eslint-disable-line react-hooks/exhaustive-deps
```

- [ ] **Step 6: Register in `pane-container.tsx`**

Add lazy import:
```ts
const BranchReviewPane = lazy(() =>
  import('@/features/branch-review/components/branch-review-pane').then(m => ({ default: m.BranchReviewPane }))
)
```

Update the type import to include `BranchReviewContent`.

Add to the `renderActiveBuffer` switch:
```ts
case "branchReview":
  return (
    <BranchReviewPane
      wsId={(buffer as BranchReviewContent).wsId}
      branchName={(buffer as BranchReviewContent).branchName}
    />
  )
```

- [ ] **Step 7: Commit**

```bash
cd web && git add src/features/tabs/components/tab-bar-item.tsx src/features/workspace/stores/slices/buffer-slice.ts src/features/workspace/stores/hooks/use-workspace-effects.ts src/features/panes/components/pane-container.tsx src/__tests__/features/branch-review/tab-bar-uncloseable.test.ts
git commit -m "feat(branch-review): suppress close button, auto-open buffer on workspace mount"
```

---

### Task 5: MergeButton component

**Files:**
- Create: `web/src/features/branch-review/components/merge-button.tsx`
- Create: `web/src/__tests__/features/branch-review/merge-button.test.tsx`

- [ ] **Step 1: Write failing tests**

```tsx
// web/src/__tests__/features/branch-review/merge-button.test.tsx
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MergeButton } from '@/features/branch-review/components/merge-button'

describe('MergeButton', () => {
  it('shows the selected strategy label', () => {
    render(<MergeButton strategy="merge" isLocked={false} hasConflicts={false} onMerge={() => {}} onStrategyChange={() => {}} />)
    expect(screen.getByRole('button', { name: /merge commit/i })).toBeTruthy()
  })

  it('is disabled and shows tooltip when parent is locked', () => {
    render(<MergeButton strategy="merge" isLocked={true} hasConflicts={false} onMerge={() => {}} onStrategyChange={() => {}} />)
    const btn = screen.getByRole('button', { name: /merge commit/i })
    expect(btn).toBeDisabled()
    expect(btn.title).toMatch(/locked/i)
  })

  it('is disabled and shows tooltip when branch has conflicts', () => {
    render(<MergeButton strategy="squash" isLocked={false} hasConflicts={true} onMerge={() => {}} onStrategyChange={() => {}} />)
    const btn = screen.getByRole('button', { name: /squash and merge/i })
    expect(btn).toBeDisabled()
    expect(btn.title).toMatch(/conflict/i)
  })

  it('calls onMerge when enabled and clicked', async () => {
    const onMerge = vi.fn()
    render(<MergeButton strategy="rebase" isLocked={false} hasConflicts={false} onMerge={onMerge} onStrategyChange={() => {}} />)
    await userEvent.click(screen.getByRole('button', { name: /rebase and merge/i }))
    expect(onMerge).toHaveBeenCalledOnce()
  })
})
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/merge-button.test.tsx
```

- [ ] **Step 3: Implement**

```tsx
// web/src/features/branch-review/components/merge-button.tsx
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

- [ ] **Step 4: Run — expect PASS**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/merge-button.test.tsx
```

- [ ] **Step 5: Commit**

```bash
cd web && git add src/features/branch-review/components/merge-button.tsx src/__tests__/features/branch-review/merge-button.test.tsx
git commit -m "feat(branch-review): add MergeButton split button component"
```

---

### Task 6: BranchReviewHeader + AboutTab + CommitsTab

**Files:**
- Create: `web/src/features/branch-review/components/branch-review-header.tsx`
- Create: `web/src/features/branch-review/components/about-tab.tsx`
- Create: `web/src/features/branch-review/components/commits-tab.tsx`

These three components have no complex logic; group them in one task.

- [ ] **Step 1: Implement `branch-review-header.tsx`**

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
  branchName, parentBranch, isLocked, hasConflicts,
  aheadCount, additions, deletions, mergeStrategy, onMerge, onStrategyChange,
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
        {stats && <p className="mt-0.5 text-xs text-muted-foreground">{stats}</p>}
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

- [ ] **Step 2: Implement `about-tab.tsx`**

```tsx
// web/src/features/branch-review/components/about-tab.tsx
import CodeMirror from '@uiw/react-codemirror'
import { markdown } from '@codemirror/lang-markdown'
import { useSidebarStore } from '@/lib/store/sidebar'
import { cn } from '@/utils/cn'

interface AboutTabProps {
  description: string
  onDescriptionChange: (value: string) => void
  onOpenConversation: (id: string) => void
}

export function AboutTab({ description, onDescriptionChange, onOpenConversation }: AboutTabProps) {
  const chats = useSidebarStore(s => s.chats)

  return (
    <div className="flex flex-col divide-y divide-border">
      <div className="p-4">
        <h3 className="mb-2.5 text-xs font-semibold text-foreground">Description</h3>
        <div className="overflow-hidden rounded-lg border border-border bg-[#0d0d0d]">
          <CodeMirror
            value={description}
            extensions={[markdown()]}
            onChange={onDescriptionChange}
            basicSetup={{ lineNumbers: false, foldGutter: false, dropCursor: false, allowMultipleSelections: false, indentOnInput: true }}
            className="text-xs"
          />
        </div>
      </div>

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
                className="flex w-full items-center gap-2.5 rounded-lg border border-border bg-[#0d0d0d] px-3 py-2 text-left transition-colors hover:bg-muted/40"
              >
                <span className={cn('h-1.5 w-1.5 shrink-0 rounded-full',
                  chat.age === 'active' ? 'bg-green-500' : 'bg-muted-foreground/30')} />
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

- [ ] **Step 3: Implement `commits-tab.tsx`**

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
      if (!cancelled) { setCommits(result); setLoading(false) }
    })
    return () => { cancelled = true }
  }, [repoPath])

  return (
    <div className="p-4">
      <h3 className="mb-2.5 text-xs font-semibold text-foreground">Commit history</h3>
      {loading ? (
        <p className="text-xs text-muted-foreground/40">Loading commits…</p>
      ) : commits.length === 0 ? (
        <p className="text-xs text-muted-foreground/40">No commits on this branch yet.</p>
      ) : (
        <div className="flex flex-col gap-0.5">
          {commits.map(commit => (
            <div key={commit.hash} className="flex items-center gap-2.5 rounded-lg border border-border bg-[#0d0d0d] px-3 py-2">
              <span className="w-12 shrink-0 font-mono text-[10px] text-muted-foreground/60">{commit.hash.slice(0, 7)}</span>
              <span className="flex-1 truncate text-xs text-foreground">{commit.message}</span>
              <span className="shrink-0 text-[10px] text-muted-foreground/50">{formatRelativeDate(commit.date)}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
```

- [ ] **Step 4: Commit**

```bash
cd web && git add src/features/branch-review/components/branch-review-header.tsx src/features/branch-review/components/about-tab.tsx src/features/branch-review/components/commits-tab.tsx
git commit -m "feat(branch-review): add BranchReviewHeader, AboutTab, CommitsTab components"
```

---

### Task 7: ReviewThread component

**Files:**
- Create: `web/src/features/branch-review/components/review-thread.tsx`
- Create: `web/src/__tests__/features/branch-review/review-thread.test.tsx`

- [ ] **Step 1: Write failing tests**

```tsx
// web/src/__tests__/features/branch-review/review-thread.test.tsx
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ReviewThreadView } from '@/features/branch-review/components/review-thread'
import type { ReviewThread } from '@/features/branch-review/types/review-types'

const thread: ReviewThread = {
  id: 't1', filePath: 'src/index.ts', lineNumber: 10, side: 'right', isResolved: false,
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

  it('calls onReply with trimmed text on Enter', async () => {
    const onReply = vi.fn()
    render(<ReviewThreadView thread={thread} onReply={onReply} onResolve={() => {}} />)
    await userEvent.type(screen.getByPlaceholderText(/reply/i), 'My reply')
    await userEvent.keyboard('{Enter}')
    expect(onReply).toHaveBeenCalledWith('My reply')
  })

  it('clears the input after submit', async () => {
    render(<ReviewThreadView thread={thread} onReply={() => {}} onResolve={() => {}} />)
    const input = screen.getByPlaceholderText(/reply/i) as HTMLInputElement
    await userEvent.type(input, 'hello')
    await userEvent.keyboard('{Enter}')
    expect(input.value).toBe('')
  })

  it('calls onResolve when resolve button clicked', async () => {
    const onResolve = vi.fn()
    render(<ReviewThreadView thread={thread} onReply={() => {}} onResolve={onResolve} />)
    await userEvent.click(screen.getByRole('button', { name: /resolve/i }))
    expect(onResolve).toHaveBeenCalledOnce()
  })
})
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/review-thread.test.tsx
```

- [ ] **Step 3: Implement**

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
    <div className={cn('my-1 rounded-lg border border-border bg-muted/20', thread.isResolved && 'opacity-60')}>
      {thread.messages.map(msg => (
        <div key={msg.id} className="border-b border-border/40 last:border-b-0 px-3 py-2">
          <div className="mb-1 flex items-center gap-1.5">
            <span className="text-[11px] font-semibold text-foreground">{msg.author ?? 'You'}</span>
            {msg.isAgent && (
              <Badge variant="outline" className="h-3.5 px-1 text-[9px] text-blue-400 border-blue-400/30">
                agent
              </Badge>
            )}
          </div>
          <p className="text-xs leading-relaxed text-muted-foreground">{msg.body}</p>
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
        {!thread.isResolved ? (
          <Button size="sm" variant="ghost" className="h-6 px-2 text-[10px]" onClick={onResolve}>Resolve</Button>
        ) : (
          <button className="text-[10px] text-muted-foreground/40 underline" onClick={() => setCollapsed(true)}>Collapse</button>
        )}
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/review-thread.test.tsx
```

- [ ] **Step 5: Commit**

```bash
cd web && git add src/features/branch-review/components/review-thread.tsx src/__tests__/features/branch-review/review-thread.test.tsx
git commit -m "feat(branch-review): add ReviewThreadView with reply and resolve"
```

---

### Task 8: Bespoke diff viewer (hybrid virtualisation)

**Files:**
- Create: `web/src/features/branch-review/components/diff-file-section.tsx`
- Create: `web/src/features/branch-review/components/branch-review-diff-viewer.tsx`
- Create: `web/src/__tests__/features/branch-review/diff-viewer.test.tsx`

This is the most complex task. The viewer virtualises files at the top level. Files with > 200 lines use a nested line-level virtualiser with `measureElement` for dynamic heights from threads. Files below the threshold render flat.

- [ ] **Step 1: Write failing tests**

```tsx
// web/src/__tests__/features/branch-review/diff-viewer.test.tsx
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BranchReviewDiffViewer } from '@/features/branch-review/components/branch-review-diff-viewer'
import { getMockBranchDiff } from '@/lib/mock/branch-diff'

const mockDiff = getMockBranchDiff('ws1')

describe('BranchReviewDiffViewer', () => {
  it('renders file names from the diff', () => {
    render(<BranchReviewDiffViewer multiDiff={mockDiff} threads={[]} onAddThread={() => {}} onReply={() => {}} onResolve={() => {}} />)
    expect(screen.getByText('routes.ts')).toBeTruthy()
    expect(screen.getByText('auth.ts')).toBeTruthy()
  })

  it('shows additions and deletions counts per file', () => {
    render(<BranchReviewDiffViewer multiDiff={mockDiff} threads={[]} onAddThread={() => {}} onReply={() => {}} onResolve={() => {}} />)
    expect(screen.getByText('+42')).toBeTruthy()
    expect(screen.getByText('-8')).toBeTruthy()
  })

  it('renders inline thread for the correct line', () => {
    const threads = [{
      id: 't1', filePath: 'src/api/routes.ts', lineNumber: 2,
      side: 'right' as const, isResolved: false,
      messages: [{ id: 'm1', author: 'Claude', isAgent: true, body: 'Use strict mode', createdAt: '2026-06-01' }],
    }]
    render(<BranchReviewDiffViewer multiDiff={mockDiff} threads={threads} onAddThread={() => {}} onReply={() => {}} onResolve={() => {}} />)
    expect(screen.getByText('Use strict mode')).toBeTruthy()
  })

  it('calls onAddThread when + button is clicked on a line', async () => {
    const onAddThread = vi.fn()
    render(<BranchReviewDiffViewer multiDiff={mockDiff} threads={[]} onAddThread={onAddThread} onReply={() => {}} onResolve={() => {}} />)
    const addButtons = screen.getAllByRole('button', { name: /add thread/i })
    await userEvent.click(addButtons[0])
    expect(onAddThread).toHaveBeenCalledOnce()
  })
})
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/diff-viewer.test.tsx
```

- [ ] **Step 3: Implement `diff-file-section.tsx`**

```tsx
// web/src/features/branch-review/components/diff-file-section.tsx
import { memo, useCallback, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { CaretDown, CaretRight, FilePlus, FileText, FileX, Plus } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { ReviewThreadView } from './review-thread'
import type { GitDiff, GitDiffLine } from '@/features/git/types/git-types'
import type { ReviewThread } from '@/features/branch-review/types/review-types'
import { cn } from '@/utils/cn'

const LINE_VIRTUALIZATION_THRESHOLD = 200

interface DiffFileSectionProps {
  diff: GitDiff
  threads: ReviewThread[]
  onAddThread: (filePath: string, lineNumber: number) => void
  onReply: (threadId: string, body: string) => void
  onResolve: (threadId: string) => void
}

const statusIcon = (diff: GitDiff) => {
  if (diff.is_new) return <FilePlus size={13} className="text-git-added shrink-0" />
  if (diff.is_deleted) return <FileX size={13} className="text-git-deleted shrink-0" />
  return <FileText size={13} className="text-git-modified shrink-0" />
}

const statusColor = (diff: GitDiff) =>
  diff.is_new ? 'text-git-added' : diff.is_deleted ? 'text-git-deleted' : 'text-git-modified'

export const DiffFileSection = memo(function DiffFileSection({
  diff, threads, onAddThread, onReply, onResolve,
}: DiffFileSectionProps) {
  const [expanded, setExpanded] = useState(true)
  const fileName = diff.file_path.split('/').pop() ?? diff.file_path
  const isLarge = diff.lines.length > LINE_VIRTUALIZATION_THRESHOLD
  const fileThreads = threads.filter(t => t.filePath === diff.file_path)

  return (
    <div className="border-b border-border last:border-b-0">
      {/* File header */}
      <div
        className="flex cursor-pointer items-center gap-2 bg-muted/30 px-3 py-1.5 hover:bg-muted/50 transition-colors"
        onClick={() => setExpanded(e => !e)}
      >
        {expanded ? <CaretDown size={12} className="text-muted-foreground" /> : <CaretRight size={12} className="text-muted-foreground" />}
        {statusIcon(diff)}
        <span className={cn('flex-1 truncate text-xs font-medium', statusColor(diff))}>{fileName}</span>
        <span className="text-[10px] text-muted-foreground">{diff.file_path}</span>
        <div className="ml-2 flex items-center gap-1.5 text-[10px]">
          {(diff.additions ?? 0) > 0 && <span className="text-git-added">+{diff.additions}</span>}
          {(diff.deletions ?? 0) > 0 && <span className="text-git-deleted">-{diff.deletions}</span>}
        </div>
      </div>

      {expanded && (
        isLarge
          ? <VirtualizedLines diff={diff} fileThreads={fileThreads} onAddThread={onAddThread} onReply={onReply} onResolve={onResolve} />
          : <FlatLines diff={diff} fileThreads={fileThreads} onAddThread={onAddThread} onReply={onReply} onResolve={onResolve} />
      )}
    </div>
  )
})

interface LinesProps {
  diff: GitDiff
  fileThreads: ReviewThread[]
  onAddThread: (filePath: string, lineNumber: number) => void
  onReply: (threadId: string, body: string) => void
  onResolve: (threadId: string) => void
}

function DiffLineRow({ line, threads, onAddThread, filePath, onReply, onResolve }: {
  line: GitDiffLine
  threads: ReviewThread[]
  onAddThread: (filePath: string, lineNumber: number) => void
  filePath: string
  onReply: (threadId: string, body: string) => void
  onResolve: (threadId: string) => void
}) {
  const lineThreads = threads.filter(t => t.lineNumber === line.new_line_number)
  const bgColor = line.line_type === 'added'
    ? 'bg-git-added/10'
    : line.line_type === 'removed'
    ? 'bg-git-deleted/10'
    : line.line_type === 'header'
    ? 'bg-muted/40 text-muted-foreground italic'
    : ''

  return (
    <>
      <div className={cn('group flex min-h-[22px] items-start font-mono text-xs', bgColor)}>
        {/* Line number gutter with + hover button */}
        <div className="relative flex w-20 shrink-0 select-none items-center justify-end gap-1 border-r border-border/30 px-2 text-[10px] text-muted-foreground/40">
          {line.old_line_number && <span className="w-6 text-right">{line.old_line_number}</span>}
          {line.new_line_number && <span className="w-6 text-right">{line.new_line_number}</span>}
          {line.new_line_number && (
            <Button
              variant="ghost"
              size="sm"
              aria-label="Add thread"
              className="absolute right-0 hidden h-4 w-4 p-0 group-hover:flex"
              onClick={() => onAddThread(filePath, line.new_line_number!)}
            >
              <Plus size={10} />
            </Button>
          )}
        </div>
        {/* Line prefix and content */}
        <div className="flex-1 overflow-hidden px-2 py-[2px] whitespace-pre-wrap break-all leading-[18px]">
          <span className={cn('mr-1', line.line_type === 'added' ? 'text-git-added' : line.line_type === 'removed' ? 'text-git-deleted' : 'text-transparent')}>
            {line.line_type === 'added' ? '+' : line.line_type === 'removed' ? '-' : ' '}
          </span>
          {line.content}
        </div>
      </div>
      {lineThreads.map(thread => (
        <div key={thread.id} className="px-4">
          <ReviewThreadView
            thread={thread}
            onReply={body => onReply(thread.id, body)}
            onResolve={() => onResolve(thread.id)}
          />
        </div>
      ))}
    </>
  )
}

function FlatLines({ diff, fileThreads, onAddThread, onReply, onResolve }: LinesProps) {
  return (
    <div>
      {diff.lines.map((line, i) => (
        <DiffLineRow
          key={i}
          line={line}
          threads={fileThreads}
          filePath={diff.file_path}
          onAddThread={onAddThread}
          onReply={onReply}
          onResolve={onResolve}
        />
      ))}
    </div>
  )
}

function VirtualizedLines({ diff, fileThreads, onAddThread, onReply, onResolve }: LinesProps) {
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: diff.lines.length,
    getScrollElement: () => parentRef.current,
    estimateSize: (i) => {
      const line = diff.lines[i]
      const lineThreads = fileThreads.filter(t => t.lineNumber === line.new_line_number)
      return 22 + lineThreads.reduce((acc, t) => acc + 44 + t.messages.length * 56, 0)
    },
    measureElement: el => el.getBoundingClientRect().height,
    overscan: 10,
  })

  return (
    <div ref={parentRef} style={{ maxHeight: 600, overflowY: 'auto' }}>
      <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
        {virtualizer.getVirtualItems().map(vItem => (
          <div
            key={vItem.key}
            data-index={vItem.index}
            ref={virtualizer.measureElement}
            style={{ position: 'absolute', top: 0, left: 0, width: '100%', transform: `translateY(${vItem.start}px)` }}
          >
            <DiffLineRow
              line={diff.lines[vItem.index]}
              threads={fileThreads}
              filePath={diff.file_path}
              onAddThread={onAddThread}
              onReply={onReply}
              onResolve={onResolve}
            />
          </div>
        ))}
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Implement `branch-review-diff-viewer.tsx`**

```tsx
// web/src/features/branch-review/components/branch-review-diff-viewer.tsx
import { useRef } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { DiffFileSection } from './diff-file-section'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { ReviewThread } from '@/features/branch-review/types/review-types'

interface BranchReviewDiffViewerProps {
  multiDiff: MultiFileDiff
  threads: ReviewThread[]
  onAddThread: (filePath: string, lineNumber: number) => void
  onReply: (threadId: string, body: string) => void
  onResolve: (threadId: string) => void
}

export function BranchReviewDiffViewer({ multiDiff, threads, onAddThread, onReply, onResolve }: BranchReviewDiffViewerProps) {
  const parentRef = useRef<HTMLDivElement>(null)

  const virtualizer = useVirtualizer({
    count: multiDiff.files.length,
    getScrollElement: () => parentRef.current,
    estimateSize: (i) => {
      const file = multiDiff.files[i]
      const fileThreads = threads.filter(t => t.filePath === file.file_path)
      const threadHeight = fileThreads.reduce((acc, t) => acc + 44 + t.messages.length * 56, 0)
      return 36 + file.lines.length * 22 + threadHeight
    },
    measureElement: el => el.getBoundingClientRect().height,
    overscan: 2,
  })

  return (
    <div ref={parentRef} className="h-full overflow-y-auto">
      <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
        {virtualizer.getVirtualItems().map(vItem => {
          const file = multiDiff.files[vItem.index]
          const fileThreads = threads.filter(t => t.filePath === file.file_path)
          return (
            <div
              key={vItem.key}
              data-index={vItem.index}
              ref={virtualizer.measureElement}
              style={{ position: 'absolute', top: 0, left: 0, width: '100%', transform: `translateY(${vItem.start}px)` }}
            >
              <DiffFileSection
                diff={file}
                threads={fileThreads}
                onAddThread={onAddThread}
                onReply={onReply}
                onResolve={onResolve}
              />
            </div>
          )
        })}
      </div>
    </div>
  )
}
```

- [ ] **Step 5: Run tests — expect PASS**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/diff-viewer.test.tsx
```

- [ ] **Step 6: Commit**

```bash
cd web && git add src/features/branch-review/components/diff-file-section.tsx src/features/branch-review/components/branch-review-diff-viewer.tsx src/__tests__/features/branch-review/diff-viewer.test.tsx
git commit -m "feat(branch-review): add bespoke hybrid-virtualised diff viewer with thread injection"
```

---

### Task 9: DiffTab + BranchReviewPane (top-level shell)

**Files:**
- Create: `web/src/features/branch-review/components/diff-tab.tsx`
- Create: `web/src/features/branch-review/components/branch-review-pane.tsx`

- [ ] **Step 1: Implement `diff-tab.tsx`**

```tsx
// web/src/features/branch-review/components/diff-tab.tsx
import { useEffect } from 'react'
import { LoadingSpinner } from '@/components/ui/loading-spinner'
import { BranchReviewDiffViewer } from './branch-review-diff-viewer'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { getMockBranchDiff } from '@/lib/mock/branch-diff'

interface DiffTabProps {
  wsId: string
}

export function DiffTab({ wsId }: DiffTabProps) {
  const diffStatus = useWorkspaceStore(s => s.branchReview.diffStatus)
  const diffCache = useWorkspaceStore(s => s.branchReview.diffCache)
  const threads = useWorkspaceStore(s => s.branchReview.threads)
  const setBranchReviewDiff = useWorkspaceStore(s => s.setBranchReviewDiff)
  const setBranchReviewDiffStatus = useWorkspaceStore(s => s.setBranchReviewDiffStatus)
  const addReviewThread = useWorkspaceStore(s => s.addReviewThread)
  const addReviewMessage = useWorkspaceStore(s => s.addReviewMessage)
  const resolveReviewThread = useWorkspaceStore(s => s.resolveReviewThread)

  useEffect(() => {
    if (diffStatus !== 'idle') return
    setBranchReviewDiffStatus('loading')
    // Simulate async load with mock data
    setTimeout(() => {
      setBranchReviewDiff(getMockBranchDiff(wsId))
    }, 300)
  }, [wsId, diffStatus, setBranchReviewDiff, setBranchReviewDiffStatus])

  if (diffStatus === 'idle' || diffStatus === 'loading') {
    return <div className="flex items-center justify-center p-12"><LoadingSpinner /></div>
  }

  if (diffStatus === 'error' || !diffCache) {
    return <div className="p-4 text-xs text-muted-foreground">Failed to load diff.</div>
  }

  return (
    <BranchReviewDiffViewer
      multiDiff={diffCache}
      threads={threads}
      onAddThread={(filePath, lineNumber) => addReviewThread({
        id: crypto.randomUUID(),
        filePath,
        lineNumber,
        side: 'right',
        messages: [],
        isResolved: false,
      })}
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

- [ ] **Step 2: Implement `branch-review-pane.tsx`**

```tsx
// web/src/features/branch-review/components/branch-review-pane.tsx
import { Tabs, TabsList, TabsTab, TabsPanel } from '@/components/ui/tabs'
import { Frame } from '@/components/ui/frame'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { useSidebarStore } from '@/lib/store/sidebar'
import { BranchReviewHeader } from './branch-review-header'
import { AboutTab } from './about-tab'
import { CommitsTab } from './commits-tab'
import { DiffTab } from './diff-tab'

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

  const repos = useSidebarStore(s => s.repos)
  const allWorkspaces = repos.flatMap(r => r.workspaces)
  const workspace = allWorkspaces.find(w => w.id === wsId)
  const parentWorkspace = workspace?.parentId ? allWorkspaces.find(w => w.id === workspace.parentId) : null
  const parentBranch = parentWorkspace?.branch ?? 'main'
  const isLocked = parentWorkspace?.status === 'locked'
  const hasConflicts = workspace?.hasConflicts ?? false
  const additions = workspace?.added ?? 0
  const deletions = workspace?.deleted ?? 0
  const repoPath = `/repos/${wsId}`

  return (
    <div className="flex h-full flex-col overflow-hidden p-2">
      <Frame className="flex h-full flex-col overflow-hidden">
        <BranchReviewHeader
          branchName={branchName}
          parentBranch={parentBranch}
          isLocked={isLocked}
          hasConflicts={hasConflicts}
          aheadCount={3}
          additions={additions}
          deletions={deletions}
          mergeStrategy={mergeStrategy}
          onMerge={() => {}}
          onStrategyChange={setBranchReviewMergeStrategy}
        />

        <Tabs
          value={activeSubtab}
          onValueChange={v => setBranchReviewSubtab(v as typeof activeSubtab)}
          className="flex flex-1 flex-col overflow-hidden px-4 pb-0"
        >
          <TabsList variant="default" className="mb-3 w-fit">
            <TabsTab value="about">About</TabsTab>
            <TabsTab value="commits">Commits</TabsTab>
            <TabsTab value="diff">Diff</TabsTab>
          </TabsList>

          {/* Content card — rounded top, flush sides + bottom */}
          <div className="flex-1 overflow-hidden rounded-t-xl border border-border bg-background">
            <TabsPanel value="about" className="h-full overflow-y-auto">
              <AboutTab
                description={description}
                onDescriptionChange={setBranchReviewDescription}
                onOpenConversation={() => {}}
              />
            </TabsPanel>
            <TabsPanel value="commits" className="h-full overflow-y-auto">
              <CommitsTab repoPath={repoPath} />
            </TabsPanel>
            <TabsPanel value="diff" className="h-full overflow-hidden">
              <DiffTab wsId={wsId} />
            </TabsPanel>
          </div>
        </Tabs>
      </Frame>
    </div>
  )
}
```

- [ ] **Step 3: Commit**

```bash
cd web && git add src/features/branch-review/components/diff-tab.tsx src/features/branch-review/components/branch-review-pane.tsx
git commit -m "feat(branch-review): add DiffTab and BranchReviewPane shell with CrossUI Tabs"
```

---

### Task 10: Full test suite + smoke test

**Files:**
- Verify all previous work compiles and tests pass

- [ ] **Step 1: Run the full test suite**

```bash
cd web && npx vitest run
```

Expected: All existing tests pass + all new branch-review tests pass. Fix any TypeScript or import errors before proceeding.

- [ ] **Step 2: Type-check**

```bash
cd web && npx tsc --noEmit
```

Expected: No errors. Fix anything before the smoke test.

- [ ] **Step 3: Start the dev server and verify**

```bash
cd web && npm run dev
```

Open `http://localhost:5173`. Navigate to any workspace and verify:
- A tab labelled with the branch name appears in the tab bar with **no × button**
- The tab can be dragged to other pane positions
- The Frame renders with header (branch name + → parent badge + merge button), Tabs switcher (About / Commits / Diff), and content card (rounded top, flush sides and bottom)
- **About tab**: CodeMirror description editor is editable; Conversations list shows chats
- **Commits tab**: Shows loading then commit list
- **Diff tab**: Shows spinner then file sections; hovering a line number shows the `+` button; clicking it creates an inline thread; typing in the thread reply box and pressing Enter submits
- On a workspace with `hasConflicts: true` (`refactor/query-layer`), the merge button is red and disabled
- On a workspace whose parent is `locked` (e.g. `develop`), the merge button is gray and disabled
- Reload the page — description text, merge strategy, and active subtab are restored from IndexDB

- [ ] **Step 4: Final commit**

```bash
cd web && git add -p
git commit -m "feat(branch-review): complete branch review screen"
```
