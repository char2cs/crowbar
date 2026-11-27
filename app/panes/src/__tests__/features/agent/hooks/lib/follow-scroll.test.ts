import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createFollowScroll } from '@/features/agent/hooks/lib/follow-scroll'

describe('createFollowScroll', () => {
  let el: HTMLDivElement

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'performance'] })
    el = document.createElement('div')
    document.body.appendChild(el)
  })

  afterEach(() => {
    el.remove()
    vi.useRealTimers()
  })

  it('converges scrollTop toward the target and settles exactly on it', () => {
    el.scrollTop = 0
    createFollowScroll(el).setTarget(1000)

    vi.advanceTimersByTime(50)
    expect(el.scrollTop).toBeGreaterThan(0)
    expect(el.scrollTop).toBeLessThan(1000)

    vi.advanceTimersByTime(1000)
    expect(el.scrollTop).toBe(1000)
  })

  it('does nothing before setTarget is called', () => {
    el.scrollTop = 42
    createFollowScroll(el)
    vi.advanceTimersByTime(500)
    expect(el.scrollTop).toBe(42)
  })

  it('retargeting mid-flight (faster than settling) continues smoothly, with no step back or restart', () => {
    const follow = createFollowScroll(el)
    el.scrollTop = 0
    follow.setTarget(200)

    vi.advanceTimersByTime(20)
    const beforeRetarget = el.scrollTop
    expect(beforeRetarget).toBeGreaterThan(0)

    follow.setTarget(220)
    vi.advanceTimersByTime(20)

    expect(el.scrollTop).toBeGreaterThan(beforeRetarget)
  })

  it('a burst of many retargets in quick succession still ends up exactly at the final target', () => {
    const follow = createFollowScroll(el)
    el.scrollTop = 0
    for (let i = 1; i <= 20; i++) {
      follow.setTarget(i * 20)
      vi.advanceTimersByTime(15)
    }
    vi.advanceTimersByTime(1000)
    expect(el.scrollTop).toBe(400)
  })

  it('calls onTick with the value it writes each frame', () => {
    const onTick = vi.fn()
    createFollowScroll(el, onTick).setTarget(500)

    vi.advanceTimersByTime(1000)

    expect(onTick).toHaveBeenCalled()
    expect(onTick.mock.calls.at(-1)?.[0]).toBe(500)
  })

  it('pauses when something else changes scrollTop between frames, and re-arms cleanly on the next setTarget', () => {
    const follow = createFollowScroll(el)
    el.scrollTop = 0
    follow.setTarget(1000)

    vi.advanceTimersByTime(20)
    const interrupted = el.scrollTop
    el.scrollTop = 5

    vi.advanceTimersByTime(500)
    expect(el.scrollTop).toBe(5)
    expect(el.scrollTop).not.toBe(interrupted)

    follow.setTarget(50)
    vi.advanceTimersByTime(1000)
    expect(el.scrollTop).toBe(50)
  })

  it('stop() halts the loop permanently — a later setTarget does nothing', () => {
    const follow = createFollowScroll(el)
    el.scrollTop = 0
    follow.setTarget(1000)
    vi.advanceTimersByTime(20)

    follow.stop()
    const stoppedAt = el.scrollTop
    vi.advanceTimersByTime(500)
    expect(el.scrollTop).toBe(stoppedAt)

    follow.setTarget(1000)
    vi.advanceTimersByTime(500)
    expect(el.scrollTop).toBe(stoppedAt)
  })
})

describe('createFollowScroll under dropped frames', () => {
  let el: HTMLDivElement

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'performance'] })
    el = document.createElement('div')
    document.body.appendChild(el)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    el.remove()
    vi.useRealTimers()
  })

  function manualRaf() {
    let pending: ((t: number) => void) | null = null
    vi.stubGlobal('requestAnimationFrame', (cb: (t: number) => void) => {
      pending = cb
      return 1
    })
    vi.stubGlobal('cancelAnimationFrame', () => {
      pending = null
    })
    return () => {
      const cb = pending
      pending = null
      cb?.(performance.now())
    }
  }

  it('does not teleport when a jank burst delays a frame', () => {
    const frame = manualRaf()
    el.scrollTop = 0
    const follow = createFollowScroll(el)
    follow.setTarget(1000)

    vi.advanceTimersByTime(16)
    frame()
    const afterNormalFrame = el.scrollTop
    expect(afterNormalFrame).toBeGreaterThan(0)

    const remaining = 1000 - afterNormalFrame
    vi.advanceTimersByTime(250)
    frame()
    const jump = el.scrollTop - afterNormalFrame

    expect(jump).toBeLessThan(remaining * 0.4)
    expect(jump).toBeGreaterThan(0)
  })

  it('still converges on the target after a burst, just over more frames', () => {
    const frame = manualRaf()
    el.scrollTop = 0
    const follow = createFollowScroll(el)
    follow.setTarget(1000)
    for (let i = 0; i < 60; i++) {
      vi.advanceTimersByTime(120)
      frame()
    }
    expect(el.scrollTop).toBe(1000)
  })
})

describe('createFollowScroll against a clamping container', () => {
  let el: HTMLDivElement

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'performance'] })
    el = document.createElement('div')
    document.body.appendChild(el)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    el.remove()
    vi.useRealTimers()
  })

  function clamped(max: () => number) {
    let top = 0
    Object.defineProperty(el, 'scrollTop', {
      configurable: true,
      get: () => top,
      set: (v: number) => {
        top = Math.max(0, Math.min(v, max()))
      },
    })
  }

  it('reports the position the container actually took, not the one it was asked for', () => {
    clamped(() => 300)
    const seen: number[] = []
    createFollowScroll(el, (v) => seen.push(v)).setTarget(1000)

    vi.advanceTimersByTime(500)

    expect(seen.length).toBeGreaterThan(0)
    expect(seen.at(-1)).toBe(el.scrollTop)
  })

  it('keeps following after the content shrinks under it', () => {
    let ceiling = 1000
    clamped(() => ceiling)
    const seen: number[] = []
    const follow = createFollowScroll(el, (v) => seen.push(v))
    follow.setTarget(1000)
    vi.advanceTimersByTime(120)

    ceiling = 200
    vi.advanceTimersByTime(500)

    expect(el.scrollTop).toBe(200)
    expect(seen.at(-1)).toBe(el.scrollTop)
  })
})

describe('createFollowScroll on an engine that keeps whole pixels', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'performance'] })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('reaches the target instead of stalling a few pixels short', () => {
    const el = document.createElement('div')
    let top = 0
    Object.defineProperty(el, 'scrollTop', {
      get: () => top,
      set: (v: number) => {
        top = Math.round(v)
      },
    })
    createFollowScroll(el).setTarget(300)

    vi.advanceTimersByTime(2000)
    expect(el.scrollTop).toBe(300)
  })
})
