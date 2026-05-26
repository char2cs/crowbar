# Dead Code Removal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove all stub, disabled, and out-of-scope features inherited from Athas that serve no purpose in Crowbar's agentic-first IDE vision.

**Architecture:** Sequential deletions — each task removes one feature directory and surgically cleans up its import sites. Order matters: `athas-editor` goes first because several later tasks (vim, debugger) have import sites inside it that disappear automatically once it's deleted.

**Tech Stack:** TypeScript, React, Vite — use `npx tsc --noEmit` after each task to verify no type errors before committing.

---

## Files Overview

| Task | Delete | Fix Import Sites |
|------|--------|-----------------|
| 1 | `features/remote/` | none |
| 2 | `features/athas-editor/`, `features/editor/lsp/` | `features/editor/components/code-editor.tsx`, `features/editor/tests/editor-key-edits.test.ts`, `features/editor/tests/language-id.test.ts`, `features/editor/stores/buffer-store.ts` |
| 3 | `features/vim/` | `features/editor/components/monaco-editor.tsx`, `features/editor/components/toolbar/editor-status-actions.tsx`, `features/command-palette/components/command-palette.tsx` |
| 4 | `features/debugger/` | `features/layout/components/bottom-pane/bottom-pane.tsx` |
| 5 | `features/lsp/` | none (top-level lsp/ had no external imports confirmed) |
| 6 | `features/collaboration/` | `features/editor/stores/editor-app-store.ts` |
| 7 | `features/local-history/` | `features/command-palette/constants/file-actions.tsx`, `features/command-palette/components/command-palette.tsx`, `features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx`, `features/editor/stores/editor-app-store.ts` |
| 8 | `features/outline/` | `features/layout/components/sidebar/main-sidebar.tsx`, `features/command-palette/components/command-palette.tsx` |
| 9 | — | `features/settings/config/default-settings.ts`, `features/settings/config/features.ts` |

---

## Task 1: Delete `features/remote/`

**Files:**
- Delete: `web/src/features/remote/` (entire directory)

This feature has zero imports anywhere in the codebase. Pure orphan.

- [ ] **Step 1: Delete the directory**

```bash
rm -rf web/src/features/remote
```

- [ ] **Step 2: Verify no references remain**

```bash
grep -r "features/remote" web/src --include="*.ts" --include="*.tsx"
```

Expected: no output.

- [ ] **Step 3: Type-check**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

