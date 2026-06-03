# Workspace Tree Interactions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add inline create/rename and drag-and-drop reparenting to the workspace tree sidebar.

**Architecture:** A shared `WorkspaceTreeContext` (React context + provider in `workspace-tree.tsx`) holds all interaction state — create, rename, drag — and wires it to store actions. `WorkspaceTreeItem` consumes the context to show `+` buttons, inline inputs, and respond to drag events. A `WorkspaceTreeFooter` (rendered outside the ScrollArea) shows a trash drop zone only while a drag is active.

**Tech Stack:** React 18, Zustand, HTML5 Drag and Drop API, `@testing-library/react` + Vitest, Tailwind v4.

---

## File Map

| File | Change |
|------|--------|
| `web/src/lib/store/sidebar.ts` | Add `parentId` to `addWorkspace`, add `renameWorkspace`, `reparentWorkspace` |
| `web/src/__tests__/lib/store/sidebar.test.ts` | Tests for all three new/updated store actions |
| `web/src/components/layout/workspace-tree-context.tsx` | NEW — context type, provider, `useWorkspaceTreeContext` hook |
| `web/src/components/layout/workspace-inline-input.tsx` | NEW — auto-focused text input for create & rename |
| `web/src/__tests__/components/layout/workspace-inline-input.test.tsx` | NEW — tests for inline input behavior |
| `web/src/components/layout/workspace-tree-footer.tsx` | NEW — trash drop zone rendered when dragging |
| `web/src/components/layout/workspace-tree-item.tsx` | Add `repoId` prop, consume context, add `+`/rename/drag |
| `web/src/components/layout/workspace-tree.tsx` | Wrap in provider, pass `repoId`, add footer, repo as drop target |

---

## Task 1: Extend sidebar store

**Files:**
- Modify: `web/src/lib/store/sidebar.ts`
- Modify: `web/src/__tests__/lib/store/sidebar.test.ts`

### Current `addWorkspace` signature (line 46 in sidebar.ts):
```typescript
addWorkspace: (repoId: string, wsId: string, branch: string) => void
```
It does NOT pass `parentId` to the stored workspace.

- [ ] **Step 1: Write failing tests**

Add to the bottom of `web/src/__tests__/lib/store/sidebar.test.ts`:

```typescript
test('addWorkspace stores parentId when provided', () => {
  useSidebarStore.getState().addWorkspace('crowbar', 'ws-child', 'feature/child', 'ws-develop')
  const ws = useSidebarStore.getState().repos
    .find(r => r.id === 'crowbar')!.workspaces
    .find(w => w.id === 'ws-child')!
  expect(ws.parentId).toBe('ws-develop')
})

test('addWorkspace stores no parentId when omitted', () => {
  useSidebarStore.getState().addWorkspace('crowbar', 'ws-root', 'feature/root')
  const ws = useSidebarStore.getState().repos
    .find(r => r.id === 'crowbar')!.workspaces
    .find(w => w.id === 'ws-root')!
  expect(ws.parentId).toBeUndefined()
})

test('renameWorkspace updates branch on matching workspace', () => {
  useSidebarStore.getState().renameWorkspace('ws3', 'feature/renamed')
  const ws = useSidebarStore.getState().repos
    .flatMap(r => r.workspaces).find(w => w.id === 'ws3')!
  expect(ws.branch).toBe('feature/renamed')
})

test('renameWorkspace leaves other workspaces unchanged', () => {
  useSidebarStore.getState().renameWorkspace('ws3', 'feature/renamed')
  const ws = useSidebarStore.getState().repos
    .flatMap(r => r.workspaces).find(w => w.id === 'ws1')!
  expect(ws.branch).toBe('enhancement/scaffold')
})

test('reparentWorkspace changes parentId', () => {
  useSidebarStore.getState().reparentWorkspace('ws2', 'ws3')
  const ws = useSidebarStore.getState().repos
    .flatMap(r => r.workspaces).find(w => w.id === 'ws2')!
  expect(ws.parentId).toBe('ws3')
})

test('reparentWorkspace to undefined makes workspace a repo root', () => {
  useSidebarStore.getState().reparentWorkspace('ws3', undefined)
  const ws = useSidebarStore.getState().repos
    .flatMap(r => r.workspaces).find(w => w.id === 'ws3')!
  expect(ws.parentId).toBeUndefined()
})

test('reparentWorkspace rejects cycles: descendant cannot become ancestor', () => {
  // ws3 is a child of ws-develop; making ws-develop a child of ws3 would cycle
  useSidebarStore.getState().reparentWorkspace('ws-develop', 'ws3')
  const ws = useSidebarStore.getState().repos
    .flatMap(r => r.workspaces).find(w => w.id === 'ws-develop')!
  expect(ws.parentId).toBeUndefined() // unchanged
})

test('reparentWorkspace rejects cross-repo moves', () => {
  // qc1 is in quiver-core; ws3 is in crowbar
  useSidebarStore.getState().reparentWorkspace('ws3', 'qc1')
  const ws = useSidebarStore.getState().repos
    .flatMap(r => r.workspaces).find(w => w.id === 'ws3')!
  expect(ws.parentId).toBe('ws-develop') // unchanged
})
```

