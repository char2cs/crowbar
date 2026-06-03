# Git Tab Consolidation — Design Spec

**Date:** 2026-06-01  
**Branch:** enhancement/design-language  

---

## Problem

The sidebar Git tab is buggy and architecturally awkward: it duplicates concerns already present in the branch review pane (which already shows commit history) and occupies sidebar real estate that doesn't match the mental model of "everything about this branch lives in the branch review pane."

---

## Decision

Remove the sidebar Git tab. Consolidate all Git operations into a new **Git tab** inside the branch review pane, alongside the existing About and Diff tabs.

---

## Git Tab Layout

### Sticky header area
The commit message textarea and Commit button are always visible at the top of the tab — they do not scroll away. This uses a refactored version of `GitCommitPanel`.

### Scrollable body
Everything below the commit header scrolls as one continuous region:
- Unstaged files
- Staged files
- Commit history (branch-scoped)
- Stashes
- Remotes
- Tags

### Responsive behaviour
A `ResizeObserver` on the pane container drives layout — no CSS media queries (the pane is user-resizable, not viewport-dependent).

| Pane width | Layout |
|---|---|
| ≥ 480 px | **Two columns** — left: sticky commit + scrollable files/stash/remotes/tags; right: scrollable commit history |
| < 480 px | **Stacked** — sticky commit at top; single scrollable column: files → commit history → stash/remotes/tags |

The 480 px threshold is stored as a named constant so it can be tuned easily.

---

## Component Map

### New component: `GitTab`
`web/src/features/branch-review/components/git-tab.tsx`

Owns the responsive layout logic via `ResizeObserver`. Composes existing components — does not re-implement their logic.

**Props:**
```ts
interface GitTabProps {
  wsId: string
  repoPath: string
}
```

### Reused components (no changes to their internals)

| Component | Source | Role in Git tab |
|---|---|---|
| `GitCommitPanel` | `features/git/components/git-commit-panel.tsx` | Sticky commit area |
| `GitStatusPanel` | `features/git/components/status/git-status-panel.tsx` | Unstaged + staged files |
| `CommitsTab` | `features/branch-review/components/commits-tab.tsx` | Commit history |
| `GitStashCommandSurface` | `features/git/components/git-stash-command-surface.tsx` | Stash section |
| `GitRemoteManager` | `features/git/components/git-remote-manager.tsx` | Remotes section |
| `GitTagManager` | `features/git/components/git-tag-manager.tsx` | Tags section |

### Modified: `BranchReviewPane`
`web/src/features/branch-review/components/branch-review-pane.tsx`

Add `"git"` to the tab list between About and Diff. Wire `GitTab` as the tab panel content.

### Removed: sidebar Git tab
`SidebarTabs.tsx` — remove the Git tab entry and its icon. `GitView` becomes unused; delete or keep for future repo-browser use (decision: delete to avoid dead code).

---

## Styling rules

- **No hardcoded colors.** Use CSS variable tokens from `theme.css`: `--background`, `--foreground`, `--muted`, `--muted-foreground`, `--border`, `--primary`, etc.
- **All UI primitives from `@/components/ui/`** — `ScrollArea`, `Separator`, `Tabs`, `TabsList`, `TabsTab`, `TabsPanel`, etc. No raw HTML alternatives.
- The sticky commit header uses `position: sticky; top: 0` within its scroll container, backed by `--background` so it doesn't bleed through scrolling content.

---

## Data flow

`GitTab` receives `repoPath` (needed by git components) and `wsId` (needed by `CommitsTab` which queries via workspace ID). Both are already available in `BranchReviewPane` — pass them down as props.

No new stores. No new API calls. All data fetching stays inside the reused components.

---

## Out of scope

- Worktree manager — stays in the sidebar (it's workspace-level, not branch-level).
- Git blame — unchanged.
- Any changes to the git API layer.

---

## Success criteria

1. Sidebar has no Git tab.
2. Branch review pane has About / Git / Diff tabs.
3. Git tab shows sticky commit area + scrollable files + history + stash + remotes + tags.
4. Layout switches between two-column and stacked at 480 px pane width.
5. No hardcoded colors anywhere in the new component.
6. All components from `@/components/ui/`.
