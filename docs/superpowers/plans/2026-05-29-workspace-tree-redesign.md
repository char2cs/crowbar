# Workspace Tree Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the flat workspace list in the sidebar with a Zen-style recursive tree using Coss UI token-based button styles and branch-state icons.

**Architecture:** `WorkspaceTree` is a self-contained panel that reads repos and navigates workspaces using the sidebar store and TanStack Router directly — no prop drilling. It renders chats + the recursive workspace tree, replacing `WorkspacesSidebarPanel` entirely. The tree is derived at render time from a flat `workspaces[]` array using `parentId` references via `buildWorkspaceTree`.

**Tech Stack:** React, Zustand (`useSidebarStore`), TanStack Router (`useNavigate`, `useRouterState`), `@agilek/cli-loaders` (agent spinner), Tailwind CSS with Coss UI design tokens, Vitest.

---

## File Map

| Action | Path | Responsibility |
|---|---|---|
| **Modify** | `web/src/lib/store/sidebar.ts` | Add `WorkspaceStatus`, `parentId`, `status`; remove `num`; update mock data |
| **Modify** | `web/src/__tests__/lib/store/sidebar.test.ts` | Remove `num` references; keep existing tests passing |
| **Create** | `web/src/components/layout/workspace-tree.tsx` | `buildWorkspaceTree` utility + `WorkspaceTree` panel component |
| **Create** | `web/src/__tests__/components/layout/workspace-tree.test.ts` | Unit tests for `buildWorkspaceTree` |
| **Create** | `web/src/components/layout/workspace-branch-icon.tsx` | `WorkspaceBranchIcon` switch + `WorkspaceAgentSpinner` |
| **Create** | `web/src/components/layout/workspace-tree-item.tsx` | Recursive row component |
| **Modify** | `web/src/components/layout/SidebarTabs.tsx` | Swap `WorkspacesSidebarPanel` → `WorkspaceTree`; slim props |
| **Modify** | `web/src/components/layout/IDEShell.tsx` | Remove workspace/chat handlers now owned by `WorkspaceTree` |
| **Delete** | `web/src/components/layout/WorkspacesSidebarPanel.tsx` | Replaced by workspace-tree.tsx |
| **Modify** | `web/src/components/layout/SidebarRow.tsx` | Remove `WorkspaceRow` and `RepoRow` (replaced); keep `ChatRow` and `NewRow` |

---

## Task 1: Update sidebar.ts data model

**Files:**
- Modify: `web/src/lib/store/sidebar.ts`
- Modify: `web/src/__tests__/lib/store/sidebar.test.ts`

- [ ] **Step 1: Replace the Workspace interface and add WorkspaceStatus**

Open `web/src/lib/store/sidebar.ts` and replace the `Workspace` interface (lines 9–17) with:

```typescript
export type WorkspaceStatus =
  | 'locked'
  | 'new'
  | 'pr-open'
  | 'pr-closed'
  | 'pr-merged'
  | 'agent-running'

export interface Workspace {
  id: string
  branch: string
  parentId?: string
  status?: WorkspaceStatus
  added?: number
  deleted?: number
  age: string
}
```

- [ ] **Step 2: Replace INITIAL_REPOS mock data**

Replace the entire `INITIAL_REPOS` constant with:

```typescript
const INITIAL_REPOS: Repo[] = [
  {
    id: 'crowbar', name: 'crowbar', avatarLabel: 'C', avatarColor: 'bg-indigo-700',
    workspaces: [
      { id: 'ws-develop', branch: 'develop', status: 'locked', age: '—' },
      { id: 'ws3', branch: 'feature/app-design', parentId: 'ws-develop', status: 'pr-open', added: 5672, age: '16h ago' },
      { id: 'ws1', branch: 'enhancement/scaffold', parentId: 'ws3', status: 'agent-running', added: 22892, age: '3d ago' },
      { id: 'ws-fix', branch: 'fix/toolbar-crash', parentId: 'ws3', status: 'new', age: 'just now' },
      { id: 'ws2', branch: 'feature/api-backend', parentId: 'ws-develop', status: 'pr-merged', added: 27347, deleted: 455, age: '1d ago' },
    ],
  },
  {
    id: 'quiver-core', name: 'quiver.core', avatarLabel: 'Q', avatarColor: 'bg-emerald-700',
    workspaces: [
      { id: 'qc-develop', branch: 'develop', status: 'locked', age: '—' },
      { id: 'qc1', branch: 'feature/old-auth', parentId: 'qc-develop', status: 'pr-closed', age: '3d ago' },
    ],
  },
  {
    id: 'quiver-desktop', name: 'quiver.desktop', avatarLabel: 'Q', avatarColor: 'bg-orange-700',
    workspaces: [
      { id: 'qd-develop', branch: 'develop', status: 'locked', age: '—' },
      { id: 'qd2', branch: 'feature/quiver-shell', parentId: 'qd-develop', status: 'pr-open', added: 13485, deleted: 69, age: '3d ago' },
    ],
  },
]
```

