# Sidebar Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the 2-tab sidebar (workspaces, files) with a 4-tab swipeable sidebar (workspaces, chats, files, git), add a ChatList panel, a GitPanel with Changes/History subtabs, and delete the entire `branch-review` feature.

**Architecture:** The new `SidebarTabBar` (pill-style, CrossUI `Tab` component) replaces `SidebarNavIcons`. The new `SidebarCarousel` (CSS scroll-snap horizontal container) replaces `SidebarTabs` and mounts all 4 panels at once — trackpad horizontal swipe switches tabs by native scroll, a `scroll` handler keeps the store in sync. The `branch-review` feature is deleted wholesale; its two reusable comment components (`comment-composer`, `review-thread`) move to `features/panes/`.

**Tech Stack:** React 19, Zustand, Tailwind CSS v4, `@base-ui/react` (via `@/components/ui/tabs`), Phosphor Icons, Vitest + Testing Library.

---

## File Map

**Create:**
- `web/src/components/layout/sidebar-tab-bar.tsx` — pill tab bar, 4 tabs, CrossUI `Tab` component
- `web/src/components/layout/sidebar-carousel.tsx` — horizontal scroll-snap carousel, 4 panels, wheel handler
- `web/src/features/chats/components/chat-list-item.tsx` — single chat row, `ROW_BASE` style
- `web/src/features/chats/components/chat-list.tsx` — flat chat list + "+ New" button
- `web/src/features/git/components/git-history-list.tsx` — commit list from `GET /api/v0/git/log`
- `web/src/features/git/components/git-panel.tsx` — CrossUI Tabs: Changes + History
- `web/src/features/panes/lib/markdown.tsx` — moved from `branch-review/lib/markdown.tsx`
- `web/src/features/panes/components/comment-composer.tsx` — moved from `branch-review/components/`
- `web/src/features/panes/components/review-thread.tsx` — moved from `branch-review/components/`

**Modify:**
- `web/src/lib/store/sidebar.ts` — expand `SidebarTab` type to include `'chats' | 'git'`
- `web/src/components/layout/IDEShell.tsx` — replace `SidebarNavIcons` + `SidebarTabs` with new components
- `web/src/features/file-explorer/file-explorer/components/file-explorer-tree.tsx` — add `filter?: 'all' | 'changed'` prop
- `web/src/features/panes/types/pane-content.ts` — remove `BranchReviewContent` type and `branchReview` from all sets/unions
- `web/src/features/panes/components/pane-container.tsx` — remove `BranchReviewPane` import and `branchReview` case

**Delete:**
- `web/src/components/layout/SidebarTabs.tsx`
- `web/src/components/layout/sidebar-nav-icons.tsx`
- `web/src/features/branch-review/` (entire directory)

**Test files:**
- `web/src/__tests__/components/layout/sidebar-tab-bar.test.tsx` — new
- `web/src/__tests__/components/layout/sidebar-carousel.test.tsx` — replaces `sidebar-tabs.test.tsx` and `sidebar-nav-icons.test.tsx`
- `web/src/__tests__/features/chats/components/chat-list.test.tsx` — new
- `web/src/__tests__/features/git/components/git-panel.test.tsx` — new

---

## Task 1: Expand SidebarTab type

**Files:**
- Modify: `web/src/lib/store/sidebar.ts`

The store already has `setActiveTab`, `addChat`, `deleteChat`, and `ProjectChat`. Only the type union needs expanding.

- [ ] **Step 1: Update the type**

In `web/src/lib/store/sidebar.ts`, change line 37:

```ts
// Before
export type SidebarTab = 'workspaces' | 'files'

// After
export type SidebarTab = 'workspaces' | 'chats' | 'files' | 'git'
```

- [ ] **Step 2: Run TypeScript check**

```bash
cd web && npx tsc --noEmit 2>&1 | head -30
```

Expected: errors only about `SidebarTab` values `'chats'` and `'git'` not being handled in exhaustive switches — those will be fixed in later tasks. No errors in `sidebar.ts` itself.

- [ ] **Step 3: Run existing store tests**

```bash
cd web && npx vitest run src/__tests__/lib/store 2>&1 | tail -20
```

Expected: all pass (the type change is additive).

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/store/sidebar.ts
git commit -m "feat(sidebar): expand SidebarTab type to 4 tabs"
```

---

## Task 2: Build SidebarTabBar

Replaces `sidebar-nav-icons.tsx`. Uses `Tab` from `@/components/ui/tabs` (the pill-style component already in the project — active tab gets rounded filled background, inactive are muted text).

**Files:**
- Create: `web/src/components/layout/sidebar-tab-bar.tsx`
- Create: `web/src/__tests__/components/layout/sidebar-tab-bar.test.tsx`
- Delete: `web/src/components/layout/sidebar-nav-icons.tsx`

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/components/layout/sidebar-tab-bar.test.tsx`:

