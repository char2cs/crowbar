# Crowbar Data Layer Design

**Date:** 2026-05-29  
**Status:** Approved  
**Scope:** Full data layer — transport, persistence, query patterns, render/store cleanup

---

## Context

Crowbar is a desktop IDE built as a Tauri 2 application. The frontend (React) runs in a Tauri webview with zero Tauri-specific imports. A Go daemon runs as a sidecar process, exposing all IDE capabilities (workspaces, chats, git, files, terminal PTY) over a Unix socket at `~/.crowbar/crowbar.sock`. A Rust layer embedded in Tauri bridges the webview to the daemon without opening any network port.

The backend is local-only today and will become a remote cloud server in the future. The frontend must not change when that transition happens — only config changes.

---

## Architecture Overview

```
React (Tauri webview) — pure web code, zero Tauri imports
  ↓ fetch(`${__CROWBAR__.endpoint}/...`)   — request/response, no port
  ↓ fetch(`${__CROWBAR__.endpoint}/...`)   — streaming (crowbar://stream/ locally)
  ↓ window.__CROWBAR__.on/emit            — real-time events, no port

Rust (Tauri) — embedded bridge
  ↓ HTTP over Unix socket (hyperlocal)
  ↓ WS/events over Unix socket (tokio-tungstenite)
  ↓ WS/terminal over Unix socket (binary frames)

Go daemon — sidecar (~/.crowbar/crowbar.sock)
  ↓ disk I/O · git CLI · LSP · PTY
```

**Remote mode (future):** Tauri injects `window.__CROWBAR__.endpoint = 'https://api.crowbar.dev'`. Webview constructs URLs as `` fetch(`${__CROWBAR__.endpoint}/workspaces`) ``. The `window.__CROWBAR__.on/emit` bridge wraps a WSS connection instead of Tauri events. Zero frontend code changes.

---

## Layer 1 — Transport

### The `crowbar://` custom URI scheme

Tauri registers `crowbar://` as a custom URI scheme. The webview makes standard `fetch()` calls; Tauri's protocol handler intercepts them in Rust, makes the call over the Unix socket via `hyperlocal`, and returns the response to the webview. No port opens.

Streaming responses (AI token-by-token, file watcher events) use `crowbar://stream/...` and return a `ReadableStream` via Tauri 2's `ResponseBody::Channel`. The webview reads them as standard `ReadableStream` — no special API.

### `window.__CROWBAR__`

Injected by Tauri before the webview loads. Declaration only — never imported at runtime.

```ts
// lib/transport/types.ts
interface CrowbarBridge {
  mode: 'local' | 'remote'
  endpoint: string  // 'crowbar://api' locally, 'https://api.crowbar.dev' remotely
  on: <T>(event: string, handler: (payload: T) => void) => () => void
  emit: (event: string, payload?: unknown) => void
}

declare global {
  interface Window { __CROWBAR__: CrowbarBridge }
}
```

All fetch calls are constructed as:
```ts
fetch(`${window.__CROWBAR__.endpoint}/workspaces`)
```

Never hardcode `crowbar://` — the endpoint is always read from the bridge.

### Three WebSocket channels over Unix socket

One persistent WS connection per concern. Each runs in its own Rust task with a 500ms auto-reconnect loop.

| Channel | Path | Format | Frequency |
|---|---|---|---|
| `events` | `/ws/events` | JSON | Low — workspace, chat, git events |
| `terminal` | `/ws/terminal` | Binary frames | High — PTY I/O per session |
| `lsp` | `/ws/lsp` | JSON | Medium — diagnostics, completions (future) |

Terminal carries raw PTY bytes. It never shares a channel with JSON domain events.

### Daemon status state machine

Rust emits `daemon:status` events through `window.__CROWBAR__.on`. A Zustand store holds the state; UI components read it to conditionally render.

States: `starting → ready → degraded → reconnecting`

### Rust module structure

```
src-tauri/src/
  protocol/
    handler.rs          # route dispatcher for crowbar://
    routes/
      workspaces.rs
      chats.rs
      git.rs
      files.rs
      stream.rs         # Channel-backed streaming responses
  connection/
    manager.rs          # ConnectionManager (local / remote)
    local/
      sidecar.rs        # spawn Go daemon, health check (25 retries × 200ms)
      http.rs           # hyperlocal HTTP over Unix socket
      ws/
        events.rs       # events channel, auto-reconnect
        terminal.rs     # terminal channel, binary frames
        lsp.rs          # lsp channel (future)
    remote/             # future: HTTPS/WSS
  events/
    bridge.rs           # WS frame → window.emit()
    types.rs            # typed Rust event enum
  inject/
    config.rs           # builds window.__CROWBAR__ JS snippet
  lib.rs                # setup: register protocol, start connection, inject config
```

---