- [ ] **Step 3: Update addWorkspace to set status**

Replace the `addWorkspace` action body so new workspaces get a default status:

```typescript
addWorkspace: (repoId, wsId, branch) =>
  set(s => ({
    repos: s.repos.map(r =>
      r.id !== repoId ? r : {
        ...r,
        workspaces: [...r.workspaces, { id: wsId, branch, status: 'new' as WorkspaceStatus, age: 'just now' }],
      },
    ),
  })),
```

- [ ] **Step 4: Run existing sidebar tests**

```bash
cd web && bun run test src/__tests__/lib/store/sidebar.test.ts
```

Expected: All 5 tests pass. If `deleteWorkspace('ws3')` test fails, it won't — `ws3` is kept in the new mock data. If any test references `num`, remove that reference.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/store/sidebar.ts web/src/__tests__/lib/store/sidebar.test.ts
git commit -m "feat: add WorkspaceStatus, parentId to Workspace data model"
```

---

## Task 2: buildWorkspaceTree utility + tests (TDD)

**Files:**
- Create: `web/src/components/layout/workspace-tree.tsx`
- Create: `web/src/__tests__/components/layout/workspace-tree.test.ts`

- [ ] **Step 1: Write the failing tests**

Create `web/src/__tests__/components/layout/workspace-tree.test.ts`:

```typescript
import { describe, expect, test } from 'vitest'
import { buildWorkspaceTree } from '@/components/layout/workspace-tree'
import type { Workspace } from '@/lib/store/sidebar'

const ws = (id: string, parentId?: string): Workspace => ({
  id, branch: `branch/${id}`, parentId, age: 'now',
})

describe('buildWorkspaceTree', () => {
  test('flat list with no parentIds → all nodes at root', () => {
    const result = buildWorkspaceTree([ws('a'), ws('b'), ws('c')])
    expect(result).toHaveLength(3)
    expect(result.every(n => n.children.length === 0)).toBe(true)
  })

  test('single level of nesting → correct parent/child attachment', () => {
    const result = buildWorkspaceTree([ws('parent'), ws('child', 'parent')])
    expect(result).toHaveLength(1)
    expect(result[0].workspace.id).toBe('parent')
    expect(result[0].children).toHaveLength(1)
    expect(result[0].children[0].workspace.id).toBe('child')
  })

  test('multi-level nesting', () => {
    const result = buildWorkspaceTree([
      ws('root'), ws('mid', 'root'), ws('leaf', 'mid'),
    ])
    expect(result).toHaveLength(1)
    expect(result[0].children[0].children[0].workspace.id).toBe('leaf')
  })

  test('orphaned parentId → treated as root', () => {
    const result = buildWorkspaceTree([ws('child', 'nonexistent')])
    expect(result).toHaveLength(1)
    expect(result[0].workspace.id).toBe('child')
    expect(result[0].children).toHaveLength(0)
  })

  test('circular reference → both treated as root', () => {
    const a: Workspace = { id: 'a', branch: 'a', parentId: 'b', age: 'now' }
    const b: Workspace = { id: 'b', branch: 'b', parentId: 'a', age: 'now' }
    const result = buildWorkspaceTree([a, b])
    expect(result).toHaveLength(2)
  })

  test('preserves workspace data on nodes', () => {
    const full: Workspace = { id: 'x', branch: 'feat/x', status: 'pr-open', added: 100, deleted: 5, age: '1h ago' }
    const result = buildWorkspaceTree([full])
    expect(result[0].workspace).toEqual(full)
  })
})
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd web && bun run test src/__tests__/components/layout/workspace-tree.test.ts
```

Expected: FAIL — `buildWorkspaceTree` not found.

- [ ] **Step 3: Create the file with the utility**

Create `web/src/components/layout/workspace-tree.tsx` with just the utility for now (the `WorkspaceTree` component is added in Task 5):

```typescript
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
      // Circular reference guard: walk up to check if adding this node
      // as a child of parent would create a cycle.
      let cursor: WorkspaceTreeNode | undefined = parent
      let cycle = false
      while (cursor) {
        if (cursor.workspace.id === ws.id) { cycle = true; break }
        cursor = cursor.workspace.parentId ? nodeMap.get(cursor.workspace.parentId) : undefined
      }
      if (cycle) {
        roots.push(node)
      } else {
        parent.children.push(node)
      }
    }
  }
  return roots
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
cd web && bun run test src/__tests__/components/layout/workspace-tree.test.ts
```

Expected: All 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/workspace-tree.tsx web/src/__tests__/components/layout/workspace-tree.test.ts
git commit -m "feat: add buildWorkspaceTree utility with cycle detection"
```

