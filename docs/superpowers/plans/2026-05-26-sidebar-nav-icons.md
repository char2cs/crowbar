# Sidebar Nav Icons Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the "Workspaces | Files | Git" text tab bar with icon buttons that live in the 38px traffic-light strip, flush with the editor's file tab bar.

**Architecture:** A new `SidebarNavIcons` component renders inside the sidebar's existing 38px top strip — icons anchor to the sidebar edge with platform-aware insets, the `TabsList` is removed from `SidebarTabs` (panels stay), and the tab-bar height is bumped from 36px to 38px/28px so both strips flush-align. No new wrappers, no `ResizablePanelGroup` changes.

**Tech Stack:** React 18, Zustand (`useSidebarStore`), `@phosphor-icons/react`, Radix Tooltip (`@/components/ui/tooltip`), Tailwind CSS, `IS_MAC`/`IS_WINDOWS` from `@/utils/platform`, Vitest + Testing Library.

---

### Task 1: SidebarNavIcons component

**Files:**
- Create: `web/src/components/layout/sidebar-nav-icons.tsx`
- Create: `web/src/__tests__/components/layout/sidebar-nav-icons.test.tsx`

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/components/layout/sidebar-nav-icons.test.tsx`:

```tsx
import React from 'react'
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { SidebarNavIcons } from '@/components/layout/sidebar-nav-icons'
import { useSidebarStore } from '@/lib/store/sidebar'

vi.mock('@/components/ui/tooltip', () => ({
  default: ({ children, content }: { children: React.ReactNode; content: string }) => (
    <div data-tooltip={content}>{children}</div>
  ),
}))

vi.mock('@/features/settings/store', () => ({
  useSettingsStore: vi.fn((selector: (s: { settings: { sidebarPosition: string } }) => unknown) =>
    selector({ settings: { sidebarPosition: 'left' } }),
  ),
}))

vi.mock('@/utils/platform', () => ({
  IS_MAC: true,
  IS_WINDOWS: false,
  IS_LINUX: false,
}))

