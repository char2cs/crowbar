// web/src/features/workspace/stores/slices/pane-slice.ts
import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import type { PaneGroup, PaneNode, SplitDirection, SplitPlacement } from '@/features/panes/types/pane'
import {
  addBufferToPane,
  closePane,
  distributeFlattenedPaneSplit,
  findPaneGroup,
  findPaneGroupByBufferId,
  findPaneNode,
  getAllPaneGroups,
  moveBufferBetweenPanes,
  normalizePaneTree,
  removeBufferFromPane,
  reorderPaneBuffers,
  resizeFlattenedPaneSplit,
  setActivePaneBuffer,
  setPaneBufferPinned,
  setPanePreviewBuffer,
  splitPane as splitPaneUtil,
} from '@/features/panes/utils/pane-tree'

export interface PaneActions {
  splitPane(paneId: string, direction: SplitDirection, bufferId?: string, placement?: SplitPlacement): string | null
  closePane(paneId: string): void
  setActivePane(paneId: string): void
  activatePaneBuffer(paneId: string, bufferId: string | null): void
  addBufferToPane(paneId: string, bufferId: string, setActive?: boolean): void
  removeBufferFromPane(paneId: string, bufferId: string, preserveEmptyPane?: boolean): void
  moveBufferToPane(bufferId: string, fromPaneId: string, toPaneId: string): void
  setPanePreviewBuffer(paneId: string, bufferId: string | null): void
  setPaneBufferPinned(paneId: string, bufferId: string, pinned: boolean): void
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
}

export interface PaneSlice {
  paneRoot: PaneNode
  bottomRoot: PaneNode
  activePaneId: string
  mostRecentActivePaneIds: string[]
  fullscreenPaneId: string | null
  paneActions: PaneActions
}

function createInitialRoot(): PaneGroup {
  return { id: ROOT_PANE_ID, type: 'group', bufferIds: [], activeBufferId: null }
}

function createInitialBottomRoot(): PaneGroup {
  return { id: BOTTOM_PANE_ID, type: 'group', bufferIds: [], activeBufferId: null }
}

// Returns which tree owns `paneId`. Searches paneRoot first, falls back to bottomRoot.
// If not found in either, uses BOTTOM_PANE_ID hint to decide, defaulting to paneRoot.
function getTreeForPane(
  state: Pick<PaneSlice, 'paneRoot' | 'bottomRoot'>,
  paneId: string,
): 'paneRoot' | 'bottomRoot' {
  if (findPaneGroup(state.paneRoot, paneId)) return 'paneRoot'
  if (findPaneGroup(state.bottomRoot, paneId)) return 'bottomRoot'
  return paneId === BOTTOM_PANE_ID ? 'bottomRoot' : 'paneRoot'
}

