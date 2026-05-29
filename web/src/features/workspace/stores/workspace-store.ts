// web/src/features/workspace/stores/workspace-store.ts
import { createStore, type StoreApi } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import type { WorkspaceState } from './workspace-store.types'
import { createPaneSlice } from './slices/pane-slice'
import { createBufferSlice } from './slices/buffer-slice'
import { createWorkflowSlice } from './slices/workflow-slice'
import { createLspSlice } from './slices/lsp-slice'
import { createTerminalSlice } from './slices/terminal-slice'
import { createFileWatcherSlice } from './slices/file-watcher-slice'
import { createRecentFilesSlice } from './slices/recent-files-slice'
import { saveSessionToStore } from '@/features/editor/stores/buffer-session-persistence'
import { findPaneGroup } from '@/features/panes/utils/pane-tree'

export type WorkspaceStore = StoreApi<WorkspaceState>

export type WorkspaceSnapshot = Partial<
  Pick<WorkspaceState,
    | 'paneRoot' | 'bottomRoot' | 'activePaneId' | 'fullscreenPaneId' | 'mostRecentActivePaneIds'
    | 'buffers'
    | 'currentStepId'
    | 'recentFiles'
    | 'terminalLayout'
  >
>

export function createWorkspaceStore(wsId: string, snapshot?: WorkspaceSnapshot): WorkspaceStore {
  const store = createStore<WorkspaceState>()(
    immer((set, get, api): WorkspaceState => ({
      workspaceId: wsId,
      ...createPaneSlice(set, get, api),
      ...createBufferSlice(set, get, api),
      ...createWorkflowSlice(set, get, api),
      ...createLspSlice(set, get, api),
      ...createTerminalSlice(set, get, api),
      ...createFileWatcherSlice(set, get, api),
      ...createRecentFilesSlice(set, get, api),
      // snapshot only contains serializable data fields (see WorkspaceSnapshot type) —
      // action objects and non-serializable state (Sets, functions) are excluded
      ...(snapshot ?? {}),
    }))
  )

  store.subscribe((state, prev) => {
    if (state.buffers === prev.buffers) return
    const activePane =
      findPaneGroup(state.paneRoot, state.activePaneId) ??
      findPaneGroup(state.bottomRoot, state.activePaneId)
    saveSessionToStore(state.buffers, activePane?.activeBufferId ?? null)
  })

  return store
}
