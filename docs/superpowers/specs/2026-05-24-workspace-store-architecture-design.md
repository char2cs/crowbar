# Workspace-as-First-Class-Store Architecture

**Date:** 2026-05-24  
**Status:** Approved — implemented. The per-workspace store, slice composition and
registry described here are still how the frontend works. Details that named the
vendored editor codebase this frontend was originally assembled from have been
generalised, and that code has since been removed entirely; the file paths in the
LSP and icon-theme sections are historical.  
**Branch:** feature/agentic-ide-gordon

---

## Problem Statement

The current frontend is assembled from vendored IDE infrastructure with global singleton stores (`usePaneStore`, `useBufferStore`) that have no concept of "which workspace". Switching workspaces tramples shared state. The Crowbar AI workflow layer (`FlowTab`, `WorkspaceStepTabs`) is stitched on via `<Outlet />` inside pane buffers — a router outlet living inside the pane system. The result is buggy tab behavior, disappearing chat panels, broken split views, and no per-workspace isolation.

---

## Decision

**Option C: Workspace as first-class store.**

Each workspace gets its own Zustand store instance, created on first visit, persisted independently to localStorage, retrieved via React context. The vendored global stores are deleted. The vendored UI components are adopted into Crowbar and updated to use workspace-scoped hooks directly — no adapters, no hidden indirection.

---

## Architecture

### Routing

```
/__root__ (IDEShell)
├── /workspaces/$wsId    → WorkspaceView
├── /chat/$chatId        → StandaloneChatView
└── /                    → HomeView
```

Workflow step is **store state**, not a URL segment. No deep-linking to a specific step. Step navigation = `workspaceStore.setCurrentStep(stepId)` — no route change, no browser history entry.

### Component Tree

```
IDEShell                            thin shell: routing + sidebar only
├── SidebarPanel                    unchanged
└── WorkspaceView [wsId]            creates/retrieves workspace store
    └── WorkspaceStoreProvider      React context holding the store instance
        └── WorkspaceLayoutRoot
            ├── SplitViewRoot       reads from WorkspaceStoreContext
            │   └── PaneNodeRenderer → PaneContainer → [buffer content]
            │       └── FlowContent [wsId]   rendered inside crowbarChat buffer
            │           └── ChatView | DiffView | SplitView  (driven by FlowDefinition)
            └── WorkspaceStepFooter always visible at bottom; reads flowDefinition + currentStep
```

### Store Registry

```typescript
// workspace-store-registry.ts
const registry = new Map<string, WorkspaceStore>()

export function getOrCreateWorkspaceStore(wsId: string): WorkspaceStore {
  if (!registry.has(wsId)) {
    const snapshot = loadFromLocalStorage(wsId)   // null on first visit
    registry.set(wsId, createWorkspaceStore(wsId, snapshot))
  }
  return registry.get(wsId)!
}

export function destroyWorkspaceStore(wsId: string): void {
  registry.delete(wsId)
}
```

Workspace switch = unmount `WorkspaceView[wsId=A]`, mount `WorkspaceView[wsId=B]`. Store A stays alive in the registry; its panes and tabs survive. Store B loads from localStorage or starts fresh.

---

## Workspace Store

Built with Zustand's `createStore` (not `createWithEqualityFn`) — required for instancing. Composed from slices. All slices share the same `set`/`get` so cross-slice reads are trivial.

```typescript
function createWorkspaceStore(wsId: string, snapshot?: Partial<WorkspaceSnapshot>) {
  return createStore<WorkspaceState>()(
    immer((set, get) => ({
      workspaceId: wsId,
      ...createPaneSlice(set, get),
      ...createBufferSlice(set, get),
      ...createWorkflowSlice(set, get),
      ...createLspSlice(set, get),
      ...createTerminalSlice(set, get),
      ...createFileWatcherSlice(set, get),
      ...createRecentFilesSlice(set, get),
      ...(snapshot ?? {}),
    }))
  )
}
```

