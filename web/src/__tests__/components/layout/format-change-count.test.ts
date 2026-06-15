import { describe, it, expect } from 'vitest'
import { formatChangeCount } from '@/components/layout/format-change-count'

describe('formatChangeCount', () => {
  it('returns the number as-is at or below 999', () => {
    expect(formatChangeCount(0)).toBe('0')
    expect(formatChangeCount(69)).toBe('69')
    expect(formatChangeCount(999)).toBe('999')
  })

  it('compacts thousands with a rounded "k" suffix', () => {
    expect(formatChangeCount(1000)).toBe('1k')
    expect(formatChangeCount(1234)).toBe('1k')
    expect(formatChangeCount(1500)).toBe('2k')
    expect(formatChangeCount(12000)).toBe('12k')
  })
})
