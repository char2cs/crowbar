import type React from 'react'
import { ChatsCircle, FolderOpen, GitBranch } from '@phosphor-icons/react'
import type { SidebarTab } from '@/features/workspace/tree/tree-ui-store'
import {
  hasRepoWorkspace,
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
