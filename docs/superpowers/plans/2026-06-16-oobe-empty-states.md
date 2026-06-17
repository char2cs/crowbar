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

### Task 1: Add app icon as static asset

**Files:**
- Create: `web/public/icon.png`

**Interfaces:**
- Produces: `/icon.png` accessible as a static URL in the app (used by Task 3)

- [ ] **Step 1: Copy the icon**

```bash
cp ../desktop/src-tauri/icons/128x128.png public/icon.png
```

- [ ] **Step 2: Verify it serves**

```bash
npm run dev
# open http://localhost:5173/icon.png — should display the Crowbar app icon
```

- [ ] **Step 3: Commit**

```bash
git add public/icon.png
git commit -m "feat: add app icon as static web asset"
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
          <img
            src="/icon.png"
            width={40}
            height={40}
            alt="Crowbar"
            style={{ borderRadius: 10 }}
          />
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
