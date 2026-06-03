# MSW Migration — Make Every Data Path Go Through the Network Layer

**Date:** 2026-06-01  
**Branch:** `enhancement/design-language`  
**Status:** Ready for implementation

---

## Problem

Multiple components bypass MSW entirely by importing directly from `lib/mock/*`. This means swapping MSW for a real backend won't work cleanly — those components will keep showing hardcoded data regardless of what the server returns. Additionally, the flow/workflow feature is being scrapped and all references must be removed.

---

## Storage Architecture (reference)

Understanding the three layers is required before touching any of this code:

```
Backend / MSW
    ↓  fetched once on startup or user action
React Query (in-memory)
    ↓  auto-persisted to IDB `query-cache` via PersistQueryClientProvider
IndexedDB (crowbar v4)
    ├── query-cache          ← React Query's serialized responses
    ├── branch-review        ← threads, description, merge strategy, active tab
    ├── workspace-layout     ← pane positions, buffer list
    ├── editor-state         ← cursor, scroll, folds per buffer
    ├── sidebar-ui           ← repo/workspace collapse state
    └── workspace-hierarchy  ← workspace parent relationships
    ↑  read by hydrate.ts on startup
Zustand (in-memory, always fast)
    ↓  auto-written back to IDB via store subscriptions
Components (always read from Zustand, never block on I/O)
```

**Direction of travel is one-way.** Data flows down from the server, sideways from Zustand into IDB. Nothing ever reconciles two competing sources.

