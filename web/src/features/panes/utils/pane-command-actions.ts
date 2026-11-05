import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { showingLayout } from '@/features/panes/lib/view-state'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { getOwningChatId } from '@/lib/workspace-scope'
import { BOTTOM_PANE_ID } from '../constants/pane'
import type { LayoutNode } from '../types/pane'
import { getAllLeafIds } from './pane-layout'
import { getPaneScopeForPaneId } from './pane-routing'
import { createPaneBeside } from './pane-split-actions'

export const getShareableSplitBufferId = (bufferId: string | null | undefined) => {
  if (!bufferId) return undefined
  const activeBuffer = windowPaneStore.getState().buffers.find((buffer) => buffer.id === bufferId)
  if (activeBuffer?.type === 'terminal') {
    return undefined
  }

  return bufferId
}

function isEditorPaneId(paneId: string): boolean {
  if (paneId === BOTTOM_PANE_ID) {
    return false
  }

  return getAllLeafIds(showingLayout(windowPaneStore.getState())).includes(paneId)
}

function getActiveEditorPane() {
  const state = windowPaneStore.getState()
  const activePane = state.paneActions.getActivePane()
  if (!activePane || !isEditorPaneId(activePane.id)) {
    return null
  }

  return activePane
}

export function toggleActiveEditorGroupLock(): boolean {
  const state = windowPaneStore.getState()
  const activePane = getActiveEditorPane()
  if (!activePane) {
    return false
  }

  state.paneActions.setPaneLocked(activePane.id, !activePane.locked)
  return true
}

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

export function splitActiveEditorGroup(direction: 'horizontal' | 'vertical'): boolean {
  const activePane = getActiveEditorPane()
  if (!activePane) {
    return false
  }

  return splitEditorGroup(activePane.id, direction, activePane.activeEditorTabId)
}

export function splitEditorGroup(
  paneId: string,
  direction: 'horizontal' | 'vertical',
  bufferId?: string | null,
): boolean {
  if (!isEditorPaneId(paneId)) {
    return false
  }

  return Boolean(createPaneBeside(paneId, direction, 'after', getShareableSplitBufferId(bufferId)))
}

export function closeActiveEditorGroup(): boolean {
  const state = windowPaneStore.getState()
  const activePane = getActiveEditorPane()
  if (!activePane) {
    return false
  }

  const paneGroups = getPaneScopeForPaneId(
    showingLayout(state),
    state.bottomLayout,
    state.panes,
    activePane.id,
  )
  if (paneGroups.length <= 1) {
    return false
  }

  state.paneActions.closePane(activePane.id)
  return true
}

export function closeOtherEditorGroups(): boolean {
  const state = windowPaneStore.getState()
  const activePane = getActiveEditorPane()
  if (!activePane) {
    return false
  }

  const editorGroups = getAllLeafIds(showingLayout(state)).flatMap((id) => {
    const pane = state.panes[id]
    return pane ? [pane] : []
  })
  if (!editorGroups.some((pane) => pane.id === activePane.id) || editorGroups.length <= 1) {
    return false
  }

  state.paneActions.setActivePane(activePane.id)
  for (const pane of editorGroups) {
    if (pane.id !== activePane.id) {
      state.paneActions.closePane(pane.id)
    }
  }

  return true
}

function collectSplitIds(node: LayoutNode): string[] {
  if (node.type === 'pane') {
    return []
  }

  return [node.id, ...collectSplitIds(node.first), ...collectSplitIds(node.second)]
}

export function resetEditorGroupSizes(): boolean {
  const state = windowPaneStore.getState()
  const splitIds = collectSplitIds(showingLayout(state))
  if (splitIds.length === 0) {
    return false
  }

  for (const splitId of splitIds) {
    state.paneActions.distributePaneSplit(splitId)
  }

  return true
}

export function moveActiveEditorToAdjacentGroup(direction: 'next' | 'previous'): boolean {
  const state = windowPaneStore.getState()
  const activePane = getActiveEditorPane()
  if (!activePane || !activePane.activeEditorTabId) {
    return false
  }

  const paneGroups = getPaneScopeForPaneId(
    showingLayout(state),
    state.bottomLayout,
    state.panes,
    activePane.id,
  )
  if (paneGroups.length <= 1) {
    return false
  }

  const currentIndex = paneGroups.findIndex((pane) => pane.id === activePane.id)
  if (currentIndex === -1) {
    return false
  }

  const offset = direction === 'next' ? 1 : -1
  const targetIndex = (currentIndex + offset + paneGroups.length) % paneGroups.length
  const targetPane = paneGroups[targetIndex]
  if (!targetPane || targetPane.id === activePane.id) {
    return false
  }

  state.paneActions.moveEditorTabToPane(activePane.activeEditorTabId, activePane.id, targetPane.id)
  return true
}

/** Show or hide the editor area beside a pane's chat (`PaneGroup.editorOpen`). */
export function togglePaneEditor(paneId: string): void {
  windowPaneStore.setState((state) => {
    const p = state.panes[paneId]
    if (!p) return {}
    return { panes: { ...state.panes, [paneId]: { ...p, editorOpen: !p.editorOpen } } }
  })
}
