import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type {
  PaneContent,
  OpenContentSpec,
  EditorContent,
  CrowbarChatContent,
  BranchReviewContent,
  DiffContent,
  TerminalContent,
  NewTabContent,
  MarkdownPreviewContent,
  HtmlPreviewContent,
  CsvPreviewContent,
  ExternalEditorContent,
  ClosedBuffer,
  PendingClose,
} from '@/features/panes/types/pane-content'
import { shouldStartLsp, isEditorContent } from '@/features/panes/types/pane-content'
import { EDITOR_CONSTANTS } from '@/features/editor/config/constants'
import { fileUri } from '@/features/editor/lib/editor-uri'
import { useHistoryStore } from '@/features/editor/stores/history-store'
import { cleanupBufferHistoryTracking } from '@/features/editor/stores/buffer-history-tracking'
import type { WorkspaceStore } from '../workspace-store'
import { nanoid } from 'nanoid'

// ── Constants ────────────────────────────────────────────────────────

const AUTO_EVICTION_PROTECTED = new Set<PaneContent['type']>([
  'externalEditor',
  'terminal',
])

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
> = (set, get, api) => {
  // See pane-slice for why reaching `editorManager` through `api` is safe: it is
  // the non-reactive handle Object.assign'd onto the store after slice creation.
  const editorManagerOf = () => (api as unknown as WorkspaceStore).editorManager

  return ({
  buffers: [],
  closedBuffersHistory: [],
  pendingClose: null,
  maxOpenTabs: EDITOR_CONSTANTS.MAX_OPEN_TABS,

  bufferActions: {
    openContent(spec) {
      // Deduplicate: return existing buffer id if already open
      const existing = (() => {
        if (spec.type === 'editor') {
          return get().buffers.find((b) => b.type === 'editor' && b.path === spec.path)
        }
        if (spec.type === 'crowbarChat') {
          return get().buffers.find(
            (b) => b.type === 'crowbarChat' && (b as CrowbarChatContent).wsId === spec.wsId,
          )
        }
        if (spec.type === 'branchReview') {
          return get().buffers.find(
            (b) => b.type === 'branchReview' && (b as BranchReviewContent).wsId === spec.wsId,
          )
        }
        if (spec.type === 'diff') {
          return get().buffers.find((b) => b.type === 'diff' && b.path === spec.path)
        }
        if (spec.type === 'terminal' && spec.sessionId) {
          return get().buffers.find(
            (b) => b.type === 'terminal' && (b as TerminalContent).sessionId === spec.sessionId,
          )
        }
        if (spec.type === 'markdownPreview') {
          return get().buffers.find((b) => b.type === 'markdownPreview' && b.path === spec.path)
        }
        if (spec.type === 'htmlPreview') {
          return get().buffers.find((b) => b.type === 'htmlPreview' && b.path === spec.path)
        }
        if (spec.type === 'csvPreview') {
          return get().buffers.find((b) => b.type === 'csvPreview' && b.path === spec.path)
        }
        if (spec.type === 'externalEditor') {
          return get().buffers.find((b) => b.type === 'externalEditor' && b.path === spec.path)
        }
        return undefined
      })()

      if (existing) {
        get().paneActions.addBufferToPane(get().activePaneId, existing.id, true)
        return existing.id
      }

      // Auto-evict when at max tabs (before creating a new buffer)
      if (get().buffers.length >= get().maxOpenTabs) {
        const evictee = get().buffers.find(
          (b) => !b.isPinned && !AUTO_EVICTION_PROTECTED.has(b.type),
        )
        if (evictee) {
          const allPanes = Object.values(get().panes)
          for (const pane of allPanes) {
            if (pane.bufferIds.includes(evictee.id)) {
              get().paneActions.removeBufferFromPane(pane.id, evictee.id, true)
            }
          }
          set((state) => {
            state.buffers = state.buffers.filter((b) => b.id !== evictee.id)
          })
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
          isActive: false,
        } satisfies EditorContent
      } else if (spec.type === 'crowbarChat') {
        buf = {
          id,
          type: 'crowbarChat',
          wsId: spec.wsId,
          name: spec.name,
          path: '',
          isPinned: false,
          isPreview: false,
          isActive: false,
        } satisfies CrowbarChatContent
      } else if (spec.type === 'branchReview') {
        buf = {
          id,
          type: 'branchReview',
          wsId: spec.wsId,
          name: spec.name,
          path: `branch-review://${spec.wsId}`,
          isPinned: false,
          isPreview: false,
          isActive: false,
        } satisfies BranchReviewContent
      } else if (spec.type === 'diff') {
        buf = {
          id,
          type: 'diff',
          path: spec.path,
          name: spec.name,
          content: spec.content,
          savedContent: spec.content,
          diffData: spec.diffData,
          isPinned: false,
          isPreview: false,
          isActive: false,
        } satisfies DiffContent
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
          isActive: false,
        } satisfies TerminalContent
      } else if (spec.type === 'newTab') {
        buf = {
          id,
          type: 'newTab',
          path: '',
          name: 'New Tab',
          isPinned: false,
          isPreview: false,
          isActive: false,
        } satisfies NewTabContent
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
          isActive: false,
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
          isActive: false,
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
          isActive: false,
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
          isActive: false,
        } satisfies ExternalEditorContent
      }

      set((state) => {
        state.buffers.push(buf)
      })
      get().paneActions.addBufferToPane(get().activePaneId, id, true)
      if (spec.type === 'editor' && spec.isPreview) {
        get().paneActions.setPanePreviewBuffer(get().activePaneId, id)
      }

      return id
    },

    closeBuffer(id) {
      const buf = get().buffers.find((b) => b.id === id)
      // Closing a terminal tab is final (terminals never enter the undo-close
      // history) — terminate the backend PTY so shell processes don't leak.
      // Dynamic import avoids a workspace-slice → terminal-feature cycle.
      if (buf && buf.type === 'terminal') {
        const { sessionId } = buf as TerminalContent
        const workspaceId = get().workspaceId
        void import('@/features/terminal/lib/kill-terminal-session').then(
          async ({ killTerminalSession }) => {
            await killTerminalSession(sessionId).catch(() => {})
            // Clear the reconnect map entry so a stale connectionId can't be
            // picked up if the same tab sessionId is reused in a later session.
            const { clearReconnect } = await import('@/features/terminal/lib/terminal-reconnect-map')
            clearReconnect(workspaceId, sessionId)
          },
        )
      }
      // Closing a chat tab is final too — drop its conversation store so the
      // streamed turns[] (full agent/user message text) don't leak for the
      // lifetime of the session. The store is keyed by the chat's wsId (the
      // nanoid minted at open). Dynamic import avoids a workspace-slice →
      // markdown-chat-feature cycle, mirroring the terminal branch above.
      if (buf && buf.type === 'crowbarChat') {
        const { wsId } = buf as CrowbarChatContent
        void import('@/features/markdown-chat/stores/conversation-store').then(
          ({ destroyConversationStore }) => destroyConversationStore(wsId),
        )
      }
      if (buf && shouldStartLsp(buf)) {
        set((state) => {
          const entry: ClosedBuffer = { path: buf.path, name: buf.name, isPinned: buf.isPinned }
          state.closedBuffersHistory.unshift(entry)
          if (state.closedBuffersHistory.length > EDITOR_CONSTANTS.MAX_CLOSED_BUFFERS_HISTORY) {
            state.closedBuffersHistory.pop()
          }
        })
      }
      // Release the held Monaco model for any pane that STILL holds this buffer.
      // The canonical close paths call `removeBufferFromPane` first (which already
      // released for that pane and stripped the id), so this only fires for panes
      // that were skipped (direct closeBuffer callers) or other panes holding the
      // same file — release exactly once per holding pane. Disposes the model when
      // the last holder releases, so a reopen reads fresh content (no stale model).
      if (buf && isEditorContent(buf)) {
        const uri = fileUri(buf.path)
        const manager = editorManagerOf()
        for (const pane of Object.values(get().panes ?? {})) {
          if (pane.bufferIds.includes(id)) manager?.closeBuffer(pane.id, uri)
        }
      }
      // Free git-blame data accumulated for this file so per-file Maps don't
      // grow unbounded across a long session. Dynamic import mirrors the pattern
      // used above for terminal/chat to avoid circular slice → git-feature deps.
      if (buf && isEditorContent(buf)) {
        const filePath = buf.path
        void import('@/features/git/stores/git-blame-store').then(({ useGitBlameStore }) => {
          useGitBlameStore.getState().clearBlameForFile(filePath)
        })
      }
      // Free full-content history snapshots so closed buffers don't leak memory.
      // clearHistory drops up to 100 HistoryEntry objects each holding a full copy
      // of the file text — the dominant source of memory growth in long sessions.
      cleanupBufferHistoryTracking(id)
      useHistoryStore.getState().actions.clearHistory(id)
      set((state) => {
        state.buffers = state.buffers.filter((b) => b.id !== id)
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
      if (found) get().paneActions.clearPreviewBufferEverywhere(id)
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
      })
      // The history entry carries no content; load it from disk and fill the
      // buffer in place. Dynamic import avoids a slice → platform-controller
      // cycle. Skip the fill if the user already typed into the empty buffer.
      // Read from this store's own workspace: the active workspace can change
      // while the read is in flight, and the same relative path in a sibling
      // worktree holds different content.
      void import('@/features/file-system/controllers/platform').then(
        async ({ readWorkspaceFile }) => {
          try {
            const content = await readWorkspaceFile(get().workspaceId, entry.path)
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
})
}
