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
    projectId: repo.projectId,
    name: repo.name,
    avatarLabel: repo.avatarLabel,
    avatarColor: repo.avatarColor,
    workspaces: workspaces.filter((ws) => ws.repoId === repo.id).map(toSidebarWorkspace),
  }))
}

// countReposByProject derives the per-project repo count the project cards
// show, from the same repo list the sidebar already fetched.
export function countReposByProject(repos: Array<{ projectId?: string }>): Map<string, number> {
  const counts = new Map<string, number>()
  for (const repo of repos) {
    if (!repo.projectId) continue
    counts.set(repo.projectId, (counts.get(repo.projectId) ?? 0) + 1)
  }
  return counts
}
