# Context Pill Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Zen-style "you are here" context pill on top of the sidebar tab bar that shows `[status icon] reponame/branchname` (or the project name when no workspace is active) and, on click, switches the sidebar to the Workspaces tab.

**Architecture:** A pure model function (`deriveContextPillModel`) maps existing store/route values to a small discriminated union; a presentational component (`ContextPill`) reads stores + route via narrow selectors, computes the model, and renders it with shadcn `Button` + existing tokens, reusing the existing `WorkspaceBranchIcon`. Mounted in `ide-shell.tsx` directly above `SidebarTabBar`, inside the existing `!hasNavScreen` block.

**Tech Stack:** React + TypeScript, Zustand stores, TanStack Router, Vitest + Testing Library, Tailwind/shadcn tokens.

**Spec:** `docs/superpowers/specs/2026-06-15-context-pill-design.md`

---

## File Structure

- **Create** `web/src/components/layout/context-pill-model.ts` — pure `deriveContextPillModel(...)` + `ContextPillModel` type. No React, no store imports beyond types. Single responsibility: input values → display model.
- **Create** `web/src/components/layout/context-pill.tsx` — `ContextPill` component. Reads route + stores, renders the pill, handles click.
- **Modify** `web/src/components/layout/ide-shell.tsx` — mount `<ContextPill />` above `<SidebarTabBar />`.
- **Create** `web/src/__tests__/components/layout/context-pill-model.test.ts` — unit tests for the pure function.
- **Create** `web/src/__tests__/components/layout/context-pill.test.tsx` — render + interaction tests for the component.

Conventions (from CLAUDE.md): kebab-case files, PascalCase exports, tests mirror `src/` under `__tests__/` with `@/` imports, narrow store selectors, `getState()` only in handlers.

---

### Task 1: Pure model function

**Files:**
- Create: `web/src/components/layout/context-pill-model.ts`
- Test: `web/src/__tests__/components/layout/context-pill-model.test.ts`

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/components/layout/context-pill-model.test.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { deriveContextPillModel } from '@/components/layout/context-pill-model'
import type { Repo } from '@/lib/store/sidebar'
import type { Project } from '@/lib/types'

const repos: Repo[] = [
  {
    id: 'r1',
    projectId: 'p1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces: [
      { id: 'ws1', branch: 'ide-polish', status: 'pr-open', age: '1d' },
      { id: 'ws2', branch: 'no-status', age: '2d' },
    ],
  },
]

const projects: Project[] = [
  { id: 'p1', name: 'Crowbar', path: '/x', lastActivity: new Date(0) },
]

