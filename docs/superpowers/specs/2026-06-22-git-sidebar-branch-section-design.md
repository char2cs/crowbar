# Git Sidebar — Unified Branch Section Design

**Date:** 2026-06-22
**Status:** Approved (brainstorm), pending spec review → plan

## Goal

Replace the conflicting "commit panel **or** merge section" either/or at the bottom of the Git sidebar's **Changes** tab with a single, parent-anchored section whose **primary action reflects the branch's current state** — commit when dirty, merge when clean — so the path forward (commit → push → merge) is always coherent and the "merge into parent" action never silently disappears.

## Architecture

One **`BranchSection`** pinned at the bottom of the Changes tab, always showing `branch → parent` and a state-driven primary button. Committing happens in a focused **modal** (`CommitDialog`); merging happens via a **popover** (`MergePopover`) on the merge button. Push/Pull are secondary actions. All state is derived from existing sources (git status, review diff, sidebar workspace record); all mutations use existing APIs.

## Background — what exists today

- `git-panel.tsx` (the Changes tab) renders, at the bottom, **either** `GitCommitPanel` **or** `MergeSection`, never both — gated on `uncommittedCount > 0 || !(wsId && parentBranch)`.
- `GitCommitPanel`: always-visible commit `Textarea` + "N files staged" + a `Commit` button (disabled unless message non-empty **and** ≥1 staged) + ahead/behind push/pull icon-buttons.
- `MergeSection`: `Merge/Squash/Rebase` selector + "Merge into {parent}" (or protected/conflict variants). **Hidden whenever there are uncommitted changes.**
- `resolveMergeState({canMergeLocally, hasUncommitted, status})` already returns an `'uncommitted'` state ("Commit your changes before merging.") that **is never rendered** — confirming the original intent to keep merge visible-but-gated.
- Data: `uncommittedCount = files.filter(f => f.uncommitted).length` (`useReviewDiff`); `gitStatus.ahead/behind/files[].staged` (`useGitStore`); `branch`, `parentBranch`, `canMergeLocally`, `status` (sidebar workspace record).
- APIs (all present): `commitChanges(wsId, message)` (commits staged), `stageFile/unstageFile/stageAllFiles/unstageAllFiles(wsId, …)`, `pushChanges/pullChanges(wsId)`, `mergeIntoParent(wsId, strategy)`, `setMergeStrategy(wsId, strategy)`.

## Components

### `BranchSection` (new) — replaces the git-panel either/or block

Always renders:
- **Header:** `⎇ {branch} → {parent}` (monospace branch names; omit `→ {parent}` when there is no parent).
- **Status line** (muted): one line describing the state (see state machine).
- **Primary button** + optional **secondary** (push/pull), per the state machine.

The component derives a single `BranchAction` from the inputs via a pure helper (`resolveBranchAction`, below) and renders accordingly. Loading/in-flight states disable the relevant button and show a progress label ("Committing…", "Merging…", "Pushing…").

### `resolveBranchAction` (new pure function) — the state machine

Input: `{ hasUncommitted, canMergeLocally, status, ahead, behind, hasParent }`.
Precedence (highest → lowest) for the **primary** action:

| # | Condition | Primary button | Status line |
|---|-----------|----------------|-------------|
| 1 | `hasUncommitted` | **Commit changes** → opens `CommitDialog` | "N uncommitted changes" |
| 2 | clean & `status === 'pr-conflicts'` | **Resolve conflicts** (destructive) | "⚠ Merge conflicts" |
| 3 | clean & `hasParent` & `!canMergeLocally` | **Open a pull request ↗** | "{parent} is protected" |
| 4 | clean & `hasParent` & `canMergeLocally` | **Merge into {parent} ▾** → opens `MergePopover` | "Clean · {ahead} to push" / "Clean · up to date" |
| 5 | clean & `!hasParent` | *(no merge/PR primary)* — Push/Pull only | "Clean · {ahead} to push" / "Up to date" |

**Secondary action** (shown alongside the primary when clean, and for state 5 as the only action):
- `ahead > 0` → **↑ Push**
- `behind > 0` → **↓ Pull**
- diverged (`ahead > 0 && behind > 0`) → show **Pull** (resolve incoming first); Push remains available after.

This is the **commit precedence reused from `resolveMergeState`** (conflict > protected > uncommitted > eligible), extended with the ahead/behind secondary. `resolveBranchAction` supersedes `resolveMergeState`'s consumption in the section (keep `resolveMergeState` if other callers exist; otherwise fold it in).

### `CommitDialog` (new) — the commit modal

Triggered by **Commit changes**. A CrossUI `Dialog`:
- **Message** `Textarea` (autofocus). Subject/body split on blank line, matching `commitChanges`.
- **File list:** every uncommitted file with a checkbox; **default all checked**. Toggling stages/unstages selection within the dialog (local state until commit).
- **Footer:** "⌘↵ to commit" hint · `Cancel` · `Commit` (disabled until message non-empty **and** ≥1 file checked).
- **On commit:** stage the checked paths + unstage the unchecked (batched: `stage {paths:[…checked]}`, `unstage {paths:[…unchecked]}` via the existing `gitPost` helper), then `commitChanges(wsId, message)`; on success close the dialog, clear state, and `dispatchEvent(new Event('git-status-changed'))` so the section + review diff refresh. On failure: inline error in the dialog, keep it open.