### Slice: Pane

Owns the pane tree. Delegates tree mutations to pure functions in `pane-tree.ts` (unchanged).

```typescript
interface PaneSlice {
  paneRoot: PaneNode
  bottomRoot: PaneNode
  activePaneId: string
  mostRecentActivePaneIds: string[]
  fullscreenPaneId: string | null
  paneActions: {
    splitPane(paneId, direction, bufferId?, placement?): string | null
    closePane(paneId): void
    setActivePane(paneId): void
    activatePaneBuffer(paneId, bufferId): void
    addBufferToPane(paneId, bufferId, setActive?): void
    removeBufferFromPane(paneId, bufferId, preserveEmpty?): void
    moveBufferToPane(bufferId, fromPaneId, toPaneId): void
    setPanePreviewBuffer(paneId, bufferId): void
    setPaneBufferPinned(paneId, bufferId, pinned): void
    reorderPaneBuffers(paneId, startIndex, endIndex): void
    resizePaneSplit(splitId, index, sizes): void
    distributePaneSplit(splitId): void
    togglePaneFullscreen(paneId): void
    exitPaneFullscreen(): void
    getAllPaneGroups(): PaneGroup[]
    getPaneById(paneId): PaneGroup | null
    getPaneByBufferId(bufferId): PaneGroup | null
    getActivePane(): PaneGroup | null
  }
}
```

### Slice: Buffer

Manages open buffers (tabs). Supports the content types Crowbar actually uses now; type-guards and content factories handle each. `pane-content.ts` type definitions are kept as-is — all 20 types defined, only the ones below are actively created.

**Active content types:**
- `editor` — file viewing and editing
- `crowbarChat` — the AI chat pane (one per workspace, opened on workspace load)
- `diff` — diff viewer (rendered inside crowbarChat area when backend says `contentType: 'diff'`)

**Inactive but typed (no-op creation, renders empty):** everything else from the vendored union, kept so type-checking doesn't break when components reference them.

```typescript
interface BufferSlice {
  buffers: PaneContent[]
  bufferActions: {
    openContent(spec: OurOpenContentSpec): string   // returns bufferId
    closeBuffer(bufferId): void
    closeOthers(bufferId, paneId): void
    setActiveBuffer(paneId, bufferId): void
    setPinned(paneId, bufferId, pinned): void
    setPreview(paneId, bufferId): void
    getBufferById(bufferId): PaneContent | null
  }
}

type OurOpenContentSpec =
  | { type: 'editor'; path: string; name: string; content: string; isPreview?: boolean }
  | { type: 'crowbarChat'; wsId: string; name: string }
  | { type: 'diff'; path: string; name: string; content: string; diffData?: GitDiff }
```

Session persistence: on workspace snapshot, persist `buffers` filtered to non-virtual content (i.e., `editor` buffers with their paths). `crowbarChat` is always re-opened on workspace load. Terminal sessions are not persisted — re-attached by session ID on load.

### Slice: Workflow

Crowbar-specific. The `flowDefinition` comes from the backend and drives what `FlowContent` renders and what `WorkspaceStepFooter` displays.

```typescript
type StepContentType = 'chat' | 'diff' | 'split'  // split = chat + diff side-by-side

interface FlowStep {
  id: string
  label: string
  icon?: string
  contentType: StepContentType
  isCompleted: boolean
  isActive: boolean
}

interface FlowDefinition {
  flowId: string
  flowType: string          // e.g. 'crowbar-default'; extensible to other flow types
  steps: FlowStep[]
}

interface WorkflowSlice {
  flowDefinition: FlowDefinition | null
  currentStepId: string | null
  workflowActions: {
    setFlowDefinition(def: FlowDefinition): void
    setCurrentStep(stepId: string): void
    markStepCompleted(stepId: string): void
  }
}
```

