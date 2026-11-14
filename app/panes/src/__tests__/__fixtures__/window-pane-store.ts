import {
  createWindowPaneStore,
  windowPaneStore,
  type WindowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { releaseDocumentOf } from '@/features/panes/lib/buffer-release'

/** Resets data fields only: copying a fresh store's whole state would bind `target`'s actions
 *  to that throwaway store's set/get, so later calls would mutate it instead. */
export function resetWindowPaneStoreForTests(target: WindowPaneStore = windowPaneStore): void {
  // The buffers go as closing them would, documents included.
  for (const b of target.getState().buffers) releaseDocumentOf(b)
  const fresh = createWindowPaneStore().getState()
  target.setState({
    panes: fresh.panes,
    views: fresh.views,
    viewOrder: fresh.viewOrder,
    stage: fresh.stage,
    bottomLayout: fresh.bottomLayout,
    activeViewId: fresh.activeViewId,
    activePaneId: fresh.activePaneId,
    fullscreenPaneId: fresh.fullscreenPaneId,
    mostRecentActivePaneIds: fresh.mostRecentActivePaneIds,
    activeViewByProject: fresh.activeViewByProject,
    activeProjectId: fresh.activeProjectId,
    buffers: fresh.buffers,
    closedBuffersHistory: fresh.closedBuffersHistory,
    pendingClose: fresh.pendingClose,
    maxOpenTabs: fresh.maxOpenTabs,
  })
}