### `MergePopover` (new) — the merge strategy popover

Triggered by **Merge into {parent} ▾**. A CrossUI `Popover` anchored to the button:
- **Title:** "Merge into {parent}" + a muted note (the merge-state reason, e.g. "{parent} is local & unprotected").
- **Strategy radio list:** *Create a merge commit* (default) / *Squash and merge* / *Rebase and merge* — each with a one-line description. Selection seeds from `branchReview.mergeStrategy`.
- **Confirm button — relabels per strategy:** "Create merge commit" / "Squash & merge" / "Rebase & merge".
- **Remember the choice:** on select, persist via `setMergeStrategy(wsId, strategy)` (optimistic, rollback on failure — the existing `MergeStrategySelector` pattern), so the default carries to next time.
- **On confirm:** `mergeIntoParent(wsId, strategy)`; toast "Merging…"; close popover; `git-status-changed`.

### Edge cases
- **Protected parent** → primary is **Open a pull request ↗** (no popover). v1: keep the existing minimal behavior — if a PR URL is available from the workspace/provider, open it; otherwise an informational/disabled button. (No PR-creation flow built here — YAGNI.)
- **Conflict** → **Resolve conflicts** keeps the existing placeholder toast ("Open the conflicting files and resolve conflicts, then commit.") until a real resolver exists.
- **No parent** → commit + push/pull only; never a merge/PR button.
- **In flight** → disable + progress label; re-enable on settle.

## Files

- **Create:** `web/src/features/git/components/branch-section.tsx`, `commit-dialog.tsx`, `merge-popover.tsx`, and `web/src/features/git/lib/branch-action.ts` (the `resolveBranchAction` pure helper).
- **Modify:** `web/src/features/git/components/git-panel.tsx` — replace the `GitCommitPanel`/`MergeSection` either/or block with `<BranchSection wsId=… />`, passing the data it already gathers (gitStatus, activeWs, uncommittedCount).
- **Retire:** `git-commit-panel.tsx` and `merge-section.tsx` (and `merge-section-state.ts` if fully subsumed) — only after confirming `git-panel` is their sole consumer.
- **Reuse unchanged:** the staging/commit/push/pull/merge APIs; `branchReview.mergeStrategy` state.
- Tests live in `web/src/__tests__/features/git/...` mirroring the above.

## Data flow

```
BranchSection
  reads → gitStatus (ahead, behind, staged, files)        [useGitStore]
        → uncommittedCount, files                          [useReviewDiff]
        → branch, parentBranch, canMergeLocally, status    [sidebar ws record]
        → branchReview.mergeStrategy                        [workspace store]
  derives → resolveBranchAction(...) → which primary/secondary to render
  Commit  → CommitDialog → stage/unstage(paths) + commitChanges → git-status-changed
  Merge   → MergePopover → setMergeStrategy + mergeIntoParent → git-status-changed
  Push/Pull → pushChanges / pullChanges → git-status-changed
```

`git-status-changed` is the single refresh signal (already wired into `useReviewDiff` and the working-tree refresh).

## Error handling

- Commit/merge/push/pull failures surface a toast (and, for commit, an inline error in the dialog) and leave the user's input intact. No optimistic mutation except the merge-strategy default (which rolls back on failure, matching today).

## Testing

- **`resolveBranchAction`** (pure): a case per state row + precedence (conflict beats protected beats uncommitted; ahead/behind secondary; no-parent path). The highest-value unit test.
- **`CommitDialog`**: renders message + file checkboxes; `Commit` disabled until message + ≥1 file; commit stages the checked set and calls `commitChanges`; failure keeps the dialog open with an error.
- **`MergePopover`**: strategy selection persists via `setMergeStrategy`; confirm relabels per strategy and calls `mergeIntoParent` with the chosen strategy.
- **`BranchSection`**: renders the correct primary/secondary for each state (dirty / clean-ahead / clean-behind / protected / conflict / no-parent).
- Live verification in the Tauri app per the standing rule.

## Out of scope (YAGNI)

- A real merge-conflict resolver (keep the toast placeholder).
- A PR-creation flow for protected parents (keep "Open a pull request" minimal).
- Hunk-level staging in the commit dialog (file-level only for v1; `stageHunk` exists but is not surfaced here).

## Open decisions (resolved)

- Commit message: **modal**, not inline (per design discussion — keeps the sidebar compact; `⌘↵` keeps it fast).
- Merge confirm: **relabels per strategy**; chosen strategy is **remembered** as the default.
- Primary once committed-but-unpushed: **Merge into {parent}** is primary; **Push** is the secondary (you can merge locally before pushing).