`FlowContent` renders based on `currentStep.contentType`:
- `'chat'` → `<ChatView workspaceId={wsId} stepId={stepId} />`
- `'diff'` → `<DiffView workspaceId={wsId} stepId={stepId} />`
- `'split'` → `<SplitView>` with both side-by-side

`WorkspaceStepFooter` maps `flowDefinition.steps` to tab buttons. Clicking a step:
1. Calls `setCurrentStep(stepId)`
2. If `crowbarChat` buffer is not the active buffer in the active pane → `activatePaneBuffer(activePaneId, crowbarChatBufferId)`

### Slice: LSP

Wraps the existing `editor/lsp/` implementation (~2,300 lines, kept as-is). The slice makes LSP workspace-scoped by owning the workspace root and providing the connection point.

```typescript
interface LspSlice {
  workspaceRoot: string
  lspStatus: LspStatusInfo     // { status, activeWorkspaces, lastError?, supportedLanguages? }
  completionCache: CompletionCache
  currentCompletionRequest: AbortController | null
  lspActions: {
    setWorkspaceRoot(root: string): void
    setCompletionHandlers(handlers: CompletionHandlers): void
    requestCompletion(filePath, line, character): Promise<CompletionItem[]>
    applyCompletion(completion: CompletionItem): void
    updateLspStatus(info: Partial<LspStatusInfo>): void
    clearCompletionCache(): void
  }
}
```

The existing `use-lsp-integration.ts` and `use-lsp-initialization.ts` hooks in the vendored editor feature directory (since removed) are updated to read from workspace context instead of the global `lsp-store`.

### Slice: Terminal

Wraps the existing terminal implementation (~5,600 lines, kept as-is). The slice adds workspace ownership — tracking which terminal session IDs belong to this workspace.

```typescript
interface TerminalSlice {
  terminalSessionIds: Set<string>    // sessions belonging to this workspace
  terminalLayout: {
    widthMode: 'full' | 'editor'
    tabLayout: 'horizontal' | 'vertical'
    tabSidebarWidth: number
    tabSidebarPosition: 'left' | 'right'
  }
  terminalActions: {
    registerSession(sessionId: string): void
    unregisterSession(sessionId: string): void
    hasSession(sessionId: string): boolean
    setWidthMode(mode: 'full' | 'editor'): void
    setTabLayout(layout: 'horizontal' | 'vertical'): void
    setTabSidebarWidth(width: number): void
    setTabSidebarPosition(pos: 'left' | 'right'): void
  }
}
```

The global `terminal-store.ts` and `terminal-slots-store.ts` remain as-is (they own xterm instances and portal slots, which are inherently global). The terminal slice owns which sessions are *associated* with this workspace. Terminal components check `terminalActions.hasSession(sessionId)` to filter to workspace sessions.

### Slice: File Watcher

Currently a 23-line stub. Properly implemented:

```typescript
interface FileWatcherSlice {
  watchedPaths: Set<string>
  pendingSaves: Set<string>
  fileWatcherActions: {
    watchPath(path: string): void
    unwatchPath(path: string): void
    markPendingSave(path: string): void
    clearPendingSave(path: string): void
    isPendingSave(path: string): boolean
    getWatchedPaths(): string[]
  }
}
```

### Slice: Recent Files

Currently a 7-line no-op stub. Properly implemented:

```typescript
interface RecentFilesSlice {
  recentFiles: RecentFileEntry[]    // max 50, most recent first
  recentFilesActions: {
    addRecentFile(path: string, name: string): void
    removeRecentFile(path: string): void
    clearRecentFiles(): void
    getRecentFiles(): RecentFileEntry[]
  }
}

interface RecentFileEntry {
  path: string
  name: string
  lastOpenedAt: number   // Date.now()
}
```

Persisted to localStorage as part of the workspace snapshot.

---

## Hooks: How UI Components Connect

The existing vendored UI components (`SplitViewRoot`, `PaneNodeRenderer`, `PaneContainer`, `TabBar`, etc.) are adopted into Crowbar and their imports updated. No adapters — the hooks are direct.

