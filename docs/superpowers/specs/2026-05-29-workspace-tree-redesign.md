# Workspace Tree Redesign

**Date:** 2026-05-29  
**Branch:** `enhancement/design-language`  
**Scope:** Replace the flat workspace list in the sidebar with a Zen Browser-style recursive tree, Coss UI button styling, and branch-state icons.

---

## Overview

Crowbar's sidebar currently renders workspaces as a flat numbered list under each repo. This redesign introduces:

- **Recursive tree** — workspaces nest under other workspaces (any depth), manually organised via `parentId`
- **Branch state icons** — 6 states communicate PR/protection/agent status at a glance
- **Coss UI token-based button styles** — `default` (active) and `ghost` (inactive) variants using design tokens, not hardcoded colours
- **cli-loaders spinner** — animated braille spinner on agent-running workspaces, random variant per mount
- **No drag-and-drop** — deferred; data model is designed to support it later

---

## Data Model

### Changes to `web/src/lib/store/sidebar.ts`

Add `WorkspaceStatus` type and two new fields to `Workspace`. Remove `num`.

```ts
export type WorkspaceStatus =
  | 'locked'       // protected branch — needs external PR to merge into it
  | 'new'          // branch exists, no PR open
  | 'pr-open'      // PR open (green)
  | 'pr-closed'    // PR closed without merging (red)
  | 'pr-merged'    // PR merged (purple)
  | 'agent-running' // an agent is actively running in this workspace

export interface Workspace {
  id: string
  branch: string
  parentId?: string          // undefined = top-level under repo; points to another Workspace.id
  status?: WorkspaceStatus   // defaults to 'new' when absent
  added?: number
  deleted?: number
  age: string
  // `num` removed — no longer displayed
}
```

`Repo` is unchanged. `workspaces[]` stays flat — the tree is derived at render time.

### Updated mock data in `INITIAL_REPOS`

The mock data must demonstrate all 6 statuses and at least 3 levels of nesting:

```
crowbar
  develop          (locked)          ← top-level
    feature/app-design  (pr-open)    ← parentId: 'ws-develop'
      enhancement/scaffold (agent-running, ACTIVE) ← parentId: 'ws-app-design'
      fix/toolbar-crash    (new)               ← parentId: 'ws-app-design'
    feature/api-backend  (pr-merged)           ← parentId: 'ws-develop'

quiver.core
  develop          (locked)
    feature/old-auth  (pr-closed)

quiver.desktop
  develop          (locked)
    feature/quiver-shell (pr-open, +13k -69)
```

---

## New Components

### `web/src/components/layout/workspace-tree.tsx`

Top-level panel. Replaces `WorkspacesSidebarPanel`.

**Responsibilities:**
- Reads `repos` from `useSidebarStore((s) => s.repos)`
- Reads active workspace ID from the URL (`useParams` or equivalent)
- For each repo, calls `buildWorkspaceTree(repo.workspaces)` to get `WorkspaceTreeNode[]`
- Renders the repo header (avatar + name) then a list of root-level `WorkspaceTreeItem` nodes

**Tree builder utility** (co-located in the same file, exported for tests):

```ts
export interface WorkspaceTreeNode {
  workspace: Workspace
  children: WorkspaceTreeNode[]
}

export function buildWorkspaceTree(workspaces: Workspace[]): WorkspaceTreeNode[] {
  // Build a map of id → node, then attach children to parents.
  // Nodes with no parentId, or whose parentId references a non-existent workspace,
  // are treated as root-level.
}
```

---

### `web/src/components/layout/workspace-tree-item.tsx`

Single recursive node. Renders itself, then its children indented.

**Props:**
```ts
interface WorkspaceTreeItemProps {
  node: WorkspaceTreeNode
  depth: number           // 0 = top-level under repo
  activeWorkspaceId: string
}
```

**Visual rules:**

| State | Classes |
|---|---|
| Active | `border-primary bg-primary text-primary-foreground shadow-xs hover:bg-primary/90` |
| Inactive | `border-transparent text-foreground hover:bg-accent` |
| Protected (`locked`) | `border-transparent text-foreground/30 hover:bg-accent` |

Base classes (always applied, adapted from Coss UI `xs` size):
```
relative inline-flex w-full shrink-0 cursor-pointer items-center gap-1.5 rounded-lg border
px-2 py-1 text-xs font-medium outline-none transition-shadow
focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background
disabled:pointer-events-none disabled:opacity-64
```

