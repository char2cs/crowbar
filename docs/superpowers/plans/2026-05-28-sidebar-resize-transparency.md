# Sidebar Resize + Transparency Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the sidebar background transparent and add drag-to-resize with localStorage persistence.

**Architecture:** Two surgical changes to `IDEShell.tsx`. Fix 1 overrides shadcn's hardcoded `bg-sidebar` on the sidebar's inner div via a Tailwind arbitrary-child selector. Fix 2 adds a `sidebarWidth` state (initialized from `localStorage`), passes `--sidebar-width` as an inline CSS variable override on `SidebarProvider`, and adds an absolutely-positioned drag handle inside `Sidebar` that updates width on `mousemove` via document-level listeners.

**Tech Stack:** React 18, Tailwind CSS v4 arbitrary variants, TypeScript, `localStorage`, Vitest (`bun run test`), `bunx tsc --noEmit`.

---

### Task 1: Transparent sidebar background

**Files:**
- Modify: `web/src/components/layout/IDEShell.tsx`

The shadcn `Sidebar` renders `<div data-slot="sidebar-inner" class="... bg-sidebar">` with a hardcoded background. The `className` prop on `<Sidebar>` is applied to the outer fixed container (`data-slot="sidebar-container"`), so a direct-child arbitrary selector can override the inner background.

No unit test is meaningful here (the Sidebar mock in tests drops `className`). Verified visually via Chrome MCP after commit.

- [ ] **Step 1: Add the transparent className to `<Sidebar>` in `IDEShell.tsx`**

Find (exact text, around line 30):
```tsx
      <Sidebar side={sidebarPosition} collapsible="offcanvas">
```

Replace with:
```tsx
      <Sidebar side={sidebarPosition} collapsible="offcanvas" className="[&>[data-slot=sidebar-inner]]:bg-transparent">
```

- [ ] **Step 2: Type-check**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && bunx tsc --noEmit 2>&1 | head -20
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/components/layout/IDEShell.tsx
git commit -m "fix: make sidebar background transparent"
```

---

### Task 2: Resizable sidebar with drag handle + localStorage persistence

**Files:**
- Modify: `web/src/components/layout/IDEShell.tsx`
- Modify: `web/src/__tests__/components/layout/IDEShell.test.tsx`

The drag handle is a thin absolutely-positioned div at the sidebar edge. A mousedown handler captures the initial cursor X and width, then attaches document-level `mousemove`/`mouseup` listeners so the drag continues even when the pointer leaves the handle. Width is clamped to `[192, 640]` px and written to `localStorage` on every move.

`SidebarProvider` accepts a `style` prop that is spread after its own `--sidebar-width: 16rem` assignment, so our override wins.

- [ ] **Step 1: Write the failing test**

Open `web/src/__tests__/components/layout/IDEShell.test.tsx`. Add this test at the end of the `describe` block (or after the existing tests):

```tsx
it('renders a resize handle inside the sidebar', () => {
  render(<IDEShell />)
  expect(screen.getByTestId('sidebar-resize-handle')).toBeInTheDocument()
})
```

`screen` is already imported (it's used in existing tests). `IDEShell` is already imported. `render` is already imported.

- [ ] **Step 2: Run the test to confirm it fails**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && bun run test src/__tests__/components/layout/IDEShell.test.tsx 2>&1 | tail -15
```

Expected: 1 failure — `Unable to find an element by: [data-testid="sidebar-resize-handle"]`.

- [ ] **Step 3: Implement the changes in `IDEShell.tsx`**

Replace the entire content of `web/src/components/layout/IDEShell.tsx` with:

```tsx
import { useState } from 'react'
import { Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import { SidebarProvider, Sidebar, SidebarInset } from '@/components/ui/sidebar'
import { SidebarHeader } from './SidebarHeader'
import { SidebarTabs } from './SidebarTabs'
import { SidebarNavIcons } from './sidebar-nav-icons'
import { IS_MAC } from '@/utils/platform'
import { useSidebarStore } from '@/lib/store/sidebar'
import { createMockChat } from '@/lib/mock/chats'
import { WorkspaceView } from '@/features/workspace/components/WorkspaceView'
import SettingsDialog from '@/features/settings/components/settings-dialog'
import { TerminalHost } from '@/features/terminal/components/terminal-host'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { cn } from '@/utils/cn'
import { useSettingsStore } from '@/features/settings/store'
import { FontStyleInjector } from '@/features/settings/components/font-style-injector'
import { destroyWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { Toaster } from '@/components/ui/sonner'

export function IDEShell() {
  const navigate = useNavigate()
  const routerState = useRouterState()
  const pathname = routerState.location.pathname
  const { chats, repos, collapsedRepos, addChat, deleteChat, deleteWorkspace, toggleRepo } =
    useSidebarStore()
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [sidebarWidth, setSidebarWidth] = useState(
    () => parseInt(localStorage.getItem('sidebar-width') ?? '256', 10)
  )
  const sidebarPosition = useSettingsStore((state) => state.settings.sidebarPosition)

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1]
  const activeChatId = pathname.match(/\/chat\/([^/]+)/)?.[1]

  const activeRepo = repos.find(r => r.workspaces.some(ws => ws.id === activeWorkspaceId))
  const activeWorkspace = activeRepo?.workspaces.find(ws => ws.id === activeWorkspaceId)
  const workspaceLabel = activeWorkspace
    ? `${activeRepo?.name ?? ''} / ${activeWorkspace.branch}`
    : undefined

  const activeWorkspaceRepoPath = activeRepo ? `/repos/${activeRepo.id}` : '/repos/default'
  const chatTabLabel = chats.find(c => c.id === activeChatId)?.title ?? 'Chat'

  function handleResizeDragStart(e: React.MouseEvent) {
    e.preventDefault()
    const startX = e.clientX
    const startWidth = sidebarWidth
    const position = sidebarPosition

    function onMouseMove(e: MouseEvent) {
      const delta = position === 'left' ? e.clientX - startX : startX - e.clientX
      const next = Math.min(640, Math.max(192, startWidth + delta))
      setSidebarWidth(next)
      localStorage.setItem('sidebar-width', String(next))
    }

    function onMouseUp() {
      document.removeEventListener('mousemove', onMouseMove)
      document.removeEventListener('mouseup', onMouseUp)
    }

    document.addEventListener('mousemove', onMouseMove)
    document.addEventListener('mouseup', onMouseUp)
  }

  return (
    <SidebarProvider
      className="h-screen overflow-hidden bg-transparent text-foreground"
      style={{ '--sidebar-width': `${sidebarWidth}px` } as React.CSSProperties}
    >
      <Sidebar side={sidebarPosition} collapsible="offcanvas" className="[&>[data-slot=sidebar-inner]]:bg-transparent">
        <div className="relative flex h-full flex-col overflow-hidden">
          <div
            data-testid="sidebar-resize-handle"
            className={cn(
              'absolute inset-y-0 z-50 w-1 cursor-col-resize opacity-0 transition-opacity hover:opacity-100 hover:bg-border',
              sidebarPosition === 'right' ? 'left-0' : 'right-0',
            )}
            onMouseDown={handleResizeDragStart}
          />
          <div
            className={cn('flex w-full flex-shrink-0 items-center', IS_MAC ? 'h-[44px]' : 'h-[34px]')}
            data-tauri-drag-region
          >
            <SidebarNavIcons />
          </div>
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
      </Sidebar>

      <SidebarInset className="min-w-0 overflow-hidden bg-transparent">
        <div className="flex h-full flex-col overflow-hidden">
          <ErrorBoundary>
            {activeWorkspaceId ? (
              <WorkspaceView wsId={activeWorkspaceId} label={workspaceLabel} />
            ) : activeChatId ? (
              <div className="flex h-full flex-col overflow-hidden">
                <div
                  className={cn(
                    'flex flex-shrink-0 items-center border-b border-border px-3 font-medium',
                    IS_MAC ? 'h-[44px] text-[13px]' : 'h-[34px] text-xs',
                  )}
                  data-tauri-drag-region
                >
                  {chatTabLabel}
                </div>
                <div className="flex min-h-0 flex-1 overflow-hidden bg-background">
                  <Outlet />
                </div>
              </div>
            ) : (
              <div className="flex h-full flex-col overflow-hidden bg-background">
                <Outlet />
              </div>
            )}
          </ErrorBoundary>
        </div>
      </SidebarInset>

      <SettingsDialog isOpen={settingsOpen} onClose={() => setSettingsOpen(false)} />
      <TerminalHost />
      <FontStyleInjector />
      <Toaster />
    </SidebarProvider>
  )
}
```

- [ ] **Step 4: Run the test to confirm it passes**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && bun run test src/__tests__/components/layout/IDEShell.test.tsx 2>&1 | tail -10
```

Expected: all 5 tests pass (4 existing + 1 new resize handle test).

- [ ] **Step 5: Run the full test suite**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && bun run test 2>&1 | tail -8
```

Expected: same pass/fail count as before (4 pre-existing `sidebar-nav-icons` positioning failures, everything else green).

- [ ] **Step 6: Type-check**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && bunx tsc --noEmit 2>&1 | head -20
```

Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add web/src/components/layout/IDEShell.tsx web/src/__tests__/components/layout/IDEShell.test.tsx
git commit -m "feat: add sidebar drag-to-resize with localStorage persistence"
```

---

### Task 3: Visual verification

**Files:** None — verification only.

The dev server is already running at `http://localhost:5173`. Use Chrome MCP tools.

- [ ] **Step 1: Take a screenshot of the initial state**

Verify:
- Sidebar background is transparent (no solid dark panel — should show the window/page background through)
- Sidebar width is 256px (default)

- [ ] **Step 2: Hover the resize handle**

Move the mouse to the right edge of the sidebar. Verify a thin blue/muted border line appears (the drag handle becoming visible on hover).

- [ ] **Step 3: Drag to resize**

Click and drag the handle to the right (~350px). Verify:
- Sidebar expands smoothly as you drag
- Content area shrinks accordingly

- [ ] **Step 4: Reload and verify persistence**

Navigate to `http://localhost:5173` (reload). Verify the sidebar opens at the dragged width (~350px), not 256px — proving localStorage was read.

- [ ] **Step 5: Verify minimum/maximum clamp**

Drag to the far left (past 192px minimum). Sidebar should stop shrinking at 192px. Drag to the far right (past 640px). Sidebar should stop growing at 640px.

- [ ] **Step 6: Verify toggle still works**

Click the sidebar toggle button. Sidebar slides out (width irrelevant — the gap collapses to 0). Click again — sidebar restores to the dragged width.
