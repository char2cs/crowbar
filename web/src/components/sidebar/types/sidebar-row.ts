export type SidebarRowKind = 'chat' | 'branch' | 'folder' | 'workflow'

export interface SidebarRow {
  id: string
  kind: SidebarRowKind
  parentId: string | null
  order: number
  label: string
  labelProvisional?: boolean
  ownsWorktree: boolean
  workspaceId: string | null
  working: boolean
  hasView: boolean
  branchName?: string
}
