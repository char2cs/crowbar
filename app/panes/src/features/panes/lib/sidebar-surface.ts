import type { SidebarTab } from '@/features/workspace/tree/tree-ui-store'
import type { PaneContent } from '../types/pane-content'

const SURFACE_BY_TYPE: Record<PaneContent['type'], SidebarTab> = {
  editor: 'files',
  markdownPreview: 'files',
  htmlPreview: 'files',
  branchReview: 'git',
  terminal: 'chats',
  browser: 'chats',
  newTabPage: 'chats',
}

export function sidebarSurfaceOf(type: PaneContent['type']): SidebarTab {
  return SURFACE_BY_TYPE[type]
}

export function sidebarSurfaceRoot(kind: SidebarTab) {
  return { 'data-sidebar-surface': kind, tabIndex: -1 } as const
}
