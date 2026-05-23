# Crowbar Production-Ready UI — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close every UX gap between the current mock UI and a production-ready frontend across five waves: sidebar bugs, project shell, chat input bar, chat experience, and polish.

**Architecture:** Zustand (already installed) replaces scattered local state in `__root.tsx`; all mock behaviour stays in `lib/mock/` and `lib/api.ts` so swapping in the real backend requires no component changes. AI Elements components are installed via `npx ai-elements@latest add <name>` — they copy source files into the project like shadcn.

**Tech Stack:** React 19, TanStack Router, TanStack Query, Zustand 5, shadcn/ui, Vercel AI Elements, Vitest + React Testing Library (already wired).

**Run tests:** `cd web && npm test`
**Run dev server:** `cd web && npm run dev`

---

## Wave 1 — Foundation

---

### Task 1: Install missing UI dependencies

**Files:**
- No source files changed — installs only.

- [ ] **Step 1: Add missing shadcn components**

```bash
cd web
npx shadcn@latest add dialog dropdown-menu toggle-group input card skeleton
```

Expected: each component appears in `src/components/ui/`.

- [ ] **Step 2: Install AI Elements components**

```bash
cd web
npx ai-elements@latest add model-selector prompt-input attachments shimmer tool reasoning
```

Expected: components appear in the project (likely `src/components/ui/` — check output and note actual path for future tasks).

- [ ] **Step 3: Commit**

```bash
git add -A && git commit -m "chore: install shadcn dialog/dropdown/toggle-group/input/card/skeleton + ai-elements model-selector/prompt-input/attachments/shimmer/tool/reasoning"
```

---

### Task 2: Create Zustand sidebar store

The `__root.tsx` currently holds `chats` and `repos` in local `useState`. Moving them to Zustand lets any route (e.g. `workspaces/new.tsx`) update the sidebar without prop-drilling or pub/sub hacks.

**Files:**
- Create: `web/src/lib/store/sidebar.ts`
- Create: `web/src/__tests__/lib/store/sidebar.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/lib/store/sidebar.test.ts
import { beforeEach, expect, test } from 'vitest'
import { useSidebarStore } from '@/lib/store/sidebar'

beforeEach(() => {
  useSidebarStore.setState(useSidebarStore.getInitialState())
})

test('addWorkspace appends to the correct repo', () => {
  useSidebarStore.getState().addWorkspace('crowbar', 'ws-new', 'feature/test')
  const repo = useSidebarStore.getState().repos.find(r => r.id === 'crowbar')!
  expect(repo.workspaces.some(w => w.id === 'ws-new')).toBe(true)
})

test('addWorkspace does not affect other repos', () => {
  useSidebarStore.getState().addWorkspace('crowbar', 'ws-new', 'feature/test')
  const other = useSidebarStore.getState().repos.find(r => r.id === 'quiver-core')!
  expect(other.workspaces.some(w => w.id === 'ws-new')).toBe(false)
})

test('deleteWorkspace removes from repo', () => {
  useSidebarStore.getState().deleteWorkspace('ws3')
  const repo = useSidebarStore.getState().repos.find(r => r.id === 'crowbar')!
  expect(repo.workspaces.some(w => w.id === 'ws3')).toBe(false)
})

test('addChat appends a new chat entry', () => {
  useSidebarStore.getState().addChat({ id: 'c-test', title: 'New', age: 'just now' })
  const chats = useSidebarStore.getState().chats
  expect(chats.some(c => c.id === 'c-test')).toBe(true)
})

test('toggleRepo flips collapsed state', () => {
  useSidebarStore.getState().toggleRepo('crowbar')
  expect(useSidebarStore.getState().collapsedRepos.has('crowbar')).toBe(true)
  useSidebarStore.getState().toggleRepo('crowbar')
  expect(useSidebarStore.getState().collapsedRepos.has('crowbar')).toBe(false)
})
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
cd web && npm test -- sidebar.test
```

Expected: FAIL — `@/lib/store/sidebar` not found.

- [ ] **Step 3: Create the store**

```ts
// web/src/lib/store/sidebar.ts
import { create } from 'zustand'
import { getAllMockChats } from '@/lib/mock/chats'

export interface ProjectChat {
  id: string
  title: string
  age: string
}

export interface Workspace {
  id: string
  num?: number
  branch: string
  added?: number
  deleted?: number
  age: string
}

export interface Repo {
  id: string
  name: string
  avatarLabel: string
  avatarColor: string
  workspaces: Workspace[]
}

interface SidebarState {
  chats: ProjectChat[]
  repos: Repo[]
  collapsedRepos: Set<string>
  addChat: (chat: ProjectChat) => void
  deleteChat: (id: string) => void
  addWorkspace: (repoId: string, wsId: string, branch: string) => void
  deleteWorkspace: (wsId: string) => void
  toggleRepo: (repoId: string) => void
}

const INITIAL_REPOS: Repo[] = [
  {
    id: 'crowbar', name: 'crowbar', avatarLabel: 'C', avatarColor: 'bg-indigo-700',
    workspaces: [
      { id: 'ws3', num: 3, branch: 'feature/app-design', added: 5672, age: '16h ago' },
      { id: 'ws2', num: 2, branch: 'feature/api-backend', added: 27347, deleted: 455, age: '1d ago' },
      { id: 'ws1', num: 1, branch: 'enhancement/scaffold', added: 22892, age: '3d ago' },
    ],
  },
  {
    id: 'quiver-core', name: 'quiver.core', avatarLabel: 'Q', avatarColor: 'bg-emerald-700',
    workspaces: [{ id: 'qc1', branch: 'develop', age: '3d ago' }],
  },
  {
    id: 'quiver-desktop', name: 'quiver.desktop', avatarLabel: 'Q', avatarColor: 'bg-orange-700',
    workspaces: [
      { id: 'qd1', branch: 'develop', age: '6d ago' },
      { id: 'qd2', branch: 'feature/quiver-shell', added: 13485, deleted: 69, age: '3d ago' },
    ],
  },
]

function getInitialState() {
  return {
    chats: getAllMockChats().map(c => ({ id: c.id, title: c.title, age: c.age })),
    repos: INITIAL_REPOS,
    collapsedRepos: new Set<string>(),
  }
}

export const useSidebarStore = create<SidebarState>()((set) => ({
  ...getInitialState(),

  addChat: (chat) => set(s => ({ chats: [...s.chats, chat] })),

  deleteChat: (id) => set(s => ({ chats: s.chats.filter(c => c.id !== id) })),

  addWorkspace: (repoId, wsId, branch) =>
    set(s => ({
      repos: s.repos.map(r =>
        r.id !== repoId ? r : {
          ...r,
          workspaces: [...r.workspaces, { id: wsId, branch, age: 'just now' }],
        },
      ),
    })),

  deleteWorkspace: (wsId) =>
    set(s => ({
      repos: s.repos.map(r => ({ ...r, workspaces: r.workspaces.filter(w => w.id !== wsId) })),
    })),

  toggleRepo: (repoId) =>
    set(s => {
      const next = new Set(s.collapsedRepos)
      next.has(repoId) ? next.delete(repoId) : next.add(repoId)
      return { collapsedRepos: next }
    }),
}))

// Expose for test reset
;(useSidebarStore as any).getInitialState = getInitialState
```

- [ ] **Step 4: Run tests**

```bash
cd web && npm test -- sidebar.test
```