- [ ] **Step 2: Run to confirm they fail**

```bash
cd web && npx vitest run src/__tests__/lib/store/sidebar.test.ts --reporter=verbose
```

Expected: 7 new tests FAIL with "not a function" or assertion errors.

- [ ] **Step 3: Update sidebar.ts**

Replace the `SidebarState` interface and store implementation in `web/src/lib/store/sidebar.ts`. The full updated interface (add to the existing interface):

```typescript
interface SidebarState {
  chats: ProjectChat[]
  repos: Repo[]
  collapsedRepos: Set<string>
  activeTab: SidebarTab
  addChat: (chat: ProjectChat) => void
  deleteChat: (id: string) => void
  addWorkspace: (repoId: string, wsId: string, branch: string, parentId?: string) => void
  deleteWorkspace: (wsId: string) => void
  renameWorkspace: (wsId: string, branch: string) => void
  reparentWorkspace: (wsId: string, newParentId: string | undefined) => void
  toggleRepo: (repoId: string) => void
  setActiveTab: (tab: SidebarTab) => void
}
```

Replace the `addWorkspace` implementation:

```typescript
addWorkspace: (repoId, wsId, branch, parentId) =>
  set(s => ({
    repos: s.repos.map(r =>
      r.id !== repoId ? r : {
        ...r,
        workspaces: [...r.workspaces, {
          id: wsId, branch, parentId, status: 'new' as WorkspaceStatus, age: 'just now',
        }],
      },
    ),
  })),
```

Add after `deleteWorkspace`:

```typescript
renameWorkspace: (wsId, branch) =>
  set(s => ({
    repos: s.repos.map(r => ({
      ...r,
      workspaces: r.workspaces.map(w => w.id === wsId ? { ...w, branch } : w),
    })),
  })),

reparentWorkspace: (wsId, newParentId) =>
  set(s => {
    const repo = s.repos.find(r => r.workspaces.some(w => w.id === wsId))
    if (!repo) return s
    // Reject cross-repo: newParentId must exist in the same repo (or be undefined for root)
    if (newParentId !== undefined && !repo.workspaces.some(w => w.id === newParentId)) return s
    // Reject cycles: walk up from newParentId; if we reach wsId it's a cycle
    if (newParentId !== undefined) {
      const wsMap = new Map(repo.workspaces.map(w => [w.id, w]))
      let cursor: string | undefined = newParentId
      while (cursor !== undefined) {
        if (cursor === wsId) return s
        cursor = wsMap.get(cursor)?.parentId
      }
    }
    return {
      repos: s.repos.map(r =>
        r.id !== repo.id ? r : {
          ...r,
          workspaces: r.workspaces.map(w =>
            w.id === wsId ? { ...w, parentId: newParentId } : w,
          ),
        },
      ),
    }
  }),
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd web && npx vitest run src/__tests__/lib/store/sidebar.test.ts --reporter=verbose
```

Expected: all tests PASS (original 5 + new 9 = 14 total).

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/store/sidebar.ts web/src/__tests__/lib/store/sidebar.test.ts
git commit -m "feat: add parentId to addWorkspace, renameWorkspace, reparentWorkspace actions"
```

---

## Task 2: WorkspaceTreeContext

**Files:**
- Create: `web/src/components/layout/workspace-tree-context.tsx`

No unit tests (pure glue between UI state and store calls; tested implicitly via Tasks 5+6).

- [ ] **Step 1: Create the context file**

Create `web/src/components/layout/workspace-tree-context.tsx`:

```typescript
import { createContext, useCallback, useContext, useState, type ReactNode } from 'react'
import { useSidebarStore } from '@/lib/store/sidebar'

interface CreatingState {
  repoId: string
  parentId: string
}

interface DraggingState {
  id: string
  repoId: string
}

