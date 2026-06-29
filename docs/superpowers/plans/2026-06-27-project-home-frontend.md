# Project Home Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface the Project Home workspace in the Crowbar UI — sidebar row above the project indicator, `/ide/$projectId/home` route, git tab suppression, and default redirect.

**Architecture:** Five independent tasks proceeding from low-level types → UI component → route → tab behavior → redirect logic. Each task compiles and tests in isolation. The route (Task 3) depends on the types added in Task 1; all other tasks are independent of each other.

**Tech Stack:** React 18, TanStack Router (file-based), `@phosphor-icons/react`, Zustand stores, `useQuery` (TanStack Query), Vitest + @testing-library/react.

## Global Constraints

- All test files live under `web/src/__tests__/` mirroring `web/src/` structure — never co-located.
- All component files use kebab-case filenames; exported component names are PascalCase.
- Use `@/` imports inside all test files (never relative `../../`).
- Icon library: `@phosphor-icons/react` — NOT Lucide.
- Row styling: use `ROW_BASE`, `ROW_ACTIVE`, `ROW_INACTIVE` from `@/components/layout/workspace-row-base`.
- Store selectors: always use narrow selectors — never `useXxxStore()` with no argument.
- Project Home row order in sidebar: **ProjectHomeRow first**, then `ProjectSwitcherRow` ("Rabbyte"), then repo rows.
- The backend endpoint is `GET /v0/projects/:projectId/home` and returns a `WorkspaceDTO` with `kind: "home"`.

---

### Task 1: Types and API function

**Files:**
- Modify: `web/src/lib/types.ts` — add `kind` to `WorkspaceDTO`
- Modify: `web/src/lib/api.ts` — add `fetchHomeWorkspace`
- Test: `web/src/__tests__/lib/api.test.ts` (create if absent, otherwise append)

**Interfaces:**
- Produces: `WorkspaceDTO.kind?: 'git' | 'home'` (used by Task 3)
- Produces: `fetchHomeWorkspace(projectId: string): Promise<WorkspaceDTO>` (used by Task 3)

- [ ] **Step 1: Add `kind` to `WorkspaceDTO` in `web/src/lib/types.ts`**

  After the existing `localPath` field, add:
  ```ts
  /** "home" for the project home workspace; absent or "git" for normal git workspaces. */
  kind?: 'git' | 'home'
  ```

  Full updated interface (the only change is the two lines at the end):
  ```ts
  export interface WorkspaceDTO {
    id: string
    repoId: string
    projectId: string
    branch: string
    parentId: string
    forkPointSha: string
    status: WorkspaceStatusDTO
    working: boolean
    lastError: string
    isDefault?: boolean
    added: number
    deleted: number
    mergeStrategy: string
    canMergeLocally: boolean
    mergeConflicts: boolean
    parentBranch: string
    prUrl: string
    prTitle: string
    prTargetBranch: string
    /** On-disk worktree directory for this workspace (e.g. /home/user/project). */
    localPath?: string
    /** "home" for the project home workspace; absent or "git" for normal git workspaces. */
    kind?: 'git' | 'home'
  }
  ```

- [ ] **Step 2: Add `fetchHomeWorkspace` to `web/src/lib/api.ts`**

  After the existing `fetchWorkspace` export (~line 126), add:
  ```ts
  export function fetchHomeWorkspace(projectId: string): Promise<WorkspaceDTO> {
    return apiFetch<WorkspaceDTO>(`/v0/projects/${projectId}/home`)
  }
  ```

- [ ] **Step 3: Write a test for `fetchHomeWorkspace` in `web/src/__tests__/lib/api.test.ts`**

  Create the file if it doesn't exist, or append this describe block:
  ```ts
  import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
  import { fetchHomeWorkspace } from '@/lib/api'

  describe('fetchHomeWorkspace', () => {
    beforeEach(() => {
      vi.stubGlobal('fetch', vi.fn())
    })
    afterEach(() => {
      vi.unstubAllGlobals()
    })

    it('fetches from the correct endpoint and returns JSON', async () => {
      const dto = { id: 'ws-home', projectId: 'p1', kind: 'home' }
      vi.mocked(fetch).mockResolvedValueOnce(
        new Response(JSON.stringify(dto), { status: 200 }),
      )
      const result = await fetchHomeWorkspace('p1')
      expect(result).toEqual(dto)
      expect(vi.mocked(fetch)).toHaveBeenCalledWith(
        expect.stringContaining('/v0/projects/p1/home'),
        expect.any(Object),
      )
    })

    it('throws on non-2xx response', async () => {
      vi.mocked(fetch).mockResolvedValueOnce(
        new Response('{"error":"not found"}', { status: 404 }),
      )
      await expect(fetchHomeWorkspace('p1')).rejects.toThrow()
    })
  })
  ```

