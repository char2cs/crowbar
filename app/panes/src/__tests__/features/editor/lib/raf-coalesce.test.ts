import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createRafCoalescer } from '@/features/editor/lib/raf-coalesce'

describe('createRafCoalescer', () => {
  let frame: (() => void) | null

  beforeEach(() => {
    frame = null
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
    vi.restoreAllMocks()
  })

  const runFrame = () => {
    const pending = frame
    frame = null
    pending?.()
  }

  it('collapses a burst of schedule() calls into one flush per frame', () => {
    const task = vi.fn()
    const coalescer = createRafCoalescer(task)

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

  describe('minIntervalMs throttle', () => {
    let clock: number
    let timers: Array<{ id: number; at: number; fn: () => void } | null>

    const armClock = () => {
      clock = 0
      timers = []
      vi.spyOn(performance, 'now').mockImplementation(() => clock)
      vi.stubGlobal('setTimeout', (fn: () => void, delay = 0) => {
        const id = timers.length + 1
        timers.push({ id, at: clock + delay, fn })
        return id
      })
      vi.stubGlobal('clearTimeout', (id: number) => {
        const index = timers.findIndex((t) => t?.id === id)
        if (index >= 0) timers[index] = null
      })
    }

    const advance = (ms: number) => {
      clock += ms
      for (let i = 0; i < timers.length; i++) {
        const timer = timers[i]
        if (timer && timer.at <= clock) {
          timers[i] = null
          timer.fn()
        }
      }
    }

    it('holds the flush back to one per interval while the gesture is in flight', () => {
      armClock()
      let dragging = true
      const task = vi.fn()
      const coalescer = createRafCoalescer(task, { minIntervalMs: () => (dragging ? 100 : 0) })

      coalescer.schedule()
      runFrame()
      expect(task).toHaveBeenCalledTimes(1)

      for (let i = 0; i < 9; i++) {
        coalescer.schedule()
        advance(10)
      }
      expect(task).toHaveBeenCalledTimes(1)

      advance(20)
      expect(task).toHaveBeenCalledTimes(2)

      dragging = false
      coalescer.schedule()
      runFrame()
      expect(task).toHaveBeenCalledTimes(3)
    })

    it('flush() lands the held task immediately (gesture end never leaves it in a timer)', () => {
      armClock()
      const task = vi.fn()
      const coalescer = createRafCoalescer(task, { minIntervalMs: () => 100 })

      coalescer.schedule()
      runFrame()
      expect(task).toHaveBeenCalledTimes(1)

      advance(10)
      coalescer.schedule()
      expect(task).toHaveBeenCalledTimes(1)

      coalescer.flush()
      expect(task).toHaveBeenCalledTimes(2)

      advance(500)
      expect(task).toHaveBeenCalledTimes(2)
    })
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
