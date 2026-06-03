# URL-Driven Routing & Mock API Layer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the single flat-route mock with URL-driven routing (`/workspaces/:wsId/:step`, `/chat/:chatId`, `/workspaces/new`) backed by a swappable mock API layer, so every workspace and flow step is addressable and swapping to the real API is a one-function change per query.

**Architecture:** Hash history (works on Tauri and browser with no server catch-all). Root layout holds AppShell + Sidebar so the resize state persists across navigation. Workspace routes derive their tabs from the backend-supplied flow definition — the frontend hardcodes nothing about step names. Mock data lives in `web/src/lib/mock/`; each fetch function is a single `apiFetch` call swap when the real API is ready.

**Tech Stack:** TanStack Router v1 (file-based routes, hash history), TanStack Query v5 (queryOptions pattern), Vite, React, TypeScript, shadcn/ui, Vitest + Testing Library.

---

## File Map

**Create:**
- `web/src/lib/types.ts` — shared types: `WorkspacePayload`, `FlowDefinition`, `FlowStateDefinition`, `ChatMessage`
- `web/src/lib/api.ts` — `apiFetch` with `window.__CROWBAR__` / `VITE_API_URL` fallback + typed fetch functions
- `web/src/lib/mock/flows.ts` — mock flow definitions
- `web/src/lib/mock/workspaces.ts` — mock workspace payloads + in-memory store for creation
- `web/src/lib/mock/conversations.ts` — mock messages keyed by `wsId:step`
- `web/src/lib/queries.ts` — TanStack Query option factories
- `web/src/components/layout/WorkspaceStepTabs.tsx` — tab bar driven by `flow.states`
- `web/src/components/review/DiffView.tsx` — placeholder for `ai_review` / `human_review` states
- `web/src/components/workspace/WorkspaceCreationForm.tsx` — creation form (testable without router)
- `web/src/routes/chat/$chatId.tsx` — standalone chat route
- `web/src/routes/workspaces/new.tsx` — creation wizard route
- `web/src/routes/workspaces/$wsId.tsx` — workspace layout (tabs + Outlet)
- `web/src/routes/workspaces/$wsId/index.tsx` — redirects `/workspaces/:wsId` to `currentState`
- `web/src/routes/workspaces/$wsId/$step.tsx` — renders `ChatView` or `DiffView` based on `state.ui`
- `web/src/__tests__/components/layout/WorkspaceStepTabs.test.tsx`
- `web/src/__tests__/components/review/DiffView.test.tsx`
- `web/src/__tests__/components/workspace/WorkspaceCreationForm.test.tsx`

**Modify:**
- `web/src/main.tsx` — add `createHashHistory()`
- `web/vite.config.ts` — pin dev server to port 5173
- `web/src/routes/__root.tsx` — add AppShell + Sidebar + navigation
- `web/src/routes/index.tsx` — replace mock page with redirect to `/workspaces/ws3`
- `web/src/components/chat/ChatView.tsx` — remove hardcoded `STEPS`/`FlowStep`, accept `ChatMessage[]` only
- `web/src/components/layout/Sidebar.tsx` — change `onNewWorkspace` to `() => void`
- `web/src/__tests__/components/chat/ChatView.test.tsx` — remove tab tests, update render calls

---

### Task 1: Hash history + Vite port pin

**Files:**
- Modify: `web/src/main.tsx`
- Modify: `web/vite.config.ts`

- [ ] **Step 1: Switch to hash history in main.tsx**

```tsx
// web/src/main.tsx  (full file)
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider, createRouter, createHashHistory } from '@tanstack/react-router'
import { QueryClientProvider } from '@tanstack/react-query'
import { routeTree } from './routeTree.gen'
import { queryClient } from './lib/query'
import './index.css'

const router = createRouter({ routeTree, history: createHashHistory() })

declare module '@tanstack/react-router' {
  interface Register { router: typeof router }
}

document.documentElement.classList.add('dark')

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
)
```

- [ ] **Step 2: Pin the Vite dev server to port 5173**

Add `server: { port: 5173 }` to `web/vite.config.ts` between the `plugins` block and `resolve`:

```ts
// web/vite.config.ts  (full file)
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { TanStackRouterVite } from '@tanstack/router-plugin/vite'
import path from 'path'

export default defineConfig({
  plugins: [
    TanStackRouterVite({ target: 'react', autoCodeSplitting: true }),
    react(),
    tailwindcss(),
  ],
  server: { port: 5173 },
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  test: {
    environment: 'jsdom',
    environmentOptions: { jsdom: { url: 'http://localhost' } },
    globals: true,
    setupFiles: ['./src/__tests__/setup.ts'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'lcov'],
      thresholds: { lines: 70, functions: 70, branches: 70, statements: 70 },
    },
  },
})
```

- [ ] **Step 3: Verify dev server starts on 5173**

Run: `cd web && npm run dev`
Expected: "Local: http://localhost:5173/"

- [ ] **Step 4: Commit**

```bash
git add web/src/main.tsx web/vite.config.ts
git commit -m "feat: switch to hash history, pin Vite dev server to port 5173"
```

---

### Task 2: Core types module

**Files:**
- Create: `web/src/lib/types.ts`

- [ ] **Step 1: Create the types file**

