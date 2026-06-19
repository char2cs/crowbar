# OOBE Empty States Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Crowbar's barren empty states with a two-screen OOBE: a full-canvas glass `/oobe` route (no shell) for first launch, and a CrossUI `Empty`-pattern empty state inside the IDE shell for when a project exists but has no repositories.

**Architecture:** Move `IDEShell` from `__root.tsx` into a new `_shell.tsx` pathless layout route so that `/oobe` can render completely outside the shell. The `/oobe` route renders a full-bleed glass component using the Crowbar app icon. The existing `/projects` route's empty state is upgraded to use the `Empty` component with a repo-focused CTA.

**Tech Stack:** TanStack Router (file-based, v1), React, Tailwind CSS, `@coss/empty` (`components/ui/empty.tsx`), `@tanstack/react-router` `createFileRoute`, `useNavigate`, `redirect`.

## Global Constraints

- All new component files use kebab-case filenames; exported components are PascalCase.
- Use `@/` imports only — no relative `../../` imports.
- Use `@/components/ui/*` and CSS variable tokens — no hardcoded colours.
- Use narrow Zustand selectors — `useStore((s) => s.specificField)`, never `useStore()`.
- Test files live in `web/src/__tests__/` mirroring `web/src/` structure.
- Run all commands from the `web/` directory.

---

### Task 1: Create CrowbarIcon React component

**Files:**
- Create: `web/src/components/ui/crowbar-icon.tsx`

**Interfaces:**
- Produces: `CrowbarIcon` — `React.FC<React.SVGProps<SVGSVGElement>>`, renders the Crowbar mark SVG with `fill="currentColor"` so callers control colour via `className` (e.g. `className="text-foreground"` or `className="text-muted-foreground"`). Size defaults to `1em × 1em`; callers override with `width`/`height` or Tailwind `size-*`.

- [ ] **Step 1: Create the component**

The SVG path comes from `~/Downloads/Crowbar/Union.svg`. Create `web/src/components/ui/crowbar-icon.tsx`:

```tsx
import type React from 'react'

export function CrowbarIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      viewBox="0 0 146 145"
      fill="currentColor"
      xmlns="http://www.w3.org/2000/svg"
      width="1em"
      height="1em"
      {...props}
    >
      <path d="M72.873 15C104.836 15 130.747 40.7436 130.747 72.5C130.747 104.256 104.836 130 72.873 130C40.9106 130 15 104.256 15 72.5C15 40.7437 40.9106 15.0002 72.873 15ZM74.9004 42.8174C72.2697 42.3669 69.5369 42.7093 66.542 44.4268C61.4646 47.3388 57.6943 50.8091 56.4541 56.2373C54.5839 64.4226 51.3599 72.0406 47.9434 79.4844C47.4037 80.66 46.7269 81.8276 46.1729 82.8574C45.5963 83.9291 45.1125 84.9197 44.8125 85.9287C42.9488 92.1979 41.3662 99.9677 40.5137 106.489C40.805 106.437 41.1189 106.36 41.3232 106.248C43.5044 105.052 45.6522 103.793 47.8574 102.644C49.1085 101.991 49.6782 101.034 49.9277 99.6807C50.7951 94.972 52.1261 90.3742 53.5264 85.7988C53.6344 85.4454 53.6387 84.9585 54.1865 84.8115C54.5877 88.7651 54.956 92.6482 55.3867 96.5234C55.6002 98.4432 56.2162 98.7468 57.8789 97.8203C61.0656 96.0446 64.2321 94.2327 67.415 92.4502C68.2328 91.9922 68.7259 91.4043 68.9033 90.4219C69.2932 88.2623 69.8475 86.1317 70.3047 83.9834C71.4089 78.7947 73.0867 73.6389 71.3145 68.2656C71.2642 68.1127 71.2941 67.9348 71.2578 67.7754C70.7202 65.4256 71.733 63.5901 73.251 61.9443C74.6855 60.3892 76.2762 60.2829 77.9375 61.5732C78.8633 62.2925 79.6177 63.1636 80.2705 64.1289C82.8309 67.9155 84.5382 72.0859 85.748 76.4551C88.4209 86.108 91.0353 95.7776 93.6719 105.44C94.6917 109.178 95.7912 111.705 96.8447 113.429C101.364 110.81 105.408 107.473 108.823 103.576C107.36 98.3302 105.1 90.5395 102.061 79.501C100.191 72.7122 98.5796 66.1397 95.1748 60.2295C91.9494 54.631 88.3876 49.6184 83.0322 46.3027C80.2471 44.5785 77.5865 43.2774 74.9004 42.8174ZM72.873 25.1533C46.5546 25.1535 25.2197 46.3513 25.2197 72.5C25.2197 84.8411 29.975 96.0764 37.7607 104.504C38.6681 98.1999 40.1742 91.0056 41.9375 85.0742C42.3288 83.758 42.9347 82.5452 43.5312 81.4365C44.1503 80.286 44.7299 79.294 45.2168 78.2334C48.6182 70.8227 51.7325 63.437 53.5303 55.5684C55.0427 48.9493 59.6644 44.9123 65.0488 41.8242C68.6535 39.757 72.0946 39.293 75.4072 39.8604C78.6643 40.4182 81.7172 41.9603 84.6113 43.752L85.168 44.1055C90.859 47.8174 94.5872 53.2002 97.7744 58.7324C101.415 65.0516 103.125 72.064 104.953 78.7041C107.65 88.5 109.672 95.5276 111.137 100.723C117.036 92.8415 120.527 83.0764 120.527 72.5C120.527 46.3512 99.1916 25.1533 72.873 25.1533Z" />
    </svg>
  )
}
```

