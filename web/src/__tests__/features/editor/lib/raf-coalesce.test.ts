import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createRafCoalescer } from '@/features/editor/lib/raf-coalesce'

/**
 * The cursor/selection sync primitive. The load-bearing guarantee: a BURST of
 * `schedule()` calls (one per cursor move / keystroke) collapses into exactly
 * ONE trailing flush per animation frame — so N keystrokes produce ≤1 store
 * write per frame instead of N. The task reads live state at flush time, so the
 * single flush always reflects the final position of the burst.
 */
describe('createRafCoalescer', () => {
  let frame: (() => void) | null

  beforeEach(() => {
    frame = null
    // Deterministic rAF: capture the callback; tests drive it via `runFrame`.
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
      frame = () => cb(0)
      return 1
    })
    vi.stubGlobal('cancelAnimationFrame', () => {
      frame = null
    })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  const runFrame = () => {
    const pending = frame
    frame = null
    pending?.()
  }

  it('collapses a burst of schedule() calls into one flush per frame', () => {
    const task = vi.fn()
    const coalescer = createRafCoalescer(task)

    // Simulate a 10-keystroke burst within a single frame.
    for (let i = 0; i < 10; i++) coalescer.schedule()
    expect(task).not.toHaveBeenCalled()

    runFrame()
    expect(task).toHaveBeenCalledTimes(1)
  })

  it('reads live state at flush time (trailing)', () => {
    let counter = 0
    const seen: number[] = []
    const coalescer = createRafCoalescer(() => seen.push(counter))

    coalescer.schedule()
    counter = 1
    coalescer.schedule()
    counter = 2
    coalescer.schedule()

    runFrame()
    expect(seen).toEqual([2])
  })

  it('schedules a fresh frame after a flush', () => {
    const task = vi.fn()
    const coalescer = createRafCoalescer(task)

    coalescer.schedule()
    runFrame()
    expect(task).toHaveBeenCalledTimes(1)

    coalescer.schedule()
    runFrame()
    expect(task).toHaveBeenCalledTimes(2)
  })

  it('flush() runs a pending task synchronously and clears the frame', () => {
    const task = vi.fn()
    const coalescer = createRafCoalescer(task)

    coalescer.schedule()
    coalescer.flush()
    expect(task).toHaveBeenCalledTimes(1)

    // The frame was cleared, so running it is a no-op.
    runFrame()
    expect(task).toHaveBeenCalledTimes(1)
  })

  it('flush() is a no-op when nothing is pending', () => {
    const task = vi.fn()
    const coalescer = createRafCoalescer(task)
    coalescer.flush()
    expect(task).not.toHaveBeenCalled()
  })

  it('cancel() drops a pending task without running it', () => {
    const task = vi.fn()
    const coalescer = createRafCoalescer(task)

    coalescer.schedule()
    coalescer.cancel()
    runFrame()
    expect(task).not.toHaveBeenCalled()
  })

  it('falls back to setTimeout when requestAnimationFrame is absent', () => {
    vi.unstubAllGlobals()
    vi.useFakeTimers()
    vi.stubGlobal('requestAnimationFrame', undefined)
    vi.stubGlobal('cancelAnimationFrame', undefined)
    try {
      const task = vi.fn()
      const coalescer = createRafCoalescer(task)
      coalescer.schedule()
      coalescer.schedule()
      expect(task).not.toHaveBeenCalled()
      vi.runAllTimers()
      expect(task).toHaveBeenCalledTimes(1)
    } finally {
      vi.useRealTimers()
    }
  })
})
