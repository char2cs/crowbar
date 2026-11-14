import type { DBSchema } from 'idb'
import type { PaneGroup, LayoutNode, ViewRecord } from '@/features/panes/types/pane'
import type { PaneContent } from '@/features/panes/types/pane-content'
import type { ChatDTO, ProjectDTO, RepoDTO, WorkspaceDTO, FolderDTO } from '@/lib/types'
import type { EditorDraft } from '@/features/editor/documents/draft-store'

interface BranchReviewPersistedState {
  wsId: string
  /** Reviewed files: path -> the signature the file had when it was marked. */
  viewed: Record<string, string>
}

export interface WorkspaceLayout {
  /** The store's keyPath is still `workspaceId`, but the value is the fixed WINDOW_SESSION_ID:
   *  layout is window-level, one row per window. Each buffer carries its own workspaceId. */
  workspaceId: string
  /** The shape version (WINDOW_LAYOUT_VERSION); absent on an older layout, which hydrate.ts
   *  upgrades once at load. */
  version?: number
  panes: Record<string, PaneGroup>
  /** The Recents rows. A record without them hydrates to an empty band. */
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
  /** Retired keys from the pre-restyle tree's chevrons and project fold. The tree folds through
   *  `collapsedChatRows` only; a record still carrying these never gates visibility. */
  collapsedRepos?: string[]
  collapsedWorkspaces?: string[]
  collapsedProjects?: string[]
  /** Folded Chats-panel rows, folder and chat ids in one list (both collapse alike). Daemon uuids
   *  are workspace-unique, so the set is flat; a stale id matches nothing. Absent on older
   *  records: "nothing folded", the product default. */
  collapsedChatRows?: string[]
  /** The sidebar's last mode. Untyped on disk: an older build wrote modes that
   *  no longer exist, so hydrate parses it (`parseSidebarTab`). */
  activeTab?: string
  /** The host whose projects the switcher lists. Absent on an older record; 'all' on one written
   *  before the switcher showed one host at a time, which matches no host. */
  hostFilter?: string
  /** Host id → the project last open on it. Absent on a record written before it existed. */
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
  // Unsaved text of dirty documents (v11), keyed by document uri.
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
  // §6 entity cache (v7) — complete DTOs keyed by their own id. WS frames
  // upsert these by id; status:'deleted' tombstones remove them.
  crowbar_projects: { key: string; value: ProjectDTO }
  crowbar_repos: { key: string; value: RepoDTO }
  crowbar_workspaces: { key: string; value: WorkspaceDTO }
  // Sidebar grouping folders (v8): a new store needs a version bump to exist (see idb.ts).
  crowbar_folders: { key: string; value: FolderDTO }
  // Sidebar chat rows (v9): its own version bump too; a store whose upgrade never ran is not a
  // runtime error but an empty list forever.
  crowbar_chats: { key: string; value: ChatDTO }
}
