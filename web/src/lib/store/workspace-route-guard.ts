import type { Repo } from '@/lib/store/sidebar'
import type { Operation } from '@/lib/store/operations'
import { holdsWorkspace } from '@/lib/store/repo-tree'

/** What the sidebar tree holds, and which parts of it the daemon has answered. */
export interface WorkspaceRouteKnowledge {
  repos: Repo[]
  /** Repos whose workspace list, as the daemon answered it, is in `repos`. */
  workspacesRead: ReadonlySet<string>
  /** Projects whose repo list, as the daemon answered it, is in `repos`. */
  reposRead: ReadonlySet<string>
  /** A merge folding the routed workspace runs: the merge, not the guard, moves the user. */
  merging: boolean
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
  if (!wsId || known.merging) return false
  if (known.repos.some((r) => holdsWorkspace(r, wsId))) return false
  const repo = known.repos.find((r) => r.id === route.repoId)
  return repo ? known.workspacesRead.has(repo.id) : known.reposRead.has(route.projectId)
}

/**
 * Whether a merge into its parent runs on `wsId`. Its fold drops the child before
 * the terminal frame, and the merge then leads to the parent. A fold frame that
 * lands after the terminal one still loses to this guard's redirect home.
 */
export function mergeRunsOn(ops: Readonly<Record<string, Operation>>, wsId: string): boolean {
  return Object.values(ops).some(
    (op) =>
      op.kind === 'workspace.merge_into_parent' &&
      op.subject.id === wsId &&
      (op.state === 'accepted' || op.state === 'running'),
  )
}