Expected: all 5 PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: Zustand sidebar store with chat/workspace/repo collapse state"
```

---

### Task 3: Migrate `__root.tsx` to Zustand store

**Files:**
- Modify: `web/src/routes/__root.tsx`
- Modify: `web/src/components/layout/Sidebar.tsx` — remove duplicate type exports, use store types
- Modify: `web/src/components/layout/SidebarHeader.tsx` — add `onProjectsClick` wiring

- [ ] **Step 1: Rewrite `__root.tsx`**

```tsx
// web/src/routes/__root.tsx
import { createRootRoute, Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import { AppShell } from '@/components/layout/AppShell'
import { Sidebar } from '@/components/layout/Sidebar'
import { useSidebarStore } from '@/lib/store/sidebar'
import { createMockChat, deleteMockChat } from '@/lib/mock/chats'
import { deleteMockWorkspace } from '@/lib/mock/workspaces'

function RootLayout() {
  const navigate = useNavigate()
  const pathname = useRouterState({ select: s => s.location.pathname })
  const { chats, repos, collapsedRepos, addChat, deleteChat, deleteWorkspace, toggleRepo } = useSidebarStore()

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1]
  const activeChatId = pathname.match(/\/chat\/([^/]+)/)?.[1]

  const handleNewChat = () => {
    const chat = createMockChat()
    addChat({ id: chat.id, title: chat.title, age: chat.age })
    navigate({ to: '/chat/$chatId', params: { chatId: chat.id } })
  }

  const handleDeleteChat = (id: string) => {
    deleteMockChat(id)
    deleteChat(id)
    if (activeChatId === id) {
      const remaining = chats.filter(c => c.id !== id)
      remaining.length > 0
        ? navigate({ to: '/chat/$chatId', params: { chatId: remaining[0].id } })
        : navigate({ to: '/' })
    }
  }

  const handleDeleteWorkspace = (wsId: string) => {
    deleteMockWorkspace(wsId)
    deleteWorkspace(wsId)
    if (activeWorkspaceId === wsId) navigate({ to: '/' })
  }

  return (
    <AppShell
      sidebar={
        <Sidebar
          projectName="Rabbyte"
          userInitials="MU"
          chats={chats}
          repos={repos}
          collapsedRepos={collapsedRepos}
          activeChatId={activeChatId}
          activeWorkspaceId={activeWorkspaceId}
          onChatClick={(id) => navigate({ to: '/chat/$chatId', params: { chatId: id } })}
          onWorkspaceClick={(_, wsId) => navigate({ to: '/workspaces/$wsId', params: { wsId } })}
          onNewChat={handleNewChat}
          onNewWorkspace={() => navigate({ to: '/workspaces/new' })}
          onDeleteChat={handleDeleteChat}
          onDeleteWorkspace={handleDeleteWorkspace}
          onRepoToggle={toggleRepo}
          onProjectsClick={() => navigate({ to: '/projects' })}
        />
      }
    >
      <Outlet />
    </AppShell>
  )
}

export const Route = createRootRoute({ component: RootLayout })
```

- [ ] **Step 2: Update `Sidebar.tsx` to accept `collapsedRepos` + `onRepoToggle` + `onProjectsClick`**

```tsx
// web/src/components/layout/Sidebar.tsx
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { SidebarHeader } from './SidebarHeader'
import { ChatRow, RepoRow, WorkspaceRow, NewRow } from './SidebarRow'
import type { ProjectChat, Repo } from '@/lib/store/sidebar'

export type { ProjectChat, Repo }

interface SidebarProps {
  projectName: string
  userInitials: string
  chats: ProjectChat[]
  repos: Repo[]
  collapsedRepos: Set<string>
  activeChatId?: string
  activeWorkspaceId?: string
  onChatClick?: (id: string) => void
  onWorkspaceClick?: (repoId: string, workspaceId: string) => void
  onNewChat?: () => void
  onNewWorkspace?: () => void
  onDeleteChat?: (id: string) => void
  onDeleteWorkspace?: (wsId: string) => void
  onRepoToggle?: (repoId: string) => void
  onProjectsClick?: () => void
}

export function Sidebar({
  projectName, userInitials, chats, repos, collapsedRepos,
  activeChatId, activeWorkspaceId,
  onChatClick, onWorkspaceClick, onNewChat, onNewWorkspace,
  onDeleteChat, onDeleteWorkspace, onRepoToggle, onProjectsClick,
}: SidebarProps) {
  return (
    <div className="flex h-full flex-col overflow-hidden bg-card">
      <SidebarHeader
        projectName={projectName}
        userInitials={userInitials}
        onProjectsClick={onProjectsClick}
      />
      <ScrollArea className="flex-1">
        <div className="py-1">
          {chats.map(chat => (
            <ChatRow
              key={chat.id}
              title={chat.title}
              age={chat.age}
              active={chat.id === activeChatId}
              onClick={() => onChatClick?.(chat.id)}
              onDelete={onDeleteChat ? () => onDeleteChat(chat.id) : undefined}
            />
          ))}
          <NewRow label="New chat" onClick={onNewChat} />
          <Separator className="my-1 mx-3" />
          {repos.map(repo => {
            const collapsed = collapsedRepos.has(repo.id)
            return (
              <div key={repo.id}>
                <RepoRow
                  name={repo.name}
                  avatarLabel={repo.avatarLabel}
                  avatarColor={repo.avatarColor}
                  collapsed={collapsed}
                  onClick={() => onRepoToggle?.(repo.id)}
                />
                {!collapsed && (
                  <>
                    {repo.workspaces.map(ws => (
                      <WorkspaceRow
                        key={ws.id}
                        num={ws.num}
                        branch={ws.branch}
                        added={ws.added}
                        deleted={ws.deleted}
                        age={ws.age}
                        active={ws.id === activeWorkspaceId}
                        onClick={() => onWorkspaceClick?.(repo.id, ws.id)}
                        onDelete={onDeleteWorkspace ? () => onDeleteWorkspace(ws.id) : undefined}
                      />
                    ))}
                    <NewRow label="New workspace" onClick={onNewWorkspace} />
                  </>
                )}
              </div>
            )
          })}
        </div>
      </ScrollArea>
    </div>
  )
}
```

- [ ] **Step 3: Run tests**

```bash
cd web && npm test
```

Expected: all existing tests PASS (types changed but behaviour unchanged).

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "refactor: migrate __root.tsx to Zustand sidebar store; wire repo collapse/expand"
```

---

### Task 4: Fix Bug 1 — new chat renders previous chat's messages

**Root cause:** `useState(() => fn())` only runs once on mount. When TanStack Router reuses the `ChatPage` component for a different `chatId`, the state never resets.

**Files:**
- Modify: `web/src/routes/chat/$chatId.tsx`
- Modify: `web/src/routes/workspaces/$wsId/$step.tsx`

- [ ] **Step 1: Fix `chat/$chatId.tsx`**

