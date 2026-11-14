import type { StateCreator } from 'zustand'
import type { WindowPaneState } from '../window-pane-store.types'
import type {
  PaneContent,
  OpenEditorTabSpec,
  BranchReviewView,
  ClosedBuffer,
  PendingClose,
} from '@/features/panes/types/pane-content'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { EDITOR_CONSTANTS } from '@/features/editor/config/constants'
import { disposeBuffers, releaseUnreferencedBuffers } from '@/features/panes/lib/buffer-release'
import { placeTab } from './pane-actions/editor-tab-actions'
import { findOpenBuffer, ownerOf, specWorkspaceId } from './find-open-buffer'
import { newBuffer } from './new-buffer'
import { useSettingsStore } from '@/features/settings/store'
import { documentRefOf } from '@/features/editor/documents/document-ref'
import type { DocumentRef } from '@/lib/api/documents'
import { nanoid } from 'nanoid'
import { report } from '@/lib/errors/report'
import { browserTabName } from '@/features/browser/lib/browser-tab-name'

// ── Constants ────────────────────────────────────────────────────────

// A live terminal is never auto-evicted; a pane emptied by eviction just shows its New Tab
// surface.
const AUTO_EVICTION_PROTECTED = new Set<PaneContent['type']>(['terminal'])

// ── Actions ──────────────────────────────────────────────────────────

interface OpenContentOptions {
  /** The pane the tab lands in (C8). Defaults to the focused pane; focus is
   *  never a precondition. */
  paneId?: string
  /** A tab in that pane this one takes the place of: a New Tab page's choice. */
  replaceTabId?: string
}

export interface BufferActions {
  openContent(spec: OpenEditorTabSpec, opts?: OpenContentOptions): string
  closeBuffer(id: string): void
  renameBuffer(id: string, name: string): void
  /** An untitled buffer saved as `doc` becomes that document's tab. */
  rebindEditorBuffer(id: string, doc: DocumentRef, name: string): void
  setBranchReviewView(id: string, view: BranchReviewView): void
  setBrowserLocation(id: string, patch: { url?: string; title?: string; loading?: boolean }): void
  setPinned(id: string, pinned: boolean): void
  setPreview(id: string, preview: boolean): void
  promotePreview(id: string): void
  getBufferById(id: string): PaneContent | undefined
  reopenLastClosedBuffer(): void
  setPendingClose(pc: PendingClose | null): void
  confirmPendingClose(): void
}

/** A pane's sole editor tab is uncloseable unless the pane can lose it (it holds a chat, or is
 *  one split of several). pane-slice resolves `canSafelyEmpty` — only it sees the layout tree —
 *  and calls this whenever a pane's editorTabIds change. */
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
        const workspaceId = specWorkspaceId(spec)
        const existing = findOpenBuffer(
          get().buffers,
          spec,
          get().panes[paneId]?.editorTabIds ?? [],
        )

        // A terminal is never seated in a second pane; see below.
        if (existing && opts.replaceTabId && existing.type !== 'terminal') {
          let released: PaneContent[] = []
          set((state) => {
            placeTab(state, paneId, existing.id, false, opts.replaceTabId)
            released = releaseUnreferencedBuffers(state)
          })
          disposeBuffers(released)
          return existing.id
        }

        if (existing) {
          // Reopening a terminal jumps to the pane holding it, never seats a second view: two
          // views of one PTY race over the transport (the daemon sends its screen model only
          // at attach) and one goes blank. Other content is safe to show in the active pane.
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

        // The one tab cap: the engine budget, narrowed by settings.maxOpenTabs. Buffers are one
        // window-wide list, so count and evict within this buffer's workspace — opening a file
        // must never discard a tab in a workspace the user isn't looking at.
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

        const terminalCount = get().buffers.filter((b) => b.type === 'terminal').length
        const buf = newBuffer(spec, id, workspaceId, terminalCount)

        // Created and seated in one write: a buffer never exists without a
        // pane listing it (invariant C2). No pane, no buffer.
        let placed = false
        let released: PaneContent[] = []
        set((state) => {
          state.buffers.push(buf)
          const preview = spec.type === 'editor' && !!spec.isPreview
          placed = placeTab(state, paneId, id, preview, opts.replaceTabId)
          if (!placed) state.buffers.pop()
          else if (opts.replaceTabId) released = releaseUnreferencedBuffers(state)
        })
        disposeBuffers(released)
        if (!placed) return ''

        return id
      },

      closeBuffer(id) {
        // A pane still listing `id` shows it live (tearing it down would kill a terminal's PTY).
        // Pane writes release a buffer when its last pane lets go (C2); this only sweeps
        // buffers nothing lists.
        if (Object.values(get().panes).some((pane) => pane.editorTabIds.includes(id))) return
        let released: PaneContent[] = []
        set((state) => {
          released = releaseUnreferencedBuffers(state)
        })
        disposeBuffers(released)
      },

      // `openContent` snapshots the name at open time; content retitled after that
      // keeps its tab label in sync through this.
      renameBuffer(id, name) {
        set((state) => {
          const buf = state.buffers.find((b) => b.id === id)
          if (buf) buf.name = name
        })
      },

      rebindEditorBuffer(id, doc, name) {
        set((state) => {
          const buf = state.buffers.find((b) => b.id === id)
          if (!buf || !isEditorContent(buf)) return
          buf.authority = doc.authority
          buf.path = doc.path
          buf.workspaceId = ownerOf(doc)
          buf.name = name
          buf.isVirtual = false
        })
      },

      setBranchReviewView(id, view) {
        set((state) => {
          const buf = state.buffers.find((b) => b.id === id)
          if (buf?.type === 'branchReview') buf.view = view
        })
      },

      // The webview is the source of truth for where it is; this stores what it
      // reports so the tab strip and a re-mounted pane agree with it.
      setBrowserLocation(id, patch) {
        set((state) => {
          const buf = state.buffers.find((b) => b.id === id)
          if (!buf || buf.type !== 'browser') return
          if (patch.url !== undefined && patch.url !== buf.url) {
            buf.url = patch.url
            buf.title = undefined
          }
          if (patch.title !== undefined) buf.title = patch.title
          if (patch.loading !== undefined) buf.loading = patch.loading
          buf.name = browserTabName(buf.title, buf.url)
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
        // Dynamic import keeps the document API out of the slice's static graph.
        void import('@/features/editor/documents/open-document')
          .then(({ openDocument }) => openDocument(documentRefOf(entry)))
          .catch((err: unknown) => report(err, { action: 'Reopen the closed tab' }))
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
        get().paneActions.removeEditorTabFromPane(pc.paneId, pc.bufferId)
        get().bufferActions.closeBuffer(pc.bufferId)
      },
    },
  }
}
