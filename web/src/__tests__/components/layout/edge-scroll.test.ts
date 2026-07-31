/**
 * Edge scroll: the band, the ramp, and the loop that drives it.
 *
 * The loop is the part worth pinning. It has to be a frame clock — a drag from
 * the last project to the first is impossible without one — and it has to stop
 * when the pointer leaves the band, or a finished drag leaves a rAF running for
 * the rest of the session.
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import {
  EDGE_SCROLL_BAND_PX,
  EDGE_SCROLL_MAX_STEP_PX,
  createEdgeScroller,
  edgeScrollStep,
  findScrollParent,
} from '@/components/layout/edge-scroll'

const TOP = 100
const HEIGHT = 400
const BOTTOM = TOP + HEIGHT

describe('the band', () => {
  it('is still in the middle of the scroller', () => {
    expect(edgeScrollStep(TOP + HEIGHT / 2, TOP, HEIGHT)).toBe(0)
  })

  it('scrolls up near the top and down near the bottom', () => {
    expect(edgeScrollStep(TOP + 1, TOP, HEIGHT)).toBeLessThan(0)
    expect(edgeScrollStep(BOTTOM - 1, TOP, HEIGHT)).toBeGreaterThan(0)
  })

  it('starts exactly 36px in, and not a pixel earlier', () => {
    expect(edgeScrollStep(TOP + EDGE_SCROLL_BAND_PX, TOP, HEIGHT)).toBe(0)
    expect(edgeScrollStep(TOP + EDGE_SCROLL_BAND_PX - 1, TOP, HEIGHT)).not.toBe(0)
  })

  it('ramps: a crawl at the inner edge of the band, full speed at the boundary', () => {
    const inner = Math.abs(edgeScrollStep(TOP + EDGE_SCROLL_BAND_PX - 2, TOP, HEIGHT))
    const edge = Math.abs(edgeScrollStep(TOP, TOP, HEIGHT))

    expect(inner).toBeLessThan(edge)
    expect(edge).toBe(EDGE_SCROLL_MAX_STEP_PX)
  })

  it('does not accelerate past the boundary', () => {
    expect(edgeScrollStep(TOP - 500, TOP, HEIGHT)).toBe(-EDGE_SCROLL_MAX_STEP_PX)
  })

  it('is a no-op for a scroller with no height', () => {
    expect(edgeScrollStep(0, 0, 0)).toBe(0)
  })
})

describe('the loop', () => {
  let frames: Array<() => void>

  beforeEach(() => {
    frames = []
    vi.stubGlobal('requestAnimationFrame', (cb: () => void) => {
      frames.push(cb)
      return frames.length
    })
    vi.stubGlobal('cancelAnimationFrame', (id: number) => {
      frames[id - 1] = () => {}
    })
  })

  afterEach(() => vi.unstubAllGlobals())

  const scroller = () => {
    const el = document.createElement('div')
    let scrollTop = 0
    Object.defineProperty(el, 'scrollTop', {
      get: () => scrollTop,
      set: (v: number) => {
        scrollTop = Math.max(0, v)
      },
    })
    return el
  }

  /** Run every frame queued so far, once. */
  const flush = () => {
    const queued = frames
    frames = []
    for (const cb of queued) cb()
  }

  it('does not start a frame while the pointer is away from the edges', () => {
    const el = scroller()
    createEdgeScroller(el, { top: TOP, height: HEIGHT }, { onScrolled: () => {} }).update(
      TOP + HEIGHT / 2,
    )

    expect(frames).toHaveLength(0)
  })

  it('scrolls once per frame while the pointer sits in the band', () => {
    const el = scroller()
    el.scrollTop = 100
    const s = createEdgeScroller(el, { top: TOP, height: HEIGHT }, { onScrolled: () => {} })

    s.update(BOTTOM - 1)
    flush()
    const afterOne = el.scrollTop
    expect(afterOne).toBeGreaterThan(100)

    flush()
    expect(el.scrollTop).toBeGreaterThan(afterOne)
    s.stop()
  })

  // The pointer can be perfectly still while the list slides past it, so the
  // row underneath changes with no pointermove to notice it.
  it('re-runs the hit test after a frame that actually moved', () => {
    const el = scroller()
    el.scrollTop = 100
    const onScrolled = vi.fn()
    const s = createEdgeScroller(el, { top: TOP, height: HEIGHT }, { onScrolled })

    s.update(BOTTOM - 1)
    flush()

    expect(onScrolled).toHaveBeenCalledTimes(1)
    s.stop()
  })

  it('does not re-run it when the scroller was already at its end', () => {
    const el = scroller()
    const onScrolled = vi.fn()
    const s = createEdgeScroller(el, { top: TOP, height: HEIGHT }, { onScrolled })

    s.update(TOP) // scrolling up, already at 0
    flush()

    expect(onScrolled).not.toHaveBeenCalled()
    s.stop()
  })

  it('stops when the pointer leaves the band', () => {
    const el = scroller()
    el.scrollTop = 100
    const s = createEdgeScroller(el, { top: TOP, height: HEIGHT }, { onScrolled: () => {} })

    s.update(BOTTOM - 1)
    flush()
    const parked = el.scrollTop
    s.update(TOP + HEIGHT / 2)
    flush()
    flush()

    expect(el.scrollTop).toBe(parked)
  })

  // The drop line animates between slots, and an edge scroll invalidates its
  // position every frame — so it has to know when one is running and stop
  // easing, or it trails the rows it is meant to be marking.
  it('says when the loop is running and when it is not', () => {
    const el = scroller()
    el.scrollTop = 100
    const onRunningChange = vi.fn()
    const s = createEdgeScroller(
      el,
      { top: TOP, height: HEIGHT },
      { onScrolled: () => {}, onRunningChange },
    )

    s.update(BOTTOM - 1)
    expect(onRunningChange).toHaveBeenLastCalledWith(true)

    // Staying in the band re-aims but does not re-announce the start.
    s.update(BOTTOM - 2)
    expect(onRunningChange).toHaveBeenCalledTimes(1)

    s.update(TOP + HEIGHT / 2)
    flush()
    expect(onRunningChange).toHaveBeenLastCalledWith(false)
  })

  it('says so on an explicit stop too, exactly once', () => {
    const el = scroller()
    el.scrollTop = 100
    const onRunningChange = vi.fn()
    const s = createEdgeScroller(
      el,
      { top: TOP, height: HEIGHT },
      { onScrolled: () => {}, onRunningChange },
    )

    s.update(BOTTOM - 1)
    s.stop()
    s.stop()

    expect(onRunningChange.mock.calls).toEqual([[true], [false]])
  })

  it('stops on demand, and leaves no frame behind', () => {
    const el = scroller()
    el.scrollTop = 100
    const s = createEdgeScroller(el, { top: TOP, height: HEIGHT }, { onScrolled: () => {} })

    s.update(BOTTOM - 1)
    s.stop()
    flush()
    flush()

    expect(el.scrollTop).toBe(100)
  })
})

describe('finding the scroller', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('walks up to the nearest ancestor that actually scrolls', () => {
    const outer = document.createElement('div')
    outer.style.overflowY = 'auto'
    Object.defineProperty(outer, 'scrollHeight', { value: 1000 })
    Object.defineProperty(outer, 'clientHeight', { value: 200 })
    const row = document.createElement('div')
    outer.appendChild(row)
    document.body.appendChild(outer)

    expect(findScrollParent(row)).toBe(outer)
  })

  it('skips an ancestor whose content already fits', () => {
    const outer = document.createElement('div')
    outer.style.overflowY = 'auto'
    Object.defineProperty(outer, 'scrollHeight', { value: 100 })
    Object.defineProperty(outer, 'clientHeight', { value: 200 })
    const row = document.createElement('div')
    outer.appendChild(row)
    document.body.appendChild(outer)

    expect(findScrollParent(row)).toBeNull()
  })

  it('is null when there is nothing to walk', () => {
    expect(findScrollParent(null)).toBeNull()
  })
})
