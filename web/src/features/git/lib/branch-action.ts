/** Which action the merge box offers, given the repo state. */
export type BranchActionKind =
  | 'commit' // uncommitted changes → commit first
  | 'resolve' // conflicts with the parent must be resolved
  | 'pull-request' // parent is protected → open a PR
  | 'merge' // mergeable into parent
  | 'sync-only' // no parent branch → nothing to merge into

export interface BranchActionInput {
  hasUncommitted: boolean
  hasParent: boolean
  canMergeLocally: boolean
  status: string
}

export interface BranchAction {
  kind: BranchActionKind
}

/**
 * Precedence: uncommitted > conflicts-with-parent > protected > mergeable >
 * sync-only. A predicted conflict arrives as status pr-conflicts (the backend
 * folds the prediction in), so the single 'resolve' case covers it.
 */
export function resolveBranchAction(input: BranchActionInput): BranchAction {
  const { hasUncommitted, hasParent, canMergeLocally, status } = input

  if (hasUncommitted) return { kind: 'commit' }
  if (hasParent && status === 'pr-conflicts') return { kind: 'resolve' }
  if (hasParent && !canMergeLocally) return { kind: 'pull-request' }
  if (hasParent && canMergeLocally) return { kind: 'merge' }
  return { kind: 'sync-only' }
}
