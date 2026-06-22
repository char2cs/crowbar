/** Which primary action the branch section offers, given the repo state. */
export type BranchActionKind =
  | 'commit' // uncommitted changes → open the commit dialog
  | 'resolve' // merge conflicts must be resolved
  | 'pull-request' // parent is protected → open a PR
  | 'merge' // mergeable into parent → open the merge popover
  | 'merge-blocked' // mergeable, but the merge would conflict → blocked until resolved
  | 'sync-only' // no parent branch → push/pull only

export interface BranchActionInput {
  hasUncommitted: boolean
  hasParent: boolean
  canMergeLocally: boolean
  /** Predicted: merging into the parent would conflict (blocks the merge). */
  wouldConflict: boolean
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
 * state. Precedence: uncommitted (commit first) > active conflict > protected >
 * would-conflict (merge blocked) > mergeable > sync-only. The remote secondary is only offered on a clean tree
 * (you commit before you push/pull); behind wins over ahead when diverged.
 */
export function resolveBranchAction(input: BranchActionInput): BranchAction {
  const { hasUncommitted, hasParent, canMergeLocally, wouldConflict, status, ahead, behind } = input

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
  // Mergeable structurally, but a clean merge isn't possible yet → block it.
  if (hasParent && canMergeLocally && wouldConflict) return { kind: 'merge-blocked', remote }
  if (hasParent && canMergeLocally) return { kind: 'merge', remote }
  return { kind: 'sync-only', remote }
}
