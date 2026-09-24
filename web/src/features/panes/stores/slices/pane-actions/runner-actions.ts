import { nanoid } from 'nanoid'
import { insertPane, removePane } from '@/features/panes/lib/view-ops'
import { makePane } from '@/features/panes/lib/view-state'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import type { PaneActions } from '../pane-slice'
import type { PaneSet } from './context'

type RunnerActions = Pick<
  PaneActions,
  'setPaneRunner' | 'retargetPane' | 'forgetChat' | 'adoptBackgroundChat'
>

/** What the daemon's chat and runner frames do to panes. */
export function createRunnerActions(set: PaneSet): RunnerActions {
  return {
    setPaneRunner(paneId, runnerId) {
      set((state) => {
        const pane = state.panes[paneId]
        if (pane && pane.runnerId !== runnerId) pane.runnerId = runnerId
      })
    },

    retargetPane(paneId, chatId, runnerId) {
      set((state) => {
        if (!state.panes[paneId]?.chatId) return
        // Law 4: any other pane on the entered chat goes.
        for (const other of Object.values(state.panes)) {
          if (other.id !== paneId && other.chatId === chatId) removePane(state, other.id)
        }
        const taker = state.panes[paneId]
        if (!taker) return
        // The runner moved within its workspace; the pane's workspace was
        // fixed when its chat was opened (C3).
        taker.chatId = chatId
        taker.runnerId = runnerId
      })
    },

    forgetChat(chatId) {
      set((state) => {
        const paneId = chatPaneIndex(state.panes).get(chatId)
        if (paneId) removePane(state, paneId)
      })
    },

    adoptBackgroundChat(chatId, projectId, workspaceId = null) {
      set((state) => {
        if (chatPaneIndex(state.panes).has(chatId)) return
        insertPane(state, makePane(nanoid(), null, { chatId, workspaceId }), {
          kind: 'view',
          projectId,
        })
      })
    },
  }
}
