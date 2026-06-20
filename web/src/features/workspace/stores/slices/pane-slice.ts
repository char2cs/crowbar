import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type { WorkspaceStore } from '../workspace-store'
import { isEditorContent } from '@/features/panes/types/pane-content'
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
  activatePaneBuffer(paneId: string, bufferId: string | null): void
  addBufferToPane(paneId: string, bufferId: string, setActive?: boolean): void
  removeBufferFromPane(paneId: string, bufferId: string, preserveEmptyPane?: boolean): void
  moveBufferToPane(bufferId: string, fromPaneId: string, toPaneId: string): void
  setPanePreviewBuffer(paneId: string, bufferId: string | null): void
  setPaneBufferPinned(paneId: string, bufferId: string, pinned: boolean): void
  setPaneLocked(paneId: string, locked: boolean): void
  reorderPaneBuffers(paneId: string, startIndex: number, endIndex: number): void
  resizePaneSplit(splitId: string, index: number, sizes: [number, number]): void
  distributePaneSplit(splitId: string): void
  togglePaneFullscreen(paneId: string): void
  exitPaneFullscreen(): void
  getAllPaneGroups(): PaneGroup[]
  getPaneById(paneId: string): PaneGroup | null
  getPaneByBufferId(bufferId: string): PaneGroup | null
  getActivePane(): PaneGroup | null
  clearPreviewBufferEverywhere(bufferId: string): void
  switchToNextBufferInPane(): void
  switchToPreviousBufferInPane(): void
  navigateToPane(direction: 'left' | 'right' | 'up' | 'down'): void
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
  return { id: ROOT_PANE_ID, type: 'group', bufferIds: [], activeBufferId: null }
}

