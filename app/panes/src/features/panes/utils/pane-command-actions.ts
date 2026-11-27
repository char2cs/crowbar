import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { getOwningChatId } from '@/lib/workspace-scope'

export function openBranchReviewForWorkspace(
  wsId: string | null | undefined,
  paneId?: string,
  replaceTabId?: string,
): string | null {
  if (!wsId) {
    return null
  }

  return windowPaneStore
    .getState()
    .bufferActions.openContent(
      { type: 'branchReview', wsId, name: 'Branch Review' },
      { paneId, replaceTabId },
    )
}

export function ensurePaneChatThenOpen(
  wsId: string,
  paneId: string,
  openTab: (paneId: string, wsId: string) => void,
): void {
  const paneActions = windowPaneStore.getState().paneActions
  const openIn = (target: string) => {
    openTab(target, wsId)
    paneActions.setActivePane(target)
  }

  if (windowPaneStore.getState().panes[paneId]?.chatId) {
    openIn(paneId)
    return
  }

  const owningChatId = getOwningChatId(wsId)
  if (!owningChatId) return

  const existingPaneId = chatPaneIndex(windowPaneStore.getState().panes).get(owningChatId)
  if (existingPaneId) {
    openIn(existingPaneId)
    return
  }

  paneActions.dropChatOnPane(owningChatId, paneId, 'center', wsId)
  openIn(paneId)
}

export function openNewTabPage(wsId: string | null | undefined, paneId: string): void {
  if (!wsId) return
  ensurePaneChatThenOpen(wsId, paneId, (target) => {
    windowPaneStore
      .getState()
      .bufferActions.openContent({ type: 'newTabPage', workspaceId: wsId }, { paneId: target })
  })
}

export function togglePaneEditor(paneId: string): void {
  windowPaneStore.setState((state) => {
    const p = state.panes[paneId]
    if (!p) return {}
    return { panes: { ...state.panes, [paneId]: { ...p, editorOpen: !p.editorOpen } } }
  })
}