## Layer 2 — Persistence

### What persists

| Data | Store | Key |
|---|---|---|
| Pane layout, split config, active pane | `workspace-layout` | `workspaceId` |
| Tab groups per pane, sidebar widths | `workspace-layout` | `workspaceId` |
| Cursor, scroll, fold state per buffer | `editor-state` | `[workspaceId, bufferId]` |
| Theme, font, editor preferences | `ui-preferences` | `'global'` |

**Never persisted:** git state, file content, chat messages, terminal sessions, command palette state, LSP diagnostics, hover/focus state.

### IDB Schema

```ts
// lib/persistence/schemas.ts

interface WorkspaceLayout {
  workspaceId: string
  panes: PaneConfig[]
  activePane: string
  tabGroups: TabGroup[]
  sidebarWidth: number
  rightSidebarWidth: number
  updatedAt: number         // reserved for future multi-device merge
}

interface EditorState {
  workspaceId: string
  bufferId: string          // file path
  cursorLine: number
  cursorColumn: number
  scrollTop: number
  folds: [number, number][] // [startLine, endLine]
  updatedAt: number
}

interface UIPreferences {
  theme: string
  fontSize: number
  fontFamily: string
  tabSize: number
  wordWrap: boolean
  minimap: boolean
  updatedAt: number
}
```

Database name: `crowbar`, version: `1`. Schema migrations bump `DB_VERSION` and are handled in the `upgrade` callback.

### Startup hydration sequence

React must not mount until IDB hydration completes. All three reads fire in parallel.

```ts
// lib/persistence/hydrate.ts
export async function hydrateFromIDB(workspaceId: string) {
  const [layout, prefs, editorStates] = await Promise.all([
    idb.get('workspace-layout', workspaceId),
    idb.get('ui-preferences', 'global'),
    idb.getAllFromIndex('editor-state', 'workspaceId', workspaceId),
  ])

  if (layout) useWorkspaceStore.setState(layout)
  if (prefs) useSettingsStore.setState(prefs)
  editorStates.forEach(s => useEditorStore(s.bufferId).setState(s))
}
```

**Startup timing targets:**
- IDB open + parallel reads: ~2ms
- First React render: ~8ms (no network dependency)
- Go daemon health check: ~40ms (background)
- Server data arrives: ~80ms (TanStack Query, background)

### Workspace switching

On switch: save current layout to IDB → read new workspace layout from IDB (~1ms) → update Zustand stores → React re-renders. Server data for the new workspace loads after render in background. The switch feels instant.

---

## Layer 3 — Query Patterns

### Data ownership rule

| Data | Owner | When stale |
|---|---|---|
| Workspaces, chats, messages | Go daemon → TanStack Query | `staleTime: Infinity`, invalidated by WS events |
| Git status / branches / log | Go daemon → TanStack Query | `staleTime: 5_000`, also WS-invalidated |
| File tree | Go daemon → TanStack Query | `staleTime: 30_000`, also file-watcher events |
| Tab layout, split config | Zustand + IDB | Persisted immediately on change |
| Editor state | Zustand + IDB | Persisted on debounced change |
| UI preferences | Zustand + IDB | Persisted immediately on change |
| Terminal sessions | Zustand only | Not persisted |

### Query key factory

All query keys live in `lib/queries/keys.ts`. No feature may define its own keys.

```ts
export const queryKeys = {
  workspaces: {
    all: ['workspaces'] as const,
    list: () => [...queryKeys.workspaces.all, 'list'] as const,
    detail: (id: string) => [...queryKeys.workspaces.all, id] as const,
  },
  chats: {
    byWorkspace: (wsId: string) => ['chats', 'workspace', wsId] as const,
    detail: (id: string) => ['chats', id] as const,
    messages: (id: string) => ['chats', id, 'messages'] as const,
  },
  git: {
    status: (wsId: string) => ['git', wsId, 'status'] as const,
    branches: (wsId: string) => ['git', wsId, 'branches'] as const,
    log: (wsId: string) => ['git', wsId, 'log'] as const,
  },
  files: {
    tree: (wsId: string, path: string) => ['files', wsId, 'tree', path] as const,
  },
}
```

### Daemon event → cache invalidation

One function, called once at app startup. This is the only place `staleTime: Infinity` queries become fresh.

```ts
// lib/events/connect.ts
export function connectDaemonEvents(qc: QueryClient) {
  window.__CROWBAR__.on('workspace:updated', () =>
    qc.invalidateQueries({ queryKey: queryKeys.workspaces.all }))

  window.__CROWBAR__.on('chat:message', (p: { chatId: string }) =>
    qc.invalidateQueries({ queryKey: queryKeys.chats.messages(p.chatId) }))

  window.__CROWBAR__.on('git:changed', (p: { workspaceId: string }) =>
    qc.invalidateQueries({ queryKey: queryKeys.git.status(p.workspaceId) }))

  window.__CROWBAR__.on('file:changed', (p: { workspaceId: string; path: string }) =>
    qc.invalidateQueries({ queryKey: queryKeys.files.tree(p.workspaceId, p.path) }))
}
```

