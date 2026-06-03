import { getActiveWorkspaceStoreRef } from "@/features/workspace/stores/workspace-store-ref";
import type { SplitDirection, SplitPlacement } from "../types/pane";

export function createPaneBeside(
  paneId: string,
  direction: SplitDirection,
  placement: SplitPlacement = "after",
  bufferId?: string,
): string | null {
  const state = getActiveWorkspaceStoreRef()?.getState();
  if (!state) return null;
  return state.paneActions.splitPane(paneId, direction, bufferId, placement) ?? null;
}
