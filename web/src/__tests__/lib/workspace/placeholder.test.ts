import { describe, expect, it } from 'vitest'
import { isPlaceholderWorkspace, placeholderReason } from '@/lib/workspace/placeholder'
import type { Workspace } from '@/lib/store/sidebar'

const ws = (over: Partial<Workspace> = {}): Workspace => ({
  id: 'w1',
  branch: 'develop',
  age: '',
  ...over,
})

describe('isPlaceholderWorkspace', () => {
  it('is true for a locked workspace with no localPath', () => {
    expect(isPlaceholderWorkspace(ws({ status: 'locked', heldByPath: '/repo' }))).toBe(true)
  })
  it('is false for a locked workspace that has a localPath (healthy managed)', () => {
    expect(isPlaceholderWorkspace(ws({ status: 'locked', localPath: '/managed' }))).toBe(false)
  })
  it('is false for a healthy non-locked workspace', () => {
    expect(isPlaceholderWorkspace(ws({ status: 'new', localPath: '/managed' }))).toBe(false)
  })
  // An imported feature branch whose worktree could not be created is a
  // placeholder too, and it is deliberately NOT locked (locked survives
  // provisioning and would block merge/rename/delete forever). Requiring locked
  // here is what left the import's failed row unrecognised — no reason, no
  // Retry/Detach…, and no toast.
  it('is true for an unlocked import placeholder held by another worktree', () => {
    expect(isPlaceholderWorkspace(ws({ status: 'new', heldByPath: '/Users/me/other' }))).toBe(true)
  })
})

describe('placeholderReason', () => {
  it('names the branch and the holder path when known', () => {
    const reason = placeholderReason(ws({ status: 'locked', heldByPath: '/Users/me/repo' }))
    expect(reason).toContain('develop')
    expect(reason).toContain('/Users/me/repo')
  })
  it('falls back to a generic reason without a holder path', () => {
    const reason = placeholderReason(ws({ status: 'locked' }))
    expect(reason).toContain('develop')
  })
  // A failure with no live holder has no reconstructable reason, so the daemon
  // persists the cause on the row; showing "Retry to provision it" instead would
  // hide the only explanation the user can act on.
  it('prefers the recorded cause when there is no holder path', () => {
    const reason = placeholderReason(ws({ status: 'new', lastError: 'worktree add: disk full' }))
    expect(reason).toContain('worktree add: disk full')
  })
  it('still prefers the holder path over a recorded cause', () => {
    const reason = placeholderReason(
      ws({ status: 'new', heldByPath: '/Users/me/other', lastError: 'branch_already_exists' }),
    )
    expect(reason).toContain('/Users/me/other')
  })
})