```tsx
import React from 'react'
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import { SidebarTabBar } from '@/components/layout/sidebar-tab-bar'
import { useSidebarStore } from '@/lib/store/sidebar'

describe('SidebarTabBar', () => {
  beforeEach(() => {
    useSidebarStore.setState((useSidebarStore as any).getInitialState())
  })

  it('renders all 4 tabs', () => {
    render(<SidebarTabBar />)
    expect(screen.getByRole('button', { name: /workspaces/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /chats/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /files/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /git/i })).toBeInTheDocument()
  })

  it('marks the active tab as pressed', () => {
    useSidebarStore.setState({ activeTab: 'files' })
    render(<SidebarTabBar />)
    expect(screen.getByRole('button', { name: /files/i })).toHaveAttribute('data-active', 'true')
  })

  it('calls setActiveTab when a tab is clicked', () => {
    render(<SidebarTabBar />)
    fireEvent.click(screen.getByRole('button', { name: /chats/i }))
    expect(useSidebarStore.getState().activeTab).toBe('chats')
  })
})
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd web && npx vitest run src/__tests__/components/layout/sidebar-tab-bar.test.tsx 2>&1 | tail -10
```

Expected: FAIL — `sidebar-tab-bar` module not found.

- [ ] **Step 3: Create SidebarTabBar**

Create `web/src/components/layout/sidebar-tab-bar.tsx`:

```tsx
import { SquaresFour, ChatsCircle, FolderOpen, GitBranch } from '@phosphor-icons/react'
import { Tab } from '@/components/ui/tabs'
import { useSidebarStore, type SidebarTab } from '@/lib/store/sidebar'

const TABS: { tab: SidebarTab; label: string; Icon: React.ComponentType<{ size: number; weight: 'fill' | 'regular' }> }[] = [
  { tab: 'workspaces', label: 'Workspaces', Icon: SquaresFour },
  { tab: 'chats',      label: 'Chats',      Icon: ChatsCircle  },
  { tab: 'files',      label: 'Files',      Icon: FolderOpen   },
  { tab: 'git',        label: 'Git',        Icon: GitBranch    },
]

export function SidebarTabBar() {
  const activeTab = useSidebarStore(s => s.activeTab)
  const setActiveTab = useSidebarStore(s => s.setActiveTab)

  return (
    <div className="flex h-8 shrink-0 items-center justify-center px-2">
      <div className="flex items-center gap-0.5 rounded-lg border border-white/8 bg-background p-0.5 shadow-sm shadow-black/30 not-disabled:inset-shadow-[0_1px_--theme(--color-white/16%)]">
        {TABS.map(({ tab, label, Icon }) => {
          const isActive = activeTab === tab
          return (
            <Tab
              key={tab}
              isActive={isActive}
              aria-label={label}
              data-active={isActive}
              className="flex h-7 items-center gap-1 px-2"
              onClick={() => setActiveTab(tab)}
            >
              <Icon size={14} weight={isActive ? 'fill' : 'regular'} />
              <span className="text-[13px] font-medium">{label}</span>
            </Tab>
          )
        })}
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Run tests**

```bash
cd web && npx vitest run src/__tests__/components/layout/sidebar-tab-bar.test.tsx 2>&1 | tail -10
```

Expected: all 3 pass.

- [ ] **Step 5: Delete the old nav icons file**

```bash
rm web/src/components/layout/sidebar-nav-icons.tsx
```

- [ ] **Step 6: Commit**

```bash
git add web/src/components/layout/sidebar-tab-bar.tsx web/src/__tests__/components/layout/sidebar-tab-bar.test.tsx
git rm web/src/components/layout/sidebar-nav-icons.tsx
git commit -m "feat(sidebar): add SidebarTabBar with 4-tab pill style, remove sidebar-nav-icons"
```

---

## Task 3: Build SidebarCarousel

Replaces `SidebarTabs.tsx`. Horizontal scroll-snap container with 4 panels always mounted. A `scroll` handler syncs the store's `activeTab`. A `useEffect` scrolls to the correct panel when `activeTab` changes programmatically (e.g. clicking the tab bar).

**Files:**
- Create: `web/src/components/layout/sidebar-carousel.tsx`
- Create: `web/src/__tests__/components/layout/sidebar-carousel.test.tsx`
- Delete: `web/src/components/layout/SidebarTabs.tsx`

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/components/layout/sidebar-carousel.test.tsx`:

