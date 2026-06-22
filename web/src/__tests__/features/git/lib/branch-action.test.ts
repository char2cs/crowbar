import { describe, it, expect } from 'vitest'
import { resolveBranchAction } from '@/features/git/lib/branch-action'

const base = {
  hasUncommitted: false,
  hasParent: true,
  canMergeLocally: true,
  status: 'new',
  ahead: 0,
  behind: 0,
}

describe('resolveBranchAction', () => {
  it('uncommitted changes → commit (overrides everything, no remote secondary)', () => {
    expect(
      resolveBranchAction({ ...base, hasUncommitted: true, ahead: 2, status: 'pr-conflicts' }),
    ).toEqual({ kind: 'commit', remote: null })
  })

  it('clean + conflicts → resolve', () => {
    expect(resolveBranchAction({ ...base, status: 'pr-conflicts' }).kind).toBe('resolve')
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

  it('remote secondary: ahead → push, behind → pull, diverged → pull, synced → null (clean only)', () => {
    expect(resolveBranchAction({ ...base, ahead: 1 }).remote).toBe('push')
    expect(resolveBranchAction({ ...base, behind: 1 }).remote).toBe('pull')
    expect(resolveBranchAction({ ...base, ahead: 1, behind: 1 }).remote).toBe('pull')
    expect(resolveBranchAction(base).remote).toBeNull()
  })
})
