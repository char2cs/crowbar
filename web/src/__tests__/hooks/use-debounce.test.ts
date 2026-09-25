import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useDebounce } from '@/hooks/use-debounce'

beforeEach(() => vi.useFakeTimers())
afterEach(() => vi.useRealTimers())

describe('useDebounce', () => {
  it('holds the value back until it has been stable for the delay', () => {
    const { result, rerender } = renderHook(({ v }) => useDebounce(v, 100), {
      initialProps: { v: 'a' },
    })
    expect(result.current[0]).toBe('a')

    rerender({ v: 'ab' })
    act(() => vi.advanceTimersByTime(60))
    rerender({ v: 'abc' })
    act(() => vi.advanceTimersByTime(60))
    expect(result.current[0]).toBe('a')

    act(() => vi.advanceTimersByTime(40))
    expect(result.current[0]).toBe('abc')
  })
})
