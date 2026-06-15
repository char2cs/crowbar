# Workspace Switcher — Design

**Date:** 2026-06-15
**Status:** Approved (pending spec review)
**Builds on:** [Context Pill](2026-06-15-context-pill-design.md)

## Problem

The context pill tells you *where you are*, but switching workspaces still requires opening the Workspaces sidebar tab and clicking through the tree. There's no fast, search-driven way to jump between workspaces — especially from the Chats / Files / Git tabs, where the tree isn't even visible.

## Solution

Turn the context pill into the trigger for a **searchable workspace switcher**: clicking the pill opens an anchored popover containing a command menu (the coss `Command` component) listing every workspace across all repos. Type to filter, ↑↓/Enter or click to jump. Selecting a workspace **navigates the route only** — it does not change the sidebar's active tab or content.

This reuses the project's existing command-menu UI (the same component documented at coss.com/ui — vendored as `@/components/ui/command.tsx`) and fits the app's architecture: workspace navigation (`/workspaces/$wsId`) is already orthogonal to the sidebar `activeTab`.

### Interaction

- **Open:** click the context pill (this replaces the pill's old "jump to Workspaces tab" click). The pill's content and styling are unchanged.
- **Search:** auto-focused input; filters across `repo / branch`.
- **Navigate:** ↑↓ to move, Enter or click to select → `navigate({ to: '/workspaces/$wsId', params: { wsId } })`, then close the popover.
- **Dismiss:** Esc or outside-click closes; no navigation.
- **Current workspace:** marked with a ✓ and visually highlighted.
- **Sidebar tab/content never changes** as a result of any of the above.

### Row content

Flat list (not grouped). Each row:

```
[WorkspaceBranchIcon status]  reponame / branchname   +added −deleted   ✓(if current)
```

- Status icon: existing `WorkspaceBranchIcon` (reused).
- `reponame / branchname`: plain text, mono, consistent with the pill.
- Change counts: `+added` (green) / `−deleted` (red), formatted like the tree (`+12k`, `−69`); omitted when both are absent/zero.
- ✓ marker only on the currently-active workspace.

## Architecture

### New files

- `web/src/components/layout/workspace-switcher-model.ts` — **pure** helpers, no React:
  - `type WorkspaceSwitcherItem = { wsId: string; repoName: string; branch: string; status: WorkspaceStatus; added?: number; deleted?: number; isCurrent: boolean }`
  - `flattenWorkspaces(repos: Repo[], activeWorkspaceId: string | undefined): WorkspaceSwitcherItem[]` — flattens `repos → workspaces`, attaches `repoName`, computes `isCurrent`, defaults `status` to `'new'`.
  - `filterWorkspaces(items: WorkspaceSwitcherItem[], query: string): WorkspaceSwitcherItem[]` — uses `matchesSearchQuery(query, [repoName, branch])` from `@/utils/search-match` (same matcher the command palette uses). Empty/whitespace query returns all items.
- `web/src/components/layout/workspace-switcher.tsx` — `WorkspaceSwitcher`: the `Popover` + `Command` menu composition. Owns the open state and the query string, reads `repos` + active workspace id, derives the item list via the model, renders rows, and handles select → navigate → close. Takes the pill as its trigger (via `children` / render).

### Modified files

- `web/src/components/layout/context-pill.tsx` — the rendered `Button` becomes the popover trigger instead of calling `setActiveTab`. Remove the `setActiveTab` onClick; update `aria-label` to `"Switch workspace"`. Pill content/markup/styling otherwise unchanged. The simplest structure: `ContextPill` renders the trigger button (its current JSX), and `WorkspaceSwitcher` wraps it as `PopoverTrigger`. Mounting in `ide-shell.tsx` stays the same (still `{!hasNavScreen && <ContextPill />}` — `ContextPill` internally renders the switcher).
- `web/src/components/layout/workspace-tree-item.tsx` — extract the inline `+added/−deleted` count formatting into a tiny shared helper so the switcher and the tree render counts identically (DRY). New helper: `web/src/components/layout/format-change-count.ts` exporting `formatChangeCount(n: number): string` (e.g. `1234 → "1k"`, `69 → "69"`). Update the tree item to use it; the switcher uses it too.

### Data sources (all existing)

- `useSidebarStore((s) => s.repos)` — repos with nested workspaces.
- Active workspace id — from the route `pathname` (`/workspaces/([^/]+)/`), same as the pill.
- `useNavigate()` from `@tanstack/react-router` — `navigate({ to: '/workspaces/$wsId', params: { wsId } })` (copied from `workspace-tree.tsx`).

### Command menu wiring

Mirror the established pattern in `web/src/features/command-palette/components/command-palette.tsx`:
- `CommandInput` `onChange` → local `query` state.
- Filter items with `filterWorkspaces(items, query)`.
- Render a `CommandItem` per filtered item; `onClick` selects.
- `CommandEmpty` when the filtered list is empty.
- The inline `Command` (always-open `Autocomplete`) handles ↑↓ highlight and Enter over the rendered items.

The `Command` menu is placed inside `PopoverPopup` (anchored under the pill, `side="bottom" align="start"`). `Popover` provides open/close, Esc, outside-click, and focus management.

### Styling

Uses only the coss/shadcn theme + existing tokens — `Popover`, `Command*`, `WorkspaceBranchIcon`, and the same green/red count colors the tree uses. No hardcoded colors; theme radius/tokens only. Popover width matches the sidebar/pill width so it reads as an extension of the pill.

## Testing

Tests in `web/src/__tests__/` mirroring `web/src/`, `@/` imports.

- `web/src/__tests__/components/layout/workspace-switcher-model.test.ts`
  - `flattenWorkspaces`: flattens multiple repos; attaches `repoName`; sets `isCurrent` for the active id and false otherwise; defaults missing `status` to `'new'`; carries `added`/`deleted`.
  - `filterWorkspaces`: matches on repo name; matches on branch; case/accent-insensitive (via `matchesSearchQuery`); empty query returns all; no match returns `[]`.
- `web/src/__tests__/components/layout/format-change-count.test.ts`
  - `formatChangeCount`: `69 → "69"`, `1234 → "1k"`, boundaries (`999 → "999"`, `1000 → "1k"`).
- `web/src/__tests__/components/layout/workspace-switcher.test.tsx`
  - renders a row per workspace (`repo / branch` text present).
  - typing in the input narrows the rendered rows.
  - selecting a row calls `navigate` with `{ to: '/workspaces/$wsId', params: { wsId } }` for that workspace.
  - the current workspace row is marked.
  - (router `useNavigate`/`useRouterState` mocked; stores seeded via `setState`, mirroring the existing context-pill test.)

## Live verification (Tauri, required)

Per project rule (verify in the running app, not just tests): confirm in Tauri that —
- clicking the pill opens the popover anchored under it;
- typing filters; ↑↓ + Enter and click both navigate to the right workspace;
- the main content/route changes but the **sidebar tab stays put**;
- Esc/outside-click closes without navigating;
- current workspace is marked; counts render like the tree;
- works from a non-Workspaces tab (Files/Git) — the whole point.

## Out of scope (YAGNI for v1)

- No global keyboard shortcut to open it (can add later).
- No chats / files / recents / "create workspace" entries — workspaces only.
- No grouping by repo (flat list chosen).
- No fuzzy/Levenshtein scoring beyond the existing `matchesSearchQuery` substring matcher.

## Consistency commitment

Reuses the vendored coss `Command` component and existing tokens exclusively. The switcher must read as a native part of the app — same command-menu look as the global palette, same workspace iconography/counts as the tree.