Expected: no new errors.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "chore: remove dead features/remote (zero imports, out of scope)"
```

---

## Task 2: Delete `features/athas-editor/` and `features/editor/lsp/`

**Files:**
- Delete: `web/src/features/athas-editor/` (entire directory — 115 files, ~40k lines)
- Delete: `web/src/features/editor/lsp/` (entire directory)
- Modify: `web/src/features/editor/components/code-editor.tsx`
- Delete: `web/src/features/editor/tests/editor-key-edits.test.ts`
- Delete: `web/src/features/editor/tests/language-id.test.ts`
- Modify: `web/src/features/editor/stores/buffer-store.ts`

`athas-editor` is the custom text editor from Athas. It is **currently disabled** — `useAthasEditor = false` is hardcoded in `code-editor.tsx:169`. Monaco is what actually runs. Deleting it also removes the `features/editor/lsp/` directory which only serves athas-editor.

- [ ] **Step 1: Delete the directories**

```bash
rm -rf web/src/features/athas-editor
rm -rf web/src/features/editor/lsp
```

- [ ] **Step 2: Delete tests that reference athas-editor**

```bash
rm web/src/features/editor/tests/editor-key-edits.test.ts
rm web/src/features/editor/tests/language-id.test.ts
```

- [ ] **Step 3: Clean up `code-editor.tsx`**

Open `web/src/features/editor/components/code-editor.tsx`.

Remove the lazy AthasEditor import (around line 95):
```tsx
// DELETE these lines:
const AthasEditor = lazy(() =>
  import("@/features/athas-editor/components/editor").then((module) => ({
    default: module.Editor,
  })),
);
```

Remove the `useAthasEditor` constant and all conditional branches that reference it (around line 169):
```tsx
// DELETE:
const useAthasEditor = false;
```

For every occurrence of `useAthasEditor &&` — the expression always evaluates to `false`, so remove the entire conditional:
- Any `useAthasEditor && someValue` → replace with `false` or remove the prop entirely
- Any `useAthasEditor ? <AthasEditor .../> : <MonacoEditor .../>` → keep only the `<MonacoEditor .../>` branch

Search for all occurrences:
```bash
grep -n "useAthasEditor\|AthasEditor" web/src/features/editor/components/code-editor.tsx
```

Remove each one. The Monaco path is always active — keep it, remove the Athas branches.

Also remove any imports of athas-editor types at the top of the file:
```bash
grep -n "athas-editor" web/src/features/editor/components/code-editor.tsx
```

Delete those import lines.

- [ ] **Step 4: Clean up `monaco-editor.tsx` athas-editor imports**

```bash
grep -n "athas-editor" web/src/features/editor/components/monaco-editor.tsx
```

Remove any import lines referencing `athas-editor`. These are type imports (InlayHint, SemanticTokenState) — check if they are used elsewhere in the file or can be replaced with inline types/removed.

- [ ] **Step 5: Clean up `buffer-store.ts` — remove LSP dynamic import**

Open `web/src/features/editor/stores/buffer-store.ts`.

Find and remove the LSP stop call in `closeBufferForce` (around line 1073):

```ts
// DELETE this entire block:
if (shouldStartLsp(closedBuffer)) {
  import("@/features/editor/lsp/lsp-client")
    .then(({ LspClient }) => {
      const lspClient = LspClient.getInstance();
      logger.info("BufferStore", `Stopping LSP for ${closedBuffer.path}`);
      return lspClient.stopForFile(closedBuffer.path);
    })
    .catch((error) => {
      logger.error("BufferStore", "Failed to stop LSP:", error);
    });

  // Add to closed history
  const closedBufferInfo: ClosedBuffer = { ... };
  ...
}
```

Wait — the closed history tracking (`closedBufferInfo`, `updatedHistory`, `state.closedBuffersHistory`) is inside that `if (shouldStartLsp(...))` block. Keep the history tracking, only delete the LSP import block. After editing, the block should look like:

```ts
if (shouldStartLsp(closedBuffer)) {
  // Add to closed history
  const closedBufferInfo: ClosedBuffer = {
    path: closedBuffer.path,
    name: closedBuffer.name,
    isPinned: closedBuffer.isPinned,
  };

  const updatedHistory = [closedBufferInfo, ...closedBuffersHistory].slice(
    0,
    EDITOR_CONSTANTS.MAX_CLOSED_BUFFERS_HISTORY,
  );

  set((state) => {
    state.closedBuffersHistory = updatedHistory;
  });
}
```

Also remove the `checkExtensionSupport` call references to LSP if any exist.

- [ ] **Step 6: Type-check**

```bash
cd web && npx tsc --noEmit 2>&1 | head -30
```

Fix any remaining errors from missed references. Re-run until clean.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "chore: remove disabled athas-editor and editor/lsp (useAthasEditor=false, Monaco-only now)"
```

---

## Task 3: Delete `features/vim/`

**Files:**
- Delete: `web/src/features/vim/` (entire directory)
- Modify: `web/src/features/editor/components/monaco-editor.tsx`
- Modify: `web/src/features/editor/components/toolbar/editor-status-actions.tsx`
- Modify: `web/src/features/command-palette/components/command-palette.tsx`

Vim mode is a stub — `vimCommands = []` and all store actions are no-ops.

> Note: `features/athas-editor/` was already deleted in Task 2, so the vim imports in `athas-editor/components/editor.tsx` and `athas-editor/components/toolbar/editor-status-actions.tsx` are already gone.

- [ ] **Step 1: Delete the directory**

```bash
rm -rf web/src/features/vim
```

- [ ] **Step 2: Clean up `monaco-editor.tsx`**

Open `web/src/features/editor/components/monaco-editor.tsx`.

Remove these imports (lines 24–25):
```ts
// DELETE:
import { parseAndExecuteVimCommand, vimCommands } from "@/features/vim/stores/vim-commands";
import { useVimStore, type VimMode as AthasVimMode } from "@/features/vim/stores/vim-store";
```

