import type { StateCreator } from 'zustand'
import type { WindowPaneState } from '../window-pane-store.types'
import type {
  PaneContent,
  OpenEditorTabSpec,
  EditorContent,
  BranchReviewContent,
  CommitDiffContent,
  TerminalContent,
  MarkdownPreviewContent,
  HtmlPreviewContent,
  CsvPreviewContent,
  ExternalEditorContent,
  ClosedBuffer,
  PendingClose,
} from '@/features/panes/types/pane-content'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { EDITOR_CONSTANTS } from '@/features/editor/config/constants'
import { disposeBuffers, releaseUnreferencedBuffers } from '@/features/panes/lib/buffer-release'
import { placeTab } from './pane-actions/editor-tab-actions'
import { useSettingsStore } from '@/features/settings/store'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { nanoid } from 'nanoid'
import { bestEffort } from '@/lib/best-effort'

// ── Constants ────────────────────────────────────────────────────────

// A pane with zero editorTabIds falls back to rendering its New Tab surface
// for free (PaneContainer) — there is no placeholder buffer to protect any
// more, only the always-live externalEditor/terminal types stay exempt from
// auto-eviction.
const AUTO_EVICTION_PROTECTED = new Set<PaneContent['type']>(['externalEditor', 'terminal'])

// ── Actions ──────────────────────────────────────────────────────────

export interface OpenContentOptions {
  /** The pane the tab lands in (C8). Defaults to the focused pane; focus is
   *  never a precondition. */
  paneId?: string
}

export interface BufferActions {
  openContent(spec: OpenEditorTabSpec, opts?: OpenContentOptions): string
  closeBuffer(id: string): void
  renameBuffer(id: string, name: string): void
  setPinned(id: string, pinned: boolean): void
  setPreview(id: string, preview: boolean): void
  promotePreview(id: string): void
  getBufferById(id: string): PaneContent | undefined
  reopenLastClosedBuffer(): void
  setPendingClose(pc: PendingClose | null): void
  confirmPendingClose(): void
}

/** Sync the isUncloseable flag on editor tabs in a pane: the sole editor tab
 *  becomes uncloseable UNLESS the pane can safely lose it — either it also
 *  holds a chat (closing its last tab just collapses it to chat-only,
 *  `chatFillsPane` in pane-container.tsx) or it is one of several panes in a
 *  split (closing its last tab drops the whole pane from the layout,
 *  `dropEmptiedPanes` in pane-slice.ts). Only a chatless pane that is truly
 *  alone — nothing to collapse into, nothing to fall back to — still protects
 *  its last tab, since THAT pane has nowhere left to go but the bare "nothing
 *  is open" fallback screen forever. `canSafelyEmpty` is resolved by the
 *  caller (pane-slice.ts), which alone has the full layout tree this needs.
 *  Called whenever a pane's editorTabIds change (add/remove/move operations). */
