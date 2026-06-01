// web/src/features/workspace/stores/workspace-store.ts
import { createStore, type StoreApi } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import type { WorkspaceState } from './workspace-store.types'
import { createPaneSlice } from './slices/pane-slice'
import { createBufferSlice } from './slices/buffer-slice'
import { createLspSlice } from './slices/lsp-slice'
import { createTerminalSlice } from './slices/terminal-slice'
import { createFileWatcherSlice } from './slices/file-watcher-slice'
import { createRecentFilesSlice } from './slices/recent-files-slice'
import { createBranchReviewSlice } from './slices/branch-review-slice'
import { saveSessionToStore } from '@/features/editor/stores/buffer-session-persistence'
import { saveBranchReview } from '@/lib/persistence/branch-review'

export type WorkspaceStore = StoreApi<WorkspaceState>

export type WorkspaceSnapshot = Partial<
  Pick<WorkspaceState,
    | 'panes' | 'rootLayout' | 'bottomLayout'
    | 'activePaneId' | 'fullscreenPaneId' | 'mostRecentActivePaneIds'
    | 'buffers'
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
      ...createLspSlice(set, get, api),
      ...createTerminalSlice(set, get, api),
      ...createFileWatcherSlice(set, get, api),
      ...createRecentFilesSlice(set, get, api),
      ...createBranchReviewSlice(set, get, api),
      ...(snapshot ?? {}),
    }))
  )

  store.subscribe((state, prev) => {
    if (state.buffers === prev.buffers) return
    const activePane = state.panes[state.activePaneId] ?? null
    saveSessionToStore(state.buffers, activePane?.activeBufferId ?? null)
  })

  store.subscribe((state, prev) => {
    const br = state.branchReview
    const prevBr = prev.branchReview
    if (
      br.description === prevBr.description &&
      br.mergeStrategy === prevBr.mergeStrategy &&
      br.activeSubtab === prevBr.activeSubtab &&
      br.threads === prevBr.threads
    ) return
    saveBranchReview(wsId, {
      description: br.description,
      mergeStrategy: br.mergeStrategy,
      activeSubtab: br.activeSubtab,
      threads: br.threads,
    })
  })

  return store
}