```typescript
// features/workspace/stores/hooks/use-pane-store.ts
// (replaces features/panes/stores/pane-store.ts)

export const usePaneRoot = () =>
  useWorkspaceStoreContext()(s => s.paneRoot)

export const usePaneActions = () =>
  useWorkspaceStoreContext()(s => s.paneActions)

export const useFullscreenPaneId = () =>
  useWorkspaceStoreContext()(s => s.fullscreenPaneId)

// features/workspace/stores/hooks/use-buffer-store.ts
// (replaces features/editor/stores/buffer-store.ts)

export const useBuffers = () =>
  useWorkspaceStoreContext()(s => s.buffers)

export const useBufferActions = () =>
  useWorkspaceStoreContext()(s => s.bufferActions)
```

Component call sites update from `usePaneStore.use.root()` → `usePaneRoot()`. One-time change, permanent clarity.

---

## Persistence

Snapshot stored to `localStorage` as `workspace:${wsId}:state`.

**Persisted:**
- `paneRoot`, `bottomRoot`, `activePaneId`, `fullscreenPaneId`
- `buffers` (editor type only, filtered — crowbarChat always re-opened fresh)
- `currentStepId`
- `recentFiles`
- `terminalLayout`

**Not persisted (reinitialized on load):**
- Terminal session instances (reconnect by session ID)
- LSP state (re-initialize on workspace load)
- `flowDefinition` (fetched from backend on workspace load)
- File watcher state (re-established on workspace load)

---

## File Structure

### New files

```
web/src/features/workspace/
  stores/
    workspace-store.ts               createWorkspaceStore() factory
    workspace-store-registry.ts      Map<wsId, Store>; getOrCreate; destroy
    workspace-persistence.ts         serialize/deserialize WorkspaceSnapshot
    workspace-context.ts             WorkspaceStoreContext; useWorkspaceStoreContext()
    slices/
      pane-slice.ts                  ~200 lines
      buffer-slice.ts                ~300 lines
      workflow-slice.ts              ~100 lines
      lsp-slice.ts                   ~150 lines (wraps editor/lsp)
      terminal-slice.ts              ~120 lines (wraps terminal feature)
      file-watcher-slice.ts          ~80 lines
      recent-files-slice.ts          ~80 lines
    hooks/
      use-pane-store.ts              replaces global usePaneStore
      use-buffer-store.ts            replaces global useBufferStore
      use-workflow.ts                currentStep, flowDefinition selectors
      use-workspace-effects.ts       file system seeding, crowbarChat buffer init

  components/
    WorkspaceView.tsx                entry: getOrCreate store, provide context, effects
    WorkspaceLayoutRoot.tsx          flex-col: SplitViewRoot + WorkspaceStepFooter
    WorkspaceStepFooter.tsx          step tab bar; always visible; navigate-to-chat logic

web/src/features/workflow/
  components/
    FlowContent.tsx                  dynamic renderer; reads currentStep.contentType
    ChatView.tsx                     chat interface (step content)
    DiffView.tsx                     diff viewer with comments (step content)
    SplitView.tsx                    chat + diff side-by-side (step content)
  types/
    workflow.ts                      FlowDefinition, FlowStep, StepContentType
```

### Deleted

```
web/src/features/panes/stores/pane-store.ts           → pane-slice.ts
web/src/features/editor/stores/buffer-store.ts        → buffer-slice.ts
web/src/components/layout/FlowTab.tsx                 → FlowContent.tsx
```

### Updated (import paths + hook call sites only)

```
web/src/features/panes/components/split-view-root.tsx
web/src/features/panes/components/pane-container.tsx
web/src/features/panes/components/pane-node-renderer.tsx
web/src/features/tabs/components/tab-bar.tsx
web/src/features/tabs/components/tab-bar-item.tsx
web/src/features/<vendored-editor>/hooks/use-lsp-integration.ts   # since removed
web/src/features/<vendored-editor>/hooks/use-lsp-initialization.ts # since removed
web/src/components/layout/IDEShell.tsx
```