export function syncSoleEditorTabCloseability(
  state: { buffers?: PaneContent[]; panes?: Record<string, { editorTabIds: string[] }> },
  paneId: string,
  canSafelyEmpty: boolean,
): void {
  const pane = state.panes?.[paneId]
  if (!pane || !Array.isArray(state.buffers)) return
  const sole = pane.editorTabIds.length === 1 && !canSafelyEmpty
  // Index once: these are immer drafts, so the map holds the same draft
  // objects and mutating through it still records the change.
  const byId = new Map(state.buffers.map((b) => [b.id, b]))
  for (const id of pane.editorTabIds) {
    const buf = byId.get(id)
    if (buf) buf.isUncloseable = sole
  }
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
  WindowPaneState,
  [['zustand/immer', never]],
  [],
  BufferSlice
> = (set, get) => {
  return {
    buffers: [],
    closedBuffersHistory: [],
    pendingClose: null,
    maxOpenTabs: EDITOR_CONSTANTS.MAX_OPEN_TABS,

    bufferActions: {
      openContent(spec, opts = {}) {
        const paneId = opts.paneId ?? get().activePaneId
        // Resolve the owning workspace once: an explicit spec.workspaceId wins
        // (the caller already knows — e.g. openFileContent's own wsId param),
        // commitDiff/branchReview fall back to their own (pre-existing) wsId
        // field, and everything else defaults to whichever workspace is
        // currently active. `buffers` is one flat, window-level list now (Task
        // 26) — every path-keyed lookup below must scope on this, or two
        // workspaces sharing a relative path (the common case: they're worktrees
        // of the same repo) would silently share one buffer.
        const workspaceId =
          spec.workspaceId ??
          (spec.type === 'branchReview' || spec.type === 'commitDiff' ? spec.wsId : undefined) ??
          getActiveWorkspaceId() ??
          ''

        // Deduplicate: return existing buffer id if already open
        const existing = (() => {
          if (spec.type === 'editor') {
            return get().buffers.find(
              (b) => b.type === 'editor' && b.path === spec.path && b.workspaceId === workspaceId,
            )
          }
          if (spec.type === 'branchReview') {
            return get().buffers.find(
              (b) =>
                b.type === 'branchReview' &&
                (b as BranchReviewContent).wsId === spec.wsId &&
                b.workspaceId === workspaceId,
            )
          }
          if (spec.type === 'commitDiff') {
            return get().buffers.find(
              (b) =>
                b.type === 'commitDiff' &&
                b.wsId === spec.wsId &&
                b.sha === spec.sha &&
                b.workspaceId === workspaceId,
            )
          }
          if (spec.type === 'terminal' && spec.sessionId) {
            return get().buffers.find(
              (b) => b.type === 'terminal' && (b as TerminalContent).sessionId === spec.sessionId,
            )
          }
          if (spec.type === 'markdownPreview') {
            return get().buffers.find(
              (b) =>
                b.type === 'markdownPreview' &&
                b.path === spec.path &&
                b.workspaceId === workspaceId,
            )
          }
          if (spec.type === 'htmlPreview') {
            return get().buffers.find(
              (b) =>
                b.type === 'htmlPreview' && b.path === spec.path && b.workspaceId === workspaceId,
            )
          }
          if (spec.type === 'csvPreview') {
            return get().buffers.find(
              (b) =>
                b.type === 'csvPreview' && b.path === spec.path && b.workspaceId === workspaceId,
            )
          }
          if (spec.type === 'externalEditor') {
            return get().buffers.find(
              (b) =>
                b.type === 'externalEditor' &&
                b.path === spec.path &&
                b.workspaceId === workspaceId,
            )
          }
          return undefined
        })()

        if (existing) {
          // A terminal buffer IS a live view onto ONE PTY. Opening it again must
          // REVEAL the view that already exists — jump to the pane that holds it —
          // never drop a SECOND copy into the active pane. Two views of the same
          // PTY race each other over the shared transport (the daemon serializes
          // its screen model to a client only at attach), and one of the two ends
          // up blank. Editors and everything else are safe to surface in the
          // active pane, so they keep the addEditorTabToPane path. (An agent chat
          // used to share this jump path too — it is no longer reachable here at
          // all: a chat is `PaneGroup.chatId`, never opened
          // through `openContent`.)
          if (existing.type === 'terminal') {
            const pane = get().paneActions.getPaneByEditorTabId(existing.id)
            if (pane) {
              get().paneActions.setActivePane(pane.id)
              get().paneActions.activateEditorTabInPane(pane.id, existing.id)
              return existing.id
            }
          }
          get().paneActions.addEditorTabToPane(paneId, existing)
          return existing.id
        }

        // Auto-evict when at max tabs (before creating a new buffer).
        //
        // ONE cap, enforced here. `maxOpenTabs` on this slice is the engine's
        // own budget; the user's `settings.maxOpenTabs` narrows it when they ask
        // for fewer tabs than that. Until now the setting was enforced a second
        // time, in the tab bar, by an effect that watched the workspace-wide
        // buffer list and closed tabs through the pane's own close handler — so
        // it only bit below the engine budget, and with two panes open BOTH tab
        // bars trimmed the same shared list.
        //
        // Task 26 fix round 1 (Critical 3): buffers are one flat window-wide
        // list now, shared across every workspace WorkspaceHost keeps
        // retained (up to RETENTION_CAP). Scoping the count AND the evictee
        // search to THIS buffer's own workspace keeps the cap per-workspace,
        // as it always was — otherwise opening a file in workspace A could
        // silently discard a tab (or, without the isDirty guard below,
        // unsaved work) belonging to workspace B, C, ... which the user isn't
        // even looking at.
        const settingCap = useSettingsStore.getState().settings.maxOpenTabs
        const cap = settingCap > 0 ? Math.min(get().maxOpenTabs, settingCap) : get().maxOpenTabs
        const workspaceBuffers = get().buffers.filter((b) => b.workspaceId === workspaceId)
        if (workspaceBuffers.length >= cap) {
          const evictee = workspaceBuffers.find(
            (b) =>
              !b.isPinned &&
              !AUTO_EVICTION_PROTECTED.has(b.type) &&
              !(isEditorContent(b) && b.isDirty),
          )
          if (evictee) {
            // The last pane letting go releases it (invariant C2).
            for (const pane of Object.values(get().panes)) {
              if (pane.editorTabIds.includes(evictee.id)) {
                get().paneActions.removeEditorTabFromPane(pane.id, evictee.id)
              }
            }
          }
        }

        const id = nanoid()

        // Build the new buffer object
        let buf: PaneContent

        if (spec.type === 'editor') {
          const isPreview = spec.isPreview ?? false
          buf = {
            id,
            type: 'editor',
            path: spec.path,
            name: spec.name,
            content: spec.content,
            savedContent: spec.content,
            isDirty: false,
            isVirtual: spec.isVirtual ?? false,
            language: spec.language,
            tokens: [],
            isPinned: false,
            isPreview,
            workspaceId,
          } satisfies EditorContent
        } else if (spec.type === 'branchReview') {
          buf = {
            id,
            type: 'branchReview',
            wsId: spec.wsId,
            name: spec.name,
            path: `branch-review://${spec.wsId}`,
            isPinned: false,
            isPreview: false,
            workspaceId,
          } satisfies BranchReviewContent
        } else if (spec.type === 'commitDiff') {
          buf = {
            id,
            type: 'commitDiff',
            wsId: spec.wsId,
            sha: spec.sha,
            name: spec.name,
            // One tab per commit per workspace: reopening the same commit
            // focuses the tab that is already there rather than stacking another.
            path: `commit-diff://${spec.wsId}/${spec.sha}`,
            isPinned: false,
            isPreview: false,
            workspaceId,
          } satisfies CommitDiffContent
        } else if (spec.type === 'terminal') {
          const terminalCount = get().buffers.filter((b) => b.type === 'terminal').length
          const sessionId = spec.sessionId ?? `terminal-tab-${Date.now()}`
          buf = {
            id,
            type: 'terminal',
            sessionId,
            path: spec.path ?? `terminal://${sessionId}`,
            name: spec.name ?? `Terminal ${terminalCount + 1}`,
            initialCommand: spec.command,
            workingDirectory: spec.workingDirectory,
            remoteConnectionId: spec.remoteConnectionId,
            isPinned: false,
            isPreview: false,
            workspaceId,
          } satisfies TerminalContent
        } else if (spec.type === 'markdownPreview') {
          buf = {
            id,
            type: 'markdownPreview',
            path: spec.path,
            name: spec.name,
            content: spec.content,
            sourceFilePath: spec.sourceFilePath,
            isPinned: false,
            isPreview: false,
            workspaceId,
          } satisfies MarkdownPreviewContent
        } else if (spec.type === 'htmlPreview') {
          buf = {
            id,
            type: 'htmlPreview',
            path: spec.path,
            name: spec.name,
            content: spec.content,
            sourceFilePath: spec.sourceFilePath,
            isPinned: false,
            isPreview: false,
            workspaceId,
          } satisfies HtmlPreviewContent
        } else if (spec.type === 'csvPreview') {
          buf = {
            id,
            type: 'csvPreview',
            path: spec.path,
            name: spec.name,
            content: spec.content,
            sourceFilePath: spec.sourceFilePath,
            isPinned: false,
            isPreview: false,
            workspaceId,
          } satisfies CsvPreviewContent
        } else {
          // spec.type === 'externalEditor'
          buf = {
            id,
            type: 'externalEditor',
            path: spec.path,
            name: spec.name,
            terminalConnectionId: spec.terminalConnectionId,
            isPinned: false,
            isPreview: false,
            workspaceId,
          } satisfies ExternalEditorContent
        }

        // Created and seated in one write: a buffer never exists without a
        // pane listing it (invariant C2). No pane, no buffer.
        let placed = false
        set((state) => {
          state.buffers.push(buf)
          placed = placeTab(state, paneId, id, spec.type === 'editor' && !!spec.isPreview)
          if (!placed) state.buffers.pop()
        })
        if (!placed) return ''

        return id
      },

      closeBuffer(id) {
        // A pane still listing `id` is a split sibling showing it live:
        // tearing it down would kill the sibling's content (a terminal's PTY)
        // out from under it. Pane writes already release a buffer the moment
        // its last pane lets go (invariant C2, see pane-slice); this is for
        // a buffer nothing lists any more.
        if (Object.values(get().panes).some((pane) => pane.editorTabIds.includes(id))) return
        let released: PaneContent[] = []
        set((state) => {
          released = releaseUnreferencedBuffers(state)
        })
        disposeBuffers(released)
      },

      // Rename an open buffer's tab label in place. `openContent` snapshots the
      // name at open time, so any content whose title can change AFTER the tab
      // exists (an agent chat auto-titled by the agent, or renamed by the user)
      // needs this to keep the tab in sync with its source of truth.
      renameBuffer(id, name) {
        set((state) => {
          const buf = state.buffers.find((b) => b.id === id)
          if (buf) buf.name = name
        })
      },

      setPinned(id, pinned) {
        set((state) => {
          const buf = state.buffers.find((b) => b.id === id)
          if (buf) buf.isPinned = pinned
        })
      },

      setPreview(id, preview) {
        set((state) => {
          const buf = state.buffers.find((b) => b.id === id)
          if (buf) buf.isPreview = preview
        })
      },

      promotePreview(id) {
        let found = false
        set((state) => {
          const buf = state.buffers.find((b) => b.id === id)
          if (buf) {
            buf.isPreview = false
            found = true
          }
        })
        if (found) get().paneActions.clearEditorTabPreviewEverywhere()
      },

      getBufferById(id) {
        return get().buffers.find((b) => b.id === id)
      },

      reopenLastClosedBuffer() {
        const entry = get().closedBuffersHistory[0]
        if (!entry) return
        set((state) => {
          state.closedBuffersHistory.shift()
        })
        const id = get().bufferActions.openContent({
          type: 'editor',
          path: entry.path,
          name: entry.name,
          content: '',
          workspaceId: entry.workspaceId,
        })
        // The history entry carries no content; load it from disk and fill the
        // buffer in place. Dynamic import avoids a slice → platform-controller
        // cycle. Skip the fill if the user already typed into the empty buffer.
        // Read from the CLOSED buffer's own workspace (not necessarily the
        // active one — the active workspace can change while the read is in
        // flight, and the same relative path in a sibling worktree holds
        // different content).
        bestEffort(
          import('@/features/file-system/controllers/platform').then(
            async ({ readWorkspaceFile }) => {
              try {
                const content = await readWorkspaceFile(entry.workspaceId, entry.path)
                set((state) => {
                  const buf = state.buffers.find((b) => b.id === id)
                  if (buf && buf.type === 'editor' && buf.content === '') {
                    buf.content = content
                    buf.savedContent = content
                    buf.isDirty = false
                  }
                })
              } catch {
                // File no longer exists — leave the empty buffer; saving will recreate it.
              }
            },
          ),
          'refill reopened buffer',
        )
      },

      setPendingClose(pc) {
        set((state) => {
          state.pendingClose = pc
        })
      },

      confirmPendingClose() {
        const pc = get().pendingClose
        if (!pc) return
        set((state) => {
          state.pendingClose = null
        })
        if (pc.type === 'single') {
          get().bufferActions.closeBuffer(pc.bufferId)
        }
        // Other close types (others, all, to-left, to-right) are handled by the
        // callers that set pendingClose — they call closeBuffer for each target
        // after confirmation. This resets the gate.
      },
    },
  }
}
