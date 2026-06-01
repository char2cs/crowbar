# Git Diff Stack → Monaco DiffEditor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the git diff stack's serialized-text + custom background-color overlay approach with Monaco's native `createDiffEditor`, so commit diffs and split-view diffs get proper syntax highlighting and native diff coloring without custom plumbing.

**Architecture:** Add `buildMonacoDiffContent` to extract clean original/modified strings from a GitDiff (no spacers — Monaco computes its own diff). Wrap `createDiffEditor` in a new `MonacoDiffEditorView` component (one instance per file, created on mount, disposed on unmount). Replace `EmbeddedDiffSectionEditor` and `LargeDiffSectionEditor` in `git-diff-editor-stack.tsx`. Delete `DiffLineBackgroundLayer` (Monaco handles its own diff coloring). `GitDiffEditorSurface` and the `TextDiffViewer` hunk-actions path are out of scope.

**Tech Stack:** `monaco-editor` v0.55.1 (already installed), React, TypeScript, Vitest.

---

## File Map

| Action | File | Why |
|---|---|---|
| Modify | `web/src/features/git/utils/diff-editor-content.ts` | Add `buildMonacoDiffContent` |
| Create | `web/src/features/git/components/diff/monaco-diff-editor-view.tsx` | Monaco DiffEditor wrapper |
| Modify | `web/src/features/git/components/diff/git-diff-editor-stack.tsx` | Replace `EmbeddedDiffSectionEditor` + `LargeDiffSectionEditor` |
| Delete | `web/src/features/git/components/diff/diff-line-background-layer.tsx` | No longer needed |
| Create | `web/src/__tests__/features/git/diff-editor-content.test.ts` | Unit tests for new utility |

---

### Task 1: Add `buildMonacoDiffContent` to diff-editor-content.ts

**Files:**
- Modify: `web/src/features/git/utils/diff-editor-content.ts`
- Create: `web/src/__tests__/features/git/diff-editor-content.test.ts`

`buildMonacoDiffContent` strips hunk headers and splits lines into two clean strings — `original` (context + removed) and `modified` (context + added). Monaco's diff algorithm then computes the visual diff. No spacers needed; Monaco handles alignment itself.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/git/diff-editor-content.test.ts`:

```typescript
import { describe, expect, test } from "vitest";
import type { GitDiff, GitDiffLine } from "@/features/git/types/git-types";
import { buildMonacoDiffContent } from "@/features/git/utils/diff-editor-content";

const makeDiff = (lines: Array<{ type: GitDiffLine["line_type"]; content: string }>): GitDiff => ({
  file_path: "src/foo.ts",
  is_new: false,
  is_deleted: false,
  is_renamed: false,
  lines: lines.map((l, i) => ({
    line_type: l.type,
    content: l.content,
    old_line_number: i + 1,
    new_line_number: i + 1,
  })),
});

describe("buildMonacoDiffContent", () => {
  test("context lines appear on both sides", () => {
    const diff = makeDiff([{ type: "context", content: "const x = 1" }]);
    const { original, modified } = buildMonacoDiffContent(diff);
    expect(original).toBe("const x = 1");
    expect(modified).toBe("const x = 1");
  });

  test("removed lines appear in original only", () => {
    const diff = makeDiff([{ type: "removed", content: "old line" }]);
    const { original, modified } = buildMonacoDiffContent(diff);
    expect(original).toBe("old line");
    expect(modified).toBe("");
  });

  test("added lines appear in modified only", () => {
    const diff = makeDiff([{ type: "added", content: "new line" }]);
    const { original, modified } = buildMonacoDiffContent(diff);
    expect(original).toBe("");
    expect(modified).toBe("new line");
  });

  test("header lines are excluded from both sides", () => {
    const diff = makeDiff([
      { type: "header", content: "@@ -1,3 +1,3 @@" },
      { type: "context", content: "kept" },
    ]);
    const { original, modified } = buildMonacoDiffContent(diff);
    expect(original).toBe("kept");
    expect(modified).toBe("kept");
  });

  test("mixed diff produces correct original and modified strings", () => {
    const diff = makeDiff([
      { type: "context", content: "line1" },
      { type: "removed", content: "old" },
      { type: "added", content: "new" },
      { type: "context", content: "line4" },
    ]);
    const { original, modified } = buildMonacoDiffContent(diff);
    expect(original).toBe("line1\nold\nline4");
    expect(modified).toBe("line1\nnew\nline4");
  });

  test("empty diff produces empty strings", () => {
    const diff = makeDiff([]);
    const { original, modified } = buildMonacoDiffContent(diff);
    expect(original).toBe("");
    expect(modified).toBe("");
  });
});
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd web && npx vitest run src/__tests__/features/git/diff-editor-content.test.ts
```

