import type { Repo } from '@/core/entities/repo-types'

// The id of the Crowbar-managed workspace already holding `branch` in this repo, or null; exact,
// case-sensitive. The default (main-worktree) workspace never counts: it is the imported folder,
// merely sitting on a branch (and already filtered out of `repo.workspaces`).
export function findWorkspaceForBranch(repo: Repo, branch: string): string | null {
  const name = branch.trim()
  if (!name) return null
  const match = repo.workspaces.find((w) => w.branch === name)
  return match ? match.id : null
}
