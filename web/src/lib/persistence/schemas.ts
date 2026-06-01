import type { DBSchema } from 'idb'
import type { PaneGroup, LayoutNode } from '@/features/panes/types/pane'
import type { PaneContent } from '@/features/panes/types/pane-content'
import type { ReviewThread, MergeStrategy } from '@/features/branch-review/types/review-types'

export interface BranchReviewPersistedState {
  wsId: string
  description: string
  mergeStrategy: MergeStrategy
  activeSubtab: 'about' | 'git' | 'diff'
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
}
