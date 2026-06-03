# Sidebar Project Bloom Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the sidebar header + footer with an IntelliJ-style project color bloom header, move the nav tab switcher to a centered row below it, and delete the footer project switcher.

**Architecture:** Create `SidebarProjectHeader` (bloom gradient + project dropdown), update `IDEShell` to use it in the header and add a centered nav row, strip the footer, remove `SidebarProjectSwitcher`. The `projectNameToHue` pure function lives in the new component file and is exported for unit testing.

**Tech Stack:** React 18, TypeScript, Tailwind CSS v4, shadcn/ui DropdownMenu, Zustand, Vitest + jsdom

---

## File Map

| File | Action |
|------|--------|
| `web/src/components/layout/sidebar-project-header.tsx` | **Create** — bloom header component + exported `projectNameToHue` |
| `web/src/__tests__/components/layout/sidebar-project-header.test.ts` | **Create** — unit tests for `projectNameToHue` |
| `web/src/components/layout/sidebar-nav-icons.tsx` | **Modify** — remove `ml-auto`/`mr-auto` wrapper + `sidebarPosition` reads |
| `web/src/components/layout/IDEShell.tsx` | **Modify** — swap header, add centered nav row, remove footer |
| `web/src/components/layout/sidebar-project-switcher.tsx` | **Delete** |

---

### Task 1: `projectNameToHue` utility + tests

**Files:**
- Create: `web/src/components/layout/sidebar-project-header.tsx` (utility export only)
- Create: `web/src/__tests__/components/layout/sidebar-project-header.test.ts`

- [ ] **Step 1: Create `sidebar-project-header.tsx` with the exported utility**

  Create `web/src/components/layout/sidebar-project-header.tsx` with this exact content:

  ```tsx
  export function projectNameToHue(name: string): number {
    let hash = 0
    for (let i = 0; i < name.length; i++) {
      hash = (hash * 31 + name.charCodeAt(i)) >>> 0
    }
    return hash % 360
  }
  ```

- [ ] **Step 2: Write the failing tests**

  Create `web/src/__tests__/components/layout/sidebar-project-header.test.ts`:

  ```ts
  import { describe, expect, it } from 'vitest'
  import { projectNameToHue } from '@/components/layout/sidebar-project-header'

  describe('projectNameToHue', () => {
    it('returns 0 for an empty string', () => {
      expect(projectNameToHue('')).toBe(0)
    })

    it('returns a value in [0, 359] for any string', () => {
      for (const name of ['crowbar', 'a', 'quiver.desktop', 'Z', '123', 'hello world']) {
        const h = projectNameToHue(name)
        expect(h).toBeGreaterThanOrEqual(0)
        expect(h).toBeLessThan(360)
      }
    })

    it('is deterministic — same input always returns same output', () => {
      expect(projectNameToHue('crowbar')).toBe(projectNameToHue('crowbar'))
      expect(projectNameToHue('quiver')).toBe(projectNameToHue('quiver'))
    })

    it('produces different hues for different project names', () => {
      const hues = ['crowbar', 'quiver', 'rabbyte', 'alpha'].map(projectNameToHue)
      const unique = new Set(hues)
      expect(unique.size).toBe(hues.length)
    })
  })
  ```

- [ ] **Step 3: Run tests — expect PASS (utility is already exported)**

  ```bash
  cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npm test -- --reporter=verbose src/__tests__/components/layout/sidebar-project-header.test.ts
  ```

  Expected: 4 tests pass.

- [ ] **Step 4: Commit**

  ```bash
  git add web/src/components/layout/sidebar-project-header.tsx web/src/__tests__/components/layout/sidebar-project-header.test.ts
  git commit -m "feat: add projectNameToHue utility with tests"
  ```

---

### Task 2: Complete `SidebarProjectHeader` component

**Files:**
- Modify: `web/src/components/layout/sidebar-project-header.tsx`

