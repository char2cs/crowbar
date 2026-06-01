import type { DBSchema } from 'idb'
import type { PaneGroup, LayoutNode } from '@/features/panes/types/pane'
import type { PaneContent } from '@/features/panes/types/pane-content'

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
  updatedAt: number
}

export interface WorkspaceHierarchy {
  repoId: string
  entries: Array<{ wsId: string; parentId?: string }>
  updatedAt: number
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
  'query-cache': {
    key: string
    value: string
  }
  'sidebar-ui': {
    key: string
    value: SidebarUI
  }
  'workspace-hierarchy': {
    key: string
    value: WorkspaceHierarchy
  }
}