interface WorkspaceTreeContextValue {
  // Create
  creatingChildOf: CreatingState | null
  startCreating: (repoId: string, parentId: string) => void
  confirmCreate: (branch: string) => void
  cancelCreate: () => void
  // Rename
  renamingId: string | null
  startRenaming: (wsId: string) => void
  confirmRename: (branch: string) => void
  cancelRename: () => void
  // Drag
  draggingWs: DraggingState | null
  startDragging: (id: string, repoId: string) => void
  endDragging: () => void
  dropOnWorkspace: (targetWsId: string, targetRepoId: string) => void
  dropOnRepo: (targetRepoId: string) => void
  dropOnTrash: () => void
}

const WorkspaceTreeContext = createContext<WorkspaceTreeContextValue | null>(null)

export function useWorkspaceTreeContext() {
  const ctx = useContext(WorkspaceTreeContext)
  if (!ctx) throw new Error('useWorkspaceTreeContext must be used inside WorkspaceTreeProvider')
  return ctx
}

export function WorkspaceTreeProvider({ children }: { children: ReactNode }) {
  const [creatingChildOf, setCreatingChildOf] = useState<CreatingState | null>(null)
  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [draggingWs, setDraggingWs] = useState<DraggingState | null>(null)

  const startCreating = useCallback((repoId: string, parentId: string) => {
    setCreatingChildOf({ repoId, parentId })
  }, [])

  const confirmCreate = useCallback((branch: string) => {
    if (!creatingChildOf || !branch.trim()) { setCreatingChildOf(null); return }
    const id = `ws-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`
    useSidebarStore.getState().addWorkspace(
      creatingChildOf.repoId, id, branch.trim(), creatingChildOf.parentId,
    )
    setCreatingChildOf(null)
  }, [creatingChildOf])

  const cancelCreate = useCallback(() => setCreatingChildOf(null), [])

  const startRenaming = useCallback((wsId: string) => setRenamingId(wsId), [])

  const confirmRename = useCallback((branch: string) => {
    if (renamingId && branch.trim()) {
      useSidebarStore.getState().renameWorkspace(renamingId, branch.trim())
    }
    setRenamingId(null)
  }, [renamingId])

  const cancelRename = useCallback(() => setRenamingId(null), [])

  const startDragging = useCallback((id: string, repoId: string) => {
    setDraggingWs({ id, repoId })
  }, [])

  const endDragging = useCallback(() => setDraggingWs(null), [])

  const dropOnWorkspace = useCallback((targetWsId: string, targetRepoId: string) => {
    if (!draggingWs || draggingWs.id === targetWsId) { setDraggingWs(null); return }
    if (draggingWs.repoId !== targetRepoId) { setDraggingWs(null); return }
    useSidebarStore.getState().reparentWorkspace(draggingWs.id, targetWsId)
    setDraggingWs(null)
  }, [draggingWs])

  const dropOnRepo = useCallback((targetRepoId: string) => {
    if (!draggingWs || draggingWs.repoId !== targetRepoId) { setDraggingWs(null); return }
    useSidebarStore.getState().reparentWorkspace(draggingWs.id, undefined)
    setDraggingWs(null)
  }, [draggingWs])

  const dropOnTrash = useCallback(() => {
    if (draggingWs) useSidebarStore.getState().deleteWorkspace(draggingWs.id)
    setDraggingWs(null)
  }, [draggingWs])

  return (
    <WorkspaceTreeContext.Provider value={{
      creatingChildOf, startCreating, confirmCreate, cancelCreate,
      renamingId, startRenaming, confirmRename, cancelRename,
      draggingWs, startDragging, endDragging, dropOnWorkspace, dropOnRepo, dropOnTrash,
    }}>
      {children}
    </WorkspaceTreeContext.Provider>
  )
}
```

- [ ] **Step 2: Run all tests to ensure nothing broke**

```bash
cd web && npx vitest run --reporter=verbose 2>&1 | tail -5
```

Expected: all previous tests still PASS.

- [ ] **Step 3: Commit**

```bash
git add web/src/components/layout/workspace-tree-context.tsx
git commit -m "feat: add WorkspaceTreeContext for create/rename/drag interaction state"
```

---

## Task 3: WorkspaceInlineInput

**Files:**
- Create: `web/src/components/layout/workspace-inline-input.tsx`
- Create: `web/src/__tests__/components/layout/workspace-inline-input.test.tsx`

- [ ] **Step 1: Write failing tests**

Create `web/src/__tests__/components/layout/workspace-inline-input.test.tsx`:

```typescript
import { render, screen, fireEvent } from '@testing-library/react'
import { vi, test, expect } from 'vitest'
import { WorkspaceInlineInput } from '@/components/layout/workspace-inline-input'

