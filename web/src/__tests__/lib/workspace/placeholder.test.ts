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
  it('is false for a non-locked workspace', () => {
    expect(isPlaceholderWorkspace(ws({ status: 'new' }))).toBe(false)
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
})
