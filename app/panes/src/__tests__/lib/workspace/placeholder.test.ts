import { describe, expect, it } from 'vitest'
import { placeholderKind, placeholderReason } from '@/lib/workspace/placeholder'
import type { Workspace } from '@/core/entities/repo-types'

const ws = (over: Partial<Workspace> = {}): Workspace => ({
  id: 'w1',
  branch: 'develop',
  age: '',
  provisioning: over.localPath ? 'provisioned' : 'placeholder',
  ...over,
})

describe('placeholderKind', () => {
  it('is unprovisioned for a locked workspace with no localPath', () => {
    expect(placeholderKind(ws({ status: 'locked', heldByPath: '/repo' }), 'main')).toBe(
      'unprovisioned',
    )
  })
  it('is none for a locked workspace that has a localPath (healthy managed)', () => {
    expect(placeholderKind(ws({ status: 'locked', localPath: '/managed' }), 'main')).toBe('none')
  })
  it('is none for a healthy non-locked workspace', () => {
    expect(placeholderKind(ws({ status: 'new', localPath: '/managed' }), 'main')).toBe('none')
  })
  it('is unprovisioned for an unlocked import placeholder held by another worktree', () => {
    expect(placeholderKind(ws({ status: 'new', heldByPath: '/Users/me/other' }), 'main')).toBe(
      'unprovisioned',
    )
  })

  it('is own-checkout when the branch is the repo’s own default branch', () => {
    const kind = placeholderKind(
      ws({ branch: 'main', status: 'locked', heldByPath: '/Users/me/repo-beta' }),
      'main',
    )
    expect(kind).toBe('own-checkout')
  })

  it('is own-checkout even when the holder path spells the repo root differently', () => {
    const kind = placeholderKind(
      ws({ branch: 'main', status: 'locked', heldByPath: '/private/var/x/repo-beta' }),
      'main',
    )
    expect(kind).toBe('own-checkout')
  })

  it('is unprovisioned for a non-default branch held by somebody else', () => {
    const kind = placeholderKind(
      ws({ branch: 'release/1.x', status: 'locked', heldByPath: '/Users/me/elsewhere' }),
      'main',
    )
    expect(kind).toBe('unprovisioned')
  })

  it('never reads as own-checkout without a repo default branch', () => {
    expect(placeholderKind(ws({ branch: 'main', heldByPath: '/repo' }), undefined)).toBe(
      'unprovisioned',
    )
    expect(placeholderKind(ws({ branch: '', heldByPath: '/repo' }), '')).toBe('unprovisioned')
  })
})

describe('placeholderReason', () => {
  it('names the branch and the holder path when known', () => {
    const w = ws({ status: 'locked', heldByPath: '/Users/me/repo' })
    const reason = placeholderReason(w, placeholderKind(w, 'main'), null)
    expect(reason).toContain('develop')
    expect(reason).toContain('/Users/me/repo')
  })
  it('falls back to a generic reason without a holder path', () => {
    const w = ws({ status: 'locked' })
    expect(placeholderReason(w, placeholderKind(w, 'main'), null)).toContain('develop')
  })
  it("a failed retry shows the operation's reason on the placeholder", () => {
    const w = ws({ status: 'new' })
    const reason = placeholderReason(w, placeholderKind(w, 'main'), {
      code: 'conflict',
      message: 'branch is still held',
    })
    expect(reason).toContain('branch is still held')
  })
  it('with no holder and no failed operation it names Retry', () => {
    const w = ws({ status: 'new' })
    expect(placeholderReason(w, placeholderKind(w, 'main'), null)).toContain('Retry')
  })
  it('still prefers the holder path over an operation reason', () => {
    const w = ws({ status: 'new', heldByPath: '/Users/me/other' })
    const reason = placeholderReason(w, placeholderKind(w, 'main'), {
      code: 'internal',
      message: 'branch_already_exists',
    })
    expect(reason).toContain('/Users/me/other')
  })

  it('never says "couldn’t set up" or promises a Retry for the repo’s own checkout', () => {
    const w = ws({ branch: 'main', status: 'locked', heldByPath: '/Users/me/repo-beta' })
    const reason = placeholderReason(w, placeholderKind(w, 'main'), null)
    expect(reason).toContain('/Users/me/repo-beta')
    expect(reason).toContain('detach')
    expect(reason.toLowerCase()).not.toContain("couldn't")
    expect(reason.toLowerCase()).not.toContain('retry')
  })

  it('has nothing to say about a workspace with a worktree', () => {
    const w = ws({ localPath: '/managed' })
    expect(placeholderReason(w, placeholderKind(w, 'main'), null)).toBe('')
  })
})