---

## Task 3: WorkspaceBranchIcon + WorkspaceAgentSpinner

**Files:**
- Create: `web/src/components/layout/workspace-branch-icon.tsx`

- [ ] **Step 1: Install cli-loaders**

```bash
cd web && bun add @agilek/cli-loaders
```

Expected: `@agilek/cli-loaders` appears in `package.json` dependencies.

- [ ] **Step 2: Create workspace-branch-icon.tsx**

Create `web/src/components/layout/workspace-branch-icon.tsx`:

```typescript
import { useState } from 'react'
import { Spinner, spinnerNames } from '@agilek/cli-loaders'
import type { WorkspaceStatus } from '@/lib/store/sidebar'

interface WorkspaceBranchIconProps {
  status: WorkspaceStatus
}

export function WorkspaceBranchIcon({ status }: WorkspaceBranchIconProps) {
  if (status === 'agent-running') return <WorkspaceAgentSpinner />

  switch (status) {
    case 'locked':
      return (
        <svg className="size-2.5 shrink-0 text-foreground/30" viewBox="0 0 16 16" fill="currentColor">
          <path d="M8 1a3 3 0 0 0-3 3v2H4a1 1 0 0 0-1 1v7h10V7a1 1 0 0 0-1-1h-1V4a3 3 0 0 0-3-3zm0 1.5A1.5 1.5 0 0 1 9.5 4v2h-3V4A1.5 1.5 0 0 1 8 2.5z" />
        </svg>
      )
    case 'new':
      return (
        <svg className="size-2.5 shrink-0 text-foreground/30" viewBox="0 0 16 16" fill="currentColor">
          <path d="M11.75 2.5a.75.75 0 1 0 1.5 0 .75.75 0 0 0-1.5 0zm.75 2.25a2.25 2.25 0 1 1 0-4.5 2.25 2.25 0 0 1 0 4.5zM4.25 5.5A2.25 2.25 0 1 0 4.25 1a2.25 2.25 0 0 0 0 4.5zM4 12.75a.75.75 0 1 1-1.5 0 .75.75 0 0 1 1.5 0zM3.25 15a2.25 2.25 0 1 0 0-4.5 2.25 2.25 0 0 0 0 4.5zm.75-10.372v5.244a2.25 2.25 0 0 0 0 4.256V15h1.5v-.872a2.25 2.25 0 0 0 0-4.256V4.628a2.25 2.25 0 0 0 0-4.256V.5H4v.372a2.25 2.25 0 0 0 0 4.256z" />
        </svg>
      )
    case 'pr-open':
      return (
        <svg className="size-2.5 shrink-0 text-green-500" viewBox="0 0 16 16" fill="currentColor">
          <path d="M5 3.25a.75.75 0 1 1-1.5 0 .75.75 0 0 1 1.5 0zm0 2.122a2.25 2.25 0 1 0-1.5 0v.878A2.25 2.25 0 0 0 5.75 8.5h1.5v1.128a2.251 2.251 0 1 0 1.5 0V8.5h1.5a2.25 2.25 0 0 0 2.25-2.25v-.878a2.25 2.25 0 1 0-1.5 0v.878a.75.75 0 0 1-.75.75h-4.5A.75.75 0 0 1 5 6.25v-.878zm3.75 7.378a.75.75 0 1 1-1.5 0 .75.75 0 0 1 1.5 0zm3-8.75a.75.75 0 1 1-1.5 0 .75.75 0 0 1 1.5 0z" />
        </svg>
      )
    case 'pr-closed':
      return (
        <svg className="size-2.5 shrink-0 text-red-500" viewBox="0 0 16 16" fill="currentColor">
          <path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06z" />
        </svg>
      )
    case 'pr-merged':
      return (
        <svg className="size-2.5 shrink-0 text-purple-500" viewBox="0 0 16 16" fill="currentColor">
          <path d="M5.45 5.154A4.25 4.25 0 0 0 9.25 7.5h1.378a2.251 2.251 0 1 1 0 1.5H9.25A5.734 5.734 0 0 1 5 7.123v3.505a2.25 2.25 0 1 1-1.5 0V5.372a2.25 2.25 0 1 1 1.95-.218zM4.25 13.5a.75.75 0 1 0 0-1.5.75.75 0 0 0 0 1.5zm8.5-4.5a.75.75 0 1 0 0-1.5.75.75 0 0 0 0 1.5zM5 3.25a.75.75 0 1 0 0 .005V3.25z" />
        </svg>
      )
  }
}

export function WorkspaceAgentSpinner() {
  const [name] = useState(
    () => spinnerNames[Math.floor(Math.random() * spinnerNames.length)]
  )
  return (
    <span className="size-2.5 shrink-0 text-violet-400 leading-none">
      <Spinner name={name} color="currentColor" size="0.625rem" />
    </span>
  )
}
```

