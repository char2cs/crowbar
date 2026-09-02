import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import {
  getActiveWorkspaceId,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { createChat } from '@/features/agent/api/agent-api'
import { selectEnabledProviders } from '@/features/workspace/stores/slices/agent-chats-slice'
import { toastSpawnFailure } from '@/features/agent/lib/spawn-error'
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

// Opens the Branch Review surface for the active workspace as a pane tab.
// Returns the opened buffer id, or null when there is no active workspace.
export function openBranchReviewForActiveWorkspace(): string | null {
  const wsId = getActiveWorkspaceId()
  if (!wsId) {
    return null
  }

  return windowPaneStore
    .getState()
    .bufferActions.openContent({ type: 'branchReview', wsId, name: 'Branch Review' })
}

// Law 3 (spec §7.2): "nothing lands in a pane of its own; everything lands in
// the editor view [of a chat]". A pane must hold a chat before anything opens
// into its editor view. When `paneId` already has one, `openTab` just runs;
// otherwise this mints a new chat the same way ⌘N does (AGENT_NEW_CHAT in
// use-pane-keyboard.ts: first enabled provider, createChat, setPaneChat) and
// only then runs `openTab`. Shared by New Tab view's New Terminal/New File
// rows and their ⌘J/⌘⇧N keyboard equivalents so the sequence lives in one
// place.
export function ensurePaneChatThenOpen(wsId: string, paneId: string, openTab: () => void): void {
  const paneActions = windowPaneStore.getState().paneActions
  paneActions.setActivePane(paneId)

  if (windowPaneStore.getState().panes[paneId]?.chatId) {
    openTab()
    return
  }

  const workspaceStore = getOrCreateWorkspaceStore(wsId)
  const provider = selectEnabledProviders(workspaceStore.getState())[0]
  if (!provider) return

  createChat(wsId, provider.id)
    .then((chatId) => {
      workspaceStore.getState().setActiveAgentChatId(chatId)
      const actions = windowPaneStore.getState().paneActions
      actions.setActivePane(paneId)
      // A brand-new chat has no runner yet — null until it spawns one.
      actions.setPaneChat(paneId, chatId, null)
      openTab()
    })
    .catch((err: unknown) => toastSpawnFailure(err, provider.displayName, 'start'))
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
