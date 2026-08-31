// web/src/features/workspace/stores/workspace-store.types.ts
import type { LspSlice } from './slices/lsp-slice'
import type { TerminalSlice } from './slices/terminal-slice'
import type { FileWatcherSlice } from './slices/file-watcher-slice'
import type { RecentFilesSlice } from './slices/recent-files-slice'
import type { BranchReviewSlice } from './slices/branch-review-slice'
import type { AgentChatsSlice } from './slices/agent-chats-slice'

export interface WorkspaceBaseState {
  workspaceId: string
}

// Task 26: `PaneSlice`/`BufferSlice` moved OFF this per-workspace state —
// they now live on the window-level store (`features/panes/stores/
// window-pane-store.ts`), created once and never destroyed on workspace
// switch/eviction. What remains here is genuinely per-workspace LIVE
// resources (an LSP connection, a terminal PTY, a file watcher, the agent
// chat list) — not declarative pane/buffer layout.
export type WorkspaceState = WorkspaceBaseState &
  LspSlice &
  TerminalSlice &
  FileWatcherSlice &
  RecentFilesSlice &
  BranchReviewSlice &
  AgentChatsSlice