- [ ] **Step 3: Run TypeScript check**

```bash
cd web && bun run typecheck
```

Expected: No errors in `workspace-branch-icon.tsx`.

- [ ] **Step 4: Commit**

```bash
git add web/src/components/layout/workspace-branch-icon.tsx web/package.json web/bun.lock
git commit -m "feat: add WorkspaceBranchIcon and WorkspaceAgentSpinner"
```

---

## Task 4: WorkspaceTreeItem (recursive row)

**Files:**
- Create: `web/src/components/layout/workspace-tree-item.tsx`

- [ ] **Step 1: Create workspace-tree-item.tsx**

Create `web/src/components/layout/workspace-tree-item.tsx`:

```typescript
import { useState } from 'react'
import { cn } from '@/lib/utils'
import { WorkspaceBranchIcon } from './workspace-branch-icon'
import type { WorkspaceTreeNode } from './workspace-tree'

interface WorkspaceTreeItemProps {
  node: WorkspaceTreeNode
  depth: number
  activeWorkspaceId: string
  onWorkspaceClick: (wsId: string) => void
}

const NAV_BASE =
  'relative flex min-w-0 flex-1 shrink-0 cursor-pointer items-center gap-1.5 rounded-lg border ' +
  'px-2 py-1 text-xs font-medium outline-none transition-shadow ' +
  'focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background'

export function WorkspaceTreeItem({
  node, depth, activeWorkspaceId, onWorkspaceClick,
}: WorkspaceTreeItemProps) {
  const { workspace, children } = node
  const isActive = workspace.id === activeWorkspaceId
  const isLocked = workspace.status === 'locked'
  const hasChildren = children.length > 0
  const [expanded, setExpanded] = useState(true)

  const variant = isActive
    ? 'border-primary bg-primary text-primary-foreground shadow-xs hover:bg-primary/90'
    : isLocked
      ? 'border-transparent text-foreground/30 hover:bg-accent'
      : 'border-transparent text-foreground hover:bg-accent'

  return (
    <div>
      {/* Row: nav button + optional chevron as siblings — no nested buttons */}
      <div className="mb-px flex items-center" style={{ paddingLeft: depth * 14 }}>
        <button
          type="button"
          className={cn(NAV_BASE, variant)}
          onClick={() => onWorkspaceClick(workspace.id)}
        >
          <WorkspaceBranchIcon status={workspace.status ?? 'new'} />

          <span className="min-w-0 flex-1 truncate font-mono text-left">
            {workspace.branch}
          </span>

          {isActive && (workspace.added !== undefined || workspace.deleted !== undefined) && (
            <span className="flex shrink-0 gap-1 font-mono">
              {workspace.added !== undefined && workspace.added > 0 && (
                <span className={isActive ? 'text-green-300' : 'text-green-500'}>
                  +{workspace.added > 999 ? `${Math.round(workspace.added / 1000)}k` : workspace.added}
                </span>
              )}
              {workspace.deleted !== undefined && workspace.deleted > 0 && (
                <span className={isActive ? 'text-red-300' : 'text-red-500'}>
                  -{workspace.deleted > 999 ? `${Math.round(workspace.deleted / 1000)}k` : workspace.deleted}
                </span>
              )}
            </span>
          )}
        </button>

        {hasChildren && (
          <button
            type="button"
            className="ml-0.5 shrink-0 rounded p-1 opacity-30 hover:opacity-80 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            onClick={() => setExpanded(v => !v)}
            aria-label={expanded ? 'Collapse' : 'Expand'}
          >
            <svg
              className={cn('size-2.5 transition-transform', expanded && 'rotate-90')}
              viewBox="0 0 16 16"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
            >
              <path d="M6 3l5 5-5 5" />
            </svg>
          </button>
        )}
      </div>

      {hasChildren && expanded && (
        <div>
          {children.map(child => (
            <WorkspaceTreeItem
              key={child.workspace.id}
              node={child}
              depth={depth + 1}
              activeWorkspaceId={activeWorkspaceId}
              onWorkspaceClick={onWorkspaceClick}
            />
          ))}
        </div>
      )}
    </div>
  )
}
```