function makeBottomLeaf(): PaneGroup {
  return { id: BOTTOM_PANE_ID, type: 'group', bufferIds: [], activeBufferId: null }
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

  /** Release the held Monaco model for `bufferId` in `paneId` (editor buffers
   *  only). A no-op when the buffer isn't an editor or the pane didn't hold it
   *  (the manager guards on `held`). Disposes the model when no pane still holds
   *  it, so closing a tab frees its model and a reopen reads fresh content. */
  const releaseBufferModel = (paneId: string, bufferId: string) => {
    const buf = get().buffers?.find((b) => b.id === bufferId)
    if (!buf || !isEditorContent(buf)) return
    editorManagerOf()?.closeBuffer(paneId, fileUri(buf.path))
  }

  return ({
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
          bufferIds: bufferId ? [bufferId] : [],
          activeBufferId: bufferId ?? null,
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
            for (const bufferId of closingPane.bufferIds) {
              const fp = state.panes[fallbackId]
              if (fp && !fp.bufferIds.includes(bufferId)) fp.bufferIds.push(bufferId)
            }
            if (state.activePaneId === paneId && closingPane.activeBufferId) {
              const fp = state.panes[fallbackId]
              if (fp) fp.activeBufferId = closingPane.activeBufferId
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
        state.mostRecentActivePaneIds = state.mostRecentActivePaneIds.filter((id) => id !== paneId)
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

    activatePaneBuffer(paneId, bufferId) {
      set((state) => {
        const pane = state.panes[paneId]
        if (!pane) return
        pane.activeBufferId = bufferId
        state.activePaneId = paneId
        state.mostRecentActivePaneIds = [
          paneId,
          ...state.mostRecentActivePaneIds.filter((id) => id !== paneId),
        ]
      })
    },

    addBufferToPane(paneId, bufferId, setActive = true) {
      set((state) => {
        const pane = state.panes[paneId]
        if (!pane) return
        if (!pane.bufferIds.includes(bufferId)) pane.bufferIds.push(bufferId)
        if (setActive) pane.activeBufferId = bufferId
      })
    },

    removeBufferFromPane(paneId, bufferId, preserveEmptyPane = false) {
      // Release this pane's held Monaco model BEFORE mutating state (reads the
      // buffer path, which still exists). Guarded internally: a no-op if the pane
      // never held it.
      if (get().panes[paneId]?.bufferIds.includes(bufferId)) {
        releaseBufferModel(paneId, bufferId)
      }
      set((state) => {
        const pane = state.panes[paneId]
        if (!pane) return
        const closedIndex = pane.bufferIds.indexOf(bufferId)
        const wasActive = pane.activeBufferId === bufferId
        pane.bufferIds = pane.bufferIds.filter((id) => id !== bufferId)
        if (wasActive) {
          // Activate the ADJACENT tab so a close keeps you on a nearby tab
          // (VS Code-style): the buffer that shifted into the closed slot (the
          // right neighbor), else the new last tab (the left neighbor when the
          // closed tab was last), else null when the pane is now empty.
          pane.activeBufferId =
            pane.bufferIds[closedIndex] ?? pane.bufferIds[pane.bufferIds.length - 1] ?? null
        }
        if (pane.previewBufferId === bufferId) pane.previewBufferId = null
        if (
          !preserveEmptyPane &&
          pane.bufferIds.length === 0 &&
          paneId !== ROOT_PANE_ID &&
          paneId !== BOTTOM_PANE_ID
        ) {
          const key = getLayoutKey(state, paneId)
          const result = closeLayout(state[key], paneId)
          if (result !== null) {
            state[key] = normalizeLayout(result)
            const remaining = getAllLeafIds(state[key])
            if (state.activePaneId === paneId) state.activePaneId = remaining[0] ?? ROOT_PANE_ID
            delete state.panes[paneId]
            state.mostRecentActivePaneIds = state.mostRecentActivePaneIds.filter(
              (id) => id !== paneId,
            )
          }
        }
      })
    },

    moveBufferToPane(bufferId, fromPaneId, toPaneId) {
      set((state) => {
        const fromPane = state.panes[fromPaneId]
        const toPane = state.panes[toPaneId]
        if (!fromPane || !toPane) return
        fromPane.bufferIds = fromPane.bufferIds.filter((id) => id !== bufferId)
        if (fromPane.activeBufferId === bufferId)
          fromPane.activeBufferId = fromPane.bufferIds[0] ?? null
        if (!toPane.bufferIds.includes(bufferId)) toPane.bufferIds.push(bufferId)
        toPane.activeBufferId = bufferId
        state.activePaneId = toPaneId
        state.mostRecentActivePaneIds = [
          toPaneId,
          ...state.mostRecentActivePaneIds.filter((id) => id !== toPaneId),
        ]
        if (
          fromPane.bufferIds.length === 0 &&
          fromPaneId !== ROOT_PANE_ID &&
          fromPaneId !== BOTTOM_PANE_ID
        ) {
          const key = getLayoutKey(state, fromPaneId)
          const result = closeLayout(state[key], fromPaneId)
          if (result !== null) {
            state[key] = normalizeLayout(result)
            delete state.panes[fromPaneId]
            state.mostRecentActivePaneIds = state.mostRecentActivePaneIds.filter(
              (id) => id !== fromPaneId,
            )
          }
        }
      })
    },

    setPanePreviewBuffer(paneId, bufferId) {
      set((state) => {
        const p = state.panes[paneId]
        if (p) p.previewBufferId = bufferId
      })
    },

    setPaneBufferPinned(paneId, bufferId, pinned) {
      set((state) => {
        const pane = state.panes[paneId]
        if (!pane) return
        if (!pane.pinnedBufferIds) pane.pinnedBufferIds = []
        if (pinned) {
          if (!pane.pinnedBufferIds.includes(bufferId)) pane.pinnedBufferIds.push(bufferId)
        } else pane.pinnedBufferIds = pane.pinnedBufferIds.filter((id) => id !== bufferId)
      })
    },

    setPaneLocked(paneId, locked) {
      set((state) => {
        const pane = state.panes[paneId]
        if (pane) pane.locked = locked
      })
    },

    reorderPaneBuffers(paneId, startIndex, endIndex) {
      set((state) => {
        const pane = state.panes[paneId]
        if (!pane) return
        const ids = [...pane.bufferIds]
        const [moved] = ids.splice(startIndex, 1)
        ids.splice(endIndex, 0, moved)
        pane.bufferIds = ids
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
    getPaneByBufferId(bufferId) {
      return Object.values(get().panes).find((p) => p.bufferIds.includes(bufferId)) ?? null
    },
    getActivePane() {
      return get().panes[get().activePaneId] ?? null
    },

    clearPreviewBufferEverywhere(bufferId) {
      set((state) => {
        for (const pane of Object.values(state.panes)) {
          if (pane.previewBufferId === bufferId) pane.previewBufferId = null
        }
      })
    },

    switchToNextBufferInPane() {
      const state = get()
      const pane = state.panes[state.activePaneId]
      if (!pane || pane.bufferIds.length <= 1) return
      const curr = pane.activeBufferId ? pane.bufferIds.indexOf(pane.activeBufferId) : -1
      get().paneActions.activatePaneBuffer(
        pane.id,
        pane.bufferIds[(curr + 1) % pane.bufferIds.length],
      )
    },

    switchToPreviousBufferInPane() {
      const state = get()
      const pane = state.panes[state.activePaneId]
      if (!pane || pane.bufferIds.length <= 1) return
      const curr = pane.activeBufferId ? pane.bufferIds.indexOf(pane.activeBufferId) : 0
      get().paneActions.activatePaneBuffer(
        pane.id,
        pane.bufferIds[(curr - 1 + pane.bufferIds.length) % pane.bufferIds.length],
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
  },
})
}
