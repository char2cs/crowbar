# Sidebar Footer: Project Switcher Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the "Projects / {ActiveProject}" breadcrumb + settings gear from the sidebar header to the sidebar footer, removing the logo square and MU avatar.

**Architecture:** Rename `SidebarHeader.tsx` → `sidebar-project-switcher.tsx` (strip logo + MU, flip border direction), then update `IDEShell.tsx` to mount it inside shadcn's `SidebarFooter` at the bottom of the sidebar instead of the top.

**Tech Stack:** React, TypeScript, shadcn/ui sidebar primitives (`SidebarFooter`), Tailwind CSS, vitest

---

## File Map

| File | Action |
|------|--------|
| `web/src/components/layout/SidebarHeader.tsx` | Delete (replaced by sidebar-project-switcher) |
| `web/src/components/layout/sidebar-project-switcher.tsx` | Create (renamed + stripped version of SidebarHeader) |
| `web/src/components/layout/IDEShell.tsx` | Modify — swap header for footer |

---

### Task 1: Create `sidebar-project-switcher.tsx`

**Files:**
- Create: `web/src/components/layout/sidebar-project-switcher.tsx`

- [ ] **Step 1: Write the file**

Create `web/src/components/layout/sidebar-project-switcher.tsx` with this exact content:

```tsx
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useProjectStore } from '@/lib/store/projects'
import { ChevronDown } from 'lucide-react'
import { GearSix } from '@phosphor-icons/react'

interface SidebarProjectSwitcherProps {
  onProjectsClick?: () => void
  onProjectSelect?: (projectId: string) => void
  onSettingsClick?: () => void
}

export function SidebarProjectSwitcher({ onProjectsClick, onProjectSelect, onSettingsClick }: SidebarProjectSwitcherProps) {
  const { projects, activeProjectId, setActiveProject } = useProjectStore()
  const activeProject = projects.find(p => p.id === activeProjectId)

  const handleSelect = (id: string) => {
    setActiveProject(id)
    onProjectSelect?.(id)
  }

  return (
    <div className="flex h-12 flex-shrink-0 items-center gap-1.5 border-t border-border px-3">
      <Button
        variant="ghost"
        size="sm"
        className="h-auto px-1.5 py-0.5 text-[13px] text-muted-foreground hover:text-foreground"
        onClick={onProjectsClick}
      >
        Projects
      </Button>

      <span className="text-[13px] text-muted-foreground/40">/</span>

      <DropdownMenu>
        <DropdownMenuTrigger
          className="inline-flex h-auto items-center gap-1 rounded-sm px-1.5 py-0.5 text-[13px] font-semibold text-foreground hover:bg-accent hover:text-accent-foreground"
        >
          {activeProject?.name ?? 'Select project'}
          <ChevronDown className="h-3 w-3 text-muted-foreground" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="min-w-[160px]">
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

      {onSettingsClick && (
        <button
          onClick={onSettingsClick}
          aria-label="Settings"
          className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
        >
          <GearSix size={16} />
        </button>
      )}
    </div>
  )
}
```

Key differences from the old `SidebarHeader`:
- No logo square (`<div className="h-[22px] w-[22px] ...">`)
- No `userInitials` prop, no `Avatar` import
- `border-b` → `border-t` on the container div

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: no errors related to the new file. (Errors about `SidebarHeader` still being referenced in IDEShell are expected — fixed in Task 2.)

---

### Task 2: Update `IDEShell.tsx`

**Files:**
- Modify: `web/src/components/layout/IDEShell.tsx`

- [ ] **Step 1: Update imports**

Replace line 4:
```tsx
import { SidebarHeader } from './SidebarHeader'
```
With:
```tsx
import { SidebarProjectSwitcher } from './sidebar-project-switcher'
```

And extend line 3 (the sidebar UI import) to include `SidebarFooter`:
```tsx
import { SidebarProvider, Sidebar, SidebarInset, SidebarFooter } from '@/components/ui/sidebar'
```

- [ ] **Step 2: Remove `<SidebarHeader>` and add `<SidebarFooter>`**

The sidebar's inner flex column (lines ~101–147) currently looks like:

```tsx
<div className="relative flex h-full flex-col overflow-hidden">
  <div
    data-testid="sidebar-resize-handle"
    ...
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
    <SidebarTabs ... />
  </ErrorBoundary>
</div>
```

Change it to:

```tsx
<div className="relative flex h-full flex-col overflow-hidden">
  <div
    data-testid="sidebar-resize-handle"
    ...
  />
  <div
    className={cn('flex w-full flex-shrink-0 items-center', IS_MAC ? 'h-[44px]' : 'h-[34px]')}
    data-tauri-drag-region
  >
    <SidebarNavIcons />
  </div>
  <ErrorBoundary>
    <SidebarTabs ... />
  </ErrorBoundary>
  <SidebarFooter className="p-0">
    <SidebarProjectSwitcher
      onProjectsClick={() => void navigate({ to: '/projects' })}
      onProjectSelect={() => void navigate({ to: '/' })}
      onSettingsClick={() => setSettingsOpen(true)}
    />
  </SidebarFooter>
</div>
```

Changes:
- `<SidebarHeader userInitials="MU" ...>` removed entirely (along with its callbacks — they move to the footer)
- `<SidebarTabs>` is now the only child inside `<ErrorBoundary>`
- `<SidebarFooter className="p-0">` added after `</ErrorBoundary>` — `p-0` overrides shadcn's default `p-2` since `SidebarProjectSwitcher` manages its own padding (`px-3`)
- `<SidebarProjectSwitcher>` carries the same three callbacks

- [ ] **Step 3: Verify TypeScript compiles clean**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: no errors.

- [ ] **Step 4: Run existing tests**

```bash
cd web && npm test 2>&1 | tail -20
```

Expected: all tests pass. No tests reference `SidebarHeader` directly so nothing should break.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/sidebar-project-switcher.tsx web/src/components/layout/IDEShell.tsx
git commit -m "feat: move project switcher to sidebar footer, remove logo and user avatar"
```

---

### Task 3: Delete `SidebarHeader.tsx`

**Files:**
- Delete: `web/src/components/layout/SidebarHeader.tsx`

- [ ] **Step 1: Remove the file**

```bash
git rm web/src/components/layout/SidebarHeader.tsx
```

- [ ] **Step 2: Confirm no remaining references**

```bash
grep -r "SidebarHeader" web/src/ --include="*.tsx" --include="*.ts"
```

Expected: no output. If any references remain, update those files before committing.

- [ ] **Step 3: Commit**

```bash
git commit -m "chore: delete SidebarHeader (replaced by sidebar-project-switcher)"
```

---

### Task 4: Visual verification

- [ ] **Step 1: Start the dev server**

```bash
cd web && npm run dev
```

Open the app in a browser.

- [ ] **Step 2: Confirm the footer**

- The sidebar should have **no header row** between the nav-icons titlebar and the tab content
- The bottom of the sidebar should show `Projects / Rabbyte ⌄  ⚙` with a top border
- No blue logo square, no MU avatar anywhere in the sidebar

- [ ] **Step 3: Confirm settings still opens**

Click the ⚙ gear in the footer — the settings dialog should open.

- [ ] **Step 4: Confirm project dropdown still works**

Click `Rabbyte ⌄` — dropdown should show available projects and "Manage projects…"
