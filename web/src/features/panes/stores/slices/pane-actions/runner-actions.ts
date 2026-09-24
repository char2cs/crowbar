import { nanoid } from 'nanoid'
import { insertPane, removePane } from '@/features/panes/lib/view-ops'
import { hasUnsavedEdits, type PaneContent } from '@/features/panes/types/pane-content'
import { getAllLeafIds, getFirstLeafId } from '@/features/panes/utils/pane-layout'
import { makePane, type ViewState } from '@/features/panes/lib/view-state'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import type { PaneActions } from '../pane-slice'
import type { PaneSet } from './context'

type RunnerActions = Pick<
  PaneActions,
  'setPaneRunner' | 'retargetPane' | 'forgetChat' | 'placeRestoredMembers' | 'adoptBackgroundChat'
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

    placeRestoredMembers(placed, gone) {
      set((state) => {
        for (const pane of Object.values(state.panes)) {
          const workspaceId = pane.chatId && !pane.workspaceId && placed.get(pane.chatId)
          if (workspaceId) pane.workspaceId = workspaceId
        }
        for (const chatId of gone) {
          const paneId = chatPaneIndex(state.panes).get(chatId)
          if (!paneId || state.panes[paneId].workspaceId) continue
          keepUnsavedTabs(state, paneId)
          removePane(state, paneId)
        }
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

/**
 * A pane about to leave as its view's last member takes its tabs with it; the
 * unsaved ones (the only copy of those edits) move to the stage instead.
 */
function keepUnsavedTabs(state: ViewState & { buffers: PaneContent[] }, paneId: string): void {
  const pane = state.panes[paneId]
  const view = pane.viewId ? state.views[pane.viewId] : undefined
  if (!view || getAllLeafIds(view.layout).length > 1) return
  const unsaved = new Set(state.buffers.filter(hasUnsavedEdits).map((b) => b.id))
  const kept = pane.editorTabIds.filter((id) => unsaved.has(id))
  if (kept.length === 0) return
  const stage = state.panes[getFirstLeafId(state.stage)]
  stage.editorTabIds = [
    ...stage.editorTabIds,
    ...kept.filter((id) => !stage.editorTabIds.includes(id)),
  ]
  stage.activeEditorTabId ??= kept[0]
  stage.editorOpen = true
}
