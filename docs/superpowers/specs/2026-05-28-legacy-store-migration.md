# Legacy Buffer Store Migration Spec

## Goal

Replace the legacy global `useBufferStore` (1,616 lines, 69 importers) with an expanded per-workspace `workspaceStore` as the single source of truth for all buffer and pane state. Delete the global store, the sync bridge, all shims, and the `activateBufferInPaneAndSync` workaround. The result is one coherent store per workspace.

---

## The Problem

The codebase runs two competing stores simultaneously:

**Legacy `useBufferStore`** (`features/editor/stores/buffer-store.ts`)
- Global Zustand singleton
- Owns 22 content types, session persistence, auto-eviction, closed buffer history, pending close state, global `activeBufferId`
- 69 importers (60 production files, 9 tests)
- CodeEditor reads **exclusively** from this store

**Workspace store** (`features/workspace/stores/workspace-store.ts`)
- Per-workspace Zustand store, instantiated via registry
- Owns pane tree (split/group/leaf) and 6 of 22 content types
- `registerExternalBuffer()` creates empty shell buffers that share an ID with legacy buffers so the pane tree has something to reference
- `activateBufferInPaneAndSync()` is called 15 times across the codebase to keep both stores aligned after every state change

This results in: race conditions (shell buffer vs. real buffer), sync failures (tab drag silently failing), and ~1,600 lines of maintenance debt.

---

## Target Architecture

**One store per workspace.** The workspace store owns everything:

```
workspaceStore (per workspace, via registry)
├── buffers: PaneContent[]          ← all 22 types, real content, session-persisted
├── closedBuffersHistory: ClosedBuffer[]  ← for "Reopen Closed Tab"
├── pendingClose: PendingClose | null     ← unsaved-changes dialog gate
├── maxOpenTabs: number
└── paneRoot / bottomRoot / activePaneId  ← existing pane tree (unchanged)
    └── paneGroup.activeBufferId          ← pane-local active buffer (replaces global)
```

**CodeEditor becomes pane-aware.** It already accepts `paneId?: string`. After migration it uses `useWorkspaceStoreContext` to:
1. Read its pane's `activeBufferId` (or use its explicit `bufferId` prop)
2. Find the buffer in `workspaceStore.buffers`

**Global `activeBufferId` is gone.** Every `activeBufferId` is pane-local, already stored in `paneGroup.activeBufferId`.

---

## What Changes in the Workspace Store

### New state fields

Add to `BufferSlice` in `buffer-slice.ts`:

```typescript
closedBuffersHistory: ClosedBuffer[]   // max EDITOR_CONSTANTS.MAX_CLOSED_BUFFERS_HISTORY
pendingClose: PendingClose | null
maxOpenTabs: number                    // default: EDITOR_CONSTANTS.MAX_OPEN_TABS
```

Types are already defined in `buffer-store.ts`:
```typescript
// ClosedBuffer (buffer-store.ts ~line 56):
interface ClosedBuffer { path: string; name: string; isPinned: boolean }

// PendingClose (buffer-store.ts ~line 49):
interface PendingClose {
  bufferId: string
  type: "single" | "others" | "all" | "to-left" | "to-right"
  anchorBufferId?: string
  keepBufferId?: string
}
```

Move these types to `features/panes/types/pane-content.ts` where other buffer types live.

### New/expanded actions

Expand `openContent` to handle all 22 content types (currently handles 6). The missing 16 are:
`newTab`, `image`, `pdf`, `binary`, `database`, `pullRequest`, `githubIssue`, `githubAction`, `markdownPreview`, `htmlPreview`, `csvPreview`, `externalEditor`, `globalSearch`, `diagnostics`, `references`, `onboarding`

Each missing type needs an `OurOpenContentSpec` variant added to `buffer-slice.ts`.

Add actions:
- `reopenLastClosedBuffer()` — pops from `closedBuffersHistory`, calls `openContent` in active pane
- `setPendingClose(pc: PendingClose | null)` — sets dialog gate
- `confirmPendingClose()` — executes the enqueued close, resets `pendingClose` to null
- Auto-eviction: called internally from `openContent` when `buffers.length >= maxOpenTabs`. Port `applyAutoEviction` logic from `buffer-eviction.ts`.
- Session persistence: subscribe inside `createWorkspaceStore` (debounced 300ms) to write persistable buffers to `useSessionStore`. Port `saveSessionToStore` logic from `buffer-session-persistence.ts`.

### Remove from workspace store

Delete `registerExternalBuffer()` from `buffer-slice.ts` once CodeEditor no longer reads from the legacy store.

---

## CodeEditor Migration

**File:** `features/editor/components/code-editor.tsx`

CodeEditor already has `paneId?: string` prop (line ~52). The parent (`PaneContainer`) already passes its pane ID there.

### Before (reads from global legacy store)

```tsx
// line 129
const activeBufferId = useBufferStore((state) => propBufferId ?? state.activeBufferId);
// line 131-139
const activeBuffer = useBufferStore(
  useCallback((state) => activeBufferId ? state.buffers.find(b => b.id === activeBufferId) ?? null : null, [activeBufferId])
);
```

### After (reads from workspace store)