export const createPaneSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  PaneSlice
> = (set, get) => ({
  paneRoot: createInitialRoot(),
  bottomRoot: createInitialBottomRoot(),
  activePaneId: ROOT_PANE_ID,
  mostRecentActivePaneIds: [ROOT_PANE_ID],
  fullscreenPaneId: null,

  paneActions: {
    splitPane(paneId, direction, bufferId?, placement = 'after') {
      let newPaneId: string | null = null
      set(state => {
        const target = getTreeForPane(state, paneId)
        const currentTree = target === 'paneRoot' ? state.paneRoot : state.bottomRoot
        const oldIds = getAllPaneGroups(currentTree).map(g => g.id)
        const newTree = splitPaneUtil(currentTree, paneId, direction, bufferId, placement)
        if (target === 'paneRoot') {
          state.paneRoot = newTree
        } else {
          state.bottomRoot = newTree
        }
        const newGroup = getAllPaneGroups(newTree).find(g => !oldIds.includes(g.id))
        newPaneId = newGroup?.id ?? null
        if (newPaneId) {
          state.activePaneId = newPaneId
          state.mostRecentActivePaneIds = [newPaneId, ...state.mostRecentActivePaneIds]
        }
      })
      return newPaneId
    },

    closePane(paneId) {
      set(state => {
        const target = getTreeForPane(state, paneId)
        const currentTree = target === 'paneRoot' ? state.paneRoot : state.bottomRoot
        const result = closePane(currentTree, paneId)
        if (result !== null) {
          const normalized = normalizePaneTree(result)
          if (target === 'paneRoot') {
            state.paneRoot = normalized
          } else {
            state.bottomRoot = normalized
          }
          if (state.activePaneId === paneId) {
            const remaining = getAllPaneGroups(normalized)
            const fallbackId = target === 'paneRoot' ? ROOT_PANE_ID : BOTTOM_PANE_ID
            state.activePaneId = remaining[0]?.id ?? fallbackId
          }
        }
        state.mostRecentActivePaneIds = state.mostRecentActivePaneIds.filter(id => id !== paneId)
        if (state.fullscreenPaneId === paneId) state.fullscreenPaneId = null
      })
    },

    setActivePane(paneId) {
      set(state => {
        state.activePaneId = paneId
        state.mostRecentActivePaneIds = [paneId, ...state.mostRecentActivePaneIds.filter(id => id !== paneId)]
      })
    },

    activatePaneBuffer(paneId, bufferId) {
      set(state => {
        const target = getTreeForPane(state, paneId)
        if (target === 'paneRoot') {
          state.paneRoot = setActivePaneBuffer(state.paneRoot, paneId, bufferId)
        } else {
          state.bottomRoot = setActivePaneBuffer(state.bottomRoot, paneId, bufferId)
        }
        state.activePaneId = paneId
      })
    },

    addBufferToPane(paneId, bufferId, setActive = true) {
      set(state => {
        const target = getTreeForPane(state, paneId)
        if (target === 'paneRoot') {
          state.paneRoot = addBufferToPane(state.paneRoot, paneId, bufferId, setActive)
        } else {
          state.bottomRoot = addBufferToPane(state.bottomRoot, paneId, bufferId, setActive)
        }
      })
    },

    removeBufferFromPane(paneId, bufferId, preserveEmptyPane = false) {
      set(state => {
        const target = getTreeForPane(state, paneId)
        if (target === 'paneRoot') {
          state.paneRoot = removeBufferFromPane(state.paneRoot, paneId, bufferId)
          if (!preserveEmptyPane) {
            const pane = findPaneGroup(state.paneRoot, paneId)
            if (pane && pane.bufferIds.length === 0 && paneId !== ROOT_PANE_ID) {
              const result = closePane(state.paneRoot, paneId)
              if (result !== null) state.paneRoot = normalizePaneTree(result)
            }
          }
        } else {
          state.bottomRoot = removeBufferFromPane(state.bottomRoot, paneId, bufferId)
          if (!preserveEmptyPane) {
            const pane = findPaneGroup(state.bottomRoot, paneId)
            if (pane && pane.bufferIds.length === 0 && paneId !== BOTTOM_PANE_ID) {
              const result = closePane(state.bottomRoot, paneId)
              if (result !== null) state.bottomRoot = normalizePaneTree(result)
            }
          }
        }
      })
    },

    moveBufferToPane(bufferId, fromPaneId, toPaneId) {
      set(state => {
        const fromTarget = getTreeForPane(state, fromPaneId)
        const toTarget = getTreeForPane(state, toPaneId)

        if (fromTarget === toTarget) {
          if (fromTarget === 'paneRoot') {
            state.paneRoot = moveBufferBetweenPanes(state.paneRoot, bufferId, fromPaneId, toPaneId)
          } else {
            state.bottomRoot = moveBufferBetweenPanes(state.bottomRoot, bufferId, fromPaneId, toPaneId)
          }
        } else {
          // Cross-tree: remove from source, add to destination
          if (fromTarget === 'paneRoot') {
            state.paneRoot = removeBufferFromPane(state.paneRoot, fromPaneId, bufferId)
            state.bottomRoot = addBufferToPane(state.bottomRoot, toPaneId, bufferId, true)
          } else {
            state.bottomRoot = removeBufferFromPane(state.bottomRoot, fromPaneId, bufferId)
            state.paneRoot = addBufferToPane(state.paneRoot, toPaneId, bufferId, true)
          }
        }
      })
    },

    setPanePreviewBuffer(paneId, bufferId) {
      set(state => {
        const target = getTreeForPane(state, paneId)
        if (target === 'paneRoot') {
          state.paneRoot = setPanePreviewBuffer(state.paneRoot, paneId, bufferId)
        } else {
          state.bottomRoot = setPanePreviewBuffer(state.bottomRoot, paneId, bufferId)
        }
      })
    },

    setPaneBufferPinned(paneId, bufferId, pinned) {
      set(state => {
        const target = getTreeForPane(state, paneId)
        if (target === 'paneRoot') {
          state.paneRoot = setPaneBufferPinned(state.paneRoot, paneId, bufferId, pinned)
        } else {
          state.bottomRoot = setPaneBufferPinned(state.bottomRoot, paneId, bufferId, pinned)
        }
      })
    },

    reorderPaneBuffers(paneId, startIndex, endIndex) {
      set(state => {
        const target = getTreeForPane(state, paneId)
        if (target === 'paneRoot') {
          state.paneRoot = reorderPaneBuffers(state.paneRoot, paneId, startIndex, endIndex)
        } else {
          state.bottomRoot = reorderPaneBuffers(state.bottomRoot, paneId, startIndex, endIndex)
        }
      })
    },

    resizePaneSplit(splitId, index, sizes) {
      set(state => {
        if (findPaneNode(state.paneRoot, splitId)) {
          state.paneRoot = resizeFlattenedPaneSplit(state.paneRoot, splitId, index, sizes)
        } else {
          state.bottomRoot = resizeFlattenedPaneSplit(state.bottomRoot, splitId, index, sizes)
        }
      })
    },

    distributePaneSplit(splitId) {
      set(state => {
        if (findPaneNode(state.paneRoot, splitId)) {
          state.paneRoot = distributeFlattenedPaneSplit(state.paneRoot, splitId)
        } else {
          state.bottomRoot = distributeFlattenedPaneSplit(state.bottomRoot, splitId)
        }
      })
    },

    togglePaneFullscreen(paneId) {
      set(state => {
        state.fullscreenPaneId = state.fullscreenPaneId === paneId ? null : paneId
      })
    },

    exitPaneFullscreen() {
      set(state => { state.fullscreenPaneId = null })
    },

    getAllPaneGroups() {
      return [
        ...getAllPaneGroups(get().paneRoot),
        ...getAllPaneGroups(get().bottomRoot),
      ]
    },

    getPaneById(paneId) {
      return (
        findPaneGroup(get().paneRoot, paneId) ??
        findPaneGroup(get().bottomRoot, paneId) ??
        null
      )
    },

    getPaneByBufferId(bufferId) {
      return (
        findPaneGroupByBufferId(get().paneRoot, bufferId) ??
        findPaneGroupByBufferId(get().bottomRoot, bufferId) ??
        null
      )
    },

    getActivePane() {
      const id = get().activePaneId
      return (
        findPaneGroup(get().paneRoot, id) ??
        findPaneGroup(get().bottomRoot, id) ??
        null
      )
    },

    clearPreviewBufferEverywhere(bufferId) {
      set(state => {
        const rootPaneIds = getAllPaneGroups(state.paneRoot)
          .filter(p => p.previewBufferId === bufferId)
          .map(p => p.id)
        for (const id of rootPaneIds) {
          state.paneRoot = setPanePreviewBuffer(state.paneRoot, id, null)
        }
        const bottomPaneIds = getAllPaneGroups(state.bottomRoot)
          .filter(p => p.previewBufferId === bufferId)
          .map(p => p.id)
        for (const id of bottomPaneIds) {
          state.bottomRoot = setPanePreviewBuffer(state.bottomRoot, id, null)
        }
      })
    },

    switchToNextBufferInPane() {
      const state = get()
      const activePane =
        findPaneGroup(state.paneRoot, state.activePaneId) ??
        findPaneGroup(state.bottomRoot, state.activePaneId)
      if (!activePane || activePane.bufferIds.length <= 1) return

      const currentIndex = activePane.activeBufferId
        ? activePane.bufferIds.indexOf(activePane.activeBufferId)
        : -1
      const nextIndex = (currentIndex + 1) % activePane.bufferIds.length
      const nextBufferId = activePane.bufferIds[nextIndex]

      get().paneActions.activatePaneBuffer(activePane.id, nextBufferId)
    },

    switchToPreviousBufferInPane() {
      const state = get()
      const activePane =
        findPaneGroup(state.paneRoot, state.activePaneId) ??
        findPaneGroup(state.bottomRoot, state.activePaneId)
      if (!activePane || activePane.bufferIds.length <= 1) return

      const currentIndex = activePane.activeBufferId
        ? activePane.bufferIds.indexOf(activePane.activeBufferId)
        : 0
      const prevIndex =
        (currentIndex - 1 + activePane.bufferIds.length) % activePane.bufferIds.length
      const prevBufferId = activePane.bufferIds[prevIndex]

      get().paneActions.activatePaneBuffer(activePane.id, prevBufferId)
    },
  },
})