```ts
// web/src/lib/types.ts
export type UIMode = 'chat' | 'diff'

export interface FlowStateDefinition {
  name: string    // machine name: 'brainstorming', 'ai_review'
  label: string   // display name: 'Brainstorm', 'AI Review'
  ui: UIMode
}

export interface FlowDefinition {
  name: string
  description: string
  states: FlowStateDefinition[]
}

export interface WorkspacePayload {
  id: string
  repoId: string
  branch: string
  flowName: string
  currentState: string
  flow: FlowDefinition
}

export interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  authorName: string
  authorInitials: string
  modelName?: string
  timestamp: string
  toolCalls?: number
  durationSec?: number
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web && npx tsc --noEmit`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/types.ts
git commit -m "feat: add shared types module (WorkspacePayload, FlowDefinition, ChatMessage)"
```

---

### Task 3: API adapter + mock data + query factories

**Files:**
- Create: `web/src/lib/mock/flows.ts`
- Create: `web/src/lib/mock/workspaces.ts`
- Create: `web/src/lib/mock/conversations.ts`
- Create: `web/src/lib/api.ts`
- Create: `web/src/lib/queries.ts`

- [ ] **Step 1: Create mock flow definitions**

```ts
// web/src/lib/mock/flows.ts
import type { FlowDefinition } from '@/lib/types'

export const FEATURE_DEV_FLOW: FlowDefinition = {
  name: 'feature-development',
  description: 'Full feature development — brainstorm to reviewed implementation',
  states: [
    { name: 'brainstorming',  label: 'Brainstorm',    ui: 'chat' },
    { name: 'spec',           label: 'Spec',           ui: 'chat' },
    { name: 'implementation', label: 'Build',          ui: 'chat' },
    { name: 'ai_review',      label: 'AI Review',      ui: 'diff' },
    { name: 'human_review',   label: 'Human Review',   ui: 'diff' },
  ],
}

export const MOCK_FLOWS: FlowDefinition[] = [FEATURE_DEV_FLOW]
```

- [ ] **Step 2: Create mock workspace store**

```ts
// web/src/lib/mock/workspaces.ts
import type { WorkspacePayload } from '@/lib/types'
import { FEATURE_DEV_FLOW, MOCK_FLOWS } from './flows'

const INITIAL_WORKSPACES: WorkspacePayload[] = [
  {
    id: 'ws3', repoId: 'crowbar', branch: 'feature/app-design',
    flowName: 'feature-development', currentState: 'brainstorming',
    flow: FEATURE_DEV_FLOW,
  },
  {
    id: 'ws2', repoId: 'crowbar', branch: 'feature/api-backend',
    flowName: 'feature-development', currentState: 'implementation',
    flow: FEATURE_DEV_FLOW,
  },
  {
    id: 'ws1', repoId: 'crowbar', branch: 'enhancement/scaffold',
    flowName: 'feature-development', currentState: 'human_review',
    flow: FEATURE_DEV_FLOW,
  },
  {
    id: 'qc1', repoId: 'quiver-core', branch: 'develop',
    flowName: 'feature-development', currentState: 'brainstorming',
    flow: FEATURE_DEV_FLOW,
  },
  {
    id: 'qd1', repoId: 'quiver-desktop', branch: 'develop',
    flowName: 'feature-development', currentState: 'brainstorming',
    flow: FEATURE_DEV_FLOW,
  },
  {
    id: 'qd2', repoId: 'quiver-desktop', branch: 'feature/quiver-shell',
    flowName: 'feature-development', currentState: 'spec',
    flow: FEATURE_DEV_FLOW,
  },
]

const store = new Map<string, WorkspacePayload>(
  INITIAL_WORKSPACES.map(ws => [ws.id, ws]),
)

export function getMockWorkspace(wsId: string): WorkspacePayload | undefined {
  return store.get(wsId)
}

export function createMockWorkspace(
  repoId: string,
  branch: string,
  flowName: string,
): WorkspacePayload {
  const flow = MOCK_FLOWS.find(f => f.name === flowName) ?? FEATURE_DEV_FLOW
  const id = `ws-${Date.now()}`
  const ws: WorkspacePayload = {
    id, repoId, branch, flowName,
    currentState: flow.states[0].name,
    flow,
  }
  store.set(id, ws)
  return ws
}
```

- [ ] **Step 3: Create mock conversation store**

```ts
// web/src/lib/mock/conversations.ts
import type { ChatMessage } from '@/lib/types'

const MOCK_MESSAGES: Record<string, ChatMessage[]> = {
  'ws3:brainstorming': [
    {
      id: 'm1', role: 'user',
      content: 'How should we handle auth across crowbar, quiver.core and quiver.desktop?',
      authorName: 'Mateo', authorInitials: 'MU', timestamp: '2h ago',
    },
    {
      id: 'm2', role: 'assistant',
      content: 'Given all three share a user identity, a shared auth service makes the most sense — lightweight Go, token issuance and refresh, each app verifying JWTs locally.',
      authorName: 'Claude', authorInitials: '✦', modelName: 'Sonnet 4.6',
      timestamp: '2h ago · 18.3s', toolCalls: 4, durationSec: 18.3,
    },
    {
      id: 'm3', role: 'user', content: "Makes sense. Let's go with that.",
      authorName: 'Mateo', authorInitials: 'MU', timestamp: '2h ago',
    },
  ],
  'ws2:implementation': [
    {
      id: 'i1', role: 'assistant',
      content: 'Starting implementation. Creating tasks for the API backend...',
      authorName: 'Claude', authorInitials: '✦', modelName: 'Sonnet 4.6',
      timestamp: '1d ago',
    },
  ],
}