- [ ] **Step 4: Run the test to make sure it passes**

  ```bash
  cd web && npx vitest run src/__tests__/lib/api.test.ts
  ```

  Expected: all tests pass. If the file already existed and has other tests, all must still pass.

- [ ] **Step 5: Type-check**

  ```bash
  cd web && npx tsc --noEmit
  ```

  Expected: no errors.

- [ ] **Step 6: Commit**

  ```bash
  git add web/src/lib/types.ts web/src/lib/api.ts web/src/__tests__/lib/api.test.ts
  git commit -m "feat(types): add kind field to WorkspaceDTO and fetchHomeWorkspace API helper"
  ```

---

### Task 2: ProjectHomeRow component + workspace-tree insertion

**Files:**
- Create: `web/src/components/layout/project-home-row.tsx`
- Modify: `web/src/components/layout/workspace-tree.tsx` — insert `<ProjectHomeRow />` before `<ProjectSwitcherRow />`
- Test: `web/src/__tests__/components/layout/project-home-row.test.tsx`

**Interfaces:**
- Consumes: `ROW_BASE`, `ROW_ACTIVE`, `ROW_INACTIVE` from `@/components/layout/workspace-row-base`
- Consumes: `useProjectStore((s) => s.activeProjectId)` from `@/lib/store/projects`
- Consumes: `useNavigate`, `useMatch` from `@tanstack/react-router`
- Produces: `<ProjectHomeRow />` — zero props, self-contained

- [ ] **Step 1: Create `web/src/components/layout/project-home-row.tsx`**

  ```tsx
  import { House } from '@phosphor-icons/react'
  import { useNavigate, useMatch } from '@tanstack/react-router'
  import { cn } from '@/lib/utils'
  import { ROW_BASE, ROW_ACTIVE, ROW_INACTIVE } from './workspace-row-base'
  import { useProjectStore } from '@/lib/store/projects'

  export function ProjectHomeRow() {
    const navigate = useNavigate()
    const projectId = useProjectStore((s) => s.activeProjectId)
    const isActive = useMatch({ from: '/ide/$projectId/home', shouldThrow: false })

    function handleClick() {
      if (!projectId) return
      void navigate({ to: '/ide/$projectId/home', params: { projectId } })
    }

    return (
      <div
        role="button"
        tabIndex={0}
        aria-label="Project Home"
        className={cn(ROW_BASE, isActive ? ROW_ACTIVE : ROW_INACTIVE)}
        onClick={handleClick}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') handleClick()
        }}
      >
        <House size={14} weight={isActive ? 'fill' : 'regular'} />
        <span>Home</span>
      </div>
    )
  }
  ```

