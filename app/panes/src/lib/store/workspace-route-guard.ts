import type { Repo } from '@/core/entities/repo-types'
import type { Operation } from '@/core/entities/operations'
import { holdsWorkspace } from '@/core/entities/repo-queries'

export interface WorkspaceRouteKnowledge {
  repos: Repo[]
  synced: boolean
  merging: boolean
}

export function shouldRedirectUnknownWorkspace(
  route: { projectId: string; repoId: string; wsId: string | undefined },
  known: WorkspaceRouteKnowledge,
): boolean {
  const { wsId } = route
  if (!wsId || known.merging) return false
  if (known.repos.some((r) => holdsWorkspace(r, wsId))) return false
  return known.synced
}

export function mergeRunsOn(ops: Readonly<Record<string, Operation>>, wsId: string): boolean {
  return Object.values(ops).some(
    (op) =>
      op.kind === 'workspace.merge_into_parent' &&
      op.subject.id === wsId &&
      (op.state === 'accepted' || op.state === 'running'),
  )
}
