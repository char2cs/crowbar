import { getActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { BOTTOM_PANE_ID } from '../constants/pane'
import type { LayoutNode } from '../types/pane'
import { getAllLeafIds } from './pane-layout'
import { getPaneScopeForPaneId } from './pane-routing'
import { createPaneBeside } from './pane-split-actions'

export const getShareableSplitBufferId = (bufferId: string | null | undefined) => {
  if (!bufferId) return undefined
  const activeBuffer = getActiveWorkspaceStoreRef()
    ?.getState()
    .buffers.find((buffer) => buffer.id === bufferId)
  if (activeBuffer?.type === 'terminal') {
    return undefined
  }

  return bufferId
}

function isEditorPaneId(paneId: string): boolean {
  if (paneId === BOTTOM_PANE_ID) {
    return false
  }

  const state = getActiveWorkspaceStoreRef()?.getState()
  if (!state) return false
  return getAllLeafIds(state.rootLayout).includes(paneId)
}

function getActiveEditorPane() {
  const state = getActiveWorkspaceStoreRef()?.getState()
  if (!state) return null
  const activePane = state.paneActions.getActivePane()
  if (!activePane || !isEditorPaneId(activePane.id)) {
    return null
  }

  return activePane
}

export function toggleActiveEditorGroupLock(): boolean {
  const state = getActiveWorkspaceStoreRef()?.getState()
  if (!state) return false
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
  const store = getActiveWorkspaceStoreRef()
  const wsId = getActiveWorkspaceId()
  if (!store || !wsId) {
    return null
  }

  return store
    .getState()
    .bufferActions.openContent({ type: 'branchReview', wsId, name: 'Branch Review' })
}

export function splitActiveEditorGroup(direction: 'horizontal' | 'vertical'): boolean {
  const activePane = getActiveEditorPane()
  if (!activePane) {
    return false
  }

  return splitEditorGroup(activePane.id, direction, activePane.activeBufferId)
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
  const state = getActiveWorkspaceStoreRef()?.getState()
  if (!state) return false
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
  const state = getActiveWorkspaceStoreRef()?.getState()
  if (!state) return false
  const activePane = getActiveEditorPane()
  if (!activePane) {
    return false
  }

  const editorGroups = getAllLeafIds(state.rootLayout)
    .map((id) => state.panes[id])
    .filter(Boolean)
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
  const state = getActiveWorkspaceStoreRef()?.getState()
  if (!state) return false
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
  const state = getActiveWorkspaceStoreRef()?.getState()
  if (!state) return false
  const activePane = getActiveEditorPane()
  if (!activePane || !activePane.activeBufferId) {
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

  state.paneActions.moveBufferToPane(activePane.activeBufferId, activePane.id, targetPane.id)
  return true
}