```tsx
// Resolve active buffer ID from pane-local state
const activeBufferId = useWorkspaceStoreContext(
  useCallback(
    (state) => propBufferId ?? (paneId ? findPaneGroup(state.paneRoot, paneId)?.activeBufferId : null) ?? null,
    [paneId, propBufferId]
  )
);
// Find buffer in workspace buffers
const activeBuffer = useWorkspaceStoreContext(
  useCallback(
    (state) => activeBufferId ? (state.buffers.find(b => b.id === activeBufferId) ?? null) : null,
    [activeBufferId]
  )
);
```

All other `useBufferStore` usages in `code-editor.tsx` (save, content change, etc.) are replaced with workspace store actions via `useWorkspaceStore().getState()`.

The same pattern applies to the other editor-feature files that read from `useBufferStore`:
- `monaco-editor.tsx`
- `external-editor-terminal.tsx`
- `toolbar/breadcrumb.tsx`
- `toolbar/editor-status-actions.tsx`
- `toolbar/find-bar.tsx`
- `markdown/markdown-preview.tsx`
- `html/html-preview.tsx`
- All editor store dependents (`editor-app-store.ts`, `state-store.ts`, `tree-cache-store.ts`, `ui-store.ts`, `view-store.ts`)

---

## Call Site Migration Categories

### Category A: `openContent` callers (~20 files)

Pattern: `useBufferStore.getState().actions.openContent(spec)`
Becomes: `useWorkspaceStore().getState().bufferActions.openContent(spec)`

Files: command palette actions, file explorer, keymaps, git feature, pane-activation, etc.

### Category B: `activateBufferInPaneAndSync` callers (15 sites)

This function calls both stores. After migration, each call site replaces it with:
```typescript
// Ensure buffer is in the pane and set it active — workspace store handles everything
workspaceStore.getState().paneActions.addBufferToPane(paneId, bufferId, true)
```

`addBufferToPane(paneId, bufferId, setActive=true)` already deduplicates (buffer-slice line 120-124 behavior: adds if not present, sets active flag). If the implementation does not deduplicate, add a `setActiveBufferInPane(paneId, bufferId)` action to the pane slice that only sets `activeBufferId` without pushing to `bufferIds`.

Sites: `tab-bar.tsx`, `bottom-pane.tsx` (×2), `terminal-tab.tsx`, `terminal-tab-bar.tsx`, `pane-container.tsx` (×9)

### Category C: Global `activeBufferId` readers

Pattern: `useBufferStore((state) => state.activeBufferId)`
Becomes: `useWorkspaceStoreContext((state) => findPaneGroup(state.paneRoot, activePaneId)?.activeBufferId)`

Files: `footer.tsx`, `git-view.tsx`, `git-inline-blame.tsx`, `editor-status-actions.tsx`, etc.

### Category D: Buffer content / metadata readers

Pattern: `useBufferStore((state) => state.buffers.find(b => b.id === someId))`
Becomes: `useWorkspaceStoreContext((state) => state.buffers.find(b => b.id === someId))`

Files: All remaining importers reading specific buffer properties.

### Category E: Pending close / closed history actions

Pattern: `useBufferStore.getState().actions.setPendingClose(...)`, `actions.reopenLastClosed()`
Becomes: workspace store actions of same name.

---

## Deletion List

Once all importers are migrated and tests pass:

| File | Lines | Notes |
|------|-------|-------|
| `features/editor/stores/buffer-store.ts` | 1,616 | Entire file deleted |
| `features/editor/stores/buffer-pane-sync.ts` | 82 | Entire file deleted |
| `registerExternalBuffer()` in `buffer-slice.ts` | ~60 | Function deleted from buffer-slice |
| `activateBufferInPaneAndSync()` in `pane-activation.ts` | ~25 | Function deleted |
| `features/editor/stores/buffer-session-persistence.ts` | ~80 | Logic ported to workspace store |
| `features/editor/stores/buffer-eviction.ts` | ~50 | Logic ported to workspace store |

Total removed: ~1,900 lines.

---

## Testing Strategy

- Existing tests in `web/src/__tests__/features/editor/` use `useBufferStore` — update them to use workspace store
- Existing tests in `web/src/__tests__/features/panes/` use `usePaneStore` / workspace store — minimal changes
- Key behaviors to test: open/close/evict, session save/restore, pending close dialog, reopen closed tab, pane-local active buffer isolation (opening file in pane A does not change active in pane B)

---

## Out of Scope

- Changing pane tree structure (already correct)
- LSP integration internals (port the buffer ID lookups, keep LSP logic unchanged)
- `usePaneStore` legacy store — it's already being migrated separately (pane-container.tsx comment on line 664 notes it's no longer authoritative)
- Workspace store registry / context wiring — use existing patterns from workspace-store-registry.ts and workspace-context.ts

---

## Migration Order (for implementation plan)

1. **Type plumbing**: Move `ClosedBuffer`, `PendingClose` types to `pane-content.ts`; add to `BufferSlice` state
2. **Expand openContent**: Add all 16 missing content types to `buffer-slice.ts`
3. **Port features to workspace store**: Auto-eviction, session persistence, closed history, pending close
4. **Migrate CodeEditor**: Remove `useBufferStore`, use `useWorkspaceStoreContext`
5. **Migrate remaining 60 importers**: By category (A → B → C/D → E)
6. **Delete legacy files**: `buffer-store.ts`, `buffer-pane-sync.ts`, eviction/persistence helpers
7. **Update tests**: Rewrite 9 test files from `useBufferStore` to workspace store
