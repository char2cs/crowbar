import type { DBSchema } from 'idb'
import type { PaneGroup, LayoutNode } from '@/features/panes/types/pane'
import type { PaneContent } from '@/features/panes/types/pane-content'
import type {
  ReviewThread,
  MergeStrategy,
} from '@/features/workspace/stores/slices/branch-review-slice'
import type { ProjectDTO, RepoDTO, WorkspaceDTO, ThreadDTO, FolderDTO } from '@/lib/types'

export interface BranchReviewPersistedState {
  wsId: string
  description: string
  mergeStrategy: MergeStrategy
  threads: ReviewThread[]
  updatedAt: number
}

export interface WorkspaceLayout {
  workspaceId: string
  panes: Record<string, PaneGroup>
  rootLayout: LayoutNode
  bottomLayout: LayoutNode
  activePaneId: string
  mostRecentActivePaneIds: string[]
  buffers: PaneContent[]
  sidebarWidth: number
  rightSidebarWidth: number
  updatedAt: number
}

export interface EditorState {
  workspaceId: string
  bufferId: string
  cursorLine: number
  cursorColumn: number
  scrollTop: number
  folds: [number, number][]
  updatedAt: number
}

export interface UIPreferences {
  theme: string
  fontSize: number
  fontFamily: string
  tabSize: number
  wordWrap: boolean
  minimap: boolean
  updatedAt: number
}

export interface SidebarUI {
  collapsedRepos: string[]
  collapsedWorkspaces?: string[]
  /**
   * Projects the user has folded away — same polarity as the two lists above,
   * so an absent value replays as "everything open", which is the product
   * default (see `collapsedProjects` in lib/store/sidebar.ts).
   *
   * A record written by an earlier build carries `expandedProjects` instead.
   * It is simply ignored: pre-production, a stale layout falls back gracefully
   * and is rewritten on the next toggle. No migration code.
   */
  collapsedProjects?: string[]
  updatedAt: number
}

export interface WorkspaceHierarchy {
  repoId: string
  entries: Array<{ wsId: string; parentId?: string }>
  updatedAt: number
}

export interface CachedRecord<T> {
  key: string
  data: T
  fetchedAt: number
}

export interface CrowbarDB extends DBSchema {
  'workspace-layout': {
    key: string
    value: WorkspaceLayout
  }
  'editor-state': {
    key: [string, string]
    value: EditorState
    indexes: { workspaceId: string }
  }
  'ui-preferences': {
    key: string
    value: UIPreferences
  }
  'sidebar-ui': {
    key: string
    value: SidebarUI
  }
  'workspace-hierarchy': {
    key: string
    value: WorkspaceHierarchy
  }
  'branch-review': {
    key: string
    value: BranchReviewPersistedState
  }
  'workspaces-data': { key: string; value: CachedRecord<unknown> }
  'git-data': { key: string; value: CachedRecord<unknown> }
  'file-tree-data': { key: string; value: CachedRecord<unknown> }
  'branch-review-data': { key: string; value: CachedRecord<unknown> }
  'chat-history': { key: string; value: CachedRecord<unknown> }
  'projects-data': { key: string; value: CachedRecord<unknown> }
  'chats-data': { key: string; value: CachedRecord<unknown> }
  // §6 entity cache (v7) — complete DTOs keyed by their own id. WS frames
  // upsert these by id; status:'deleted' tombstones remove them.
  crowbar_projects: { key: string; value: ProjectDTO }
  crowbar_repos: { key: string; value: RepoDTO }
  crowbar_workspaces: { key: string; value: WorkspaceDTO }
  crowbar_threads: { key: string; value: ThreadDTO }
  // Sidebar grouping folders (v8). A new object store only exists if the DB is
  // opened at a version that ran its upgrade, so adding one is a version bump —
  // see idb.ts.
  crowbar_folders: { key: string; value: FolderDTO }
}