```tsx
import React from 'react'
import { render, screen } from '@testing-library/react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { SidebarCarousel } from '@/components/layout/sidebar-carousel'
import { useSidebarStore } from '@/lib/store/sidebar'

vi.mock('@/components/layout/workspace-tree', () => ({
  WorkspaceTree: () => <div data-testid="panel-workspaces" />,
}))
vi.mock('@/features/chats/components/chat-list', () => ({
  ChatList: () => <div data-testid="panel-chats" />,
}))
vi.mock('@/features/file-explorer/components/file-explorer-tree', () => ({
  FileExplorerTree: () => <div data-testid="panel-files" />,
}))
vi.mock('@/features/git/components/git-panel', () => ({
  GitPanel: () => <div data-testid="panel-git" />,
}))
vi.mock('@/components/ErrorBoundary', () => ({
  ErrorBoundary: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))
vi.mock('@/features/file-system/controllers/store', () => ({
  useFileSystemStore: Object.assign(
    (sel: any) => sel({ files: [], handleFileOpen: null, handleFileSelect: null }),
    { use: { handleFileOpen: () => null, handleFileSelect: () => null } },
  ),
}))
vi.mock('@/features/file-explorer/stores/file-explorer-tree-store', () => ({
  useFileTreeStore: { getState: () => ({ toggleFolder: vi.fn() }) },
}))

describe('SidebarCarousel', () => {
  beforeEach(() => {
    useSidebarStore.setState((useSidebarStore as any).getInitialState())
  })

  it('mounts all 4 panels', () => {
    render(<SidebarCarousel activeWorkspaceRepoPath="/repos/default" />)
    expect(screen.getByTestId('panel-workspaces')).toBeInTheDocument()
    expect(screen.getByTestId('panel-chats')).toBeInTheDocument()
    expect(screen.getByTestId('panel-files')).toBeInTheDocument()
    expect(screen.getByTestId('panel-git')).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd web && npx vitest run src/__tests__/components/layout/sidebar-carousel.test.tsx 2>&1 | tail -10
```

Expected: FAIL — `sidebar-carousel` module not found.

- [ ] **Step 3: Create SidebarCarousel**

Create `web/src/components/layout/sidebar-carousel.tsx`:

```tsx
import { useEffect, useRef, Suspense } from 'react'
import { WorkspaceTree } from './workspace-tree'
import { ChatList } from '@/features/chats/components/chat-list'
import { FileExplorerTree } from '@/features/file-explorer/components/file-explorer-tree'
import { GitPanel } from '@/features/git/components/git-panel'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { SidebarSkeleton } from './SidebarSkeleton'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useSidebarStore, type SidebarTab } from '@/lib/store/sidebar'

const TABS: SidebarTab[] = ['workspaces', 'chats', 'files', 'git']

interface SidebarCarouselProps {
  activeWorkspaceRepoPath: string
}

export function SidebarCarousel({ activeWorkspaceRepoPath }: SidebarCarouselProps) {
  const activeTab = useSidebarStore(s => s.activeTab)
  const setActiveTab = useSidebarStore(s => s.setActiveTab)
  const files = useFileSystemStore(s => s.files)
  const handleFileOpen = useFileSystemStore.use.handleFileOpen?.()
  const handleFileSelect = useFileSystemStore.use.handleFileSelect?.()
  const containerRef = useRef<HTMLDivElement>(null)
  const isScrollingProgrammatically = useRef(false)

  // Scroll to the correct panel when activeTab changes (e.g. tab bar click)
  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const index = TABS.indexOf(activeTab)
    if (index === -1) return
    isScrollingProgrammatically.current = true
    el.scrollTo({ left: index * el.clientWidth, behavior: 'smooth' })
    // Reset flag after animation (~300ms)
    const id = setTimeout(() => { isScrollingProgrammatically.current = false }, 350)
    return () => clearTimeout(id)
  }, [activeTab])

  // Sync activeTab when user swipes
  function handleScroll() {
    if (isScrollingProgrammatically.current) return
    const el = containerRef.current
    if (!el) return
    const index = Math.round(el.scrollLeft / el.clientWidth)
    const tab = TABS[index]
    if (tab && tab !== useSidebarStore.getState().activeTab) {
      setActiveTab(tab)
    }
  }

  return (
    <div
      ref={containerRef}
      onScroll={handleScroll}
      className="flex flex-1 overflow-x-scroll overflow-y-hidden [scroll-snap-type:x_mandatory] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
    >
      {/* Workspaces panel */}
      <div className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden">
        <WorkspaceTree />
      </div>

      {/* Chats panel */}
      <div className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden">
        <ChatList />
      </div>

      {/* Files panel */}
      <div className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden">
        <ErrorBoundary>
          <Suspense fallback={<SidebarSkeleton />}>
            <FileExplorerTree
              files={files}
              rootFolderPath={activeWorkspaceRepoPath}
              onFileSelect={(path, isDir) => {
                if (isDir) {
                  useFileTreeStore.getState().toggleFolder(path)
                } else {
                  handleFileSelect?.(path, false)
                }
              }}
              onFileOpen={handleFileOpen ? (path: string, isDir: boolean) => {
                if (!isDir) void handleFileOpen(path, false)
              } : undefined}
              onCreateNewFileInDirectory={() => {}}
            />
          </Suspense>
        </ErrorBoundary>
      </div>

      {/* Git panel */}
      <div className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden">
        <GitPanel />
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Run tests**

```bash
cd web && npx vitest run src/__tests__/components/layout/sidebar-carousel.test.tsx 2>&1 | tail -10
```

Expected: all pass.

- [ ] **Step 5: Delete old SidebarTabs**

```bash
rm web/src/components/layout/SidebarTabs.tsx
```

- [ ] **Step 6: Commit**

```bash
git add web/src/components/layout/sidebar-carousel.tsx web/src/__tests__/components/layout/sidebar-carousel.test.tsx
git rm web/src/components/layout/SidebarTabs.tsx
git commit -m "feat(sidebar): add SidebarCarousel with 4 panels + swipe support, remove SidebarTabs"
```

---

## Task 4: Wire IDEShell

Replace the old `SidebarNavIcons` + `SidebarTabs` with `SidebarTabBar` + `SidebarCarousel` in `IDEShell.tsx`. Also delete the stale test file for `sidebar-nav-icons`.

**Files:**
- Modify: `web/src/components/layout/IDEShell.tsx`
- Delete: `web/src/__tests__/components/layout/sidebar-nav-icons.test.tsx` (if it exists)
- Delete: `web/src/__tests__/components/layout/sidebar-tabs.test.tsx`

- [ ] **Step 1: Update IDEShell**

In `web/src/components/layout/IDEShell.tsx`, replace imports and usage:

```tsx
// Remove these imports:
// import { SidebarTabs } from './SidebarTabs'
// import { SidebarNavIcons } from './sidebar-nav-icons'

