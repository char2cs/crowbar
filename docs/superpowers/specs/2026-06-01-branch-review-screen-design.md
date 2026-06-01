# Branch Review Screen — Design Spec

**Date:** 2026-06-01 (updated after implementation design session)
**Feature:** `branchReview` pane content type — the PR/merge screen for Crowbar workspaces.

---

## Overview

Every workspace gets a permanent, uncloseable **Branch Review** tab that surfaces the PR/merge workflow directly inside Crowbar. It mirrors GitHub's PR UI — diff, commits, description, code review threads — but lives natively in the editor as a first-class pane.

---

## Layout & Shell

### Pane tab behaviour
- The branch review tab is a **regular pane buffer** (`PaneContentType: "branchReview"`). It is **not** pinned — pinning prevents dragging, which is allowed here.
- The tab renders normally in the tab bar (draggable, moveable between pane groups) but **has no close button** — the × is suppressed via `buffer.isUncloseable === true`.
- The tab label is the branch name (e.g. `feature/api-backend`), prefixed with a git-branch icon and a coloured status dot.

### Frame structure (CrossUI)
The screen uses the existing local `Frame` / `FramePanel` components from `components/ui/frame.tsx`:

```
Frame (rounded outer shell, bg: muted/72)
├── Header area — bare on frame bg, no FramePanel card
│   ├── Branch name + "→ parent" badge
│   ├── Commit-ahead count + additions/deletions summary
│   └── Split merge button (right-aligned)
├── Tabs switcher (CrossUI Tabs component, default variant)
│   └── About | Commits | Diff
└── Content area — single FramePanel card
    └── Rounded top corners, flush to frame sides and bottom (no side/bottom gaps)
```

The header sits **bare** on the frame background — no card border or dark background. Only the content area below the switcher gets the `FramePanel` dark card treatment (`bg-background` with border, `border-radius: 12px 12px 0 0`, flush sides and bottom).

### Subtab switcher
Uses `<Tabs>` / `<TabsList variant="default">` / `<TabsTab>` from `components/ui/tabs.tsx` (CrossUI Tabs, backed by `@base-ui/react/tabs`). The `default` variant gives the pill-style sliding indicator look. The active subtab is stored in `branchReviewSlice.activeSubtab` and synced to the `Tabs` component's `value` prop.

---

## Merge Button

A **split button** (CrossUI `Group` component with two `Button` children):

| State | Appearance | Behaviour |
|---|---|---|
| Normal | Green "Merge commit" + chevron | Left: executes merge with selected strategy. Right: dropdown. |
| Locked parent | Gray, disabled | Tooltip: "Cannot merge into a locked branch." No action. |
| Has conflicts | Red/destructive, disabled | Tooltip: "Branch has conflicts with parent." No action. |

**Merge strategies** (dropdown, right side of split button):
- Merge commit
- Squash and merge
- Rebase and merge

The selected strategy is stored in `branchReviewSlice` and persists (IndexDB).

---

## Subtabs

1. **About** — Description + Conversations
2. **Commits** — Commit history list
3. **Diff** — Bespoke diff view with inline review threads

---

## About Tab

Single `FramePanel` card, two sections divided by a 1px separator:

### Description
- A **CodeMirror** instance (`@uiw/react-codemirror`, already in `package.json`) with markdown language support and live syntax highlighting.
- Always in edit mode — no view/edit toggle.
- Content stored in `branchReviewSlice.description` and auto-saved on every change.
- Persisted to IndexDB.
- Rendered inside a darker inset box (`bg: #0d0d0d`, rounded, bordered) within the FramePanel.

### Conversations
- A list of all past and active **chat sessions** in this workspace, sourced from `useSidebarStore`.
- Each row: status dot (green = active, gray = closed) · conversation title · relative timestamp.
- Clicking a row opens that conversation (focuses the crowbarChat pane for that session).
- Read-only list — no reply box.

---

## Commits Tab

Scrollable list of `GitCommit` objects for commits on this branch. Uses the existing `getGitLog` API from `git-commits-api.ts`. Mock data returned in web mode.

Each row inside the FramePanel card: short hash (monospace, muted) · commit message · relative date (right-aligned).

---

## Diff Tab

### Bespoke diff viewer

The diff tab uses a **purpose-built diff viewer** (`branch-review-diff-viewer.tsx`) — separate from the existing `GitDiffMultiFileViewer`. This avoids coupling the generic diff viewer to branch-review-specific thread logic.

### Mock diff data
Diff data is **seeded mock data** for now. A `getMockBranchDiff(wsId)` function returns a `MultiFileDiff` object. Real git diff computation is out of scope.

### Performance — Hybrid virtualisation (Strategy C)

The viewer virtualises at **two levels**:

1. **File level** (always): Each file section is a virtual row managed by `@tanstack/react-virtual`. Uses `measureElement` for accurate heights since files expand/collapse.

2. **Line level** (for large files only): Files with > 200 changed lines switch to a nested virtualiser for their line rows. Files below the threshold render lines in a flat list. This keeps small files cheap and large files safe.

Thread components are **variable height** and injected inline after their anchored line. `measureElement` handles remeasurement when threads expand (new replies) or resolve (collapse).

### Thread creation UX
- A **`+` button** appears on hover over any diff line number (left gutter). Uses the existing `Button` primitive from `components/ui/button.tsx`.
- Clicking the `+` (or the line number itself) opens an inline reply box below that line, creating a new `ReviewThread` anchored to `lineNumber`.
- The new thread is immediately added to `branchReviewSlice.threads` and rendered open.

### Inline review threads
Each diff line can have one or more `ReviewThread` objects anchored to it by `lineNumber`.

