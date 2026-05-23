# Athas UI Integration — Design Spec

**Date:** 2026-05-23  
**Status:** Approved  
**Author:** Mateo Urrutia  

---

## 1. Goal

Crowbar's web frontend gains a full IDE shell by copying Athas's feature modules wholesale and composing them with Crowbar's existing UI. The result must be **100% production-ready frontend code**: no TODOs, no stubs (other than explicitly marked API swap points), full TypeScript coverage, error boundaries, loading states, and the existing 95% test coverage bar maintained throughout.

All Athas components are wired to mock data this session. Swapping mock data for real Crowbar API calls is a future session — but the call sites must be written in native Crowbar style from day one so the swap is a one-line change per query function.

---

## 2. What We Are NOT Changing

The following Crowbar components are **untouched**. No edits, no renames, no restructuring of internals:

- `components/chat/ChatView.tsx`
- `components/chat/ChatInput.tsx`
- `components/chat/MessageBubble.tsx`
- `components/chat/ToolCallSeparator.tsx`
- `components/chat/ChatEmptyState.tsx`
- `components/layout/WorkspaceStepTabs.tsx`
- `components/review/DiffView.tsx` (stub — stays as stub)
- `components/layout/SidebarHeader.tsx`
- `components/layout/SidebarRow.tsx`
- `components/layout/SidebarSkeleton.tsx`
- `lib/store/sidebar.ts`
- `lib/mock/workspaces.ts`, `lib/mock/chats.ts`, `lib/mock/conversations.ts`, `lib/mock/flows.ts`, `lib/mock/projects.ts`
- All routes under `routes/` — **with one exception**: `routes/workspaces/$wsId.tsx` has its `WorkspaceStepTabs` wrapper removed (it moves into `FlowTab`). The route otherwise becomes a pass-through `<Outlet />`.
- All existing shadcn/ui components under `components/ui/`

---

## 3. Architecture

```
IDEShell                              ← new root layout (replaces AppShell in __root.tsx)
├── MainSidebar (Athas)               ← outer panel: resize, show/hide, panel chrome
│   └── SidebarTabs                   ← new glue component
│       ├── [tab: Workspaces]
│       │   └── WorkspacesSidebarPanel  ← Sidebar.tsx renamed, internals untouched
│       │       ├── SidebarHeader
│       │       ├── ChatRow / NewRow
│       │       ├── Separator
│       │       └── RepoRow / WorkspaceRow / NewRow
│       ├── [tab: Files]
│       │   └── FileExplorerTree (Athas)
│       └── [tab: Git]
│           └── GitView (Athas)
│
└── SplitViewRoot (Athas)             ← main area, manages split pane tree
    ├── PaneNodeRenderer (Athas)      ← recursive, renders each pane
    │   └── PaneContainer (Athas)
    │       ├── TabBar (Athas)        ← file tabs, drag-to-reorder via @dnd-kit
    │       │   ├── [tab: Flow — locked, always first]
    │       │   │   └── FlowTab                       ← new glue component
    │       │   │       ├── ChatView / DiffView        ← untouched, driven by route
    │       │   │       └── [pinned bottom]
    │       │   │           ├── ChatInput              ← untouched
    │       │   │           └── WorkspaceStepTabs      ← untouched, repositioned here
    │       │   └── [tab: filename.ts …]
    │       │       └── CodeEditor (Athas / Monaco)
    │       └── BottomPane (Athas)
    │           └── Terminal (Athas / xterm.js)
    │
    └── CommandPalette (Athas)        ← global overlay, outside pane tree
```

**Routing changes (two files only):**
- `__root.tsx` — swaps `AppShell` for `IDEShell`
- `routes/workspaces/$wsId.tsx` — `WorkspaceStepTabs` is removed from `WorkspaceLayout` (it moves into `FlowTab`); the layout becomes a bare `<Outlet />`

All other routes are untouched.

---

## 4. Component Inventory

### 4a. Copied from Athas (full feature modules)

Copy each feature folder from `athas/src/features/` into `web/src/features/`. Bring components, stores, hooks, types, and utils. Strip Athas-specific dependencies that don't apply (LSP, remote, vim, extensions).

