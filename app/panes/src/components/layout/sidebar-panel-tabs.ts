import type React from 'react'
import { ChatsCircle, FolderOpen, GitBranch } from '@phosphor-icons/react'
import type { SidebarTab } from '@/features/workspace/tree/tree-ui-store'
import {
  hasRepoWorkspace,
  type FocusedWorkspaceContext,
} from '@/features/window/stores/focused-workspace-context-store'

interface SidebarPanelTabDescriptor {
  tab: SidebarTab
  label: string
  Icon: React.ComponentType<{ size: number; weight: 'fill' | 'regular' }>
  available: (ctx: FocusedWorkspaceContext) => boolean
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
