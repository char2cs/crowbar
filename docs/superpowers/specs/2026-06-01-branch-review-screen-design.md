# Branch Review Screen — Design Spec

**Date:** 2026-06-01  
**Feature:** `branchReview` pane content type — the PR/merge screen for Crowbar workspaces.

---

## Overview

Every workspace gets a permanent, uncloseable **Branch Review** tab that surfaces the PR/merge workflow directly inside Crowbar. It mirrors GitHub's PR UI — diff, commits, description, code review threads — but lives natively in the editor as a first-class pane.

---

## Layout & Shell

### Pane tab behaviour
- The branch review tab is a **regular pane buffer** (`PaneContentType: "branchReview"`). It is **not** pinned — pinning prevents dragging, which is allowed here.
- The tab renders normally in the tab bar (draggable, moveable between pane groups) but **has no close button** — the × is suppressed for this content type via a type check in the tab bar.
- The tab label is the branch name (e.g. `feature/api-backend`), prefixed with a git-branch icon and a coloured status dot.

### Frame structure (CrossUI)
The screen uses the existing local `Frame` / `FramePanel` components from `components/ui/frame.tsx`:

```
Frame (rounded outer shell, bg: muted/72)
├── Header area — bare on frame bg, no FramePanel card
│   ├── Branch name + "→ parent" badge
│   ├── Commit-ahead count + additions/deletions summary
│   └── Split merge button (right-aligned)
├── Group switcher (CrossUI Group component)
│   └── About | Commits | Diff
└── Content area — single FramePanel card
    └── Rounded top corners, flush to frame sides and bottom (no side/bottom gaps)
```

The header and Group switcher sit **bare** on the frame background — no card border or dark background behind them. Only the content area below the switcher gets the `FramePanel` dark card treatment (`bg-background` with border, `border-radius: 12px 12px 0 0`, flush sides and bottom).

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

The selected strategy is stored in `branchReviewSlice` and persists across navigation.

---

## Subtabs (Group switcher)

Uses the CrossUI `Group` component (`components/ui/group.tsx`), horizontal orientation, three segments:

1. **About** — Description + Conversations
2. **Commits** — Commit history list
3. **Diff** — Multi-file diff with inline review threads

---

## About Tab

Single `FramePanel` card, two sections divided by a 1px separator:

### Description
- A **CodeMirror** instance (`@uiw/react-codemirror`, already in `package.json`) with markdown language support and live syntax highlighting.
- Always in edit mode — no view/edit toggle.
- Content is stored in `branchReviewSlice.description: string` and auto-saved to the store on every change.
- Rendered inside a darker inset box (`bg: #0d0d0d`, rounded, bordered) within the FramePanel.

### Conversations
- A list of all past and active **chat sessions** (markdown-chat conversations) that exist in this workspace, sourced from the existing chat/conversation store.
- Each row: status dot (green = active, gray = closed) · conversation title · relative timestamp.
- Clicking a row opens that conversation (navigates to or focuses the crowbarChat pane for that session).
- No reply box, no threading — read-only list.

---

## Commits Tab

A scrollable list of `GitCommit` objects for commits on this branch not yet merged into the parent. Uses the existing `getGitLog` API from `git-commits-api.ts`.

Each row inside the FramePanel card:
- Short hash (monospace, muted)
- Commit message
- Relative date (right-aligned, muted)

---

## Diff Tab

Reuses the existing `GitDiffMultiFileViewer` component (`features/git/components/diff/git-diff-multi-file.tsx`) which already uses `@tanstack/react-virtual` for virtualised rendering.

### Performance
- Diff is computed **lazily on mount** of the Diff tab, not on workspace load.
- Results are cached in `branchReviewSlice.diffCache` — not recomputed on tab re-focus unless the branch has new commits.
- Large diffs (> 500 lines per file) auto-collapse per the existing `LARGE_DIFF_THRESHOLD` constant.
- `contentVisibility: auto` with `containIntrinsicHeight` already applied per file section — no changes needed.

### Inline review threads
Each diff line can have **one or more review threads** anchored to it. A thread is a sequence of messages between agents and the user.