- [ ] **Step 2: Run TypeScript check**

```bash
cd web && bun run typecheck
```

Expected: No errors in `workspace-tree-item.tsx`.

- [ ] **Step 3: Commit**

```bash
git add web/src/components/layout/workspace-tree-item.tsx
git commit -m "feat: add WorkspaceTreeItem recursive component"
```

---

## Task 5: WorkspaceTree panel

**Files:**
- Modify: `web/src/components/layout/workspace-tree.tsx` (add the panel component)

- [ ] **Step 1: Replace workspace-tree.tsx with the complete final file**

Overwrite `web/src/components/layout/workspace-tree.tsx` entirely with:

```typescript
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { useSidebarStore } from '@/lib/store/sidebar'
import { createMockChat } from '@/lib/mock/chats'
import { cn } from '@/lib/utils'
import { WorkspaceTreeItem } from './workspace-tree-item'
import { ChatRow, NewRow } from './SidebarRow'
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
      if (cycle) {
        roots.push(node)
      } else {
        parent.children.push(node)
      }
    }
  }
  return roots
}

export function WorkspaceTree() {
  const navigate = useNavigate()
  const pathname = useRouterState({ select: s => s.location.pathname })
  const chats = useSidebarStore(s => s.chats)
  const repos = useSidebarStore(s => s.repos)

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1] ?? ''
  const activeChatId = pathname.match(/\/chat\/([^/]+)/)?.[1] ?? ''

  function handleWorkspaceClick(wsId: string) {
    void navigate({ to: '/workspaces/$wsId', params: { wsId } })
  }

  function handleChatClick(id: string) {
    void navigate({ to: '/chat/$chatId', params: { chatId: id } })
  }

  function handleNewChat() {
    const chat = createMockChat()
    useSidebarStore.getState().addChat({ id: chat.id, title: chat.title, age: 'just now' })
    void navigate({ to: '/chat/$chatId', params: { chatId: chat.id } })
  }

  function handleDeleteChat(id: string) {
    useSidebarStore.getState().deleteChat(id)
  }

  return (
    <ScrollArea className="flex-1">
      <div className="py-1">
        {chats.map(chat => (
          <ChatRow
            key={chat.id}
            title={chat.title}
            age={chat.age}
            active={chat.id === activeChatId}
            onClick={() => handleChatClick(chat.id)}
            onDelete={() => handleDeleteChat(chat.id)}
          />
        ))}
        <NewRow label="New chat" onClick={handleNewChat} />
        <Separator className="my-1 mx-3" />
        {repos.map(repo => {
          const roots = buildWorkspaceTree(repo.workspaces)
          return (
            <div key={repo.id} className="mb-2">
              <div className="flex items-center gap-1.5 px-2 py-1 mt-1">
                <span className={cn('rounded px-1.5 py-0.5 text-[10px] font-bold text-primary-foreground', repo.avatarColor)}>
                  {repo.avatarLabel}
                </span>
                <span className="text-[10px] tracking-widest text-muted-foreground/40 uppercase">
                  {repo.name}
                </span>
              </div>
              <div className="px-1">
                {roots.map(node => (
                  <WorkspaceTreeItem
                    key={node.workspace.id}
                    node={node}
                    depth={0}
                    activeWorkspaceId={activeWorkspaceId}
                    onWorkspaceClick={handleWorkspaceClick}
                  />
                ))}
              </div>
            </div>
          )
        })}
      </div>
    </ScrollArea>
  )
}
```