describe('SidebarNavIcons', () => {
  beforeEach(() => {
    useSidebarStore.setState({ activeTab: 'workspaces' })
  })

  it('renders three icon buttons', () => {
    render(<SidebarNavIcons />)
    expect(screen.getByRole('button', { name: 'Workspaces' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Files' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Git' })).toBeInTheDocument()
  })

  it('active tab button has aria-pressed true', () => {
    render(<SidebarNavIcons />)
    expect(screen.getByRole('button', { name: 'Workspaces' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: 'Files' })).toHaveAttribute('aria-pressed', 'false')
    expect(screen.getByRole('button', { name: 'Git' })).toHaveAttribute('aria-pressed', 'false')
  })

  it('active button has bg-accent class', () => {
    render(<SidebarNavIcons />)
    expect(screen.getByRole('button', { name: 'Workspaces' })).toHaveClass('bg-accent')
    expect(screen.getByRole('button', { name: 'Files' })).not.toHaveClass('bg-accent')
  })

  it('clicking a button sets the active tab in the store', () => {
    render(<SidebarNavIcons />)
    fireEvent.click(screen.getByRole('button', { name: 'Files' }))
    expect(useSidebarStore.getState().activeTab).toBe('files')
  })

  it('clicking the git button sets active tab to git', () => {
    render(<SidebarNavIcons />)
    fireEvent.click(screen.getByRole('button', { name: 'Git' }))
    expect(useSidebarStore.getState().activeTab).toBe('git')
  })

  it('each button has a tooltip wrapping it', () => {
    render(<SidebarNavIcons />)
    expect(screen.getByRole('button', { name: 'Workspaces' }).closest('[data-tooltip="Workspaces"]')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Files' }).closest('[data-tooltip="Files"]')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Git' }).closest('[data-tooltip="Git"]')).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
cd web && npx vitest run src/__tests__/components/layout/sidebar-nav-icons.test.tsx
```

Expected: FAIL — `Cannot find module '@/components/layout/sidebar-nav-icons'`

- [ ] **Step 3: Create the component**

Create `web/src/components/layout/sidebar-nav-icons.tsx`:

```tsx
import { SquaresFour, FolderOpen, GitBranch } from '@phosphor-icons/react'
import { useSidebarStore, type SidebarTab } from '@/lib/store/sidebar'
import { useSettingsStore } from '@/features/settings/store'
import { IS_MAC, IS_WINDOWS } from '@/utils/platform'
import Tooltip from '@/components/ui/tooltip'
import { cn } from '@/utils/cn'

const NAV_ITEMS = [
  { tab: 'workspaces' as SidebarTab, label: 'Workspaces', Icon: SquaresFour },
  { tab: 'files'      as SidebarTab, label: 'Files',      Icon: FolderOpen  },
  { tab: 'git'        as SidebarTab, label: 'Git',        Icon: GitBranch   },
] as const

export function SidebarNavIcons() {
  const activeTab       = useSidebarStore(s => s.activeTab)
  const setActiveTab    = useSidebarStore(s => s.setActiveTab)
  const sidebarPosition = useSettingsStore(s => s.settings.sidebarPosition)

  // Icons always anchor to the sidebar edge.
  // macOS traffic lights (~80 px) occupy the left side of the window — only
  // relevant when the sidebar is on the left.
  // Windows native chrome (~138 px) occupies the right side — only relevant
  // when the sidebar is on the right.
  const groupClass = cn(
    'flex items-center gap-1',
    sidebarPosition === 'right'
      ? (IS_WINDOWS ? 'mr-[138px]' : 'mr-2')
      : (IS_MAC     ? 'ml-[80px]'  : 'ml-2'),
  )

  return (
    <div className={groupClass}>
      {NAV_ITEMS.map(({ tab, label, Icon }) => (
        <Tooltip key={tab} content={label} side="bottom">
          <button
            onClick={() => setActiveTab(tab)}
            aria-label={label}
            aria-pressed={activeTab === tab}
            className={cn(
              'flex h-7 w-7 items-center justify-center rounded-md transition-colors',
              activeTab === tab
                ? 'bg-accent text-foreground'
                : 'text-muted-foreground hover:text-foreground',
            )}
          >
            <Icon size={15} weight={activeTab === tab ? 'fill' : 'regular'} />
          </button>
        </Tooltip>
      ))}
    </div>
  )
}
```

- [ ] **Step 4: Run the test to confirm it passes**

```bash
cd web && npx vitest run src/__tests__/components/layout/sidebar-nav-icons.test.tsx
```

Expected: PASS — 6 tests passing.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/sidebar-nav-icons.tsx \
        web/src/__tests__/components/layout/sidebar-nav-icons.test.tsx
git commit -m "feat: add SidebarNavIcons component with icon buttons and tooltips"
```

---

### Task 2: Wire SidebarNavIcons into IDEShell

**Files:**
- Modify: `web/src/components/layout/IDEShell.tsx:44-48`
- Modify: `web/src/__tests__/components/layout/IDEShell.test.tsx`

- [ ] **Step 1: Update the IDEShell test to expect SidebarNavIcons**

Open `web/src/__tests__/components/layout/IDEShell.test.tsx`. Add the mock and assertion for `SidebarNavIcons`. The full file should be:

```tsx
// web/src/__tests__/components/layout/IDEShell.test.tsx
import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import React from 'react'
import { IDEShell } from '@/components/layout/IDEShell'

vi.mock('@/features/workspace/components/WorkspaceView', () => ({
  WorkspaceView: () => <div data-testid="workspace-view" />,
}))
vi.mock('@/components/layout/SidebarTabs', () => ({
  SidebarTabs: () => <div data-testid="sidebar-tabs" />,
}))
vi.mock('@/components/layout/SidebarHeader', () => ({
  SidebarHeader: () => <div data-testid="sidebar-header" />,
}))
vi.mock('@/components/layout/sidebar-nav-icons', () => ({
  SidebarNavIcons: () => <div data-testid="sidebar-nav-icons" />,
}))
vi.mock('@/features/settings/components/settings-dialog', () => ({
  default: () => null,
}))
vi.mock('@/lib/store/sidebar', () => ({
  useSidebarStore: () => ({
    chats: [],
    repos: [],
    collapsedRepos: new Set(),
    addChat: vi.fn(),
    deleteChat: vi.fn(),
    deleteWorkspace: vi.fn(),
    toggleRepo: vi.fn(),
  }),
}))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useRouterState: () => ({ location: { pathname: '/' } }),
  Outlet: () => <div data-testid="outlet" />,
}))
vi.mock('@/components/ui/resizable', () => ({
  ResizablePanelGroup: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ResizablePanel: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ResizableHandle: () => null,
}))
vi.mock('@/components/ErrorBoundary', () => ({
  ErrorBoundary: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))
vi.mock('@/components/ui/sonner', () => ({
  Toaster: () => null,
}))
vi.mock('@/features/settings/components/font-style-injector', () => ({
  FontStyleInjector: () => null,
}))
vi.mock('@/features/terminal/components/terminal-host', () => ({
  TerminalHost: () => null,
}))

describe('IDEShell', () => {
  it('renders sidebar header', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('sidebar-header')).toBeInTheDocument()
  })

  it('renders SidebarTabs', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('sidebar-tabs')).toBeInTheDocument()
  })

  it('renders SidebarNavIcons', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('sidebar-nav-icons')).toBeInTheDocument()
  })

  it('renders Outlet when no workspace or chat is active', () => {
    render(<IDEShell />)
    expect(screen.getByTestId('outlet')).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run to confirm new test fails**

```bash
cd web && npx vitest run src/__tests__/components/layout/IDEShell.test.tsx
```

Expected: FAIL — `renders SidebarNavIcons` fails because IDEShell doesn't render it yet.

- [ ] **Step 3: Update IDEShell.tsx**

Open `web/src/components/layout/IDEShell.tsx`.

Add imports after the existing layout imports:

```tsx
import { SidebarNavIcons } from './sidebar-nav-icons'
import { IS_MAC } from '@/utils/platform'
```

The outer sidebar wrapper currently carries `border-r border-border` on the whole column, which would cut a vertical line through the unified bar. Move that border to a wrapper around `SidebarHeader` + `SidebarTabs` only.

Replace the entire sidebar panel inner div (line ~46 through the closing `</div>` before `</ResizablePanel>`):

```tsx
<div className="flex h-full flex-col overflow-hidden bg-chrome-bg backdrop-blur-sm">
  {/* Unified titlebar strip — no border here so both strips read as one bar */}
  <div
    className={cn('flex w-full flex-shrink-0 items-center', IS_MAC ? 'h-[38px]' : 'h-[28px]')}
    data-tauri-drag-region
  >
    <SidebarNavIcons />
  </div>
  {/* Sidebar content — border starts here, below the unified strip */}
  <div className={cn(
    'flex flex-1 flex-col overflow-hidden',
    sidebarPosition === 'right' ? 'border-l border-border' : 'border-r border-border',
  )}>
    <ErrorBoundary>
      <SidebarHeader
        userInitials="MU"
        onProjectsClick={() => void navigate({ to: '/projects' })}
        onProjectSelect={() => void navigate({ to: '/' })}
        onSettingsClick={() => setSettingsOpen(true)}
      />
      <SidebarTabs
        chats={chats}
        repos={repos}
        collapsedRepos={collapsedRepos}
        activeChatId={activeChatId}
        activeWorkspaceId={activeWorkspaceId}
        activeWorkspaceRepoPath={activeWorkspaceRepoPath}
        onChatClick={id => void navigate({ to: '/chat/$chatId', params: { chatId: id } })}
        onWorkspaceClick={(_repoId, wsId) => void navigate({ to: '/workspaces/$wsId', params: { wsId } })}
        onNewChat={() => {
          const chat = createMockChat()
          addChat({ id: chat.id, title: chat.title, age: chat.age })
          void navigate({ to: '/chat/$chatId', params: { chatId: chat.id } })
        }}
        onNewWorkspace={() => void navigate({ to: '/workspaces/new' })}
        onDeleteChat={id => { deleteChat(id); if (activeChatId === id) void navigate({ to: '/' }) }}
        onDeleteWorkspace={wsId => {
          deleteWorkspace(wsId)
          destroyWorkspaceStore(wsId)
          if (activeWorkspaceId === wsId) void navigate({ to: '/' })
        }}
        onRepoToggle={toggleRepo}
      />
    </ErrorBoundary>
  </div>
</div>
```

- [ ] **Step 4: Run tests to confirm all pass**

```bash
cd web && npx vitest run src/__tests__/components/layout/IDEShell.test.tsx
```

Expected: PASS — 4 tests passing.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/IDEShell.tsx \
        web/src/__tests__/components/layout/IDEShell.test.tsx
git commit -m "feat: wire SidebarNavIcons into IDEShell titlebar strip"
```

---

### Task 3: Remove TabsList from SidebarTabs

**Files:**
- Modify: `web/src/components/layout/SidebarTabs.tsx:47-66`
- Create: `web/src/__tests__/components/layout/sidebar-tabs.test.tsx`

The `SidebarNavIcons` now owns tab switching. `SidebarTabs` only needs to render the correct panel. The `<Tabs>` wrapper stays so `TabsContent value=` matching still works semantically, but `<TabsList>` and all three `<TabsTrigger>` elements are removed.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/components/layout/sidebar-tabs.test.tsx`:

```tsx
import React from 'react'
import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { SidebarTabs } from '@/components/layout/SidebarTabs'
import { useSidebarStore } from '@/lib/store/sidebar'

vi.mock('@/components/layout/WorkspacesSidebarPanel', () => ({
  WorkspacesSidebarPanel: () => <div data-testid="workspaces-panel" />,
}))
vi.mock('@/features/file-explorer/components/file-explorer-tree', () => ({
  FileExplorerTree: () => <div data-testid="files-panel" />,
}))
vi.mock('@/features/git/components/git-view', () => ({
  default: () => <div data-testid="git-panel" />,
}))
vi.mock('@/components/ErrorBoundary', () => ({
  ErrorBoundary: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))
vi.mock('@/components/layout/SidebarSkeleton', () => ({
  SidebarSkeleton: () => null,
}))
vi.mock('@/lib/mock/files', () => ({
  getMockFileTree: () => [],
}))
vi.mock('@/features/file-system/controllers/store', () => ({
  useFileSystemStore: { use: { handleFileOpen: () => undefined, handleFileSelect: () => undefined } },
}))
vi.mock('@/features/file-explorer/stores/file-explorer-tree-store', () => ({
  useFileTreeStore: { getState: () => ({ toggleFolder: vi.fn() }) },
}))

const defaultProps = {
  chats: [],
  repos: [],
  collapsedRepos: new Set<string>(),
  activeChatId: undefined,
  activeWorkspaceId: undefined,
  activeWorkspaceRepoPath: '/repos/default',
}

describe('SidebarTabs', () => {
  it('does not render text tab triggers', () => {
    useSidebarStore.setState({ activeTab: 'workspaces' })
    render(<SidebarTabs {...defaultProps} />)
    expect(screen.queryByRole('tab', { name: 'Workspaces' })).not.toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: 'Files' })).not.toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: 'Git' })).not.toBeInTheDocument()
  })

  it('shows workspaces panel when activeTab is workspaces', () => {
    useSidebarStore.setState({ activeTab: 'workspaces' })
    render(<SidebarTabs {...defaultProps} />)
    expect(screen.getByTestId('workspaces-panel')).toBeVisible()
  })

  it('shows files panel when activeTab is files', () => {
    useSidebarStore.setState({ activeTab: 'files' })
    render(<SidebarTabs {...defaultProps} />)
    expect(screen.getByTestId('files-panel')).toBeVisible()
  })

  it('shows git panel when activeTab is git', () => {
    useSidebarStore.setState({ activeTab: 'git' })
    render(<SidebarTabs {...defaultProps} />)
    expect(screen.getByTestId('git-panel')).toBeVisible()
  })
})
```

- [ ] **Step 2: Run to confirm current behavior**

```bash
cd web && npx vitest run src/__tests__/components/layout/sidebar-tabs.test.tsx
```

Expected: `does not render text tab triggers` FAILS (triggers exist today). The panel visibility tests should PASS. Note which tests fail before proceeding.

- [ ] **Step 3: Remove TabsList from SidebarTabs.tsx**

In `web/src/components/layout/SidebarTabs.tsx`, delete the entire `<TabsList>` block — lines 47–66:

```tsx
      <TabsList className="h-9 w-full justify-start gap-0 rounded-none border-b border-border bg-transparent px-0">
        <TabsTrigger
          value="workspaces"
          className="flex-1 rounded-none border-b-2 border-transparent px-3 py-1.5 text-[12px] text-muted-foreground data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:text-foreground data-[state=active]:shadow-none"
        >
          Workspaces
        </TabsTrigger>
        <TabsTrigger
          value="files"
          className="flex-1 rounded-none border-b-2 border-transparent px-3 py-1.5 text-[12px] text-muted-foreground data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:text-foreground data-[state=active]:shadow-none"
        >
          Files
        </TabsTrigger>
        <TabsTrigger
          value="git"
          className="flex-1 rounded-none border-b-2 border-transparent px-3 py-1.5 text-[12px] text-muted-foreground data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:text-foreground data-[state=active]:shadow-none"
        >
          Git
        </TabsTrigger>
      </TabsList>