- [ ] **Step 2: Write the failing test at `web/src/__tests__/components/layout/project-home-row.test.tsx`**

  ```tsx
  import { render, screen, fireEvent } from '@testing-library/react'
  import { describe, it, expect, beforeEach, vi } from 'vitest'
  import { ProjectHomeRow } from '@/components/layout/project-home-row'
  import { useProjectStore } from '@/lib/store/projects'

  const navigateMock = vi.fn()
  let mockMatch: object | null = null

  vi.mock('@tanstack/react-router', () => ({
    useNavigate: () => navigateMock,
    useMatch: () => mockMatch,
  }))

  beforeEach(() => {
    navigateMock.mockClear()
    mockMatch = null
    useProjectStore.setState({ activeProjectId: 'p1', projects: [] })
  })

  describe('ProjectHomeRow', () => {
    it('renders with label "Home"', () => {
      render(<ProjectHomeRow />)
      expect(screen.getByRole('button', { name: /Home/i })).toBeInTheDocument()
    })

    it('navigates to home route on click', () => {
      render(<ProjectHomeRow />)
      fireEvent.click(screen.getByRole('button', { name: /Home/i }))
      expect(navigateMock).toHaveBeenCalledWith({
        to: '/ide/$projectId/home',
        params: { projectId: 'p1' },
      })
    })

    it('does not navigate when no active project', () => {
      useProjectStore.setState({ activeProjectId: '', projects: [] })
      render(<ProjectHomeRow />)
      fireEvent.click(screen.getByRole('button', { name: /Home/i }))
      expect(navigateMock).not.toHaveBeenCalled()
    })

    it('applies ROW_ACTIVE class when route matches', () => {
      mockMatch = { params: { projectId: 'p1' } }
      render(<ProjectHomeRow />)
      const btn = screen.getByRole('button', { name: /Home/i })
      // ROW_ACTIVE contains 'bg-background' — confirm presence
      expect(btn.className).toContain('bg-background')
    })

    it('applies ROW_INACTIVE class when route does not match', () => {
      mockMatch = null
      render(<ProjectHomeRow />)
      const btn = screen.getByRole('button', { name: /Home/i })
      expect(btn.className).toContain('border-transparent')
    })
  })
  ```

- [ ] **Step 3: Run the test to confirm it fails (component not yet imported)**

  ```bash
  cd web && npx vitest run src/__tests__/components/layout/project-home-row.test.tsx
  ```

  Expected: fails with module resolution error (file doesn't exist yet if you haven't created it, or assertion failures).

- [ ] **Step 4: Run the test after creating the component**

  ```bash
  cd web && npx vitest run src/__tests__/components/layout/project-home-row.test.tsx
  ```

  Expected: all 5 tests pass.

- [ ] **Step 5: Insert `<ProjectHomeRow />` into `workspace-tree.tsx`**

  In `web/src/components/layout/workspace-tree.tsx`, add the import at the top (with other layout imports):
  ```ts
  import { ProjectHomeRow } from './project-home-row'
  ```

  Then find the `WorkspaceTreeInner` function's return block. The current first line of the tree content is:
  ```tsx
  <ProjectSwitcherRow />
  ```

  Change it to:
  ```tsx
  <ProjectHomeRow />
  <ProjectSwitcherRow />
  ```

  The full updated block starts at the `return (` of `WorkspaceTreeInner`:
  ```tsx
  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <ProjectHomeRow />
      <ProjectSwitcherRow />
      <ScrollArea className="flex-1">
  ```

- [ ] **Step 6: Run all layout tests to confirm nothing broke**

  ```bash
  cd web && npx vitest run src/__tests__/components/layout/
  ```

  Expected: all tests pass.

- [ ] **Step 7: Type-check**

  ```bash
  cd web && npx tsc --noEmit
  ```

  Expected: no errors.

- [ ] **Step 8: Commit**

  ```bash
  git add web/src/components/layout/project-home-row.tsx \
          web/src/components/layout/workspace-tree.tsx \
          web/src/__tests__/components/layout/project-home-row.test.tsx
  git commit -m "feat(sidebar): add ProjectHomeRow above ProjectSwitcherRow"
  ```

---

### Task 3: Route `/ide/$projectId/home`

**Files:**
- Create: `web/src/routes/_shell/ide/$projectId/home.tsx`
- Test: `web/src/__tests__/routes/_shell/ide/$projectId/home.test.tsx`

**Interfaces:**
- Consumes: `fetchHomeWorkspace(projectId: string): Promise<WorkspaceDTO>` from Task 1
- Consumes: `WorkspaceView({ wsId: string })` from `@/features/workspace/components/workspace-view`
- Consumes: TanStack Router file-based routing — `createFileRoute('/_shell/ide/$projectId/home')`

Note: The test for this route tests the component in isolation with mocked dependencies (no real router required).

