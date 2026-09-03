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

  // Regression: a real browser stops INVOKING requestAnimationFrame callbacks
  // while the OS window is unfocused — confirmed live via Tauri MCP: a
  // chained rAF promise never resolved for as long as the app's window sat
  // unfocused, even though `document.visibilityState` stayed "visible" the
  // whole time (so `visibilitychange` never fires, and can't be the signal
  // this hook listens for). But the CALL to requestAnimationFrame itself is
  // never refused — the spec (and real engines) hand back a genuine, unique,
  // non-zero request id synchronously regardless of visibility; only the
  // callback's invocation is deferred, indefinitely if focus never returns.
  // vitest's own fake rAF has no such notion at all (a scheduled callback
  // just sits pending and fires on the next `advanceTimersByTime`), so both
  // tests below model it with a stub instead: a monotonic id is always
  // issued, and only registration of its callback is gated on `frozen`.
  const stubFrozenRaf = (startFrozen: boolean) => {
    let frozen = startFrozen
    let nextId = 1
    const pending = new Map<number, FrameRequestCallback>()
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
      const id = nextId++
      if (!frozen) pending.set(id, cb)
      return id
    })
    vi.stubGlobal('cancelAnimationFrame', (id: number) => {
      pending.delete(id)
    })
    return {
      freeze: () => {
        frozen = true
      },
      unfreeze: () => {
        frozen = false
      },
      pending,
    }
  }

  it('catches up to the true bottom on window refocus, after growth that landed while requestAnimationFrame was frozen', () => {
    const raf = stubFrozenRaf(true)

    const { getByTestId } = render(<Host />)
    const scroller = getByTestId('scroller')
    expect(scroller.scrollTop).toBe(600) // mount ceiling: 1000 - 400

    // A reply keeps streaming while the window sits unfocused — the resize
    // observer still fires (it is not rAF-gated), but the glide it kicks off
    // never actually gets a frame to run.
    grow(1400) // new ceiling: 1400 - 400 = 1000
    expect(scroller.scrollTop).toBe(600) // unmoved — no frame has run

    // The window regains focus — rAF can schedule again.
    raf.unfreeze()
    fireEvent(window, new Event('focus'))

    expect(raf.pending.size).toBe(1)
    act(() => [...raf.pending.values()][0](performance.now() + 50))
    expect(scroller.scrollTop).toBeGreaterThan(600)
  })

  // Regression: the fix above alone still misses the far more common shape
  // of this bug. `createFollowScroll`'s `setTarget` only schedules a FRESH
  // rAF request when its own internal `raf === 0` — i.e. only when nothing
  // is already (supposedly) pending. If a glide was already IN FLIGHT the
  // instant focus was lost — the ordinary case, since a reply is usually
  // mid-stream, not idle, when someone alt-tabs away — that pending
  // request's id is still sitting in `raf`, non-zero, forever: real engines
  // never invoke it (confirmed live), so it will never come back on its
  // own. A window-focus handler that just calls the SAME `setTarget` the
  // ResizeObserver already uses inherits that guard and silently no-ops —
  // `target` is updated but nothing ever reads it again. This is "the
  // scroll bug came back" as reported live: the first fix only covered
  // focus lost while already caught up, not focus lost mid-glide.
  it('recovers even when a glide was already in flight the instant focus was lost — a stale pending request id must not block a fresh one', () => {
    const raf = stubFrozenRaf(false) // starts focused: an ordinary glide can actually schedule

    const { getByTestId } = render(<Host />)
    const scroller = getByTestId('scroller')

    // A normal glide starts while still focused — a real request is now
    // pending, exactly as it would be mid-stream.
    grow(1400) // new ceiling: 1400 - 400 = 1000
    expect(raf.pending.size).toBe(1)
    const idsBeforeFreeze = new Set(raf.pending.keys())

    // Focus is lost mid-glide: the request above will now NEVER be invoked
    // (confirmed live) — real engines do not cancel it, they just stop
    // calling it. More growth arrives while frozen; nothing can move.
    raf.freeze()
    grow(1800) // new ceiling: 1800 - 400 = 1400 — still nothing moves
    expect(scroller.scrollTop).toBe(600) // unmoved the whole time

    raf.unfreeze()
    fireEvent(window, new Event('focus'))

    // A FRESH request must exist — not merely the one from before the
    // freeze, which this environment has already proven will never fire.
    const freshIds = [...raf.pending.keys()].filter((id) => !idsBeforeFreeze.has(id))
    expect(freshIds.length).toBeGreaterThan(0)
    act(() => raf.pending.get(freshIds[0])?.(performance.now() + 50))
    expect(scroller.scrollTop).toBeGreaterThan(600)
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

  // Regression, reported live: "the turn has closed, but I can't reach the
  // bottom — until more text pushes past it." The docked composer OVERLAYS
  // the transcript — `.scroll`'s `padding-bottom` reserves room for it —
  // rather than taking dedicated flex space, so the composer growing (a
  // halted-turn banner appearing on one particular close path, say) changes
  // `scrollHeight` alone: neither `.scroll`'s own box nor its content's
  // moved, so the ResizeObserver this hook already runs never fires, and
  // the follow target is left stale short of the new true bottom. The
  // caller has no way to reach in and re-trigger the SAME resync a real
  // resize would have caused — until `notifyReflow`.
  it('notifyReflow catches up to a scrollHeight change neither internal ResizeObserver could have seen', () => {
    let anchor: TranscriptAnchor | undefined
    const { getByTestId } = render(<Host onReady={(a) => (anchor = a)} />)
    const scroller = getByTestId('scroller')
    expect(scroller.scrollTop).toBe(600) // mount ceiling: 1000 - 400

    // The dock grows: scrollHeight moves because the padding it drives grew,
    // not because anything this hook observes was resized.
    scrollHeight = 1400 // new ceiling: 1400 - 400 = 1000
    expect(scroller.scrollTop).toBe(600) // unmoved — nothing told it to look

    act(() => anchor?.notifyReflow())
    vi.advanceTimersByTime(1500) // several time constants — fully settled

    expect(scroller.scrollTop).toBe(1000)
  })

  it('notifyReflow is a no-op once the reader has scrolled away — it must not fight a real gesture back to the bottom', () => {
    let anchor: TranscriptAnchor | undefined
    const { getByTestId } = render(<Host onReady={(a) => (anchor = a)} />)
    const scroller = getByTestId('scroller')

    grow(1400) // ceiling: 1400 - 400 = 1000
    vi.advanceTimersByTime(1500)
    scroller.scrollTop = 200
    fireEvent.scroll(scroller) // a real gesture — following stops

    scrollHeight = 1800 // the dock grows again while scrolled away
    act(() => anchor?.notifyReflow())
    vi.advanceTimersByTime(1500)

    expect(scroller.scrollTop).toBe(200)
  })
})
