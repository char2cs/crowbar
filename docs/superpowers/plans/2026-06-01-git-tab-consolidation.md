# Git Tab Consolidation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move all Git operations out of the sidebar into a unified "Git" tab inside the branch review pane, then remove the sidebar Git tab entirely.

**Architecture:** A new `GitTab` component composites all existing git sub-components (commit panel, status panel, history, stash, remotes, tags). It uses a `ResizeObserver` on its container to switch between a two-column layout (wide ≥ 480 px) and a stacked layout (narrow). The sidebar `SidebarTab` type loses its `'git'` member; the branch-review `activeSubtab` type replaces `'commits'` with `'git'`.

**Tech Stack:** React, Zustand (`useGitStore`, `useRepositoryStore`), `@/components/ui/*`, CSS variable tokens from `theme.css`, Vitest + `@testing-library/react`.

---

## File Map

| Action | Path | Responsibility |
|---|---|---|
| **Create** | `web/src/features/branch-review/components/git-tab.tsx` | Responsive Git tab — sticky commit header, scrollable file/history/stash/remotes/tags sections |
| **Create** | `web/src/__tests__/features/branch-review/components/git-tab.test.tsx` | Render smoke test + responsive layout toggle |
| **Modify** | `web/src/features/workspace/stores/slices/branch-review-slice.ts` | `activeSubtab` type: `'commits'` → `'git'` |
| **Modify** | `web/src/features/branch-review/components/branch-review-pane.tsx` | Replace Commits tab with Git tab |
| **Modify** | `web/src/lib/store/sidebar.ts` | Remove `'git'` from `SidebarTab` union |
| **Modify** | `web/src/components/layout/sidebar-nav-icons.tsx` | Remove Git nav item from `NAV_ITEMS` |
| **Modify** | `web/src/components/layout/SidebarTabs.tsx` | Remove `TabsContent value="git"` and `GitView` import |
| **Delete** | `web/src/features/git/components/git-view.tsx` | No longer used |

---

### Task 1: Update `activeSubtab` type in the branch-review slice

**Files:**
- Modify: `web/src/features/workspace/stores/slices/branch-review-slice.ts`

- [ ] **Step 1: Replace `'commits'` with `'git'` in the `activeSubtab` union**

In `branch-review-slice.ts`, find line 9:
```ts
activeSubtab: 'about' | 'commits' | 'diff'
```
Change to:
```ts
activeSubtab: 'about' | 'git' | 'diff'
```
The default value on line 34 is `'about'` — leave it unchanged.

Also update the setter signature on line 20:
```ts
setBranchReviewSubtab: (tab: BranchReviewState['activeSubtab']) => void
```
This derives from the type so no literal string change needed there.

- [ ] **Step 2: Run TypeScript check to catch any callsites that pass `'commits'`**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "commits\|activeSubtab"
```
Expected: errors pointing to `branch-review-pane.tsx` (the tab value). Fix them in Task 3. Any other files failing here need the string `'commits'` changed to `'git'` at that callsite.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/workspace/stores/slices/branch-review-slice.ts
git commit -m "refactor(branch-review): rename activeSubtab 'commits' → 'git'"
```

---

### Task 2: Create the `GitTab` component

**Files:**
- Create: `web/src/features/branch-review/components/git-tab.tsx`
- Create: `web/src/__tests__/features/branch-review/components/git-tab.test.tsx`

- [ ] **Step 1: Write the failing render test**

Create `web/src/__tests__/features/branch-review/components/git-tab.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeAll } from 'vitest'
import { render, screen } from '@testing-library/react'
import { GitTab } from '@/features/branch-review/components/git-tab'

// ResizeObserver is not available in jsdom
beforeAll(() => {
  global.ResizeObserver = vi.fn().mockImplementation((cb) => ({
    observe: vi.fn(),
    unobserve: vi.fn(),
    disconnect: vi.fn(),
  }))
})

vi.mock('@/features/git/stores/git-store', () => ({
  useGitStore: vi.fn((selector) => selector({
    gitStatus: { files: [], ahead: 0, behind: 0 },
  })),
}))

vi.mock('@/features/git/stores/git-repository-store', () => ({
  useRepositoryStore: Object.assign(
    vi.fn((selector) => selector({ activeRepoPath: '/mock/repo' })),
    { use: { activeRepoPath: () => '/mock/repo' } }
  ),
}))

vi.mock('@/features/settings/store', () => ({
  useSettingsStore: vi.fn((selector) => selector({
    settings: { showUntrackedFiles: true },
  })),
}))

describe('GitTab', () => {
  it('renders without crashing', () => {
    render(<GitTab wsId="ws-1" />)
    // The commit panel textarea should be present
    expect(screen.getByPlaceholderText('Commit message...')).toBeDefined()
  })
})
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/components/git-tab.test.tsx
```
Expected: FAIL — `GitTab` does not exist yet.