```tsx
// web/src/routes/chat/$chatId.tsx
import { useState, useEffect } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { ChatView } from '@/components/chat/ChatView'
import { getMockChat } from '@/lib/mock/chats'
import type { ChatMessage } from '@/lib/types'

export const Route = createFileRoute('/chat/$chatId')({
  component: ChatPage,
})

function ChatPage() {
  const { chatId } = Route.useParams()
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [sending, setSending] = useState(false)

  // Reset messages whenever chatId changes
  useEffect(() => {
    setMessages(getMockChat(chatId)?.messages ?? [])
  }, [chatId])

  const handleSend = (content: string) => {
    const userMsg: ChatMessage = {
      id: `u${Date.now()}`, role: 'user', content,
      authorName: 'Mateo', authorInitials: 'MU', timestamp: 'just now',
    }
    setMessages(prev => [...prev, userMsg])
    setSending(true)
    simulateStream('I can help with that. What would you like to know?', (chunk) => {
      setMessages(prev => {
        const last = prev[prev.length - 1]
        if (last?.role === 'assistant' && last.id === 'streaming') {
          return [...prev.slice(0, -1), { ...last, content: last.content + chunk }]
        }
        return [...prev, {
          id: 'streaming', role: 'assistant',
          content: chunk,
          authorName: 'Claude', authorInitials: '✦', modelName: 'Sonnet 4.6',
          timestamp: 'just now',
        }]
      })
    }, () => {
      setMessages(prev => prev.map(m => m.id === 'streaming' ? { ...m, id: `a${Date.now()}` } : m))
      setSending(false)
    })
  }

  return (
    <ChatView
      messages={messages}
      onSend={handleSend}
      sending={sending}
      inputPlaceholder="Ask about the Rabbyte project…"
    />
  )
}

// Simulate token streaming with a ReadableStream
function simulateStream(
  text: string,
  onChunk: (chunk: string) => void,
  onDone: () => void,
) {
  const words = text.split(' ')
  let i = 0
  const tick = () => {
    if (i >= words.length) { onDone(); return }
    onChunk((i === 0 ? '' : ' ') + words[i])
    i++
    setTimeout(tick, 40)
  }
  setTimeout(tick, 400) // initial delay before first token
}
```

- [ ] **Step 2: Fix `$step.tsx` — reset on workspaceId/step change**

```tsx
// web/src/routes/workspaces/$wsId/$step.tsx
import { useState, useEffect } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { workspaceQueryOptions } from '@/lib/queries'
import { getMockConversation } from '@/lib/mock/conversations'
import { ChatView } from '@/components/chat/ChatView'
import type { ChatMessage } from '@/lib/types'

export const Route = createFileRoute('/workspaces/$wsId/$step')({
  component: StepPage,
})

function StepPage() {
  const { wsId, step } = Route.useParams()
  const { data: workspace } = useQuery(workspaceQueryOptions(wsId))
  if (!workspace) return null
  return <WorkspaceChatView workspaceId={wsId} step={step} />
}

function WorkspaceChatView({ workspaceId, step }: { workspaceId: string; step: string }) {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [sending, setSending] = useState(false)

  useEffect(() => {
    setMessages(getMockConversation(workspaceId, step))
  }, [workspaceId, step])

  const handleSend = (content: string) => {
    const userMsg: ChatMessage = {
      id: `u${Date.now()}`, role: 'user', content,
      authorName: 'Mateo', authorInitials: 'MU', timestamp: 'just now',
    }
    setMessages(prev => [...prev, userMsg])
    setSending(true)
    simulateStream(
      'Understood. I\'ll start working on that now and update you as I make progress.',
      (chunk) => {
        setMessages(prev => {
          const last = prev[prev.length - 1]
          if (last?.role === 'assistant' && last.id === 'streaming') {
            return [...prev.slice(0, -1), { ...last, content: last.content + chunk }]
          }
          return [...prev, {
            id: 'streaming', role: 'assistant', content: chunk,
            authorName: 'Claude', authorInitials: '✦', modelName: 'Sonnet 4.6',
            timestamp: 'just now',
          }]
        })
      },
      () => {
        setMessages(prev => prev.map(m => m.id === 'streaming' ? { ...m, id: `a${Date.now()}` } : m))
        setSending(false)
      },
    )
  }

  return (
    <ChatView
      messages={messages}
      onSend={handleSend}
      sending={sending}
      inputPlaceholder={`Message… (${step})`}
    />
  )
}

function simulateStream(
  text: string,
  onChunk: (chunk: string) => void,
  onDone: () => void,
) {
  const words = text.split(' ')
  let i = 0
  const tick = () => {
    if (i >= words.length) { onDone(); return }
    onChunk((i === 0 ? '' : ' ') + words[i])
    i++
    setTimeout(tick, 40)
  }
  setTimeout(tick, 400)
}
```

- [ ] **Step 3: Run tests**

```bash
cd web && npm test
```

Expected: all PASS.

- [ ] **Step 4: Verify manually**

Start dev server (`npm run dev`). Navigate to `http://localhost:5173/#/workspaces/qd1/brainstorming`, click `+ New chat`. The new chat must show an empty message area, not Architecture decisions messages.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "fix: reset chat messages on chatId/workspaceId change; add mock token streaming"
```

---

### Task 5: Fix Bug 2 — new workspace appears in sidebar

**Files:**
- Modify: `web/src/routes/workspaces/new.tsx`

- [ ] **Step 1: Update `new.tsx` to call `store.addWorkspace` after creation**

```tsx
// web/src/routes/workspaces/new.tsx
import { useState } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { flowsQueryOptions } from '@/lib/queries'
import { postWorkspace } from '@/lib/api'
import { WorkspaceCreationForm } from '@/components/workspace/WorkspaceCreationForm'
import { useSidebarStore } from '@/lib/store/sidebar'

export const Route = createFileRoute('/workspaces/new')({
  component: NewWorkspacePage,
})

const REPOS = [
  { id: 'crowbar', name: 'crowbar' },
  { id: 'quiver-core', name: 'quiver.core' },
  { id: 'quiver-desktop', name: 'quiver.desktop' },
]

