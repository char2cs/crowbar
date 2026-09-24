import { isEditorContent } from '@/features/panes/types/pane-content'
import { fileUri } from '@/features/editor/lib/editor-uri'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { getWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { removePane } from '@/features/panes/lib/view-ops'
import { focusPane, homeOf, layoutOf, touchPane } from '@/features/panes/lib/view-state'
import { isPaneEmpty } from '@/features/panes/lib/view-selectors'
import { syncSoleEditorTabCloseability } from '../buffer-slice'
import type { WindowPaneState } from '../../window-pane-store.types'
import type { PaneActions } from '../pane-slice'
import type { PaneGet, PaneSet } from './context'

type EditorTabActions = Pick<
  PaneActions,
  | 'activateEditorTabInPane'
  | 'activateChatInPane'
  | 'addEditorTabToPane'
  | 'removeEditorTabFromPane'
  | 'moveEditorTabToPane'
  | 'setEditorTabPreview'
  | 'setEditorTabPinned'
  | 'setPaneLocked'
  | 'reorderEditorTabs'
  | 'clearEditorTabPreviewEverywhere'
  | 'switchToNextEditorTab'
  | 'switchToPreviousEditorTab'
>

/** A chatless pane may lose its last tab only when a sibling takes its place. */
function paneCanSafelyLoseLastTab(state: WindowPaneState, paneId: string): boolean {
  if (state.panes[paneId]?.chatId) return true
  const home = homeOf(state, paneId)
  return !!home && getAllLeafIds(layoutOf(state, home)).length > 1
}

/** The editor tabs a pane holds beside its chat. */
export function createEditorTabActions(set: PaneSet, get: PaneGet): EditorTabActions {
  // Never CREATE a workspace store just to reach its editor manager — a
  // buffer can outlive its workspace, and a store created here would leak.
  const releaseEditorTabModel = (paneId: string, tabId: string) => {
    const buf = get().buffers?.find((b) => b.id === tabId)
    if (!buf || !isEditorContent(buf) || !buf.path) return
    getWorkspaceStore(buf.workspaceId)?.editorManager?.closeBuffer(
      paneId,
      fileUri(buf.workspaceId, buf.path),
    )
  }

  return {
    activateEditorTabInPane(paneId, tabId) {
      set((state) => {
        const pane = state.panes[paneId]
        // Only a tab the pane HOLDS: an id outside `editorTabIds` renders an
        // empty pane under a populated tab strip.
        if (!pane || !pane.editorTabIds.includes(tabId)) return
        pane.activeEditorTabId = tabId
        pane.chatSelected = false
        state.activePaneId = paneId
        touchPane(state, paneId)
      })
    },

    activateChatInPane(paneId) {
      set((state) => {
        const pane = state.panes[paneId]
        if (!pane || !pane.chatId) return
        // Leaves `activeEditorTabId` alone so switching back never remounts it.
        pane.chatSelected = true
        state.activePaneId = paneId
        touchPane(state, paneId)
      })
    },

    addEditorTabToPane(paneId, tab) {
      set((state) => {
        const pane = state.panes[paneId]
        if (!pane) return
        const hadEditorTabs = pane.editorTabIds.length > 0
        if (!pane.editorTabIds.includes(tab.id)) pane.editorTabIds.push(tab.id)
        pane.activeEditorTabId = tab.id
        // Respect a split the user already toggled off.
        if (!hadEditorTabs) pane.editorOpen = true
        pane.chatSelected = false
        syncSoleEditorTabCloseability(state, paneId, paneCanSafelyLoseLastTab(state, paneId))
      })
    },

    removeEditorTabFromPane(paneId, tabId) {
      if (get().panes[paneId]?.editorTabIds.includes(tabId)) {
        releaseEditorTabModel(paneId, tabId)
      }
      set((state) => {
        const pane = state.panes[paneId]
        if (!pane) return
        const closedIndex = pane.editorTabIds.indexOf(tabId)
        const wasActive = pane.activeEditorTabId === tabId
        pane.editorTabIds = pane.editorTabIds.filter((id) => id !== tabId)
        if (wasActive) {
          // The adjacent tab that still has content; with no buffer list
          // (slice-only tests) every id counts as alive.
          const known = state.buffers
          const isAlive = (id: string) => !Array.isArray(known) || known.some((b) => b.id === id)
          const alive = pane.editorTabIds.filter(isAlive)
          const rightNeighbor = pane.editorTabIds.slice(closedIndex).find(isAlive)
          pane.activeEditorTabId = rightNeighbor ?? alive[alive.length - 1] ?? null
        }
        if (pane.editorTabIds.length === 0) pane.editorOpen = false
        syncSoleEditorTabCloseability(state, paneId, paneCanSafelyLoseLastTab(state, paneId))
        if (isPaneEmpty(pane) && paneCanSafelyLoseLastTab(state, paneId)) removePane(state, paneId)
      })
    },

    moveEditorTabToPane(tabId, fromPaneId, toPaneId) {
      set((state) => {
        const fromPane = state.panes[fromPaneId]
        const toPane = state.panes[toPaneId]
        if (!fromPane || !toPane) return
        fromPane.editorTabIds = fromPane.editorTabIds.filter((id) => id !== tabId)
        if (fromPane.activeEditorTabId === tabId) {
          fromPane.activeEditorTabId = fromPane.editorTabIds[0] ?? null
        }
        if (fromPane.editorTabIds.length === 0) fromPane.editorOpen = false

        if (!toPane.editorTabIds.includes(tabId)) toPane.editorTabIds.push(tabId)
        toPane.activeEditorTabId = tabId
        toPane.editorOpen = true

        focusPane(state, toPaneId)
        syncSoleEditorTabCloseability(
          state,
          fromPaneId,
          paneCanSafelyLoseLastTab(state, fromPaneId),
        )
        syncSoleEditorTabCloseability(state, toPaneId, paneCanSafelyLoseLastTab(state, toPaneId))
      })
    },

    setEditorTabPreview(paneId, tabId) {
      set((state) => {
        const pane = state.panes[paneId]
        if (!pane || !pane.editorTabIds.includes(tabId)) return
        if (!Array.isArray(state.buffers)) return
        const bufferById = new Map(state.buffers.map((b) => [b.id, b]))
        for (const id of pane.editorTabIds) {
          const buf = bufferById.get(id)
          if (buf) buf.isPreview = id === tabId
        }
      })
    },

    setEditorTabPinned(paneId, tabId, pinned) {
      set((state) => {
        const pane = state.panes[paneId]
        if (!pane || !pane.editorTabIds.includes(tabId)) return
        if (!Array.isArray(state.buffers)) return
        const buf = state.buffers.find((b) => b.id === tabId)
        if (buf) buf.isPinned = pinned
      })
    },

    setPaneLocked(paneId, locked) {
      set((state) => {
        const pane = state.panes[paneId]
        if (pane) pane.locked = locked
      })
    },

    reorderEditorTabs(paneId, tabId, targetIndex) {
      set((state) => {
        const pane = state.panes[paneId]
        if (!pane) return
        const ids = [...pane.editorTabIds]
        const startIndex = ids.indexOf(tabId)
        if (startIndex === -1) return
        const [moved] = ids.splice(startIndex, 1)
        const clampedTarget = Math.max(0, Math.min(targetIndex, ids.length))
        ids.splice(clampedTarget, 0, moved)
        pane.editorTabIds = ids
      })
    },

    clearEditorTabPreviewEverywhere() {
      set((state) => {
        if (!Array.isArray(state.buffers)) return
        for (const buf of state.buffers) buf.isPreview = false
      })
    },

    switchToNextEditorTab(paneId) {
      const pane = get().panes[paneId]
      if (!pane || pane.editorTabIds.length <= 1) return
      const curr = pane.activeEditorTabId ? pane.editorTabIds.indexOf(pane.activeEditorTabId) : -1
      get().paneActions.activateEditorTabInPane(
        pane.id,
        pane.editorTabIds[(curr + 1) % pane.editorTabIds.length],
      )
    },

    switchToPreviousEditorTab(paneId) {
      const pane = get().panes[paneId]
      if (!pane || pane.editorTabIds.length <= 1) return
      const curr = pane.activeEditorTabId ? pane.editorTabIds.indexOf(pane.activeEditorTabId) : 0
      get().paneActions.activateEditorTabInPane(
        pane.id,
        pane.editorTabIds[(curr - 1 + pane.editorTabIds.length) % pane.editorTabIds.length],
      )
    },
  }
}