- [ ] **Step 1: Write the failing test**

  Create `web/src/__tests__/routes/_shell/ide/$projectId/home.test.tsx`:
  ```tsx
  import { render, screen, waitFor } from '@testing-library/react'
  import { describe, it, expect, vi, beforeEach } from 'vitest'

  // Mock the route module so createFileRoute doesn't blow up in test
  vi.mock('@tanstack/react-router', () => ({
    createFileRoute: () => ({ component: (c: unknown) => c }),
    useParams: () => ({ projectId: 'p1' }),
  }))

  vi.mock('@/lib/api', () => ({
    fetchHomeWorkspace: vi.fn(),
  }))

  vi.mock('@/features/workspace/components/workspace-view', () => ({
    WorkspaceView: ({ wsId }: { wsId: string }) => <div data-testid="workspace-view">{wsId}</div>,
  }))

  // Import after mocks
  const { fetchHomeWorkspace } = await import('@/lib/api')
  const { HomeRoute } = await import('@/routes/_shell/ide/$projectId/home')

  beforeEach(() => {
    vi.mocked(fetchHomeWorkspace).mockClear()
  })

  describe('HomeRoute component', () => {
    it('renders WorkspaceView with the home workspace id on success', async () => {
      vi.mocked(fetchHomeWorkspace).mockResolvedValueOnce({
        id: 'ws-home-1',
        projectId: 'p1',
        kind: 'home',
      } as never)

      render(<HomeRoute />)

      await waitFor(() => {
        expect(screen.getByTestId('workspace-view')).toBeInTheDocument()
        expect(screen.getByTestId('workspace-view').textContent).toBe('ws-home-1')
      })
    })

    it('renders loading state initially', () => {
      vi.mocked(fetchHomeWorkspace).mockReturnValue(new Promise(() => {}))
      render(<HomeRoute />)
      expect(screen.queryByTestId('workspace-view')).not.toBeInTheDocument()
    })

    it('renders fallback on error', async () => {
      vi.mocked(fetchHomeWorkspace).mockRejectedValueOnce(new Error('not found'))
      render(<HomeRoute />)
      await waitFor(() => {
        expect(screen.getByText(/unavailable/i)).toBeInTheDocument()
      })
    })
  })
  ```

- [ ] **Step 2: Run the test to confirm it fails**

  ```bash
  cd web && npx vitest run "src/__tests__/routes/_shell/ide/\$projectId/home.test.tsx"
  ```

  Expected: fails with module-not-found or import error.

- [ ] **Step 3: Create the route at `web/src/routes/_shell/ide/$projectId/home.tsx`**

  ```tsx
  import { createFileRoute, useParams } from '@tanstack/react-router'
  import { useQuery } from '@tanstack/react-query'
  import { WorkspaceView } from '@/features/workspace/components/workspace-view'
  import { fetchHomeWorkspace } from '@/lib/api'

  export function HomeRoute() {
    const { projectId } = useParams({ from: '/_shell/ide/$projectId/home' })
    const { data, isPending, isError } = useQuery({
      queryKey: ['home-workspace', projectId],
      queryFn: () => fetchHomeWorkspace(projectId),
    })

    if (isPending) return null
    if (isError || !data) {
      return (
        <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
          Project Home unavailable
        </div>
      )
    }

    return <WorkspaceView wsId={data.id} />
  }

  export const Route = createFileRoute('/_shell/ide/$projectId/home')({
    component: HomeRoute,
  })
  ```

- [ ] **Step 4: Run the tests**

  ```bash
  cd web && npx vitest run "src/__tests__/routes/_shell/ide/\$projectId/home.test.tsx"
  ```

  Expected: all 3 tests pass.

- [ ] **Step 5: Type-check**

  ```bash
  cd web && npx tsc --noEmit
  ```

  Expected: no errors. TanStack Router's file-based routing will pick up the new file automatically.

- [ ] **Step 6: Commit**

  ```bash
  git add "web/src/routes/_shell/ide/\$projectId/home.tsx" \
          "web/src/__tests__/routes/_shell/ide/\$projectId/home.test.tsx"
  git commit -m "feat(route): add /ide/\$projectId/home route for Project Home"
  ```

---

### Task 4: Suppress git tab on home route

**Files:**
- Modify: `web/src/components/layout/sidebar-tab-bar.tsx`
- Test: `web/src/__tests__/components/layout/sidebar-tab-bar.test.tsx` — extend existing file

**Interfaces:**
- Consumes: `useMatch` from `@tanstack/react-router`
- Consumes: `useSidebarStore` for `activeTab` / `setActiveTab`

