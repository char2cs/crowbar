// Pure eligibility state machine for the merge section.
// No React, no side effects — all inputs are passed as plain values.

export type MergeStateKind = 'eligible' | 'uncommitted' | 'protected' | 'conflict'

export interface MergeStateInput {
  canMergeLocally: boolean
  hasUncommitted: boolean
  status: string
}

export interface MergeStateResult {
  kind: MergeStateKind
  reason: string
}

/**
 * Resolve the merge eligibility state from the workspace's current conditions.
 *
 * Precedence (highest to lowest):
 *   conflict (status === 'pr-conflicts')
 *   > protected (!canMergeLocally)
 *   > uncommitted (hasUncommitted)
 *   > eligible
 */
export function resolveMergeState({
  canMergeLocally,
  hasUncommitted,
  status,
}: MergeStateInput): MergeStateResult {
  if (status === 'pr-conflicts') {
    return {
      kind: 'conflict',
      reason: 'This branch has merge conflicts that must be resolved before merging.',
    }
  }

  if (!canMergeLocally) {
    return {
      kind: 'protected',
      reason: 'The parent branch is protected — open a pull request to merge.',
    }
  }

  if (hasUncommitted) {
    return {
      kind: 'uncommitted',
      reason: 'Commit your changes before merging.',
    }
  }

  return {
    kind: 'eligible',
    reason: 'Branch is local & unprotected — ready to merge.',
  }
}
