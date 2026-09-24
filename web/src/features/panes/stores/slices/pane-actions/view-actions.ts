import { nanoid } from 'nanoid'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { releaseClosedChat } from '@/features/panes/lib/release-closed-chat'
import {
  fillPane,
  insertPane,
  movePane,
  removePane,
  removeView,
} from '@/features/panes/lib/view-ops'
import {
  focusPane,
  homeOf,
  makePane,
  mayShow,
  nextViewFor,
  projectOf,
  showView,
  touchPane,
  viewChatIds,
  viewMembers,
} from '@/features/panes/lib/view-state'
import type { ViewMember } from '@/features/panes/types/pane'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import type { PaneActions } from '../pane-slice'
import type { PaneGet, PaneSet } from './context'

type ViewActions = Pick<
  PaneActions,
  | 'openChat'
  | 'detachPane'
  | 'closePane'
  | 'closeView'
  | 'reorderView'
  | 'activateView'
  | 'setActiveProject'
  | 'closeViewsForProject'
>

/** The row lifecycle: open, detach, close, reorder, activate, project switch. */
export function createViewActions(set: PaneSet, get: PaneGet): ViewActions {
  const release = (members: readonly ViewMember[]) => {
    for (const { chatId, workspaceId } of members) {
      void releaseClosedChat(chatId, workspaceId, () => get().panes)
    }
  }

  return {
    openChat(chatId, opts = {}) {
      set((state) => {
        const existing = chatPaneIndex(state.panes).get(chatId)
        if (existing) {
          focusPane(state, existing)
          return
        }
        const projectId = opts.projectId || state.activeProjectId || ''
        const foreign =
          !!projectId && !!state.activeProjectId && projectId !== state.activeProjectId
        const runnerId = opts.runnerId ?? null
        const workspaceId = opts.workspaceId ?? null

        if (!foreign && state.activeViewId === null) {
          const leaves = getAllLeafIds(state.stage)
          const target = leaves.includes(state.activePaneId) ? state.activePaneId : leaves[0]
          const viewId = fillPane(state, target, chatId, runnerId, projectId, workspaceId)
          if (viewId) {
            showView(state, viewId, target)
            return
          }
        }

        const paneId = nanoid()
        const viewId = insertPane(
          state,
          makePane(paneId, null, { chatId, runnerId, workspaceId }),
          {
            kind: 'view',
            projectId,
          },
        )
        if (!viewId) return
        if (foreign) {
          state.activeViewByProject[projectId] = viewId
          touchPane(state, paneId)
          return
        }
        showView(state, viewId, paneId)
      })
    },

    detachPane(paneId) {
      set((state) => {
        const home = homeOf(state, paneId)
        if (!state.panes[paneId]?.chatId || home?.kind !== 'view') return
        if (viewChatIds(state, home.viewId).length < 2) return
        const source = state.views[home.viewId]
        const wasShowing = state.activeViewId === source.id
        const moved = movePane(state, paneId, {
          kind: 'view',
          projectId: source.projectId,
          after: source.id,
        })
        const viewId = state.panes[paneId]?.viewId
        if (moved && wasShowing && viewId) showView(state, viewId, paneId)
      })
    },

    closePane(paneId) {
      const pane = get().panes[paneId]
      set((state) => {
        removePane(state, paneId)
      })
      if (pane?.chatId) release([{ chatId: pane.chatId, workspaceId: pane.workspaceId ?? null }])
    },

    closeView(viewId) {
      const members = viewMembers(get(), viewId)
      set((state) => {
        removeView(state, viewId)
      })
      release(members)
    },

    reorderView(viewId, targetId, mode) {
      set((state) => {
        if (viewId === targetId || !state.views[viewId] || !state.views[targetId]) return
        const order = state.viewOrder.filter((id) => id !== viewId)
        order.splice(order.indexOf(targetId) + (mode === 'after' ? 1 : 0), 0, viewId)
        state.viewOrder = order
      })
    },

    activateView(viewId) {
      set((state) => {
        if (!state.views[viewId] || viewId === state.activeViewId) return
        if (!mayShow(state, viewId)) {
          state.activeViewByProject[projectOf(state, viewId)] = viewId
          return
        }
        showView(state, viewId)
      })
    },

    setActiveProject(projectId) {
      set((state) => {
        if (state.activeProjectId === projectId) return
        const first = state.activeProjectId === null
        state.activeProjectId = projectId
        if (first) {
          // Records minted before the window knew its project join it.
          for (const view of Object.values(state.views)) {
            if (!view.projectId) view.projectId = projectId
          }
          // A view already on screen wins over a persisted pointer: this
          // first call can land after an open already showed one.
          if (state.activeViewId && projectOf(state, state.activeViewId) === projectId) {
            state.activeViewByProject[projectId] = state.activeViewId
            return
          }
        }
        const remembered = state.activeViewByProject[projectId]
        if (remembered && state.views[remembered]?.projectId === projectId) {
          showView(state, remembered)
          return
        }
        if (state.activeViewId && projectOf(state, state.activeViewId) === projectId) return
        showView(state, nextViewFor(state, projectId))
      })
    },

    closeViewsForProject(projectId) {
      const doomed = get().viewOrder.filter((id) => get().views[id]?.projectId === projectId)
      for (const viewId of doomed) get().paneActions.closeView(viewId)
      set((state) => {
        delete state.activeViewByProject[projectId]
      })
    },
  }
}
