# Legacy Buffer Store Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace global `useBufferStore` (1,616 lines, 45 production importers) with an expanded per-workspace `workspaceStore` as the sole source of truth for all buffer and pane state, then delete all legacy bridge code.

**Architecture:** The workspace store's `buffer-slice.ts` gains all 22 content types (currently handles 6), session persistence, auto-eviction, closed buffer history, and pending close dialogs. A new `workspace-store-ref.ts` module enables imperative access from non-React utility code and Zustand sub-stores. `CodeEditor` switches its two `useBufferStore` selectors to `useWorkspaceStoreContext`. All remaining 44 production importers are migrated in batch tasks, then `buffer-store.ts` and its bridge helpers are deleted entirely.

**Tech Stack:** Zustand 4, Immer, React 18, TypeScript 5, Vitest

**Spec:** `docs/superpowers/specs/2026-05-28-legacy-store-migration.md`

---

## File Structure

**Create:**
- `web/src/features/workspace/stores/workspace-store-ref.ts` — module-level ref for imperative workspace store access from utility functions and sub-stores

**Expand:**
- `web/src/features/panes/types/pane-content.ts` — add `ClosedBuffer`, `PendingClose` exports
- `web/src/features/workspace/stores/slices/buffer-slice.ts` — all 22 content types, `closedBuffersHistory`, `pendingClose`, `maxOpenTabs`, eviction, closed history, pending close
- `web/src/features/workspace/stores/workspace-store.ts` — session persistence subscriber
- `web/src/features/workspace/components/WorkspaceView.tsx` — call `setActiveWorkspaceStoreRef`

**Migrate (45 production files):** Listed per-task below.

**Delete after migration:**
- `web/src/features/editor/stores/buffer-store.ts` (1,616 lines)
- `web/src/features/editor/stores/buffer-pane-sync.ts` (82 lines)
- `web/src/features/editor/stores/buffer-eviction.ts` (43 lines)
- `web/src/features/panes/utils/pane-activation.ts` (27 lines)

**Keep (still used after migration):**
- `web/src/features/editor/stores/buffer-session-persistence.ts` — `workspace-store.ts` imports `saveSessionToStore` from here after Task 4

---

## Task 1: Workspace Store Ref + ClosedBuffer/PendingClose Types

Add the imperative workspace store ref (needed by utility functions outside React), export `ClosedBuffer`/`PendingClose` types from `pane-content.ts`, and wire up the ref in `WorkspaceView.tsx`.

**Files:**
- Create: `web/src/features/workspace/stores/workspace-store-ref.ts`
- Modify: `web/src/features/panes/types/pane-content.ts`
- Modify: `web/src/features/workspace/stores/workspace-store-registry.ts`
- Modify: `web/src/features/workspace/components/WorkspaceView.tsx`

- [ ] **Step 1: Add `ClosedBuffer` and `PendingClose` types to `pane-content.ts`**

Append to the end of `web/src/features/panes/types/pane-content.ts`:

```typescript
// ── Buffer history / dialog state (used by workspace store) ─────────

export interface ClosedBuffer {
  path: string;
  name: string;
  isPinned: boolean;
}

export interface PendingClose {
  bufferId: string;
  type: "single" | "others" | "all" | "to-left" | "to-right";
  anchorBufferId?: string;
  keepBufferId?: string;
}
```

- [ ] **Step 2: Create `workspace-store-ref.ts`**

Create `web/src/features/workspace/stores/workspace-store-ref.ts`:

```typescript
import type { WorkspaceStore } from './workspace-store'

let _activeWorkspaceStore: WorkspaceStore | null = null

export function setActiveWorkspaceStoreRef(store: WorkspaceStore | null): void {
  _activeWorkspaceStore = store
}

export function getActiveWorkspaceStoreRef(): WorkspaceStore | null {
  return _activeWorkspaceStore
}
```

- [ ] **Step 3: Wire up the ref in `WorkspaceView.tsx`**

`WorkspaceView.tsx` is at `web/src/features/workspace/components/WorkspaceView.tsx`. It currently is:

```tsx
import { WorkspaceStoreContext } from '../stores/workspace-context'
import { getOrCreateWorkspaceStore } from '../stores/workspace-store-registry'
import { WorkspaceLayoutRoot } from './WorkspaceLayoutRoot'
import { useWorkspaceEffects } from '../stores/hooks/use-workspace-effects'

// ...

export function WorkspaceView({ wsId, label }: WorkspaceViewProps) {
  const store = getOrCreateWorkspaceStore(wsId)

  return (
    <WorkspaceStoreContext.Provider value={store}>
      <WorkspaceViewInner wsId={wsId} label={label} />
    </WorkspaceStoreContext.Provider>
  )
}
```

Add the `useEffect` import and the ref call:

```tsx
import { useEffect } from 'react'
import { WorkspaceStoreContext } from '../stores/workspace-context'
import { getOrCreateWorkspaceStore } from '../stores/workspace-store-registry'
import { setActiveWorkspaceStoreRef } from '../stores/workspace-store-ref'
import { WorkspaceLayoutRoot } from './WorkspaceLayoutRoot'
import { useWorkspaceEffects } from '../stores/hooks/use-workspace-effects'

// ...

export function WorkspaceView({ wsId, label }: WorkspaceViewProps) {
  const store = getOrCreateWorkspaceStore(wsId)

  useEffect(() => {
    setActiveWorkspaceStoreRef(store)
    return () => { setActiveWorkspaceStoreRef(null) }
  }, [store])

  return (
    <WorkspaceStoreContext.Provider value={store}>
      <WorkspaceViewInner wsId={wsId} label={label} />
    </WorkspaceStoreContext.Provider>
  )
}
```

- [ ] **Step 4: Run TypeScript check**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npx tsc --noEmit 2>&1 | head -30
```

Expected: No errors related to the new files.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/panes/types/pane-content.ts \
        web/src/features/workspace/stores/workspace-store-ref.ts \
        web/src/features/workspace/components/WorkspaceView.tsx
git commit -m "feat: add ClosedBuffer/PendingClose types and workspace store imperative ref"
```

---

## Task 2: Expand Buffer Slice — All 22 Types + New State

Replace `OurOpenContentSpec` with the canonical `OpenContentSpec` from `pane-content.ts`, expand `openContent` to handle all 22 content types, and add `closedBuffersHistory`, `pendingClose`, and `maxOpenTabs` to the slice state.

**Files:**
- Modify: `web/src/features/workspace/stores/slices/buffer-slice.ts`

- [ ] **Step 1: Replace `OurOpenContentSpec` and add new state fields**

Replace the entire content of `web/src/features/workspace/stores/slices/buffer-slice.ts` with the following. This preserves all existing logic and adds the 16 missing type handlers + new state fields:

