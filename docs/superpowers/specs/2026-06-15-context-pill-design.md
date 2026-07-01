# Context Pill — Design

**Date:** 2026-06-15
**Status:** Approved (pending spec review)

## Problem

While navigating Crowbar's interface, it's hard to tell **which workspace and repo you're currently in**. There's no persistent, always-visible "you are here" indicator in the sidebar.

## Solution

A Zen-browser-style **context pill** placed directly on top of the sidebar tab bar (Workspaces / Chats / Files / Git). It passively displays the current location, and clicking it navigates the sidebar to the Workspaces page — nothing else.

### What it displays

A single line, plain text, no avatars or repo logos:

```
[WORKSPACE_ACTION]  reponame/branchname
```

- **`WORKSPACE_ACTION`** — the existing `WorkspaceBranchIcon` (the status indicator already shown beside each workspace: `agent-running` spinner, `new`, `pr-open`, `pr-closed`, `pr-merged`, `locked`). **Reused as-is, not rebuilt.**
- **`reponame`** — muted/grey (`text-muted-foreground`).
- **`/`** — muted separator.
- **`branchname`** — bright + semibold (`text-foreground`, `font-semibold`).

### Display states (discriminated model)

The pill renders one of three states derived from existing app state:

1. **`workspace`** — an active workspace is resolvable from the route. Show `[WorkspaceBranchIcon status] reponame/branchname`.
2. **`project`** — no active workspace, but a current project exists. Show the **project name** as plain text, no status icon.
3. **`empty`** — nothing resolvable. Render nothing (defensive; should be rare).

## Architecture

### New files

- `web/src/components/layout/context-pill.tsx` — `ContextPill` presentational component. Reads the derived model, renders the pill, handles click. Built from shadcn `Button` (`variant="ghost"`) with sizing overrides so it remains a real, accessible button. Styling uses only existing CSS-variable tokens — no new visual language.
- `web/src/components/layout/context-pill-model.ts` — a **pure function** `deriveContextPillModel(args)` that maps inputs (active workspace id, repos, active project) to the discriminated model. Pure so it is unit-testable without rendering.

### Data sources (all existing)

- **Active workspace id** — from the route `pathname` (match `/workspaces/([^/]+)/`), the same pattern `ide-shell.tsx` already uses.
- **Repo + workspace** — from `useSidebarStore` (`repos`), finding the repo whose `workspaces` contains the active id; `workspace.branch` and `workspace.status ?? 'new'`, `repo.name`.
- **Active project** — from `useProjectStore` (`projects`, `activeProjectId`); the active project's `name`.

The component subscribes to stores via narrow selectors (per CLAUDE.md: `useXxxStore((s) => s.field)`), passes the plain values into `deriveContextPillModel`, and renders the result.

### Styling (Compact — locked choice)

- Height ~34px, `px-3`, `rounded-[10px]`.
- Translucent fill via tokens (e.g. `bg-white/5`, hover `bg-white/10`) consistent with existing sidebar surfaces; final token classes matched to neighbouring components during implementation so it blends in.
- Font ~13px (`text-[13px]`), single line, `truncate` (ellipsis) so long repo/branch names don't overflow the sidebar width.
- Icon + text gap matches existing workspace rows.

### Placement & behavior

- In `web/src/components/layout/ide-shell.tsx`, render `<ContextPill />` immediately **before** `<SidebarTabBar />`, inside the existing `!hasNavScreen` block — so the pill appears and hides together with the tab bar (hidden during nav-screen overlays).
- **Click** → `useSidebarStore.getState().setActiveTab('workspaces')` (called from the click handler per the store-usage convention). No other side effects.

## Testing

Tests live in `web/src/__tests__/` mirroring `web/src/` (per CLAUDE.md), using `@/` imports.

- `web/src/__tests__/components/layout/context-pill-model.test.ts`
  - workspace mode: resolves status / repo / branch from inputs.
  - project fallback: no active workspace → project name, no status.
  - empty: nothing resolvable → empty model.
  - status fallback: missing `workspace.status` → `'new'`.
- `web/src/__tests__/components/layout/context-pill.test.tsx`
  - renders `reponame/branchname` text in workspace mode.
  - renders project name in project mode.
  - clicking the pill calls `setActiveTab('workspaces')`.

## Out of scope (YAGNI)

- No editing/typing in the pill (it is not a real search input — display + single click only).
- No command palette, no search results, no keyboard shortcut (can be added later if desired).
- No new icons or status states — strictly the existing `WorkspaceBranchIcon`.
- No changes to the tab bar, project header, or sidebar nav.

## Consistency commitment

Uses the established shadcn/ui theme and CSS-variable tokens exclusively. The pill must read as a native part of the existing sidebar — no hardcoded colors, no bespoke components, no deviation from the current design language.
