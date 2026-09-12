import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
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

  return getAllLeafIds(windowPaneStore.getState().rootLayout).includes(paneId)
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

// Opens the Branch Review surface for the given workspace as a pane tab.
// Returns the opened buffer id, or null when there is no workspace to open
// one for. The caller supplies the workspace: a button living inside a
// SPECIFIC pane (TabBar, ChatOnlyPaneHeader) must open review for THAT
// pane's own chat/workspace, never whichever one happens to be globally
// active — a different pane in the same split can easily be showing a
// different chat and workspace entirely.
export function openBranchReviewForWorkspace(wsId: string | null | undefined): string | null {
  if (!wsId) {
    return null
  }

  return windowPaneStore
    .getState()
    .bufferActions.openContent({ type: 'branchReview', wsId, name: 'Branch Review' })
}

// Opens the Branch Review surface for the globally active workspace — for
// callers with no pane/chat context of their own (GitPanel, a keyboard
// shortcut), where "active workspace" is genuinely the only meaningful
// answer.
export function openBranchReviewForActiveWorkspace(): string | null {
  return openBranchReviewForWorkspace(getActiveWorkspaceId())
}

// Law 3 (spec §7.2): "nothing lands in a pane of its own; everything lands in
// the editor view [of a chat]". A pane must hold a chat before anything opens
// into its editor view. When `paneId` already has one, `openTab` just runs.
//
// Every workspace already has a real, permanent owning chat — the daemon
// mints one per locked branch, repo home and project home
// (rows-from-repo.ts's `branchRowIds` doc) — so a chatless PANE never means a
// chatless WORKSPACE. This resolves and reuses that owning chat
// (`getOwningChatId`, the same read every other workspace-scoped surface
// uses — lsp-client.ts, terminal.tsx, branch-review-pane.tsx, etc.) rather
// than minting a second, redundant chat, which is what this used to do
// unconditionally on any pane that merely hadn't been told its workspace's
// chat yet. If no owning chat can be resolved (e.g. the sidebar hasn't
// loaded this workspace's scope yet), this does nothing — never creates one
// as a side effect of opening a terminal, a file, or a branch review.
export function ensurePaneChatThenOpen(wsId: string, paneId: string, openTab: () => void): void {
  const paneActions = windowPaneStore.getState().paneActions
  paneActions.setActivePane(paneId)

  if (windowPaneStore.getState().panes[paneId]?.chatId) {
    openTab()
    return
  }

  const owningChatId = getOwningChatId(wsId)
  if (!owningChatId) return

  // Same dedup rule every other "put a chat in a pane" path already follows
  // (open-agent-chat.ts's openAgentChat, drop-actions.ts's openChatIntoPane):
  // a chat already showing somewhere is REVEALED, never duplicated into a
  // second pane.
  const existingPane = Object.values(windowPaneStore.getState().panes).find(
    (p) => p.chatId === owningChatId,
  )
  if (existingPane) {
    paneActions.setActivePane(existingPane.id)
    openTab()
    return
  }

  // No runner known yet — agent-chat-pane's own mount-time revive resolves
  // and writes back the real one (same convention openAgentChat/
  // openChatIntoPane use for a freshly attached chat).
  paneActions.setPaneChat(paneId, owningChatId, null)
  openTab()
}

export function splitActiveEditorGroup(direction: 'horizontal' | 'vertical'): boolean {
  const activePane = getActiveEditorPane()
  if (!activePane) {
    return false
  }

  // I8 (Task 26 fix round 1): same activeBufferId dead-field bug as above.
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
    state.rootLayout,
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

  const editorGroups = getAllLeafIds(state.rootLayout).flatMap((id) => {
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
  const splitIds = collectSplitIds(state.rootLayout)
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
    state.rootLayout,
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

  // I8 (Task 26 fix round 1): moveBufferToPane has not existed on PaneActions
  // since Task 1's editorTabIds rename — real name is moveEditorTabToPane.
  state.paneActions.moveEditorTabToPane(activePane.activeEditorTabId, activePane.id, targetPane.id)
  return true
}