- [ ] **Step 1: Update `sidebar-tab-bar.tsx` to filter the git tab and add reset effect**

  Current file imports only:
  ```ts
  import { SquaresFour, ChatsCircle, FolderOpen, GitBranch } from '@phosphor-icons/react'
  import { Tabs, TabsList, TabsTab } from '@/components/ui/tabs'
  import { cn } from '@/utils/cn'
  import { useSidebarStore, type SidebarTab } from '@/lib/store/sidebar'
  ```

  Add `useEffect` and `useMatch`:
  ```ts
  import { useEffect } from 'react'
  import { useMatch } from '@tanstack/react-router'
  import { SquaresFour, ChatsCircle, FolderOpen, GitBranch } from '@phosphor-icons/react'
  import { Tabs, TabsList, TabsTab } from '@/components/ui/tabs'
  import { cn } from '@/utils/cn'
  import { useSidebarStore, type SidebarTab } from '@/lib/store/sidebar'
  ```

  Replace the `SidebarTabBar` function body:
  ```tsx
  export function SidebarTabBar() {
    const activeTab = useSidebarStore((s) => s.activeTab)
    const setActiveTab = useSidebarStore((s) => s.setActiveTab)
    const isHomeRoute = useMatch({ from: '/ide/$projectId/home', shouldThrow: false })

    useEffect(() => {
      if (isHomeRoute && activeTab === 'git') {
        setActiveTab('workspaces')
      }
    }, [isHomeRoute, activeTab, setActiveTab])

    const visibleTabs = isHomeRoute ? TABS.filter((t) => t.tab !== 'git') : TABS

    return (
      // @container so child labels can respond to the sidebar's actual width
      <div className="@container flex shrink-0 items-center px-2 py-1.5">
        <Tabs
          value={activeTab}
          onValueChange={(v) => setActiveTab(v as SidebarTab)}
          className="w-full"
        >
          <TabsList variant="default" className="w-full">
            {visibleTabs.map(({ tab, label, Icon }) => {
              const isActive = activeTab === tab
              return (
                <TabsTab
                  key={tab}
                  value={tab}
                  className="flex flex-1 items-center justify-center gap-1"
                >
                  <Icon size={14} weight={isActive ? 'fill' : 'regular'} />
                  <span
                    className={cn(
                      // < 280px: no labels for anyone
                      'hidden',
                      // 280–420px: label only for the active tab
                      isActive && '@[280px]:inline',
                      // ≥ 420px: labels for all tabs
                      '@[420px]:inline',
                    )}
                  >
                    {label}
                  </span>
                </TabsTab>
              )
            })}
          </TabsList>
        </Tabs>
      </div>
    )
  }
  ```

- [ ] **Step 2: Extend `web/src/__tests__/components/layout/sidebar-tab-bar.test.tsx`**

  The file currently has a `describe('SidebarTabBar', ...)` block with 3 passing tests. Append these cases inside the same describe block. First, the existing test `vi.mock` for `@tanstack/react-router` is NOT yet present — add it at the top of the file (before existing imports or as the first thing):

  Full updated test file:
  ```tsx
  import { render, screen, fireEvent } from '@testing-library/react'
  import { describe, it, expect, beforeEach, vi } from 'vitest'
  import { SidebarTabBar } from '@/components/layout/sidebar-tab-bar'
  import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'

  let mockMatch: object | null = null

  vi.mock('@tanstack/react-router', () => ({
    useMatch: () => mockMatch,
  }))

  describe('SidebarTabBar', () => {
    beforeEach(() => {
      useSidebarStore.setState(getInitialState())
      mockMatch = null
    })

    it('renders all 4 tabs when not on home route', () => {
      render(<SidebarTabBar />)
      expect(screen.getByRole('tab', { name: /workspaces/i })).toBeInTheDocument()
      expect(screen.getByRole('tab', { name: /chats/i })).toBeInTheDocument()
      expect(screen.getByRole('tab', { name: /files/i })).toBeInTheDocument()
      expect(screen.getByRole('tab', { name: /git/i })).toBeInTheDocument()
    })

    it('marks the active tab as selected', () => {
      useSidebarStore.setState({ activeTab: 'files' })
      render(<SidebarTabBar />)
      expect(screen.getByRole('tab', { name: /files/i })).toHaveAttribute('aria-selected', 'true')
    })

    it('calls setActiveTab when a tab is clicked', () => {
      render(<SidebarTabBar />)
      fireEvent.click(screen.getByRole('tab', { name: /chats/i }))
      expect(useSidebarStore.getState().activeTab).toBe('chats')
    })

    it('hides the git tab on the home route', () => {
      mockMatch = { params: { projectId: 'p1' } }
      render(<SidebarTabBar />)
      expect(screen.queryByRole('tab', { name: /git/i })).not.toBeInTheDocument()
      expect(screen.getByRole('tab', { name: /workspaces/i })).toBeInTheDocument()
      expect(screen.getByRole('tab', { name: /chats/i })).toBeInTheDocument()
      expect(screen.getByRole('tab', { name: /files/i })).toBeInTheDocument()
    })

    it('resets active tab to workspaces when navigating to home route with git active', () => {
      useSidebarStore.setState({ activeTab: 'git' })
      mockMatch = { params: { projectId: 'p1' } }
      render(<SidebarTabBar />)
      expect(useSidebarStore.getState().activeTab).toBe('workspaces')
    })
  })
  ```