**Thread data model:**
```ts
interface ReviewThread {
  id: string
  filePath: string
  lineNumber: number        // the new_line_number of the anchored line
  side: 'left' | 'right'   // for split view
  messages: ReviewMessage[]
  isResolved: boolean
}

interface ReviewMessage {
  id: string
  author: string | null     // agent name, or null for the current user ("You")
  isAgent: boolean
  body: string              // markdown
  createdAt: string
}
```

**Thread display:**
- Threads are always **open by default** (not collapsed).
- Rendered inline below the anchored diff line, inside the diff view.
- User messages show "You" (no author label). Agent messages show the agent name + a small "agent" badge.
- Users can reply to any thread via a small text input at the bottom of the thread.
- Agents post threads programmatically via the `branchReviewSlice.addReviewThread` action — this is how automated code reviews surface in the UI.
- Resolved threads can be toggled collapsed.

---

## Data Model — `branchReviewSlice`

New slice added to `WorkspaceState` in `features/workspace/stores/slices/`:

`MergeStrategy` is a named type exported from `review-types.ts`:

```ts
export type MergeStrategy = 'merge' | 'squash' | 'rebase'
```

```ts
interface BranchReviewState {
  description: string
  mergeStrategy: MergeStrategy
  activeSubtab: 'about' | 'commits' | 'diff'
  diffCache: MultiFileDiff | null
  diffStatus: 'idle' | 'loading' | 'loaded' | 'error'
  threads: ReviewThread[]
}

interface BranchReviewSlice {
  branchReview: BranchReviewState
  setBranchReviewDescription: (description: string) => void
  setBranchReviewMergeStrategy: (strategy: MergeStrategy) => void
  setBranchReviewSubtab: (tab: 'about' | 'commits' | 'diff') => void
  setBranchReviewDiff: (diff: MultiFileDiff) => void
  setBranchReviewDiffStatus: (status: BranchReviewState['diffStatus']) => void
  addReviewThread: (thread: ReviewThread) => void
  addReviewMessage: (threadId: string, message: ReviewMessage) => void
  resolveReviewThread: (threadId: string) => void
}
```

---

## Conflict detection (mock)

Add `hasConflicts?: boolean` to the `Workspace` type in `lib/store/sidebar.ts`. A few branches in the mock data have it set to `true` to exercise the blocked merge button state. When real git integration arrives, this field gets populated by actual merge-base conflict detection.

---

## `PaneContentType` addition

Add `"branchReview"` to the `PaneContentType` union in `features/panes/types/pane-content.ts`:

Add `isUncloseable?: boolean` to `PaneContentBase` (the base interface), then:

```ts
export interface BranchReviewContent extends PaneContentBase {
  type: "branchReview"
  wsId: string
  branchName: string
  isUncloseable: true
}
```

The tab bar suppresses the close button when `buffer.isUncloseable === true` (type-safe, works without narrowing to a specific content type).

---

## New files

| Path | Purpose |
|---|---|
| `features/branch-review/` | Feature root |
| `features/branch-review/components/branch-review-pane.tsx` | Top-level pane component (Frame shell, routing subtabs) |
| `features/branch-review/components/branch-review-header.tsx` | Header: branch name, stats, split merge button |
| `features/branch-review/components/about-tab.tsx` | Description (CodeMirror) + Conversations list |
| `features/branch-review/components/commits-tab.tsx` | Commit history list |
| `features/branch-review/components/diff-tab.tsx` | Diff viewer wrapper with thread injection |
| `features/branch-review/components/review-thread.tsx` | Inline thread + reply box |
| `features/branch-review/components/merge-button.tsx` | Split button with strategy dropdown |
| `features/branch-review/types/review-types.ts` | `ReviewThread`, `ReviewMessage`, `MergeStrategy` |
| `features/workspace/stores/slices/branch-review-slice.ts` | Zustand slice |

---

## Modified files

| Path | Change |
|---|---|
| `features/panes/types/pane-content.ts` | Add `BranchReviewContent` type + type guard |
| `features/tabs/components/tab-bar-item.tsx` | Suppress × button for `branchReview` type |
| `lib/store/sidebar.ts` | Add `hasConflicts?: boolean` to `Workspace` |
| `lib/mock/workspaces.ts` | Set `hasConflicts: true` on select mock branches |

---

## Out of scope

- Real git merge execution (mock only for now)
- GitHub API integration for PR creation
- Thread notifications or badges
- Diff comment line anchoring in split view (unified view only for v1)
