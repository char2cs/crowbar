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
        const oldIds = getAllPaneGroups(state.paneRoot).map(g => g.id)
        state.paneRoot = splitPaneUtil(state.paneRoot, paneId, direction, bufferId, placement)
        const newGroup = getAllPaneGroups(state.paneRoot).find(g => !oldIds.includes(g.id))
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
        const result = closePane(state.paneRoot, paneId)
        if (result !== null) {
          state.paneRoot = normalizePaneTree(result)
        }
        if (state.activePaneId === paneId) {
          const remaining = getAllPaneGroups(state.paneRoot)
          state.activePaneId = remaining[0]?.id ?? ROOT_PANE_ID
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
        state.paneRoot = setActivePaneBuffer(state.paneRoot, paneId, bufferId)
        state.activePaneId = paneId
      })
    },

    addBufferToPane(paneId, bufferId, setActive = true) {
      set(state => {
        state.paneRoot = addBufferToPane(state.paneRoot, paneId, bufferId, setActive)
      })
    },

    removeBufferFromPane(paneId, bufferId, preserveEmptyPane = false) {
      set(state => {
        state.paneRoot = removeBufferFromPane(state.paneRoot, paneId, bufferId)
        if (!preserveEmptyPane) {
          const pane = findPaneGroup(state.paneRoot, paneId)
          if (pane && pane.bufferIds.length === 0 && paneId !== ROOT_PANE_ID) {
            const result = closePane(state.paneRoot, paneId)
            if (result !== null) state.paneRoot = normalizePaneTree(result)
          }
        }
      })
    },

    moveBufferToPane(bufferId, fromPaneId, toPaneId) {
      set(state => {
        state.paneRoot = moveBufferBetweenPanes(state.paneRoot, bufferId, fromPaneId, toPaneId)
      })
    },

    setPanePreviewBuffer(paneId, bufferId) {
      set(state => {
        state.paneRoot = setPanePreviewBuffer(state.paneRoot, paneId, bufferId)
      })
    },

    setPaneBufferPinned(paneId, bufferId, pinned) {
      set(state => {
        state.paneRoot = setPaneBufferPinned(state.paneRoot, paneId, bufferId, pinned)
      })
    },

    reorderPaneBuffers(paneId, startIndex, endIndex) {
      set(state => {
        state.paneRoot = reorderPaneBuffers(state.paneRoot, paneId, startIndex, endIndex)
      })
    },

    resizePaneSplit(splitId, index, sizes) {
      set(state => {
        state.paneRoot = resizeFlattenedPaneSplit(state.paneRoot, splitId, index, sizes)
      })
    },

    distributePaneSplit(splitId) {
      set(state => {
        state.paneRoot = distributeFlattenedPaneSplit(state.paneRoot, splitId)
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

    getAllPaneGroups() { return getAllPaneGroups(get().paneRoot) },
    getPaneById(paneId) { return findPaneGroup(get().paneRoot, paneId) },
    getPaneByBufferId(bufferId) { return findPaneGroupByBufferId(get().paneRoot, bufferId) },
    getActivePane() { return findPaneGroup(get().paneRoot, get().activePaneId) },
  },
})
