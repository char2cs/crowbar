import type { SidebarTab } from '@/lib/store/sidebar-ui'
import type { PaneContent } from '../types/pane-content'

// Each tab type's `data-sidebar-surface` value (use-sidebar-focus-mode.ts).
const SURFACE_BY_TYPE: Record<PaneContent['type'], SidebarTab> = {
  editor: 'files',
  markdownPreview: 'files',
  htmlPreview: 'files',
  csvPreview: 'files',
  commitDiff: 'git',
  branchReview: 'git',
  terminal: 'chats',
}

export function sidebarSurfaceOf(type: PaneContent['type']): SidebarTab {
  return SURFACE_BY_TYPE[type]
}
