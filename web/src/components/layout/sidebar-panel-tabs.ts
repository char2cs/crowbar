import type React from 'react'
import { ChatsCircle, FolderOpen, GitBranch } from '@phosphor-icons/react'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { SidebarTab } from '@/lib/store/sidebar-ui'
import {
  hasRepoWorkspace,
  useFocusedWorkspaceContextStore,
  type FocusedWorkspaceContext,
} from '@/features/window/stores/focused-workspace-context-store'

/** One mode of the sidebar. Availability is decided by the focused
 *  workspace context alone — never the route. */
interface SidebarPanelTabDescriptor {
  tab: SidebarTab
  label: string
  Icon: React.ComponentType<{ size: number; weight: 'fill' | 'regular' }>
  available: (ctx: FocusedWorkspaceContext) => boolean
  /** Drawn only while `available`, instead of as a disabled tile. */
  hideWhenUnavailable?: true
}

export const SIDEBAR_PANEL_TABS: readonly SidebarPanelTabDescriptor[] = [
  { tab: 'chats', label: 'Chats', Icon: ChatsCircle, available: () => true },
  { tab: 'files', label: 'Files', Icon: FolderOpen, available: (ctx) => ctx.workspaceId != null },
  {
    tab: 'git',
    label: 'Changes',
    Icon: GitBranch,
    available: hasRepoWorkspace,
    hideWhenUnavailable: true,
  },
]

const FALLBACK_SIDEBAR_PANEL_TAB: SidebarTab = 'chats'

/** Stable string key of the available tabs, for a primitive-returning selector. */
export function availableSidebarPanelTabsKey(ctx: FocusedWorkspaceContext): string {
  return SIDEBAR_PANEL_TABS.filter((t) => t.available(ctx))
    .map((t) => t.tab)
    .join(',')
}

function resolveShownTab(chosen: SidebarTab, ctx: FocusedWorkspaceContext): SidebarTab {
  const descriptor = SIDEBAR_PANEL_TABS.find((t) => t.tab === chosen)
  return descriptor?.available(ctx) ? chosen : FALLBACK_SIDEBAR_PANEL_TAB
}

/**
 * The mode on screen: the chosen one while the focused workspace offers it,
 * else Chats. Derived, not written back, so the store stays the one owner.
 */
export function useShownSidebarTab(): SidebarTab {
  const chosen = useSidebarStore((s) => s.activeTab)
  return useFocusedWorkspaceContextStore((ctx) => resolveShownTab(chosen, ctx))
}
