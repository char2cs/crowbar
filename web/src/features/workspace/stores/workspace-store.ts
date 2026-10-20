// web/src/features/workspace/stores/workspace-store.ts
import { createStore, type StoreApi } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import type { WorkspaceState } from './workspace-store.types'
import { createTerminalSlice } from './slices/terminal-slice'
import { createFileWatcherSlice } from './slices/file-watcher-slice'
import { createRecentFilesSlice } from './slices/recent-files-slice'
import { createBranchReviewSlice } from './slices/branch-review-slice'
import { createAgentChatsSlice } from './slices/agent-chats-slice'
import { createFileTreeSlice } from './slices/file-tree-slice'
import { createGitSlice } from './slices/git-slice'

export type WorkspaceStore = StoreApi<WorkspaceState>

export type WorkspaceSnapshot = Partial<Pick<WorkspaceState, 'recentFiles' | 'terminalLayout'>>

export function createWorkspaceStore(wsId: string, snapshot?: WorkspaceSnapshot): WorkspaceStore {
  return createStore<WorkspaceState>()(
    immer((set, get, api): WorkspaceState => ({
      workspaceId: wsId,
      ...createTerminalSlice(set, get, api),
      ...createFileWatcherSlice(set, get, api),
      ...createRecentFilesSlice(set, get, api),
      ...createBranchReviewSlice(set, get, api),
      ...createAgentChatsSlice(set, get, api),
      ...createFileTreeSlice(set, get, api),
      ...createGitSlice(set, get, api),
      ...(snapshot ?? {}),
    })),
  )
}