Expected: FAIL — `buildMonacoDiffContent` is not exported from `diff-editor-content.ts`.

- [ ] **Step 3: Add `buildMonacoDiffContent` to diff-editor-content.ts**

Append to the end of `web/src/features/git/utils/diff-editor-content.ts`:

```typescript
export function buildMonacoDiffContent(diff: GitDiff): { original: string; modified: string } {
  const originalLines: string[] = [];
  const modifiedLines: string[] = [];

  for (const line of diff.lines) {
    if (line.line_type === "header") continue;
    if (line.line_type === "context") {
      originalLines.push(line.content);
      modifiedLines.push(line.content);
    } else if (line.line_type === "removed") {
      originalLines.push(line.content);
    } else if (line.line_type === "added") {
      modifiedLines.push(line.content);
    }
  }

  return {
    original: originalLines.join("\n"),
    modified: modifiedLines.join("\n"),
  };
}
```

- [ ] **Step 4: Run test to confirm it passes**

```bash
cd web && npx vitest run src/__tests__/features/git/diff-editor-content.test.ts
```

Expected: 6 tests pass.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/git/utils/diff-editor-content.ts web/src/__tests__/features/git/diff-editor-content.test.ts
git commit -m "feat(git-diff): add buildMonacoDiffContent to extract original/modified strings from GitDiff"
```

---

### Task 2: Create `MonacoDiffEditorView` component

**Files:**
- Create: `web/src/features/git/components/diff/monaco-diff-editor-view.tsx`

Wraps Monaco's imperative `createDiffEditor` API in a React component. The editor is created once per `cacheKey` (which is unique per file in the diff stack) and disposed on unmount. Content changes and `renderSideBySide` toggling happen in separate effects so they don't force a remount. Monaco's theme is global — when other editors in the app call `setTheme`, this editor picks it up automatically.

No unit test: Monaco requires a browser DOM and real WebWorkers; it cannot run in jsdom. Type-check serves as verification.

- [ ] **Step 1: Create `web/src/features/git/components/diff/monaco-diff-editor-view.tsx`**

```tsx
import { memo, useEffect, useRef } from "react";
import { editor as monacoEditor, Uri } from "monaco-editor";
import { detectLanguageFromPath } from "@/features/editor/utils/language-detection";

interface MonacoDiffEditorViewProps {
  originalContent: string;
  modifiedContent: string;
  filePath: string;
  renderSideBySide: boolean;
  height: number | string;
  cacheKey: string;
  scrollable?: boolean;
}

