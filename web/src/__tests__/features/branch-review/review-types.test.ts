import { describe, it, expect } from 'vitest'
import type { ReviewThread, ReviewMessage, MergeStrategy } from '@/features/branch-review/types/review-types'
import { getMockBranchDiff } from '@/lib/mock/branch-diff'

describe('review-types', () => {
  it('ReviewMessage allows null author for the current user', () => {
    const msg: ReviewMessage = {
      id: '1', author: null, isAgent: false, body: 'looks good', createdAt: '2026-06-01',
    }
    expect(msg.author).toBeNull()
  })

  it('ReviewMessage stores agent name as author', () => {
    const msg: ReviewMessage = {
      id: '2', author: 'Claude', isAgent: true, body: 'consider caching', createdAt: '2026-06-01',
    }
    expect(msg.author).toBe('Claude')
    expect(msg.isAgent).toBe(true)
  })

  it('MergeStrategy covers all three values', () => {
    const strategies: MergeStrategy[] = ['merge', 'squash', 'rebase']
    expect(strategies).toHaveLength(3)
  })

  it('getMockBranchDiff returns a valid MultiFileDiff', () => {
    const diff = getMockBranchDiff('ws1')
    expect(diff.files.length).toBeGreaterThan(0)
    expect(diff.totalFiles).toBe(diff.files.length)
  })
})