- [ ] **Step 2: Run TypeScript check**

```bash
cd web && bun run typecheck
```

Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/components/layout/workspace-tree.tsx
git commit -m "feat: add WorkspaceTree panel component"
```

---

## Task 6: Wire SidebarTabs → WorkspaceTree

**Files:**
- Modify: `web/src/components/layout/SidebarTabs.tsx`

- [ ] **Step 1: Replace WorkspacesSidebarPanel with WorkspaceTree in SidebarTabs**

Replace the entire content of `web/src/components/layout/SidebarTabs.tsx`:

```typescript
import { Suspense } from 'react'
import { Tabs, TabsContent } from '@/components/ui/tabs'
import { WorkspaceTree } from './workspace-tree'
import { FileExplorerTree } from '@/features/file-explorer/components/file-explorer-tree'
import GitView from '@/features/git/components/git-view'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { SidebarSkeleton } from './SidebarSkeleton'
import { getMockFileTree } from '@/lib/mock/files'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { useSidebarStore, type SidebarTab } from '@/lib/store/sidebar'
import { useFileSystemStore } from '@/features/file-system/controllers/store'

interface SidebarTabsProps {
  activeWorkspaceRepoPath: string
}

export function SidebarTabs({ activeWorkspaceRepoPath }: SidebarTabsProps) {
  const activeTab = useSidebarStore(s => s.activeTab)
  const setActiveTab = useSidebarStore(s => s.setActiveTab)
  const handleFileOpen = useFileSystemStore.use.handleFileOpen?.()
  const handleFileSelect = useFileSystemStore.use.handleFileSelect?.()

  return (
    <Tabs
      value={activeTab}
      onValueChange={(v) => setActiveTab(v as SidebarTab)}
      className="flex flex-1 flex-col overflow-hidden"
    >
      <TabsContent value="workspaces" keepMounted className="flex flex-1 flex-col overflow-hidden mt-0">
        <WorkspaceTree />
      </TabsContent>

      <TabsContent value="files" keepMounted className="flex flex-1 flex-col overflow-hidden mt-0">
        <ErrorBoundary>
          <Suspense fallback={<SidebarSkeleton />}>
            <FileExplorerTree
              files={getMockFileTree(activeWorkspaceRepoPath)}
              rootFolderPath={activeWorkspaceRepoPath}
              onFileSelect={(path, isDir) => {
                if (isDir) {
                  useFileTreeStore.getState().toggleFolder(path)
                } else {
                  handleFileSelect?.(path, false)
                }
              }}
              onFileOpen={handleFileOpen ? (path: string, isDir: boolean) => {
                if (!isDir) void handleFileOpen(path, false)
              } : undefined}
              onCreateNewFileInDirectory={() => {}}
            />
          </Suspense>
        </ErrorBoundary>
      </TabsContent>

      <TabsContent value="git" keepMounted className="flex flex-1 flex-col overflow-hidden mt-0">
        <ErrorBoundary>
          <Suspense fallback={<SidebarSkeleton />}>
            <GitView repoPath={activeWorkspaceRepoPath} />
          </Suspense>
        </ErrorBoundary>
      </TabsContent>
    </Tabs>
  )
}
```

- [ ] **Step 2: Update IDEShell — remove props now owned by WorkspaceTree**

Open `web/src/components/layout/IDEShell.tsx`. The `<SidebarTabs>` call currently passes many props. Replace that call with just `activeWorkspaceRepoPath`. Also remove the store subscriptions that are no longer needed in IDEShell.

Find the lines that read from the store and are ONLY used for passing to SidebarTabs/WorkspacesSidebarPanel, and remove them:

```typescript
// REMOVE these lines (no longer needed in IDEShell):
const collapsedRepos = useSidebarStore((s) => s.collapsedRepos)
const addChat = useSidebarStore((s) => s.addChat)
const deleteChat = useSidebarStore((s) => s.deleteChat)
const deleteWorkspace = useSidebarStore((s) => s.deleteWorkspace)
const toggleRepo = useSidebarStore((s) => s.toggleRepo)
```

Keep `repos` (still used for deriving `activeRepo` / `workspaceLabel` for the header).
Keep `chats` if it's used for `chatTabLabel` in the tab bar display.

Find the `<SidebarTabs` JSX and replace its props with just:

```tsx
<SidebarTabs activeWorkspaceRepoPath={activeWorkspaceRepoPath} />
```

Remove any local handler functions in IDEShell that were only passed to SidebarTabs (e.g., `handleNewChat`, `handleDeleteChat`, `handleWorkspaceClick`, `handleNewWorkspace`, `handleDeleteWorkspace`, `handleRepoToggle` — if they exist and are only used for SidebarTabs).

- [ ] **Step 3: Run TypeScript check**

```bash
cd web && bun run typecheck
```

Expected: No errors. Fix any type errors that arise from the props removal.

- [ ] **Step 4: Run all tests**

```bash
cd web && bun run test
```

Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/SidebarTabs.tsx web/src/components/layout/IDEShell.tsx
git commit -m "feat: wire WorkspaceTree into SidebarTabs, remove prop drilling"
```