- [ ] **Step 2: Type-check**

```bash
npm run typecheck
# Expected: 0 errors
```

- [ ] **Step 3: Commit**

```bash
git add src/components/ui/crowbar-icon.tsx
git commit -m "feat: add CrowbarIcon SVG component"
```

---

### Task 2: Restructure routing — `_shell.tsx` pathless layout

Move `IDEShell` out of `__root.tsx` into a pathless layout route so `/oobe` can render without the shell.

**Files:**
- Modify: `web/src/routes/__root.tsx`
- Create: `web/src/routes/_shell.tsx`
- Create: `web/src/routes/_shell/` directory containing all current routes except `__root.tsx`
  - Move: `web/src/routes/index.tsx` → `web/src/routes/_shell/index.tsx`
  - Move: `web/src/routes/projects/index.tsx` → `web/src/routes/_shell/projects/index.tsx`
  - Move: `web/src/routes/workspaces/$wsId.tsx` → `web/src/routes/_shell/workspaces/$wsId.tsx`
  - Move: `web/src/routes/workspaces/$wsId/index.tsx` → `web/src/routes/_shell/workspaces/$wsId/index.tsx`
  - Move: `web/src/routes/workspaces/new.tsx` → `web/src/routes/_shell/workspaces/new.tsx`
  - Move: `web/src/routes/chat/$chatId.tsx` → `web/src/routes/_shell/chat/$chatId.tsx`

**Interfaces:**
- Produces: `IDEShell` mounted only for routes under `_shell/`; `__root.tsx` renders providers only

- [ ] **Step 1: Strip `IDEShell` from `__root.tsx`**

Replace the full contents of `web/src/routes/__root.tsx` with:

```tsx
import { createRootRoute, Outlet } from '@tanstack/react-router'
import { ErrorBoundary } from '@/components/error-boundary'
import { HydrationGate } from '@/components/hydration-gate'
import { AppSyncProvider } from '@/components/app-sync-provider'

function RootComponent() {
  return (
    <HydrationGate>
      <ErrorBoundary>
        <AppSyncProvider>
          <Outlet />
        </AppSyncProvider>
      </ErrorBoundary>
    </HydrationGate>
  )
}

export const Route = createRootRoute({
  component: RootComponent,
})
```

- [ ] **Step 2: Create `_shell.tsx`**

Create `web/src/routes/_shell.tsx`:

```tsx
import { createFileRoute } from '@tanstack/react-router'
import { IDEShell } from '@/components/layout/ide-shell'

export const Route = createFileRoute('/_shell')({
  component: IDEShell,
})
```

- [ ] **Step 3: Move route files into `_shell/`**

```bash
mkdir -p src/routes/_shell/projects
mkdir -p src/routes/_shell/workspaces/\$wsId
mkdir -p src/routes/_shell/chat

mv src/routes/index.tsx src/routes/_shell/index.tsx
mv src/routes/projects/index.tsx src/routes/_shell/projects/index.tsx
mv src/routes/workspaces/\$wsId.tsx src/routes/_shell/workspaces/\$wsId.tsx
mv src/routes/workspaces/\$wsId/index.tsx src/routes/_shell/workspaces/\$wsId/index.tsx
mv src/routes/workspaces/new.tsx src/routes/_shell/workspaces/new.tsx
mv src/routes/chat/\$chatId.tsx src/routes/_shell/chat/\$chatId.tsx

# remove now-empty dirs
rmdir src/routes/projects src/routes/workspaces/\$wsId src/routes/workspaces src/routes/chat 2>/dev/null || true
```

