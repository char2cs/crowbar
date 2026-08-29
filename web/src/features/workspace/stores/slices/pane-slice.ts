import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type { WorkspaceStore } from '../workspace-store'
import { isEditorContent, type EditorTabBase } from '@/features/panes/types/pane-content'
import { fileUri } from '@/features/editor/lib/editor-uri'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import type {
  PaneGroup,
  LayoutNode,
  SplitDirection,
  SplitPlacement,
} from '@/features/panes/types/pane'
import {
  createLeaf,
  splitLayout,
  closeLayout,
  findLeaf,
  findSplit,
  getAllLeafIds,
  distributeSplit,
  resizeFlattenedLayout,
  normalizeLayout,
  getAdjacentLeafId,
} from '@/features/panes/utils/pane-layout'

export interface PaneActions {
  splitPane(
    paneId: string,
    direction: SplitDirection,
    bufferId?: string,
    placement?: SplitPlacement,
  ): string | null
  closePane(paneId: string): void
  setActivePane(paneId: string): void
  activateEditorTabInPane(paneId: string, tabId: string): void
  addEditorTabToPane(paneId: string, tab: EditorTabBase): void
  removeEditorTabFromPane(paneId: string, tabId: string): void
  moveEditorTabToPane(tabId: string, fromPaneId: string, toPaneId: string): void
  setEditorTabPreview(paneId: string, tabId: string): void
  setEditorTabPinned(paneId: string, tabId: string, pinned: boolean): void
  setPaneLocked(paneId: string, locked: boolean): void
  reorderEditorTabs(paneId: string, tabId: string, targetIndex: number): void
  resizePaneSplit(splitId: string, index: number, sizes: [number, number]): void
  distributePaneSplit(splitId: string): void
  togglePaneFullscreen(paneId: string): void
  exitPaneFullscreen(): void
  getAllPaneGroups(): PaneGroup[]
  getPaneById(paneId: string): PaneGroup | null
  getPaneByEditorTabId(tabId: string): PaneGroup | null
  getActivePane(): PaneGroup | null
  clearEditorTabPreviewEverywhere(): void
  switchToNextEditorTab(paneId: string): void
  switchToPreviousEditorTab(paneId: string): void
  navigateToPane(direction: 'left' | 'right' | 'up' | 'down'): void
  /** The one write path for what chat a pane holds. */
  setPaneChat(paneId: string, chatId: string | null, runnerId: string | null): void
}

export interface PaneSlice {
  panes: Record<string, PaneGroup>
  rootLayout: LayoutNode
  bottomLayout: LayoutNode
  activePaneId: string
  mostRecentActivePaneIds: string[]
  fullscreenPaneId: string | null
  paneActions: PaneActions
}

function makeRootLeaf(): PaneGroup {
  return {
    id: ROOT_PANE_ID,
    type: 'group',
    chatId: null,
    runnerId: null,
    editorTabIds: [],
    activeEditorTabId: null,
    editorOpen: false,
  }
}

function makeBottomLeaf(): PaneGroup {
  return {
    id: BOTTOM_PANE_ID,
    type: 'group',
    chatId: null,
    runnerId: null,
    editorTabIds: [],
    activeEditorTabId: null,
    editorOpen: false,
  }
}

function layoutContainsPane(layout: LayoutNode, paneId: string): boolean {
  return findLeaf(layout, paneId) !== null
}

function getLayoutKey(
  state: Pick<PaneSlice, 'rootLayout' | 'bottomLayout'>,
  paneId: string,
): 'rootLayout' | 'bottomLayout' {
  if (layoutContainsPane(state.rootLayout, paneId)) return 'rootLayout'
  if (layoutContainsPane(state.bottomLayout, paneId)) return 'bottomLayout'
  return paneId === BOTTOM_PANE_ID ? 'bottomLayout' : 'rootLayout'
}

