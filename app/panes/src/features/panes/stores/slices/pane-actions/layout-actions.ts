import { nanoid } from 'nanoid'
import {
  findSplit,
  getAllLeafIds,
  distributeSplit,
  updateSplitSizes,
  getAdjacentLeafId,
} from '@/features/panes/utils/pane-layout'
import { insertPane } from '@/features/panes/lib/view-ops'
import { focusPane, makePane, showingLayout, touchPane } from '@/features/panes/lib/view-state'
import type { PaneActions } from '../pane-slice'
import type { PaneGet, PaneSet } from './context'

type LayoutActions = Pick<
  PaneActions,
  | 'splitPane'
  | 'setActivePane'
  | 'resizePaneSplit'
  | 'distributePaneSplit'
  | 'togglePaneFullscreen'
  | 'exitPaneFullscreen'
  | 'getAllPaneGroups'
  | 'getPaneById'
  | 'getPaneByEditorTabId'
  | 'getActivePane'
  | 'navigateToPane'
>

/** Pane-level geometry and focus: split, focus, resize, fullscreen, navigate. */
export function createLayoutActions(set: PaneSet, get: PaneGet): LayoutActions {
  return {
    splitPane(paneId, direction, bufferId?, placement = 'after') {
      let newPaneId: string | null = null
      set((state) => {
        const id = nanoid()
        const pane = makePane(id, null, {
          editorTabIds: bufferId ? [bufferId] : [],
          activeEditorTabId: bufferId ?? null,
          editorOpen: Boolean(bufferId),
          chatSelected: !bufferId,
        })
        const joined = insertPane(state, pane, {
          kind: 'split',
          targetPaneId: paneId,
          direction,
          placement,
        })
        if (joined === undefined) return
        newPaneId = id
        const onScreen =
          getAllLeafIds(showingLayout(state)).includes(id) ||
          getAllLeafIds(state.bottomLayout).includes(id)
        if (onScreen) state.activePaneId = id
        touchPane(state, id)
      })
      return newPaneId
    },

    setActivePane(paneId) {
      set((state) => {
        focusPane(state, paneId)
      })
    },

    resizePaneSplit(splitId, sizes) {
      set((state) => {
        if (findSplit(state.stage, splitId)) {
          state.stage = updateSplitSizes(state.stage, splitId, sizes)
        } else if (findSplit(state.bottomLayout, splitId)) {
          state.bottomLayout = updateSplitSizes(state.bottomLayout, splitId, sizes)
        } else {
          const view = Object.values(state.views).find((v) => findSplit(v.layout, splitId))
          if (view) view.layout = updateSplitSizes(view.layout, splitId, sizes)
        }
      })
    },

    distributePaneSplit(splitId) {
      set((state) => {
        if (findSplit(state.stage, splitId)) {
          state.stage = distributeSplit(state.stage, splitId)
        } else if (findSplit(state.bottomLayout, splitId)) {
          state.bottomLayout = distributeSplit(state.bottomLayout, splitId)
        } else {
          const view = Object.values(state.views).find((v) => findSplit(v.layout, splitId))
          if (view) view.layout = distributeSplit(view.layout, splitId)
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

    navigateToPane(direction) {
      const state = get()
      for (const layout of [showingLayout(state), state.bottomLayout]) {
        const adj = getAdjacentLeafId(layout, state.activePaneId, direction)
        if (adj && state.panes[adj]) {
          set((s) => {
            s.activePaneId = adj
            touchPane(s, adj)
          })
          return
        }
      }
    },
  }
}
