import { API_BASE } from '@/lib/api'
import type { Repo, Workspace, WorkspaceStatus } from '@/lib/store/sidebar'
import type { RepoDTO, WorkspaceDTO } from '@/lib/types'

// Re-export the canonical §5 DTOs so existing importers (workspace-list, tests)
// keep a single source of truth instead of a divergent local shape.
export type { RepoDTO, WorkspaceDTO } from '@/lib/types'

const AVATAR_COLORS = [
  'bg-indigo-700',
  'bg-emerald-700',
  'bg-orange-700',
  'bg-sky-700',
  'bg-rose-700',
  'bg-violet-700',
  'bg-teal-700',
  'bg-amber-700',
]

function repoAvatarLabel(name: string): string {
  const words = name
    .replace(/[^a-zA-Z\s]/g, ' ')
    .trim()
    .split(/\s+/)
    .filter(Boolean)
  if (words.length === 0) return 'R'
  if (words.length === 1) return words[0][0].toUpperCase()
  return (words[0][0] + words[1][0]).toUpperCase()
}

function repoAvatarColor(name: string): string {
  let hash = 0
  for (const ch of name) hash = (hash * 31 + ch.charCodeAt(0)) & 0xffffff
  return AVATAR_COLORS[hash % AVATAR_COLORS.length]
}

// §5/§6: status now passes straight through — locked / pr-conflicts / deleted
// are first-class WorkspaceStatus values, no longer derived from removed
// agentRunning/locked/hasConflicts overlays. `working` is a separate flag the
// sidebar renders as an in-progress indicator, not a status.
function toSidebarStatus(ws: WorkspaceDTO): WorkspaceStatus {
  return ws.status as WorkspaceStatus
}

// repoAvatar resolves the sidebar avatar source from the §5 RepoDTO: a custom
// emoji takes precedence (rendered as an `emoji:<char>` marker the sidebar
// understands), otherwise the proxied /icon URL (prefixed with API_BASE so
// WKWebView can load it cross-origin), otherwise undefined → initials fallback.
function repoAvatarURL(repo: RepoDTO): string | undefined {
  if (repo.avatarEmoji) return `emoji:${repo.avatarEmoji}`
  if (repo.avatarUrl) return `${API_BASE}${repo.avatarUrl}`
  return undefined
}

export function toSidebarWorkspace(ws: WorkspaceDTO): Workspace {
  return {
    id: ws.id,
    branch: ws.branch,
    ...(ws.parentId ? { parentId: ws.parentId } : {}),
    status: toSidebarStatus(ws),
    added: ws.added,
    deleted: ws.deleted,
    working: ws.working,
    canMergeLocally: ws.canMergeLocally,
    mergeConflicts: ws.mergeConflicts,
    ...(ws.parentBranch ? { parentBranch: ws.parentBranch } : {}),
    ...(ws.prUrl ? { prUrl: ws.prUrl } : {}),
    // Always present (not conditionally spread): applyWorkspaceDTO merges frames
    // with {...w, ...ws}, so a cleared error must overwrite a stale one — an
    // omitted key would leave the previous error lingering.
    lastError: ws.lastError ?? '',
    // Always present (not conditionally spread): applyWorkspaceDTO merges frames
    // with {...w, ...ws}, so a HeldByPath cleared by a successful Retry (absent
    // under omitempty) must overwrite the stale value rather than linger.
    heldByPath: ws.heldByPath ?? '',
    age: '',
    ...(ws.localPath ? { localPath: ws.localPath } : {}),
  }
}

export function toSidebarRepo(repo: RepoDTO, workspaces: WorkspaceDTO[]): Repo {
  const repoWs = workspaces.filter((ws) => ws.repoId === repo.id)
  const defaultWs = repoWs.find((ws) => ws.isDefault)
  const sidebarWorkspaces: Workspace[] = []
  for (const ws of repoWs) {
    if (!ws.isDefault) sidebarWorkspaces.push(toSidebarWorkspace(ws))
  }
  return {
    id: repo.id,
    projectId: repo.projectId,
    name: repo.name,
    avatarLabel: repo.avatarLabel || repoAvatarLabel(repo.name),
    avatarColor: repo.avatarColor || repoAvatarColor(repo.name),
    avatarURL: repoAvatarURL(repo),
    workspaces: sidebarWorkspaces,
    // The default (repo-home) workspace is filtered out of `workspaces` above —
    // it is the repo header, not a tree row — so its live `working` overlay would
    // be dropped with it. Lift it onto the repo as defaultWorking so the header
    // and the context pill can spin the repo's icon during an agent turn.
    ...(defaultWs
      ? {
          defaultWorkspaceId: defaultWs.id,
          defaultBranch: defaultWs.branch,
          defaultWorking: defaultWs.working,
          // Its status is lifted too: adopted protected branches are locked AND
          // default, and mutation gating (isWorkspaceLockedInSidebar) must see
          // the lock even though the default ws is not a tree row.
          defaultWorkspaceStatus: toSidebarStatus(defaultWs),
        }
      : {}),
    ...(repo.path ? { localPath: repo.path } : {}),
  }
}

// buildRepoTree groups the workspace list under their repos to produce the
// nested Repo[] the sidebar renders.
export function buildRepoTree(repos: RepoDTO[], workspaces: WorkspaceDTO[]): Repo[] {
  return repos.map((repo) => toSidebarRepo(repo, workspaces))
}

// buildScopedRepoTree is buildRepoTree filtered to a single project. The entity
// cache holds repos from every imported project (each project's stream prunes
// only its own scope), but the sidebar only ever shows the ACTIVE project — so
// repos outside it are dropped before grouping. An empty/absent activeProjectId
// (no project selected) yields an empty tree.
export function buildScopedRepoTree(
  repos: RepoDTO[],
  workspaces: WorkspaceDTO[],
  activeProjectId: string | undefined,
): Repo[] {
  if (!activeProjectId) return []
  return buildRepoTree(
    repos.filter((repo) => repo.projectId === activeProjectId),
    workspaces,
  )
}
