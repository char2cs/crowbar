import { closeRequestStore } from '@/features/panes/stores/close-request-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { viewMembers } from '@/features/panes/lib/view-state'
import type { ViewMember } from '@/features/panes/types/pane'
import { readChatWorking } from '@/features/workspace/stores/workspace-store-registry'

const isWorking = (m: ViewMember) => !!m.workspaceId && readChatWorking(m.workspaceId, m.chatId)

function ask(kind: 'pane' | 'view', id: string, members: readonly ViewMember[], close: () => void) {
  const working = members.filter(isWorking).length
  if (working === 0) {
    close()
    return
  }
  closeRequestStore.setState({ request: { kind, id, working } })
}

/** The user asked to close a pane: a chat mid-turn waits for their answer. */
export function requestClosePane(paneId: string): void {
  const state = windowPaneStore.getState()
  const pane = state.panes[paneId]
  const members = pane?.chatId
    ? [{ chatId: pane.chatId, workspaceId: pane.workspaceId ?? null }]
    : []
  ask('pane', paneId, members, () => state.paneActions.closePane(paneId))
}

/** The user asked to close a view: any chat in it mid-turn waits for their answer. */
export function requestCloseView(viewId: string): void {
  const state = windowPaneStore.getState()
  ask('view', viewId, viewMembers(state, viewId), () => state.paneActions.closeView(viewId))
}

export function confirmCloseRequest(): void {
  const request = closeRequestStore.getState().request
  if (!request) return
  closeRequestStore.setState({ request: null })
  const { paneActions } = windowPaneStore.getState()
  if (request.kind === 'pane') paneActions.closePane(request.id)
  else paneActions.closeView(request.id)
}

export function cancelCloseRequest(): void {
  closeRequestStore.setState({ request: null })
}
