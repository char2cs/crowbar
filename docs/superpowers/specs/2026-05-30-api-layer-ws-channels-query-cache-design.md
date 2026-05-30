# Crowbar API Layer, WebSocket Channels & Query Cache Persistence

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Connect Crowbar's web UI to a mock-stubbed Go backend with typed REST and per-resource WebSocket channels, replace `IS_MOCK` branching with MSW so query functions contain no mock awareness, and persist the TanStack Query cache to IDB for instant startup.

**Architecture:** The Go server exposes REST endpoints backed by embedded JSON fixture files and a chaos middleware for per-request latency/error injection. One WebSocket endpoint exists per resource type; each carries exactly one object shape. MSW intercepts all HTTP and WS traffic when `VITE_USE_MOCK=true`, so application query code has zero mock awareness. A floating dev chaos panel sends `X-Crowbar-Latency` and `X-Crowbar-Error-Rate` headers on every `apiFetch` call for stress testing. `PersistQueryClientProvider` reads the TanStack Query cache from IDB on startup, giving instant data before WS channels deliver fresh values. Cache buster is the git commit SHA injected at Vite build time.

**Tech Stack:** Go 1.25 + Gin, `gorilla/websocket`, `//go:embed` fixtures; MSW v2, `@tanstack/react-query-persist-client`, `@tanstack/query-async-storage-persister`; Vite `define` for git SHA injection.

---

## File Map

### Go (`api/`)

| Action | Path | Responsibility |
|---|---|---|
| Create | `api/internal/fixtures/workspaces.json` | 20 workspaces, 3 repos, 4-level nesting |
| Create | `api/internal/fixtures/flows.json` | 5 flow types |
| Create | `api/internal/fixtures/conversations.json` | 500-message thread with tool calls |
| Create | `api/internal/fixtures/projects.json` | 10 projects |
| Create | `api/internal/fixtures/file-tree.json` | 5 000-node tree, depth 12 |
| Create | `api/internal/fixtures/git-log.json` | 2 000 commits, merge commits |
| Create | `api/internal/fixtures/git-branches.json` | 50 branches, 30 stale |
| Create | `api/internal/fixtures/git-status.json` | 200 changed files, staged/unstaged/conflicts |
| Create | `api/internal/fixtures/loader.go` | `//go:embed *.json` + unmarshal into `Store` struct |
| Modify | `api/internal/api/v0/router.go` | register all new REST + WS handlers |
| Create | `api/internal/api/v0/workspaces_handler.go` | `GET /workspaces/:id`, `POST /workspaces` |
| Create | `api/internal/api/v0/flows_handler.go` | `GET /flows` |
| Create | `api/internal/api/v0/conversations_handler.go` | `GET /conversations/:wsId/:step` |
| Create | `api/internal/api/v0/projects_handler.go` | `GET /projects`, `POST /projects` |
| Create | `api/internal/api/v0/fs_handler.go` | `GET /fs/tree?root=` |
| Create | `api/internal/api/v0/git_handler.go` | `GET /git/status`, `/git/log`, `/git/branches` |
| Create | `api/internal/api/v0/terminal_handler.go` | `POST /terminal/sessions` + WS upgrade |
| Create | `api/internal/api/v0/ws_workspaces_handler.go` | WS `/ws/workspaces` |
| Create | `api/internal/api/v0/ws_git_handler.go` | WS `/ws/git?repo=` |
| Create | `api/internal/api/v0/ws_files_handler.go` | WS `/ws/files?root=` |
| Create | `api/internal/api/v0/ws_chat_handler.go` | WS `/ws/chat/:chatId` |
| Create | `api/internal/api/v0/ws_daemon_handler.go` | WS `/ws/daemon` |
| Create | `api/internal/api/middleware/chaos.go` | reads `X-Crowbar-Latency` + `X-Crowbar-Error-Rate` |

### Web (`web/src/`)

