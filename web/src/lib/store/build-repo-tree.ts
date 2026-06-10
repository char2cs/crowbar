import type { Repo, Workspace, WorkspaceStatus } from '@/lib/store/sidebar'

export interface RepoDTO {
  id: string
  projectId: string
  name: string
  path: string
  defaultBranch: string
  avatarLabel: string
  avatarColor: string
}

export interface WorkspaceDTO {
  id: string
  repoId: string
  projectId: string
  parentId?: string
  branch: string
  status: WorkspaceStatus
  locked: boolean
  hasConflicts: boolean
  added: number
  deleted: number
  mergeStrategy: string
  agentRunning: boolean
}

function toSidebarStatus(ws: WorkspaceDTO): WorkspaceStatus {
  if (ws.agentRunning) return 'agent-running'
  if (ws.locked) return 'locked'
  return ws.status
}

function toSidebarWorkspace(ws: WorkspaceDTO): Workspace {
  return {
    id: ws.id,
    branch: ws.branch,
    ...(ws.parentId !== undefined && { parentId: ws.parentId }),
    status: toSidebarStatus(ws),
    added: ws.added,
    deleted: ws.deleted,
    hasConflicts: ws.hasConflicts,
    age: '',
  }
}

// buildRepoTree groups the backend's flat workspace list under their repos to
// produce the nested Repo[] the sidebar renders. Workspace parent/child links
// are overlaid separately from the persisted hierarchy.
export function buildRepoTree(repos: RepoDTO[], workspaces: WorkspaceDTO[]): Repo[] {
  return repos.map((repo) => ({
    id: repo.id,
    name: repo.name,
    avatarLabel: repo.avatarLabel,
    avatarColor: repo.avatarColor,
    workspaces: workspaces.filter((ws) => ws.repoId === repo.id).map(toSidebarWorkspace),
  }))
}