test('confirms trimmed value on Enter', () => {
  const onConfirm = vi.fn()
  const onCancel = vi.fn()
  render(<WorkspaceInlineInput onConfirm={onConfirm} onCancel={onCancel} />)
  fireEvent.change(screen.getByRole('textbox'), { target: { value: '  feature/foo  ' } })
  fireEvent.keyDown(screen.getByRole('textbox'), { key: 'Enter' })
  expect(onConfirm).toHaveBeenCalledWith('feature/foo')
  expect(onCancel).not.toHaveBeenCalled()
})

test('calls onCancel on Escape', () => {
  const onConfirm = vi.fn()
  const onCancel = vi.fn()
  render(<WorkspaceInlineInput onConfirm={onConfirm} onCancel={onCancel} />)
  fireEvent.keyDown(screen.getByRole('textbox'), { key: 'Escape' })
  expect(onCancel).toHaveBeenCalled()
  expect(onConfirm).not.toHaveBeenCalled()
})

test('calls onCancel when Enter pressed with blank value', () => {
  const onConfirm = vi.fn()
  const onCancel = vi.fn()
  render(<WorkspaceInlineInput onConfirm={onConfirm} onCancel={onCancel} />)
  fireEvent.keyDown(screen.getByRole('textbox'), { key: 'Enter' })
  expect(onCancel).toHaveBeenCalled()
  expect(onConfirm).not.toHaveBeenCalled()
})

test('pre-fills input with defaultValue', () => {
  render(<WorkspaceInlineInput defaultValue="feat/existing" onConfirm={vi.fn()} onCancel={vi.fn()} />)
  expect(screen.getByRole('textbox')).toHaveValue('feat/existing')
})

test('does not double-fire after Enter then blur', () => {
  const onConfirm = vi.fn()
  const onCancel = vi.fn()
  render(<WorkspaceInlineInput onConfirm={onConfirm} onCancel={onCancel} />)
  const input = screen.getByRole('textbox')
  fireEvent.change(input, { target: { value: 'feature/test' } })
  fireEvent.keyDown(input, { key: 'Enter' })
  fireEvent.blur(input)
  expect(onConfirm).toHaveBeenCalledTimes(1)
})
```

- [ ] **Step 2: Run to confirm they fail**

```bash
cd web && npx vitest run src/__tests__/components/layout/workspace-inline-input.test.tsx --reporter=verbose
```

Expected: FAIL — module not found.

- [ ] **Step 3: Create the component**

Create `web/src/components/layout/workspace-inline-input.tsx`:

```typescript
import { useEffect, useRef, useState } from 'react'

interface WorkspaceInlineInputProps {
  defaultValue?: string
  placeholder?: string
  onConfirm: (value: string) => void
  onCancel: () => void
}

export function WorkspaceInlineInput({
  defaultValue = '',
  placeholder = 'branch-name',
  onConfirm,
  onCancel,
}: WorkspaceInlineInputProps) {
  const [value, setValue] = useState(defaultValue)
  const ref = useRef<HTMLInputElement>(null)
  // Prevents blur from double-firing after Enter/Escape already handled
  const handledRef = useRef(false)

  useEffect(() => {
    ref.current?.focus()
    ref.current?.select()
  }, [])

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter') {
      e.preventDefault()
      handledRef.current = true
      if (value.trim()) onConfirm(value.trim())
      else onCancel()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      handledRef.current = true
      onCancel()
    }
  }

  function handleBlur() {
    if (handledRef.current) return
    if (value.trim()) onConfirm(value.trim())
    else onCancel()
  }

  return (
    <input
      ref={ref}
      type="text"
      value={value}
      onChange={e => setValue(e.target.value)}
      onKeyDown={handleKeyDown}
      onBlur={handleBlur}
      placeholder={placeholder}
      className="min-w-0 flex-1 bg-transparent font-mono text-[13px] outline-none placeholder:text-muted-foreground/40"
    />
  )
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd web && npx vitest run src/__tests__/components/layout/workspace-inline-input.test.tsx --reporter=verbose
```

Expected: 5 tests PASS.

- [ ] **Step 5: Run full suite**

```bash
cd web && npx vitest run 2>&1 | tail -5
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/components/layout/workspace-inline-input.tsx \
        web/src/__tests__/components/layout/workspace-inline-input.test.tsx