**Thread data model:**
```ts
interface ReviewThread {
  id: string
  filePath: string
  lineNumber: number        // new_line_number of the anchored diff line
  side: 'left' | 'right'
  messages: ReviewMessage[]
  isResolved: boolean
}

interface ReviewMessage {
  id: string
  author: string | null     // agent name, or null = current user ("You")
  isAgent: boolean
  body: string              // markdown
  createdAt: string
}
```

**Thread display:**
- Always **open by default** (not collapsed).
- User messages show "You". Agent messages show the agent name + a small "agent" badge.
- Reply input at the bottom of each thread. Enter to submit.
- Resolved threads show dimmed + a "Collapse" link; collapsed resolved threads show "Thread resolved / Show".
- Agents post threads programmatically via `branchReviewSlice.addReviewThread`.

---

## Data Model — `branchReviewSlice`

New slice in `features/workspace/stores/slices/branch-review-slice.ts`:

```ts
export type MergeStrategy = 'merge' | 'squash' | 'rebase'

interface BranchReviewState {
  description: string
  mergeStrategy: MergeStrategy
  activeSubtab: 'about' | 'commits' | 'diff'
  diffCache: MultiFileDiff | null
  diffStatus: 'idle' | 'loading' | 'loaded' | 'error'
  threads: ReviewThread[]
}
```

Actions: `setBranchReviewDescription`, `setBranchReviewMergeStrategy`, `setBranchReviewSubtab`, `setBranchReviewDiff`, `setBranchReviewDiffStatus`, `addReviewThread`, `addReviewMessage`, `resolveReviewThread`.

---

## IndexDB Persistence

`branchReview` is added to `WorkspaceSnapshot` with explicit field selection:

| Field | Persist | Reason |
|---|---|---|
| `description` | ✅ | User notes — must survive refresh |
| `mergeStrategy` | ✅ | User preference |
| `threads` | ✅ | Code review comments — must not be lost |
| `activeSubtab` | ✅ | Nice UX — returns to last-viewed tab |
| `diffCache` | ❌ | Recomputable; large; reset to `null` on load |
| `diffStatus` | ❌ | Always reset to `'idle'` on load |

In `workspace-persistence.ts`, `branchReview` is snapshotted as:
```ts
branchReview: {
  description: state.branchReview.description,
  mergeStrategy: state.branchReview.mergeStrategy,
  activeSubtab: state.branchReview.activeSubtab,
  threads: state.branchReview.threads,
  diffCache: null,
  diffStatus: 'idle',
}
```

---

## Conflict detection (mock)

Add `hasConflicts?: boolean` to the `Workspace` interface in `lib/store/sidebar.ts`. Set `hasConflicts: true` on a couple of mock branches (`ws5`, `qc3`) to exercise the blocked merge button state.

---

## `PaneContentType` addition

Add `isUncloseable?: boolean` to `PaneContentBase`, then:

```ts
export interface BranchReviewContent extends PaneContentBase {
  type: "branchReview";
  wsId: string;
  branchName: string;
  isUncloseable: true;
}
```

The tab bar suppresses the close button when `buffer.isUncloseable === true`.

Add `"branchReview"` to `VIRTUAL_TYPES` and to `OpenContentSpec`.

---

## New files

| Path | Purpose |
|---|---|
| `features/branch-review/types/review-types.ts` | `ReviewThread`, `ReviewMessage`, `MergeStrategy` |
| `features/branch-review/components/branch-review-pane.tsx` | Top-level pane: Frame shell + Tabs routing |
| `features/branch-review/components/branch-review-header.tsx` | Header: branch name, stats, split merge button |
| `features/branch-review/components/merge-button.tsx` | Split button with strategy dropdown |
| `features/branch-review/components/about-tab.tsx` | CodeMirror description + conversations list |
| `features/branch-review/components/commits-tab.tsx` | Commit history list |
| `features/branch-review/components/diff-tab.tsx` | Diff tab shell: loads data, renders viewer |
| `features/branch-review/components/branch-review-diff-viewer.tsx` | Bespoke diff viewer with hybrid virtualisation |
| `features/branch-review/components/diff-file-section.tsx` | Per-file section (collapsible, virtualised lines for large files) |
| `features/branch-review/components/review-thread.tsx` | Inline thread + reply box |
| `features/workspace/stores/slices/branch-review-slice.ts` | Zustand slice |
| `lib/mock/branch-diff.ts` | `getMockBranchDiff(wsId)` — seeded mock diff data |

---

## Modified files

| Path | Change |
|---|---|
| `features/panes/types/pane-content.ts` | Add `isUncloseable` to base, `BranchReviewContent` type + guard |
| `features/tabs/components/tab-bar-item.tsx` | Suppress × for `isUncloseable` |
| `features/workspace/stores/workspace-store.types.ts` | Add `BranchReviewSlice` |
| `features/workspace/stores/workspace-store.ts` | Spread `createBranchReviewSlice` |
| `features/workspace/stores/workspace-persistence.ts` | Snapshot safe `branchReview` fields |
| `features/workspace/stores/hooks/use-workspace-effects.ts` | Auto-open `branchReview` buffer |
| `features/workspace/stores/slices/buffer-slice.ts` | Singleton guard for `branchReview` |
| `features/panes/components/pane-container.tsx` | Add `case "branchReview"` |
| `lib/store/sidebar.ts` | Add `hasConflicts?: boolean` to `Workspace` |
| `lib/mock/workspaces.ts` | Set `hasConflicts: true` on `ws5`, `qc3` |

---

## Out of scope

- Real git diff computation (mock only)
- Real git merge execution (mock only)
- GitHub API integration
- Thread notifications or badges
- Line-level virtualisation in split view (unified view only for v1)
