import { describe, expect, it } from 'vitest'
import { occurrenceKeys } from '@/features/agent/lib/occurrence-keys'

describe('occurrenceKeys', () => {
  it('keeps identical entries distinct and stable when the list grows', () => {
    const before = occurrenceKeys(['a', 'b', 'a'], (item) => item)
    const after = occurrenceKeys(['a', 'b', 'a', 'c'], (item) => item)

    expect(new Set(before).size).toBe(3)
    expect(after.slice(0, 3)).toEqual(before)
  })
})
