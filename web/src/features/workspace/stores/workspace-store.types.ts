import type { TerminalSlice } from './slices/terminal-slice'
import type { FileWatcherSlice } from './slices/file-watcher-slice'
import type { RecentFilesSlice } from './slices/recent-files-slice'
import type { BranchReviewSlice } from './slices/branch-review-slice'
import type { AgentChatsSlice } from './slices/agent-chats-state'
import type { FileTreeSlice } from './slices/file-tree-slice'
import type { GitSlice } from './slices/git-slice'

export interface WorkspaceBaseState {
  workspaceId: string
}

// Per-workspace LIVE resources only; pane/buffer layout is window-level (window-pane-store.ts).
export type WorkspaceState = WorkspaceBaseState &
  TerminalSlice &
  FileWatcherSlice &
  RecentFilesSlice &
  BranchReviewSlice &
  AgentChatsSlice &
  FileTreeSlice &
  GitSlice