**Two types of IDB data:**
- `query-cache` — server responses (read-only from the app's perspective)
- Direct stores (`branch-review`, `sidebar-ui`, etc.) — user's local mutations

These never conflict because they store different things.

---

## Startup Ordering Contract

**This order is mandatory.** Getting it wrong silently breaks the sidebar tree.

```
1. apiFetch /api/v0/workspaces  →  useSidebarStore.setRepos(data)
2. apiFetch /api/v0/projects    →  useProjectStore.setProjects(data)
3. hydrateSidebar()             →  overlays collapse state + hierarchy onto repos
4. hydrateWorkspace(wsId)       →  overlays layout + branch review from IDB
```

`hydrateSidebar()` maps over `s.repos` to apply hierarchy. If it runs before `setRepos()`, it operates on `[]` and the workspace tree is flat. Enforce this order explicitly in the boot sequence — not via `Promise.all`.

---

## Cold-Start Guard

Branch review data (threads, description) may exist in IDB from a previous session. The API must only seed Zustand when IDB has no data:

```ts
// Correct — preserves user's local work across reloads
const { branchReview } = store.getState()
if (branchReview.threads.length === 0) {
  const threads = await queryClient.fetchQuery(branchThreadsQueryOptions(wsId))
  store.getState().setInitialThreads(threads)
}
if (!branchReview.description) {
  const description = await queryClient.fetchQuery(branchDescriptionQueryOptions(wsId))
  store.getState().setBranchReviewDescription(description)
}
```

The diff is always fetched fresh — it is never stored in IDB, so no guard is needed.

---

## What Gets Removed — Flow Feature

The flow/workflow feature is scrapped. Full removal:

**Delete entirely:**
- `features/workflow/` (entire directory — `flow-content.tsx`, `split-view.tsx`, `types/workflow.ts`)
- `features/workspace/stores/slices/workflow-slice.ts`
- `features/workspace/stores/hooks/use-workflow.ts`
- `lib/mock/flows.ts`
- `mocks/handlers/flows.ts`
- `components/layout/WorkspaceStepTabs.tsx`
- `features/workspace/components/WorkspaceStepFooter.tsx`

**Modify to remove flow references:**
- `lib/api.ts` — remove `fetchFlows()`
- `lib/queries.ts` — remove `flowsQueryOptions()`
- `lib/types.ts` — remove `FlowDefinition`, `FlowState` types
- `lib/mock/workspaces.ts` — remove `FEATURE_DEV_FLOW`, remove `flow` field from `WorkspacePayload`
- `mocks/handlers/index.ts` — remove `flowHandlers`
- `mocks/handlers/workspaces.ts` — remove `flow` from workspace response shape
- `features/workspace/stores/workspace-store.ts` — remove `createWorkflowSlice`
- `features/workspace/stores/workspace-store.types.ts` — remove `WorkflowSlice`
- `features/workspace/stores/hooks/use-workspace-effects.ts` — remove flow seeding `useEffect`
- `components/workspace/WorkspaceCreationForm.tsx` — remove `flowName` field
- `routes/workspaces/$wsId/$step.tsx` — remove step-based routing
- `lib/mock/workspaces.ts` — remove `flowName`, `currentState`, `flow` from `WorkspacePayload`

---

## Layer 1 — New MSW Handlers

### `mocks/handlers/branch-review.ts` (new file)

```
GET /api/v0/branch-review/:wsId/diff         → getMockBranchDiff(wsId)
GET /api/v0/branch-review/:wsId/threads      → getMockBranchReviewThreads(wsId)
GET /api/v0/branch-review/:wsId/description  → getMockBranchReviewDescription(wsId)
GET /api/v0/branch-review/:wsId/chats        → getMockBranchReviewChats(wsId)
```

### `mocks/handlers/fs.ts` (extend)

```
GET /api/v0/fs/file?path=<path>  → getMockFileContent(path), returned as JSON string
```

### `mocks/handlers/index.ts` (extend)

Register `branchReviewHandlers` alongside existing handlers.

---

## Layer 2 — Query Options Split

### `lib/queries.ts` (add 3, keep existing 7)

```ts
workspacesQueryOptions()          // GET /api/v0/workspaces → Repo[]
projectsQueryOptions()            // GET /api/v0/projects → Project[]
fileContentQueryOptions(path)     // GET /api/v0/fs/file?path=<path> → string
```

Used imperatively via `queryClient.fetchQuery()` — not `useQuery` — because file content is loaded on user action, not on component mount.

### `features/branch-review/queries.ts` (new file, 4 options)

```ts
branchDiffQueryOptions(wsId)         // GET /api/v0/branch-review/:wsId/diff
branchThreadsQueryOptions(wsId)      // GET /api/v0/branch-review/:wsId/threads
branchDescriptionQueryOptions(wsId)  // GET /api/v0/branch-review/:wsId/description
branchChatsQueryOptions(wsId)        // GET /api/v0/branch-review/:wsId/chats
```

### `features/markdown-chat/queries.ts` (new file, 1 option)

```ts
markdownChatQueryOptions(wsId, stepId)  // GET /api/v0/markdown-chat/:wsId/:stepId
```

**Rule:** queries consumed by exactly one feature live in that feature. Queries shared across features live in `lib/queries.ts`.

---

## Layer 3 — Store Fixes

### `lib/store/sidebar.ts`

- Remove `INITIAL_REPOS` constant entirely
- Change initial state to `repos: []`
- `setRepos` action already exists — no change needed

### `lib/store/projects.ts`

- Remove `getAllMockProjects()` from `getInitialState()`
- Change initial state to `projects: []`
- Add `setProjects(projects: Project[])` action alongside existing `addProject`
- Remove `getAllMockProjects` import

### `lib/store/conversations.ts`

- Remove `getMockConversation` import
- Cold-start seeding goes through `GET /api/v0/conversations/:wsId/:step` (handler already exists)

---

## Layer 4 — Component Fixes

### `features/workspace/stores/hooks/use-workspace-effects.ts`

Replace both direct mock calls:

```ts
// Before
const files = getMockFileTree(repoPath)
useFileSystemStore.setState({ files, ... })

const content = getMockFileContent(path)

// After
const files = await queryClient.fetchQuery(fileTreeQueryOptions(repoPath))
useFileSystemStore.setState({ files, ... })

const content = await queryClient.fetchQuery(fileContentQueryOptions(path))
```

Remove flow-seeding `useEffect` entirely (flow feature removed).

### `features/branch-review/components/branch-review-pane.tsx`

Replace the `useEffect` that seeds from mock:

```ts
// Before
setBranchReviewDiff(getMockBranchDiff(wsId))
if (branchReview.threads.length === 0) {
  getMockBranchReviewThreads(wsId).forEach(t => addReviewThread(t))
}
if (!branchReview.description) {
  setBranchReviewDescription(getMockBranchReviewDescription(wsId))
}

// After — diff always from API, threads/description cold-start guarded
const { data: diff } = useQuery(branchDiffQueryOptions(wsId))
useEffect(() => {
  if (diff) setBranchReviewDiff(diff)
}, [diff])

// threads and description respect the cold-start guard (see above)
```

### `features/branch-review/components/about-tab.tsx`

```ts
// Before
const chats = getMockBranchReviewChats(wsId)

// After
const { data: chats = [] } = useQuery(branchChatsQueryOptions(wsId))
```

### `features/branch-review/components/commits-tab.tsx`

```ts
// Before
const commits = getMockCommitHistory(repoPath)

// After
const { data: commits = [], isLoading } = useQuery(gitHistoryQueryOptions(repoPath))
```

### `features/markdown-chat/components/markdown-chat-view.tsx`

**Initial turns (cold-start):**

```ts
// Before
getMockMarkdownTurns(workspaceId, stepId).forEach(t => state.appendTurn(t))

// After
const { data: initialTurns } = useQuery(markdownChatQueryOptions(workspaceId, stepId))
useEffect(() => {
  if (!initialTurns || store.getState().turns.length > 0) return
  initialTurns.forEach(t => store.getState().appendTurn(t))
}, [initialTurns])
```

**Streaming (send message):**

Replace `simulateMarkdownStream` with the singleton `wsManager` from `lib/ws/manager.ts`.

`WSManager` uses a shared channel model: `subscribe(endpoint, cb)` opens the socket if not already open and returns an unsubscribe function. `send(endpoint, data)` writes to the open socket.

```ts
// Before
cancelStreamRef.current = simulateMarkdownStream(
  MOCK_RESPONSE,
  chunk => state.updateStreamingTurn(agentId, chunk),
  () => { state.finalizeStreamingTurn(agentId); setIsStreaming(false) },
)

// After — wsManager is the singleton from lib/ws/manager.ts
const endpoint = `/api/v0/ws/chat/${workspaceId}`
const unsubscribe = wsManager.subscribe(endpoint, (msg: unknown) => {
  const m = msg as { content: string; done: boolean }
  if (!m.done) state.updateStreamingTurn(agentId, m.content)
  else { state.finalizeStreamingTurn(agentId); setIsStreaming(false); unsubscribe() }
})
wsManager.send(endpoint, { turnId: agentId, message: content })
cancelStreamRef.current = unsubscribe
```

Remove `MOCK_RESPONSE` constant entirely.

### `components/layout/IDEShell.tsx`

Replace raw `fetch('/api/v0/workspaces')` on line 42 with `useQuery(workspacesQueryOptions())` and sync to store via effect.

---

## Layer 5 — Startup Sequence

Wherever the app currently calls `hydrateSidebar()`, add the API fetches before it. The exact component depends on where the boot sequence is triggered (likely a layout route or app shell). The implementation task must locate this call site and enforce the ordering contract defined above.

---

## Layer 6 — Streaming WebSocket

**Current handler behaviour (`mocks/handlers/ws/chat.ts`):** connects and immediately starts streaming a hardcoded word list — it does not wait for a client message before streaming. This must be updated to be message-triggered so it behaves like a real AI backend.

**Updated handler protocol:**
- Connection: `ws.link('/api/v0/ws/chat/:chatId')`
- On `connection`: wait for a message event before streaming
- Inbound from client: `{ turnId: string, message: string }`
- Outbound to client: `{ content: string, done: false }` per chunk, then `{ content: '', done: true }`

Update the handler to listen for `client.addEventListener('message', ...)` and only start the streaming interval after receiving the first message. This makes the mock match real backend behaviour.

**`wsManager` API (singleton from `lib/ws/manager.ts`):**
- `wsManager.subscribe(endpoint, cb)` — opens socket if not open, registers callback, returns unsubscribe fn
- `wsManager.send(endpoint, data)` — sends JSON to the open socket
- Unsubscribing the last callback automatically closes the socket

---

## Files Touched Summary

| File | Action |
|------|--------|
| `mocks/handlers/branch-review.ts` | Create |
| `features/branch-review/queries.ts` | Create |
| `features/markdown-chat/queries.ts` | Create |
| `features/workflow/` (entire dir) | Delete |
| `features/workspace/stores/slices/workflow-slice.ts` | Delete |
| `features/workspace/stores/hooks/use-workflow.ts` | Delete |
| `components/layout/WorkspaceStepTabs.tsx` | Delete |
| `features/workspace/components/WorkspaceStepFooter.tsx` | Delete |
| `lib/mock/flows.ts` | Delete |
| `mocks/handlers/flows.ts` | Delete |
| `lib/queries.ts` | Add 3 query options, remove `flowsQueryOptions` |
| `lib/api.ts` | Remove `fetchFlows` |
| `lib/types.ts` | Remove flow types |
| `lib/store/sidebar.ts` | Remove `INITIAL_REPOS`, `repos: []` |
| `lib/store/projects.ts` | Remove mock seed, add `setProjects` |
| `lib/store/conversations.ts` | Remove direct mock import |
| `lib/mock/workspaces.ts` | Remove `FEATURE_DEV_FLOW`, flow fields |
| `mocks/handlers/fs.ts` | Add file content endpoint |
| `mocks/handlers/index.ts` | Register branch-review, remove flows |
| `mocks/handlers/workspaces.ts` | Remove flow fields from response |
| `features/workspace/stores/workspace-store.ts` | Remove `createWorkflowSlice` |
| `features/workspace/stores/workspace-store.types.ts` | Remove `WorkflowSlice` |
| `features/workspace/stores/hooks/use-workspace-effects.ts` | Replace mock calls, remove flow effect |
| `features/workspace/components/WorkspaceCreationForm.tsx` | Remove `flowName` |
| `features/branch-review/components/branch-review-pane.tsx` | Replace mock calls with queries |
| `features/branch-review/components/about-tab.tsx` | Replace mock call with query |
| `features/branch-review/components/commits-tab.tsx` | Replace mock call with query |
| `features/markdown-chat/components/markdown-chat-view.tsx` | Replace mock calls, wire WSManager |
| `components/layout/IDEShell.tsx` | Replace raw fetch with useQuery |
| `routes/workspaces/$wsId/$step.tsx` | Remove step routing |
| Boot sequence (locate during impl) | Enforce startup ordering contract |

---

## Success Criteria

1. `VITE_USE_MOCK=true` — app works identically to today
2. Every component that previously imported from `lib/mock/*` no longer does so (except the MSW handlers themselves, which are allowed)
3. No file outside `mocks/` or `lib/mock/` imports from `lib/mock/`
4. Disabling MSW (`VITE_USE_MOCK=false`) shows empty states or loading spinners — never hardcoded mock data
5. All flow/workflow references are gone — TypeScript compiler finds zero references to `FlowDefinition`, `flowsQueryOptions`, `fetchFlows`
6. Startup ordering: sidebar tree shows correct parent/child relationships on first load
7. Branch review: switching workspaces after editing a thread description preserves edits across page reload
