# Sidebar Project Switcher Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the sidebar header's project dropdown with a thin entry-row at the top of the Workspaces tab that opens a pushed sidebar screen for switching between and importing projects.

**Architecture:** A thin `ProjectSwitcherRow` (Image #21 style) renders as the first element inside `WorkspaceTree`. Clicking it pushes a `ProjectSwitcherPanel` onto the existing `useSidebarNavStore` stack (the same mechanism `RepoSettingsPanel` uses). The panel lists projects as workspace-style rows and reuses the existing `ImportProjectModal` for adding projects. The header's dropdown is removed.

**Tech Stack:** React + TypeScript, Zustand stores, Vitest + @testing-library/react, Tailwind (CSS-variable tokens), lucide-react icons.

---

## File Structure

- **Create** `web/src/components/layout/project-switcher-panel.tsx` — pushed-screen body: project list + import row.
- **Create** `web/src/components/layout/project-switcher-row.tsx` — thin entry-point row; pushes the panel.
- **Create** `web/src/__tests__/components/layout/project-switcher-panel.test.tsx` — panel behavior.
- **Create** `web/src/__tests__/components/layout/project-switcher-row.test.tsx` — row behavior.
- **Modify** `web/src/components/layout/workspace-tree.tsx` — render `<ProjectSwitcherRow />` above the repo list.
- **Modify** `web/src/components/layout/sidebar-project-header.tsx` — remove the project dropdown + unused props.
- **Modify** `web/src/components/layout/ide-shell.tsx:100-104` — drop the removed props from the `SidebarProjectHeader` call.

Reference facts used below (already verified in the codebase):
- `useSidebarNavStore` API: `push({ id, title, component })`, `pop()`, `reset()` (`web/src/features/layout/stores/sidebar-nav.ts`).
- `useProjectStore` selectors: `projects: Project[]`, `activeProjectId: string`, `setActiveProject(id)`, `addProject(project)` (`web/src/lib/store/projects.ts`).
- `Project` = `{ id: string; name: string; path: string; lastActivity: Date }` (`web/src/lib/types.ts`).
- `ImportProjectModal` props: `{ open, onOpenChange, onImport }`; its `DialogTitle` text is `Import project` (`web/src/components/projects/import-project-modal.tsx`).
- `ROW_BASE` constant (`web/src/components/layout/workspace-row-base.ts`).
- The import-success side effects pattern (copy from `web/src/components/projects/project-list-page.tsx` `handleImport`).

---

## Task 1: `ProjectSwitcherPanel` (pushed-screen body)

**Files:**
- Create: `web/src/components/layout/project-switcher-panel.tsx`
- Test: `web/src/__tests__/components/layout/project-switcher-panel.test.tsx`

- [ ] **Step 1: Write the failing test**

```tsx
// web/src/__tests__/components/layout/project-switcher-panel.test.tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ProjectSwitcherPanel } from '@/components/layout/project-switcher-panel'
import { useProjectStore } from '@/lib/store/projects'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'

const PROJECTS = [
  { id: 'p1', name: 'Rabbyte', path: '/a', lastActivity: new Date(0) },
  { id: 'p2', name: 'Quiver', path: '/b', lastActivity: new Date(0) },
]

beforeEach(() => {
  useSidebarNavStore.getState().reset()
  useProjectStore.setState({ projects: PROJECTS, activeProjectId: 'p1' })
})

describe('ProjectSwitcherPanel', () => {
  it('renders a row per project and marks the active one', () => {
    render(<ProjectSwitcherPanel />)
    expect(screen.getByRole('button', { name: /Rabbyte/ })).toHaveAttribute(
      'aria-current',
      'true',
    )
    expect(screen.getByRole('button', { name: /Quiver/ })).toHaveAttribute(
      'aria-current',
      'false',
    )
  })

  it('switches the active project and pops the stack when a row is clicked', async () => {
    const setActiveProject = vi.spyOn(useProjectStore.getState(), 'setActiveProject')
    useSidebarNavStore.getState().push({ id: 'x', title: 'x', component: null })
    render(<ProjectSwitcherPanel />)
    await userEvent.click(screen.getByRole('button', { name: /Quiver/ }))
    expect(setActiveProject).toHaveBeenCalledWith('p2')
    expect(useSidebarNavStore.getState().stack).toHaveLength(0)
  })

  it('opens the import modal when the import row is clicked', async () => {
    render(<ProjectSwitcherPanel />)
    await userEvent.click(screen.getByRole('button', { name: /Import project/i }))
    // The dialog title is a heading, distinct from the trigger row button.
    // (`getByText` would be ambiguous — the text appears on both.)
    expect(screen.getByRole('heading', { name: 'Import project' })).toBeTruthy()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/components/layout/project-switcher-panel.test.tsx`
Expected: FAIL — cannot resolve `@/components/layout/project-switcher-panel`.

- [ ] **Step 3: Write minimal implementation**

```tsx
// web/src/components/layout/project-switcher-panel.tsx
import { useState } from 'react'
import { Plus } from 'lucide-react'
import { cn } from '@/lib/utils'
import { ROW_BASE } from './workspace-row-base'
import { ImportProjectModal } from '@/components/projects/import-project-modal'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'
import { useProjectStore, useProjectDataStore } from '@/lib/store/projects'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useSidebarStore } from '@/lib/store/sidebar'
import { dataOf } from '@/lib/loadable'
import type { Project } from '@/lib/types'

/**
 * Body of the pushed "Projects" sidebar screen. NavStack supplies the
 * back button + title; this renders the project list and the import row.
 */
export function ProjectSwitcherPanel() {
  const projects = useProjectStore((s) => s.projects)
  const activeProjectId = useProjectStore((s) => s.activeProjectId)
  const [importOpen, setImportOpen] = useState(false)

  function handleSelect(id: string) {
    useProjectStore.getState().setActiveProject(id)
    useSidebarNavStore.getState().pop()
  }

  // Mirror ProjectListPage.handleImport: add to the live store, refetch the
  // projects + workspace lists, and merge the new repos into the sidebar tree.
  function handleImport(project: Project) {
    useProjectStore.getState().addProject(project)
    setImportOpen(false)
    void useProjectDataStore.getState().fetch()
    void useWorkspaceListStore
      .getState()
      .fetch()
      .then(() => {
        const repos = dataOf(useWorkspaceListStore.getState().data)
        if (repos) useSidebarStore.getState().mergeRepos(repos)
      })
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex flex-col py-1">
        {projects.map((p) => {
          const isActive = p.id === activeProjectId
          return (
            <button
              key={p.id}
              type="button"
              aria-current={isActive ? 'true' : 'false'}
              onClick={() => handleSelect(p.id)}
              className={cn(
                ROW_BASE,
                'border-transparent text-left hover:bg-accent',
                isActive && 'bg-accent/60 text-foreground',
              )}
            >
              <span className="min-w-0 flex-1 truncate font-mono">{p.name}</span>
            </button>
          )
        })}

        <button
          type="button"
          onClick={() => setImportOpen(true)}
          className={cn(
            ROW_BASE,
            'border-transparent text-muted-foreground hover:bg-accent hover:text-foreground',
          )}
        >
          <Plus className="size-3.5 shrink-0" />
          <span className="min-w-0 flex-1 truncate text-left">Import project</span>
        </button>
      </div>

      <ImportProjectModal
        open={importOpen}
        onOpenChange={setImportOpen}
        onImport={handleImport}
      />
    </div>
  )
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/components/layout/project-switcher-panel.test.tsx`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/project-switcher-panel.tsx web/src/__tests__/components/layout/project-switcher-panel.test.tsx
git commit -m "feat(sidebar): project switcher panel (pushed screen)"
```

---

## Task 2: `ProjectSwitcherRow` (thin entry-point row)

**Files:**
- Create: `web/src/components/layout/project-switcher-row.tsx`
- Test: `web/src/__tests__/components/layout/project-switcher-row.test.tsx`

- [ ] **Step 1: Write the failing test**

```tsx
// web/src/__tests__/components/layout/project-switcher-row.test.tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, beforeEach } from 'vitest'
import { ProjectSwitcherRow } from '@/components/layout/project-switcher-row'
import { useProjectStore } from '@/lib/store/projects'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'

beforeEach(() => {
  useSidebarNavStore.getState().reset()
  useProjectStore.setState({
    projects: [{ id: 'p1', name: 'Rabbyte', path: '/a', lastActivity: new Date(0) }],
    activeProjectId: 'p1',
  })
})

describe('ProjectSwitcherRow', () => {
  it('shows the active project name', () => {
    render(<ProjectSwitcherRow />)
    expect(screen.getByRole('button', { name: /Rabbyte/ })).toBeTruthy()
  })

  it('pushes the project switcher screen on click', async () => {
    render(<ProjectSwitcherRow />)
    await userEvent.click(screen.getByRole('button', { name: /Rabbyte/ }))
    const stack = useSidebarNavStore.getState().stack
    expect(stack).toHaveLength(1)
    expect(stack[0].id).toBe('project-switcher')
    expect(stack[0].title).toBe('Projects')
  })

  it('falls back to "Select project" when none is active', () => {
    useProjectStore.setState({ projects: [], activeProjectId: '' })
    render(<ProjectSwitcherRow />)
    expect(screen.getByRole('button', { name: /Select project/ })).toBeTruthy()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/components/layout/project-switcher-row.test.tsx`
Expected: FAIL — cannot resolve `@/components/layout/project-switcher-row`.

- [ ] **Step 3: Write minimal implementation**

```tsx
// web/src/components/layout/project-switcher-row.tsx
import { ChevronRight } from 'lucide-react'
import { useProjectStore } from '@/lib/store/projects'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'
import { ProjectSwitcherPanel } from './project-switcher-panel'

/**
 * Thin entry-point row at the top of the Workspaces tab. Shows the active
 * project and opens the project switcher as a pushed sidebar screen.
 */
export function ProjectSwitcherRow() {
  const projects = useProjectStore((s) => s.projects)
  const activeProjectId = useProjectStore((s) => s.activeProjectId)
  const activeProject = projects.find((p) => p.id === activeProjectId)

  function open() {
    useSidebarNavStore.getState().push({
      id: 'project-switcher',
      title: 'Projects',
      component: <ProjectSwitcherPanel />,
    })
  }

  return (
    <button
      type="button"
      onClick={open}
      className="flex w-full items-center gap-2 border-b border-border/60 px-3 py-2 text-left text-xs text-muted-foreground transition-colors hover:text-foreground"
    >
      <span className="min-w-0 flex-1 truncate font-mono">
        {activeProject?.name ?? 'Select project'}
      </span>
      <ChevronRight className="size-3.5 shrink-0 text-muted-foreground/60" />
    </button>
  )
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/components/layout/project-switcher-row.test.tsx`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/project-switcher-row.tsx web/src/__tests__/components/layout/project-switcher-row.test.tsx
git commit -m "feat(sidebar): thin project switcher entry row"
```

---

## Task 3: Mount the row in `WorkspaceTree`

**Files:**
- Modify: `web/src/components/layout/workspace-tree.tsx`

- [ ] **Step 1: Add the import**

In `web/src/components/layout/workspace-tree.tsx`, add after the existing `RepoSettingsPanel` import (line 13):

```tsx
import { ProjectSwitcherRow } from './project-switcher-row'
```

- [ ] **Step 2: Render the row above the repo list**

Replace the opening of the returned tree container (currently lines 82-85):

```tsx
  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <ScrollArea className="flex-1">
        <div className="py-1">
```

with:

```tsx
  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <ProjectSwitcherRow />
      <ScrollArea className="flex-1">
        <div className="py-1">
```

- [ ] **Step 3: Verify the existing tree tests still pass**

Run: `cd web && npx vitest run src/__tests__/components/layout/workspace-tree.test.ts src/__tests__/components/layout/workspace-tree-error.test.tsx`
Expected: PASS (unchanged — these test `buildWorkspaceTree` / error state, not the row).

- [ ] **Step 4: Typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/workspace-tree.tsx
git commit -m "feat(sidebar): mount project switcher row atop workspaces tab"
```

---

## Task 4: Remove the dropdown from `SidebarProjectHeader`

**Files:**
- Modify: `web/src/components/layout/sidebar-project-header.tsx`
- Modify: `web/src/components/layout/ide-shell.tsx:100-104`

- [ ] **Step 1: Replace `sidebar-project-header.tsx` with the dropdown-free version**

Full new file contents:

```tsx
// web/src/components/layout/sidebar-project-header.tsx
import { useSettingsStore } from '@/features/settings/store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { IS_MAC } from '@/utils/platform'
import { cn } from '@/utils/cn'
import { GearSix } from '@phosphor-icons/react'

export function SidebarProjectHeader() {
  const sidebarPosition = useSettingsStore((s) => s.settings.sidebarPosition)
  const isRight = sidebarPosition === 'right'

  return (
    <div
      className={cn(
        'flex w-full flex-shrink-0 items-center px-3',
        IS_MAC ? 'h-[44px]' : 'h-[34px]',
      )}
      data-tauri-drag-region
    >
      {IS_MAC && !isRight && <div className="w-[52px] shrink-0" />}

      <button
        onClick={() => useUIState.getState().openSettingsDialog()}
        aria-label="Settings"
        className={cn(
          'flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground',
          isRight ? 'mr-auto' : 'ml-auto',
        )}
      >
        <GearSix size={16} />
      </button>
    </div>
  )
}
```

Note: the gear keeps its edge alignment via `ml-auto` (or `mr-auto` when the sidebar is on the right), preserving the previous layout now that the dropdown that anchored it is gone.

- [ ] **Step 2: Update the call site in `ide-shell.tsx`**

Replace lines 100-104:

```tsx
        <SidebarProjectHeader
          onProjectsClick={() => void navigate({ to: '/projects' })}
          onProjectSelect={() => void navigate({ to: '/' })}
        />
```

with:

```tsx
        <SidebarProjectHeader />
```

- [ ] **Step 3: Verify `navigate` is still used elsewhere in `ide-shell.tsx`**

Run: `cd web && grep -n "navigate(" src/components/layout/ide-shell.tsx`
Expected: at least one remaining usage (the projects-route nav helper passed elsewhere). If `navigate` becomes unused, remove its declaration (`const navigate = useNavigate()`) and the `useNavigate` import to satisfy the linter.

- [ ] **Step 4: Typecheck + run the affected layout tests**

Run: `cd web && npx tsc --noEmit && npx vitest run src/__tests__/components/layout/ide-shell.test.tsx`
Expected: no type errors; `ide-shell.test.tsx` passes (it mocks `SidebarProjectHeader` as a stub, so the prop removal does not affect it).

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/sidebar-project-header.tsx web/src/components/layout/ide-shell.tsx
git commit -m "feat(sidebar): drop project dropdown from header"
```

---

## Task 5: Full verification

- [ ] **Step 1: Run the full web test suite**

Run: `cd web && npx vitest run`
Expected: all tests pass.

- [ ] **Step 2: Typecheck the whole web app**

Run: `cd web && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 3: Lint the changed files**

Run: `cd web && npx eslint src/components/layout/project-switcher-panel.tsx src/components/layout/project-switcher-row.tsx src/components/layout/workspace-tree.tsx src/components/layout/sidebar-project-header.tsx src/components/layout/ide-shell.tsx`
Expected: no errors.

- [ ] **Step 4: Manual check in Tauri (per project rule: verify live before claiming done)**

Launch the app and confirm:
- The Workspaces tab shows the thin project row at the very top (Image #21 style).
- The header no longer has the `Rabbyte ⌄` dropdown; the gear remains.
- Clicking the row slides in the "Projects" screen with a back button.
- Each project row switches the active project and returns to Workspaces.
- "Import project" opens the existing import modal.
- The row is absent on Chats / Files / Git tabs.

---

## Notes for the implementer

- Follow project conventions: kebab-case component files, PascalCase exports, narrow Zustand selectors in render, `getState()` only in handlers, `@/` imports in tests.
- Do not add a new store field or backend endpoint — everything routes through existing stores and `ImportProjectModal`.
- The thin row style is intentionally lighter than `ROW_BASE` (which is the pill-row used for repos/projects-in-panel); keep the entry row flat with the bottom hairline.