- [ ] **Step 3: Create `git-tab.tsx`**

Create `web/src/features/branch-review/components/git-tab.tsx`:

```tsx
import { useEffect, useRef, useState } from 'react'
import { useGitStore } from '@/features/git/stores/git-store'
import { useRepositoryStore } from '@/features/git/stores/git-repository-store'
import { useSettingsStore } from '@/features/settings/store'
import { useGitFileDiffStats } from '@/features/git/hooks/use-git-file-diff-stats'
import { Separator } from '@/components/ui/separator'
import GitCommitPanel from '@/features/git/components/git-commit-panel'
import GitStatusPanel from '@/features/git/components/status/git-status-panel'
import { GitStashCommandSurface } from '@/features/git/components/git-stash-command-surface'
import GitRemoteManager from '@/features/git/components/git-remote-manager'
import GitTagManager from '@/features/git/components/git-tag-manager'
import { CommitsTab } from './commits-tab'

const WIDE_BREAKPOINT = 480

interface GitTabProps {
  wsId: string
}

export function GitTab({ wsId }: GitTabProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [isWide, setIsWide] = useState(false)
  const [showStash, setShowStash] = useState(false)
  const [showRemotes, setShowRemotes] = useState(false)
  const [showTags, setShowTags] = useState(false)

  const activeRepoPath = useRepositoryStore.use.activeRepoPath()
  const gitStatus = useGitStore(s => s.gitStatus)
  const showUntracked = useSettingsStore(s => s.settings.showUntrackedFiles)

  const visibleFiles = showUntracked
    ? (gitStatus?.files ?? [])
    : (gitStatus?.files ?? []).filter(f => f.status !== 'untracked')

  const stagedCount = visibleFiles.filter(f => f.staged).length
  const fileDiffStats = useGitFileDiffStats(activeRepoPath, visibleFiles)

  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const observer = new ResizeObserver(([entry]) => {
      setIsWide(entry.contentRect.width >= WIDE_BREAKPOINT)
    })
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  const stickyCommit = (
    <div className="shrink-0 border-b border-border bg-background px-3 py-2">
      <GitCommitPanel
        stagedFilesCount={stagedCount}
        repoPath={activeRepoPath ?? undefined}
        ahead={gitStatus?.ahead ?? 0}
        behind={gitStatus?.behind ?? 0}
      />
    </div>
  )

  const fileSections = (
    <div className="flex flex-col gap-4 p-3">
      <GitStatusPanel
        files={visibleFiles}
        fileDiffStats={fileDiffStats}
        repoPath={activeRepoPath ?? undefined}
      />
      <GitStashCommandSurface
        isOpen={showStash}
        onClose={() => setShowStash(false)}
        repoPath={activeRepoPath}
        onViewStashDiff={async () => {}}
      />
      <GitRemoteManager
        isOpen={showRemotes}
        onClose={() => setShowRemotes(false)}
        repoPath={activeRepoPath ?? undefined}
      />
      <GitTagManager
        isOpen={showTags}
        onClose={() => setShowTags(false)}
        repoPath={activeRepoPath ?? undefined}
      />
    </div>
  )

  const historySection = (
    <div className="p-3">
      <CommitsTab repoPath={wsId} />
    </div>
  )

  return (
    <div ref={containerRef} className="flex h-full overflow-hidden">
      {isWide ? (
        <>
          <div className="flex flex-1 flex-col overflow-hidden border-r border-border">
            {stickyCommit}
            <div className="flex-1 overflow-y-auto">{fileSections}</div>
          </div>
          <div className="flex-1 overflow-y-auto">{historySection}</div>
        </>
      ) : (
        <div className="flex flex-1 flex-col overflow-hidden">
          {stickyCommit}
          <div className="flex-1 overflow-y-auto">
            {fileSections}
            <Separator className="my-2" />
            {historySection}
          </div>
        </div>
      )}
    </div>
  )
}
```

- [ ] **Step 4: Run the test to confirm it passes**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/components/git-tab.test.tsx
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/branch-review/components/git-tab.tsx \
        web/src/__tests__/features/branch-review/components/git-tab.test.tsx