```typescript
import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type {
  PaneContent,
  OpenContentSpec,
  EditorContent,
  CrowbarChatContent,
  DiffContent,
  TerminalContent,
  WebViewerContent,
  AgentContent,
  NewTabContent,
  ImageContent,
  PdfContent,
  BinaryContent,
  DatabaseContent,
  PullRequestContent,
  GitHubIssueContent,
  GitHubActionContent,
  MarkdownPreviewContent,
  HtmlPreviewContent,
  CsvPreviewContent,
  ExternalEditorContent,
  GlobalSearchContent,
  DiagnosticsContent,
  ReferencesContent,
  OnboardingContent,
  ClosedBuffer,
  PendingClose,
} from '@/features/panes/types/pane-content'
import { shouldStartLsp } from '@/features/panes/types/pane-content'
import { EDITOR_CONSTANTS } from '@/features/editor/config/constants'
import { nanoid } from 'nanoid'

// ── Actions ──────────────────────────────────────────────────────────

export interface BufferActions {
  openContent(spec: OpenContentSpec): string
  closeBuffer(id: string): void
  setPinned(id: string, pinned: boolean): void
  setPreview(id: string, preview: boolean): void
  promotePreview(id: string): void
  getBufferById(id: string): PaneContent | undefined
  reopenLastClosedBuffer(): void
  setPendingClose(pc: PendingClose | null): void
  confirmPendingClose(): void
}

// ── Slice ────────────────────────────────────────────────────────────

export interface BufferSlice {
  buffers: PaneContent[]
  closedBuffersHistory: ClosedBuffer[]
  pendingClose: PendingClose | null
  maxOpenTabs: number
  bufferActions: BufferActions
}

export const createBufferSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  BufferSlice
> = (set, get) => ({
  buffers: [],
  closedBuffersHistory: [],
  pendingClose: null,
  maxOpenTabs: EDITOR_CONSTANTS.MAX_OPEN_TABS,

  bufferActions: {
    openContent(spec) {
      // Deduplicate: return existing buffer id if already open
      const existing = (() => {
        if (spec.type === 'editor') {
          return get().buffers.find(b => b.type === 'editor' && b.path === spec.path)
        }
        if (spec.type === 'crowbarChat') {
          return get().buffers.find(
            b => b.type === 'crowbarChat' && (b as CrowbarChatContent).wsId === spec.wsId,
          )
        }
        if (spec.type === 'diff') {
          return get().buffers.find(b => b.type === 'diff' && b.path === spec.path)
        }
        if (spec.type === 'terminal' && spec.sessionId) {
          return get().buffers.find(
            b => b.type === 'terminal' && (b as TerminalContent).sessionId === spec.sessionId,
          )
        }
        if (spec.type === 'webViewer') {
          return get().buffers.find(
            b => b.type === 'webViewer' && (b as WebViewerContent).url === spec.url,
          )
        }
        if (spec.type === 'agent' && spec.sessionId) {
          return get().buffers.find(
            b => b.type === 'agent' && (b as AgentContent).sessionId === spec.sessionId,
          )
        }
        if (spec.type === 'image') {
          return get().buffers.find(b => b.type === 'image' && b.path === spec.path)
        }
        if (spec.type === 'pdf') {
          return get().buffers.find(b => b.type === 'pdf' && b.path === spec.path)
        }
        if (spec.type === 'binary') {
          return get().buffers.find(b => b.type === 'binary' && b.path === spec.path)
        }
        if (spec.type === 'database') {
          return get().buffers.find(b => b.type === 'database' && b.path === spec.path)
        }
        if (spec.type === 'pullRequest') {
          return get().buffers.find(
            b => b.type === 'pullRequest' && (b as PullRequestContent).prNumber === spec.prNumber,
          )
        }
        if (spec.type === 'githubIssue') {
          return get().buffers.find(
            b => b.type === 'githubIssue' &&
              (b as GitHubIssueContent).issueNumber === spec.issueNumber,
          )
        }
        if (spec.type === 'githubAction') {
          return get().buffers.find(
            b => b.type === 'githubAction' && (b as GitHubActionContent).runId === spec.runId,
          )
        }
        if (spec.type === 'markdownPreview') {
          return get().buffers.find(b => b.type === 'markdownPreview' && b.path === spec.path)
        }
        if (spec.type === 'htmlPreview') {
          return get().buffers.find(b => b.type === 'htmlPreview' && b.path === spec.path)
        }
        if (spec.type === 'csvPreview') {
          return get().buffers.find(b => b.type === 'csvPreview' && b.path === spec.path)
        }
        if (spec.type === 'externalEditor') {
          return get().buffers.find(b => b.type === 'externalEditor' && b.path === spec.path)
        }
        if (spec.type === 'globalSearch') {
          return get().buffers.find(b => b.type === 'globalSearch')
        }
        if (spec.type === 'diagnostics') {
          return get().buffers.find(b => b.type === 'diagnostics')
        }
        if (spec.type === 'references') {
          return get().buffers.find(b => b.type === 'references')
        }
        if (spec.type === 'onboarding') {
          return get().buffers.find(b => b.type === 'onboarding')
        }
        return undefined
      })()

      if (existing) {
        get().paneActions.addBufferToPane(get().activePaneId, existing.id, true)
        return existing.id
      }

      const id = nanoid()

      // Build the new buffer object
      let buf: PaneContent

      if (spec.type === 'editor') {
        const isPreview = spec.isPreview ?? false
        buf = {
          id, type: 'editor',
          path: spec.path, name: spec.name,
          content: spec.content, savedContent: spec.content,
          isDirty: false,
          isVirtual: spec.isVirtual ?? false,
          language: spec.language,
          tokens: [],
          isPinned: false, isPreview, isActive: false,
        } satisfies EditorContent
      } else if (spec.type === 'crowbarChat') {
        buf = {
          id, type: 'crowbarChat',
          wsId: spec.wsId, name: spec.name,
          path: '', isPinned: false, isPreview: false, isActive: false,
        } satisfies CrowbarChatContent
      } else if (spec.type === 'diff') {
        buf = {
          id, type: 'diff',
          path: spec.path, name: spec.name,
          content: spec.content, savedContent: spec.content,
          diffData: spec.diffData,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies DiffContent
      } else if (spec.type === 'terminal') {
        const terminalCount = get().buffers.filter(b => b.type === 'terminal').length
        const sessionId = spec.sessionId ?? `terminal-tab-${Date.now()}`
        buf = {
          id, type: 'terminal',
          sessionId,
          path: spec.path ?? `terminal://${sessionId}`,
          name: spec.name ?? `Terminal ${terminalCount + 1}`,
          initialCommand: spec.command,
          workingDirectory: spec.workingDirectory,
          remoteConnectionId: spec.remoteConnectionId,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies TerminalContent
      } else if (spec.type === 'webViewer') {
        let displayName = 'Web Viewer'
        if (spec.url && spec.url !== 'about:blank') {
          try { displayName = new URL(spec.url).hostname || displayName } catch { /* invalid url */ }
        }
        buf = {
          id, type: 'webViewer',
          url: spec.url,
          path: `web-viewer://${spec.url}`,
          name: displayName,
          zoomLevel: spec.zoomLevel,
          profileKey: spec.profileKey,
          history: spec.history,
          historyIndex: spec.historyIndex,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies WebViewerContent
      } else if (spec.type === 'agent') {
        const agentCount = get().buffers.filter(b => b.type === 'agent').length
        const sessionId = spec.sessionId ?? nanoid()
        buf = {
          id, type: 'agent',
          sessionId,
          path: `agent://${sessionId}`,
          name: `Agent ${agentCount + 1}`,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies AgentContent
      } else if (spec.type === 'newTab') {
        buf = {
          id, type: 'newTab',
          path: '', name: 'New Tab',
          isPinned: false, isPreview: false, isActive: false,
        } satisfies NewTabContent
      } else if (spec.type === 'image') {
        buf = {
          id, type: 'image',
          path: spec.path, name: spec.name,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies ImageContent
      } else if (spec.type === 'pdf') {
        buf = {
          id, type: 'pdf',
          path: spec.path, name: spec.name,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies PdfContent
      } else if (spec.type === 'binary') {
        buf = {
          id, type: 'binary',
          path: spec.path, name: spec.name,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies BinaryContent
      } else if (spec.type === 'database') {
        buf = {
          id, type: 'database',
          path: spec.path, name: spec.name,
          databaseType: spec.databaseType,
          connectionId: spec.connectionId,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies DatabaseContent
      } else if (spec.type === 'pullRequest') {
        buf = {
          id, type: 'pullRequest',
          path: `pr://${spec.prNumber}`,
          name: spec.name ?? `PR #${spec.prNumber}`,
          prNumber: spec.prNumber,
          authorAvatarUrl: spec.authorAvatarUrl,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies PullRequestContent
      } else if (spec.type === 'githubIssue') {
        buf = {
          id, type: 'githubIssue',
          path: `github-issue://${spec.issueNumber}`,
          name: spec.name ?? `Issue #${spec.issueNumber}`,
          issueNumber: spec.issueNumber,
          repoPath: spec.repoPath,
          authorAvatarUrl: spec.authorAvatarUrl,
          url: spec.url,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies GitHubIssueContent
      } else if (spec.type === 'githubAction') {
        buf = {
          id, type: 'githubAction',
          path: `github-action://${spec.runId}`,
          name: spec.name ?? `Action #${spec.runId}`,
          runId: spec.runId,
          repoPath: spec.repoPath,
          url: spec.url,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies GitHubActionContent
      } else if (spec.type === 'markdownPreview') {
        buf = {
          id, type: 'markdownPreview',
          path: spec.path, name: spec.name,
          content: spec.content,
          sourceFilePath: spec.sourceFilePath,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies MarkdownPreviewContent
      } else if (spec.type === 'htmlPreview') {
        buf = {
          id, type: 'htmlPreview',
          path: spec.path, name: spec.name,
          content: spec.content,
          sourceFilePath: spec.sourceFilePath,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies HtmlPreviewContent
      } else if (spec.type === 'csvPreview') {
        buf = {
          id, type: 'csvPreview',
          path: spec.path, name: spec.name,
          content: spec.content,
          sourceFilePath: spec.sourceFilePath,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies CsvPreviewContent
      } else if (spec.type === 'externalEditor') {
        buf = {
          id, type: 'externalEditor',
          path: spec.path, name: spec.name,
          terminalConnectionId: spec.terminalConnectionId,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies ExternalEditorContent
      } else if (spec.type === 'globalSearch') {
        buf = {
          id, type: 'globalSearch',
          path: 'global-search://', name: 'Search',
          isPinned: false, isPreview: false, isActive: false,
        } satisfies GlobalSearchContent
      } else if (spec.type === 'diagnostics') {
        buf = {
          id, type: 'diagnostics',
          path: 'diagnostics://', name: 'Problems',
          isPinned: false, isPreview: false, isActive: false,
        } satisfies DiagnosticsContent
      } else if (spec.type === 'references') {
        buf = {
          id, type: 'references',
          path: 'references://', name: 'References',
          isPinned: false, isPreview: false, isActive: false,
        } satisfies ReferencesContent
      } else {
        // spec.type === 'onboarding'
        buf = {
          id, type: 'onboarding',
          path: 'onboarding://', name: 'Welcome',
          mode: spec.context.mode,
          currentVersion: spec.context.currentVersion,
          previousVersion: spec.context.previousVersion,
          isPinned: false, isPreview: false, isActive: false,
        } satisfies OnboardingContent
      }

      set(state => { state.buffers.push(buf) })
      get().paneActions.addBufferToPane(get().activePaneId, id, true)
      if (spec.type === 'editor' && spec.isPreview) {
        get().paneActions.setPanePreviewBuffer(get().activePaneId, id)
      }

      return id
    },

    closeBuffer(id) {
      const buf = get().buffers.find(b => b.id === id)
      if (buf && shouldStartLsp(buf)) {
        set(state => {
          const entry: ClosedBuffer = { path: buf.path, name: buf.name, isPinned: buf.isPinned }
          state.closedBuffersHistory.unshift(entry)
          if (state.closedBuffersHistory.length > EDITOR_CONSTANTS.MAX_CLOSED_BUFFERS_HISTORY) {
            state.closedBuffersHistory.pop()
          }
        })
      }
      set(state => { state.buffers = state.buffers.filter(b => b.id !== id) })
    },

    setPinned(id, pinned) {
      set(state => {
        const buf = state.buffers.find(b => b.id === id)
        if (buf) buf.isPinned = pinned
      })
    },

    setPreview(id, preview) {
      set(state => {
        const buf = state.buffers.find(b => b.id === id)
        if (buf) buf.isPreview = preview
      })
    },

    promotePreview(id) {
      let found = false
      set(state => {
        const buf = state.buffers.find(b => b.id === id)
        if (buf) { buf.isPreview = false; found = true }
      })
      if (found) get().paneActions.clearPreviewBufferEverywhere(id)
    },

    getBufferById(id) {
      return get().buffers.find(b => b.id === id)
    },

    reopenLastClosedBuffer() {
      const entry = get().closedBuffersHistory[0]
      if (!entry) return
      set(state => { state.closedBuffersHistory.shift() })
      get().bufferActions.openContent({
        type: 'editor',
        path: entry.path,
        name: entry.name,
        content: '',
      })
    },

    setPendingClose(pc) {
      set(state => { state.pendingClose = pc })
    },

    confirmPendingClose() {
      const pc = get().pendingClose
      if (!pc) return
      set(state => { state.pendingClose = null })
      if (pc.type === 'single') {
        get().bufferActions.closeBuffer(pc.bufferId)
      }
      // Other close types (others, all, to-left, to-right) are handled by the
      // callers that set pendingClose — they call closeBuffer for each target
      // after confirmation. This resets the gate.
    },
  },
})
```

- [ ] **Step 2: Run TypeScript check**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npx tsc --noEmit 2>&1 | head -40
```

Expected: Errors only about `OurOpenContentSpec` still being imported in other files (those will be fixed in later tasks). No errors inside `buffer-slice.ts` itself.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/workspace/stores/slices/buffer-slice.ts
git commit -m "feat: expand workspace buffer slice to handle all 22 content types + history/pending close"
```

---

## Task 3: Port Auto-Eviction into openContent

When `openContent` is about to create a new buffer and `buffers.length >= maxOpenTabs`, evict the least-recently-used non-protected buffer first. Also remove old preview buffer when opening a new preview in the same pane (already partially done — verify the preview cleanup path works for new types).

**Files:**
- Modify: `web/src/features/workspace/stores/slices/buffer-slice.ts`

- [ ] **Step 1: Add eviction call at start of `openContent`, after the deduplication block**

In `buffer-slice.ts`, locate the `if (existing)` block at the end of the deduplication logic. Immediately after it (before `const id = nanoid()`), add:

```typescript
// Auto-evict when at max tabs
if (get().buffers.length >= get().maxOpenTabs) {
  const AUTO_EVICTION_PROTECTED = new Set<PaneContent['type']>([
    'agent', 'externalEditor', 'terminal', 'webViewer',
  ])
  const candidates = get().buffers.filter(
    b => !b.isPinned && !AUTO_EVICTION_PROTECTED.has(b.type),
  )
  if (candidates.length > 0) {
    const evictId = candidates[0]!.id
    get().bufferActions.closeBuffer(evictId)
    get().paneActions.removeBufferFromPane
    // removeBufferFromPane for all panes
    const allPanes = [
      ...getAllPaneGroups(get().paneRoot),
      ...getAllPaneGroups(get().bottomRoot),
    ]
    for (const pane of allPanes) {
      if (pane.bufferIds.includes(evictId)) {
        get().paneActions.removeBufferFromPane(pane.id, evictId)
      }
    }
  }
}
```

Import `getAllPaneGroups` at the top of the file:
```typescript
import { getAllPaneGroups } from '@/features/panes/utils/pane-tree'
```

Wait — `closeBuffer` already removes from `state.buffers`, but `removeBufferFromPane` is needed to clean pane references. The correct eviction sequence is: remove from pane tree, then remove from buffers. Use this instead:

```typescript
// Auto-evict when at max tabs (before creating a new buffer)
if (get().buffers.length >= get().maxOpenTabs) {
  const AUTO_EVICTION_PROTECTED = new Set<PaneContent['type']>([
    'agent', 'externalEditor', 'terminal', 'webViewer',
  ])
  const evictee = get().buffers.find(b => !b.isPinned && !AUTO_EVICTION_PROTECTED.has(b.type))
  if (evictee) {
    const allPanes = [
      ...getAllPaneGroups(get().paneRoot),
      ...getAllPaneGroups(get().bottomRoot),
    ]
    for (const pane of allPanes) {
      if (pane.bufferIds.includes(evictee.id)) {
        get().paneActions.removeBufferFromPane(pane.id, evictee.id, true)
      }
    }
    set(state => { state.buffers = state.buffers.filter(b => b.id !== evictee.id) })
  }
}
```

Place this block immediately after `if (existing) { ... return existing.id }` and before `const id = nanoid()`.

- [ ] **Step 2: Run TypeScript check**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npx tsc --noEmit 2>&1 | head -20
```

Expected: Clean.

- [ ] **Step 3: Write a test**

In `web/src/__tests__/features/workspace/stores/buffer-slice-eviction.test.ts` (create new):

```typescript
import { describe, it, expect, beforeEach } from 'vitest'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

describe('buffer slice auto-eviction', () => {
  it('evicts least-recent non-protected buffer when at maxOpenTabs', () => {
    const store = createWorkspaceStore('test-ws')
    store.setState({ maxOpenTabs: 2 })
    const activePaneId = store.getState().activePaneId

    store.getState().bufferActions.openContent({
      type: 'editor', path: '/a.ts', name: 'a.ts', content: '',
    })
    store.getState().bufferActions.openContent({
      type: 'editor', path: '/b.ts', name: 'b.ts', content: '',
    })
    expect(store.getState().buffers).toHaveLength(2)

    store.getState().bufferActions.openContent({
      type: 'editor', path: '/c.ts', name: 'c.ts', content: '',
    })
    // /a.ts should be evicted (was first, least recent)
    expect(store.getState().buffers).toHaveLength(2)
    expect(store.getState().buffers.map(b => b.name)).not.toContain('a.ts')
    expect(store.getState().buffers.map(b => b.name)).toContain('c.ts')
  })

  it('never evicts pinned buffers', () => {
    const store = createWorkspaceStore('test-ws')
    store.setState({ maxOpenTabs: 2 })

    const idA = store.getState().bufferActions.openContent({
      type: 'editor', path: '/a.ts', name: 'a.ts', content: '',
    })
    store.getState().bufferActions.setPinned(idA, true)
    store.getState().bufferActions.openContent({
      type: 'editor', path: '/b.ts', name: 'b.ts', content: '',
    })
    store.getState().bufferActions.openContent({
      type: 'editor', path: '/c.ts', name: 'c.ts', content: '',
    })
    // b.ts should be evicted; a.ts (pinned) stays
    expect(store.getState().buffers.map(b => b.name)).toContain('a.ts')
    expect(store.getState().buffers.map(b => b.name)).not.toContain('b.ts')
  })

  it('never evicts terminal or agent buffers', () => {
    const store = createWorkspaceStore('test-ws')
    store.setState({ maxOpenTabs: 2 })

    store.getState().bufferActions.openContent({ type: 'terminal' })
    store.getState().bufferActions.openContent({
      type: 'editor', path: '/a.ts', name: 'a.ts', content: '',
    })
    store.getState().bufferActions.openContent({
      type: 'editor', path: '/b.ts', name: 'b.ts', content: '',
    })
    // Terminal stays, a.ts evicted
    expect(store.getState().buffers.map(b => b.type)).toContain('terminal')
    expect(store.getState().buffers.map(b => b.name)).not.toContain('a.ts')
  })
})
```

- [ ] **Step 4: Run the test**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npx vitest run src/__tests__/features/workspace/stores/buffer-slice-eviction.test.ts --reporter=verbose
```

Expected: 3 tests pass.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/workspace/stores/slices/buffer-slice.ts \
        web/src/__tests__/features/workspace/stores/buffer-slice-eviction.test.ts
git commit -m "feat: port auto-eviction into workspace store openContent"
```

---

## Task 4: Port Session Persistence into createWorkspaceStore

Subscribe to workspace store state changes and save persistable buffers to `useSessionStore` (debounced 300ms). The logic is ported from `buffer-session-persistence.ts` which uses `buffer-session-save-queue.ts`.

**Files:**
- Modify: `web/src/features/workspace/stores/workspace-store.ts`

- [ ] **Step 1: Add persistence subscription in `createWorkspaceStore`**

`buffer-session-persistence.ts` exports `saveSessionToStore(buffers, activeBufferId)` which already does debouncing via `createWorkspaceSessionSaveQueue`. We re-use it directly. The workspace store `activeBufferId` is pane-local, not global. Pass the active pane's `activeBufferId` instead.

Replace `web/src/features/workspace/stores/workspace-store.ts` with:

```typescript
import { createStore, type StoreApi } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import type { WorkspaceState } from './workspace-store.types'
import { createPaneSlice } from './slices/pane-slice'
import { createBufferSlice } from './slices/buffer-slice'
import { createWorkflowSlice } from './slices/workflow-slice'
import { createLspSlice } from './slices/lsp-slice'
import { createTerminalSlice } from './slices/terminal-slice'
import { createFileWatcherSlice } from './slices/file-watcher-slice'
import { createRecentFilesSlice } from './slices/recent-files-slice'
import { saveSessionToStore } from '@/features/editor/stores/buffer-session-persistence'
import { findPaneGroup } from '@/features/panes/utils/pane-tree'

export type WorkspaceStore = StoreApi<WorkspaceState>

export type WorkspaceSnapshot = Partial<
  Pick<WorkspaceState,
    | 'paneRoot' | 'bottomRoot' | 'activePaneId' | 'fullscreenPaneId' | 'mostRecentActivePaneIds'
    | 'buffers'
    | 'closedBuffersHistory'
    | 'currentStepId'
    | 'recentFiles'
    | 'terminalLayout'
  >
>

export function createWorkspaceStore(wsId: string, snapshot?: WorkspaceSnapshot): WorkspaceStore {
  const store = createStore<WorkspaceState>()(
    immer((set, get, api): WorkspaceState => ({
      workspaceId: wsId,
      ...createPaneSlice(set, get, api),
      ...createBufferSlice(set, get, api),
      ...createWorkflowSlice(set, get, api),
      ...createLspSlice(set, get, api),
      ...createTerminalSlice(set, get, api),
      ...createFileWatcherSlice(set, get, api),
      ...createRecentFilesSlice(set, get, api),
      ...(snapshot ?? {}),
    }))
  )

  // Subscribe to persist buffer sessions on change
  store.subscribe((state, prev) => {
    if (state.buffers === prev.buffers) return
    const activePane = findPaneGroup(state.paneRoot, state.activePaneId)
    saveSessionToStore(state.buffers, activePane?.activeBufferId ?? null)
  })

  return store
}
```

- [ ] **Step 2: Add `closedBuffersHistory` to `WorkspaceSnapshot` type**

The change above already includes `closedBuffersHistory` in `WorkspaceSnapshot`. This ensures closed history is persisted across workspace re-mounts. No further change needed.

- [ ] **Step 3: Run TypeScript check**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npx tsc --noEmit 2>&1 | head -20
```

Expected: Clean.

- [ ] **Step 4: Commit**

```bash
git add web/src/features/workspace/stores/workspace-store.ts
git commit -m "feat: add session persistence subscription to workspace store"
```

---

## Task 5: Migrate CodeEditor to Workspace Store

Remove `useBufferStore` from `code-editor.tsx` and replace the two selectors with `useWorkspaceStoreContext`. The `paneId` prop (already present) is used to look up the pane-local `activeBufferId`.

**Files:**
- Modify: `web/src/features/editor/components/code-editor.tsx`

- [ ] **Step 1: Remove `useBufferStore` import, add workspace store imports**

In `web/src/features/editor/components/code-editor.tsx`, find line 13:
```typescript
import { useBufferStore } from "@/features/editor/stores/buffer-store";
```
Replace with:
```typescript
import { useWorkspaceStoreContext, useWorkspaceStore } from "@/features/workspace/stores/workspace-context";
import { findPaneGroup } from "@/features/panes/utils/pane-tree";
```

- [ ] **Step 2: Replace the two `useBufferStore` selectors (lines 129-139)**

Find:
```typescript
const activeBufferId = useBufferStore((state) => propBufferId ?? state.activeBufferId);
const zoomLevel = useZoomStore.use.editorZoomLevel();
const activeBuffer = useBufferStore(
  useCallback(
    (state) =>
      activeBufferId
        ? state.buffers.find((buffer) => buffer.id === activeBufferId) || null
        : null,
    [activeBufferId],
  ),
);
```

Replace with:
```typescript
const activeBufferId = useWorkspaceStoreContext(
  useCallback(
    (state) => {
      if (propBufferId) return propBufferId
      const paneToUse = paneId ?? state.activePaneId
      return findPaneGroup(state.paneRoot, paneToUse)?.activeBufferId ?? null
    },
    [paneId, propBufferId],
  ),
);
const zoomLevel = useZoomStore.use.editorZoomLevel();
const activeBuffer = useWorkspaceStoreContext(
  useCallback(
    (state) =>
      activeBufferId
        ? state.buffers.find((buffer) => buffer.id === activeBufferId) || null
        : null,
    [activeBufferId],
  ),
);
```

- [ ] **Step 3: Check for any other `useBufferStore` usages in `code-editor.tsx`**

```bash
grep -n "useBufferStore\|bufferStore" /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web/src/features/editor/components/code-editor.tsx
```

Expected: No matches (all usages replaced).

- [ ] **Step 4: Run TypeScript check**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npx tsc --noEmit 2>&1 | grep "code-editor" | head -20
```

Expected: No errors in `code-editor.tsx`.

- [ ] **Step 5: Verify app renders correctly**

Start the dev server and open a file. Verify the editor renders the file content. Open a second pane and verify each pane shows its own active file independently.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/editor/components/code-editor.tsx
git commit -m "feat: migrate CodeEditor to workspace store (removes useBufferStore dependency)"
```

---

## Task 6: Migrate Editor Feature Files (React hooks and components)

Migrate the editor-feature files that use `useBufferStore` exclusively in React context (hooks/components). These all follow the same pattern: replace `useBufferStore(selector)` with `useWorkspaceStoreContext(selector)` and `useBufferStore.getState()` with `useWorkspaceStore().getState()`.

**Files:**
- Modify: `web/src/features/editor/components/monaco-editor.tsx`
- Modify: `web/src/features/editor/components/external-editor-terminal.tsx`
- Modify: `web/src/features/editor/components/html/html-preview.tsx`
- Modify: `web/src/features/editor/components/toolbar/breadcrumb.tsx`
- Modify: `web/src/features/editor/components/toolbar/editor-status-actions.tsx`
- Modify: `web/src/features/editor/components/toolbar/find-bar.tsx`
- Modify: `web/src/features/editor/markdown/markdown-preview.tsx`
- Modify: `web/src/features/editor/hooks/use-lsp-integration.ts`
- Modify: `web/src/features/editor/hooks/use-snippet-completion.ts`

**Pattern for each file:**

Read the file first. Then apply this pattern:

**Import change:**
```typescript
// Remove:
import { useBufferStore } from "@/features/editor/stores/buffer-store";
// Add:
import { useWorkspaceStoreContext, useWorkspaceStore } from "@/features/workspace/stores/workspace-context";
```

**Reactive selector change:**
```typescript
// Before:
const x = useBufferStore((state) => state.someField)
// After:
const x = useWorkspaceStoreContext((state) => state.someField)
```

**Imperative access change (inside event handlers / useEffect):**
```typescript
// Before:
useBufferStore.getState().actions.someAction(...)
// After:
useWorkspaceStore().getState().bufferActions.someAction(...)
```

Note: `useBufferStore.getState().actions` → `useWorkspaceStore().getState().bufferActions` (the action namespace is `bufferActions` not `actions`).

Note: `useBufferStore.getState().activeBufferId` (global) → look up from pane: `findPaneGroup(useWorkspaceStore().getState().paneRoot, activePaneId)?.activeBufferId`

Note: `useBufferStore.getState().buffers` → `useWorkspaceStore().getState().buffers`

- [ ] **Step 1: Read and migrate each file in the list above**

For each file: read it, apply the import change, apply all selector/action replacements. One file at a time.

- [ ] **Step 2: Run TypeScript check after all files**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npx tsc --noEmit 2>&1 | grep -E "editor/components|editor/hooks" | head -30
```

Expected: No errors in editor feature files.

- [ ] **Step 3: Commit**

```bash
git add \
  web/src/features/editor/components/monaco-editor.tsx \
  web/src/features/editor/components/external-editor-terminal.tsx \
  web/src/features/editor/components/html/html-preview.tsx \
  web/src/features/editor/components/toolbar/breadcrumb.tsx \
  web/src/features/editor/components/toolbar/editor-status-actions.tsx \
  web/src/features/editor/components/toolbar/find-bar.tsx \
  web/src/features/editor/markdown/markdown-preview.tsx \
  web/src/features/editor/hooks/use-lsp-integration.ts \
  web/src/features/editor/hooks/use-snippet-completion.ts
git commit -m "feat: migrate editor components and hooks to workspace store"
```

---

## Task 7: Migrate Editor Zustand Sub-Stores and LSP Utilities

Migrate the global Zustand sub-stores and LSP utility files. These use `useBufferStore.getState()` imperatively (no React context). They will use `getActiveWorkspaceStoreRef()` from Task 1.

**Files:**
- Modify: `web/src/features/editor/stores/editor-app-store.ts`
- Modify: `web/src/features/editor/stores/state-store.ts`
- Modify: `web/src/features/editor/stores/tree-cache-store.ts`
- Modify: `web/src/features/editor/stores/ui-store.ts`
- Modify: `web/src/features/editor/stores/view-store.ts`
- Modify: `web/src/features/editor/extensions/api.ts`
- Modify: `web/src/features/editor/lsp/workspace-edit.ts`
- Modify: `web/src/features/editor/lsp/use-go-to-definition.ts`

**Pattern for each file:**

Read the file first. These files call `useBufferStore.getState()` inside Zustand actions or utility functions. Replace:

```typescript
// Remove:
import { useBufferStore } from "@/features/editor/stores/buffer-store";

// Add:
import { getActiveWorkspaceStoreRef } from "@/features/workspace/stores/workspace-store-ref";
```

For every `useBufferStore.getState()` usage:
```typescript
// Before:
const store = useBufferStore.getState()
store.actions.someAction(...)
store.activeBufferId
store.buffers.find(...)

// After:
const store = getActiveWorkspaceStoreRef()?.getState()
if (!store) return // or handle gracefully
store.bufferActions.someAction(...)
findPaneGroup(store.paneRoot, store.activePaneId)?.activeBufferId
store.buffers.find(...)
```

If the import of `findPaneGroup` is needed:
```typescript
import { findPaneGroup } from "@/features/panes/utils/pane-tree";
```

- [ ] **Step 1: Read and migrate each file in the list**

For each file: read it, apply the import and usage changes. One file at a time.

- [ ] **Step 2: Run TypeScript check**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npx tsc --noEmit 2>&1 | grep "editor/stores\|editor/lsp\|editor/extensions" | head -30
```

Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add \
  web/src/features/editor/stores/editor-app-store.ts \
  web/src/features/editor/stores/state-store.ts \
  web/src/features/editor/stores/tree-cache-store.ts \
  web/src/features/editor/stores/ui-store.ts \
  web/src/features/editor/stores/view-store.ts \
  web/src/features/editor/extensions/api.ts \
  web/src/features/editor/lsp/workspace-edit.ts \
  web/src/features/editor/lsp/use-go-to-definition.ts
git commit -m "feat: migrate editor sub-stores and LSP utilities to workspace store"
```

---

## Task 8: Delete `activateBufferInPaneAndSync` and Migrate All 13 Call Sites

`pane-activation.ts` defines `activateBufferInPaneAndSync(paneId, bufferId)` which calls the legacy buffer store to sync `activeBufferId`. After CodeEditor migration (Task 5), this sync is no longer needed. Replace every call site with `workspaceStore.getState().paneActions.addBufferToPane(paneId, bufferId, true)`.

**Call sites (confirmed via grep):**
- `pane-container.tsx` lines 341, 371, 406, 648, 656, 659, 687, 697, 702 (9 sites)
- `bottom-pane.tsx` lines 128, 136 (2 sites)
- `tab-bar.tsx` line 592 (1 site)
- `terminal-tab-bar.tsx` line 581 (1 site)
- `terminal-tab.tsx` line 39 (1 site)

Also migrate `activatePaneAndSyncBuffer` (also in `pane-activation.ts`, imported by `pane-container.tsx` line 33): replace with `workspaceStore.getState().paneActions.setActivePane(paneId)`.

**Files:**
- Modify: `web/src/features/panes/components/pane-container.tsx`
- Modify: `web/src/features/layout/components/bottom-pane/bottom-pane.tsx`
- Modify: `web/src/features/tabs/components/tab-bar.tsx`
- Modify: `web/src/features/terminal/components/terminal-tab-bar.tsx`
- Modify: `web/src/features/terminal/components/terminal-tab.tsx`
- Delete: `web/src/features/panes/utils/pane-activation.ts`

- [ ] **Step 1: Migrate `pane-container.tsx`**

Read `web/src/features/panes/components/pane-container.tsx` (1,199 lines). Find all call sites of `activateBufferInPaneAndSync` and `activatePaneAndSyncBuffer`.

For each `activateBufferInPaneAndSync(targetPaneId, bufferId)`:
```typescript
// Remove this call entirely — workspaceStore.getState().paneActions.addBufferToPane
// already happened on the line above in most sites. The only extra thing
// activateBufferInPaneAndSync was doing was setting legacy global activeBufferId.
// Delete the call. If addBufferToPane was not already called, replace with:
workspaceStore.getState().paneActions.addBufferToPane(targetPaneId, bufferId, true)
```

For each `activatePaneAndSyncBuffer(paneId)`:
```typescript
// Replace with:
workspaceStore.getState().paneActions.setActivePane(pane.id)
```

Remove the import line:
```typescript
import { activateBufferInPaneAndSync, activatePaneAndSyncBuffer } from "../utils/pane-activation";
```

Also remove the `useBufferStore` import if present in this file (check line by line).

- [ ] **Step 2: Migrate `bottom-pane.tsx`**

Read `web/src/features/layout/components/bottom-pane/bottom-pane.tsx`. Remove the `activateBufferInPaneAndSync` import. Replace line 128 and 136:

```typescript
// Line 128 — after openContent call that returns bufferId:
// Remove: activateBufferInPaneAndSync(BOTTOM_PANE_ID, bufferId);
// The openContent call already called addBufferToPane. Delete this line.

// Line 136 — after moveBufferToPane:
// Remove: activateBufferInPaneAndSync(BOTTOM_PANE_ID, tabData.bufferId);
// Replace with: workspaceStore.getState().paneActions.addBufferToPane(BOTTOM_PANE_ID, tabData.bufferId, true)
```

`bottom-pane.tsx` does not import `workspaceStore` directly. Read the file to check — if it needs `workspaceStore`, import it:
```typescript
import { useWorkspaceStore } from "@/features/workspace/stores/workspace-context";
```
And access it in the event handler:
```typescript
const workspaceStore = useWorkspaceStore()
```

- [ ] **Step 3: Migrate `tab-bar.tsx`**

Read `web/src/features/tabs/components/tab-bar.tsx`. Line 592:
```typescript
// Before:
activateBufferInPaneAndSync(destinationPaneId, dragged.id);
// After (delete this line — activatePaneBuffer on line 591 already does the pane activation):
// (no replacement needed)
```

Remove the `activateBufferInPaneAndSync` import from `pane-activation`.

- [ ] **Step 4: Migrate `terminal-tab-bar.tsx`**

Read `web/src/features/terminal/components/terminal-tab-bar.tsx`. Line 581:
```typescript
// Before:
activateBufferInPaneAndSync(destinationPaneId, bufferId);
// After — openContent was just called which already activates in pane:
// Delete this line.
```

Remove the `activateBufferInPaneAndSync` import.

- [ ] **Step 5: Migrate `terminal-tab.tsx`**

Read `web/src/features/terminal/components/terminal-tab.tsx`. Line 39:
```typescript
// Before:
activateBufferInPaneAndSync(paneId, bufferId);
// After:
workspaceStore.getState().paneActions.addBufferToPane(paneId, bufferId, true)
```

`terminal-tab.tsx` is a React component — get `workspaceStore` via hook:
```typescript
const workspaceStore = useWorkspaceStore()
```

Add import:
```typescript
import { useWorkspaceStore } from "@/features/workspace/stores/workspace-context";
```

Remove: `import { activateBufferInPaneAndSync } from "@/features/panes/utils/pane-activation";`

- [ ] **Step 6: Delete `pane-activation.ts`**

```bash
rm web/src/features/panes/utils/pane-activation.ts
```

- [ ] **Step 7: Run TypeScript check**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npx tsc --noEmit 2>&1 | head -40
```

Expected: No errors about deleted `pane-activation.ts` or its imports.

- [ ] **Step 8: Commit**

```bash
git add \
  web/src/features/panes/components/pane-container.tsx \
  web/src/features/layout/components/bottom-pane/bottom-pane.tsx \
  web/src/features/tabs/components/tab-bar.tsx \
  web/src/features/terminal/components/terminal-tab-bar.tsx \
  web/src/features/terminal/components/terminal-tab.tsx
git rm web/src/features/panes/utils/pane-activation.ts
git commit -m "feat: remove activateBufferInPaneAndSync — workspace store is now sole authority"
```

---

## Task 9: Migrate openContent / openXxxBuffer Callers

Migrate the files that call `useBufferStore.getState().actions.openContent(...)` or the legacy named wrappers like `openBuffer`, `openTerminalBuffer`, `openPRBuffer`, etc. After migration, they call `workspaceStore.getState().bufferActions.openContent(spec)` (or `getActiveWorkspaceStoreRef()?.getState().bufferActions.openContent(spec)` for non-React utility functions).

The legacy wrappers map to workspace `openContent` specs as follows:
```
openBuffer(path, name, content, ...) → { type: 'editor', path, name, content }
openBuffer(..., isImage) → { type: 'image', path, name }
openBuffer(..., isPdf) → { type: 'pdf', path, name }
openBuffer(..., isBinary) → { type: 'binary', path, name }
openBuffer(..., isDiff) → { type: 'diff', path, name, content, diffData }
openBuffer(..., isMarkdownPreview) → { type: 'markdownPreview', path, name, content, sourceFilePath }
openBuffer(..., isHtmlPreview) → { type: 'htmlPreview', path, name, content, sourceFilePath }
openBuffer(..., isCsvPreview) → { type: 'csvPreview', path, name, content, sourceFilePath }
openTerminalBuffer(opts) → { type: 'terminal', ...opts }
openAgentBuffer(sessionId) → { type: 'agent', sessionId }
openWebViewerBuffer(url) → { type: 'webViewer', url }
openPRBuffer(prNumber, meta) → { type: 'pullRequest', prNumber, ...meta }
openGitHubIssueBuffer(opts) → { type: 'githubIssue', ...opts }
openGitHubActionBuffer(opts) → { type: 'githubAction', ...opts }
openGlobalSearchBuffer() → { type: 'globalSearch' }
openDiagnosticsBuffer() → { type: 'diagnostics' }
openReferencesBuffer() → { type: 'references' }
openOnboardingBuffer(ctx) → { type: 'onboarding', context: ctx }
openExternalEditorBuffer(path, name, tcId) → { type: 'externalEditor', path, name, terminalConnectionId: tcId }
openDatabaseBuffer(path, name, dbType, connId) → { type: 'database', path, name, databaseType: dbType, connectionId: connId }
convertPreviewToDefinite(bufferId) → bufferActions.promotePreview(bufferId)
```

**Files to migrate:**
- `web/src/features/command-palette/components/command-palette.tsx`
- `web/src/features/command-palette/constants/navigation-actions.tsx`
- `web/src/features/command-palette/constants/view-actions.tsx`
- `web/src/features/keymaps/commands/file-command-actions.ts`
- `web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx`
- `web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-sync.ts`
- `web/src/features/git/components/diff/git-diff-editor-stack.tsx`
- `web/src/features/git/components/diff/git-diff-header.tsx`
- `web/src/features/git/components/git-view.tsx`
- `web/src/features/git/hooks/use-diff-editor-buffer.ts`
- `web/src/features/git/hooks/use-git-diff-data.ts`
- `web/src/features/terminal/components/terminal-container.tsx`
- `web/src/features/workspace/components/WorkspaceStepFooter.tsx`
- `web/src/features/workspace/stores/hooks/use-workspace-effects.ts`

**Pattern (React components/hooks):**

```typescript
// Remove:
import { useBufferStore } from "@/features/editor/stores/buffer-store";

// Add (if not already imported):
import { useWorkspaceStore } from "@/features/workspace/stores/workspace-context";

// Inside component/hook body, get the store:
const workspaceStore = useWorkspaceStore()

// Replace calls:
useBufferStore.getState().actions.openTerminalBuffer({ ... })
// →
workspaceStore.getState().bufferActions.openContent({ type: 'terminal', ... })
```

**Pattern (utility functions without React context — `navigation-actions.tsx`, `view-actions.tsx`, `file-command-actions.ts`):**

```typescript
// Remove:
import { useBufferStore } from "@/features/editor/stores/buffer-store";

// Add:
import { getActiveWorkspaceStoreRef } from "@/features/workspace/stores/workspace-store-ref";

// Replace:
useBufferStore.getState().actions.openGlobalSearchBuffer()
// →
getActiveWorkspaceStoreRef()?.getState().bufferActions.openContent({ type: 'globalSearch' })
```

- [ ] **Step 1: Read and migrate each file in the list**

For each file: read it, apply the appropriate pattern. Note: some files may also read `activeBufferId` or `buffers` from `useBufferStore` — see Task 10 for those patterns.

- [ ] **Step 2: Run TypeScript check**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npx tsc --noEmit 2>&1 | head -40
```

Expected: No errors in the migrated files.

- [ ] **Step 3: Commit**

```bash
git add \
  web/src/features/command-palette/components/command-palette.tsx \
  web/src/features/command-palette/constants/navigation-actions.tsx \
  web/src/features/command-palette/constants/view-actions.tsx \
  web/src/features/keymaps/commands/file-command-actions.ts \
  web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.tsx \
  web/src/features/file-explorer/file-explorer/hooks/use-file-explorer-sync.ts \
  web/src/features/git/components/diff/git-diff-editor-stack.tsx \
  web/src/features/git/components/diff/git-diff-header.tsx \
  web/src/features/git/components/git-view.tsx \
  web/src/features/git/hooks/use-diff-editor-buffer.ts \
  web/src/features/git/hooks/use-git-diff-data.ts \
  web/src/features/terminal/components/terminal-container.tsx \
  web/src/features/workspace/components/WorkspaceStepFooter.tsx \
  web/src/features/workspace/stores/hooks/use-workspace-effects.ts
git commit -m "feat: migrate openContent callers to workspace store"
```

---

## Task 10: Migrate Remaining Callers (activeBufferId readers + misc)

Migrate the remaining files that read `activeBufferId`, read buffers by ID, or use misc buffer store actions (pendingClose, closedHistory, setPinned, etc.).

**Files to migrate:**
- `web/src/features/git/components/git-inline-blame.tsx`
- `web/src/features/layout/components/footer/footer.tsx`
- `web/src/features/panes/components/empty-editor-state.tsx`
- `web/src/features/tabs/components/tab-context-menu.tsx`
- `web/src/features/panes/utils/pane-command-actions.ts`
- `web/src/features/settings/stores/whats-new-store.ts`
- `web/src/features/editor/utils/jump-navigation.ts`

**Patterns:**

**Reading global `activeBufferId` (React component):**
```typescript
// Before:
const activeBufferId = useBufferStore((state) => state.activeBufferId)
// After (reads from active pane):
const activePaneId = useWorkspaceStoreContext((state) => state.activePaneId)
const activeBufferId = useWorkspaceStoreContext(
  useCallback(
    (state) => findPaneGroup(state.paneRoot, state.activePaneId)?.activeBufferId ?? null,
    [],
  )
)
```

Add import: `import { findPaneGroup } from "@/features/panes/utils/pane-tree";`

**Reading buffer by ID (React component):**
```typescript
// Before:
const buffer = useBufferStore(useCallback(state => state.buffers.find(b => b.id === id), [id]))
// After:
const buffer = useWorkspaceStoreContext(useCallback(state => state.buffers.find(b => b.id === id), [id]))
```

**Utility function (`pane-command-actions.ts` line 11):**
```typescript
// Before:
const activeBuffer = useBufferStore.getState().buffers.find((buffer) => buffer.id === bufferId);
// After:
import { getActiveWorkspaceStoreRef } from "@/features/workspace/stores/workspace-store-ref";
const activeBuffer = getActiveWorkspaceStoreRef()?.getState().buffers.find(b => b.id === bufferId)
```

**Pending close / history (React component):**
```typescript
// Before:
useBufferStore.getState().actions.setPendingClose({ ... })
// After:
useWorkspaceStore().getState().bufferActions.setPendingClose({ ... })
```

- [ ] **Step 1: Read and migrate each file in the list**

For each file: read it, apply the appropriate pattern above.

- [ ] **Step 2: Run TypeScript check**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npx tsc --noEmit 2>&1 | head -40
```

Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add \
  web/src/features/git/components/git-inline-blame.tsx \
  web/src/features/layout/components/footer/footer.tsx \
  web/src/features/panes/components/empty-editor-state.tsx \
  web/src/features/tabs/components/tab-context-menu.tsx \
  web/src/features/panes/utils/pane-command-actions.ts \
  web/src/features/settings/stores/whats-new-store.ts \
  web/src/features/editor/utils/jump-navigation.ts
git commit -m "feat: migrate remaining buffer store callers (activeBufferId readers, misc)"
```

---

## Task 11: Delete Legacy Files

At this point, no production file should import from `buffer-store.ts` or `buffer-pane-sync.ts`. Verify, then delete.

**Files to delete:**
- `web/src/features/editor/stores/buffer-store.ts`
- `web/src/features/editor/stores/buffer-pane-sync.ts`
- `web/src/features/editor/stores/buffer-eviction.ts`

Note: `buffer-session-persistence.ts` is intentionally kept — `workspace-store.ts` now imports `saveSessionToStore` from it.

- [ ] **Step 1: Verify no production imports remain**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web/src && grep -r "from.*buffer-store\|from.*buffer-pane-sync\|from.*buffer-eviction\|from.*buffer-session-persistence" --include="*.ts" --include="*.tsx" -l | grep -v "__tests__"
```

Expected: **Empty output.** If any files are listed, migrate them before proceeding.

- [ ] **Step 2: Remove `registerExternalBuffer` from `buffer-slice.ts`**

In `web/src/features/workspace/stores/slices/buffer-slice.ts`, the `registerExternalBuffer` method is a legacy shim (Task 2 replaced `BufferActions` but if it was left in, remove it now). Verify it's not in the file from Task 2 — Task 2 rewrote the entire file without `registerExternalBuffer`. Run:

```bash
grep "registerExternalBuffer" /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web/src/features/workspace/stores/slices/buffer-slice.ts
```

Expected: No output. If present, delete the function.

- [ ] **Step 3: Delete the legacy files**

```bash
git rm \
  web/src/features/editor/stores/buffer-store.ts \
  web/src/features/editor/stores/buffer-pane-sync.ts \
  web/src/features/editor/stores/buffer-eviction.ts
```

- [ ] **Step 4: Run full TypeScript check**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npx tsc --noEmit 2>&1 | head -60
```

Expected: **Zero errors.** If there are errors, fix them before committing.

- [ ] **Step 5: Run all tests**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npx vitest run --reporter=verbose 2>&1 | tail -30
```

Expected: All non-buffer-store tests pass. Tests for `pane-activation.test.ts` and legacy buffer store tests will fail — those are fixed in Task 12.

- [ ] **Step 6: Commit**

```bash
git commit -m "feat: delete legacy buffer-store, buffer-pane-sync, eviction, and persistence files"
```

---

## Task 12: Update Test Files

Migrate the 9 test files that import from `useBufferStore`. Some test the deleted `pane-activation.ts` — those are rewritten to test workspace store behavior.

**Files to migrate:**
```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web/src && grep -r "from.*buffer-store\|from.*pane-activation" --include="*.test.ts" --include="*.test.tsx" -l
```

Run the above command to get the exact list. Typical files:
- `__tests__/features/panes/pane-activation.test.ts`
- `__tests__/features/editor/buffer-store.test.ts` (or similar)
- Other test files found by the grep

**Pattern for each test file:**

Read the file. Replace:
```typescript
// Before:
import { useBufferStore } from "@/features/editor/stores/buffer-store"
// After:
import { createWorkspaceStore } from "@/features/workspace/stores/workspace-store"
```

Replace store setup:
```typescript
// Before:
beforeEach(() => { useBufferStore.setState({ buffers: [], activeBufferId: null }) })
// After:
let store: ReturnType<typeof createWorkspaceStore>
beforeEach(() => { store = createWorkspaceStore('test-ws') })
```

Replace store access:
```typescript
// Before:
useBufferStore.getState().actions.openContent(spec)
useBufferStore.getState().buffers
// After:
store.getState().bufferActions.openContent(spec)
store.getState().buffers
```

For the `pane-activation.test.ts` — it tests that `activateBufferInPaneAndSync` syncs `useBufferStore.activeBufferId`. That behavior is gone. Rewrite as a test that `addBufferToPane` sets `paneGroup.activeBufferId`:

```typescript
// web/src/__tests__/features/workspace/stores/pane-activation-compat.test.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

describe('workspace store pane activation', () => {
  it('addBufferToPane with setActive=true sets pane activeBufferId', () => {
    const store = createWorkspaceStore('test-ws')
    const paneId = store.getState().activePaneId

    const bufferId = store.getState().bufferActions.openContent({
      type: 'editor', path: '/a.ts', name: 'a.ts', content: '',
    })

    expect(store.getState().paneActions.getPaneById(paneId)?.activeBufferId).toBe(bufferId)
  })
})
```

- [ ] **Step 1: Find all test files to migrate**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web/src && grep -r "from.*buffer-store\|from.*pane-activation" --include="*.test.ts" --include="*.test.tsx" -l
```

- [ ] **Step 2: Read and migrate each test file**

For each file: read it, apply the migration pattern. Delete tests for deleted APIs (like `activateBufferInPaneAndSync`).

- [ ] **Step 3: Run all tests**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npx vitest run --reporter=verbose 2>&1 | tail -50
```

Expected: All tests pass.

- [ ] **Step 4: Full TypeScript check**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npx tsc --noEmit
```

Expected: Zero errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/__tests__/
git commit -m "feat: update tests to use workspace store — legacy buffer store fully removed"
```

---

## Completion Verification

Run the full verification suite before declaring done:

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web

# 1. No legacy imports remain
grep -r "from.*buffer-store\|from.*buffer-pane-sync\|from.*pane-activation" \
  --include="*.ts" --include="*.tsx" src/ | grep -v "__tests__" | grep -v "node_modules"
# Expected: empty

# 2. Zero TypeScript errors
npx tsc --noEmit
# Expected: no output

# 3. All tests pass
npx vitest run --reporter=verbose 2>&1 | tail -20
# Expected: all pass
```

Files deleted (verify with `git status`):
- `web/src/features/editor/stores/buffer-store.ts` ✓
- `web/src/features/editor/stores/buffer-pane-sync.ts` ✓
- `web/src/features/editor/stores/buffer-eviction.ts` ✓
- `web/src/features/panes/utils/pane-activation.ts` ✓
