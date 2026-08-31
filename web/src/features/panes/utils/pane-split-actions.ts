import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { SplitDirection, SplitPlacement } from '../types/pane'

export function createPaneBeside(
  paneId: string,
  direction: SplitDirection,
  placement: SplitPlacement = 'after',
  bufferId?: string,
): string | null {
  return (
    windowPaneStore.getState().paneActions.splitPane(paneId, direction, bufferId, placement) ??
    null
  )
}
