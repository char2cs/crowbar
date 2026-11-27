import type { DBSchema } from 'idb'
import type { PaneGroup, LayoutNode, ViewRecord } from '@/features/panes/types/pane'
import type { PaneContent } from '@/features/panes/types/pane-content'
import type { ChatDTO, ProjectDTO, RepoDTO, WorkspaceDTO, FolderDTO } from '@/lib/types'
import type { CachedFeed } from '@/features/agent/stores/lib/cached-feed'
import type { PlacementRecord } from '@/lib/persistence/placement-record'
import type { EditorDraft } from '@/features/editor/documents/draft-store'

interface BranchReviewPersistedState {
  wsId: string
  viewed: Record<string, string>
}

export interface WorkspaceLayout {
  workspaceId: string
  version?: number
  panes: Record<string, PaneGroup>
  views?: Record<string, ViewRecord>
  viewOrder?: string[]
  activeViewId?: string | null
  activeViewByProject?: Record<string, string>
  stage?: LayoutNode
  bottomLayout: LayoutNode
  activePaneId: string
  mostRecentActivePaneIds: string[]
  buffers: PaneContent[]
  updatedAt: number
}

export interface SidebarUI {
  collapsedRepos?: string[]
  collapsedWorkspaces?: string[]
  collapsedProjects?: string[]
  collapsedChatRows?: string[]
  activeTab?: string
  hostFilter?: string
  lastProjectByHost?: Record<string, string>
  updatedAt: number
}

interface CachedRecord<T> {
  key: string
  data: T
  fetchedAt: number
}

export interface CrowbarDB extends DBSchema {
  'workspace-layout': {
    key: string
    value: WorkspaceLayout
  }
  'editor-drafts': { key: string; value: EditorDraft }
  'sidebar-ui': {
    key: string
    value: SidebarUI
  }
  'branch-review': {
    key: string
    value: BranchReviewPersistedState
  }
  'workspaces-data': { key: string; value: CachedRecord<unknown> }
  'chat-history': { key: string; value: CachedRecord<unknown> }
  'projects-data': { key: string; value: CachedRecord<unknown> }
  crowbar_projects: { key: string; value: ProjectDTO }
  crowbar_repos: { key: string; value: RepoDTO }
  crowbar_workspaces: { key: string; value: WorkspaceDTO }
  crowbar_folders: { key: string; value: FolderDTO }
  crowbar_chats: { key: string; value: ChatDTO }
  'chat-feeds': {
    key: [string, string]
    value: CachedFeed
    indexes: { savedAt: number; chatId: string }
  }
  'host-placements': { key: string; value: PlacementRecord; indexes: { hostId: string } }
}
