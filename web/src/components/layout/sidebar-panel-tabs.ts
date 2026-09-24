import type React from 'react'
import { FolderOpen, GitBranch } from '@phosphor-icons/react'
import type { SidebarTab } from '@/lib/store/sidebar-ui'
import {
  hasRepoWorkspace,
  type FocusedWorkspaceContext,
} from '@/features/window/stores/focused-workspace-context-store'

/** One tab of the sidebar panel. Availability is decided by the focused
 *  workspace context alone — never the route. */
export interface SidebarPanelTabDescriptor {
  tab: SidebarTab
  label: string
  Icon: React.ComponentType<{ size: number; weight: 'fill' | 'regular' }>
  available: (ctx: FocusedWorkspaceContext) => boolean
}

// Panel order (Files, then Git) — must match use-carousel-scroll-sync.ts's TABS.
export const SIDEBAR_PANEL_TABS: SidebarPanelTabDescriptor[] = [
  { tab: 'files', label: 'Files', Icon: FolderOpen, available: () => true },
  { tab: 'git', label: 'Git', Icon: GitBranch, available: hasRepoWorkspace },
]

export const FALLBACK_SIDEBAR_PANEL_TAB: SidebarTab = 'files'

/** Stable string key of the available tabs, for a primitive-returning selector. */
export function availableSidebarPanelTabsKey(ctx: FocusedWorkspaceContext): string {
  return SIDEBAR_PANEL_TABS.filter((t) => t.available(ctx))
    .map((t) => t.tab)
    .join(',')
}