### Unchanged (pure functions, no store dependency)

```
web/src/features/panes/utils/pane-tree.ts
web/src/features/panes/components/pane-resize-handle.tsx
web/src/features/panes/types/pane-content.ts
web/src/components/ui/tabs.tsx
web/src/features/terminal/**   (all 32 files; terminal-slice wraps, not replaces)
web/src/features/editor/lsp/** (all 18 files; lsp-slice wraps, not replaces)
```

---

## File Explorer Icons

### Current State

The rendering infrastructure is complete — `FileExplorerIcon` component, icon theme store (defaults to `"material"`), settings UI, and command palette selector all work. The `IconThemeRegistry` is a stub: `registerTheme()` is a no-op, so the component always falls back to generic Phosphor icons.

### What We Port

All 6 builtin icon themes, plus the real registry implementation and an initializer:

| Theme | Description | Icon source |
|---|---|---|
| `colorful-material` | Material Design, full color (default) | `material-file-icons` npm package |
| `material` | Material Design, monochrome (CSS `currentColor`) | `material-file-icons` npm package |
| `minimal` | Simple monochrome | Phosphor icons + 3 bundled SVGs |
| `classic` | Traditional file manager style | Phosphor icons |
| `compact` | Space-efficient | Phosphor icons |
| `none` | Generic file/folder only | Phosphor icons |

### Files

**Ported in to replace stubs:**
```
web/src/extensions/icon-themes/icon-theme-registry.ts    ← real implementation
web/src/extensions/icon-themes/icon-theme-initializer.ts ← registers all 6 themes at startup
web/src/extensions/icon-themes/builtin/
  manifest.json
  material-theme.tsx
  colorful-material-theme.tsx
  minimal-theme.tsx
  classic-theme.tsx
  compact-theme.tsx
  none-theme.tsx
web/src/extensions/bundled/icon-themes/minimal/icons/
  file.svg
  folder.svg
  folder-open.svg
```

**Unchanged (already correct):**
```
web/src/extensions/icon-themes/types.ts
web/src/features/file-explorer/file-explorer/components/file-explorer-icon.tsx
web/src/features/settings/stores/icon-theme-store.ts
web/src/features/settings/components/tabs/appearance-settings.tsx
web/src/features/command-palette/components/icon-theme-selector.tsx
```

**Stub to delete:**
```
web/src/features/file-explorer/components/file-explorer-icon.tsx  ← "will be replaced" comment
```

### Wiring

`icon-theme-initializer.ts` is called once at app startup in `main.tsx`, before the React tree mounts. This registers all 6 themes with the registry. The active theme is read from `icon-theme-store` (persisted to localStorage). No workspace-scoped state — icon theme is a global user preference.

### Dependency

```
npm install material-file-icons
```

One package. No other dependencies.

---

## What This Fixes

| Bug | Root Cause | Fix |
|---|---|---|
| Chat disappears on split | IDEShell rendered only first pane | SplitViewRoot renders full tree ✓ (already fixed) |
| Tabs lose state on workspace switch | Global store trampled | Per-workspace store, pointer switch |
| `<Outlet />` inside pane buffer | FlowTab used router outlet | FlowContent reads from store |
| Step tabs inside chat | WorkspaceStepTabs in wrong place | WorkspaceStepFooter outside pane area |
| No per-workspace pane isolation | Global usePaneStore | Pane slice per workspace |

---

## Out of Scope (this refactor)

- LSP server connection implementation (slice interface defined, server wiring is a follow-on)
- Terminal WebSocket/PTY implementation (slice interface defined, connection is a follow-on)
- Full file system backend (mock layer stays for now)
- Multiple simultaneous workspace views (one workspace active at a time)