**Layout of a single row:**
```
[WorkspaceBranchIcon]  [branch name, flex-1, truncate]  [+added -deleted, only when active]  [chevron, only when has children]
```

**Indentation:** `paddingLeft: depth * 14 + 8` px (inline style — cannot be done with static Tailwind classes for arbitrary depth).

**Collapse/expand:** Each node tracks `expanded` in local `useState`, defaulting to `true`. Clicking the chevron toggles it. Clicking the branch name navigates to the workspace (does not toggle).

**Agent spinner:** When `status === 'agent-running'`, the icon slot renders `<WorkspaceAgentSpinner />` instead of `<WorkspaceBranchIcon />`.

---

### `web/src/components/layout/workspace-branch-icon.tsx`

Pure presentational — no state, no logic beyond a switch.

```ts
interface WorkspaceBranchIconProps {
  status: WorkspaceStatus
}
```

| Status | Icon | Colour token |
|---|---|---|
| `locked` | Lock SVG | `text-foreground/30` |
| `new` | Git branch SVG | `text-foreground/30` |
| `pr-open` | Git merge SVG | `text-green-500` |
| `pr-closed` | X SVG | `text-red-500` |
| `pr-merged` | Merge commit SVG | `text-purple-500` |
| `agent-running` | `<WorkspaceAgentSpinner />` | `text-violet-400` |

All SVGs are inline (no icon library dependency). Size: `w-2.5 h-2.5 shrink-0`.

**`WorkspaceAgentSpinner`** (defined in the same file):
- On mount, picks one random spinner name from `cli-loaders`'s exported list and stores it in `useState` — the variant does not change on re-render.
- Renders that spinner at `text-violet-400`.

---

## Files Deleted

| File | Reason |
|---|---|
| `web/src/components/layout/WorkspacesSidebarPanel.tsx` | Replaced by `workspace-tree.tsx` |
| `web/src/components/layout/SidebarRow.tsx` (workspace parts) | Replaced by `workspace-tree-item.tsx`; verify nothing else imports it before deleting |

The import site (wherever `WorkspacesSidebarPanel` is used in `IDEShell.tsx` or `SidebarTabs.tsx`) is updated to point to `WorkspaceTree`.

---

## Dependency

```
bun add cli-loaders
```

`cli-loaders` is a zero-dependency React package. Verify it is already in `package.json` before adding.

---

## Tests

File: `web/src/__tests__/components/layout/workspace-tree.test.ts`

**`buildWorkspaceTree` unit tests:**
- Flat list (no parentIds) → all nodes at root
- Single level of nesting → correct parent/child attachment
- Orphaned `parentId` (references non-existent workspace) → treated as root
- Circular reference guard (A is parent of B, B is parent of A) → both treated as root

**`WorkspaceBranchIcon` tests:**
- Snapshot per status value (6 snapshots)
- `agent-running` renders a spinner element

---

## QA Phase — Chrome DevTools MCP

After implementation, verify the running app with the Chrome MCP before marking complete.

**Steps:**

1. Start the dev server (`bun dev` in `web/`)
2. Navigate to `http://localhost:5173` (or whatever port)
3. **Screenshot the sidebar** — confirm the recursive tree renders with the correct repo headers
4. **Verify all 6 branch states** — check the mock data covers all statuses; confirm icons and colours match the spec table above
5. **Active workspace** — confirm `enhancement/scaffold` shows the filled `default` button style and `+22k` diff stat
6. **Inactive workspaces** — confirm all others render ghost style (no background)
7. **Protected branch** — confirm `develop` is visually dimmed and its lock icon is present
8. **Nesting depth** — confirm `enhancement/scaffold` is indented two levels under `crowbar → develop → feature/app-design`
9. **Agent spinner** — confirm the braille spinner is animating on the active `agent-running` workspace
10. **Collapse/expand** — click the chevron on `feature/app-design`; confirm children collapse and expand
11. **Workspace navigation** — click an inactive workspace; confirm the URL changes and the clicked item becomes active (filled style, diff stats appear)
12. **Screenshot final state** — attach to PR description

Any visual discrepancy found in steps 3–12 must be fixed before the spec is considered implemented.

---

## Out of Scope

- Drag-and-drop reordering (deferred — `parentId` structure supports it)
- Real git/GitHub data for `status` (deferred — mock data only)
- Context menu on workspace rows (deferred)
- The `age` field display (removed from the new design for cleanliness)