Remove all `vimModeEnabled` and `vimCurrentMode` usage. Search for them:
```bash
grep -n "vim\|Vim" web/src/features/editor/components/monaco-editor.tsx
```

For each usage:
- `vimModeEnabled && vimRelativeLineNumbers` → remove the vim relative line numbers logic entirely (simplify to just `lineNumbers: showLineNumbers ? "on" : "off"`)
- `cursorStyle: vimModeEnabled && vimCurrentMode === "normal" ? "block" : "line"` → simplify to `cursorStyle: "line"`
- `cursorBlinking: vimModeEnabled && vimCurrentMode === "normal" ? "solid" : "blink"` → simplify to `cursorBlinking: "blink"`
- Any vim command execution block → delete

- [ ] **Step 3: Clean up `editor/components/toolbar/editor-status-actions.tsx`**

```bash
grep -n "vim\|Vim" web/src/features/editor/components/toolbar/editor-status-actions.tsx
```

Remove any vim imports and the vim mode indicator rendering from the toolbar.

- [ ] **Step 4: Clean up `command-palette.tsx`**

Open `web/src/features/command-palette/components/command-palette.tsx`.

Remove these imports (lines 26–27):
```ts
// DELETE:
import { vimCommands } from "@/features/vim/stores/vim-commands";
import { useVimStore } from "@/features/vim/stores/vim-store";
```

Remove `vimMode: settings.vimMode` and `vimCommands` from wherever they're passed (around line 278–279). Remove the `useVimStore` call.

- [ ] **Step 5: Type-check**

```bash
cd web && npx tsc --noEmit 2>&1 | head -30
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "chore: remove stub features/vim (vimCommands=[], all no-ops)"
```

---

## Task 4: Delete `features/debugger/`

**Files:**
- Delete: `web/src/features/debugger/` (entire directory)
- Modify: `web/src/features/layout/components/bottom-pane/bottom-pane.tsx`

> Note: `features/athas-editor/` was already deleted in Task 2, so the debugger imports in `gutter.tsx` and `flow-line-numbers.tsx` are already gone.

- [ ] **Step 1: Delete the directory**

```bash
rm -rf web/src/features/debugger
```

- [ ] **Step 2: Clean up `bottom-pane.tsx`**

Open `web/src/features/layout/components/bottom-pane/bottom-pane.tsx`.

Remove line 3:
```ts
// DELETE:
import DebuggerView from "@/features/debugger/components/debugger-view";
```

Remove the feature gate effect (lines ~49–54):
```ts
// DELETE this entire useEffect:
useEffect(() => {
  if (
    isBottomPaneVisible &&
    bottomPaneActiveTab === "debugger" &&
    !settings.coreFeatures.debugger
  ) {
    useUIState.getState().setIsBottomPaneVisible(false);
  }
}, [bottomPaneActiveTab, isBottomPaneVisible, settings.coreFeatures.debugger]);
```

Remove the debugger tab render (around line 217):
```tsx
// DELETE:
{settings.coreFeatures.debugger && bottomPaneActiveTab === "debugger" && (
  <DebuggerView />
)}
```

Also remove `"debugger"` from the tab list if it exists in the bottom pane tab definitions. Search:
```bash
grep -n "debugger" web/src/features/layout/components/bottom-pane/bottom-pane.tsx
```

- [ ] **Step 3: Type-check**

```bash
cd web && npx tsc --noEmit 2>&1 | head -30
```

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "chore: remove stub features/debugger (debugger-view returns null)"
```

---

## Task 5: Delete `features/lsp/`

**Files:**
- Delete: `web/src/features/lsp/` (entire directory — top-level, confirmed stub)

The top-level `features/lsp/` had no external imports (confirmed by grep). It's the stub with all no-op methods.

- [ ] **Step 1: Delete the directory**

```bash
rm -rf web/src/features/lsp
```

- [ ] **Step 2: Verify no references**

```bash
grep -r "from \"@/features/lsp" web/src --include="*.ts" --include="*.tsx"
```

Expected: no output.

- [ ] **Step 3: Type-check**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "chore: remove stub features/lsp (all methods were no-ops)"
```

---

## Task 6: Delete `features/collaboration/`

**Files:**
- Delete: `web/src/features/collaboration/` (entire directory)
- Modify: `web/src/features/editor/stores/editor-app-store.ts`