- [ ] **Step 4: Update route strings in moved files**

TanStack Router file-based routing encodes the path from the file location. The `createFileRoute` string in each moved file must be updated to include the `/_shell` prefix.

Update `src/routes/_shell/index.tsx`:
```tsx
export const Route = createFileRoute('/_shell/')({
```

Update `src/routes/_shell/projects/index.tsx`:
```tsx
export const Route = createFileRoute('/_shell/projects/')({
```

Update `src/routes/_shell/workspaces/$wsId.tsx`:
```tsx
export const Route = createFileRoute('/_shell/workspaces/$wsId')({
```

Update `src/routes/_shell/workspaces/$wsId/index.tsx`:
```tsx
export const Route = createFileRoute('/_shell/workspaces/$wsId/')({
```

Update `src/routes/_shell/workspaces/new.tsx`:
```tsx
export const Route = createFileRoute('/_shell/workspaces/new')({
```

Update `src/routes/_shell/chat/$chatId.tsx`:
```tsx
export const Route = createFileRoute('/_shell/chat/$chatId')({
```

- [ ] **Step 5: Verify the app still loads**

```bash
npm run dev
# Navigate to http://localhost:5173 — should redirect to /projects or a workspace as before
# The IDE shell (sidebar + resizable panels) should still be visible
```

- [ ] **Step 6: Run type-check**

```bash
npm run typecheck
# Expected: 0 errors
```

- [ ] **Step 7: Commit**

```bash
git add src/routes/
git commit -m "refactor(routing): move IDEShell into _shell pathless layout route"
```

---

### Task 3: Update index redirect — no projects → `/oobe`

**Files:**
- Modify: `web/src/routes/_shell/index.tsx`

**Interfaces:**
- Consumes: `fetchLandingWorkspaceId(): Promise<string | null>` from `@/lib/api`
- Consumes: `fetchProjects(): Promise<Project[]>` from `@/lib/api`
- Produces: redirect to `/oobe` when projects array is empty

- [ ] **Step 1: Update `_shell/index.tsx`**

```tsx
import { createFileRoute, redirect } from '@tanstack/react-router'
import { fetchLandingWorkspaceId, fetchProjects } from '@/lib/api'

export const Route = createFileRoute('/_shell/')({
  component: () => null,
  beforeLoad: async () => {
    let projects: { id: string }[] = []
    try {
      projects = await fetchProjects()
    } catch {
      projects = []
    }

    if (projects.length === 0) {
      throw redirect({ to: '/oobe' })
    }

    let wsId: string | null = null
    try {
      wsId = await fetchLandingWorkspaceId()
    } catch {
      wsId = null
    }

    if (wsId) {
      throw redirect({ to: '/workspaces/$wsId', params: { wsId } })
    }

    throw redirect({ to: '/projects' })
  },
})
```

- [ ] **Step 2: Verify redirect in dev**

```bash
npm run dev
# Clear app state: open devtools → Application → Storage → Clear site data
# Navigate to http://localhost:5173
# Expected: redirected to /oobe (the route doesn't exist yet — you'll see a 404, that's fine)
# With mock data active (projects exist): still redirects to workspace or /projects
```

- [ ] **Step 3: Commit**

```bash
git add src/routes/_shell/index.tsx
git commit -m "feat(routing): redirect to /oobe when no projects exist"
```

---

### Task 4: Create `/oobe` route — Screen 1

**Files:**
- Create: `web/src/routes/oobe.tsx`
- Create: `web/src/components/oobe/oobe-screen.tsx`

**Interfaces:**
- Consumes: `Empty, EmptyMedia, EmptyHeader, EmptyTitle, EmptyDescription, EmptyContent` from `@/components/ui/empty`
- Consumes: `CrowbarIcon` from `@/components/ui/crowbar-icon` (Task 1)
- Consumes: `Button` from `@/components/ui/button`
- Consumes: `ImportProjectModal` from `@/components/projects/import-project-modal`
- Consumes: `importProjectAndSync` from `@/lib/store/projects`
- Consumes: `useNavigate` from `@tanstack/react-router`
- Produces: `/oobe` route rendering `OobeScreen` outside the IDE shell

- [ ] **Step 1: Write the test**