// Add these imports:
import { SidebarTabBar } from './sidebar-tab-bar'
import { SidebarCarousel } from './sidebar-carousel'
```

Then in the `sidebarEl` JSX, replace:
```tsx
// Remove:
<div className="flex h-8 flex-shrink-0 items-center justify-center">
  <SidebarNavIcons />
</div>
<ErrorBoundary>
  <SidebarTabs activeWorkspaceRepoPath={activeWorkspaceRepoPath} />
</ErrorBoundary>

// Replace with:
<SidebarTabBar />
<ErrorBoundary>
  <SidebarCarousel activeWorkspaceRepoPath={activeWorkspaceRepoPath} />
</ErrorBoundary>
```

- [ ] **Step 2: Delete stale test files**

```bash
rm -f web/src/__tests__/components/layout/sidebar-nav-icons.test.tsx
rm -f web/src/__tests__/components/layout/sidebar-tabs.test.tsx
```

- [ ] **Step 3: TypeScript check**

```bash
cd web && npx tsc --noEmit 2>&1 | grep -v "branch-review\|branchReview" | head -20
```

Expected: no errors in IDEShell, sidebar-tab-bar, or sidebar-carousel.

- [ ] **Step 4: Run layout tests**

```bash
cd web && npx vitest run src/__tests__/components/layout 2>&1 | tail -20
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/IDEShell.tsx
git rm -f web/src/__tests__/components/layout/sidebar-nav-icons.test.tsx web/src/__tests__/components/layout/sidebar-tabs.test.tsx
git commit -m "feat(sidebar): wire SidebarTabBar + SidebarCarousel into IDEShell"
```

---

## Task 5: Build ChatListItem + ChatList

The `ProjectChat` type (`{ id: string; title: string; age: string }`) already exists in the sidebar store, as do `chats`, `addChat`, and `deleteChat`.

**Files:**
- Create: `web/src/features/chats/components/chat-list-item.tsx`
- Create: `web/src/features/chats/components/chat-list.tsx`
- Create: `web/src/__tests__/features/chats/components/chat-list.test.tsx`

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/chats/components/chat-list.test.tsx`:

```tsx
import React from 'react'
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import { ChatList } from '@/features/chats/components/chat-list'
import { useSidebarStore } from '@/lib/store/sidebar'

describe('ChatList', () => {
  beforeEach(() => {
    useSidebarStore.setState((useSidebarStore as any).getInitialState())
  })

  it('renders a row for each chat', () => {
    useSidebarStore.setState({
      chats: [
        { id: '1', title: 'Architecture review', age: '5m' },
        { id: '2', title: 'Performance bottlenecks', age: '2h' },
      ],
    })
    render(<ChatList />)
    expect(screen.getByText('Architecture review')).toBeInTheDocument()
    expect(screen.getByText('Performance bottlenecks')).toBeInTheDocument()
    expect(screen.getByText('5m')).toBeInTheDocument()
  })

  it('renders a "+ New" button', () => {
    render(<ChatList />)
    expect(screen.getByRole('button', { name: /new/i })).toBeInTheDocument()
  })

  it('adds a chat when "+ New" is clicked', () => {
    render(<ChatList />)
    fireEvent.click(screen.getByRole('button', { name: /new/i }))
    expect(useSidebarStore.getState().chats).toHaveLength(1)
  })
})
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd web && npx vitest run src/__tests__/features/chats 2>&1 | tail -10
```

Expected: FAIL — `chat-list` module not found.

- [ ] **Step 3: Create ChatListItem**

Create `web/src/features/chats/components/chat-list-item.tsx`:

```tsx
import { cn } from '@/utils/cn'
import { ROW_BASE } from '@/components/layout/workspace-row-base'
import type { ProjectChat } from '@/lib/store/sidebar'

interface ChatListItemProps {
  chat: ProjectChat
  isActive: boolean
  onClick: () => void
}

export function ChatListItem({ chat, isActive, onClick }: ChatListItemProps) {
  return (
    <button
      type="button"
      className={cn(
        ROW_BASE,
        isActive
          ? 'border-background bg-background text-foreground shadow-xs shadow-black/10 not-disabled:inset-shadow-[0_1px_--theme(--color-white/16%)]'
          : 'border-transparent text-foreground hover:bg-accent',
      )}
      onClick={onClick}
    >
      <span
        className={cn(
          'size-1.5 shrink-0 rounded-full',
          isActive ? 'bg-green-400' : 'bg-muted-foreground/40',
        )}
      />
      <span className="min-w-0 flex-1 truncate text-left text-[13px]">{chat.title}</span>
      <span className="shrink-0 font-mono text-[11px] text-muted-foreground">{chat.age}</span>
    </button>
  )
}
```

- [ ] **Step 4: Create ChatList**

Create `web/src/features/chats/components/chat-list.tsx`:

```tsx
import { nanoid } from 'nanoid'
import { Plus } from '@phosphor-icons/react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { cn } from '@/utils/cn'
import { ROW_BASE } from '@/components/layout/workspace-row-base'
import { useSidebarStore } from '@/lib/store/sidebar'
import { ChatListItem } from './chat-list-item'

export function ChatList() {
  const chats = useSidebarStore(s => s.chats)
  const addChat = useSidebarStore(s => s.addChat)

  function handleNew() {
    addChat({ id: nanoid(), title: 'New chat', age: 'just now' })
  }

  return (
    <ScrollArea className="flex-1">
      <div className="py-1">
        {chats.map(chat => (
          <ChatListItem
            key={chat.id}
            chat={chat}
            isActive={false}
            onClick={() => {}}
          />
        ))}
        <button
          type="button"
          aria-label="New chat"
          className={cn(ROW_BASE, 'border-transparent text-muted-foreground/50 hover:bg-accent hover:text-muted-foreground')}
          onClick={handleNew}
        >
          <Plus size={14} className="shrink-0" />
          <span className="text-[13px]">New</span>
        </button>
      </div>
    </ScrollArea>
  )
}
```

- [ ] **Step 5: Run tests**

```bash
cd web && npx vitest run src/__tests__/features/chats 2>&1 | tail -10
```

