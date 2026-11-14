import { createStore, type StoreApi } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import type { WindowPaneState } from './window-pane-store.types'
import { createPaneSlice } from './slices/pane-slice'
import { createBufferSlice } from './slices/buffer-slice'
import { saveWorkspaceLayout } from '@/lib/persistence/workspace-layout'
import { toPersistedBuffers } from '@/features/panes/utils/persisted-layout'
import { viewIntegrityViolations } from '@/features/panes/lib/view-integrity'

export type WindowPaneStore = StoreApi<WindowPaneState>

/** @internal Exported for unit tests. */
export type WindowPaneSnapshot = Partial<
  Pick<
    WindowPaneState,
    | 'panes'
    | 'views'
    | 'viewOrder'
    | 'stage'
    | 'bottomLayout'
    | 'activeViewId'
    | 'activePaneId'
    | 'fullscreenPaneId'
    | 'mostRecentActivePaneIds'
    | 'activeViewByProject'
    | 'activeProjectId'
    | 'buffers'
  >
>

/** The window's pane/buffer store: never destroyed on a workspace switch or eviction, so a live
 *  layout survives its workspace leaving keep-alive. A pane's workspace comes from its record.
 *  @internal Exported so tests get a fresh, isolated store. */
export function createWindowPaneStore(snapshot?: WindowPaneSnapshot): WindowPaneStore {
  const store = createStore<WindowPaneState>()(
    immer((set, get, api): WindowPaneState => ({
      ...createPaneSlice(set, get, api),
      ...createBufferSlice(set, get, api),
      ...(snapshot ?? {}),
    })),
  )

  if (import.meta.env.DEV) {
    store.subscribe((state, prev) => {
      if (
        state.panes === prev.panes &&
        state.views === prev.views &&
        state.viewOrder === prev.viewOrder &&
        state.stage === prev.stage &&
        state.bottomLayout === prev.bottomLayout &&
        state.activeViewId === prev.activeViewId &&
        state.activePaneId === prev.activePaneId &&
        state.activeViewByProject === prev.activeViewByProject
      ) {
        return
      }
      const violations = viewIntegrityViolations(state)
      if (violations.length === 0) return
      if (import.meta.env.MODE === 'test')
        throw new Error(`view integrity: ${violations.join('; ')}`)
      console.error('view integrity violated:', violations)
    })
  }

  // A field left out of this comparison silently never persists.
  let persistTimer: ReturnType<typeof setTimeout> | undefined
  store.subscribe((state, prev) => {
    if (
      state.panes === prev.panes &&
      state.views === prev.views &&
      state.viewOrder === prev.viewOrder &&
      state.activeViewId === prev.activeViewId &&
      state.activeViewByProject === prev.activeViewByProject &&
      state.stage === prev.stage &&
      state.bottomLayout === prev.bottomLayout &&
      state.activePaneId === prev.activePaneId &&
      state.mostRecentActivePaneIds === prev.mostRecentActivePaneIds &&
      state.buffers === prev.buffers
    ) {
      return
    }

    if (persistTimer !== undefined) clearTimeout(persistTimer)
    persistTimer = setTimeout(() => {
      persistTimer = undefined
      const current = store.getState()
      saveWorkspaceLayout({
        // Overwritten with WINDOW_SESSION_ID in saveWorkspaceLayout.
        workspaceId: '',
        panes: current.panes,
        views: current.views,
        viewOrder: current.viewOrder,
        activeViewId: current.activeViewId,
        activeViewByProject: current.activeViewByProject,
        stage: current.stage,
        bottomLayout: current.bottomLayout,
        activePaneId: current.activePaneId,
        mostRecentActivePaneIds: current.mostRecentActivePaneIds,
        buffers: toPersistedBuffers(current.buffers),
        updatedAt: Date.now(),
      })
    }, 300)
  })

  return store
}

/** The one window-level pane/buffer store, created at module load and never destroyed. */
export const windowPaneStore = createWindowPaneStore()