export function getMockConversation(wsId: string, step: string): ChatMessage[] {
  return MOCK_MESSAGES[`${wsId}:${step}`] ?? []
}
```

- [ ] **Step 4: Create the API adapter**

```ts
// web/src/lib/api.ts
import type { WorkspacePayload, FlowDefinition, ChatMessage } from './types'
import { getMockWorkspace, createMockWorkspace } from './mock/workspaces'
import { MOCK_FLOWS } from './mock/flows'
import { getMockConversation } from './mock/conversations'

const crowbar = (window as any).__CROWBAR__
export const API_BASE = crowbar?.api ?? import.meta.env.VITE_API_URL ?? ''

export function apiFetch(path: string, init?: RequestInit): Promise<unknown> {
  return fetch(`${API_BASE}${path}`, init).then(r => {
    if (!r.ok) throw new Error(`${r.status} ${r.statusText}`)
    return r.json()
  })
}

export function fetchWorkspace(wsId: string): Promise<WorkspacePayload> {
  const ws = getMockWorkspace(wsId)
  if (!ws) return Promise.reject(new Error(`Unknown workspace: ${wsId}`))
  return Promise.resolve(ws)
}

export function postWorkspace(
  repoId: string,
  branch: string,
  flowName: string,
): Promise<WorkspacePayload> {
  return Promise.resolve(createMockWorkspace(repoId, branch, flowName))
}

export function fetchFlows(): Promise<FlowDefinition[]> {
  return Promise.resolve(MOCK_FLOWS)
}

export function fetchConversation(
  wsId: string,
  step: string,
): Promise<{ messages: ChatMessage[] }> {
  return Promise.resolve({ messages: getMockConversation(wsId, step) })
}
```

- [ ] **Step 5: Create query factories**

```ts
// web/src/lib/queries.ts
import { queryOptions } from '@tanstack/react-query'
import { fetchWorkspace, fetchFlows, fetchConversation } from './api'

export const workspaceQueryOptions = (wsId: string) =>
  queryOptions({
    queryKey: ['workspace', wsId],
    queryFn: () => fetchWorkspace(wsId),
  })

export const flowsQueryOptions = () =>
  queryOptions({
    queryKey: ['flows'],
    queryFn: fetchFlows,
  })

export const conversationQueryOptions = (wsId: string, step: string) =>
  queryOptions({
    queryKey: ['conversation', wsId, step],
    queryFn: () => fetchConversation(wsId, step),
  })
```

- [ ] **Step 6: Verify TypeScript compiles**

Run: `cd web && npx tsc --noEmit`
Expected: no errors

- [ ] **Step 7: Commit**

```bash
git add web/src/lib/
git commit -m "feat: API adapter, mock data modules, and query factories"
```

---

### Task 4: WorkspaceStepTabs component (TDD)

**Files:**
- Create: `web/src/components/layout/WorkspaceStepTabs.tsx`
- Create: `web/src/__tests__/components/layout/WorkspaceStepTabs.test.tsx`

- [ ] **Step 1: Write the failing tests**

```tsx
// web/src/__tests__/components/layout/WorkspaceStepTabs.test.tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { WorkspaceStepTabs } from '@/components/layout/WorkspaceStepTabs'
import type { FlowStateDefinition } from '@/lib/types'

const STATES: FlowStateDefinition[] = [
  { name: 'brainstorming', label: 'Brainstorm', ui: 'chat' },
  { name: 'spec',          label: 'Spec',        ui: 'chat' },
  { name: 'ai_review',     label: 'AI Review',   ui: 'diff' },
]

test('renders all state labels', () => {
  render(<WorkspaceStepTabs states={STATES} currentStep="brainstorming" onStepChange={() => {}} />)
  expect(screen.getByText('Brainstorm')).toBeInTheDocument()
  expect(screen.getByText('Spec')).toBeInTheDocument()
  expect(screen.getByText('AI Review')).toBeInTheDocument()
})

test('calls onStepChange with state name when tab clicked', () => {
  const onStepChange = vi.fn()
  render(<WorkspaceStepTabs states={STATES} currentStep="brainstorming" onStepChange={onStepChange} />)
  fireEvent.click(screen.getByText('Spec'))
  expect(onStepChange).toHaveBeenCalledWith('spec')
})