export const createPaneSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  PaneSlice
> = (set, get, api) => {
  // `api` is the same object onto which `createWorkspaceStore` later attaches the
  // non-reactive `editorManager` handle (via Object.assign). It exists by the time
  // any action runs, so the cast is safe and keeps the editor lib decoupled from
  // the store state (the slice never imports editor *components*).
  const editorManagerOf = () => (api as unknown as WorkspaceStore).editorManager

  /** Release the held Monaco model for `tabId` in `paneId` (editor tabs only). A
   *  no-op when the tab isn't an editor or the pane didn't hold it (the manager
   *  guards on `held`). Disposes the model when no pane still holds it, so
   *  closing a tab frees its model and a reopen reads fresh content. */
  const releaseEditorTabModel = (paneId: string, tabId: string) => {
    const buf = get().buffers?.find((b) => b.id === tabId)
    if (!buf || !isEditorContent(buf) || !buf.path) return
    editorManagerOf()?.closeBuffer(paneId, fileUri(buf.path))
  }

  return {
    panes: { [ROOT_PANE_ID]: makeRootLeaf(), [BOTTOM_PANE_ID]: makeBottomLeaf() },
    rootLayout: createLeaf(ROOT_PANE_ID),
    bottomLayout: createLeaf(BOTTOM_PANE_ID),
    activePaneId: ROOT_PANE_ID,
    mostRecentActivePaneIds: [ROOT_PANE_ID],
    fullscreenPaneId: null,

    paneActions: {
      splitPane(paneId, direction, bufferId?, placement = 'after') {
        let newPaneId: string | null = null
        set((state) => {
          const key = getLayoutKey(state, paneId)
          const result = splitLayout(state[key], paneId, direction, placement)
          if (!result) return
          state[key] = result.layout
          newPaneId = result.newPaneId
          state.panes[newPaneId] = {
            id: newPaneId,
            type: 'group',
            chatId: null,
            runnerId: null,
            editorTabIds: bufferId ? [bufferId] : [],
            activeEditorTabId: bufferId ?? null,
            editorOpen: Boolean(bufferId),
          }
          state.activePaneId = newPaneId
          state.mostRecentActivePaneIds = [newPaneId, ...state.mostRecentActivePaneIds]
        })
        return newPaneId
      },

      closePane(paneId) {
        set((state) => {
          const key = getLayoutKey(state, paneId)
          const closingPane = state.panes[paneId]
          const result = closeLayout(state[key], paneId)
          if (result !== null) {
            state[key] = normalizeLayout(result)
            const remainingIds = getAllLeafIds(state[key])
            const fallbackId =
              remainingIds[0] ?? (key === 'rootLayout' ? ROOT_PANE_ID : BOTTOM_PANE_ID)
            if (closingPane) {
              const fp = state.panes[fallbackId]
              if (fp) {
                const existingTabIds = new Set(fp.editorTabIds)
                for (const tabId of closingPane.editorTabIds) {
                  if (existingTabIds.has(tabId)) continue
                  fp.editorTabIds.push(tabId)
                  existingTabIds.add(tabId)
                }
                if (fp.editorTabIds.length > 0) fp.editorOpen = true
                // Only adopt the closing pane's active tab if it actually
                // survived the merge above — otherwise `fp` keeps whichever tab
                // it already had active rather than pointing at an id it
                // doesn't hold.
                if (
                  state.activePaneId === paneId &&
                  closingPane.activeEditorTabId &&
                  fp.editorTabIds.includes(closingPane.activeEditorTabId)
                ) {
                  fp.activeEditorTabId = closingPane.activeEditorTabId
                }
              }
            }
            if (state.activePaneId === paneId) state.activePaneId = fallbackId
          } else {
            const fallbackId = key === 'rootLayout' ? ROOT_PANE_ID : BOTTOM_PANE_ID
            state.panes[fallbackId] = key === 'rootLayout' ? makeRootLeaf() : makeBottomLeaf()
            state[key] = createLeaf(fallbackId)
            if (state.activePaneId === paneId) state.activePaneId = ROOT_PANE_ID
          }
          delete state.panes[paneId]
          state.mostRecentActivePaneIds = state.mostRecentActivePaneIds.filter(
            (id) => id !== paneId,
          )
          if (state.fullscreenPaneId === paneId) state.fullscreenPaneId = null
        })
      },

      setActivePane(paneId) {
        set((state) => {
          state.activePaneId = paneId
          state.mostRecentActivePaneIds = [
            paneId,
            ...state.mostRecentActivePaneIds.filter((id) => id !== paneId),
          ]
        })
      },

      activateEditorTabInPane(paneId, tabId) {
        set((state) => {
          const pane = state.panes[paneId]
          if (!pane) return
          // Only a tab the pane actually HOLDS may be activated. A pane resolves
          // its content as `editorTabs.find(t => t.id === activeEditorTabId)`,
          // so an id outside `editorTabIds` draws the empty-pane fallback WITH a
          // populated tab strip above it: tabs visible, none selected, nothing
          // rendered. Same class of defence the old buffer-slice era needed.
          if (!pane.editorTabIds.includes(tabId)) return
          pane.activeEditorTabId = tabId
          state.activePaneId = paneId
          state.mostRecentActivePaneIds = [
            paneId,
            ...state.mostRecentActivePaneIds.filter((id) => id !== paneId),
          ]
        })
      },

      addEditorTabToPane(paneId, tab) {
        set((state) => {
          const pane = state.panes[paneId]
          if (!pane) return
          if (!pane.editorTabIds.includes(tab.id)) pane.editorTabIds.push(tab.id)
          pane.activeEditorTabId = tab.id
          pane.editorOpen = true
        })
      },

      removeEditorTabFromPane(paneId, tabId) {
        // Release this pane's held Monaco model BEFORE mutating state (reads the
        // buffer path, which still exists). Guarded internally: a no-op if the
        // pane never held it.
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
            // Activate the ADJACENT tab so a close keeps you on a nearby tab
            // (VS Code-style): the tab that shifted into the closed slot (the
            // right neighbor), else the new last tab (the left neighbor when
            // the closed tab was last), else null when the pane holds none.
            //
            // Only a tab that still HAS content may be activated. A tabId whose
            // buffer is gone renders nothing. (When the slice is exercised
            // without a buffer list — pane-slice's own unit tests — every id
            // counts as alive: there is nothing to check against.)
            const known = state.buffers
            const isAlive = (id: string) => !Array.isArray(known) || known.some((b) => b.id === id)
            const alive = pane.editorTabIds.filter(isAlive)
            const rightNeighbor = pane.editorTabIds.slice(closedIndex).find(isAlive)
            pane.activeEditorTabId = rightNeighbor ?? alive[alive.length - 1] ?? null
          }
          if (pane.editorTabIds.length === 0) pane.editorOpen = false
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

          state.activePaneId = toPaneId
          state.mostRecentActivePaneIds = [
            toPaneId,
            ...state.mostRecentActivePaneIds.filter((id) => id !== toPaneId),
          ]
        })
      },

      setEditorTabPreview(paneId, tabId) {
        set((state) => {
          const pane = state.panes[paneId]
          if (!pane || !pane.editorTabIds.includes(tabId)) return
          if (!Array.isArray(state.buffers)) return
          // Preview is a single-slot concept per pane: mark `tabId`'s content
          // as the preview and clear every other tab this pane holds.
          for (const id of pane.editorTabIds) {
            const buf = state.buffers.find((b) => b.id === id)
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

      resizePaneSplit(splitId, index, sizes) {
        set((state) => {
          if (findSplit(state.rootLayout, splitId)) {
            state.rootLayout = resizeFlattenedLayout(state.rootLayout, splitId, index, sizes)
          } else if (findSplit(state.bottomLayout, splitId)) {
            state.bottomLayout = resizeFlattenedLayout(state.bottomLayout, splitId, index, sizes)
          }
        })
      },

      distributePaneSplit(splitId) {
        set((state) => {
          if (findSplit(state.rootLayout, splitId)) {
            state.rootLayout = distributeSplit(state.rootLayout, splitId)
          } else {
            state.bottomLayout = distributeSplit(state.bottomLayout, splitId)
          }
        })
      },

      togglePaneFullscreen(paneId) {
        set((state) => {
          state.fullscreenPaneId = state.fullscreenPaneId === paneId ? null : paneId
        })
      },

      exitPaneFullscreen() {
        set((state) => {
          state.fullscreenPaneId = null
        })
      },

      getAllPaneGroups() {
        return Object.values(get().panes)
      },
      getPaneById(paneId) {
        return get().panes[paneId] ?? null
      },
      getPaneByEditorTabId(tabId) {
        return Object.values(get().panes).find((p) => p.editorTabIds.includes(tabId)) ?? null
      },
      getActivePane() {
        return get().panes[get().activePaneId] ?? null
      },

      clearEditorTabPreviewEverywhere() {
        set((state) => {
          if (!Array.isArray(state.buffers)) return
          for (const buf of state.buffers) buf.isPreview = false
        })
      },

      switchToNextEditorTab(paneId) {
        const state = get()
        const pane = state.panes[paneId]
        if (!pane || pane.editorTabIds.length <= 1) return
        const curr = pane.activeEditorTabId ? pane.editorTabIds.indexOf(pane.activeEditorTabId) : -1
        get().paneActions.activateEditorTabInPane(
          pane.id,
          pane.editorTabIds[(curr + 1) % pane.editorTabIds.length],
        )
      },

      switchToPreviousEditorTab(paneId) {
        const state = get()
        const pane = state.panes[paneId]
        if (!pane || pane.editorTabIds.length <= 1) return
        const curr = pane.activeEditorTabId ? pane.editorTabIds.indexOf(pane.activeEditorTabId) : 0
        get().paneActions.activateEditorTabInPane(
          pane.id,
          pane.editorTabIds[(curr - 1 + pane.editorTabIds.length) % pane.editorTabIds.length],
        )
      },

      navigateToPane(direction) {
        const state = get()
        for (const layout of [state.rootLayout, state.bottomLayout]) {
          const adj = getAdjacentLeafId(layout, state.activePaneId, direction)
          if (adj && state.panes[adj]) {
            set((s) => {
              s.activePaneId = adj
              s.mostRecentActivePaneIds = [
                adj,
                ...s.mostRecentActivePaneIds.filter((id) => id !== adj),
              ]
            })
            return
          }
        }
      },

      setPaneChat(paneId, chatId, runnerId) {
        set((state) => {
          const pane = state.panes[paneId]
          if (!pane) return
          pane.chatId = chatId
          pane.runnerId = runnerId
        })
      },
    },
  }
}
