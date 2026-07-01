/** Which primary action the branch section offers, given the repo state. */
export type BranchActionKind =
  | 'commit' // uncommitted changes → open the commit dialog
  | 'resolve' // conflicts with the parent must be resolved (rebase onto parent)
  | 'pull-request' // parent is protected → open a PR
  | 'merge' // mergeable into parent → open the merge popover
  | 'sync-only' // no parent branch → push/pull only

export interface BranchActionInput {
  hasUncommitted: boolean
  hasParent: boolean
  canMergeLocally: boolean
  status: string
  ahead: number
  behind: number
}

export interface BranchAction {
  kind: BranchActionKind
  /** Remote secondary action shown alongside the primary (only when clean). */
  remote: 'push' | 'pull' | null
}

/**
 * Resolve the branch section's primary + secondary action from the current repo
 * state. Precedence: uncommitted (commit first) > conflicts-with-parent > protected
 * > mergeable > sync-only. A branch that conflicts with its parent — whether an
 * active merge conflict or a predicted one — arrives here as status pr-conflicts
 * (the backend folds the prediction into the status), so the single 'resolve'
 * case covers it. The remote secondary is only offered on a clean tree (you
 * commit before you push/pull); behind wins over ahead when diverged.
 */
export function resolveBranchAction(input: BranchActionInput): BranchAction {
  const { hasUncommitted, hasParent, canMergeLocally, status, ahead, behind } = input

  const remote: BranchAction['remote'] = hasUncommitted
    ? null
    : behind > 0
      ? 'pull'
      : ahead > 0
        ? 'push'
        : null

  if (hasUncommitted) return { kind: 'commit', remote }
  if (hasParent && status === 'pr-conflicts') return { kind: 'resolve', remote }
  if (hasParent && !canMergeLocally) return { kind: 'pull-request', remote }
  if (hasParent && canMergeLocally) return { kind: 'merge', remote }
  return { kind: 'sync-only', remote }
}
