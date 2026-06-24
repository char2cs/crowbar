import type { Repo } from '@/lib/store/sidebar'

/**
 * Returns the id of the Crowbar-MANAGED workspace that already holds `branch` in
 * this repo, or null if the branch is free. Exact, case-sensitive match (git
 * branch semantics), scoped to the repo. The default (main-worktree) workspace
 * is deliberately NOT counted — it is the imported repo folder, which merely
 * sits on some branch and must never block creating/importing that same branch
 * as a real managed workspace. (It is already filtered out of `repo.workspaces`.)
 */
export function findWorkspaceForBranch(repo: Repo, branch: string): string | null {
  const name = branch.trim()
  if (!name) return null
  const match = repo.workspaces.find((w) => w.branch === name)
  return match ? match.id : null
}
