import { nanoid } from 'nanoid'
import type { SplitDirection, SplitPlacement } from '@/features/panes/types/pane'
import { fillPane, insertPane, movePane } from '@/features/panes/lib/view-ops'
import { focusPane, homeOf, makePane, projectOf } from '@/features/panes/lib/view-state'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import type { PaneActions } from '../pane-slice'
import type { PaneSet } from './context'

/** A chat dropped onto a pane: the only gesture that puts two chats in one view. */
export function createChatDropActions(set: PaneSet): Pick<PaneActions, 'dropChatOnPane'> {
  return {
    dropChatOnPane(chatId, paneId, zone) {
      set((state) => {
        const target = state.panes[paneId]
        if (!target) return
        const existing = chatPaneIndex(state.panes).get(chatId)

        // The middle of a chatless pane fills it in place; an already-open
        // chat is revealed rather than opened twice.
        if (zone === 'center' && target.chatId === null) {
          if (existing) {
            focusPane(state, existing)
            return
          }
          const home = homeOf(state, paneId)
          const projectId =
            home?.kind === 'view' ? projectOf(state, home.viewId) : (state.activeProjectId ?? '')
          if (fillPane(state, paneId, chatId, null, projectId) === undefined) return
          focusPane(state, paneId)
          return
        }

        // Anything else splits the target — the middle of an occupied pane
        // only ever ADDS, on the right.
        const side = zone === 'center' ? 'right' : zone
        const direction: SplitDirection =
          side === 'left' || side === 'right' ? 'horizontal' : 'vertical'
        const placement: SplitPlacement = side === 'left' || side === 'top' ? 'before' : 'after'

        if (existing) {
          if (existing !== paneId) {
            movePane(state, existing, { kind: 'split', targetPaneId: paneId, direction, placement })
          }
          focusPane(state, existing)
          return
        }
        const id = nanoid()
        const joined = insertPane(state, makePane(id, null, { chatId }), {
          kind: 'split',
          targetPaneId: paneId,
          direction,
          placement,
        })
        if (joined !== undefined) focusPane(state, id)
      })
    },
  }
}