---

## Task 7: Delete old files + trim SidebarRow

**Files:**
- Delete: `web/src/components/layout/WorkspacesSidebarPanel.tsx`
- Modify: `web/src/components/layout/SidebarRow.tsx`

- [ ] **Step 1: Confirm WorkspaceRow and RepoRow have no other consumers**

```bash
cd web && grep -r "WorkspaceRow\|RepoRow" src/ --include="*.tsx" --include="*.ts" -l
```

Expected: Only `SidebarRow.tsx` itself (no other imports). If something else imports them, fix that first.

- [ ] **Step 2: Remove WorkspaceRow and RepoRow from SidebarRow.tsx**

Open `web/src/components/layout/SidebarRow.tsx` and delete the `RepoRow` and `WorkspaceRow` export functions and their interfaces. Keep only `ChatRow` and `NewRow`.

The file should end up as:

```typescript
import { cn } from '@/lib/utils'

const ROW = 'flex items-center h-9 px-2 mx-1.5 gap-2 rounded-lg my-0.5 cursor-pointer overflow-hidden select-none'

// ── ChatRow ──────────────────────────────────────────────────────────────────

interface ChatRowProps {
  title: string
  age: string
  active?: boolean
  onClick?: () => void
  onDelete?: () => void
}

export function ChatRow({ title, age, active, onClick, onDelete }: ChatRowProps) {
  return (
    <div className={cn(ROW, 'group', active ? 'bg-accent' : 'hover:bg-accent/50')} onClick={onClick} role="button" tabIndex={0} onKeyDown={onClick ? (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onClick() } } : undefined}>
      <div aria-hidden="true" className="flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-md border border-border bg-card text-[11px]">
        💬
      </div>
      <span className={cn('flex-1 truncate text-[13px]', active ? 'text-foreground' : 'text-muted-foreground')}>
        {title}
      </span>
      <span className={cn('flex-shrink-0 text-[11px] text-muted-foreground/50', onDelete && 'group-hover:hidden')}>{age}</span>
      {onDelete && (
        <button
          className="hidden group-hover:flex h-4 w-4 flex-shrink-0 items-center justify-center rounded text-muted-foreground/40 hover:text-muted-foreground"
          onClick={(e) => { e.stopPropagation(); onDelete() }}
          aria-label="Delete chat"
        >×</button>
      )}
    </div>
  )
}

// ── NewRow ────────────────────────────────────────────────────────────────────

interface NewRowProps {
  label: string
  onClick?: () => void
}

export function NewRow({ label, onClick }: NewRowProps) {
  return (
    <div className={cn(ROW, 'hover:bg-accent/50')} onClick={onClick} role="button" tabIndex={0} onKeyDown={onClick ? (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onClick() } } : undefined}>
      <span className="text-[15px] leading-none text-muted-foreground/40">+</span>
      <span className="flex-1 truncate text-[12.5px] text-muted-foreground/40">{label}</span>
    </div>
  )
}
```

