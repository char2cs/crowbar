# Git Diff Stack → Monaco DiffEditor

**Date:** 2026-06-01  
**Scope:** Replace the git diff stack's serialized-text + background overlay approach with Monaco's native DiffEditor. Branch-review is explicitly out of scope (kept on Shiki + React components due to inline comment complexity and large diff sizes).

---

## Problem

The current git diff stack (`git-diff-editor-stack.tsx`) renders diffs by:
1. Serializing the `GitDiff` patch into synthetic left/right text strings
2. Loading each into a separate regular `CodeEditor` (Monaco) instance
3. Overlaying `DiffLineBackgroundLayer` to paint diff colors on top

This is a workaround. Monaco has a first-class `DiffEditor` API designed for exactly this use case, which provides: native diff highlighting, synchronized pane scrolling, minimap, fold-unchanged-regions, and proper side-by-side / inline toggle — all without custom background layers.

---

## Decision

Replace the git diff stack with Monaco DiffEditor. Keep branch-review on its current Shiki renderer.

**Why not branch-review:** Inline comments are React components injected between diff lines. Monaco's equivalent (view zones + DOM portals) is significantly more complex, and branch-review diffs can reach 80k+ lines requiring full original+modified file content that would require per-file API calls.

---

## Architecture

### Data flow (unchanged)

The existing `serializeGitDiffSourceForSplitEditor` already produces left/right strings by distributing removed lines to left and added lines to right, with context lines on both sides. These map directly to Monaco DiffEditor's `originalValue` / `modifiedValue`. No new API calls needed.

For unified (inline) view, Monaco DiffEditor supports `renderSideBySide: false`.

### Components affected

| File | Change |
|---|---|
| `git-diff-editor-stack.tsx` | Replace dual `CodeEditor` + background layer with single `MonacoDiffEditorView` |
| `git-diff-editor-surface.tsx` | Replace `CodeEditor` + buffer approach with `MonacoDiffEditorView` |
| `diff-line-background-layer.tsx` | **Delete** — Monaco DiffEditor handles its own coloring |
| `diff-editor-content.ts` | Remove `serializeGitDiffForEditor` (unified patch text, no longer needed); keep `serializeGitDiffSourceForSplitEditor` which produces left/right content |
| `use-diff-editor-buffer.ts` | **Delete** — Monaco DiffEditor manages its own models |

### New component: `MonacoDiffEditorView`

A thin wrapper around Monaco's `createDiffEditor` API:

```
MonacoDiffEditorView
  props:
    originalContent: string      // left (before)
    modifiedContent: string      // right (after)
    language: string             // inferred from file path
    renderSideBySide: boolean    // true = split, false = inline
    readOnly: boolean            // always true for diff views
    height?: number | string
```

Internally:
- Creates two `ITextModel` instances (original + modified)
- Creates a `IDiffEditorConstructionOptions` diff editor
- Updates models when content changes (dispose + recreate to avoid stale state)
- Respects the app's Monaco theme

### View mode toggle

The existing split/unified toggle in `git-diff-editor-stack.tsx` maps to `editor.updateOptions({ renderSideBySide })` — no remount needed when toggling.

---

## What we gain

- Native diff highlighting (no more `DiffLineBackgroundLayer`)
- Synchronized scrolling between panes (built-in)
- Fold unchanged context regions (Monaco's `diffCodeLens` / `hideUnchangedRegions`)
- Consistent with Monaco theme/styling across the entire app
- Simpler codebase: delete ~3 files, remove background overlay logic

## What changes behavior

- Diff colors shift from our custom tailwind tokens (`bg-git-added/12`, etc.) to Monaco's built-in diff theme colors. These should be configured to match via Monaco theme customization (`editor.diffInserted`, `editor.diffRemoved` tokens).
- Hunk headers (`@@ -1,4 +1,7 @@`) currently rendered as styled rows will become Monaco's native fold controls.

---

## Out of scope

- Branch-review diff view (Shiki renderer + inline comments — separate effort)
- Monaco DiffEditor for image diffs (`git-diff-image.tsx` unchanged)
- Editing capability in diff view (always read-only)