Create `web/src/__tests__/components/oobe/oobe-screen.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { OobeScreen } from '@/components/oobe/oobe-screen'
import { vi } from 'vitest'

vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))
vi.mock('@/lib/store/projects', () => ({ importProjectAndSync: vi.fn() }))

describe('OobeScreen', () => {
  it('renders headline and CTA', () => {
    render(<OobeScreen />)
    expect(screen.getByText('Open a project folder')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /choose folder/i })).toBeInTheDocument()
  })

  it('opens import modal when CTA clicked', async () => {
    render(<OobeScreen />)
    await userEvent.click(screen.getByRole('button', { name: /choose folder/i }))
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
npm run test -- oobe-screen --run
# Expected: FAIL — OobeScreen not found
```

- [ ] **Step 3: Create `OobeScreen` component**

Create `web/src/components/oobe/oobe-screen.tsx`:

```tsx
import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import {
  Empty,
  EmptyMedia,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
  EmptyContent,
} from '@/components/ui/empty'
import { CrowbarIcon } from '@/components/ui/crowbar-icon'
import { Button } from '@/components/ui/button'
import { ImportProjectModal } from '@/components/projects/import-project-modal'
import { importProjectAndSync } from '@/lib/store/projects'
import type { Project } from '@/lib/types'

export function OobeScreen() {
  const [importOpen, setImportOpen] = useState(false)
  const navigate = useNavigate()

  function handleImport(project: Project) {
    importProjectAndSync(project)
    setImportOpen(false)
    void navigate({ to: '/projects' })
  }

  return (
    <div className="flex h-screen flex-col bg-background">
      <Empty>
        <EmptyMedia variant="icon">
          <CrowbarIcon className="text-foreground" />
        </EmptyMedia>
        <EmptyHeader>
          <EmptyTitle>Open a project folder</EmptyTitle>
          <EmptyDescription>Choose a local directory to get started.</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button className="w-full rounded-full" onClick={() => setImportOpen(true)}>
            Choose folder
          </Button>
        </EmptyContent>
      </Empty>

      <ImportProjectModal
        open={importOpen}
        onOpenChange={setImportOpen}
        onImport={handleImport}
      />
    </div>
  )
}
```

> Note: `Empty` has `flex-1` baked in — it fills the `h-screen` parent and centers its children vertically. No custom centering needed.

- [ ] **Step 4: Create the route file**

Create `web/src/routes/oobe.tsx`:

```tsx
import { createFileRoute } from '@tanstack/react-router'
import { OobeScreen } from '@/components/oobe/oobe-screen'

export const Route = createFileRoute('/oobe')({
  component: OobeScreen,
})
```

- [ ] **Step 5: Run tests**

```bash
npm run test -- oobe-screen --run
# Expected: PASS
```

- [ ] **Step 6: Verify in dev — clear state and reload**

```bash
npm run dev
# Devtools → Application → Clear site data → reload http://localhost:5173
# Expected: lands on /oobe, full-canvas, no sidebar, Crowbar icon with ghost cards, "Choose folder" button
# Click "Choose folder" → import modal opens
# Import a folder → navigates to /projects
```

- [ ] **Step 7: Commit**

```bash
git add src/routes/oobe.tsx src/components/oobe/oobe-screen.tsx src/__tests__/components/oobe/oobe-screen.test.tsx
git commit -m "feat(oobe): add /oobe route — full-canvas first-launch screen"
```

---

### Task 5: Screen 2 — update `ProjectListPage` empty state

When a project exists but no repos/workspaces have been added yet, show "No repositories yet" instead of the project grid.

**Files:**
- Modify: `web/src/components/projects/project-list-page.tsx`
- Test: `web/src/__tests__/components/projects/project-list-page.test.tsx` (if it exists, update; otherwise create)

**Interfaces:**
- Consumes: `Empty, EmptyMedia, EmptyHeader, EmptyTitle, EmptyDescription, EmptyContent` from `@/components/ui/empty`
- Consumes: `GitBranchIcon` from `lucide-react`
- Consumes: `dataOf` from `@/lib/loadable`
- Consumes: `useWorkspaceListStore` already imported in this file

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/components/projects/project-list-page.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import { ProjectListPage } from '@/components/projects/project-list-page'
import { useProjectDataStore } from '@/lib/store/projects'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { vi } from 'vitest'

vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))
vi.mock('@/lib/store/projects', () => ({
  useProjectDataStore: vi.fn(),
  useProjectStore: () => ({ activeProjectId: '', setActiveProject: vi.fn() }),
  importProjectAndSync: vi.fn(),
}))
vi.mock('@/lib/store/workspace-list', () => ({ useWorkspaceListStore: vi.fn() }))

const mockProject = { id: 'p1', name: 'Rabbyte', path: '/dev/rabbyte' }