test('renders separator chevrons between tabs', () => {
  render(<WorkspaceStepTabs states={STATES} currentStep="brainstorming" onStepChange={() => {}} />)
  expect(screen.getAllByText('›')).toHaveLength(2)
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && npx vitest run src/__tests__/components/layout/WorkspaceStepTabs.test.tsx`
Expected: FAIL — "Cannot find module '@/components/layout/WorkspaceStepTabs'"

- [ ] **Step 3: Implement WorkspaceStepTabs**

```tsx
// web/src/components/layout/WorkspaceStepTabs.tsx
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { FlowStateDefinition } from '@/lib/types'

interface WorkspaceStepTabsProps {
  states: FlowStateDefinition[]
  currentStep: string
  onStepChange: (step: string) => void
}

function StepDot({ state }: { state: 'done' | 'active' | 'pending' }) {
  return (
    <span className={
      'h-1.5 w-1.5 rounded-full flex-shrink-0 ' +
      (state === 'done' ? 'bg-green-500' : state === 'active' ? 'bg-primary' : 'bg-muted')
    } />
  )
}

export function WorkspaceStepTabs({ states, currentStep, onStepChange }: WorkspaceStepTabsProps) {
  const activeIdx = states.findIndex(s => s.name === currentStep)
  return (
    <Tabs value={currentStep} onValueChange={onStepChange}>
      <TabsList className="h-10 w-full justify-start gap-0 rounded-none border-b border-border bg-card px-4">
        {states.map((s, i) => (
          <div key={s.name} className="flex items-center">
            <TabsTrigger
              value={s.name}
              className="flex items-center gap-1.5 rounded-none border-b-2 border-transparent px-3 py-2 text-[13px] text-muted-foreground data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:text-foreground data-[state=active]:shadow-none"
            >
              <StepDot state={i < activeIdx ? 'done' : i === activeIdx ? 'active' : 'pending'} />
              {s.label}
            </TabsTrigger>
            {i < states.length - 1 && (
              <span className="mx-0.5 text-[12px] text-muted-foreground/30">›</span>
            )}
          </div>
        ))}
      </TabsList>
    </Tabs>
  )
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run src/__tests__/components/layout/WorkspaceStepTabs.test.tsx`
Expected: PASS (3 tests)

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/WorkspaceStepTabs.tsx web/src/__tests__/components/layout/WorkspaceStepTabs.test.tsx
git commit -m "feat: WorkspaceStepTabs driven by flow state definitions"
```

---

### Task 5: Refactor ChatView — remove hardcoded tabs

**Files:**
- Modify: `web/src/components/chat/ChatView.tsx`
- Modify: `web/src/__tests__/components/chat/ChatView.test.tsx`

`ChatMessage` moves to `@/lib/types`. The `FlowStep` type and `STEPS` constant are deleted. ChatView becomes a pure message list + input — no tabs.

- [ ] **Step 1: Update the ChatView tests first**

Replace `web/src/__tests__/components/chat/ChatView.test.tsx` entirely:

```tsx
// web/src/__tests__/components/chat/ChatView.test.tsx
import { render, screen } from '@testing-library/react'
import { ChatView } from '@/components/chat/ChatView'
import type { ChatMessage } from '@/lib/types'

const noop = () => {}

test('renders messages', () => {
  const messages: ChatMessage[] = [
    { id: '1', role: 'user', content: 'Hello world', authorName: 'Mateo', authorInitials: 'MU', timestamp: 'now' },
  ]
  render(<ChatView messages={messages} onSend={noop} />)
  expect(screen.getByText('Hello world')).toBeInTheDocument()
})

test('renders input placeholder', () => {
  render(<ChatView messages={[]} onSend={noop} inputPlaceholder="Type here…" />)
  expect(screen.getByPlaceholderText('Type here…')).toBeInTheDocument()
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && npx vitest run src/__tests__/components/chat/ChatView.test.tsx`
Expected: FAIL — ChatView still expects `step`/`onStepChange` props that the tests no longer pass

- [ ] **Step 3: Rewrite ChatView**

```tsx
// web/src/components/chat/ChatView.tsx
import { useEffect, useRef } from 'react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { MessageBubble } from './MessageBubble'
import { ToolCallSeparator } from './ToolCallSeparator'
import { ChatInput } from './ChatInput'
import type { ChatMessage } from '@/lib/types'

export type { ChatMessage }

interface ChatViewProps {
  messages: ChatMessage[]
  onSend: (content: string) => void
  modelName?: string
  tokenPct?: number
  inputPlaceholder?: string
  sending?: boolean
}

export function ChatView({
  messages,
  onSend,
  modelName = 'Sonnet 4.6',
  tokenPct = 0,
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
          {messages.map((msg, i) => (
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
              />
            </div>
          ))}
          <div ref={bottomRef} />
        </div>
      </ScrollArea>
      <ChatInput
        placeholder={inputPlaceholder}
        onSend={onSend}
        modelName={modelName}
        tokenPct={tokenPct}
        disabled={sending}
      />
    </div>
  )
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run src/__tests__/components/chat/ChatView.test.tsx`
Expected: PASS (2 tests)

- [ ] **Step 5: Run full suite to confirm nothing else broke**

Run: `cd web && npx vitest run`
Expected: all tests pass

- [ ] **Step 6: Commit**

```bash
git add web/src/components/chat/ChatView.tsx web/src/__tests__/components/chat/ChatView.test.tsx
git commit -m "refactor: remove hardcoded flow tabs from ChatView"
```

---

### Task 6: DiffView placeholder (TDD)

**Files:**
- Create: `web/src/components/review/DiffView.tsx`
- Create: `web/src/__tests__/components/review/DiffView.test.tsx`

- [ ] **Step 1: Write the failing test**

```tsx
// web/src/__tests__/components/review/DiffView.test.tsx
import { render, screen } from '@testing-library/react'
import { DiffView } from '@/components/review/DiffView'

test('renders workspace id and step name', () => {
  render(<DiffView workspaceId="ws1" step="ai_review" />)
  expect(screen.getByText(/ws1/)).toBeInTheDocument()
  expect(screen.getByText(/ai_review/)).toBeInTheDocument()
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/__tests__/components/review/DiffView.test.tsx`
Expected: FAIL — "Cannot find module '@/components/review/DiffView'"

- [ ] **Step 3: Implement DiffView placeholder**

```tsx
// web/src/components/review/DiffView.tsx
interface DiffViewProps {
  workspaceId: string
  step: string
}

export function DiffView({ workspaceId, step }: DiffViewProps) {
  return (
    <div className="flex flex-1 items-center justify-center text-muted-foreground">
      <div className="text-center">
        <div className="text-sm font-medium">Review view coming soon</div>
        <div className="mt-1 text-xs">{workspaceId} · {step}</div>
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/__tests__/components/review/DiffView.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/components/review/ web/src/__tests__/components/review/
git commit -m "feat: DiffView placeholder for ai_review / human_review states"
```

---

### Task 7: Root layout — wire AppShell + Sidebar + navigation

**Files:**
- Modify: `web/src/routes/__root.tsx`
- Modify: `web/src/components/layout/Sidebar.tsx`

The root layout holds `AppShell` + `Sidebar` so the panel width persists across navigation. Active workspace/chat is derived from the current URL so no extra state is needed.

- [ ] **Step 1: Change Sidebar.onNewWorkspace signature**

In `web/src/components/layout/Sidebar.tsx`, make two edits:

Change the interface line:
```ts
// old:
onNewWorkspace?: (repoId: string) => void
// new:
onNewWorkspace?: () => void
```

Change the NewRow callback inside the `repos.map`:
```tsx
// old:
<NewRow label="New workspace" onClick={() => onNewWorkspace?.(repo.id)} />
// new:
<NewRow label="New workspace" onClick={onNewWorkspace} />
```

- [ ] **Step 2: Run existing Sidebar tests to confirm they still pass**

Run: `cd web && npx vitest run src/__tests__/components/layout/Sidebar.test.tsx`
Expected: PASS (3 tests — the tests don't use onNewWorkspace)

- [ ] **Step 3: Replace __root.tsx**

```tsx
// web/src/routes/__root.tsx
import { createRootRoute, Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import { AppShell } from '@/components/layout/AppShell'
import { Sidebar, type Repo, type ProjectChat } from '@/components/layout/Sidebar'

const MOCK_CHATS: ProjectChat[] = [
  { id: 'c1', title: 'Architecture decisions', age: '2h' },
  { id: 'c2', title: 'Auth strategy across services', age: '5d' },
]

const MOCK_REPOS: Repo[] = [
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

function RootLayout() {
  const navigate = useNavigate()
  const pathname = useRouterState({ select: s => s.location.pathname })

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1]
  const activeChatId = pathname.match(/\/chat\/([^/]+)/)?.[1]

  return (
    <AppShell
      sidebar={
        <Sidebar
          projectName="Rabbyte"
          userInitials="MU"
          chats={MOCK_CHATS}
          repos={MOCK_REPOS}
          activeChatId={activeChatId}
          activeWorkspaceId={activeWorkspaceId}
          onChatClick={(id) => navigate({ to: '/chat/$chatId', params: { chatId: id } })}
          onWorkspaceClick={(_, wsId) => navigate({ to: '/workspaces/$wsId', params: { wsId } })}
          onNewChat={() => navigate({ to: '/chat/$chatId', params: { chatId: 'new' } })}
          onNewWorkspace={() => navigate({ to: '/workspaces/new' })}
        />
      }
    >
      <Outlet />
    </AppShell>
  )
}

export const Route = createRootRoute({ component: RootLayout })
```

- [ ] **Step 4: Verify TypeScript compiles**

Run: `cd web && npx tsc --noEmit`
Expected: no errors

- [ ] **Step 5: Run full test suite**

Run: `cd web && npx vitest run`
Expected: all tests pass

- [ ] **Step 6: Commit**

```bash
git add web/src/routes/__root.tsx web/src/components/layout/Sidebar.tsx
git commit -m "feat: root layout with persistent AppShell + Sidebar and URL-driven navigation"
```

---

### Task 8: Workspace routes

**Files:**
- Create: `web/src/routes/workspaces/$wsId.tsx`
- Create: `web/src/routes/workspaces/$wsId/index.tsx`
- Create: `web/src/routes/workspaces/$wsId/$step.tsx`
- Modify: `web/src/routes/index.tsx`

After creating new route files, always run `cd web && npx tsr generate` to update `routeTree.gen.ts`.

- [ ] **Step 1: Create the workspace layout route**

Create the directory first if needed: `mkdir -p web/src/routes/workspaces`

```tsx
// web/src/routes/workspaces/$wsId.tsx
import { createFileRoute, Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { workspaceQueryOptions } from '@/lib/queries'
import { WorkspaceStepTabs } from '@/components/layout/WorkspaceStepTabs'

export const Route = createFileRoute('/workspaces/$wsId')({
  component: WorkspaceLayout,
})

function WorkspaceLayout() {
  const { wsId } = Route.useParams()
  const navigate = useNavigate()
  const pathname = useRouterState({ select: s => s.location.pathname })
  const { data: workspace } = useQuery(workspaceQueryOptions(wsId))

  if (!workspace) {
    return (
      <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
        Loading…
      </div>
    )
  }

  const currentStep = pathname.split('/').pop() ?? workspace.currentState

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <WorkspaceStepTabs
        states={workspace.flow.states}
        currentStep={currentStep}
        onStepChange={(step) =>
          navigate({ to: '/workspaces/$wsId/$step', params: { wsId, step } })
        }
      />
      <Outlet />
    </div>
  )
}
```

- [ ] **Step 2: Create the workspace index redirect**

Create directory: `mkdir -p web/src/routes/workspaces/$wsId`

```tsx
// web/src/routes/workspaces/$wsId/index.tsx
import { createFileRoute, redirect } from '@tanstack/react-router'
import { getMockWorkspace } from '@/lib/mock/workspaces'

export const Route = createFileRoute('/workspaces/$wsId/')({
  beforeLoad: ({ params }) => {
    const ws = getMockWorkspace(params.wsId)
    throw redirect({
      to: '/workspaces/$wsId/$step',
      params: { wsId: params.wsId, step: ws?.currentState ?? 'brainstorming' },
    })
  },
  component: () => null,
})
```

- [ ] **Step 3: Create the workspace step route**

```tsx
// web/src/routes/workspaces/$wsId/$step.tsx
import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { workspaceQueryOptions } from '@/lib/queries'
import { getMockConversation } from '@/lib/mock/conversations'
import { ChatView } from '@/components/chat/ChatView'
import { DiffView } from '@/components/review/DiffView'
import type { ChatMessage } from '@/lib/types'

export const Route = createFileRoute('/workspaces/$wsId/$step')({
  component: StepPage,
})

function StepPage() {
  const { wsId, step } = Route.useParams()
  const { data: workspace } = useQuery(workspaceQueryOptions(wsId))

  if (!workspace) return null

  const stateDef = workspace.flow.states.find(s => s.name === step)
  const ui = stateDef?.ui ?? 'chat'

  if (ui === 'diff') {
    return <DiffView workspaceId={wsId} step={step} />
  }

  return <WorkspaceChatView workspaceId={wsId} step={step} />
}

function WorkspaceChatView({ workspaceId, step }: { workspaceId: string; step: string }) {
  const [messages, setMessages] = useState<ChatMessage[]>(() =>
    getMockConversation(workspaceId, step),
  )
  const [sending, setSending] = useState(false)

  const handleSend = (content: string) => {
    setMessages(prev => [...prev, {
      id: `u${Date.now()}`, role: 'user', content,
      authorName: 'Mateo', authorInitials: 'MU', timestamp: 'just now',
    }])
    setSending(true)
    setTimeout(() => {
      setMessages(prev => [...prev, {
        id: `a${Date.now()}`, role: 'assistant',
        content: '(Mock response — AI not yet connected)',
        authorName: 'Claude', authorInitials: '✦', modelName: 'Sonnet 4.6',
        timestamp: 'just now', toolCalls: 0, durationSec: 1.5,
      }])
      setSending(false)
    }, 1500)
  }

  return (
    <ChatView
      messages={messages}
      onSend={handleSend}
      sending={sending}
      modelName="Sonnet 4.6"
      tokenPct={12}
      inputPlaceholder={`Message… (${step})`}
    />
  )
}
```

- [ ] **Step 4: Replace index.tsx with a redirect**

```tsx
// web/src/routes/index.tsx
import { createFileRoute, redirect } from '@tanstack/react-router'

export const Route = createFileRoute('/')({
  component: () => null,
  beforeLoad: () => {
    throw redirect({ to: '/workspaces/$wsId', params: { wsId: 'ws3' } })
  },
})
```

- [ ] **Step 5: Regenerate route tree**

Run: `cd web && npx tsr generate`
Expected: `routeTree.gen.ts` updated — new routes appear in the exports

- [ ] **Step 6: Verify the app works end-to-end**

Run: `cd web && npm run dev`

Open http://localhost:5173 — expected:
- Redirects to `/#/workspaces/ws3/brainstorming`
- Shows AppShell with Sidebar on the left
- Shows tab bar: Brainstorm (active) › Spec › Build › AI Review › Human Review
- Shows the three mock messages from `ws3:brainstorming`

Click the "Spec" tab — URL changes to `/#/workspaces/ws3/spec`, message list clears (no mock data for that step).

Click "ws2 · feature/api-backend" in sidebar — URL changes to `/#/workspaces/ws2/implementation`, shows the single mock message.

Click "AI Review" tab — URL changes to `/#/workspaces/ws2/ai_review`, DiffView placeholder renders.

- [ ] **Step 7: Run full test suite**

Run: `cd web && npx vitest run`
Expected: all tests pass

- [ ] **Step 8: Commit**

```bash
git add web/src/routes/ web/src/routeTree.gen.ts
git commit -m "feat: workspace routes — dynamic tabs, per-step ChatView/DiffView, mock AI reply"
```

---

### Task 9: Standalone chat route

**Files:**
- Create: `web/src/routes/chat/$chatId.tsx`

- [ ] **Step 1: Create the directory and route**

Run: `mkdir -p web/src/routes/chat`

```tsx
// web/src/routes/chat/$chatId.tsx
import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { ChatView } from '@/components/chat/ChatView'
import type { ChatMessage } from '@/lib/types'

export const Route = createFileRoute('/chat/$chatId')({
  component: ChatPage,
})

const MOCK_CHAT_MESSAGES: Record<string, ChatMessage[]> = {
  c1: [
    {
      id: 'a1', role: 'user',
      content: 'How should we structure the architecture across all three Rabbyte products?',
      authorName: 'Mateo', authorInitials: 'MU', timestamp: '2h ago',
    },
    {
      id: 'a2', role: 'assistant',
      content: 'The three products share a user identity layer, so a shared auth service is the right foundation. crowbar handles agent orchestration, quiver.core is the shared library, and quiver.desktop consumes both.',
      authorName: 'Claude', authorInitials: '✦', modelName: 'Sonnet 4.6',
      timestamp: '2h ago · 6.1s', toolCalls: 2, durationSec: 6.1,
    },
  ],
}

function ChatPage() {
  const { chatId } = Route.useParams()
  const [messages, setMessages] = useState<ChatMessage[]>(() =>
    MOCK_CHAT_MESSAGES[chatId] ?? [],
  )
  const [sending, setSending] = useState(false)

  const handleSend = (content: string) => {
    setMessages(prev => [...prev, {
      id: `u${Date.now()}`, role: 'user', content,
      authorName: 'Mateo', authorInitials: 'MU', timestamp: 'just now',
    }])
    setSending(true)
    setTimeout(() => {
      setMessages(prev => [...prev, {
        id: `a${Date.now()}`, role: 'assistant',
        content: '(Mock response — AI not yet connected)',
        authorName: 'Claude', authorInitials: '✦', modelName: 'Sonnet 4.6',
        timestamp: 'just now',
      }])
      setSending(false)
    }, 1500)
  }

  return (
    <ChatView
      messages={messages}
      onSend={handleSend}
      sending={sending}
      modelName="Sonnet 4.6"
      inputPlaceholder="Ask about the Rabbyte project…"
    />
  )
}
```

- [ ] **Step 2: Regenerate route tree**

Run: `cd web && npx tsr generate`

- [ ] **Step 3: Verify clicking a chat navigates correctly**

In the running dev server, click "Architecture decisions" in the sidebar.
Expected: URL changes to `/#/chat/c1`, two mock messages appear, no flow tabs.

- [ ] **Step 4: Commit**

```bash
git add web/src/routes/chat/ web/src/routeTree.gen.ts
git commit -m "feat: standalone chat route at /chat/:chatId"
```

---

### Task 10: WorkspaceCreationForm + wizard route (TDD)

**Files:**
- Create: `web/src/components/workspace/WorkspaceCreationForm.tsx`
- Create: `web/src/__tests__/components/workspace/WorkspaceCreationForm.test.tsx`
- Create: `web/src/routes/workspaces/new.tsx`

- [ ] **Step 1: Write the failing tests**

```tsx
// web/src/__tests__/components/workspace/WorkspaceCreationForm.test.tsx
import { render, screen, fireEvent } from '@testing-library/react'
import { WorkspaceCreationForm } from '@/components/workspace/WorkspaceCreationForm'

const REPOS = [
  { id: 'crowbar', name: 'crowbar' },
  { id: 'quiver-core', name: 'quiver.core' },
]
const FLOWS = [
  { name: 'feature-development', description: 'Full feature development' },
]

test('renders repo select, branch input, and workflow select', () => {
  render(<WorkspaceCreationForm repos={REPOS} flows={FLOWS} onSubmit={() => {}} />)
  expect(screen.getByLabelText('Repo')).toBeInTheDocument()
  expect(screen.getByLabelText('Branch')).toBeInTheDocument()
  expect(screen.getByLabelText('Workflow')).toBeInTheDocument()
})

test('Create button is disabled when branch is empty', () => {
  render(<WorkspaceCreationForm repos={REPOS} flows={FLOWS} onSubmit={() => {}} />)
  expect(screen.getByRole('button', { name: /create/i })).toBeDisabled()
})

test('calls onSubmit with selected values', () => {
  const onSubmit = vi.fn()
  render(<WorkspaceCreationForm repos={REPOS} flows={FLOWS} onSubmit={onSubmit} />)
  fireEvent.change(screen.getByLabelText('Repo'), { target: { value: 'quiver-core' } })
  fireEvent.change(screen.getByLabelText('Branch'), { target: { value: 'feature/new-thing' } })
  fireEvent.change(screen.getByLabelText('Workflow'), { target: { value: 'feature-development' } })
  fireEvent.click(screen.getByRole('button', { name: /create/i }))
  expect(onSubmit).toHaveBeenCalledWith({
    repoId: 'quiver-core',
    branch: 'feature/new-thing',
    flowName: 'feature-development',
  })
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && npx vitest run src/__tests__/components/workspace/WorkspaceCreationForm.test.tsx`
Expected: FAIL — "Cannot find module"

- [ ] **Step 3: Implement WorkspaceCreationForm**

```tsx
// web/src/components/workspace/WorkspaceCreationForm.tsx
import { useState } from 'react'
import { Button } from '@/components/ui/button'

interface Props {
  repos: { id: string; name: string }[]
  flows: { name: string; description: string }[]
  onSubmit: (data: { repoId: string; branch: string; flowName: string }) => void
  loading?: boolean
}

export function WorkspaceCreationForm({ repos, flows, onSubmit, loading }: Props) {
  const [repoId, setRepoId] = useState(repos[0]?.id ?? '')
  const [branch, setBranch] = useState('')
  const [flowName, setFlowName] = useState(flows[0]?.name ?? '')

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit({ repoId, branch, flowName })
      }}
    >
      <label className="flex flex-col gap-1 text-sm">
        <span className="text-muted-foreground">Repo</span>
        <select
          aria-label="Repo"
          value={repoId}
          onChange={e => setRepoId(e.target.value)}
          className="rounded-md border border-border bg-background px-3 py-2 text-foreground"
        >
          {repos.map(r => <option key={r.id} value={r.id}>{r.name}</option>)}
        </select>
      </label>

      <label className="flex flex-col gap-1 text-sm">
        <span className="text-muted-foreground">Branch</span>
        <input
          aria-label="Branch"
          type="text"
          value={branch}
          onChange={e => setBranch(e.target.value)}
          placeholder="feature/my-feature"
          className="rounded-md border border-border bg-background px-3 py-2 text-foreground placeholder:text-muted-foreground"
        />
      </label>

      <label className="flex flex-col gap-1 text-sm">
        <span className="text-muted-foreground">Workflow</span>
        <select
          aria-label="Workflow"
          value={flowName}
          onChange={e => setFlowName(e.target.value)}
          className="rounded-md border border-border bg-background px-3 py-2 text-foreground"
        >
          {flows.map(f => <option key={f.name} value={f.name}>{f.description}</option>)}
        </select>
      </label>

      <Button type="submit" disabled={!branch.trim() || loading}>
        {loading ? 'Creating…' : 'Create workspace'}
      </Button>
    </form>
  )
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run src/__tests__/components/workspace/WorkspaceCreationForm.test.tsx`
Expected: PASS (3 tests)

- [ ] **Step 5: Create the wizard route**

```tsx
// web/src/routes/workspaces/new.tsx
import { useState } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { flowsQueryOptions } from '@/lib/queries'
import { postWorkspace } from '@/lib/api'
import { WorkspaceCreationForm } from '@/components/workspace/WorkspaceCreationForm'

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

  const handleSubmit = async (data: { repoId: string; branch: string; flowName: string }) => {
    setLoading(true)
    const ws = await postWorkspace(data.repoId, data.branch, data.flowName)
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

- [ ] **Step 6: Regenerate route tree and verify TypeScript**

Run: `cd web && npx tsr generate && npx tsc --noEmit`
Expected: route tree updated, no TS errors

- [ ] **Step 7: Verify wizard works end-to-end**

In the running dev server, click "New workspace" in the sidebar.
Expected: navigates to `/#/workspaces/new`, shows the creation form.
Fill in a branch name and click Create — should navigate to the new workspace at its first step.

- [ ] **Step 8: Run full test suite**

Run: `cd web && npx vitest run`
Expected: all tests pass

- [ ] **Step 9: Commit**

```bash
git add web/src/components/workspace/ web/src/__tests__/components/workspace/ web/src/routes/workspaces/new.tsx web/src/routeTree.gen.ts
git commit -m "feat: new workspace creation wizard with repo, branch, and workflow selection"
```

---

## Self-Review

**Spec coverage:**
- ✅ Hash history so Tauri + browser both work without a catch-all (Task 1)
- ✅ Vite port pinned to 5173 to match `tauri.conf.json` devUrl (Task 1)
- ✅ `ChatMessage` and workspace types in shared module (Task 2)
- ✅ `apiFetch` uses `window.__CROWBAR__` in Tauri, `VITE_API_URL` in web (Task 3)
- ✅ Mock data modules — each fetch function is a one-line swap for real API (Task 3)
- ✅ Query factories using `queryOptions` pattern (Task 3)
- ✅ `WorkspaceStepTabs` driven by `flow.states` — no hardcoded step names (Task 4)
- ✅ `ChatView` is pure messages + input, tabs removed (Task 5)
- ✅ `DiffView` placeholder renders for `diff` UI mode states (Task 6)
- ✅ Root layout holds AppShell + Sidebar; active state derived from URL (Task 7)
- ✅ `/workspaces/:wsId` redirects to `currentState` before any render (Task 8)
- ✅ `/workspaces/:wsId/:step` renders `ChatView` or `DiffView` based on `state.ui` (Task 8)
- ✅ Switching workspaces changes conversation context (per-wsId:step `useState` init) (Task 8)
- ✅ Mock AI reply fires after 1.5s so streaming state can be prepared (Task 8)
- ✅ `/` redirects to ws3 (Task 8)
- ✅ Kanban discarded — `implementation` uses `chat` UI (Task 3, `FEATURE_DEV_FLOW`)
- ✅ `/chat/:chatId` standalone chat with no flow tabs (Task 9)
- ✅ `/workspaces/new` creation wizard with repo / branch / workflow selection (Task 10)

**Placeholder scan:** None found. All steps contain complete code.

**Type consistency:**
- `ChatMessage` defined in `types.ts` Task 2 — imported by `api.ts`, `mock/conversations.ts`, `ChatView.tsx`, route files ✓
- `FlowStateDefinition` defined in `types.ts` Task 2 — used in `WorkspaceStepTabs`, `mock/flows.ts` ✓
- `WorkspacePayload` defined in `types.ts` Task 2 — used in `api.ts`, `mock/workspaces.ts`, route files ✓
- `getMockConversation(wsId, step)` defined in `mock/conversations.ts` Task 3 — called in `$step.tsx` Task 8 ✓
- `getMockWorkspace(wsId)` defined in `mock/workspaces.ts` Task 3 — called in `$wsId/index.tsx` Task 8 ✓
- `postWorkspace(repoId, branch, flowName)` defined in `api.ts` Task 3 — called in `new.tsx` Task 10 ✓
- `workspaceQueryOptions(wsId)` defined in `queries.ts` Task 3 — called in `$wsId.tsx` and `$step.tsx` Task 8 ✓
- `flowsQueryOptions()` defined in `queries.ts` Task 3 — called in `new.tsx` Task 10 ✓