describe('deriveContextPillModel', () => {
  it('returns workspace model when the active workspace resolves', () => {
    const model = deriveContextPillModel({
      activeWorkspaceId: 'ws1',
      repos,
      projects,
      activeProjectId: 'p1',
    })
    expect(model).toEqual({
      kind: 'workspace',
      status: 'pr-open',
      repoName: 'crowbar',
      branchName: 'ide-polish',
    })
  })

  it('falls back to status "new" when the workspace has no status', () => {
    const model = deriveContextPillModel({
      activeWorkspaceId: 'ws2',
      repos,
      projects,
      activeProjectId: 'p1',
    })
    expect(model).toMatchObject({ kind: 'workspace', status: 'new', branchName: 'no-status' })
  })

  it('returns project model when no workspace is active', () => {
    const model = deriveContextPillModel({
      activeWorkspaceId: undefined,
      repos,
      projects,
      activeProjectId: 'p1',
    })
    expect(model).toEqual({ kind: 'project', projectName: 'Crowbar' })
  })

  it('returns project model when the workspace id does not resolve', () => {
    const model = deriveContextPillModel({
      activeWorkspaceId: 'missing',
      repos,
      projects,
      activeProjectId: 'p1',
    })
    expect(model).toEqual({ kind: 'project', projectName: 'Crowbar' })
  })

  it('returns empty when nothing resolves', () => {
    const model = deriveContextPillModel({
      activeWorkspaceId: undefined,
      repos,
      projects,
      activeProjectId: '',
    })
    expect(model).toEqual({ kind: 'empty' })
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/components/layout/context-pill-model.test.ts`
Expected: FAIL — cannot resolve `@/components/layout/context-pill-model`.

- [ ] **Step 3: Write minimal implementation**

Create `web/src/components/layout/context-pill-model.ts`:

```ts
import type { Repo, WorkspaceStatus } from '@/lib/store/sidebar'
import type { Project } from '@/lib/types'

export type ContextPillModel =
  | { kind: 'workspace'; status: WorkspaceStatus; repoName: string; branchName: string }
  | { kind: 'project'; projectName: string }
  | { kind: 'empty' }

interface DeriveArgs {
  activeWorkspaceId: string | undefined
  repos: Repo[]
  projects: Project[]
  activeProjectId: string
}

/**
 * Maps current route/store values to what the context pill should display.
 * Workspace context wins; otherwise fall back to the active project name.
 */
export function deriveContextPillModel({
  activeWorkspaceId,
  repos,
  projects,
  activeProjectId,
}: DeriveArgs): ContextPillModel {
  if (activeWorkspaceId) {
    const repo = repos.find((r) => r.workspaces?.some((ws) => ws.id === activeWorkspaceId))
    const workspace = repo?.workspaces.find((ws) => ws.id === activeWorkspaceId)
    if (repo && workspace) {
      return {
        kind: 'workspace',
        status: workspace.status ?? 'new',
        repoName: repo.name,
        branchName: workspace.branch,
      }
    }
  }

  const project = projects.find((p) => p.id === activeProjectId)
  if (project) {
    return { kind: 'project', projectName: project.name }
  }

  return { kind: 'empty' }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/components/layout/context-pill-model.test.ts`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/context-pill-model.ts web/src/__tests__/components/layout/context-pill-model.test.ts
git commit -m "feat(sidebar): context pill display model"
```

---

### Task 2: ContextPill component

**Files:**
- Create: `web/src/components/layout/context-pill.tsx`
- Test: `web/src/__tests__/components/layout/context-pill.test.tsx`

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/components/layout/context-pill.test.tsx`:

```tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import { ContextPill } from '@/components/layout/context-pill'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useProjectStore } from '@/lib/store/projects'

let mockPathname = '/'
vi.mock('@tanstack/react-router', () => ({
  useRouterState: ({ select }: { select: (s: { location: { pathname: string } }) => unknown }) =>
    select({ location: { pathname: mockPathname } }),
}))

const repos: Repo[] = [
  {
    id: 'r1',
    projectId: 'p1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces: [{ id: 'ws1', branch: 'ide-polish', status: 'pr-open', age: '1d' }],
  },
]

beforeEach(() => {
  mockPathname = '/'
  useSidebarStore.setState({ repos, activeTab: 'files' })
  useProjectStore.setState({
    projects: [{ id: 'p1', name: 'Crowbar', path: '/x', lastActivity: new Date(0) }],
    activeProjectId: 'p1',
  })
})

describe('ContextPill', () => {
  it('renders reponame/branchname in workspace mode', () => {
    mockPathname = '/workspaces/ws1'
    render(<ContextPill />)
    expect(screen.getByText('crowbar')).toBeInTheDocument()
    expect(screen.getByText('ide-polish')).toBeInTheDocument()
  })

  it('renders the project name when no workspace is active', () => {
    mockPathname = '/'
    render(<ContextPill />)
    expect(screen.getByText('Crowbar')).toBeInTheDocument()
  })

  it('switches the sidebar to the workspaces tab on click', () => {
    mockPathname = '/workspaces/ws1'
    render(<ContextPill />)
    fireEvent.click(screen.getByRole('button'))
    expect(useSidebarStore.getState().activeTab).toBe('workspaces')
  })

  it('renders nothing when nothing resolves', () => {
    mockPathname = '/'
    useProjectStore.setState({ projects: [], activeProjectId: '' })
    const { container } = render(<ContextPill />)
    expect(container).toBeEmptyDOMElement()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/components/layout/context-pill.test.tsx`
Expected: FAIL — cannot resolve `@/components/layout/context-pill`.

- [ ] **Step 3: Write minimal implementation**

Create `web/src/components/layout/context-pill.tsx`:

```tsx
import { useRouterState } from '@tanstack/react-router'
import { Button } from '@/components/ui/button'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useProjectStore } from '@/lib/store/projects'
import { WorkspaceBranchIcon } from './workspace-branch-icon'
import { deriveContextPillModel } from './context-pill-model'

/**
 * "You are here" pill above the sidebar tab bar: shows the current
 * workspace (status icon + reponame/branchname) or the active project name.
 * Clicking it jumps the sidebar to the Workspaces tab.
 */
export function ContextPill() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const repos = useSidebarStore((s) => s.repos)
  const projects = useProjectStore((s) => s.projects)
  const activeProjectId = useProjectStore((s) => s.activeProjectId)

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1]
  const model = deriveContextPillModel({ activeWorkspaceId, repos, projects, activeProjectId })

  if (model.kind === 'empty') return null

  return (
    <div className="shrink-0 px-2 pt-2 pb-1">
      <Button
        variant="ghost"
        onClick={() => useSidebarStore.getState().setActiveTab('workspaces')}
        className="h-8 w-full justify-start gap-2 rounded-[10px] bg-foreground/5 px-3 text-[13px] font-normal hover:bg-foreground/10"
      >
        {model.kind === 'workspace' ? (
          <span className="flex min-w-0 items-center gap-2">
            <WorkspaceBranchIcon status={model.status} />
            <span className="truncate">
              <span className="text-muted-foreground">{model.repoName}</span>
              <span className="text-muted-foreground">/</span>
              <span className="font-semibold text-foreground">{model.branchName}</span>
            </span>
          </span>
        ) : (
          <span className="truncate text-foreground">{model.projectName}</span>
        )}
      </Button>
    </div>
  )
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/components/layout/context-pill.test.tsx`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/context-pill.tsx web/src/__tests__/components/layout/context-pill.test.tsx
git commit -m "feat(sidebar): context pill component"
```

---

### Task 3: Mount the pill in IDEShell

**Files:**
- Modify: `web/src/components/layout/ide-shell.tsx` (import + `sidebarContent` block ~line 95-108)

- [ ] **Step 1: Add the import**

In `web/src/components/layout/ide-shell.tsx`, add alongside the other layout imports (after the `SidebarTabBar` import on line 7):

```tsx
import { ContextPill } from './context-pill'
```

- [ ] **Step 2: Render the pill above the tab bar**

In the `sidebarContent` JSX, change:

```tsx
      {!hasNavScreen && <SidebarTabBar />}
```

to:

```tsx
      {!hasNavScreen && <ContextPill />}
      {!hasNavScreen && <SidebarTabBar />}
```

- [ ] **Step 3: Typecheck + full test run**

Run: `cd web && npx tsc --noEmit && npx vitest run src/__tests__/components/layout/`
Expected: tsc clean; context-pill tests PASS.

- [ ] **Step 4: Verify live in Tauri (REQUIRED — do not claim done without this)**

Per project rule: an editor/UI change is not "working" until sampled live in the running Tauri app (tests/tsc ≠ visible result). Launch the app, then confirm by observation:
- The pill appears directly above the Workspaces/Chats/Files/Git tab bar.
- Inside a workspace it shows `[status icon] reponame/branchname` (repo muted, branch bright); the icon matches the workspace's status.
- On a screen with no active workspace it shows the project name.
- Clicking the pill switches the sidebar to the Workspaces tab and does nothing else.
- It hides when a nav-screen overlay is open (together with the tab bar).
- Long repo/branch names truncate with ellipsis rather than overflowing.

Use the `run` skill (or the Tauri MCP webview tools) to launch and observe; capture a screenshot.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/ide-shell.tsx
git commit -m "feat(sidebar): mount context pill above the tab bar"
```

---

## Self-Review

**Spec coverage:**
- Content `[status icon] reponame/branchname` → Task 1 model + Task 2 render (reuses `WorkspaceBranchIcon`). ✓
- Repo muted / branch bright → Task 2 classes (`text-muted-foreground` / `font-semibold text-foreground`). ✓
- No active workspace → project name → Task 1 `project` branch + Task 2 render. ✓
- Empty fallback renders nothing → Task 1 `empty` + Task 2 `return null`. ✓
- Compact styling, shadcn Button + tokens, truncation → Task 2. ✓
- Placement above tab bar, inside `!hasNavScreen` → Task 3. ✓
- Click → `setActiveTab('workspaces')`, nothing else → Task 2 handler + test. ✓
- Tests mirror `src/` under `__tests__/` with `@/` imports → all test paths. ✓
- Live Tauri verification → Task 3 Step 4. ✓

**Placeholder scan:** No TBD/TODO/"handle edge cases"; every code step has full code. ✓

**Type consistency:** `ContextPillModel` discriminants (`workspace`/`project`/`empty`) and fields (`status`/`repoName`/`branchName`/`projectName`) are identical across `deriveContextPillModel`, its tests, and `ContextPill`. `WorkspaceStatus`, `Repo`, `Project` imported from their real modules. ✓

**Note on styling tokens:** `bg-foreground/5` / `hover:bg-foreground/10` are the starting translucent values; during Task 3 Step 4 (live verify), nudge to match neighbouring sidebar surfaces if it doesn't blend — tokens only, no hardcoded colors.
