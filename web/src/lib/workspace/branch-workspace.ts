import type { Repo } from '@/lib/store/sidebar'

/**
 * Returns the id of the workspace that already holds `branch` in this repo —
 * including the default (main-worktree) workspace — or null if the branch is
 * free. Exact, case-sensitive match (git branch semantics), scoped to the repo.
 * Used to enforce one-workspace-per-branch in the create input.
 */
export function findWorkspaceForBranch(repo: Repo, branch: string): string | null {
  const name = branch.trim()
  if (!name) return null
  if (repo.defaultBranch === name && repo.defaultWorkspaceId) {
    return repo.defaultWorkspaceId
  }
  const match = repo.workspaces.find((w) => w.branch === name)
  return match ? match.id : null
}
