// web/src/features/workspace/stores/workspace-store.types.ts
import type { PaneSlice } from './slices/pane-slice'
import type { BufferSlice } from './slices/buffer-slice'
import type { LspSlice } from './slices/lsp-slice'
import type { TerminalSlice } from './slices/terminal-slice'
import type { FileWatcherSlice } from './slices/file-watcher-slice'
import type { RecentFilesSlice } from './slices/recent-files-slice'
import type { BranchReviewSlice } from './slices/branch-review-slice'

export interface WorkspaceBaseState {
  workspaceId: string
}

export type WorkspaceState =
  & WorkspaceBaseState
  & PaneSlice
  & BufferSlice
  & LspSlice
  & TerminalSlice
  & FileWatcherSlice
  & RecentFilesSlice
  & BranchReviewSlice
