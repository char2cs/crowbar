import { describe, it, expect, vi } from 'vitest'
import { pollUntilResizeSettles } from '@/features/terminal/lib/refit'

/**
 * Deterministic fake requestAnimationFrame: callbacks queue by id and only run
 * when the test flushes. A frame scheduled DURING a flush runs on the NEXT
 * flush, mirroring real rAF — no real timers, no polling in the test.
 */
function makeFakeRaf() {
  let nextId = 1
  const scheduled = new Map<number, () => void>()
  return {
    requestFrame: (cb: () => void) => {
      const id = nextId++
      scheduled.set(id, cb)
      return id
    },
    cancelFrame: (id: number) => {
      scheduled.delete(id)
    },
    flush() {
      const batch = [...scheduled.values()]
      scheduled.clear()
      for (const cb of batch) cb()
    },
    pending: () => scheduled.size,
  }
}

describe('pollUntilResizeSettles', () => {
  it('defers while resizing, then fires onSettled exactly once when the drag ends', () => {
    const raf = makeFakeRaf()
    let resizing = true
    const onSettled = vi.fn()

    pollUntilResizeSettles({
      isResizing: () => resizing,
      onSettled,
      requestFrame: raf.requestFrame,
      cancelFrame: raf.cancelFrame,
    })

    // Frames tick while the drag is live: keep polling, never settle.
    raf.flush()
    raf.flush()
    expect(onSettled).not.toHaveBeenCalled()
    expect(raf.pending()).toBe(1) // still polling

    // Drag ends → the next frame settles once and stops polling.
    resizing = false
    raf.flush()
    expect(onSettled).toHaveBeenCalledTimes(1)
    expect(raf.pending()).toBe(0)

    // No further frames scheduled; extra flushes are inert.
    raf.flush()
    expect(onSettled).toHaveBeenCalledTimes(1)
  })

  it('settles on the first frame when the drag has already ended', () => {
    const raf = makeFakeRaf()
    const onSettled = vi.fn()

    pollUntilResizeSettles({
      isResizing: () => false,
      onSettled,
      requestFrame: raf.requestFrame,
      cancelFrame: raf.cancelFrame,
    })

    raf.flush()
    expect(onSettled).toHaveBeenCalledTimes(1)
  })

  it('cancel stops the poll and guarantees onSettled never fires', () => {
    // Models effect cleanup / an earlier pane-resize-end arriving mid-drag: even
    // after the drag later ends, the cancelled poll must not fit a torn-down or
    // superseded terminal.
    const raf = makeFakeRaf()
    let resizing = true
    const onSettled = vi.fn()

    const cancel = pollUntilResizeSettles({
      isResizing: () => resizing,
      onSettled,
      requestFrame: raf.requestFrame,
      cancelFrame: raf.cancelFrame,
    })

    raf.flush() // still resizing → rescheduled
    expect(raf.pending()).toBe(1)

    cancel()
    expect(raf.pending()).toBe(0) // pending frame cancelled

    resizing = false
    raf.flush()
    expect(onSettled).not.toHaveBeenCalled()
  })
})