### Optimistic mutations

Every mutation uses `useOptimisticMutation`. No exceptions.

```ts
// lib/mutations/optimistic.ts
export function useOptimisticMutation<TData, TVariables>(
  mutationFn: (vars: TVariables) => Promise<TData>,
  options: {
    onMutate: (vars: TVariables) => Promise<{ snapshot: unknown; queryKey: QueryKey }>
    invalidateKey: QueryKey
  }
)
```

Pattern: `onMutate` writes optimistic cache update + saves snapshot → `onError` rolls back via snapshot → `onSettled` invalidates to sync with server.

---

## Layer 4 — Render & Store Rules

### State placement decision tree

1. Will a daemon event change this? → **TanStack Query**
2. Should it survive a page reload? → **Zustand + IDB**
3. Is it local UI state for this session only? → **Zustand only**
4. Is it derived from other state? → **`useMemo` — no store**

### Selector discipline

Every `useXxxStore` call must use a narrow selector. Re-renders only when the specific field changes.

```ts
// Bad — re-renders on any store mutation
const store = useWorkspaceStore()
const { settings, update } = useSettingsStore()

// Good — re-renders only when theme changes
const theme = useSettingsStore(s => s.settings.theme)
const update = useSettingsStore(s => s.actions.update)  // stable ref
```

Actions are stable references — a single `useXxxStore(s => s.actions)` selector is acceptable for action bags.

### Module structure

```
web/src/
  lib/                    # infrastructure — no JSX, no component imports
    transport/
      client.ts           # fetch wrapper using __CROWBAR__.endpoint
      stream.ts           # ReadableStream helpers for crowbar://stream/
      types.ts            # CrowbarBridge interface, DaemonEvent union type
    queries/
      keys.ts             # ALL query keys — features never define their own
      client.ts           # QueryClient singleton with staleTime defaults
      workspaces.ts       # queryOptions per domain
      chats.ts
      git.ts
      files.ts
    mutations/
      optimistic.ts       # useOptimisticMutation wrapper
      workspaces.ts       # workspace mutations
      chats.ts
    events/
      types.ts            # discriminated union of all daemon events
      connect.ts          # connectDaemonEvents(queryClient)
    persistence/
      idb.ts              # IndexedDB adapter (idb library)
      schemas.ts          # object store type definitions
      hydrate.ts          # startup hydration sequence
  features/               # UI only — imports from lib/, never the reverse
    workspace/stores/
    editor/stores/
    git/stores/
    terminal/stores/
    chat/stores/
```

**The wall:** `lib/` never imports from `features/`. Cross-feature data flows through `lib/` only — a store, a query, or an event.

### Component decomposition

No component file exceeds ~200 lines. Each extracted component has one concern and is understandable without reading its parent.

| Monolith | Lines | Extract to |
|---|---|---|
| `file-explorer-tree.tsx` | 1472 | FileExplorerTree, FileExplorerItem, FileExplorerContextMenu, FileExplorerRenameInput |
| `pane-container.tsx` | 1208 | PaneContainer + EditorPane, DiffPane, ImagePane, TerminalPane, WebViewerPane, PdfPane |
| `git-view.tsx` | 1182 | GitView + GitStatusPanel, GitCommitPanel, GitBranchPanel, GitStashPanel |
| `tab-bar.tsx` | 995 | TabBar, TabItem (memoized), TabContextMenu, TabNewButton |

### Memoization rules

- `useMemo` — any `.filter()`, `.map()`, `.sort()`, `.reduce()` in a component body
- `useCallback` — any handler passed as a prop inside a `.map()`
- `React.memo` — pure presentational leaves rendered inside frequently-updating parents

### Animation constraints

Only GPU-composited properties may be animated: `transform`, `opacity`. Layout-affecting properties (`width`, `height`, `top`, `left`, `margin`, `padding`) are never animated.

During drag: all transitions on the dragged element and its layout siblings are disabled. Re-enable after `pointerup`.

Immediate fix: remove `transition-colors` from `pane-resize-handle.tsx`.

---

## Enforcement Rules (Three Things That Cause Rot)

1. **Query keys defined inside features.** All keys live in `lib/queries/keys.ts`. Any key defined outside this file breaks cross-domain invalidation.
2. **Features importing from other features.** Cross-feature data flows through `lib/` — a store, a query, or an event. Direct feature-to-feature imports create invisible coupling.
3. **Stores mixing UI state and server cache.** If a store field would be invalidated by a daemon event, it belongs in TanStack Query, not Zustand.
