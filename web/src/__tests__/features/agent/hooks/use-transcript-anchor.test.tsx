import { useEffect } from 'react'
import { act, fireEvent, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  useTranscriptAnchor,
  type TranscriptAnchor,
} from '@/features/agent/hooks/use-transcript-anchor'

/**
 * jsdom has no layout engine — `scrollHeight`/`clientHeight` are otherwise
 * always 0, and its ResizeObserver stub (src/__tests__/setup.ts) never calls
 * back. A controllable observer, and mutable test-scoped dimensions read
 * live by the mounted element's getters, stand in for both — same pattern
 * use-preserved-scroll.test.tsx uses for the same reason.
 */
describe('useTranscriptAnchor', () => {
  let scrollHeight = 0
  let clientHeight = 400
  let observerCallbacks: Array<() => void> = []
  const RealResizeObserver = globalThis.ResizeObserver

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'performance'] })
    scrollHeight = 1000
    clientHeight = 400
    observerCallbacks = []
    class ControllableResizeObserver {
      callback: () => void
      constructor(callback: () => void) {
        this.callback = callback
      }
      observe() {
        if (!observerCallbacks.includes(this.callback)) observerCallbacks.push(this.callback)
      }
      unobserve() {}
      disconnect() {
        observerCallbacks = observerCallbacks.filter((c) => c !== this.callback)
      }
    }
    Object.defineProperty(globalThis, 'ResizeObserver', {
      value: ControllableResizeObserver,
      configurable: true,
      writable: true,
    })
  })

  afterEach(() => {
    Object.defineProperty(globalThis, 'ResizeObserver', {
      value: RealResizeObserver,
      configurable: true,
      writable: true,
    })
    vi.useRealTimers()
  })

  /** Fires every live observer, as the browser would once the content resized. */
  const grow = (newHeight: number) => {
    scrollHeight = newHeight
    act(() => {
      for (const cb of [...observerCallbacks]) cb()
    })
  }

  function Host({ onReady }: { onReady?: (anchor: TranscriptAnchor) => void }) {
    const anchor = useTranscriptAnchor()
    useEffect(() => {
      onReady?.(anchor)
    }, [anchor, onReady])
    return (
      <div
        data-testid="scroller"
        ref={(node) => {
          anchor.scrollRef.current = node
          if (!node || Object.hasOwn(node, 'scrollHeight')) return
          let top = 0
          Object.defineProperty(node, 'scrollTop', {
            configurable: true,
            get: () => top,
            // Real browsers clamp scrollTop to [0, scrollHeight - clientHeight].
            // This clamp is what makes the target-miscalculation regression
            // (see use-transcript-anchor.ts) observable at all — an unclamped
            // mock lets scrollTop sail past the real ceiling right along with
            // the bug, so a broken target and a correct one settle at the same
            // place and every assertion here still passes either way.
            set: (v: number) => {
              top = Math.max(0, Math.min(v, Math.max(0, scrollHeight - clientHeight)))
            },
          })
          Object.defineProperty(node, 'scrollHeight', {
            configurable: true,
            get: () => scrollHeight,
          })
          Object.defineProperty(node, 'clientHeight', {
            configurable: true,
            get: () => clientHeight,
          })
        }}
        onScroll={anchor.onScroll}
      >
        <div data-testid="content" />
      </div>
    )
  }

  it('snaps to the bottom instantly on mount — no animation to arrive from', () => {
    const { getByTestId } = render(<Host />)
    // scrollHeight 1000 - clientHeight 400 = 600: the true scrollable ceiling.
    expect(getByTestId('scroller').scrollTop).toBe(600)
  })

  it('eases toward the bottom, rather than jumping, when content grows while stuck', () => {
    const { getByTestId } = render(<Host />)
    const scroller = getByTestId('scroller')

    grow(1400) // ceiling: 1400 - 400 = 1000
    vi.advanceTimersByTime(50) // partway through the follow animation
    expect(scroller.scrollTop).toBeGreaterThan(600)
    expect(scroller.scrollTop).toBeLessThan(1000)

    vi.advanceTimersByTime(1500) // several time constants — fully settled
    expect(scroller.scrollTop).toBe(1000)
  })

  // Regression: a fixed-duration tween, cancelled and restarted on every
  // resize, reset to the steep part of a fresh ease curve each time — and
  // during real streaming, new lines land faster than one tween's duration,
  // so every restart got cut short a few pixels in. That reads as near-
  // instant, choppy motion, not smooth easing, even though each individual
  // tween was technically eased.
  it('a burst of new lines arriving faster than one glide would take still ends up smooth, not stuck at the top', () => {
    const { getByTestId } = render(<Host />)
    const scroller = getByTestId('scroller')

    // One "new line" landing every 20ms — faster than this hook's own follow
    // loop settles — for a full second, as a fast turn streaming in would.
    let height = 1000
    for (let i = 0; i < 50; i++) {
      height += 20
      grow(height)
      vi.advanceTimersByTime(20)
    }

    // It has been making real, continuous progress throughout — not pinned
    // near its starting point by constant restarts.
    expect(scroller.scrollTop).toBeGreaterThan(1000)

    vi.advanceTimersByTime(1500) // the burst has stopped — let it fully catch up
    expect(scroller.scrollTop).toBe(height - 400) // ceiling: height - clientHeight
  })

  it('a real scroll mid-animation cancels it and stops following, immediately', () => {
    const { getByTestId } = render(<Host />)
    const scroller = getByTestId('scroller')

    grow(1400) // ceiling: 1400 - 400 = 1000
    vi.advanceTimersByTime(50)
    expect(scroller.scrollTop).toBeLessThan(1000)

    // The reader scrolls up, mid-animation — theirs wins at once.
    scroller.scrollTop = 200
    fireEvent.scroll(scroller)

    vi.advanceTimersByTime(1500) // the animation would otherwise have finished by now
    expect(scroller.scrollTop).toBe(200)

    // And following stays off — new growth must not yank them back down.
    grow(2000)
    vi.advanceTimersByTime(1500) // several time constants — fully settled
    expect(scroller.scrollTop).toBe(200)
  })

  it('does not mistake its own animation writes for the reader scrolling away', () => {
    // The bug this guards: both a real gesture and this hook's own follow
    // animation change the same `scrollTop`, and every write — ours included
    // — fires a scroll event. Without telling them apart, the hook would
    // read its own progress mid-animation as "the reader scrolled up" (still
    // short of the bottom) and cancel itself.
    const { getByTestId } = render(<Host />)
    const scroller = getByTestId('scroller')

    grow(1400) // ceiling: 1400 - 400 = 1000
    vi.advanceTimersByTime(50)
    fireEvent.scroll(scroller) // echoes this hook's own write, not a real gesture

    vi.advanceTimersByTime(1500) // several time constants — fully settled
    expect(scroller.scrollTop).toBe(1000)
  })

  it('resumes following once the reader scrolls back to the bottom', () => {
    const { getByTestId } = render(<Host />)
    const scroller = getByTestId('scroller')

    grow(1400)
    vi.advanceTimersByTime(50)
    scroller.scrollTop = 200
    fireEvent.scroll(scroller)
    vi.advanceTimersByTime(1500) // several time constants — fully settled

    // Back at the bottom (scrollHeight 1400 - clientHeight 400 = 1000).
    scroller.scrollTop = 1000
    fireEvent.scroll(scroller)

    grow(1800) // ceiling: 1800 - 400 = 1400
    vi.advanceTimersByTime(50)
    expect(scroller.scrollTop).toBeGreaterThan(1000)
    vi.advanceTimersByTime(1500) // several time constants — fully settled
    expect(scroller.scrollTop).toBe(1400)
  })

  it('restoring position for a prepend is an instant jump, not an eased one', () => {
    let anchor: TranscriptAnchor | undefined
    const { getByTestId } = render(<Host onReady={(a) => (anchor = a)} />)
    const scroller = getByTestId('scroller')
    // ceiling here is 1000 - 400 = 600, so 500 is 100px short of the bottom.
    scroller.scrollTop = 500

    act(() => anchor?.preservePosition())
    grow(2000) // older messages landed above the fold

    // Same distance from the bottom preserved, on the very first tick — no
    // partial state to observe, unlike the eased follow path above.
    expect(scroller.scrollTop).toBe(1500)
  })

  // Regression: the follow target was `el.scrollHeight` — the whole content
  // height, not the scrollable CEILING (`scrollHeight - clientHeight`). Both
  // eventually clamp to the same final position, so a large-growth test
  // (like the one above) can't tell buggy from fixed apart — only the FIRST
  // frame does, and only when one chunk's growth is small relative to
  // clientHeight, which is the realistic streaming case (a paragraph is
  // tens of px; the viewport is often 1000+). With the bug, the error is
  // exactly `clientHeight` — an order of magnitude bigger than a real
  // chunk's growth — so the very first frame's step already overshoots the
  // true ceiling and the native clamp snaps it there: zero perceptible
  // easing, which is exactly what this was reported as live.
  it('a small chunk on a tall viewport still eases over several frames, not one', () => {
    clientHeight = 1000
    scrollHeight = 2000 // mount ceiling: 2000 - 1000 = 1000
    const { getByTestId } = render(<Host />)
    const scroller = getByTestId('scroller')
    expect(scroller.scrollTop).toBe(1000)

    grow(2050) // one small paragraph landing — new ceiling: 2050 - 1000 = 1050
    vi.advanceTimersByTime(16) // one frame
    // The buggy version targeted 2050 directly: distance 1050, ~15% closed
    // in one frame overshoots the true 50px gap by more than 20x, so the
    // native clamp puts it at 1050 — the fully-settled value — after a
    // SINGLE frame. Genuine easing must still be short of it here.
    expect(scroller.scrollTop).toBeLessThan(1050)
    expect(scroller.scrollTop).toBeGreaterThan(1000)

    vi.advanceTimersByTime(1500) // several time constants — fully settled
    expect(scroller.scrollTop).toBe(1050)
  })
})