- [ ] **Step 3: Run the test**

  ```bash
  cd web && npx vitest run src/__tests__/components/layout/sidebar-tab-bar.test.tsx
  ```

  Expected: all 5 tests pass.

- [ ] **Step 4: Type-check**

  ```bash
  cd web && npx tsc --noEmit
  ```

  Expected: no errors.

- [ ] **Step 5: Commit**

  ```bash
  git add web/src/components/layout/sidebar-tab-bar.tsx \
          web/src/__tests__/components/layout/sidebar-tab-bar.test.tsx
  git commit -m "feat(sidebar): hide git tab and reset active tab on Project Home route"
  ```

---

### Task 5: Default redirect to Project Home

**Files:**
- Modify: `web/src/routes/_shell/index.tsx`
- Test: `web/src/__tests__/routes/_shell/index.test.ts` (create)

**Interfaces:**
- Consumes: `fetchProjects` from `@/lib/api`
- Consumes: `useProjectStore` for `activeProjectId`
- Consumes: TanStack Router `redirect` / `isRedirect`

- [ ] **Step 1: Write the failing test**

  Create `web/src/__tests__/routes/_shell/index.test.ts`:
  ```ts
  import { describe, it, expect, vi, beforeEach } from 'vitest'

  const redirectMock = vi.fn((opts: unknown) => {
    const err = new Error('redirect')
    Object.assign(err, { isRedirect: true, options: opts })
    return err
  })

  vi.mock('@tanstack/react-router', () => ({
    createFileRoute: () => (config: unknown) => config,
    redirect: redirectMock,
    isRedirect: (e: unknown) => (e as { isRedirect?: boolean })?.isRedirect === true,
  }))

  vi.mock('@/lib/api', () => ({
    fetchProjects: vi.fn(),
  }))

  vi.mock('@/lib/store/projects', () => ({
    useProjectStore: { getState: vi.fn(() => ({ activeProjectId: '' })) },
  }))

  const { fetchProjects } = await import('@/lib/api')
  const { useProjectStore } = await import('@/lib/store/projects')

  async function runBeforeLoad() {
    // Dynamically import to pick up mocks
    const mod = await import('@/routes/_shell/index')
    const config = (mod as { Route?: { options?: { beforeLoad?: () => Promise<void> } } }).Route
    // @ts-expect-error dynamic
    await config?.options?.beforeLoad?.()
  }

  describe('_shell/index beforeLoad', () => {
    beforeEach(() => {
      redirectMock.mockClear()
      vi.mocked(fetchProjects).mockReset()
      vi.mocked(useProjectStore.getState).mockReturnValue({ activeProjectId: '' })
    })

    it('redirects to /oobe when no projects', async () => {
      vi.mocked(fetchProjects).mockResolvedValueOnce([])
      try {
        await runBeforeLoad()
      } catch (e) {
        expect(redirectMock).toHaveBeenCalledWith({ to: '/oobe' })
        return
      }
      // redirect should have thrown
      expect.fail('expected redirect to throw')
    })

    it('redirects to home route of active project', async () => {
      vi.mocked(fetchProjects).mockResolvedValueOnce([
        { id: 'p1', name: 'Rabbyte', path: '/a', lastActivity: '2024-01-01' },
        { id: 'p2', name: 'Other', path: '/b', lastActivity: '2024-01-01' },
      ])
      vi.mocked(useProjectStore.getState).mockReturnValue({ activeProjectId: 'p1' })
      try {
        await runBeforeLoad()
      } catch (e) {
        expect(redirectMock).toHaveBeenCalledWith({
          to: '/ide/$projectId/home',
          params: { projectId: 'p1' },
        })
        return
      }
      expect.fail('expected redirect to throw')
    })

    it('falls back to first project when no active project', async () => {
      vi.mocked(fetchProjects).mockResolvedValueOnce([
        { id: 'p2', name: 'Other', path: '/b', lastActivity: '2024-01-01' },
      ])
      vi.mocked(useProjectStore.getState).mockReturnValue({ activeProjectId: '' })
      try {
        await runBeforeLoad()
      } catch (e) {
        expect(redirectMock).toHaveBeenCalledWith({
          to: '/ide/$projectId/home',
          params: { projectId: 'p2' },
        })
        return
      }
      expect.fail('expected redirect to throw')
    })
  })
  ```