| Feature | Key contents |
|---|---|
| `features/panes/` | `SplitViewRoot`, `PaneContainer`, `PaneNodeRenderer`, `PaneResizeHandle`, `SplitDropOverlay`, pane store |
| `features/tabs/` | `TabBar`, `TabBarItem`, `TabContextMenu`, `TabDragPreview`, buffer store |
| `features/editor/` | `CodeEditor`, `MonacoEditor`, toolbar, breadcrumb, find bar |
| `features/athas-editor/` | Custom rendering layers (highlight, cursor, gutter, git blame) |
| `features/file-explorer/` | `FileExplorerTree`, `FileExplorerTreeItem`, `FileExplorerIcon`, tree store, clipboard store |
| `features/terminal/` | `Terminal`, `TerminalContainer`, `TerminalTabBar`, `TerminalSession`, terminal store |
| `features/git/` | `GitView`, `GitStatusPanel`, `GitCommitHistory`, `GitBranchManager`, `GitCommitPanel` |
| `features/settings/` | `SettingsDialog`, `SettingsVerticalTabs`, `SettingsSection` + 7 panel components |
| `features/command-palette/` | `CommandPalette`, quick-open file picker |
| `features/layout/` | `MainSidebar`, `SidebarActivityRail`, `BottomPane`, `Footer` (status bar) |

### 4b. New files in Crowbar

| File | Purpose |
|---|---|
| `components/layout/IDEShell.tsx` | Root layout — replaces AppShell, composes MainSidebar + SplitViewRoot |
| `components/layout/SidebarTabs.tsx` | Tab strip (Workspaces / Files / Git) + slots for panel content |
| `components/layout/WorkspacesSidebarPanel.tsx` | Rename of `Sidebar.tsx` — same internals, extracted from outer container |
| `components/layout/FlowTab.tsx` | Locked tab wrapper — renders ChatView or DiffView + repositions WorkspaceStepTabs below ChatInput |
| `lib/mock/files.ts` | Mock file tree for the active workspace worktree |
| `lib/mock/git-data.ts` | Mock git status, staged/unstaged files, branches, commit history |
| `lib/mock/terminal.ts` | Mock terminal session seed (prompt + prior output) |

### 4c. Athas features NOT copied

