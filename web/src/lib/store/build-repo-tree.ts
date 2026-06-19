import { API_BASE } from '@/lib/api'
import type { Repo, Workspace, WorkspaceStatus } from '@/lib/store/sidebar'

const AVATAR_COLORS = [
  'bg-indigo-700', 'bg-emerald-700', 'bg-orange-700', 'bg-sky-700',
  'bg-rose-700', 'bg-violet-700', 'bg-teal-700', 'bg-amber-700',
]

function repoAvatarLabel(name: string): string {
  const words = name.replace(/[^a-zA-Z\s]/g, ' ').trim().split(/\s+/).filter(Boolean)
  if (words.length === 0) return 'R'
  if (words.length === 1) return words[0][0].toUpperCase()
  return (words[0][0] + words[1][0]).toUpperCase()
}

function repoAvatarColor(name: string): string {
  let hash = 0
  for (const ch of name) hash = (hash * 31 + ch.charCodeAt(0)) & 0xffffff
  return AVATAR_COLORS[hash % AVATAR_COLORS.length]
}

export interface RepoDTO {
  id: string
  projectId: string
  name: string
  path: string
  defaultBranch: string
  avatarLabel: string
  avatarColor: string
  avatarUrl?: string
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
    avatarLabel: repo.avatarLabel || repoAvatarLabel(repo.name),
    avatarColor: repo.avatarColor || repoAvatarColor(repo.name),
    // Backend now always serves avatarUrl as the proxied /icon endpoint so
    // WKWebView can load it without cross-origin restrictions.
    avatarURL: repo.avatarUrl ? `${API_BASE}${repo.avatarUrl}` : undefined,
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
