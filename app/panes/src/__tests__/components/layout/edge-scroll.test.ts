import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createEdgeScroller, EDGE_SCROLL_BAND_PX } from '@/components/layout/edge-scroll'

describe('createEdgeScroller', () => {
  let frames: FrameRequestCallback[] = []
  beforeEach(() => {
    frames = []
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => frames.push(cb))
    vi.stubGlobal('cancelAnimationFrame', () => {})
  })
  afterEach(() => vi.unstubAllGlobals())

  const flush = () => frames.splice(0).forEach((cb) => cb(0))

  it('scrolls a horizontal scroller from the pointer x, and reports each moving frame', () => {
    const el = document.createElement('div')
    // jsdom does not clamp or lay out, so scrollLeft is a plain number here.
    const onScrolled = vi.fn()
    const scroller = createEdgeScroller(el, { left: 100, width: 400 }, { onScrolled })

    scroller.update(100 + EDGE_SCROLL_BAND_PX + 20)
    expect(frames).toHaveLength(0)

    scroller.update(499)
    flush()
    expect(el.scrollLeft).toBeGreaterThan(0)
    expect(el.scrollTop).toBe(0)
    expect(onScrolled).toHaveBeenCalledTimes(1)

    scroller.update(101)
    flush()
    expect(onScrolled).toHaveBeenCalledTimes(2)
    scroller.stop()
  })

  it('still scrolls a vertical scroller from the pointer y', () => {
    const el = document.createElement('div')
    const scroller = createEdgeScroller(el, { top: 0, height: 300 }, { onScrolled: vi.fn() })
    scroller.update(299)
    flush()
    expect(el.scrollTop).toBeGreaterThan(0)
    expect(el.scrollLeft).toBe(0)
    scroller.stop()
  })
})