function NewWorkspacePage() {
  const navigate = useNavigate()
  const { data: flows = [] } = useQuery(flowsQueryOptions())
  const [loading, setLoading] = useState(false)
  const addWorkspace = useSidebarStore(s => s.addWorkspace)

  const handleSubmit = async (data: { repoId: string; branch: string; flowName: string }) => {
    setLoading(true)
    const ws = await postWorkspace(data.repoId, data.branch, data.flowName)
    addWorkspace(data.repoId, ws.id, data.branch)
    navigate({ to: '/workspaces/$wsId/$step', params: { wsId: ws.id, step: ws.currentState } })
  }

  return (
    <div className="flex flex-1 items-center justify-center">
      <div className="w-full max-w-sm">
        <h1 className="mb-6 text-lg font-semibold text-foreground">New workspace</h1>
        <WorkspaceCreationForm
          repos={REPOS}
          flows={flows.map(f => ({ name: f.name, description: f.description }))}
          onSubmit={handleSubmit}
          loading={loading}
        />
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Verify manually**

Dev server running. Click `+ New workspace` under crowbar, fill in a branch name, submit. The new branch must appear in the crowbar repo section of the sidebar.

- [ ] **Step 3: Commit**

```bash
git add -A && git commit -m "fix: new workspace registers in sidebar via Zustand store"
```

---

## Wave 2 — Project Shell

---

### Task 6: Add Project type, mock data, and API functions

**Files:**
- Modify: `web/src/lib/types.ts`
- Create: `web/src/lib/mock/projects.ts`
- Modify: `web/src/lib/api.ts`
- Create: `web/src/__tests__/lib/mock/projects.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/lib/mock/projects.test.ts
import { expect, test } from 'vitest'
import { getAllMockProjects, createMockProject, getMockProject } from '@/lib/mock/projects'

test('returns two initial projects', () => {
  expect(getAllMockProjects()).toHaveLength(2)
})

test('createMockProject adds project to store', () => {
  const before = getAllMockProjects().length
  createMockProject({ name: 'test-proj', path: '/tmp/test-proj' })
  expect(getAllMockProjects()).toHaveLength(before + 1)
})

test('getMockProject returns created project', () => {
  const proj = createMockProject({ name: 'lookup', path: '/tmp/lookup' })
  expect(getMockProject(proj.id)?.name).toBe('lookup')
})
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd web && npm test -- projects.test
```

- [ ] **Step 3: Add `Project` type to `lib/types.ts`**

Append to the end of `web/src/lib/types.ts`:

```ts
export interface Project {
  id: string
  name: string
  path: string
  lastActivity: Date
}
```

- [ ] **Step 4: Create `lib/mock/projects.ts`**

```ts
// web/src/lib/mock/projects.ts
import type { Project } from '@/lib/types'

const INITIAL_PROJECTS: Project[] = [
  { id: 'rabbyte', name: 'Rabbyte', path: '/Users/mateo/dev/rabbyte', lastActivity: new Date(Date.now() - 2 * 60 * 60 * 1000) },
  { id: 'personal', name: 'Personal', path: '/Users/mateo/dev/personal', lastActivity: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000) },
]

const store = new Map<string, Project>(INITIAL_PROJECTS.map(p => [p.id, p]))

export function getAllMockProjects(): Project[] {
  return Array.from(store.values())
}

export function getMockProject(id: string): Project | undefined {
  return store.get(id)
}

export function createMockProject(data: { name: string; path: string }): Project {
  const id = `proj-${Date.now()}`
  const project: Project = { id, ...data, lastActivity: new Date() }
  store.set(id, project)
  return project
}
```

- [ ] **Step 5: Add API functions to `lib/api.ts`**

Append to `web/src/lib/api.ts`:

```ts
import type { Project } from './types'
import { getAllMockProjects, createMockProject } from './mock/projects'

export function fetchProjects(): Promise<Project[]> {
  return Promise.resolve(getAllMockProjects())
}

export function postProject(name: string, path: string): Promise<Project> {
  return Promise.resolve(createMockProject({ name, path }))
}
```

- [ ] **Step 6: Run tests**

```bash
cd web && npm test -- projects.test
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add -A && git commit -m "feat: Project type, mock store, and API stubs"
```

---

### Task 7: Create Zustand project store

**Files:**
- Create: `web/src/lib/store/projects.ts`
- Create: `web/src/__tests__/lib/store/projects.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/lib/store/projects.test.ts
import { beforeEach, expect, test } from 'vitest'
import { useProjectStore } from '@/lib/store/projects'

beforeEach(() => {
  useProjectStore.setState(useProjectStore.getInitialState())
})

test('active project defaults to rabbyte', () => {
  expect(useProjectStore.getState().activeProjectId).toBe('rabbyte')
})

test('setActiveProject changes active project', () => {
  useProjectStore.getState().setActiveProject('personal')
  expect(useProjectStore.getState().activeProjectId).toBe('personal')
})

test('addProject appends a project', () => {
  const before = useProjectStore.getState().projects.length
  useProjectStore.getState().addProject({ id: 'x', name: 'X', path: '/tmp/x', lastActivity: new Date() })
  expect(useProjectStore.getState().projects).toHaveLength(before + 1)
})
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd web && npm test -- store/projects.test
```

- [ ] **Step 3: Create the store**

```ts
// web/src/lib/store/projects.ts
import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { Project } from '@/lib/types'
import { getAllMockProjects } from '@/lib/mock/projects'

interface ProjectState {
  projects: Project[]
  activeProjectId: string
  setActiveProject: (id: string) => void
  addProject: (project: Project) => void
}

function getInitialState() {
  const projects = getAllMockProjects()
  return { projects, activeProjectId: projects[0]?.id ?? '' }
}

export const useProjectStore = create<ProjectState>()(
  persist(
    (set) => ({
      ...getInitialState(),
      setActiveProject: (id) => set({ activeProjectId: id }),
      addProject: (project) => set(s => ({ projects: [...s.projects, project] })),
    }),
    { name: 'crowbar.activeProject', partialize: s => ({ activeProjectId: s.activeProjectId }) },
  ),
)

;(useProjectStore as any).getInitialState = getInitialState
```

- [ ] **Step 4: Run tests**

```bash
cd web && npm test -- store/projects.test
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: Zustand project store with localStorage persistence"
```

---

### Task 8: Create `/projects` route and `ProjectListPage`

**Files:**
- Create: `web/src/routes/projects/index.tsx`
- Create: `web/src/components/projects/ProjectCard.tsx`
- Create: `web/src/components/projects/ProjectListPage.tsx`

- [ ] **Step 1: Create `ProjectCard.tsx`**

```tsx
// web/src/components/projects/ProjectCard.tsx
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import type { Project } from '@/lib/types'

interface ProjectCardProps {
  project: Project
  active?: boolean
  repoCount?: number
  onClick: () => void
}

function timeAgo(date: Date): string {
  const diffMs = Date.now() - date.getTime()
  const diffH = Math.floor(diffMs / (1000 * 60 * 60))
  if (diffH < 1) return 'just now'
  if (diffH < 24) return `${diffH}h ago`
  return `${Math.floor(diffH / 24)}d ago`
}

export function ProjectCard({ project, active, repoCount = 0, onClick }: ProjectCardProps) {
  return (
    <Card
      className={`cursor-pointer transition-colors hover:bg-accent/50 ${active ? 'ring-1 ring-primary' : ''}`}
      onClick={onClick}
    >
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-semibold">{project.name}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-1">
        <p className="truncate font-mono text-[11px] text-muted-foreground">{project.path}</p>
        <div className="flex items-center gap-2">
          <Badge variant="outline" className="text-[10px]">{repoCount} repos</Badge>
          <span className="text-[11px] text-muted-foreground/60">{timeAgo(project.lastActivity)}</span>
        </div>
      </CardContent>
    </Card>
  )
}
```

- [ ] **Step 2: Create `ProjectListPage.tsx`**

```tsx
// web/src/components/projects/ProjectListPage.tsx
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { ProjectCard } from './ProjectCard'
import { ImportProjectModal } from './ImportProjectModal'
import { useProjectStore } from '@/lib/store/projects'
import type { Project } from '@/lib/types'

interface ProjectListPageProps {
  onSelect: (projectId: string) => void
}

export function ProjectListPage({ onSelect }: ProjectListPageProps) {
  const { projects, activeProjectId, setActiveProject, addProject } = useProjectStore()
  const [importOpen, setImportOpen] = useState(false)

  const handleSelect = (id: string) => {
    setActiveProject(id)
    onSelect(id)
  }

  const handleImport = (project: Project) => {
    addProject(project)
    setImportOpen(false)
  }

  return (
    <div className="flex flex-1 flex-col p-8">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-xl font-semibold text-foreground">Projects</h1>
        <Button size="sm" onClick={() => setImportOpen(true)}>+ Import project</Button>
      </div>

      {projects.length === 0 ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-3 text-center">
          <p className="text-lg font-medium text-foreground">No projects yet</p>
          <p className="text-sm text-muted-foreground">Import a local project folder to get started.</p>
          <Button onClick={() => setImportOpen(true)}>Import project</Button>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {projects.map(project => (
            <ProjectCard
              key={project.id}
              project={project}
              active={project.id === activeProjectId}
              repoCount={3}
              onClick={() => handleSelect(project.id)}
            />
          ))}
        </div>
      )}

      <ImportProjectModal
        open={importOpen}
        onOpenChange={setImportOpen}
        onImport={handleImport}
      />
    </div>
  )
}
```

- [ ] **Step 3: Create the route**

```tsx
// web/src/routes/projects/index.tsx
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { ProjectListPage } from '@/components/projects/ProjectListPage'

export const Route = createFileRoute('/projects/')({
  component: ProjectsPage,
})

function ProjectsPage() {
  const navigate = useNavigate()
  return (
    <ProjectListPage
      onSelect={() => navigate({ to: '/' })}
    />
  )
}
```

- [ ] **Step 4: Run tests**

```bash
cd web && npm test
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: /projects route with project list and card grid"
```

---

### Task 9: Create `ImportProjectModal`

**Files:**
- Create: `web/src/components/projects/ImportProjectModal.tsx`
- Create: `web/src/__tests__/components/projects/ImportProjectModal.test.tsx`

- [ ] **Step 1: Write the failing test**

```tsx
// web/src/__tests__/components/projects/ImportProjectModal.test.tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { ImportProjectModal } from '@/components/projects/ImportProjectModal'
import { vi, expect, test } from 'vitest'

test('Import button is disabled when no folder is selected', () => {
  render(<ImportProjectModal open={true} onOpenChange={() => {}} onImport={() => {}} />)
  expect(screen.getByRole('button', { name: /import/i })).toBeDisabled()
})

test('shows selected folder name after pick', () => {
  render(<ImportProjectModal open={true} onOpenChange={() => {}} onImport={() => {}} />)
  // Simulate file selection via the hidden input
  const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement
  const file = new File([''], 'my-project', { type: '' })
  Object.defineProperty(file, 'webkitRelativePath', { value: 'my-project/', configurable: true })
  Object.defineProperty(fileInput, 'files', { value: [file], configurable: true })
  fireEvent.change(fileInput)
  expect(screen.getByDisplayValue('my-project')).toBeInTheDocument()
})

test('calls onImport with name and path on submit', () => {
  const onImport = vi.fn()
  render(<ImportProjectModal open={true} onOpenChange={() => {}} onImport={onImport} />)
  const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement
  const file = new File([''], 'test-proj', { type: '' })
  Object.defineProperty(file, 'webkitRelativePath', { value: 'test-proj/', configurable: true })
  Object.defineProperty(fileInput, 'files', { value: [file], configurable: true })
  fireEvent.change(fileInput)
  fireEvent.click(screen.getByRole('button', { name: /import/i }))
  expect(onImport).toHaveBeenCalledWith(expect.objectContaining({ name: 'test-proj' }))
})
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd web && npm test -- ImportProjectModal.test
```

- [ ] **Step 3: Create `ImportProjectModal.tsx`**

```tsx
// web/src/components/projects/ImportProjectModal.tsx
import { useRef, useState } from 'react'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { postProject } from '@/lib/api'
import type { Project } from '@/lib/types'

interface ImportProjectModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onImport: (project: Project) => void
}

export function ImportProjectModal({ open, onOpenChange, onImport }: ImportProjectModalProps) {
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [selectedPath, setSelectedPath] = useState('')
  const [projectName, setProjectName] = useState('')
  const [loading, setLoading] = useState(false)

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    // webkitRelativePath is "folderName/..." — take the first segment
    const folderName = file.webkitRelativePath.split('/')[0] || file.name
    setSelectedPath(folderName)
    setProjectName(prev => prev || folderName)
  }

  const handleImport = async () => {
    if (!selectedPath) return
    setLoading(true)
    const project = await postProject(projectName || selectedPath, selectedPath)
    onImport(project)
    setLoading(false)
    setSelectedPath('')
    setProjectName('')
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Import project</DialogTitle>
        </DialogHeader>

        <div className="space-y-4 py-2">
          {/* Hidden OS folder picker */}
          <input
            ref={fileInputRef}
            type="file"
            // @ts-ignore — webkitdirectory is not in React types
            webkitdirectory=""
            className="hidden"
            onChange={handleFileChange}
          />

          <div className="space-y-1.5">
            <label className="text-sm font-medium text-foreground">Project folder</label>
            <div className="flex gap-2">
              <Input
                readOnly
                value={selectedPath}
                placeholder="No folder selected"
                className="flex-1 font-mono text-[12px]"
              />
              <Button
                variant="outline"
                size="sm"
                type="button"
                onClick={() => fileInputRef.current?.click()}
              >
                Choose…
              </Button>
            </div>
          </div>

          <div className="space-y-1.5">
            <label className="text-sm font-medium text-foreground">Project name</label>
            <Input
              value={projectName}
              onChange={e => setProjectName(e.target.value)}
              placeholder="My project"
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button onClick={handleImport} disabled={!selectedPath || loading}>
            {loading ? 'Importing…' : 'Import'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
```

- [ ] **Step 4: Run tests**

```bash
cd web && npm test -- ImportProjectModal.test
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: ImportProjectModal with OS folder picker"
```

---

### Task 10: Wire project switcher in `SidebarHeader` + breadcrumb link

**Files:**
- Modify: `web/src/components/layout/SidebarHeader.tsx`

- [ ] **Step 1: Rewrite `SidebarHeader.tsx` with project switcher dropdown**

```tsx
// web/src/components/layout/SidebarHeader.tsx
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
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

interface SidebarHeaderProps {
  userInitials: string
  onProjectsClick?: () => void
  onProjectSelect?: (projectId: string) => void
}

export function SidebarHeader({ userInitials, onProjectsClick, onProjectSelect }: SidebarHeaderProps) {
  const { projects, activeProjectId, setActiveProject } = useProjectStore()
  const activeProject = projects.find(p => p.id === activeProjectId)

  const handleSelect = (id: string) => {
    setActiveProject(id)
    onProjectSelect?.(id)
  }

  return (
    <div className="flex h-12 flex-shrink-0 items-center gap-1.5 border-b border-border px-3">
      <div className="h-[22px] w-[22px] flex-shrink-0 rounded-[6px] bg-primary" />

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
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="h-auto gap-1 px-1.5 py-0.5 text-[13px] font-semibold text-foreground"
          >
            {activeProject?.name ?? 'Select project'}
            <ChevronDown className="h-3 w-3 text-muted-foreground" />
          </Button>
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

      <Avatar className="ml-auto h-6 w-6">
        <AvatarFallback className="text-[10px] font-bold">{userInitials}</AvatarFallback>
      </Avatar>
    </div>
  )
}
```

- [ ] **Step 2: Update `Sidebar.tsx` to pass `onProjectSelect`**

In `web/src/components/layout/Sidebar.tsx`, add `onProjectSelect?: (id: string) => void` to `SidebarProps` and pass it to `<SidebarHeader>`.

- [ ] **Step 3: Update `__root.tsx` to pass `onProjectSelect`**

In `web/src/routes/__root.tsx`, add `onProjectSelect={(id) => navigate({ to: '/' })}` to the `<Sidebar>` call.

- [ ] **Step 4: Run tests**

```bash
cd web && npm test
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: project switcher dropdown in sidebar header; wire Projects breadcrumb"
```

---

## Wave 3 — Chat Input Bar

---

### Task 11: Rebuild `ChatInput` with AI Elements `PromptInput` + `ModelSelector`

**Before starting:** Check where `npx ai-elements@latest add model-selector prompt-input` copied the component files (output from Task 1 install step). Update import paths below if they differ from `@/components/ui/`.

**Files:**
- Modify: `web/src/components/chat/ChatInput.tsx`
- Create: `web/src/hooks/useModelPreference.ts`
- Create: `web/src/__tests__/hooks/useModelPreference.test.ts`
- Modify: `web/src/__tests__/components/chat/ChatInput.test.tsx`

- [ ] **Step 1: Create `useModelPreference` hook**

```ts
// web/src/hooks/useModelPreference.ts
import { useState } from 'react'

export const MODELS = [
  { id: 'claude-haiku-4-5-20251001', label: 'Haiku 4.5' },
  { id: 'claude-sonnet-4-6', label: 'Sonnet 4.6' },
  { id: 'claude-opus-4-7', label: 'Opus 4.7' },
] as const

export type ModelId = typeof MODELS[number]['id']

const STORAGE_KEY = 'crowbar.model'
const DEFAULT_MODEL: ModelId = 'claude-sonnet-4-6'

export function useModelPreference() {
  const [model, setModelState] = useState<ModelId>(() => {
    const stored = localStorage.getItem(STORAGE_KEY)
    return (MODELS.find(m => m.id === stored)?.id ?? DEFAULT_MODEL)
  })

  const setModel = (id: ModelId) => {
    localStorage.setItem(STORAGE_KEY, id)
    setModelState(id)
  }

  const modelLabel = MODELS.find(m => m.id === model)?.label ?? 'Sonnet 4.6'

  return { model, setModel, modelLabel, models: MODELS }
}
```

- [ ] **Step 2: Write test for `useModelPreference`**

```ts
// web/src/__tests__/hooks/useModelPreference.test.ts
import { renderHook, act } from '@testing-library/react'
import { useModelPreference, MODELS } from '@/hooks/useModelPreference'
import { beforeEach, expect, test } from 'vitest'

beforeEach(() => localStorage.clear())

test('defaults to Sonnet 4.6', () => {
  const { result } = renderHook(() => useModelPreference())
  expect(result.current.model).toBe('claude-sonnet-4-6')
})

test('setModel persists to localStorage', () => {
  const { result } = renderHook(() => useModelPreference())
  act(() => result.current.setModel('claude-haiku-4-5-20251001'))
  expect(localStorage.getItem('crowbar.model')).toBe('claude-haiku-4-5-20251001')
})

test('reads from localStorage on mount', () => {
  localStorage.setItem('crowbar.model', 'claude-opus-4-7')
  const { result } = renderHook(() => useModelPreference())
  expect(result.current.model).toBe('claude-opus-4-7')
})

test('returns all three model options', () => {
  const { result } = renderHook(() => useModelPreference())
  expect(result.current.models).toHaveLength(3)
})
```

- [ ] **Step 3: Run test**

```bash
cd web && npm test -- useModelPreference
```

Expected: all PASS.

- [ ] **Step 4: Rewrite `ChatInput.tsx`**

> **Note:** The import paths for `PromptInput` and `ModelSelector` depend on where AI Elements installed them. Check the actual file locations first (`ls src/components/ui/` or wherever the install output indicated). Adjust the imports below accordingly.

```tsx
// web/src/components/chat/ChatInput.tsx
import { useRef, useState } from 'react'
import { ArrowUp, Paperclip } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { useModelPreference, MODELS } from '@/hooks/useModelPreference'
// Adjust these import paths to match where ai-elements installed the files:
import { ModelSelector, ModelSelectorTrigger, ModelSelectorContent, ModelSelectorItem, ModelSelectorGroup } from '@/components/ui/model-selector'
import { PromptInput, PromptInputTextarea, PromptInputActions } from '@/components/ui/prompt-input'

const EFFORT_LEVELS = [
  { value: 'low', label: 'Low' },
  { value: 'medium', label: 'Mid' },
  { value: 'high', label: 'High' },
  { value: 'max', label: 'Max' },
] as const

type EffortLevel = typeof EFFORT_LEVELS[number]['value']

interface ChatInputProps {
  placeholder: string
  onSend: (message: string, attachments: File[]) => void
  disabled?: boolean
}

export function ChatInput({ placeholder, onSend, disabled }: ChatInputProps) {
  const [value, setValue] = useState('')
  const [attachments, setAttachments] = useState<File[]>([])
  const [effort, setEffort] = useState<EffortLevel>('medium')
  const fileInputRef = useRef<HTMLInputElement>(null)
  const { model, setModel, modelLabel } = useModelPreference()

  // Rough token estimate: 1 token ≈ 4 chars
  const tokenPct = Math.min(Math.round((value.length / 4 / 200000) * 100), 100)

  const handleSend = () => {
    if (!value.trim()) return
    onSend(value.trim(), attachments)
    setValue('')
    setAttachments([])
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) handleSend()
  }

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files ?? [])
    setAttachments(prev => [...prev, ...files])
    e.target.value = '' // allow re-selecting same file
  }

  return (
    <div className="border-t border-border bg-card px-4 pb-4 pt-2.5">
      {attachments.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-1.5">
          {attachments.map((file, i) => (
            <Badge
              key={i}
              variant="outline"
              className="gap-1 text-[11px]"
            >
              {file.name}
              <button
                className="ml-0.5 text-muted-foreground/50 hover:text-muted-foreground"
                onClick={() => setAttachments(prev => prev.filter((_, j) => j !== i))}
                aria-label={`Remove ${file.name}`}
              >
                ×
              </button>
            </Badge>
          ))}
        </div>
      )}

      <PromptInput
        value={value}
        onValueChange={setValue}
        isLoading={disabled}
        onSubmit={handleSend}
        className="rounded-xl border border-border bg-background"
      >
        <PromptInputTextarea
          placeholder={placeholder}
          onKeyDown={handleKeyDown}
          className="min-h-5 resize-none border-0 bg-transparent p-3 text-[13px] shadow-none focus-visible:ring-0"
        />

        <PromptInputActions className="flex items-center gap-1.5 px-3 pb-2">
          {/* Attach */}
          <input
            ref={fileInputRef}
            type="file"
            multiple
            className="hidden"
            onChange={handleFileChange}
          />
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7 text-muted-foreground"
            onClick={() => fileInputRef.current?.click()}
            type="button"
            aria-label="Attach file"
          >
            <Paperclip className="h-3.5 w-3.5" />
          </Button>

          {/* Model selector */}
          <ModelSelector value={model} onValueChange={(v) => setModel(v as any)}>
            <ModelSelectorTrigger asChild>
              <Button variant="outline" size="sm" className="h-7 gap-1.5 px-2 text-[12px] text-muted-foreground">
                <span>✦</span><span>{modelLabel}</span>
              </Button>
            </ModelSelectorTrigger>
            <ModelSelectorContent>
              <ModelSelectorGroup>
                {MODELS.map(m => (
                  <ModelSelectorItem key={m.id} value={m.id}>
                    {m.label}
                  </ModelSelectorItem>
                ))}
              </ModelSelectorGroup>
            </ModelSelectorContent>
          </ModelSelector>

          {/* Effort level */}
          <ToggleGroup
            type="single"
            value={effort}
            onValueChange={(v) => v && setEffort(v as EffortLevel)}
            className="h-7 gap-0 rounded-md border border-border"
          >
            {EFFORT_LEVELS.map(l => (
              <ToggleGroupItem
                key={l.value}
                value={l.value}
                className="h-7 rounded-none px-2 text-[11px] first:rounded-l-md last:rounded-r-md data-[state=on]:bg-primary data-[state=on]:text-primary-foreground"
              >
                {l.label}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>

          {/* Token indicator */}
          {tokenPct > 0 && (
            <Badge variant="outline" className="h-7 gap-1.5 px-2 text-[11px] text-muted-foreground">
              <Progress value={tokenPct} className="h-[3px] w-7" />
              {tokenPct}%
            </Badge>
          )}

          {/* Send */}
          <Button
            size="icon"
            className="ml-auto h-7 w-7"
            onClick={handleSend}
            disabled={disabled || !value.trim()}
            aria-label="send"
            type="button"
          >
            <ArrowUp className="h-4 w-4" />
          </Button>
        </PromptInputActions>
      </PromptInput>
    </div>
  )
}
```

- [ ] **Step 5: Update `ChatView.tsx` — remove modelName/tokenPct props (now internal to ChatInput)**

```tsx
// web/src/components/chat/ChatView.tsx
import { useEffect, useRef } from 'react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { MessageBubble } from './MessageBubble'
import { ToolCallSeparator } from './ToolCallSeparator'
import { ChatInput } from './ChatInput'
import { ChatEmptyState } from './ChatEmptyState'
import type { ChatMessage } from '@/lib/types'

export type { ChatMessage }

interface ChatViewProps {
  messages: ChatMessage[]
  onSend: (content: string, attachments: File[]) => void
  inputPlaceholder?: string
  sending?: boolean
}

export function ChatView({
  messages,
  onSend,
  inputPlaceholder = 'Message…',
  sending,
}: ChatViewProps) {
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <ScrollArea className="flex-1">
        <div className="py-6">
          {messages.length === 0 && !sending ? (
            <ChatEmptyState />
          ) : (
            messages.map((msg, i) => (
              <div key={msg.id}>
                {i > 0 &&
                  msg.role === 'assistant' &&
                  messages[i - 1].role === 'user' &&
                  msg.toolCalls !== undefined &&
                  msg.durationSec !== undefined && (
                    <ToolCallSeparator toolCalls={msg.toolCalls} durationSec={msg.durationSec} />
                  )}
                <MessageBubble
                  role={msg.role}
                  content={msg.content}
                  authorName={msg.authorName}
                  authorInitials={msg.authorInitials}
                  modelName={msg.modelName}
                  timestamp={msg.timestamp}
                  isStreaming={msg.id === 'streaming'}
                />
              </div>
            ))
          )}
          <div ref={bottomRef} />
        </div>
      </ScrollArea>
      <ChatInput
        placeholder={inputPlaceholder}
        onSend={onSend}
        disabled={sending}
      />
    </div>
  )
}
```

- [ ] **Step 6: Update callers of `ChatView` — remove `modelName`/`tokenPct` props**

In `web/src/routes/chat/$chatId.tsx` and `web/src/routes/workspaces/$wsId/$step.tsx`, update the `onSend` signature to accept `(content: string, attachments: File[])`:

In `$chatId.tsx`:
```tsx
const handleSend = (content: string, _attachments: File[]) => {
```

In `$step.tsx`:
```tsx
const handleSend = (content: string, _attachments: File[]) => {
```

Remove `modelName` and `tokenPct` props from `<ChatView>` in both files.

- [ ] **Step 7: Update ChatInput tests**

```tsx
// web/src/__tests__/components/chat/ChatInput.test.tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { ChatInput } from '@/components/chat/ChatInput'
import { expect, test, vi } from 'vitest'

test('calls onSend with trimmed value and empty attachments', () => {
  const onSend = vi.fn()
  render(<ChatInput placeholder="Message…" onSend={onSend} />)
  const textarea = screen.getByPlaceholderText('Message…')
  fireEvent.change(textarea, { target: { value: '  hello  ' } })
  fireEvent.click(screen.getByRole('button', { name: /send/i }))
  expect(onSend).toHaveBeenCalledWith('hello', [])
})

test('clears input after send', () => {
  render(<ChatInput placeholder="Message…" onSend={() => {}} />)
  const textarea = screen.getByPlaceholderText('Message…')
  fireEvent.change(textarea, { target: { value: 'hello' } })
  fireEvent.click(screen.getByRole('button', { name: /send/i }))
  expect((textarea as HTMLTextAreaElement).value).toBe('')
})

test('send button disabled when empty', () => {
  render(<ChatInput placeholder="Message…" onSend={() => {}} />)
  expect(screen.getByRole('button', { name: /send/i })).toBeDisabled()
})

test('shows model selector trigger', () => {
  render(<ChatInput placeholder="Message…" onSend={() => {}} />)
  expect(screen.getByText('Sonnet 4.6')).toBeInTheDocument()
})

test('shows effort level toggles', () => {
  render(<ChatInput placeholder="Message…" onSend={() => {}} />)
  expect(screen.getByText('Mid')).toBeInTheDocument()
})

test('sends on Cmd+Enter', () => {
  const onSend = vi.fn()
  render(<ChatInput placeholder="Message…" onSend={onSend} />)
  const textarea = screen.getByPlaceholderText('Message…')
  fireEvent.change(textarea, { target: { value: 'hello' } })
  fireEvent.keyDown(textarea, { key: 'Enter', metaKey: true })
  expect(onSend).toHaveBeenCalledWith('hello', [])
})
```

- [ ] **Step 8: Run all tests**

```bash
cd web && npm test
```

Expected: all PASS.

- [ ] **Step 9: Commit**

```bash
git add -A && git commit -m "feat: rebuild ChatInput with PromptInput, ModelSelector, effort toggle, attachments, token counter"
```

---

## Wave 4 — Chat Experience

---

### Task 12: Add `ChatEmptyState` component

**Files:**
- Create: `web/src/components/chat/ChatEmptyState.tsx`
- Create: `web/src/__tests__/components/chat/ChatEmptyState.test.tsx`

- [ ] **Step 1: Write the failing test**

```tsx
// web/src/__tests__/components/chat/ChatEmptyState.test.tsx
import { render, screen } from '@testing-library/react'
import { ChatEmptyState } from '@/components/chat/ChatEmptyState'
import { expect, test } from 'vitest'

test('renders the empty state heading', () => {
  render(<ChatEmptyState />)
  expect(screen.getByText('Start a conversation')).toBeInTheDocument()
})
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd web && npm test -- ChatEmptyState.test
```

- [ ] **Step 3: Create `ChatEmptyState.tsx`**

```tsx
// web/src/components/chat/ChatEmptyState.tsx
export function ChatEmptyState() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-2 py-24 text-center">
      <span className="text-2xl text-primary/50">✦</span>
      <p className="text-sm font-medium text-foreground">Start a conversation</p>
      <p className="text-xs text-muted-foreground">Ask anything about this workspace</p>
    </div>
  )
}
```

- [ ] **Step 4: Run tests**

```bash
cd web && npm test -- ChatEmptyState.test
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: ChatEmptyState for new chats and workspaces"
```

---

### Task 13: Add streaming cursor to `MessageBubble`

The `isStreaming` flag (already passed from `ChatView` in Task 11) should render an animated cursor at the end of streaming content.

**Files:**
- Modify: `web/src/components/chat/MessageBubble.tsx`
- Modify: `web/src/__tests__/components/chat/MessageBubble.test.tsx`

- [ ] **Step 1: Write the failing test**

Add to `web/src/__tests__/components/chat/MessageBubble.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import { MessageBubble } from '@/components/chat/MessageBubble'
import { expect, test } from 'vitest'

test('shows streaming cursor when isStreaming=true', () => {
  render(
    <MessageBubble
      role="assistant"
      content="Hello"
      authorName="Claude"
      authorInitials="✦"
      timestamp="now"
      isStreaming={true}
    />
  )
  expect(document.querySelector('.animate-pulse')).toBeInTheDocument()
})

test('no streaming cursor when isStreaming=false', () => {
  render(
    <MessageBubble
      role="assistant"
      content="Hello"
      authorName="Claude"
      authorInitials="✦"
      timestamp="now"
    />
  )
  expect(document.querySelector('.animate-pulse')).not.toBeInTheDocument()
})
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd web && npm test -- MessageBubble.test
```

- [ ] **Step 3: Update `MessageBubble.tsx`**

```tsx
// web/src/components/chat/MessageBubble.tsx
import { cn } from '@/lib/utils'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'

interface MessageBubbleProps {
  role: 'user' | 'assistant'
  content: string
  authorName: string
  authorInitials: string
  modelName?: string
  timestamp: string
  isStreaming?: boolean
}

export function MessageBubble({
  role, content, authorName, authorInitials, modelName, timestamp, isStreaming,
}: MessageBubbleProps) {
  const isUser = role === 'user'

  return (
    <div className={cn('flex flex-col px-6 mb-4', isUser ? 'items-end' : 'items-start')}>
      <div
        className={cn(
          'max-w-[75%] rounded-xl px-3.5 py-2.5 text-[13.5px] leading-relaxed',
          isUser
            ? 'rounded-br-sm bg-primary/15 text-primary'
            : 'max-w-[80%] rounded-tl-sm border border-border bg-card text-foreground',
        )}
      >
        {content}
        {isStreaming && (
          <span className="ml-0.5 inline-block h-3.5 w-0.5 animate-pulse rounded-sm bg-primary/60 align-middle" />
        )}
      </div>
      {!isStreaming && (
        <div className="mt-1.5 flex items-center gap-1.5 text-[10.5px]">
          <Avatar className="h-[17px] w-[17px]">
            <AvatarFallback
              className={cn(
                'text-[7px] font-bold',
                isUser ? 'bg-muted text-muted-foreground' : 'bg-primary text-primary-foreground',
              )}
            >
              {authorInitials}
            </AvatarFallback>
          </Avatar>
          <span className="text-muted-foreground">{authorName}</span>
          {modelName && (
            <>
              <span className="text-muted-foreground/30">·</span>
              <span className="text-primary/70">{modelName}</span>
            </>
          )}
          <span className="text-muted-foreground/30">·</span>
          <span className="text-muted-foreground/50">{timestamp}</span>
        </div>
      )}
    </div>
  )
}
```

- [ ] **Step 4: Run all tests**

```bash
cd web && npm test
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: streaming cursor in MessageBubble"
```

---

## Wave 5 — Polish

---

### Task 14: Add `ErrorBoundary` component

**Files:**
- Create: `web/src/components/ErrorBoundary.tsx`
- Modify: `web/src/routes/__root.tsx`

- [ ] **Step 1: Create `ErrorBoundary.tsx`**

```tsx
// web/src/components/ErrorBoundary.tsx
import { Component } from 'react'
import type { ReactNode, ErrorInfo } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

interface Props {
  children: ReactNode
  fallback?: ReactNode
}

interface State {
  error: Error | null
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props)
    this.state = { error: null }
  }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('[ErrorBoundary]', error, info)
  }

  reset = () => this.setState({ error: null })

  render() {
    if (this.state.error) {
      if (this.props.fallback) return this.props.fallback
      return (
        <div className="flex flex-1 items-center justify-center p-8">
          <Card className="w-full max-w-sm border-destructive/20 bg-destructive/10">
            <CardHeader className="pb-2">
              <CardTitle className="text-sm text-destructive">Something went wrong</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <p className="font-mono text-[11px] text-muted-foreground">
                {this.state.error.message}
              </p>
              <Button variant="outline" size="sm" onClick={this.reset}>
                Try again
              </Button>
            </CardContent>
          </Card>
        </div>
      )
    }
    return this.props.children
  }
}
```

- [ ] **Step 2: Wrap `<Outlet />` in `__root.tsx`**

In `web/src/routes/__root.tsx`, update the `<AppShell>` body:

```tsx
import { ErrorBoundary } from '@/components/ErrorBoundary'

// inside RootLayout return, replace <Outlet /> with:
<ErrorBoundary>
  <Outlet />
</ErrorBoundary>
```

- [ ] **Step 3: Run tests**

```bash
cd web && npm test
```

Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "feat: ErrorBoundary wrapping all routes"
```

---

### Task 15: Add sidebar shimmer loading skeleton

Shown while the TanStack Query workspace data is loading (`isPending`).

**Files:**
- Create: `web/src/components/layout/SidebarSkeleton.tsx`
- Modify: `web/src/routes/__root.tsx`

- [ ] **Step 1: Create `SidebarSkeleton.tsx`**

```tsx
// web/src/components/layout/SidebarSkeleton.tsx
import { Skeleton } from '@/components/ui/skeleton'

export function SidebarSkeleton() {
  return (
    <div className="py-1 px-2 space-y-0.5">
      {/* Chat rows */}
      {[1, 2].map(i => (
        <div key={i} className="flex h-9 items-center gap-2 mx-1.5 px-2">
          <Skeleton className="h-5 w-5 rounded-md flex-shrink-0" />
          <Skeleton className="h-3 flex-1 rounded" />
          <Skeleton className="h-3 w-8 rounded" />
        </div>
      ))}
      <div className="my-1 mx-3 h-px bg-border" />
      {/* Repo + workspace rows */}
      {[1, 2].map(i => (
        <div key={i} className="space-y-0.5">
          <div className="flex h-9 items-center gap-2 mx-1.5 px-2">
            <Skeleton className="h-5 w-5 rounded-md flex-shrink-0" />
            <Skeleton className="h-3 w-24 rounded" />
          </div>
          {[1, 2].map(j => (
            <div key={j} className="flex h-9 items-center gap-2 mx-1.5 px-2 pl-6">
              <Skeleton className="h-3 flex-1 rounded" />
              <Skeleton className="h-3 w-12 rounded" />
            </div>
          ))}
        </div>
      ))}
    </div>
  )
}
```

> **Note:** `Shimmer` from AI Elements can be used here instead of plain `Skeleton` if you prefer an animated glow effect — just swap `<Skeleton>` for `<Shimmer>` and adjust class names per the AI Elements Shimmer docs.

- [ ] **Step 2: Run tests**

```bash
cd web && npm test
```

Expected: all PASS.

- [ ] **Step 3: Commit**

```bash
git add -A && git commit -m "feat: SidebarSkeleton shimmer loading state"
```

---

## Self-Review Checklist

- [x] **Spec coverage:**
  - Bug 1 (new chat empty state) → Task 4 ✓
  - Bug 2 (workspace not in sidebar) → Task 5 ✓
  - Repo collapse/expand → Task 3 (Sidebar.tsx) ✓
  - Project list page → Task 8 ✓
  - Import project (OS folder picker) → Task 9 ✓
  - Project switcher → Task 10 ✓
  - Projects breadcrumb link → Task 10 ✓
  - Model selector → Task 11 ✓
  - Effort level toggle → Task 11 ✓
  - Attachments → Task 11 ✓
  - Token counter (reactive) → Task 11 ✓
  - Mock streaming → Task 4 (`simulateStream`) ✓
  - Empty chat state → Task 12 ✓
  - Error boundaries → Task 14 ✓
  - Sidebar loading shimmer → Task 15 ✓
  - Streaming cursor → Task 13 ✓

- [x] **Placeholder scan:** No TBDs. AI Elements import path caveat is explicit with instructions to check actual install output.

- [x] **Type consistency:**
  - `ChatInput.onSend` is `(content: string, attachments: File[]) => void` throughout Tasks 11, 4, and callers.
  - `ProjectChat`, `Repo`, `Workspace` types exported from `lib/store/sidebar.ts` and re-exported from `Sidebar.tsx` for backwards compat.
  - `MessageBubble` accepts `isStreaming?: boolean` — added in Task 13, used in Task 11 (ChatView).
