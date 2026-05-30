import type { DBSchema } from 'idb'

export interface WorkspaceLayout {
  workspaceId: string
  panes: unknown[]
  activePane: string
  tabGroups: unknown[]
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
}
