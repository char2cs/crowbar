# Sidebar Refactor — Design Spec

**Date:** 2026-06-02
**Branch:** enhancement/design-language
**Approach:** Big bang — single PR, no feature flag. Pre-production, no users.

---

## Overview

Replace the current 2-tab sidebar (workspaces, files) with a 4-tab sidebar (workspaces, chats, files, git). Add trackpad horizontal swipe to switch tabs. Add a DiffPane for file-level diff viewing with inline comments. Delete the entire `branch-review` feature.

---

## What Changes

### Removed
- `branch-review` feature — all components, stores, routes (see Deletions section)
- Branch "Protected" tab — redundant with GitHub
- Branch description / About tab
- Dedicated branch review pane / double-sidebar layout
- `GitStatusPanel` usage in the sidebar (replaced by filtered FileExplorer)

### Added
- `SidebarCarousel` — horizontal scroll container with snap, owns trackpad swipe logic
- `SidebarTabBar` — pill-style tab strip using `Tab` from `@/components/ui/tabs.tsx`
- `ChatList` — flat conversation list for the active workspace
- `ChatListItem` — row component using `ROW_BASE` + dot + title + age
- `GitPanel` — CrossUI `Tabs` with Changes and History subtabs
- `DiffPane` — new pane type: file diff + inline comment threads
- `GitHistoryList` — commit list for the current branch (inside GitPanel > History)

### Modified
- `SidebarTab` type: `'workspaces' | 'files'` → `'workspaces' | 'chats' | 'files' | 'git'`
- `sidebar store`: add `setActiveTab` action; persist new type
- `FileExplorerTree`: add `filter?: 'all' | 'changed'` prop — when `'changed'`, shows only git-modified files; clicking any file opens DiffPane instead of EditorPane
- `SidebarTabs` + `sidebar-nav-icons`: replaced by `SidebarTabBar` + `SidebarCarousel`

---

## Component Design

### SidebarTabBar

- Uses `Tab` from `@/components/ui/tabs.tsx` (pill style — active tab gets filled rounded background)
- 4 items: Workspaces, Chats, Files, Git — each with Phosphor icon + label
- Sits above the carousel, always visible
- Clicking a tab calls `setActiveTab` on the store and `scrollTo()` on the carousel ref

### SidebarCarousel

- `display: flex; overflow-x: scroll; scroll-snap-type: x mandatory; scrollbar-width: none`
- Each panel: `min-width: 100%; scroll-snap-align: start`
- `wheel` event handler: if `Math.abs(deltaX) > Math.abs(deltaY)` → intercept and drive tab switch; else propagate to panel for vertical scroll
- On scroll settle (debounced `scrollend` with fallback): reads `scrollLeft / clientWidth` → syncs `activeTab` in store
- No visible scrollbar; trackpad-only feature — mouse users use the tab bar

### ChatList

**File:** `features/chats/components/chat-list.tsx`

- Flat list of conversations belonging to the active workspace
- Data source: existing conversations store (scoped to active workspace ID)
- No repo/branch header — context is already implicit from the active workspace
- `+ New` button pinned at bottom using `ROW_BASE` + `+` icon + "New" label, same pattern as workspace child creation. On click: creates a new conversation for the active workspace.

### ChatListItem

**File:** `features/chats/components/chat-list-item.tsx`

- Uses `ROW_BASE` from `workspace-row-base.ts` for visual consistency with workspace rows
- Layout: dot indicator (green = active/open, muted = past) + title (truncated) + relative age
- Clicking opens the conversation in the main area
- No drag, no rename, no children — intentionally simple

### GitPanel

**File:** `features/git/components/git-panel.tsx`

- Uses `Tabs`, `TabsList`, `TabsTab`, `TabsPanel` from `@/components/ui/tabs.tsx`
- Two subtabs:
  - **Changes**: `FileExplorerTree` with `filter="changed"` — same component as Files tab, different context. Clicking a file opens DiffPane.
  - **History**: `GitHistoryList` — commit list for the current branch (sha short, message, author avatar, relative age)
- Data: existing `git-store` for changed files; `GET /api/git/log` for history (already exists in the Go daemon — handler in `api/internal/api/v0/git_handler.go`, route registered in `router.go`)

### DiffPane

**File:** `features/panes/components/diff-pane.tsx`

- New pane type added alongside `EditorPane`, `TerminalPane`, etc.
- Pane identifier: `diff:<filePath>`
- Opens when a git-changed file is clicked in either the Files tab or the Git > Changes tab
- If a DiffPane for the same file already exists, focus it instead of opening a duplicate
- Renders unified or side-by-side diff for the file (vs. base branch)
- Inline comments:
  - Line-level comment threads, shown inline in the diff
  - Uses `comment-composer.tsx` and `review-thread.tsx` (moved from `branch-review`, see below)
  - Both human and agent comments supported
  - State: `diff-comments-store.ts` (renamed/gutted from `branch-review-data-store.ts`)

---

## Deletions

### `features/branch-review/` — deleted in full except:

| File | Action |
|------|--------|
| `comment-composer.tsx` | Move → `features/panes/components/` |
| `review-thread.tsx` | Move → `features/panes/components/` |
| `branch-review-data-store.ts` | Gut → keep only comment/thread state → rename `diff-comments-store.ts` in `features/panes/stores/` |
| Everything else | Delete |

Files deleted: `about-tab.tsx`, `git-tab.tsx`, `commits-tab.tsx`, `branch-review-pane.tsx`, `branch-review-diff-panel.tsx`, `branch-review-diff-viewer.tsx`, `diff-file-tree.tsx`, `diff-file-section.tsx`, `merge-button.tsx`, and any remaining files in the directory.

All imports, routes, and references to `branch-review` must be cleaned up across the codebase.

---

## Sidebar Store Changes

```ts
// Before
type SidebarTab = 'workspaces' | 'files'

// After
type SidebarTab = 'workspaces' | 'chats' | 'files' | 'git'
```

Add `setActiveTab(tab: SidebarTab)` action. Persisted via `sidebar-ui.ts` as before.

---

## FileExplorerTree Filter

`FileExplorerTree` gains a `filter?: 'all' | 'changed'` prop:

- `'all'` (default): existing behavior, full file tree
- `'changed'`: shows only git-modified/added/deleted files; clicking opens `DiffPane`

Used in:
- **Files tab**: `filter="all"` — full tree shown; git-changed files open DiffPane on click, unchanged files open EditorPane
- **Git > Changes tab**: `filter="changed"`

---

## UI Component Rule

Every new component must use only `@/components/ui/*`. No hardcoded colors, no custom primitives outside that directory. Specifically:

- Tab bars: `Tab`, `Tabs`, `TabsList`, `TabsTab` from `@/components/ui/tabs.tsx`
- Row layouts: `ROW_BASE` from `workspace-row-base.ts`
- Scroll areas: `@/components/ui/scroll-area.tsx`
- All other UI: existing `@/components/ui/` components

---

## Out of Scope

- Workspace switcher header redesign (separate ticket)
- Git > History: full commit detail view (clicking a commit)
- DiffPane: merge/approve actions
- Chat list: search or filtering
