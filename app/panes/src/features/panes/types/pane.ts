export interface PaneGroup {
  id: string
  type: 'group'
  chatId: string | null
  runnerId: string | null
  workspaceId?: string | null
  draftParentId?: string
  editorTabIds: string[]
  activeEditorTabId: string | null
  editorOpen: boolean
  chatSelected?: boolean
  locked?: boolean
  viewId: string | null
}

export interface ViewMember {
  chatId: string
  workspaceId: string | null
}

export interface ViewRecord {
  id: string
  projectId: string
  layout: LayoutNode
  pinned?: boolean
}

export interface LayoutLeaf {
  type: 'pane'
  id: string
}

export interface LayoutSplit {
  type: 'split'
  id: string
  direction: 'horizontal' | 'vertical'
  sizes: [number, number]
  first: LayoutNode
  second: LayoutNode
}

export type LayoutNode = LayoutLeaf | LayoutSplit

export type SplitDirection = 'horizontal' | 'vertical'
export type SplitPlacement = 'before' | 'after'

export interface PanePosition {
  atLeft: boolean
  atTop: boolean
  atRight: boolean
  atBottom: boolean
}

export const ROOT_PANE_POSITION: PanePosition = {
  atLeft: true,
  atTop: true,
  atRight: true,
  atBottom: true,
}
