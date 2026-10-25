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
  browser: 'chats',
  newTabPage: 'chats',
}

export function sidebarSurfaceOf(type: PaneContent['type']): SidebarTab {
  return SURFACE_BY_TYPE[type]
}

// A pane surface's root props. tabIndex -1: a press with nothing focusable under
// the pointer (preview prose, transcript text) focuses the root, not the body,
// so the focusin reaches use-sidebar-focus-mode. Pair it with `outline-none`.
export function sidebarSurfaceRoot(kind: SidebarTab) {
  return { 'data-sidebar-surface': kind, tabIndex: -1 } as const
}
