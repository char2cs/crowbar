# shadcn/ui Sidebar Migration Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the custom `ResizablePanelGroup` sidebar with shadcn/ui's `Sidebar` component (`collapsible="offcanvas"`) and wire a toggle button in the tab bar via `useSidebar().toggleSidebar`.

**Architecture:** Four changes applied in sequence. A prior failed attempt added `sidebarVisible`/`setSidebarVisible` to the layout sidebar store, wired `IDEShell` to `.collapse()`/`.expand()` a `ResizablePanel` ref, and added a custom toggle button reading from that store — all of this gets replaced. The correct approach: `SidebarProvider` wraps the entire shell, `<Sidebar collapsible="offcanvas">` holds the sidebar content and handles all show/hide animation internally via CSS, and the toggle button in the tab bar calls `useSidebar().toggleSidebar` from context (which works because `tab-bar.tsx` renders inside `WorkspaceView` inside `SidebarInset` inside `SidebarProvider`). No custom Zustand state is needed for sidebar visibility — `SidebarProvider` manages it.

**Tech Stack:** React 18, shadcn/ui Sidebar (already installed at `web/src/components/ui/sidebar.tsx`), Zustand (`createSelectors`), Phosphor Icons (`SidebarSimple as PanelLeftClose`), Tailwind CSS v4, TypeScript, Vitest (`bun run test`), `bunx tsc --noEmit` for type checking.

> **⚠️ IMPORTANT — intermediate broken state:** Tasks 1–3 together form one atomic change. After Task 1, `IDEShell.tsx` and `tab-bar.tsx` will have TypeScript errors until Tasks 2 and 3 are also done. Do NOT run `bunx tsc --noEmit` between Tasks 1–3. Only type-check at the end of Task 3.

---

### Task 1: Revert the wrong sidebar-store additions

**Files:**
- Modify: `web/src/features/layout/stores/sidebar-store.ts`
- Delete: `web/src/__tests__/features/layout/stores/sidebar-store.test.ts`

The failed attempt added `sidebarVisible` and `setSidebarVisible` to the layout sidebar store. These must be removed — shadcn `SidebarProvider` owns this state instead.

- [ ] **Step 1: Replace the entire content of `sidebar-store.ts`**

```typescript
import { create } from "zustand";
import { createSelectors } from "@/utils/zustand-selectors";

interface SidebarState {
  activePath?: string;
  updateActivePath: (path: string) => void;
}

const useSidebarStoreBase = create<SidebarState>()((set) => ({
  activePath: undefined,
  updateActivePath: (path: string) => {
    set({ activePath: path });
  },
}));

export const useSidebarStore = createSelectors(useSidebarStoreBase);
```

- [ ] **Step 2: Delete the wrong test file**

```bash
rm web/src/__tests__/features/layout/stores/sidebar-store.test.ts
```

- [ ] **Step 3: Commit (do NOT type-check yet — IDEShell and tab-bar still reference the removed state)**

```bash
git add web/src/features/layout/stores/sidebar-store.ts
git rm web/src/__tests__/features/layout/stores/sidebar-store.test.ts
git commit -m "revert: remove sidebarVisible state from layout sidebar store"
```

---

### Task 2: Restructure IDEShell with shadcn SidebarProvider + Sidebar

**Files:**
- Modify: `web/src/components/layout/IDEShell.tsx`

The failed attempt added a `sidebarPanelRef`, a `useLayoutSidebarStore` import, a `useEffect` driving `.collapse()`/`.expand()`, `collapsible`/`collapsedSize`/`onResize` props on the `ResizablePanel`, and conditional rendering of `ResizableHandle`. All of this is removed. The layout switches from `ResizablePanelGroup` to `SidebarProvider` + `Sidebar` + `SidebarInset`.

Key facts about shadcn's `Sidebar`:
- `SidebarProvider` wraps everything; it creates the context, stores open state in a cookie, and sets `--sidebar-width: 16rem` as a CSS custom property.
- `<Sidebar collapsible="offcanvas">` renders a fixed-positioned panel. When collapsed, it slides out of view via `left: calc(var(--sidebar-width) * -1)`. The "gap div" inside Sidebar also transitions from 16rem to 0rem width, pushing the SidebarInset to full width automatically.
- `SidebarInset` is a `<main>` element with `flex-1` that fills the space not occupied by the sidebar gap.
- Sidebar position (left/right) is controlled by the `side` prop, which accepts `"left" | "right"` — the same type as `settings.sidebarPosition`.
- Naming conflict: our custom `SidebarHeader` component (from `'./SidebarHeader'`) has the same name as the shadcn `SidebarHeader` export from `'@/components/ui/sidebar'`. We import from `'./SidebarHeader'` only — do NOT import `SidebarHeader` from `'@/components/ui/sidebar'`.