function MonacoDiffEditorViewComponent({
  originalContent,
  modifiedContent,
  filePath,
  renderSideBySide,
  height,
  cacheKey,
  scrollable = false,
}: MonacoDiffEditorViewProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const editorRef = useRef<monacoEditor.IStandaloneDiffEditor | null>(null);
  const language = detectLanguageFromPath(filePath);

  // Create the diff editor once per cacheKey; dispose on unmount or cacheKey change.
  // language and initial content are captured at mount time — cacheKey drives identity.
  useEffect(() => {
    if (!containerRef.current) return;

    const originalModel = monacoEditor.createModel(
      originalContent,
      language,
      Uri.parse(`diff-original://${cacheKey}`),
    );
    const modifiedModel = monacoEditor.createModel(
      modifiedContent,
      language,
      Uri.parse(`diff-modified://${cacheKey}`),
    );

    const diffEditor = monacoEditor.createDiffEditor(containerRef.current, {
      readOnly: true,
      renderSideBySide,
      scrollBeyondLastLine: false,
      minimap: { enabled: false },
      renderOverviewRuler: false,
      scrollbar: {
        vertical: scrollable ? "auto" : "hidden",
        horizontal: "auto",
        alwaysConsumeMouseWheel: scrollable,
      },
      lineNumbers: "on",
    });

    diffEditor.setModel({ original: originalModel, modified: modifiedModel });
    editorRef.current = diffEditor;

    return () => {
      editorRef.current = null;
      diffEditor.dispose();
      originalModel.dispose();
      modifiedModel.dispose();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cacheKey]);

  // Update model content without recreating the editor.
  useEffect(() => {
    const editor = editorRef.current;
    if (!editor) return;
    const models = editor.getModel();
    if (!models) return;
    if (models.original.getValue() !== originalContent) {
      models.original.setValue(originalContent);
    }
    if (models.modified.getValue() !== modifiedContent) {
      models.modified.setValue(modifiedContent);
    }
  }, [originalContent, modifiedContent]);

  // Toggle unified/split without a remount.
  useEffect(() => {
    editorRef.current?.updateOptions({ renderSideBySide });
  }, [renderSideBySide]);

  return (
    <div
      ref={containerRef}
      className="overflow-hidden"
      style={{ height: typeof height === "number" ? `${height}px` : height }}
    />
  );
}

export default memo(MonacoDiffEditorViewComponent);
```

- [ ] **Step 2: Type-check the new file**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "monaco-diff-editor-view"
```

Expected: no output (no errors).

- [ ] **Step 3: Commit**

```bash
git add web/src/features/git/components/diff/monaco-diff-editor-view.tsx
git commit -m "feat(git-diff): add MonacoDiffEditorView wrapping Monaco createDiffEditor"
```

---

### Task 3: Replace `EmbeddedDiffSectionEditor` and `LargeDiffSectionEditor`

**Files:**
- Modify: `web/src/features/git/components/diff/git-diff-editor-stack.tsx`

Both functions are inside `git-diff-editor-stack.tsx`. Replace them with versions that use `MonacoDiffEditorView`. The surrounding `DiffSectionEditor` router and all other code in the file stays unchanged.

- [ ] **Step 1: Update the import block**

Replace lines 1–47 of `web/src/features/git/components/diff/git-diff-editor-stack.tsx` with:

```tsx
import {
  Check,
  CaretDown as ChevronDown,
  CaretRight as ChevronRight,
  Columns as Columns2,
  ArrowSquareOut as ExternalLink,
  Rows as Rows3,
  Trash as Trash2,
} from "@phosphor-icons/react";
import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useStore } from "zustand";
const openUrl = (url: string) => { window.open(url, "_blank") }
import Breadcrumb from "@/features/editor/components/toolbar/breadcrumb";
import { FileExplorerIcon } from "@/features/file-explorer/components/file-explorer-icon";
import { useWorkspaceStore } from "@/features/workspace/stores/workspace-context";
import { useEditorSettingsStore } from "@/features/editor/stores/settings-store";
import { calculateLineHeight, splitLines } from "@/features/editor/utils/lines";
import { useZoomStore } from "@/features/window/stores/zoom-store";
import { useFileSystemStore } from "@/features/file-system/controllers/store";
import { Button } from "@/components/ui/button";
import Tooltip from "@/components/ui/tooltip";
import { cn } from "@/utils/cn";
import { formatRelativeDate } from "@/utils/date";
import { joinPath } from "@/utils/path-helpers";
import { getRemotes } from "../../api/git-remotes-api";
import { getGitStatus } from "../../api/git-status-api";
import type { MultiFileDiff } from "../../types/git-diff-types";
import type { GitDiff } from "../../types/git-types";
import { gitDiffCache } from "../../utils/git-diff-cache";
import { getFileStatus } from "../../utils/git-diff-helpers";
import {
  getInitialExpandedDiffFileKeys,
  shouldUseScrollableDiffEditor,
} from "../../utils/diff-viewer-scale";
import { buildWorkingTreeMultiDiff } from "../../utils/working-tree-multi-diff";
import { buildMonacoDiffContent } from "../../utils/diff-editor-content";
import MonacoDiffEditorView from "./monaco-diff-editor-view";
import ImageDiffViewer from "./git-diff-image";
import TextDiffViewer from "./git-diff-text";
import { Badge } from "@/components/ui/badge";
```

- [ ] **Step 2: Replace `LargeDiffSectionEditor`**

Find and replace the entire `LargeDiffSectionEditor` function (lines 116–140 in the original file) with:

```tsx
function LargeDiffSectionEditor({ diff, cacheKey }: { diff: GitDiff; cacheKey: string }) {
  const { original, modified } = useMemo(() => buildMonacoDiffContent(diff), [diff]);
  const filePath = diff.new_path || diff.old_path || diff.file_path;

  return (
    <div
      className="relative overflow-hidden border-border border-t bg-background"
      style={{ height: "min(72vh, 760px)", minHeight: "420px" }}
    >
      <MonacoDiffEditorView
        originalContent={original}
        modifiedContent={modified}
        filePath={filePath}
        renderSideBySide={false}
        height="100%"
        cacheKey={`${cacheKey}_large`}
        scrollable={true}
      />
    </div>
  );
}
```

- [ ] **Step 3: Replace `EmbeddedDiffSectionEditor`**

Find and replace the entire `EmbeddedDiffSectionEditor` function (lines 142–293 in the original file) with:

```tsx
function EmbeddedDiffSectionEditor({
  diff,
  cacheKey,
  viewMode,
}: {
  diff: GitDiff;
  cacheKey: string;
  viewMode: "unified" | "split";
}) {
  const fontSize = useEditorSettingsStore.use.fontSize();
  const zoomLevel = useZoomStore.use.editorZoomLevel();
  const { original, modified } = useMemo(() => buildMonacoDiffContent(diff), [diff]);

  const height = useMemo(() => {
    const lineCount = Math.max(splitLines(original).length, splitLines(modified).length);
    const lineHeight = calculateLineHeight(fontSize * zoomLevel);
    return Math.max(lineCount * lineHeight + 16, 160);
  }, [original, modified, fontSize, zoomLevel]);

  const filePath = diff.new_path || diff.old_path || diff.file_path;

  return (
    <div className="border-border border-t bg-background" style={{ height: `${height}px` }}>
      <MonacoDiffEditorView
        originalContent={original}
        modifiedContent={modified}
        filePath={filePath}
        renderSideBySide={viewMode === "split"}
        height={height}
        cacheKey={cacheKey}
        scrollable={false}
      />
    </div>
  );
}
```

- [ ] **Step 4: Type-check**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "git-diff-editor-stack"
```

Expected: no output (no errors).

- [ ] **Step 5: Run the full test suite**

```bash
cd web && npx vitest run
```

Expected: all tests pass (our change is pure component replacement — no logic shared with existing tests).

- [ ] **Step 6: Commit**

```bash
git add web/src/features/git/components/diff/git-diff-editor-stack.tsx
git commit -m "feat(git-diff): replace EmbeddedDiffSectionEditor + LargeDiffSectionEditor with MonacoDiffEditorView"
```

---

### Task 4: Delete `DiffLineBackgroundLayer` and clean up dead exports

**Files:**
- Delete: `web/src/features/git/components/diff/diff-line-background-layer.tsx`
- Modify: `web/src/features/git/utils/diff-editor-content.ts` (remove unused exports)

After Task 3, `DiffLineBackgroundLayer` has no importers. `serializeGitDiffSourceForEditor` and `serializeGitDiffSourceForSplitEditor` are also no longer imported in `git-diff-editor-stack.tsx`; verify they have no other callers before removing.

- [ ] **Step 1: Confirm no remaining importers of `DiffLineBackgroundLayer`**

```bash
grep -r "DiffLineBackgroundLayer\|diff-line-background-layer" web/src --include="*.ts" --include="*.tsx"
```

Expected: no output. If any file still imports it, do not delete — investigate first.

- [ ] **Step 2: Delete `diff-line-background-layer.tsx`**

```bash
rm web/src/features/git/components/diff/diff-line-background-layer.tsx
```

- [ ] **Step 3: Confirm no remaining callers of the two serializers**

```bash
grep -r "serializeGitDiffSourceForEditor\|serializeGitDiffSourceForSplitEditor" web/src --include="*.ts" --include="*.tsx"
```

Expected: matches only in `diff-editor-content.ts` (the definitions). If any other file imports them, skip removing those exports.

- [ ] **Step 4: Remove the two dead exports from `diff-editor-content.ts`**

If Step 3 showed definitions only, delete the `serializeGitDiffSourceForEditor` function (lines 106–137) and the `serializeGitDiffSourceForSplitEditor` function (lines 139–219) from `web/src/features/git/utils/diff-editor-content.ts`. Also delete the now-unused `SerializedEditorDiffContent` and `SerializedSplitEditorDiffContent` interfaces and the `pushLine` helper if it is only used by those two functions.

After deletion, verify `pushLine` is not used anywhere else:

```bash
grep -n "pushLine\|SerializedEditorDiffContent\|SerializedSplitEditorDiffContent" web/src/features/git/utils/diff-editor-content.ts
```

Expected: no matches (all removed).

- [ ] **Step 5: Type-check**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: no errors.

- [ ] **Step 6: Run tests**

```bash
cd web && npx vitest run
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "chore(git-diff): delete DiffLineBackgroundLayer and remove dead serializer exports"
```