| Feature | Reason |
|---|---|
| `features/vim/` | Out of scope |
| `features/lsp/` | Out of scope — no language server this session |
| `features/extensions/` | Out of scope |
| `features/remote/` | Out of scope — Crowbar is local-first |
| `features/ai/` | Crowbar owns its own AI/model wiring |
| `features/collaboration/` | Out of scope |
| `features/window/` | Tauri window chrome — Crowbar has its own Tauri wrapper. The settings trigger button (normally in Athas's title bar) is placed in the `SidebarHeader` instead, which already has a project menu and user avatar. |
| `features/search/` | Deferred — add in a future session |

---

## 5. Sidebar Refactor

`components/layout/Sidebar.tsx` is split into two concerns:

**`WorkspacesSidebarPanel.tsx`** — the content panel for the Workspaces tab. Identical internals to the current `Sidebar.tsx`. Props interface unchanged. This component is the inner content only — no outer container, no resize logic.

**`SidebarTabs.tsx`** — the new tab strip and panel switcher. Uses shadcn/ui `Tabs` for the Workspaces / Files / Git strip. Renders `WorkspacesSidebarPanel` for the Workspaces tab, `FileExplorerTree` for Files, `GitView` for Git. Active workspace context (from sidebar store) is passed into FileExplorerTree and GitView so they reflect the correct repo/worktree when the workspace changes.

**`MainSidebar` (from Athas)** — the outer container. Handles panel width, resize handle, show/hide. Wraps `SidebarTabs` entirely. The existing `useSidebarWidth` hook in Crowbar is retired — Athas's sidebar resize infrastructure takes over.

### Workspace context propagation

When the user switches workspaces in the Workspaces tab, the `Files` and `Git` tabs must update to reflect that workspace's repo and worktree. This is handled by reading `activeWorkspaceId` from `useSidebarStore` (already exists) and passing the resolved `repoPath` and `worktreePath` as props into `FileExplorerTree` and `GitView`.

---

## 6. Flow Tab

`FlowTab.tsx` is a thin wrapper component that:

1. Renders the current route's `<Outlet />` content (which is either `ChatView` or `DiffView` — unchanged)
2. Pins `WorkspaceStepTabs` **below** `ChatInput` at the bottom of the panel
3. Locks itself as the first, non-closeable tab in the `TabBar` — no close button rendered, cannot be dragged away from position 0

The flow tab label reflects the active workspace's branch name (e.g. `feat/payment-flow`). A small status dot (green = active agent, grey = idle) sits left of the label.

`WorkspaceStepTabs` is not moved in the file — only its render location changes. It is lifted out of `WorkspaceLayout` (`routes/workspaces/$wsId.tsx`) and placed inside `FlowTab`. The `WorkspaceLayout` route becomes a pass-through `<Outlet />` with no wrapping chrome.

---

## 7. Data Layer

### Mock data pattern

Extends Crowbar's existing `lib/mock/` convention. All mock files export plain synchronous functions returning typed data — no async, no network.

```ts
// lib/mock/files.ts
export function getMockFileTree(rootPath: string): FileNode[] { ... }

// lib/mock/git-data.ts
export function getMockGitStatus(repoPath: string): GitStatus { ... }
export function getMockCommitHistory(repoPath: string): Commit[] { ... }
export function getMockBranches(repoPath: string): Branch[] { ... }

// lib/mock/terminal.ts
export function getMockTerminalSeed(): TerminalSeed { ... }
```

### TanStack Query integration

All new query options live in `lib/queries.ts` alongside existing options:

```ts
export const fileTreeQueryOptions = (rootPath: string) =>
  queryOptions({
    queryKey: ['file-tree', rootPath],
    queryFn: () => getMockFileTree(rootPath),
    // FUTURE: queryFn: () => api.get(`/fs/tree?root=${rootPath}`)
  })

export const gitStatusQueryOptions = (repoPath: string) =>
  queryOptions({
    queryKey: ['git-status', repoPath],
    queryFn: () => getMockGitStatus(repoPath),
    // FUTURE: queryFn: () => api.get(`/git/status?repo=${repoPath}`)
  })

export const gitHistoryQueryOptions = (repoPath: string) =>
  queryOptions({
    queryKey: ['git-history', repoPath],
    queryFn: () => getMockCommitHistory(repoPath),
    // FUTURE: queryFn: () => api.get(`/git/log?repo=${repoPath}`)
  })

export const gitBranchesQueryOptions = (repoPath: string) =>
  queryOptions({
    queryKey: ['git-branches', repoPath],
    queryFn: () => getMockBranches(repoPath),
    // FUTURE: queryFn: () => api.get(`/git/branches?repo=${repoPath}`)
  })
```

### Athas invoke() replacement

Every `invoke()` call site in the copied Athas stores is replaced with the equivalent `queryClient.fetchQuery()` call using the options above. Each replaced call site has a single-line comment: `// FUTURE: real Crowbar API call`.

---

## 8. shadcn/ui Replacement Map

All Athas feature module imports of `@/ui/*` are rewritten to `@/components/ui/*`.

### Category 1 — Already in Crowbar (import path swap only)

`button` `badge` `card` `dialog` `dropdown-menu` `input` `scroll-area` `select` `separator` `skeleton` `spinner` `tabs` `textarea` `tooltip` `toggle` `toggle-group` `progress` `collapsible` `command` `hover-card` `avatar`

### Category 2 — Standard shadcn, added to Crowbar

```bash
npx shadcn add context-menu   # right-click menus throughout IDE
npx shadcn add resizable      # replaces Athas's custom ResizablePane
npx shadcn add sonner         # toast notifications
npx shadcn add switch         # settings toggles
npx shadcn add checkbox       # settings checkboxes
npx shadcn add label          # settings form labels
npx shadcn add slider         # font size / zoom sliders in settings
```

### Category 3 — Custom Athas Radix wrappers (no shadcn equivalent, copied as-is)

These four components have no shadcn/ui equivalent and are copied directly into `web/src/components/ui/`, clearly marked:

| Component | Why it cannot be replaced |
|---|---|
| `pane.tsx` | Pane resize/drop-target primitives tightly coupled to the split pane system |
| `sidebar-tree.tsx` | File tree row styling with indent levels, icon slots, and git status badges |
| `item.tsx` | Generic interactive list item used throughout IDE panels |
| `primitive-dialog-service.tsx` | Imperative programmatic dialog triggered from store actions |

Athas's custom `dropdown.tsx` is **replaced** by shadcn's `dropdown-menu` — it covers all use cases within the copied features.

---

## 9. Settings

The settings modal shell (dialog, vertical tab nav, section wrapper) is copied wholesale from Athas.

### Panels copied (7 of 15)

| Panel | Contents |
|---|---|
| General | Theme selection, language, startup behaviour |
| Appearance | Font family, font size, UI density, color scheme |
| Editor | Tab size, word wrap, minimap, line numbers, format on save, bracket coloring |
| Terminal | Shell profile, terminal font, cursor style, scrollback buffer size |
| Keybindings | Keyboard shortcut browser and customisation |
| Git | Default branch name, commit signing, diff presentation preferences |
| File Tree | Show hidden files, exclude glob patterns, indent size |

### Panels dropped (8 of 15)

`AI Settings` `API Connections` `Account` `Enterprise` `Collaboration` `Extensions` `Database` `Remote` — these either conflict with Crowbar's own implementations or are irrelevant to a local-first agentic IDE.

---

## 10. Dependencies Added

The following npm packages are added to `web/package.json`. All are already present in Athas — versions are taken directly from Athas's `package.json` to avoid compatibility issues.

| Package | Purpose |
|---|---|
| `monaco-editor` | Code editor core |
| `@monaco-editor/react` | Monaco React wrapper |
| `xterm` | Terminal emulator |
| `xterm-addon-fit` | Terminal auto-resize |
| `xterm-addon-web-links` | Clickable links in terminal |
| `xterm-addon-search` | In-terminal search |
| `xterm-addon-clipboard` | Clipboard integration |
| `@tanstack/react-virtual` | Virtualization for file explorer |
| `@dnd-kit/core` | Drag-and-drop core |
| `@dnd-kit/sortable` | Tab drag-to-reorder |
| `@dnd-kit/modifiers` | Drag constraints |
| `@dnd-kit/utilities` | Drag utilities |
| `material-file-icons` | File type icons in explorer |
| `zustand/middleware` (immer) | Already in Athas stores — immer middleware for Zustand |

---

## 11. Production Quality Requirements

This is non-negotiable. The frontend must be production-ready after this plan executes.

### TypeScript
- No `any` types. Every component, store, hook, and utility is fully typed.
- Strict mode remains enabled in `tsconfig.json`.

### Error handling
- Every Athas feature that can fail (file load, git query, terminal session) is wrapped in an `ErrorBoundary`.
- TanStack Query error states are handled with visible UI feedback, not silent failures.
- No unhandled promise rejections.

### Loading states
- Every async query has a loading state. Skeleton components (already in Crowbar) are used where content loads.
- The terminal shows a "connecting…" state before the mock session seeds.

### Tests
- The 95% frontend coverage target is maintained.
- Every new component in `components/layout/` has a corresponding test in `__tests__/components/layout/`.
- Copied Athas stores that are modified (invoke() replacements) have tests covering the modified actions.
- Mock data functions in `lib/mock/` are tested.

### No debug artifacts
- No `console.log`, no `console.error` outside of error boundaries.
- No hardcoded strings that should be constants.
- No commented-out code blocks.

### Accessibility
- All interactive elements have appropriate ARIA labels.
- Tab bar, file explorer, and settings panels are keyboard-navigable.
- Focus management is correct when modals open/close.

---

## 12. Out of Scope

The following are explicitly deferred to future sessions:

- Real file I/O (reading/writing files via Go API)
- Real terminal PTY (Go backend WebSocket)
- Real git operations (Go backend git endpoints)
- LSP / language server integration
- Tab drag-and-drop persistence across sessions
- Split pane layout persistence
- Vim mode
- Extension system
- Search panel
- Remote connections
- Memory management panel
- Activity graph
