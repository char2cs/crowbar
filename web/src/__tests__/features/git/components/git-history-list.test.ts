import { describe, expect, it } from 'vitest'
import { nearListEnd } from '@/features/git/components/git-history-list'

describe('nearListEnd', () => {
  it('is false near the top of a long list', () => {
    expect(nearListEnd(0, 600, 5000)).toBe(false)
  })

  it('is true once 80% of the list has scrolled past', () => {
    expect(nearListEnd(3400, 600, 5000)).toBe(true)
  })

  it('is true exactly at the bottom', () => {
    expect(nearListEnd(4400, 600, 5000)).toBe(true)
  })

  it('is false for a zero-height list (no crash, no fetch loop)', () => {
    expect(nearListEnd(0, 0, 0)).toBe(false)
  })
})

import { commitDateLabel } from '@/features/git/components/git-history-list'

describe('commitDateLabel', () => {
  it('renders an ISO date as relative time', () => {
    const twoHoursAgo = new Date(Date.now() - 2 * 3600 * 1000).toISOString()
    expect(commitDateLabel(twoHoursAgo)).toBe('2 hours ago')
  })

  it('falls back to the raw string for unparseable input', () => {
    expect(commitDateLabel('not-a-date')).toBe('not-a-date')
  })
})