- [ ] **Step 1: Replace the file with the full component**

  Overwrite `web/src/components/layout/sidebar-project-header.tsx`:

  ```tsx
  import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
  import { useProjectStore } from '@/lib/store/projects'
  import { useSettingsStore } from '@/features/settings/store'
  import { IS_MAC } from '@/utils/platform'
  import { cn } from '@/utils/cn'
  import { ChevronDown } from 'lucide-react'

  export function projectNameToHue(name: string): number {
    let hash = 0
    for (let i = 0; i < name.length; i++) {
      hash = (hash * 31 + name.charCodeAt(i)) >>> 0
    }
    return hash % 360
  }

  interface SidebarProjectHeaderProps {
    onProjectsClick?: () => void
    onProjectSelect?: (projectId: string) => void
  }

  export function SidebarProjectHeader({ onProjectsClick, onProjectSelect }: SidebarProjectHeaderProps) {
    const projects = useProjectStore(s => s.projects)
    const activeProjectId = useProjectStore(s => s.activeProjectId)
    const setActiveProject = useProjectStore(s => s.setActiveProject)
    const sidebarPosition = useSettingsStore(s => s.settings.sidebarPosition)

    const activeProject = projects.find(p => p.id === activeProjectId)
    const hue = projectNameToHue(activeProject?.name ?? '')
    const isRight = sidebarPosition === 'right'

    const bloomStyle = {
      background: `linear-gradient(${isRight ? '270deg' : '90deg'}, hsla(${hue}, 40%, 60%, 0.35) 0%, hsla(${hue}, 40%, 60%, 0.08) 60%, transparent 100%)`,
    }

    const handleSelect = (id: string) => {
      setActiveProject(id)
      onProjectSelect?.(id)
    }

    return (
      <div
        className={cn(
          'relative flex w-full flex-shrink-0 items-center overflow-hidden px-3',
          IS_MAC ? 'h-[44px]' : 'h-[34px]',
        )}
        data-tauri-drag-region
      >
        {/* Bloom gradient — header height only, no bottom border */}
        <div className="pointer-events-none absolute inset-0 z-0" style={bloomStyle} />

        {/* On Mac with left sidebar, leave space for OS traffic lights */}
        {IS_MAC && !isRight && <div className="relative z-10 w-[52px] shrink-0" />}

        <DropdownMenu>
          <DropdownMenuTrigger
            className={cn(
              'relative z-10 inline-flex items-center gap-1 rounded-sm px-1.5 py-0.5 text-[13px] font-semibold text-foreground outline-none hover:bg-accent/50',
              isRight ? 'mr-auto' : 'ml-auto',
            )}
          >
            {activeProject?.name ?? 'Select project'}
            <ChevronDown className="h-3 w-3 text-muted-foreground" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align={isRight ? 'start' : 'end'} className="min-w-[160px]">
            {projects.map(p => (
              <DropdownMenuItem
                key={p.id}
                onClick={() => handleSelect(p.id)}
                className={p.id === activeProjectId ? 'font-medium text-primary' : ''}
              >
                {p.name}
              </DropdownMenuItem>
            ))}
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={onProjectsClick} className="text-muted-foreground">
              Manage projects…
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        {/* On Mac with right sidebar, traffic lights are in content area — no spacer needed */}
      </div>
    )
  }
  ```

- [ ] **Step 2: Re-run tests to confirm utility is still exported and tests still pass**

  ```bash
  cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npm test -- --reporter=verbose src/__tests__/components/layout/sidebar-project-header.test.ts
  ```

  Expected: 4 tests pass.

- [ ] **Step 3: Commit**

  ```bash
  git add web/src/components/layout/sidebar-project-header.tsx
  git commit -m "feat: add SidebarProjectHeader with project bloom gradient"
  ```

---

### Task 3: Strip positioning logic from `SidebarNavIcons`

**Files:**
- Modify: `web/src/components/layout/sidebar-nav-icons.tsx`

The component currently wraps the Toolbar in a `div` with `ml-auto`/`mr-auto` positioning driven by `sidebarPosition`. That logic moves to the parent. The component now just returns the Toolbar directly.

- [ ] **Step 1: Edit `sidebar-nav-icons.tsx`**

  Replace the entire `SidebarNavIcons` function (lines 58–84) with:

  ```tsx
  export function SidebarNavIcons() {
    const activeTab = useSidebarStore(s => s.activeTab)

    return (
      <Toolbar className="gap-0 p-0.5 rounded-lg">
        <ToolbarGroup>
          {NAV_ITEMS.map(({ tab, label, Icon }) => (
            <NavItem
              key={tab}
              tab={tab}
              label={label}
              Icon={Icon}
              isActive={activeTab === tab}
            />
          ))}
        </ToolbarGroup>
      </Toolbar>
    )
  }
  ```

  Also remove the now-unused imports at the top:
  - Remove `useSettingsStore` import (no longer used in this file)

  Final imports section should be:

  ```tsx
  import { useState, useEffect } from 'react'
  import { motion, useMotionValue, animate } from 'motion/react'
  import { SquaresFour, FolderOpen, GitBranch } from '@phosphor-icons/react'
  import { useSidebarStore, type SidebarTab } from '@/lib/store/sidebar'
  import { Toolbar, ToolbarButton, ToolbarGroup } from '@/components/ui/toolbar'
  import { cn } from '@/utils/cn'
  ```

- [ ] **Step 2: Run the full test suite to confirm no breakage**

  ```bash
  cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npm test
  ```

  Expected: all tests pass.

- [ ] **Step 3: Commit**

  ```bash
  git add web/src/components/layout/sidebar-nav-icons.tsx
  git commit -m "refactor: remove sidebar-position alignment from SidebarNavIcons"
  ```

---

### Task 4: Rewire `IDEShell` layout

**Files:**
- Modify: `web/src/components/layout/IDEShell.tsx`