| Action | Path | Responsibility |
|---|---|---|
| Modify | `web/vite.config.ts` | inject `__APP_VERSION__` from git SHA |
| Modify | `web/src/vite-env.d.ts` | `declare const __APP_VERSION__: string` |
| Modify | `web/src/lib/api.ts` | add `apiFetch`, remove all `IS_MOCK` branches |
| Modify | `web/src/lib/queries.ts` | queryFns always call `apiFetch`, no mock imports |
| Create | `web/src/lib/ws/types.ts` | per-channel TypeScript message types |
| Create | `web/src/lib/ws/manager.ts` | ref-counted singleton WS manager |
| Modify | `web/src/lib/persistence/schemas.ts` | add `query-cache` store to `CrowbarDB` |
| Modify | `web/src/lib/persistence/idb.ts` | add `query-cache` object store in v1 block |
| Create | `web/src/lib/persistence/query-persister.ts` | IDB `AsyncStorage` + `createAsyncStoragePersister` |
| Create | `web/src/lib/store/chaos.ts` | Zustand store for chaos panel state |
| Create | `web/src/mocks/browser.ts` | MSW service worker registration |
| Create | `web/src/mocks/handlers/workspaces.ts` | `http.get` / `http.post` MSW handlers |
| Create | `web/src/mocks/handlers/flows.ts` | |
| Create | `web/src/mocks/handlers/conversations.ts` | |
| Create | `web/src/mocks/handlers/projects.ts` | |
| Create | `web/src/mocks/handlers/git.ts` | |
| Create | `web/src/mocks/handlers/fs.ts` | |
| Create | `web/src/mocks/handlers/ws/git.ts` | MSW `ws` handler — pushes mock events |
| Create | `web/src/mocks/handlers/ws/chat.ts` | MSW `ws` handler — simulates token stream |
| Create | `web/src/mocks/handlers/ws/terminal.ts` | MSW `ws` handler — echoes input |
| Create | `web/src/mocks/handlers/index.ts` | re-exports all handlers as flat array |
| Create | `web/src/components/dev/chaos-panel.tsx` | floating overlay, only in dev + non-mock mode |
| Modify | `web/src/main.tsx` | `PersistQueryClientProvider`, MSW start, `ChaosPanel` |

---

## 1. Go REST Endpoints

### Fixture Store (`api/internal/fixtures/loader.go`)

JSON files and the loader live in the same package so `//go:embed` paths stay simple (Go does not allow `..` in embed paths). Mutations (POST) write to in-memory maps seeded from fixture data; state resets on server restart.

```go
package fixtures

import (
    "encoding/json"
    _ "embed"
    "sync"
)

//go:embed workspaces.json
var workspacesJSON []byte

//go:embed flows.json
var flowsJSON []byte

// ... one //go:embed per file

type Store struct {
    // Read-only after Load()
    Flows         []FlowDefinition
    FileTree      FileNode
    GitLog        []Commit
    GitBranches   []Branch
    GitStatus     GitStatus
    Conversations map[string][]ChatMessage

    // Mutable (POST handlers write here)
    mu         sync.RWMutex
    Workspaces map[string]Workspace // key: workspaceId
    Projects   []Project
}

func Load() (*Store, error) {
    s := &Store{}

    var wsSlice []Workspace
    if err := json.Unmarshal(workspacesJSON, &wsSlice); err != nil {
        return nil, err
    }
    s.Workspaces = make(map[string]Workspace, len(wsSlice))
    for _, w := range wsSlice {
        s.Workspaces[w.ID] = w
    }

    if err := json.Unmarshal(flowsJSON, &s.Flows); err != nil {
        return nil, err
    }
    // ... unmarshal remaining fixtures
    return s, nil
}
```

The `Store` is created once in `api/internal/api/container.go` and injected into all handlers.

### REST Endpoints

| Method | Path | Response type |
|---|---|---|
| `GET` | `/api/v0/workspaces/:id` | `WorkspacePayload` |
| `POST` | `/api/v0/workspaces` | `WorkspacePayload` |
| `GET` | `/api/v0/flows` | `[]FlowDefinition` |
| `GET` | `/api/v0/conversations/:wsId/:step` | `{ messages: []ChatMessage }` |
| `GET` | `/api/v0/projects` | `[]Project` |
| `POST` | `/api/v0/projects` | `Project` |
| `GET` | `/api/v0/fs/tree?root=<path>` | `FileNode` |
| `GET` | `/api/v0/git/status?repo=<path>` | `GitStatus` |
| `GET` | `/api/v0/git/log?repo=<path>` | `[]Commit` |
| `GET` | `/api/v0/git/branches?repo=<path>` | `[]Branch` |
| `POST` | `/api/v0/terminal/sessions` | `{ sessionId: string }` |

All handlers: read from `Store` with `RLock`; POST handlers acquire `Lock` and write to mutable fields.

---

## 2. WebSocket Channels

One Go handler per channel. Each carries exactly one object type. No shared multiplexer.