- [ ] **Step 1: Replace the entire content of `IDEShell.tsx`**

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

  return (
    <SidebarProvider className="h-screen overflow-hidden bg-transparent text-foreground">
      <Sidebar side={sidebarPosition} collapsible="offcanvas">
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

- [ ] **Step 2: Commit (do NOT type-check yet — tab-bar still references the removed store state)**

```bash
git add web/src/components/layout/IDEShell.tsx
git commit -m "feat: restructure IDEShell with shadcn SidebarProvider + Sidebar + SidebarInset"
```

---

### Task 3: Update tab-bar toggle button to use useSidebar()

**Files:**
- Modify: `web/src/features/tabs/components/tab-bar.tsx`

The failed attempt added three wrong lines and a wrong toggle button that read from the now-reverted store. Replace those with `useSidebar()` from shadcn.

Context:
- `tab-bar.tsx` already imports `useSidebarStore` from `"@/features/layout/stores/sidebar-store"` (for `updateActivePath` — keep this).
- It also already imports `useSettingsStore` from `"@/features/settings/store"` (keep existing usages).
- `useSidebar` is exported from `"@/components/ui/sidebar"` and returns `{ open, toggleSidebar, state, ... }`.
- `useSidebar()` works here because `tab-bar.tsx` renders inside `WorkspaceView` → `SidebarInset` → `SidebarProvider` (set up in Task 2).
- The `sidebarPosition` selector added by the failed attempt is still needed for icon mirroring — keep it.

- [ ] **Step 1: Add the useSidebar import**

Find (around line 76):
```tsx
import { Button } from "@/components/ui/button";
```

Insert immediately after that line:
```tsx
import { useSidebar } from "@/components/ui/sidebar";
```

- [ ] **Step 2: Replace the three wrong selectors with useSidebar()**

Find (around line 181):
```tsx
  const sidebarVisible = useSidebarStore((s) => s.sidebarVisible)
  const setSidebarVisible = useSidebarStore((s) => s.setSidebarVisible)
  const sidebarPosition = useSettingsStore((s) => s.settings.sidebarPosition)
```

Replace with:
```tsx
  const sidebarPosition = useSettingsStore((s) => s.settings.sidebarPosition)
  const { open: sidebarOpen, toggleSidebar } = useSidebar()
```

- [ ] **Step 3: Update the toggle button JSX**

Find (around line 756):
```tsx
          {!isBottomPane && (
            <Button
              type="button"
              onClick={() => setSidebarVisible(!sidebarVisible)}
              variant="ghost"
              compact
              className={cn(
                "h-6 w-6 shrink-0 rounded-full p-0 text-muted-foreground",
                sidebarPosition === 'right' && "scale-x-[-1]",
              )}
              tooltip={sidebarVisible ? "Hide Sidebar" : "Show Sidebar"}
              tooltipSide="bottom"
              aria-label={sidebarVisible ? "Hide sidebar" : "Show sidebar"}
            >
              <PanelLeftClose size={14} />
            </Button>
          )}
```

Replace with:
```tsx
          {!isBottomPane && (
            <Button
              type="button"
              onClick={toggleSidebar}
              variant="ghost"
              compact
              className={cn(
                "h-6 w-6 shrink-0 rounded-full p-0 text-muted-foreground",
                sidebarPosition === 'right' && "scale-x-[-1]",
              )}
              tooltip={sidebarOpen ? "Hide Sidebar" : "Show Sidebar"}
              tooltipSide="bottom"
              aria-label={sidebarOpen ? "Hide sidebar" : "Show sidebar"}
            >
              <PanelLeftClose size={14} />
            </Button>
          )}
```

- [ ] **Step 4: Type-check**

```bash
cd web && bunx tsc --noEmit 2>&1 | head -40
```

Expected: no new errors. Note any pre-existing errors so you can distinguish them from regressions. If you see errors about `sidebarVisible` or `setSidebarVisible`, a wrong reference was missed — search and fix.

- [ ] **Step 5: Run the full test suite**

```bash
cd web && bun run test
```

Expected: all existing tests pass. The wrong `sidebar-store.test.ts` was deleted in Task 1, so no tests for sidebarVisible should appear.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/tabs/components/tab-bar.tsx
git commit -m "feat: replace custom sidebar toggle with useSidebar().toggleSidebar"
```

---

### Task 4: Visual verification with Chrome MCP dev tools

**Files:** None — verification only.

The dev server should already be running. If not:

```bash
cd web && bun run dev
```

- [ ] **Step 1: Take a screenshot of the initial state**

Use the Chrome MCP `take_screenshot` tool. Verify:
- Sidebar is visible by default (left side, standard width ~256px).
- The animated nav pills (Workspaces / Files / Git) appear in the sidebar titlebar strip.
- The toggle button (panel icon) appears in the tab bar, to the left of the ← → arrows.

- [ ] **Step 2: Toggle the sidebar hidden**

Click the toggle button (or use the Chrome MCP `click` tool). Take a screenshot. Verify:
- Sidebar slides out of view with a smooth CSS transition (200ms ease-linear).
- Content area expands to full width.
- No resize handle is visible.

- [ ] **Step 3: Toggle the sidebar visible again**

Click the toggle button again. Verify:
- Sidebar slides back in to the same width.

- [ ] **Step 4: Check keyboard shortcut**

Press Cmd+B (Mac) or Ctrl+B (Windows). Verify the sidebar toggles. (This is built into `SidebarProvider` automatically.)

- [ ] **Step 5: Verify bottom pane exclusion**

Open a terminal tab (bottom pane). Verify the toggle button does NOT appear in the bottom pane's tab bar (because `isBottomPane` is true there).

- [ ] **Step 6: Verify sidebar-right position (if easily accessible)**

If the Settings UI is accessible in the app: change sidebar position to "right". Verify:
- Sidebar moves to the right side.
- Toggle icon is mirrored horizontally (the `scale-x-[-1]` class).
- Toggle still works.

- [ ] **Step 7: Report findings**

Describe what you observed and flag any visual issues (wrong sidebar width, wrong background color, animation artifacts, border issues). Do not commit anything for this task.