describe('ProjectListPage', () => {
  it('shows "No repositories yet" when projects exist but no repos', () => {
    ;(useProjectDataStore as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
      status: 'success',
      data: [mockProject],
    })
    ;(useWorkspaceListStore as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
      status: 'success',
      data: [],
    })

    render(<ProjectListPage onSelect={vi.fn()} />)
    expect(screen.getByText('No repositories yet')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /add repository/i })).toBeInTheDocument()
  })

  it('shows project grid when repos exist', () => {
    ;(useProjectDataStore as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
      status: 'success',
      data: [mockProject],
    })
    ;(useWorkspaceListStore as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
      status: 'success',
      data: [{ id: 'r1', projectId: 'p1' }],
    })

    render(<ProjectListPage onSelect={vi.fn()} />)
    expect(screen.queryByText('No repositories yet')).not.toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
npm run test -- project-list-page --run
# Expected: FAIL
```

- [ ] **Step 3: Update `ProjectListPage`**

Replace the empty-state branch in `web/src/components/projects/project-list-page.tsx`:

```tsx
import { useState } from 'react'
import { GitBranchIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyMedia,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
  EmptyContent,
} from '@/components/ui/empty'
import { ProjectCard } from './project-card'
import { ImportProjectModal } from './import-project-modal'
import { useProjectStore, useProjectDataStore, importProjectAndSync } from '@/lib/store/projects'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { countReposByProject } from '@/lib/store/build-repo-tree'
import { useRetry } from '@/lib/store/use-retry'
import { DataState } from '@/components/ui/data-state'
import { dataOf } from '@/lib/loadable'
import type { Project } from '@/lib/types'

interface ProjectListPageProps {
  onSelect: (projectId: string) => void
}

export function ProjectListPage({ onSelect }: ProjectListPageProps) {
  const projectsLoadable = useProjectDataStore((s) => s.data)
  const reposLoadable = useWorkspaceListStore((s) => s.data)
  const repos = dataOf(reposLoadable) ?? []
  const repoCounts = countReposByProject(repos)
  const retry = useRetry(useProjectDataStore)
  const activeProjectId = useProjectStore((s) => s.activeProjectId)
  const setActiveProject = useProjectStore((s) => s.setActiveProject)
  const [importOpen, setImportOpen] = useState(false)

  const handleSelect = (id: string) => {
    setActiveProject(id)
    onSelect(id)
  }
  const handleImport = (project: Project) => {
    importProjectAndSync(project)
    setImportOpen(false)
  }

  return (
    <div className="flex flex-1 flex-col p-8">
      <DataState loadable={projectsLoadable} onRetry={retry} loadingLabel="Loading projects">
        {(projects) => {
          const hasRepos = repos.length > 0

          if (projects.length > 0 && !hasRepos) {
            return (
              <Empty>
                <EmptyMedia variant="icon">
                  <GitBranchIcon />
                </EmptyMedia>
                <EmptyHeader>
                  <EmptyTitle>No repositories yet</EmptyTitle>
                  <EmptyDescription>
                    Add a git repository to open a workspace.
                  </EmptyDescription>
                </EmptyHeader>
                <EmptyContent>
                  <Button
                    variant="outline"
                    size="sm"
                    className="rounded-full"
                    onClick={() => setImportOpen(true)}
                  >
                    Add repository
                  </Button>
                </EmptyContent>
              </Empty>
            )
          }

          return (
            <>
              <div className="mb-6 flex items-center justify-between">
                <h1 className="text-xl font-semibold text-foreground">Projects</h1>
                <Button size="sm" onClick={() => setImportOpen(true)}>
                  + Import project
                </Button>
              </div>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                {projects.map((project) => (
                  <ProjectCard
                    key={project.id}
                    project={project}
                    active={project.id === activeProjectId}
                    repoCount={repoCounts.get(project.id) ?? 0}
                    onClick={() => handleSelect(project.id)}
                  />
                ))}
              </div>
            </>
          )
        }}
      </DataState>

      <ImportProjectModal open={importOpen} onOpenChange={setImportOpen} onImport={handleImport} />
    </div>
  )
}
```

- [ ] **Step 4: Run tests**

```bash
npm run test -- project-list-page --run
# Expected: PASS
```

- [ ] **Step 5: Run full test suite**

```bash
npm run test --run
# Expected: all passing (or no regressions vs baseline)
```

- [ ] **Step 6: Run type-check**

```bash
npm run typecheck
# Expected: 0 errors
```

- [ ] **Step 7: Commit**

```bash
git add src/components/projects/project-list-page.tsx src/__tests__/components/projects/project-list-page.test.tsx
git commit -m "feat(oobe): screen 2 — no-repos empty state in ProjectListPage"
```