Expected: all 3 pass.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/chats/components/chat-list-item.tsx web/src/features/chats/components/chat-list.tsx web/src/__tests__/features/chats/components/chat-list.test.tsx
git commit -m "feat(chats): add ChatList and ChatListItem sidebar panel"
```

---

## Task 6: Add filter prop to FileExplorerTree

The `FileExplorerTree` already has `useGitStore` imported and builds git status decorations. Add a `filter?: 'all' | 'changed'` prop that, when `'changed'`, hides files with no git status decoration.

**Files:**
- Modify: `web/src/features/file-explorer/file-explorer/components/file-explorer-tree.tsx`

- [ ] **Step 1: Add the prop to the interface**

In `web/src/features/file-explorer/file-explorer/components/file-explorer-tree.tsx`, add `filter` to `FileExplorerTreeProps` (around line 71):

```ts
interface FileExplorerTreeProps {
  files: FileEntry[]
  filter?: 'all' | 'changed'   // add this line
  activePath?: string
  // ... rest unchanged
```

- [ ] **Step 2: Thread filter through the component function**

In the `FileExplorerTreeComponent` function signature (around line 108), add `filter = 'all' as const` to the destructured props:

```ts
function FileExplorerTreeComponent({
  files,
  filter = 'all',
  activePath,
  // ... rest unchanged
```

- [ ] **Step 3: Apply the filter to visible rows**

The component builds `visibleRows` via hooks. Find where `visibleRows` (or the row list) is rendered and wrap each item's visibility check. Look for the render of `FileExplorerTreeItem` rows and add a guard:

```ts
// In the rows render loop, add a filter guard before rendering:
// If filter === 'changed', only render rows that have a non-null git status decoration
const rowsToRender = filter === 'changed'
  ? visibleRows.filter(row => {
      const decoration = getFileTreeEntryGitStatusDecoration(row.entry, gitStatusLookup)
      return decoration !== null
    })
  : visibleRows
```

Then use `rowsToRender` instead of `visibleRows` in the render output.

> **Note:** Read the full `FileExplorerTreeComponent` function to find the exact variable names for `visibleRows` and where it's rendered before making this change. The `getFileTreeEntryGitStatusDecoration` and `gitStatusLookup` are already computed in the component.

- [ ] **Step 4: Run TypeScript check**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "file-explorer-tree" | head -10
```

Expected: no errors.

- [ ] **Step 5: Run file-explorer tests**

```bash
cd web && npx vitest run src/__tests__/features/file-explorer 2>&1 | tail -10
```

Expected: all pass (filter defaults to `'all'`, no behavior change).

- [ ] **Step 6: Commit**

```bash
git add web/src/features/file-explorer/file-explorer/components/file-explorer-tree.tsx
git commit -m "feat(file-explorer): add filter prop to show only git-changed files"
```

---

## Task 7: Build GitHistoryList + GitPanel

The git log API: `GET /api/v0/git/log?repo=&limit=50` returns `Commit[]` where each `Commit` is `{ hash: string, shortHash: string, message: string, author: string, date: string }`.

**Files:**
- Create: `web/src/features/git/components/git-history-list.tsx`
- Create: `web/src/features/git/components/git-panel.tsx`
- Create: `web/src/__tests__/features/git/components/git-panel.test.tsx`

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/git/components/git-panel.test.tsx`:

```tsx
import React from 'react'
import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { GitPanel } from '@/features/git/components/git-panel'

vi.mock('@/features/file-explorer/components/file-explorer-tree', () => ({
  FileExplorerTree: () => <div data-testid="file-explorer" />,
}))
vi.mock('@/features/git/components/git-history-list', () => ({
  GitHistoryList: () => <div data-testid="git-history" />,
}))
vi.mock('@/features/file-system/controllers/store', () => ({
  useFileSystemStore: Object.assign(
    (sel: any) => sel({ files: [], handleFileOpen: null }),
    { use: { handleFileOpen: () => null } },
  ),
}))

describe('GitPanel', () => {
  it('renders Changes and History tabs', () => {
    render(<GitPanel />)
    expect(screen.getByRole('tab', { name: /changes/i })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /history/i })).toBeInTheDocument()
  })

  it('shows the file explorer in the Changes tab by default', () => {
    render(<GitPanel />)
    expect(screen.getByTestId('file-explorer')).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd web && npx vitest run src/__tests__/features/git/components/git-panel.test.tsx 2>&1 | tail -10
```

Expected: FAIL — module not found.

- [ ] **Step 3: Create GitHistoryList**

Create `web/src/features/git/components/git-history-list.tsx`:

```tsx
import { useEffect, useState } from 'react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { apiFetch } from '@/lib/api'

interface Commit {
  hash: string
  shortHash: string
  message: string
  author: string
  date: string
}

export function GitHistoryList() {
  const [commits, setCommits] = useState<Commit[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    apiFetch<Commit[]>('/api/v0/git/log?limit=50')
      .then(setCommits)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  if (loading) {
    return <div className="flex flex-1 items-center justify-center text-[13px] text-muted-foreground">Loading…</div>
  }

  return (
    <ScrollArea className="flex-1">
      <div className="py-1">
        {commits.map(commit => (
          <div key={commit.hash} className="flex items-start gap-2 px-3 py-1.5 hover:bg-accent rounded-md mx-1.5 cursor-pointer">
            <span className="mt-0.5 shrink-0 font-mono text-[11px] text-muted-foreground">{commit.shortHash}</span>
            <div className="min-w-0 flex-1">
              <p className="truncate text-[13px]">{commit.message}</p>
              <p className="text-[11px] text-muted-foreground">{commit.author} · {commit.date}</p>
            </div>
          </div>
        ))}
      </div>
    </ScrollArea>
  )
}
```

- [ ] **Step 4: Create GitPanel**

Create `web/src/features/git/components/git-panel.tsx`:

```tsx
import { Suspense } from 'react'
import { Tabs, TabsList, TabsTab, TabsPanel } from '@/components/ui/tabs'
import { FileExplorerTree } from '@/features/file-explorer/components/file-explorer-tree'
import { GitHistoryList } from './git-history-list'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { SidebarSkeleton } from '@/components/layout/SidebarSkeleton'

export function GitPanel() {
  const files = useFileSystemStore(s => s.files)
  const handleFileOpen = useFileSystemStore.use.handleFileOpen?.()

  return (
    <Tabs defaultValue="changes" className="flex flex-1 flex-col overflow-hidden">
      <div className="flex shrink-0 items-center px-3 py-1.5">
        <TabsList variant="default">
          <TabsTab value="changes">Changes</TabsTab>
          <TabsTab value="history">History</TabsTab>
        </TabsList>
      </div>

      <TabsPanel value="changes" className="flex flex-1 flex-col overflow-hidden">
        <Suspense fallback={<SidebarSkeleton />}>
          <FileExplorerTree
            files={files}
            filter="changed"
            onFileSelect={(path, isDir) => {
              if (isDir) useFileTreeStore.getState().toggleFolder(path)
            }}
            onFileOpen={handleFileOpen ? (path: string, isDir: boolean) => {
              if (!isDir) void handleFileOpen(path, false)
            } : undefined}
            onCreateNewFileInDirectory={() => {}}
          />
        </Suspense>
      </TabsPanel>

      <TabsPanel value="history" className="flex flex-1 flex-col overflow-hidden">
        <GitHistoryList />
      </TabsPanel>
    </Tabs>
  )
}
```

- [ ] **Step 5: Run tests**

```bash
cd web && npx vitest run src/__tests__/features/git/components/git-panel.test.tsx 2>&1 | tail -10
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/git/components/git-history-list.tsx web/src/features/git/components/git-panel.tsx web/src/__tests__/features/git/components/git-panel.test.tsx
git commit -m "feat(git): add GitPanel with Changes + History subtabs"
```

---

## Task 8: Move comment components from branch-review

`comment-composer.tsx` and `review-thread.tsx` depend on `../lib/markdown` (relative to `branch-review/components`). Move that file too and fix imports before the directory is deleted.

**Files:**
- Create: `web/src/features/panes/lib/markdown.tsx` (moved from `branch-review/lib/markdown.tsx`)
- Create: `web/src/features/panes/components/comment-composer.tsx` (moved, import path fixed)
- Create: `web/src/features/panes/components/review-thread.tsx` (moved, import path fixed)

- [ ] **Step 1: Move markdown lib**

```bash
mkdir -p web/src/features/panes/lib
cp web/src/features/branch-review/lib/markdown.tsx web/src/features/panes/lib/markdown.tsx
```

- [ ] **Step 2: Copy comment-composer with fixed import**

```bash
cp web/src/features/branch-review/components/comment-composer.tsx web/src/features/panes/components/comment-composer.tsx
```

Then in `web/src/features/panes/components/comment-composer.tsx`, update the import:

```ts
// Change:
import { transparentMarkdownTheme, MarkdownPreview } from '../lib/markdown'
// To:
import { transparentMarkdownTheme, MarkdownPreview } from '@/features/panes/lib/markdown'
```

- [ ] **Step 3: Copy review-thread with fixed import**

```bash
cp web/src/features/branch-review/components/review-thread.tsx web/src/features/panes/components/review-thread.tsx
```

Then in `web/src/features/panes/components/review-thread.tsx`, update the import:

```ts
// Change:
import { MarkdownPreview } from '../lib/markdown'
// To:
import { MarkdownPreview } from '@/features/panes/lib/markdown'
```

- [ ] **Step 4: TypeScript check on panes components**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "panes/components/comment\|panes/components/review\|panes/lib/markdown" | head -10
```

Expected: no errors in the moved files.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/panes/lib/markdown.tsx web/src/features/panes/components/comment-composer.tsx web/src/features/panes/components/review-thread.tsx
git commit -m "refactor(panes): move comment-composer, review-thread, markdown lib from branch-review"
```

---

## Task 9: Delete branch-review feature

Remove `features/branch-review/` entirely and clean up all references in the pane system.

**Files:**
- Delete: `web/src/features/branch-review/` (entire directory)
- Modify: `web/src/features/panes/types/pane-content.ts`
- Modify: `web/src/features/panes/components/pane-container.tsx`

- [ ] **Step 1: Remove BranchReviewContent from pane-content.ts**

In `web/src/features/panes/types/pane-content.ts`:

1. Remove `"branchReview"` from the `PaneContentType` union (line ~26)
2. Delete the `BranchReviewContent` interface (lines ~112-117)
3. Remove `| BranchReviewContent` from the `PaneContent` union (line ~132)
4. Delete the `isBranchReviewContent` type guard function
5. Remove `"branchReview"` from the `VIRTUAL_TYPES` set (line ~177)
6. Remove `| { type: "branchReview"; wsId: string; branchName: string; name: string; }` from `OpenContentSpec`

- [ ] **Step 2: Remove branchReview from pane-container.tsx**

In `web/src/features/panes/components/pane-container.tsx`:

1. Remove the `lazy` import of `BranchReviewPane` (lines ~36-40)
2. Remove `import type { BranchReviewContent, ... }` — remove `BranchReviewContent` from the destructured type imports (line ~23)
3. Remove the `case "branchReview":` block from `renderActiveBuffer` (lines ~465-472)

- [ ] **Step 3: Delete the branch-review directory**

```bash
rm -rf web/src/features/branch-review
```

- [ ] **Step 4: TypeScript check**

```bash
cd web && npx tsc --noEmit 2>&1 | head -30
```

Expected: no errors. If any remain, they'll be stray imports of `branch-review` symbols elsewhere — grep and fix them:

```bash
grep -r "branch-review\|branchReview\|BranchReview" web/src --include="*.ts" --include="*.tsx" -l | grep -v __tests__
```

Fix any remaining imports by removing or updating them.

- [ ] **Step 5: Run full test suite**

```bash
cd web && npm test 2>&1 | tail -30
```

Expected: all tests pass. Any test files that imported `branch-review` symbols will need their imports removed or updated.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(sidebar): delete branch-review feature, clean up pane-content types"
```

---

## Task 10: Wire inline comments into DiffPane

The existing `DiffPane` (`features/panes/components/diff-pane.tsx`) renders a `DiffViewer`. The comment components are now in `features/panes/components/`. This task wires them together.

> **Read first:** Before editing, read `web/src/features/git/components/diff/git-diff-viewer.tsx` in full to understand how the diff viewer renders lines and whether it exposes a hook or slot for inline UI at a specific line number.

**Files:**
- Modify: `web/src/features/panes/components/diff-pane.tsx`

- [ ] **Step 1: Read the DiffViewer**

```bash
cat web/src/features/git/components/diff/git-diff-viewer.tsx
```

Look for:
- Whether it renders individual diff lines with a known structure
- Whether there's an `onLineClick` or similar callback prop
- Whether inline content can be injected per-line

- [ ] **Step 2: Add inline comment support**

Based on what you find in Step 1, update `web/src/features/panes/components/diff-pane.tsx` to:

1. Import `CommentComposer` and `ReviewThread` from `./comment-composer` and `./review-thread`
2. Maintain a local state `commentTarget: { lineNumber: number } | null`
3. When a diff line is clicked (use whatever callback the DiffViewer exposes), set `commentTarget`
4. Render `CommentComposer` below the clicked line (or in an overlay if the viewer doesn't support inline injection)
5. On submit, add the comment to a local list and clear `commentTarget`

The exact implementation depends on DiffViewer's API. If it exposes no line-click callback, wrap each rendered line to add one.

- [ ] **Step 3: TypeScript check**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "diff-pane" | head -10
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add web/src/features/panes/components/diff-pane.tsx
git commit -m "feat(panes): wire inline comment composer into DiffPane"
```

---

## Task 11: Final TypeScript + test sweep

Verify the whole codebase compiles and all tests pass before closing the PR.

**Files:** no new files — verification only.

- [ ] **Step 1: Full TypeScript check**

```bash
cd web && npx tsc --noEmit 2>&1
```

Expected: zero errors.

- [ ] **Step 2: Full test suite**

```bash
cd web && npm test 2>&1 | tail -40
```

Expected: all tests pass, no skipped tests from this refactor.

- [ ] **Step 3: Check for dead imports**

```bash
grep -r "SidebarNavIcons\|SidebarTabs\|sidebar-nav-icons\|branch-review" web/src --include="*.ts" --include="*.tsx" | grep -v __tests__ | grep -v ".test."
```

Expected: no output.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "chore(sidebar): final cleanup — verify zero TS errors and all tests pass"
```

---

## Self-Review Notes

- **Task 6 (FileExplorerTree filter)** requires reading the full component to find exact variable names before editing. The step explicitly calls this out.
- **Task 8 (move comment components)** copies files and fixes relative imports — the branch-review directory still exists at this point, which is intentional (deletion is Task 9).
- **Task 9 (delete branch-review)** must come after Task 8 (moved files) and after Tasks 2-4 (no more SidebarNavIcons/SidebarTabs imports).
- Task ordering is strict: 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9 → 10 → 11. Each task depends on the previous.
- **Task 10 (DiffPane inline comments)** is exploratory — it requires reading `git-diff-viewer.tsx` before writing code. The exact API of that component determines the implementation approach.
