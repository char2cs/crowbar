import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useScrollFrameSpan } from '@/features/agent/hooks/use-scroll-frame-span'
import { __resetPerfForTests } from '@/lib/perf/instrumentation'

function measures() {
  return performance.getEntriesByName('chat.scroll.frame', 'measure')
}

describe('useScrollFrameSpan', () => {
  beforeEach(() => {
    __resetPerfForTests()
    vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'setTimeout', 'clearTimeout'] })
    window.__CROWBAR_PERF__ = true
  })

  afterEach(() => {
    vi.useRealTimers()
    delete window.__CROWBAR_PERF__
  })

  it('records one measure per animation frame while scrolling', () => {
    const { result } = renderHook(() => useScrollFrameSpan())

    act(() => result.current.onScrollEvent())
    act(() => {
      vi.advanceTimersByTime(16) // one frame
    })
    act(() => {
      vi.advanceTimersByTime(16) // a second frame
    })

    expect(measures().length).toBeGreaterThanOrEqual(2)
  })

  it('stops sampling once scrolling goes idle', () => {
    const { result } = renderHook(() => useScrollFrameSpan())

    act(() => result.current.onScrollEvent())
    act(() => vi.advanceTimersByTime(16))
    const countAtFirstFrame = measures().length

    act(() => vi.advanceTimersByTime(500)) // well past the idle window
    const countAfterIdle = measures().length

    act(() => vi.advanceTimersByTime(500)) // no more onScrollEvent calls
    expect(measures().length).toBe(countAfterIdle)
    expect(countAfterIdle).toBeGreaterThanOrEqual(countAtFirstFrame)
  })

  it('leaves no open mark when scrolling stops', () => {
    const { result } = renderHook(() => useScrollFrameSpan())
    act(() => result.current.onScrollEvent())
    act(() => vi.advanceTimersByTime(16))
    act(() => vi.advanceTimersByTime(500))

    // A dangling `:start` mark would make the NEXT scroll's first measure
    // include the idle gap. Assert every recorded frame is frame-sized.
    for (const entry of measures()) {
      expect(entry.duration).toBeLessThan(100)
    }
  })
})
