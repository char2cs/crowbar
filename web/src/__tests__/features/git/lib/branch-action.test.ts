import { describe, it, expect } from 'vitest'
import { resolveBranchAction } from '@/features/git/lib/branch-action'

const base = {
  hasUncommitted: false,
  hasParent: true,
  canMergeLocally: true,
  status: 'new',
}

describe('resolveBranchAction', () => {
  it('uncommitted changes → commit (overrides everything)', () => {
    expect(resolveBranchAction({ ...base, hasUncommitted: true, status: 'pr-conflicts' })).toEqual({
      kind: 'commit',
    })
  })

  it('conflicts with parent (pr-conflicts) → resolve, ahead of merge/pull-request', () => {
    // The backend folds a predicted conflict into pr-conflicts, so the single
    // resolve case covers both an active and a predicted conflict. It wins even
    // when the parent looks mergeable or protected.
    expect(resolveBranchAction({ ...base, status: 'pr-conflicts' }).kind).toBe('resolve')
    expect(
      resolveBranchAction({ ...base, status: 'pr-conflicts', canMergeLocally: false }).kind,
    ).toBe('resolve')
  })

  it('clean + protected parent → pull-request', () => {
    expect(resolveBranchAction({ ...base, canMergeLocally: false }).kind).toBe('pull-request')
  })

  it('clean + mergeable parent → merge', () => {
    expect(resolveBranchAction(base).kind).toBe('merge')
  })

  it('clean + no parent → sync-only', () => {
    expect(resolveBranchAction({ ...base, hasParent: false }).kind).toBe('sync-only')
  })
})
