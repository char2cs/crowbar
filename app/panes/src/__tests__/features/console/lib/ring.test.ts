import { describe, expect, it } from 'vitest'
import { appendCapped } from '@/features/console/lib/ring'

describe('appendCapped', () => {
  it('appends below the cap', () => {
    expect(appendCapped([1, 2], [3], 5)).toEqual([1, 2, 3])
  })

  it('drops the oldest items past the cap', () => {
    expect(appendCapped([1, 2, 3], [4, 5], 4)).toEqual([2, 3, 4, 5])
    expect(appendCapped([], [1, 2, 3, 4], 2)).toEqual([3, 4])
  })

  it('does not mutate its inputs', () => {
    const list = [1, 2]
    const result = appendCapped(list, [], 5)
    expect(result).not.toBe(list)
    expect(list).toEqual([1, 2])
  })
})
