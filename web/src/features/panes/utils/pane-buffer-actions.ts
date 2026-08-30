import { getActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'

export function ensureBufferInPane(
  paneId: string,
  bufferId: string,
  setActive = true,
): string | null {
  const state = getActiveWorkspaceStoreRef()?.getState()
  if (!state) return null
  const paneActions = state.paneActions
  const pane = paneActions.getPaneById(paneId)
  if (!pane) {
    return null
  }

  if (!pane.editorTabIds.includes(bufferId)) {
    // addEditorTabToPane takes the tab's own EditorTabBase-shaped object (only
    // its `id` is read today, but the shape is the contract) — this utility
    // only ever receives a bare id, so it can add a REFERENCE to a buffer
    // that already exists in state.buffers, but has nothing to construct a
    // new one from. It also always activates whatever it adds, so `setActive`
    // can only suppress activation on the "already present" branch below.
    const tab = state.buffers.find((b) => b.id === bufferId)
    if (tab) paneActions.addEditorTabToPane(paneId, tab)
  } else if (setActive) {
    paneActions.activateEditorTabInPane(paneId, bufferId)
  }

  return paneId
}