| Endpoint | Direction | Object type |
|---|---|---|
| `GET /api/v0/ws/workspaces` | server→client | `WorkspaceEvent` |
| `GET /api/v0/ws/git?repo=<path>` | server→client | `GitEvent` |
| `GET /api/v0/ws/files?root=<path>` | server→client | `FileEvent` |
| `GET /api/v0/ws/chat/:chatId` | server→client | `ChatChunk` |
| `GET /api/v0/ws/terminal/:sessionId` | bidirectional | `TerminalFrame` |
| `GET /api/v0/ws/daemon` | server→client | `DaemonStatus` |

**Go:** Each handler upgrades HTTP to WS using `gorilla/websocket`. Each has its own Hub (goroutine + broadcast channel). No cross-channel state.

**Terminal session lifecycle:**
1. Component calls `POST /api/v0/terminal/sessions` → receives `{ sessionId: "uuid" }`
2. Component opens WS to `/api/v0/ws/terminal/:sessionId`
3. Server matches session ID, begins PTY bridge
4. On WS close, server cleans up session

### Frontend WS Manager (`web/src/lib/ws/manager.ts`)

Singleton. Ref-counted map of open `WebSocket` instances keyed by full endpoint URL.

```ts
interface WSManager {
  subscribe(endpoint: string, onMessage: (data: unknown) => void): () => void
  send(endpoint: string, data: unknown): void
}
```

Reconnect: exponential backoff per channel — 1s, 2s, 4s, 8s, max 30s. On successful reconnect, the manager re-fires all active `onMessage` callbacks with a synthetic `{ reconnected: true }` payload. Each subscriber is responsible for deciding what to do — typically invalidating its own query key. The manager has no knowledge of TanStack Query.

**Query hooks** subscribe in `useEffect`:
```ts
useEffect(() => {
  return wsManager.subscribe(
    `/api/v0/ws/git?repo=${encodeURIComponent(repoPath)}`,
    () => queryClient.invalidateQueries({ queryKey: queryKeys.git.status(repoPath) })
  )
}, [repoPath])
```

`window.__CROWBAR__.on/emit` is unchanged — it handles OS-level bridge events from the native shell, not API channels.

---

## 3. Chaos Middleware (`api/internal/api/middleware/chaos.go`)

Applied to all `/api/v0/*` routes. Both headers are optional; absent headers are no-ops.

```go
func Chaos() gin.HandlerFunc {
    return func(c *gin.Context) {
        if d := c.GetHeader("X-Crowbar-Latency"); d != "" {
            if ms, err := strconv.Atoi(d); err == nil && ms > 0 {
                time.Sleep(time.Duration(ms) * time.Millisecond)
            }
        }
        if r := c.GetHeader("X-Crowbar-Error-Rate"); r != "" {
            if rate, err := strconv.ParseFloat(r, 64); err == nil && rand.Float64() < rate {
                c.AbortWithStatusJSON(500, gin.H{"error": "chaos injection"})
                return
            }
        }
        c.Next()
    }
}
```

---

## 4. MSW Mock Layer

Install as devDependency: `msw@^2`. Run once: `npx msw init public/ --save`.

### Handler structure

Each handler file imports from `web/src/lib/mock/` (existing mock data stays in place):

```ts
// web/src/mocks/handlers/workspaces.ts
import { http, HttpResponse } from 'msw'
import { getMockWorkspace, createMockWorkspace } from '@/lib/mock/workspaces'

export const workspaceHandlers = [
  http.get('/api/v0/workspaces/:id', ({ params }) =>
    HttpResponse.json(getMockWorkspace(params.id as string))
  ),
  http.post('/api/v0/workspaces', async ({ request }) => {
    const body = await request.json() as { repoId: string; branch: string; flowName: string }
    return HttpResponse.json(createMockWorkspace(body.repoId, body.branch, body.flowName))
  }),
]
```

WS handlers simulate live data:
```ts
// web/src/mocks/handlers/ws/git.ts
import { ws } from 'msw'

export const gitWsHandler = ws.link('/api/v0/ws/git').addEventListener('connection', ({ client }) => {
  const interval = setInterval(
    () => client.send(JSON.stringify({ repo: 'mock', changed: true })),
    5000,
  )
  client.addEventListener('close', () => clearInterval(interval))
})
```

### Activation in `main.tsx`

```ts
if (import.meta.env.VITE_USE_MOCK === 'true') {
  const { worker } = await import('./mocks/browser')
  await worker.start({ onUnhandledRequest: 'warn' })
}
```