Replace the old header div + footer block with `SidebarProjectHeader` and a centered nav row.

- [ ] **Step 1: Update imports in `IDEShell.tsx`**

  Replace:
  ```tsx
  import { SidebarProvider, Sidebar, SidebarInset, SidebarFooter } from '@/components/ui/sidebar'
  import { SidebarProjectSwitcher } from './sidebar-project-switcher'
  ```

  With:
  ```tsx
  import { SidebarProvider, Sidebar, SidebarInset } from '@/components/ui/sidebar'
  import { SidebarProjectHeader } from './sidebar-project-header'
  ```

- [ ] **Step 2: Replace the header div and footer block inside `sidebarEl`**

  Find this block (roughly lines 122–158):
  ```tsx
  <div
    className={cn('flex w-full flex-shrink-0 items-center', IS_MAC ? 'h-[44px]' : 'h-[34px]')}
    data-tauri-drag-region
  >
    <SidebarNavIcons />
  </div>
  <ErrorBoundary>
    <SidebarTabs
      ...
    />
  </ErrorBoundary>
  <SidebarFooter className="p-0">
    <SidebarProjectSwitcher
      onProjectsClick={() => void navigate({ to: '/projects' })}
      onProjectSelect={() => void navigate({ to: '/' })}
      onSettingsClick={() => setSettingsOpen(true)}
    />
  </SidebarFooter>
  ```

  Replace with:
  ```tsx
  <SidebarProjectHeader
    onProjectsClick={() => void navigate({ to: '/projects' })}
    onProjectSelect={() => void navigate({ to: '/' })}
  />
  <div className="flex h-8 flex-shrink-0 items-center justify-center">
    <SidebarNavIcons />
  </div>
  <ErrorBoundary>
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
  ```

  Note: `onSettingsClick` is intentionally removed — settings remains accessible via the application menu.

- [ ] **Step 3: Run the full test suite**

  ```bash
  cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npm test
  ```

  Expected: all tests pass.

- [ ] **Step 4: Commit**

  ```bash
  git add web/src/components/layout/IDEShell.tsx
  git commit -m "feat: replace sidebar header/footer with SidebarProjectHeader and centered nav row"
  ```

---

### Task 5: Delete `sidebar-project-switcher.tsx`

**Files:**
- Delete: `web/src/components/layout/sidebar-project-switcher.tsx`

- [ ] **Step 1: Verify no remaining imports**

  ```bash
  grep -r "sidebar-project-switcher" /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web/src
  ```

  Expected: no output. If any file still imports it, fix that import before deleting.

- [ ] **Step 2: Delete the file**

  ```bash
  rm /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web/src/components/layout/sidebar-project-switcher.tsx
  ```

- [ ] **Step 3: Run the full test suite**

  ```bash
  cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npm test
  ```

  Expected: all tests pass. If TypeScript complains, there's a lingering import — find and remove it.

- [ ] **Step 4: Commit**

  ```bash
  git add -A web/src/components/layout/sidebar-project-switcher.tsx
  git commit -m "chore: delete sidebar-project-switcher (replaced by SidebarProjectHeader)"
  ```

---

### Task 6: Visual verification with Chrome DevTools MCP

**Goal:** Confirm the bloom renders correctly in the browser, project switching changes the color, and both sidebar positions (left/right) work.

- [ ] **Step 1: Start the dev server**

  ```bash
  cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npm run dev &
  ```

  Wait ~3 seconds for Vite to finish bundling.

- [ ] **Step 2: Open the app and take a baseline screenshot**

  Use `mcp__chrome-devtools-mcp__navigate_page` to go to `http://localhost:5173`.
  Use `mcp__chrome-devtools-mcp__take_screenshot` to capture the initial state.

  Verify:
  - Sidebar header shows the project name dropdown (right-aligned for left sidebar)
  - A color gradient is visible behind the traffic lights
  - The Workspaces/Files/Git nav tabs appear centered just below the header
  - No footer project switcher is visible

- [ ] **Step 3: Verify color changes on project switch**

  Use `mcp__chrome-devtools-mcp__take_screenshot` before switching projects.
  Use `mcp__chrome-devtools-mcp__click` to open the project dropdown and select a different project.
  Use `mcp__chrome-devtools-mcp__take_screenshot` after switching.

  Verify: the gradient color in the header changed to reflect the new project name.

- [ ] **Step 4: Verify right-side sidebar**

  Open settings, switch sidebar to the right side.
  Use `mcp__chrome-devtools-mcp__take_screenshot`.

  Verify:
  - Gradient direction is reversed (comes from the right edge)
  - Project name dropdown is now left-aligned
  - No traffic light spacer visible on the sidebar side

- [ ] **Step 5: Kill the dev server and final commit**

  ```bash
  kill %1 2>/dev/null || pkill -f "vite" 2>/dev/null; true
  ```

  No commit needed — this task only verifies.
