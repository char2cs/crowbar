import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { getOwningChatId } from '@/lib/workspace-scope'

// Opens Branch Review as a tab; null without a workspace. The caller names workspace and pane
// (C8): a pane's own button reviews that pane's workspace, in that pane, whatever has focus.
// `replaceTabId` is a New Tab page the review replaces.
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

// A pane must hold a chat before anything opens in its editor view (spec §7.2). A chatless pane
// gets the workspace's owning chat (revealed where already shown), never a new one; with no
// owning chat resolved yet this does nothing. `openTab` gets the target pane and workspace.
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

  // A chat already showing somewhere is revealed, never duplicated.
  const existingPaneId = chatPaneIndex(windowPaneStore.getState().panes).get(owningChatId)
  if (existingPaneId) {
    openIn(existingPaneId)
    return
  }

  paneActions.dropChatOnPane(owningChatId, paneId, 'center', wsId)
  openIn(paneId)
}

/** A browser-style New Tab in `paneId`'s editor area: the `+`, ⌘T, and ⌘N over
 *  the editor. Nothing without the pane's workspace (see above). */
export function openNewTabPage(wsId: string | null | undefined, paneId: string): void {
  if (!wsId) return
  ensurePaneChatThenOpen(wsId, paneId, (target) => {
    windowPaneStore
      .getState()
      .bufferActions.openContent({ type: 'newTabPage', workspaceId: wsId }, { paneId: target })
  })
}

/** Show or hide the editor area beside a pane's chat (`PaneGroup.editorOpen`). */
export function togglePaneEditor(paneId: string): void {
  windowPaneStore.setState((state) => {
    const p = state.panes[paneId]
    if (!p) return {}
    return { panes: { ...state.panes, [paneId]: { ...p, editorOpen: !p.editorOpen } } }
  })
}
