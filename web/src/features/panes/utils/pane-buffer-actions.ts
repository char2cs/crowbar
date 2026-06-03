import { getActiveWorkspaceStoreRef } from "@/features/workspace/stores/workspace-store-ref";

export function ensureBufferInPane(
  paneId: string,
  bufferId: string,
  setActive = true,
): string | null {
  const state = getActiveWorkspaceStoreRef()?.getState();
  if (!state) return null;
  const paneActions = state.paneActions;
  const pane = paneActions.getPaneById(paneId);
  if (!pane) {
    return null;
  }

  if (!pane.bufferIds.includes(bufferId)) {
    paneActions.addBufferToPane(paneId, bufferId, setActive);
    if (setActive) {
      paneActions.activatePaneBuffer(paneId, bufferId);
    }
  } else if (setActive) {
    paneActions.activatePaneBuffer(paneId, bufferId);
  }

  return paneId;
}