- [ ] **Step 3: Delete WorkspacesSidebarPanel.tsx**

```bash
rm web/src/components/layout/WorkspacesSidebarPanel.tsx
```

- [ ] **Step 4: Confirm no remaining imports**

```bash
cd web && grep -r "WorkspacesSidebarPanel\|WorkspaceRow\|RepoRow" src/ --include="*.tsx" --include="*.ts"
```

Expected: No results.

- [ ] **Step 5: Run TypeScript check and tests**

```bash
cd web && bun run typecheck && bun run test
```

Expected: Clean.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "chore: remove WorkspacesSidebarPanel and unused SidebarRow exports"
```

---

## Task 8: QA — Chrome DevTools MCP

- [ ] **Step 1: Start the dev server**

```bash
cd web && bun dev
```

Note the port (typically 5173). Leave running.

- [ ] **Step 2: Open the app in Chrome via MCP**

Navigate to `http://localhost:5173` using the Chrome DevTools MCP `navigate_page` tool.

- [ ] **Step 3: Take a full screenshot of the sidebar**

Use `take_screenshot` to capture the current state. Verify:
- The sidebar shows the `crowbar`, `quiver.core`, and `quiver.desktop` repo headers with coloured avatar badges
- Under `crowbar`, `develop` is dimmed with a lock icon
- `feature/app-design` (green PR icon) is a child of `develop` with a chevron
- `enhancement/scaffold` (violet braille spinner) is a child of `feature/app-design` and shows `+22k`
- `fix/toolbar-crash` (grey branch icon) is a child of `feature/app-design`
- `feature/api-backend` (purple merge icon) is a child of `develop`

If anything is wrong, fix it before proceeding.

- [ ] **Step 4: Verify the active workspace style**

`enhancement/scaffold` should be the first workspace in the URL (or navigate to `/workspaces/ws1`). Confirm it renders with the filled `bg-primary` button style and shows `+22k`.

- [ ] **Step 5: Verify all 6 branch status icons are present**

Take a screenshot. Cross-check against spec:
- `locked` → lock SVG, dimmed (`text-foreground/30`)
- `new` → branch SVG, dimmed
- `pr-open` → merge SVG, `text-green-500`
- `pr-closed` → X SVG, `text-red-500` (visible in quiver.core under feature/old-auth)
- `pr-merged` → merge-commit SVG, `text-purple-500` (visible in crowbar feature/api-backend)
- `agent-running` → animated braille spinner, `text-violet-400`

- [ ] **Step 6: Test collapse/expand**

Use `click` on the chevron next to `feature/app-design`. Take a screenshot confirming its children (`enhancement/scaffold`, `fix/toolbar-crash`) are hidden. Click again — confirm they reappear.

- [ ] **Step 7: Test workspace navigation**

Click on `feature/api-backend`. Take a screenshot confirming:
- URL changes to `/workspaces/ws2`
- `feature/api-backend` is now the active (filled) item
- `enhancement/scaffold` reverts to ghost style

- [ ] **Step 8: Take final screenshot and confirm all looks correct**

Take a full-page screenshot of the final state. If any step above revealed a bug, fix it and re-run from Step 3.