```

Also remove the now-unused imports at the top:

```tsx
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
```

Becomes:

```tsx
import { Tabs, TabsContent } from '@/components/ui/tabs'
```

- [ ] **Step 4: Run tests to confirm all pass**

```bash
cd web && npx vitest run src/__tests__/components/layout/sidebar-tabs.test.tsx
```

Expected: PASS — 4 tests passing.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/SidebarTabs.tsx \
        web/src/__tests__/components/layout/sidebar-tabs.test.tsx
git commit -m "feat: remove text TabsList from SidebarTabs — nav icons drive tab switching"
```

---

### Task 4: Fix tab-bar height to flush-align with sidebar strip

**Files:**
- Modify: `web/src/features/tabs/components/tab-bar.tsx:720`

The pane tab bar is `h-9` (36px). It must match the sidebar strip height: `h-[38px]` on macOS, `h-[28px]` on Windows/Linux. `IS_MAC` is not yet imported in this file.

- [ ] **Step 1: Confirm `h-9` is present before the change**

```bash
grep -n "h-9" web/src/features/tabs/components/tab-bar.tsx
```

Expected: one line printed containing `h-9` inside the container `className` string. Note the line number.

- [ ] **Step 3: Update tab-bar.tsx**

In `web/src/features/tabs/components/tab-bar.tsx`, add `IS_MAC` to the platform import. Find the existing import of platform utilities — if none exists, add:

```tsx
import { IS_MAC } from '@/utils/platform'
```

Then find line ~720 (the container `className` that includes `h-9`):

```tsx
className="relative flex h-9 shrink-0 items-center gap-1.5 overflow-hidden bg-chrome-bg backdrop-blur-sm px-2 py-1"
```

Replace with:

```tsx
className={cn(
  'relative flex shrink-0 items-center gap-1.5 overflow-hidden bg-chrome-bg backdrop-blur-sm px-2 py-1',
  IS_MAC ? 'h-[38px]' : 'h-[28px]',
)}
```

Ensure `cn` is already imported — it will be, since the file uses Tailwind merging throughout. If not, add:

```tsx
import { cn } from '@/utils/cn'
```

- [ ] **Step 4: Confirm `h-9` is gone and `h-[38px]` is present**

```bash
grep -n "h-9\|h-\[38px\]\|h-\[28px\]" web/src/features/tabs/components/tab-bar.tsx
```

Expected: one line showing `h-[38px]` and `h-[28px]`, no line showing bare `h-9` in the container className.

- [ ] **Step 5: Run full test suite to confirm no regressions**

```bash
cd web && npx vitest run
```

Expected: all tests pass. If any test fails due to the `h-9` → `h-[38px]` change (snapshot or class assertion), update that test to expect `h-[38px]`.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/tabs/components/tab-bar.tsx \
        web/src/__tests__/features/tabs/components/tab-bar-height.test.tsx
git commit -m "feat: align tab-bar height with sidebar strip (38px macOS / 28px Win-Linux)"
```

---

### Task 5: Final verification

- [ ] **Step 1: Run full test suite one more time**

```bash
cd web && npx vitest run
```

Expected: all tests pass.

- [ ] **Step 2: Verify no stale `h-9` or `TabsList`/`TabsTrigger` references remain**

```bash
grep -rn "TabsList\|TabsTrigger" web/src/components/layout/SidebarTabs.tsx
# Expected: no output

grep -n "h-9" web/src/features/tabs/components/tab-bar.tsx
# Expected: no output (all occurrences replaced)
```

- [ ] **Step 3: Commit if anything was missed, otherwise done**

```bash
git status
# Should be clean. If not, fix and commit.
```