git commit -m "feat(branch-review): add responsive GitTab component"
```

---

### Task 3: Wire `GitTab` into `BranchReviewPane`

**Files:**
- Modify: `web/src/features/branch-review/components/branch-review-pane.tsx`

- [ ] **Step 1: Replace the Commits tab with the Git tab**

In `branch-review-pane.tsx`:

1. Add import at the top (alongside other tab imports):
```tsx
import { GitTab } from './git-tab'
```

2. Remove this import:
```tsx
import { CommitsTab } from './commits-tab'
```

3. In the `TabsList` (around line 139), replace:
```tsx
<TabsTab value="commits">Commits</TabsTab>
```
With:
```tsx
<TabsTab value="git">Git</TabsTab>
```

4. Replace the entire `commits` `TabsPanel`:
```tsx
<TabsPanel value="commits" className="p-5">
  <CommitsTab repoPath={wsId} />
</TabsPanel>
```
With:
```tsx
<TabsPanel value="git" className="h-full overflow-hidden p-0">
  <GitTab wsId={wsId} />
</TabsPanel>
```

Note: The Git tab panel needs `h-full overflow-hidden p-0` (same pattern as the diff tab) because `GitTab` manages its own internal scroll and layout.

- [ ] **Step 2: Run TypeScript check**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "branch-review-pane\|git-tab\|CommitsTab"
```
Expected: no errors.

- [ ] **Step 3: Run the full test suite**

```bash
cd web && npx vitest run
```
Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add web/src/features/branch-review/components/branch-review-pane.tsx
git commit -m "feat(branch-review): replace Commits tab with unified Git tab"
```

---

### Task 4: Remove the sidebar Git tab

**Files:**
- Modify: `web/src/lib/store/sidebar.ts`
- Modify: `web/src/components/layout/sidebar-nav-icons.tsx`
- Modify: `web/src/components/layout/SidebarTabs.tsx`
- Delete: `web/src/features/git/components/git-view.tsx`

- [ ] **Step 1: Remove `'git'` from `SidebarTab` in the store**

In `web/src/lib/store/sidebar.ts`, find line 37:
```ts
export type SidebarTab = 'workspaces' | 'files' | 'git'
```
Change to:
```ts
export type SidebarTab = 'workspaces' | 'files'
```

If the default `activeTab` is set to `'git'` anywhere in the initial state, change it to `'workspaces'`. (Current default is `'workspaces'` — verify and leave as-is.)

- [ ] **Step 2: Remove the Git nav item from `SidebarNavIcons`**

In `web/src/components/layout/sidebar-nav-icons.tsx`, remove the git entry from `NAV_ITEMS`:

```ts
const NAV_ITEMS = [
  { tab: 'workspaces' as SidebarTab, label: 'Workspaces', Icon: SquaresFour },
  { tab: 'files'      as SidebarTab, label: 'Files',      Icon: FolderOpen  },
] as const
```

Also remove the unused `GitBranch` import:
```ts
import { SquaresFour, FolderOpen } from '@phosphor-icons/react'
```

- [ ] **Step 3: Remove the git `TabsContent` from `SidebarTabs`**

In `web/src/components/layout/SidebarTabs.tsx`:

1. Remove the import:
```tsx
import GitView from '@/features/git/components/git-view'
```

2. Remove the entire git TabsContent block:
```tsx
<TabsContent value="git" keepMounted className="flex flex-1 flex-col overflow-hidden mt-0">
  <ErrorBoundary>
    <Suspense fallback={<SidebarSkeleton />}>
      <GitView repoPath={activeWorkspaceRepoPath} />
    </Suspense>
  </ErrorBoundary>
</TabsContent>
```

- [ ] **Step 4: Delete `git-view.tsx`**

```bash
git rm web/src/features/git/components/git-view.tsx
```

- [ ] **Step 5: TypeScript check — confirm no remaining references**

```bash
cd web && npx tsc --noEmit 2>&1 | head -30
```
Expected: zero errors. If `git-view.tsx` is still imported somewhere, remove that import too.

- [ ] **Step 6: Run the full test suite**

```bash
cd web && npx vitest run
```
Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add web/src/lib/store/sidebar.ts \
        web/src/components/layout/sidebar-nav-icons.tsx \
        web/src/components/layout/SidebarTabs.tsx
git commit -m "feat: remove sidebar Git tab — all git ops now live in branch review pane"
```