- [ ] **Step 1: Delete the directory**

```bash
rm -rf web/src/features/collaboration
```

- [ ] **Step 2: Clean up `editor-app-store.ts`**

Open `web/src/features/editor/stores/editor-app-store.ts`.

Find and remove the collaboration dynamic import and usage (around line 202):
```ts
// DELETE this import and the block that uses it:
await import("@/features/collaboration/lib/collaboration-sidebar-model");
const collaborationNoteTarget = parseCollaborationNoteBufferPath(activeBuffer.path);
```

Also remove the `collaborationNoteTarget` conditional branch (around line 223):
```ts
// DELETE:
} else if (collaborationNoteTarget) {
  // ... collaboration logic
}
```

Search for all collaboration references:
```bash
grep -n "collaboration\|parseCollaborationNote" web/src/features/editor/stores/editor-app-store.ts
```

Remove every match.

- [ ] **Step 3: Type-check**

```bash
cd web && npx tsc --noEmit 2>&1 | head -20
```

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "chore: remove stub features/collaboration (parseCollaborationNoteBufferPath returns null)"
```

---

## Task 7: Delete `features/local-history/`

**Files:**
- Delete: `web/src/features/local-history/` (entire directory)
- Modify: `web/src/features/command-palette/constants/file-actions.tsx`
- Modify: `web/src/features/command-palette/components/command-palette.tsx`
- Modify: `web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx`
- Modify: `web/src/features/editor/stores/editor-app-store.ts`
- Delete: `web/src/features/editor/tests/editor-save-all.test.ts`

- [ ] **Step 1: Delete the directory and test**

```bash
rm -rf web/src/features/local-history
rm web/src/features/editor/tests/editor-save-all.test.ts
```

- [ ] **Step 2: Clean up `file-actions.tsx`**

Open `web/src/features/command-palette/constants/file-actions.tsx`.

Remove line 2:
```ts
// DELETE:
import { openLocalHistoryForActiveFile } from "@/features/local-history/utils/open-local-history";
```

Remove the local history action entry (around line 35–40):
```ts
// DELETE this action object:
{
  id: "file-local-history",
  // ...
  commandId: "file.localHistory",
  // ...
},
```

- [ ] **Step 3: Clean up `command-palette.tsx`**

Open `web/src/features/command-palette/components/command-palette.tsx`.

Remove line 10:
```ts
// DELETE:
import { LocalHistoryCommandContent } from "@/features/local-history/components/local-history-command";
```

Remove the `local-history` view branch (around line 402–405):
```tsx
// DELETE:
) : currentView === "local-history" ? (
  <LocalHistoryCommandContent
    isActive={currentView === "local-history"}
  />
```

- [ ] **Step 4: Clean up `use-file-explorer-context-menu.tsx`**

Open `web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx`.

Remove line 32:
```ts
// DELETE:
import { openLocalHistoryForPath } from "@/features/local-history/utils/open-local-history";
```

Remove the local history menu item (around line 320):
```ts
// DELETE:
{
  id: "local-history",
  // ...
},
```

- [ ] **Step 5: Clean up `editor-app-store.ts`**

```bash
grep -n "local-history\|localHistory\|recordLocalHistory" web/src/features/editor/stores/editor-app-store.ts
```

Remove line 8:
```ts
// DELETE:
import { recordLocalHistoryFile } from "@/features/local-history/api/local-history-api";
```

Remove any call to `recordLocalHistoryFile(...)` in the store logic.

- [ ] **Step 6: Type-check**

```bash
cd web && npx tsc --noEmit 2>&1 | head -30
```

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "chore: remove stub features/local-history (all api functions were no-ops)"
```

---

## Task 8: Delete `features/outline/`

**Files:**
- Delete: `web/src/features/outline/` (entire directory)
- Modify: `web/src/features/layout/components/sidebar/main-sidebar.tsx`
- Modify: `web/src/features/command-palette/components/command-palette.tsx`

- [ ] **Step 1: Delete the directory**

```bash
rm -rf web/src/features/outline
```

- [ ] **Step 2: Clean up `main-sidebar.tsx`**

Open `web/src/features/layout/components/sidebar/main-sidebar.tsx`.

Remove line 13:
```ts
// DELETE:
import { OutlineSidebar } from "@/features/outline/components/outline-sidebar";
```

Remove the outline feature check and sidebar entry (around line 112 and 180–181):
```ts
// DELETE:
const isOutlineFeatureEnabled = settings.coreFeatures.outline;
```

```tsx
// DELETE the outline sidebar item:
{
  id: "outline" as const,
  content: <OutlineSidebar />,
  // ...
}
```

Search for all outline references:
```bash
grep -n "outline\|Outline" web/src/features/layout/components/sidebar/main-sidebar.tsx
```

- [ ] **Step 3: Clean up `command-palette.tsx`**

Open `web/src/features/command-palette/components/command-palette.tsx`.

Remove line 11:
```ts
// DELETE:
import { OutlineCommandContent } from "@/features/outline/components/outline-command";
```

Remove the outline view branch (around line 411–413):
```tsx
// DELETE:
) : currentView === "outline" ? (
  <OutlineCommandContent
    isActive={currentView === "outline"}
  />
```

- [ ] **Step 4: Type-check**

```bash
cd web && npx tsc --noEmit 2>&1 | head -30
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "chore: remove stub features/outline (returns placeholder div, out of scope)"
```

---

## Task 9: Settings Cleanup

**Files:**
- Modify: `web/src/features/settings/config/default-settings.ts`
- Modify: `web/src/features/settings/config/features.ts`

Remove dead feature flags from settings now that their features are gone. The flags to remove: `remote`, `debugger`, `outline`, `teamCollaboration` from `coreFeatures`. Also remove `vimMode` and `vimRelativeLineNumbers` from top-level settings.

- [ ] **Step 1: Check what uses these flags**

```bash
grep -rn "coreFeatures.remote\|coreFeatures.debugger\|coreFeatures.outline\|coreFeatures.teamCollaboration\|settings.vimMode\|settings.vimRelativeLineNumbers" web/src --include="*.ts" --include="*.tsx"
```

For each result: remove the reference (the feature is gone, the flag is meaningless).

- [ ] **Step 2: Update `default-settings.ts`**

Open `web/src/features/settings/config/default-settings.ts`.

In the `coreFeatures` block, remove:
```ts
// DELETE these keys:
remote: true,
debugger: false,
outline: true,
teamCollaboration: true,
```

At the top level of settings, remove:
```ts
// DELETE:
vimMode: false,
vimRelativeLineNumbers: false,
```

- [ ] **Step 3: Update `features.ts`**

Open `web/src/features/settings/config/features.ts`.

Remove the feature entries for `remote`, `debugger`, `outline`, `teamCollaboration`:
```ts
// DELETE each of these blocks:
{
  enabled: coreFeatures.remote,
  // ...
},
{
  enabled: coreFeatures.debugger,
  // ...
},
{
  enabled: coreFeatures.outline,
  // ...
},
{
  enabled: coreFeatures.teamCollaboration,
  // ...
},
```

- [ ] **Step 4: Find and fix any settings UI that references removed flags**

```bash
grep -rn "vimMode\|vimRelativeLineNumbers\|coreFeatures.remote\|coreFeatures.debugger\|coreFeatures.outline\|coreFeatures.teamCollaboration" web/src --include="*.ts" --include="*.tsx"
```

For any settings panel UI that renders toggles for these — remove those UI rows.

- [ ] **Step 5: Type-check**

```bash
cd web && npx tsc --noEmit 2>&1 | head -30
```

Fix any type errors from the narrowed settings type (TypeScript will catch anything that still references removed keys).

- [ ] **Step 6: Final verification — no dead references remain**

```bash
grep -r "features/remote\|features/athas-editor\|features/vim\|features/debugger\|features/lsp\|features/collaboration\|features/local-history\|features/outline" web/src --include="*.ts" --include="*.tsx"
```

Expected: no output.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "chore: remove dead feature flags from settings (vim, debugger, outline, remote, teamCollaboration)"
```

---

## Done

After all 9 tasks complete, the codebase should be ~30-40k lines lighter with:
- 8 dead feature directories deleted
- All import sites cleaned up
- TypeScript validating cleanly
- Monaco-only editor path (no disabled Athas branch)
- Settings type narrowed to only live features
