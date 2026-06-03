import { describe, it, expect, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { create } from 'zustand'
import { useRetry } from '@/lib/store/use-retry'

describe('useRetry', () => {
  it('returns a callback that calls fetch with the given args', () => {
    const fetchSpy = vi.fn()
    const useStore = create<{ fetch: (k: string) => void }>()(() => ({ fetch: fetchSpy }))
    const { result } = renderHook(() => useRetry(useStore, 'repo-1'))
    result.current()
    expect(fetchSpy).toHaveBeenCalledWith('repo-1')
  })

  it('works with no args', () => {
    const fetchSpy = vi.fn()
    const useStore = create<{ fetch: () => void }>()(() => ({ fetch: fetchSpy }))
    const { result } = renderHook(() => useRetry(useStore))
    result.current()
    expect(fetchSpy).toHaveBeenCalledOnce()
  })
})
