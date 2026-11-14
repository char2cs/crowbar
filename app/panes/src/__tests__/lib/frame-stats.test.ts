import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useFrameStats } from '@/lib/frame-stats'

let frames: FrameRequestCallback[] = []
let now = 0

beforeEach(() => {
  frames = []
  now = 0
  vi.spyOn(performance, 'now').mockImplementation(() => now)
  vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => frames.push(cb))
  vi.stubGlobal('cancelAnimationFrame', () => {
    frames = []
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

/** Delivers one frame per `step` ms until `until`. */
function runFrames(step: number, until: number) {
  while (now < until && frames.length > 0) {
    now += step
    const cb = frames.shift()!
    act(() => cb(now))
  }
}

describe('useFrameStats', () => {
  it('starts no frame loop while disabled', () => {
    renderHook(() => useFrameStats(false))
    expect(frames).toHaveLength(0)
  })

  it('reports the rate over a 500ms window, with max frame time and drops', () => {
    const { result } = renderHook(() => useFrameStats(true))
    runFrames(8, 520)
    expect(result.current.fps).toBeGreaterThan(110)
    expect(result.current.maxDt).toBe(8)
    expect(result.current.drops).toBe(0)
  })

  it('counts a frame longer than 16ms as a drop', () => {
    const { result } = renderHook(() => useFrameStats(true))
    runFrames(20, 520)
    expect(result.current.drops).toBeGreaterThan(0)
  })

  it('stops the loop once the last reader leaves', () => {
    const { unmount } = renderHook(() => useFrameStats(true))
    expect(frames).toHaveLength(1)
    unmount()
    expect(frames).toHaveLength(0)
  })

  it('runs one loop for several readers', () => {
    renderHook(() => useFrameStats(true))
    renderHook(() => useFrameStats(true))
    expect(frames).toHaveLength(1)
  })
})