git commit -m "feat: add WorkspaceInlineInput for create/rename workflows"
```

---

## Task 4: WorkspaceTreeFooter

**Files:**
- Create: `web/src/components/layout/workspace-tree-footer.tsx`

- [ ] **Step 1: Create the component**

Create `web/src/components/layout/workspace-tree-footer.tsx`:

```typescript
import { useState } from 'react'
import { cn } from '@/lib/utils'
import { useWorkspaceTreeContext } from './workspace-tree-context'

export function WorkspaceTreeFooter() {
  const { draggingWs, dropOnTrash } = useWorkspaceTreeContext()
  const [isOver, setIsOver] = useState(false)

  if (!draggingWs) return null

  return (
    <div className="shrink-0 flex items-center justify-center border-t border-border bg-background p-2">
      <div
        className={cn(
          'flex h-10 w-full items-center justify-center gap-2 rounded-lg border border-dashed text-[13px] font-medium transition-colors',
          isOver
            ? 'border-destructive bg-destructive/10 text-destructive'
            : 'border-destructive/40 text-destructive/40',
        )}
        onDragOver={(e) => { e.preventDefault(); e.dataTransfer.dropEffect = 'move' }}
        onDragEnter={(e) => { e.preventDefault(); setIsOver(true) }}
        onDragLeave={() => setIsOver(false)}
        onDrop={(e) => { e.preventDefault(); setIsOver(false); dropOnTrash() }}
      >
        <svg aria-hidden="true" className="size-4" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
          <path d="M2 4h12M5 4V2h6v2M6 7v5M10 7v5M3 4l1 9a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1l1-9" />
        </svg>
        Drop to delete
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Run tests**

```bash
cd web && npx vitest run 2>&1 | tail -5
```

Expected: all tests PASS (no tests for this component — it is visual-only).

- [ ] **Step 3: Commit**

```bash
git add web/src/components/layout/workspace-tree-footer.tsx
git commit -m "feat: add WorkspaceTreeFooter trash drop zone for drag-delete"
```

---

## Task 5: Update WorkspaceTreeItem

**Files:**
- Modify: `web/src/components/layout/workspace-tree-item.tsx`

This is a full rewrite of the component. Replace the entire file contents.

Key changes:
- Add `repoId: string` prop
- Consume `useWorkspaceTreeContext()`
- `+` button (leaf nodes, no children)
- "New" placeholder row (inside expanded children)
- Inline input when creating a child
- Inline input when renaming (double-click)
- Drag events: `draggable`, drag handlers, `isDragOver` visual feedback

- [ ] **Step 1: Write the updated component**

Replace `web/src/components/layout/workspace-tree-item.tsx` entirely:

```typescript
import { useRef, useState } from 'react'
import { cn } from '@/lib/utils'
import { WorkspaceBranchIcon } from './workspace-branch-icon'
import { WorkspaceInlineInput } from './workspace-inline-input'
import { ROW_BASE } from './workspace-row-base'
import { useWorkspaceTreeContext } from './workspace-tree-context'
import type { WorkspaceTreeNode } from './workspace-tree'

interface WorkspaceTreeItemProps {
  node: WorkspaceTreeNode
  depth: number
  repoId: string
  activeWorkspaceId: string
  onWorkspaceClick: (wsId: string) => void
}

export function WorkspaceTreeItem({
  node, depth, repoId, activeWorkspaceId, onWorkspaceClick,
}: WorkspaceTreeItemProps) {
  const { workspace, children } = node
  const isActive = workspace.id === activeWorkspaceId
  const isLocked = workspace.status === 'locked'
  const hasChildren = children.length > 0
  const [expanded, setExpanded] = useState(true)
  const [isDragOver, setIsDragOver] = useState(false)
  const dragCounter = useRef(0)

  const {
    creatingChildOf, startCreating, confirmCreate, cancelCreate,
    renamingId, startRenaming, confirmRename, cancelRename,
    draggingWs, startDragging, endDragging, dropOnWorkspace,
  } = useWorkspaceTreeContext()

  const isCreatingChild = creatingChildOf?.parentId === workspace.id
  const isRenaming = renamingId === workspace.id
  const isDraggingThis = draggingWs?.id === workspace.id
  const showChildrenSection = (hasChildren && expanded) || isCreatingChild

  const variant = isActive
    ? 'border-transparent bg-secondary text-secondary-foreground hover:bg-secondary/90'
    : isLocked
      ? 'border-transparent text-foreground/30 hover:bg-accent'
      : 'border-transparent text-foreground hover:bg-accent'

  function handleDragStart(e: React.DragEvent) {
    e.dataTransfer.effectAllowed = 'move'
    // Slight delay so the drag image captures the pre-fade state
    requestAnimationFrame(() => startDragging(workspace.id, repoId))
  }

  function handleDragEnter(e: React.DragEvent) {
    e.preventDefault()
    dragCounter.current++
    setIsDragOver(true)
  }

  function handleDragLeave() {
    dragCounter.current--
    if (dragCounter.current === 0) setIsDragOver(false)
  }

  function handleDragOver(e: React.DragEvent) {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
  }

  function handleDrop(e: React.DragEvent) {
    e.preventDefault()
    dragCounter.current = 0
    setIsDragOver(false)
    dropOnWorkspace(workspace.id, repoId)
  }

  return (
    <div>
      <div style={{ paddingLeft: (depth + 1) * 14 }}>
        <div
          role="button"
          tabIndex={0}
          draggable={!isRenaming}
          className={cn(
            ROW_BASE,
            variant,
            isDraggingThis && 'opacity-40',
            isDragOver && !isDraggingThis && 'ring-1 ring-ring',
          )}
          onClick={() => !isRenaming && onWorkspaceClick(workspace.id)}
          onKeyDown={(e) => {
            if (!isRenaming && (e.key === 'Enter' || e.key === ' ')) {
              e.preventDefault()
              onWorkspaceClick(workspace.id)
            }
          }}
          onDragStart={handleDragStart}
          onDragEnd={endDragging}
          onDragEnter={handleDragEnter}
          onDragLeave={handleDragLeave}
          onDragOver={handleDragOver}
          onDrop={handleDrop}
        >
          <WorkspaceBranchIcon status={workspace.status ?? 'new'} />

          {isRenaming ? (
            <WorkspaceInlineInput
              defaultValue={workspace.branch}
              onConfirm={confirmRename}
              onCancel={cancelRename}
            />
          ) : (
            <span
              className="min-w-0 flex-1 truncate font-mono text-left"
              onDoubleClick={(e) => { e.stopPropagation(); startRenaming(workspace.id) }}
            >
              {workspace.branch}
            </span>
          )}

          {isActive && !isRenaming && (workspace.added !== undefined || workspace.deleted !== undefined) && (
            <span className="flex shrink-0 gap-1 font-mono">
              {workspace.added !== undefined && workspace.added > 0 && (
                <span className="text-green-300">
                  +{workspace.added > 999 ? `${Math.round(workspace.added / 1000)}k` : workspace.added}
                </span>
              )}
              {workspace.deleted !== undefined && workspace.deleted > 0 && (
                <span className="text-red-300">
                  -{workspace.deleted > 999 ? `${Math.round(workspace.deleted / 1000)}k` : workspace.deleted}
                </span>
              )}
            </span>
          )}

          {/* Chevron for branch nodes; + for leaf nodes */}
          {hasChildren ? (
            <button
              type="button"
              className="shrink-0 rounded-md p-1 text-foreground/30 hover:text-foreground/60 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
              onClick={(e) => { e.stopPropagation(); setExpanded(v => !v) }}
              aria-label={expanded ? 'Collapse' : 'Expand'}
            >
              <svg
                aria-hidden="true"
                className={cn('size-3 transition-transform', expanded && 'rotate-90')}
                viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"
              >
                <path d="M6 3l5 5-5 5" />
              </svg>
            </button>
          ) : !isCreatingChild ? (
            <button
              type="button"
              className="shrink-0 rounded-md p-1 text-foreground/30 hover:text-foreground/60 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
              onClick={(e) => {
                e.stopPropagation()
                setExpanded(true)
                startCreating(repoId, workspace.id)
              }}
              aria-label="Add child workspace"
            >
              <svg aria-hidden="true" className="size-3" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
                <path d="M8 3v10M3 8h10" />
              </svg>
            </button>
          ) : null}
        </div>
      </div>

      {showChildrenSection && (
        <div>
          {hasChildren && expanded && children.map(child => (
            <WorkspaceTreeItem
              key={child.workspace.id}
              node={child}
              depth={depth + 1}
              repoId={repoId}
              activeWorkspaceId={activeWorkspaceId}
              onWorkspaceClick={onWorkspaceClick}
            />
          ))}

          {/* Inline input row or "New" placeholder */}
          <div style={{ paddingLeft: (depth + 2) * 14 }}>
            {isCreatingChild ? (
              <div className={cn(ROW_BASE, 'border-transparent text-foreground')}>
                <svg aria-hidden="true" className="size-4 shrink-0 text-foreground/30" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
                  <path d="M8 3v10M3 8h10" />
                </svg>
                <WorkspaceInlineInput
                  onConfirm={confirmCreate}
                  onCancel={cancelCreate}
                />
              </div>
            ) : (
              <div
                role="button"
                tabIndex={0}
                className={cn(ROW_BASE, 'border-transparent text-muted-foreground/40 hover:bg-accent hover:text-muted-foreground/60')}
                onClick={() => startCreating(repoId, workspace.id)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault()
                    startCreating(repoId, workspace.id)
                  }
                }}
              >
                <svg aria-hidden="true" className="size-4 shrink-0" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
                  <path d="M8 3v10M3 8h10" />
                </svg>
                <span className="font-mono text-left text-[13px]">New</span>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
```

- [ ] **Step 2: Run all tests**

```bash
cd web && npx vitest run --reporter=verbose 2>&1 | tail -10
```

Expected: all tests PASS. The `buildWorkspaceTree` tests in `workspace-tree.test.ts` do not test the component, so they are unaffected.

- [ ] **Step 3: Commit**

```bash
git add web/src/components/layout/workspace-tree-item.tsx
git commit -m "feat: add +/New/rename/drag interactions to WorkspaceTreeItem"
```

---

## Task 6: Update WorkspaceTree

**Files:**
- Modify: `web/src/components/layout/workspace-tree.tsx`

Changes:
1. Import and use `WorkspaceTreeProvider` — wrap the return value
2. Import and render `WorkspaceTreeFooter` — outside the `ScrollArea`
3. Pass `repoId` to each `WorkspaceTreeItem`
4. Add drag-and-drop handlers to the repo header div (drop = make root in that repo)
5. Wrap in a `flex flex-col` container so the footer sticks to the bottom

- [ ] **Step 1: Write the updated component**

Replace `web/src/components/layout/workspace-tree.tsx` entirely:

```typescript
import { useState } from 'react'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useSidebarStore } from '@/lib/store/sidebar'
import { cn } from '@/lib/utils'
import { ROW_BASE } from './workspace-row-base'
import { WorkspaceTreeFooter } from './workspace-tree-footer'
import { WorkspaceTreeItem } from './workspace-tree-item'
import { WorkspaceTreeProvider, useWorkspaceTreeContext } from './workspace-tree-context'
import type { Workspace } from '@/lib/store/sidebar'

export interface WorkspaceTreeNode {
  workspace: Workspace
  children: WorkspaceTreeNode[]
}

export function buildWorkspaceTree(workspaces: Workspace[]): WorkspaceTreeNode[] {
  const nodeMap = new Map<string, WorkspaceTreeNode>()
  for (const ws of workspaces) {
    nodeMap.set(ws.id, { workspace: ws, children: [] })
  }

  const roots: WorkspaceTreeNode[] = []
  for (const ws of workspaces) {
    const node = nodeMap.get(ws.id)!
    const parent = ws.parentId ? nodeMap.get(ws.parentId) : undefined

    if (!parent || parent === node) {
      roots.push(node)
    } else {
      let cursor: WorkspaceTreeNode | undefined = parent
      let cycle = false
      while (cursor) {
        if (cursor.workspace.id === ws.id) { cycle = true; break }
        cursor = cursor.workspace.parentId ? nodeMap.get(cursor.workspace.parentId) : undefined
      }
      if (cycle) roots.push(node)
      else parent.children.push(node)
    }
  }
  return roots
}

function WorkspaceTreeInner() {
  const navigate = useNavigate()
  const pathname = useRouterState({ select: s => s.location.pathname })
  const repos = useSidebarStore(s => s.repos)
  const [collapsedRepos, setCollapsedRepos] = useState<Set<string>>(new Set())
  const { draggingWs, dropOnRepo, endDragging } = useWorkspaceTreeContext()

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1] ?? ''

  function handleWorkspaceClick(wsId: string) {
    void navigate({ to: '/workspaces/$wsId', params: { wsId } })
  }

  function toggleRepo(repoId: string) {
    setCollapsedRepos(prev => {
      const next = new Set(prev)
      if (next.has(repoId)) next.delete(repoId)
      else next.add(repoId)
      return next
    })
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <ScrollArea className="flex-1">
        <div className="py-1">
          {repos.map(repo => {
            const roots = buildWorkspaceTree(repo.workspaces)
            const isCollapsed = collapsedRepos.has(repo.id)
            const isRepoDragOver = draggingWs?.repoId === repo.id
            return (
              <div key={repo.id} className="mb-1">
                <div
                  role="button"
                  tabIndex={0}
                  className={cn(
                    ROW_BASE,
                    'border-transparent text-foreground hover:bg-accent',
                    isRepoDragOver && 'ring-1 ring-ring',
                  )}
                  onClick={() => toggleRepo(repo.id)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); toggleRepo(repo.id) }
                  }}
                  aria-label={isCollapsed ? 'Expand repo' : 'Collapse repo'}
                  onDragOver={(e) => { e.preventDefault(); e.dataTransfer.dropEffect = 'move' }}
                  onDrop={(e) => { e.preventDefault(); dropOnRepo(repo.id) }}
                >
                  <span className={cn('inline-flex h-4 w-4 shrink-0 items-center justify-center rounded px-1 text-[10px] font-bold text-primary-foreground', repo.avatarColor)}>
                    {repo.avatarLabel}
                  </span>
                  <span className="min-w-0 flex-1 truncate text-left font-mono text-muted-foreground/60">
                    {repo.name}
                  </span>
                  <span className="shrink-0 rounded-md p-1 text-foreground/30">
                    <svg
                      aria-hidden="true"
                      className={cn('size-3 transition-transform', !isCollapsed && 'rotate-90')}
                      viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"
                    >
                      <path d="M6 3l5 5-5 5" />
                    </svg>
                  </span>
                </div>
                {!isCollapsed && (
                  <div>
                    {roots.map(node => (
                      <WorkspaceTreeItem
                        key={node.workspace.id}
                        node={node}
                        depth={0}
                        repoId={repo.id}
                        activeWorkspaceId={activeWorkspaceId}
                        onWorkspaceClick={handleWorkspaceClick}
                      />
                    ))}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      </ScrollArea>
      <WorkspaceTreeFooter />
    </div>
  )
}

export function WorkspaceTree() {
  return (
    <WorkspaceTreeProvider>
      <WorkspaceTreeInner />
    </WorkspaceTreeProvider>
  )
}
```

Note: `WorkspaceTreeInner` is extracted so it can call `useWorkspaceTreeContext()` inside the provider.

- [ ] **Step 2: Run all tests**

```bash
cd web && npx vitest run --reporter=verbose 2>&1 | tail -10
```

Expected: all tests PASS.

- [ ] **Step 3: Commit**

```bash
git add web/src/components/layout/workspace-tree.tsx
git commit -m "feat: wire WorkspaceTreeProvider, footer, repoId, and repo drop target into WorkspaceTree"
```

---

## Self-Review

### Spec coverage

| Requirement | Task |
|-------------|------|
| Leaf nodes show `+` instead of chevron | Task 5 |
| Clicking `+` creates inline input row | Task 5 |
| Confirming with text creates workspace | Task 2 (context), Task 1 (store) |
| Cancelling or empty input removes row | Task 3 (WorkspaceInlineInput), Task 2 |
| "New" row always visible for nodes with children | Task 5 |
| Double-click to rename | Task 5 |
| Drag anywhere on row | Task 5 (draggable + onDragStart) |
| Footer trash appears only while dragging | Task 4 |
| Drag to workspace → reparent as child | Task 5 + Task 2 + Task 1 |
| Drag to repo header → make root | Task 6 + Task 2 + Task 1 |
| Drag to trash → delete | Task 4 + Task 2 |
| Cannot move across repos | Task 1 (reparentWorkspace guard) |
| Cycle prevention | Task 1 (reparentWorkspace guard) |

### Placeholder scan

No TBD, TODO, or "add appropriate" language found.

### Type consistency

- `CreatingState.parentId: string` used in Task 2 context and Task 5 `startCreating(repoId, workspace.id)` — consistent.
- `WorkspaceTreeItemProps.repoId: string` added in Task 5, passed in Task 6 — consistent.
- `confirmCreate`, `cancelCreate`, `confirmRename`, `cancelRename` — all defined in Task 2, consumed in Task 5 — consistent.
- `dropOnWorkspace(targetWsId, targetRepoId)` — defined in Task 2 with two args, called in Task 5 with two args — consistent.
- `dropOnRepo(targetRepoId)` — defined in Task 2, called in Task 6 — consistent.
