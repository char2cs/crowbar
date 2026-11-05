import { windowPaneStore } from '@/features/panes/stores/window-pane-store'

export function ensureBufferInPane(
  paneId: string,
  bufferId: string,
  setActive = true,
): string | null {
  const state = windowPaneStore.getState()
  const paneActions = state.paneActions
  const pane = paneActions.getPaneById(paneId)
  if (!pane) {
    return null
  }

  if (!pane.editorTabIds.includes(bufferId)) {
    // Given only an id, this can reference an existing buffer but never construct one.
    // addEditorTabToPane always activates, so `setActive` only matters when already present.
    const tab = state.buffers.find((b) => b.id === bufferId)
    if (tab) paneActions.addEditorTabToPane(paneId, tab)
  } else if (setActive) {
    paneActions.activateEditorTabInPane(paneId, bufferId)
  }

  return paneId
}