After this change, `api.ts` contains no `IS_MOCK` checks. `queries.ts` imports no mock functions. Query functions are always:
```ts
queryFn: () => apiFetch(`/api/v0/fs/tree?root=${encodeURIComponent(rootPath)}`)
```

---

## 5. TanStack Query Cache Persistence

### New npm packages
```
@tanstack/react-query-persist-client
@tanstack/query-async-storage-persister
```

### IDB changes (no schema version bump — zero existing users)

`web/src/lib/persistence/schemas.ts` — add to `CrowbarDB`:
```ts
'query-cache': {
  key: string
  value: string
}
```

`web/src/lib/persistence/idb.ts` — inside the existing `if (oldVersion < 1)` block:
```ts
db.createObjectStore('query-cache')
```

### Persister (`web/src/lib/persistence/query-persister.ts`)

```ts
import { createAsyncStoragePersister } from '@tanstack/query-async-storage-persister'
import { getDB } from './idb'

const idbAsyncStorage = {
  getItem: async (key: string): Promise<string | null> =>
    (await getDB()).get('query-cache', key) ?? null,
  setItem: async (key: string, value: string): Promise<void> => {
    await (await getDB()).put('query-cache', value, key)
  },
  removeItem: async (key: string): Promise<void> => {
    await (await getDB()).delete('query-cache', key)
  },
}

export const persister = createAsyncStoragePersister({
  storage: idbAsyncStorage,
  key: 'crowbar-query-cache',
})
```

### `web/vite.config.ts` — inject git SHA

```ts
import { execSync } from 'child_process'

const gitSHA = (() => {
  try { return execSync('git rev-parse --short HEAD').toString().trim() }
  catch { return 'dev' }
})()

// Inside defineConfig:
define: { __APP_VERSION__: JSON.stringify(gitSHA) },
```

`web/src/vite-env.d.ts`: add `declare const __APP_VERSION__: string`

### `web/src/main.tsx` — swap provider

```tsx
import { PersistQueryClientProvider } from '@tanstack/react-query-persist-client'
import { persister } from '@/lib/persistence/query-persister'

<PersistQueryClientProvider
  client={queryClient}
  persistOptions={{
    persister,
    maxAge: 7 * 24 * 60 * 60 * 1000,
    buster: __APP_VERSION__,
  }}
>
  {/* existing app tree */}
</PersistQueryClientProvider>
```

### Startup sequence with persistence

1. `PersistQueryClientProvider` mounts → restores cache from IDB (~1ms)
2. React renders immediately with last-known data (no loading states for already-seen data)
3. WS channels connect → server events invalidate stale keys
4. Fresh data loads in background and replaces stale values

---

## 6. Chaos Dev Panel (`web/src/components/dev/chaos-panel.tsx`)

Rendered only when `import.meta.env.DEV && import.meta.env.VITE_USE_MOCK !== 'true'` (i.e. dev mode pointing at the real Go server).

Fixed-position overlay, bottom-right corner. Contains:
- **Latency slider:** 0ms–5 000ms → stored in `useChaosStore`
- **Error rate input:** 0.0–1.0 → stored in `useChaosStore`
- **Reset button** → clears both to 0

`apiFetch` reads `useChaosStore.getState()` and appends headers when non-zero:
```ts
const { latency, errorRate } = useChaosStore.getState()
const headers: HeadersInit = {}
if (latency > 0) headers['X-Crowbar-Latency'] = String(latency)
if (errorRate > 0) headers['X-Crowbar-Error-Rate'] = String(errorRate)
```

`useChaosStore` (`web/src/lib/store/chaos.ts`) is a minimal Zustand store: `{ latency: number; errorRate: number; set: (patch) => void }`.

---

## 7. Stress Test Fixture Data

Fixture files are designed to hit real UI performance limits:

| File | Stress factor |
|---|---|
| `workspaces.json` | 20 workspaces, 4 levels deep, all status types represented |
| `file-tree.json` | 5 000 nodes, max depth 12, mix of large + empty files |
| `git-log.json` | 2 000 commits, merge commits, 500-char messages, binary files |
| `git-branches.json` | 50 branches: 20 active, 30 stale (>90 days), remote-only variants |
| `git-status.json` | 200 changed files: staged, unstaged, untracked, conflicts, renames |
| `conversations.json` | 500 messages, 50 with tool calls, 10 with image content |