- [ ] **Step 2: Run the test to confirm it fails**

  ```bash
  cd web && npx vitest run src/__tests__/routes/_shell/index.test.ts
  ```

  Expected: fails because the beforeLoad still redirects to the workspace route.

- [ ] **Step 3: Update `web/src/routes/_shell/index.tsx`**

  Replace the entire file:
  ```tsx
  import { useState } from 'react'
  import { GitBranchIcon } from 'lucide-react'
  import { createFileRoute, redirect } from '@tanstack/react-router'
  import { fetchProjects } from '@/lib/api'
  import { useProjectStore } from '@/lib/store/projects'
  import {
    Empty,
    EmptyMedia,
    EmptyHeader,
    EmptyTitle,
    EmptyDescription,
  } from '@/components/ui/empty'
  import { Button } from '@/components/ui/button'
  import { AddRepositoryModal } from '@/components/projects/add-repository-modal'

  function NoReposScreen() {
    const [open, setOpen] = useState(false)

    return (
      <>
        <Empty>
          <EmptyMedia variant="icon">
            <GitBranchIcon />
          </EmptyMedia>
          <EmptyHeader>
            <EmptyTitle>No repositories yet</EmptyTitle>
            <EmptyDescription>Add a git repository to open a workspace.</EmptyDescription>
          </EmptyHeader>
          <Button onClick={() => setOpen(true)}>Add repository</Button>
        </Empty>
        <AddRepositoryModal open={open} onOpenChange={setOpen} />
      </>
    )
  }

  export const Route = createFileRoute('/_shell/')({
    component: NoReposScreen,
    beforeLoad: async () => {
      const projects = await fetchProjects()
      if (projects.length === 0) {
        throw redirect({ to: '/oobe' })
      }

      const activeId = useProjectStore.getState().activeProjectId
      const project = projects.find((p) => p.id === activeId) ?? projects[0]

      throw redirect({
        to: '/ide/$projectId/home',
        params: { projectId: project.id },
      })
    },
  })
  ```

- [ ] **Step 4: Run the test**

  ```bash
  cd web && npx vitest run src/__tests__/routes/_shell/index.test.ts
  ```

  Expected: all 3 tests pass.

- [ ] **Step 5: Run the full test suite**

  ```bash
  cd web && npx vitest run
  ```

  Expected: all tests pass. Pay attention to any failures in `workspace-tree.test.ts` or other files that may reference the old redirect behavior.

- [ ] **Step 6: Type-check**

  ```bash
  cd web && npx tsc --noEmit
  ```

  Expected: no errors.

- [ ] **Step 7: Commit**

  ```bash
  git add web/src/routes/_shell/index.tsx \
          web/src/__tests__/routes/_shell/index.test.ts
  git commit -m "feat(routing): redirect to Project Home by default instead of walking repo workspaces"
  ```
