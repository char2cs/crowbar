import type { Repo } from '@/lib/store/sidebar'

/** What the sidebar tree holds, and which parts of it the daemon has answered. */
export interface WorkspaceRouteKnowledge {
  repos: Repo[]
  /** Repos whose workspace list, as the daemon answered it, is in `repos`. */
  workspacesRead: ReadonlySet<string>
  /** Projects whose repo list, as the daemon answered it, is in `repos`. */
  reposRead: ReadonlySet<string>
}

/**
 * Whether an `/ide/…/<wsId>` route names a workspace that does not exist.
 * Only the daemon's answer can say so: a tree read from a cold cache is empty
 * because nothing is known yet, not because the workspace is gone.
 */
export function shouldRedirectUnknownWorkspace(
  route: { projectId: string; repoId: string; wsId: string | undefined },
  known: WorkspaceRouteKnowledge,
): boolean {
  const { wsId } = route
  if (!wsId) return false
  const holds = (r: Repo) =>
    r.defaultWorkspaceId === wsId || r.workspaces.some((w) => w.id === wsId)
  if (known.repos.some(holds)) return false
  const repo = known.repos.find((r) => r.id === route.repoId)
  return repo ? known.workspacesRead.has(repo.id) : known.reposRead.has(route.projectId)
}
