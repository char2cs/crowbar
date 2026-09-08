import { useEffect } from 'react'
import { act, fireEvent, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  tailRoom,
  useTranscriptAnchor,
  type TranscriptAnchor,
  type UseTranscriptAnchorOptions,
} from '@/features/agent/hooks/use-transcript-anchor'

/**
 * jsdom has no layout engine — `scrollHeight`/`clientHeight` are otherwise
 * always 0, and its ResizeObserver stub (src/__tests__/setup.ts) never calls
 * back. A controllable observer, and mutable test-scoped dimensions read
 * live by the mounted element's getters, stand in for both — same pattern
 * use-preserved-scroll.test.tsx uses for the same reason.
 */
/**
 * A scroll the READER caused. A browser never delivers a bare scroll event
 * for a gesture — it always emits the input that caused it first (`wheel`
 * here), and `useTranscriptAnchor` relies on exactly that to tell a real
 * gesture apart from the browser moving the view by itself (scroll
 * anchoring, which a streaming transcript triggers constantly). Setting
 * `scrollTop` and firing `scroll` alone simulates the browser, not a person.
 */
function readerScrollsTo(scroller: HTMLElement, top: number) {
  fireEvent.wheel(scroller)
  scroller.scrollTop = top
  fireEvent.scroll(scroller)
}

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

  function Host({
    onReady,
    anchorOptions,
  }: {
    onReady?: (anchor: TranscriptAnchor) => void
    anchorOptions?: UseTranscriptAnchorOptions
  }) {
    const anchor = useTranscriptAnchor(anchorOptions)
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
    readerScrollsTo(scroller, 200)

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
    readerScrollsTo(scroller, 200)
    vi.advanceTimersByTime(1500) // several time constants — fully settled

    // Back at the bottom (scrollHeight 1400 - clientHeight 400 = 1000).
    readerScrollsTo(scroller, 1000)

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
    readerScrollsTo(scroller, 200) // a real gesture — following stops

    scrollHeight = 1800 // the dock grows again while scrolled away
    act(() => anchor?.notifyReflow())
    vi.advanceTimersByTime(1500)

    expect(scroller.scrollTop).toBe(200)
  })

  // Regression: a cold/warm chat open reads as the whole transcript sweeping
  // from wherever the loading state left it up to the true bottom, because
  // the virtualized list's own initial estimated→measured row corrections
  // retarget the SAME eased glide built for a single new line streaming in.
  describe('loadingHistory (initial-load settling)', () => {
    it('snaps instantly, not eased, on every resync while loadingHistory is true', () => {
      const { getByTestId } = render(<Host anchorOptions={{ loadingHistory: true }} />)
      const scroller = getByTestId('scroller')
      expect(scroller.scrollTop).toBe(600) // mount ceiling: 1000 - 400

      // Simulates the virtualizer's own initial estimated→measured corrections.
      grow(1400) // ceiling: 1400 - 400 = 1000
      // Instant: already at the target with zero time advanced — the eased
      // path would still be short of it here (compare the "eases toward the
      // bottom" test above, which advances 50ms and asserts strictly less).
      expect(scroller.scrollTop).toBe(1000)

      grow(1800) // a second correction lands
      expect(scroller.scrollTop).toBe(1400) // ceiling: 1800 - 400
    })

    it('arms eased follow only once loadingHistory goes false AND a full frame passes with nothing left to settle', () => {
      const { getByTestId, rerender } = render(<Host anchorOptions={{ loadingHistory: true }} />)
      const scroller = getByTestId('scroller')

      // The initial page's rows are still settling.
      grow(1400)
      expect(scroller.scrollTop).toBe(1000) // instant, as above

      // loadInitial resolves — but another correction lands right after,
      // still within the same settle burst.
      rerender(<Host anchorOptions={{ loadingHistory: false }} />)
      grow(1500)
      expect(scroller.scrollTop).toBe(1100) // still instant: 1500 - 400

      // Quiet now: let the pending arm frame actually fire.
      vi.advanceTimersByTime(16)

      // A genuinely new message streams in — now eased, not instant.
      grow(1900) // ceiling: 1900 - 400 = 1500
      vi.advanceTimersByTime(50)
      expect(scroller.scrollTop).toBeGreaterThan(1100)
      expect(scroller.scrollTop).toBeLessThan(1500)

      vi.advanceTimersByTime(1500) // several time constants — fully settled
      expect(scroller.scrollTop).toBe(1500)
    })

    it('arms eased follow even with no resize at all — an empty chat whose loadingHistory simply goes false', () => {
      const { getByTestId, rerender } = render(<Host anchorOptions={{ loadingHistory: true }} />)
      const scroller = getByTestId('scroller')

      rerender(<Host anchorOptions={{ loadingHistory: false }} />)
      vi.advanceTimersByTime(16) // the backstop's own arm frame

      grow(1400) // ceiling: 1400 - 400 = 1000
      vi.advanceTimersByTime(50)
      expect(scroller.scrollTop).toBeGreaterThan(600)
      expect(scroller.scrollTop).toBeLessThan(1000)
    })
  })

  // Regression: no per-chat scroll persistence existed at all — switching
  // chats and back always defaulted to the bottom, even for a chat the
  // reader had deliberately scrolled up in.
  describe('initialPosition / onPositionChange (per-chat restore)', () => {
    it('restores a saved non-bottom position instantly on mount, and marks it not-stuck', () => {
      const { getByTestId } = render(
        <Host anchorOptions={{ initialPosition: { stuck: false, distanceFromBottom: 500 } }} />,
      )
      const scroller = getByTestId('scroller')
      // scrollHeight 1000 - distanceFromBottom 500.
      expect(scroller.scrollTop).toBe(500)

      // Not-stuck carried over: ordinary growth must not auto-follow.
      grow(1400)
      vi.advanceTimersByTime(1500)
      expect(scroller.scrollTop).toBe(500)
    })

    it('a saved stuck position still lands at the bottom, same as no saved position', () => {
      const { getByTestId } = render(
        <Host anchorOptions={{ initialPosition: { stuck: true, distanceFromBottom: 300 } }} />,
      )
      expect(getByTestId('scroller').scrollTop).toBe(600) // ceiling: 1000 - 400
    })

    it('reports the final position, once, on unmount', () => {
      const onPositionChange = vi.fn()
      const { getByTestId, unmount } = render(<Host anchorOptions={{ onPositionChange }} />)
      const scroller = getByTestId('scroller')
      readerScrollsTo(scroller, 250) // well short of the 600 ceiling — not stuck

      expect(onPositionChange).not.toHaveBeenCalled()
      unmount()

      // distanceFromBottom is scrollHeight (1000) - scrollTop (250), not
      // relative to the ceiling — see the type's own doc.
      expect(onPositionChange).toHaveBeenCalledTimes(1)
      expect(onPositionChange).toHaveBeenCalledWith({ stuck: false, distanceFromBottom: 750 })
    })

    it('a chat that stayed stuck to the bottom reports stuck: true on unmount', () => {
      const onPositionChange = vi.fn()
      const { unmount } = render(<Host anchorOptions={{ onPositionChange }} />)

      unmount()

      // At the mount ceiling (scrollTop 600), distanceFromBottom is
      // scrollHeight (1000) - 600 = 400 — clientHeight is the closest a
      // fully-stuck reader's own distance-from-bottom ever gets to zero.
      expect(onPositionChange).toHaveBeenCalledWith({ stuck: true, distanceFromBottom: 400 })
    })
  })
})

/**
 * Turn-start pinning — see `tailRoom`'s own doc comment.
 *
 * Measured against Claude Code Desktop frame-by-frame: it lifts the prompt
 * you just sent to the TOP of the transcript the instant the turn starts, and
 * the reply fills the space underneath. Crowbar left the new turn wherever
 * bottom-following had parked the previous one — roughly mid-viewport, with
 * the whole previous exchange still stacked above it — so a reply had far
 * less room to grow into before the view had to scroll again, and every one
 * of those extra scrolls was another chance to visibly lag and then snap.
 */
describe('tailRoom', () => {
  it('reserves exactly the shortfall while the reply is shorter than the viewport', () => {
    // The prompt starts 900px down a 1000px-tall content, viewport 400px:
    // only 100px sits below it, so 300px more is needed to lift it to the top.
    expect(tailRoom(900, 1000, 400)).toBe(300)
  })

  it('reserves nothing — releasing the pin — once the reply fills the viewport', () => {
    // 500px of reply below the prompt already exceeds the 400px viewport.
    expect(tailRoom(500, 1000, 400)).toBe(0)
  })

  it('is exactly zero at the handover point, so the two phases meet with no jump', () => {
    // The instant the content below the pin equals the viewport, the
    // reservation reaches zero — the pinned position and the true bottom are
    // then the same pixel, which is what makes the handoff invisible.
    expect(tailRoom(600, 1000, 400)).toBe(0)
    expect(tailRoom(601, 1000, 400)).toBe(1)
  })

  it('never reserves negative room for a reply far past the viewport', () => {
    expect(tailRoom(0, 5000, 400)).toBe(0)
  })
})

describe('useTranscriptAnchor: pinning a starting turn to the top', () => {
  let scrollHeight = 0
  let clientHeight = 400
  let pinTop = 0
  let observerCallbacks: Array<() => void> = []
  const RealResizeObserver = globalThis.ResizeObserver

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'performance'] })
    scrollHeight = 1000
    clientHeight = 400
    pinTop = 900
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

  const fire = () =>
    act(() => {
      for (const cb of [...observerCallbacks]) cb()
    })

  function PinHost({ onReady }: { onReady: (anchor: TranscriptAnchor) => void }) {
    const anchor = useTranscriptAnchor()
    useEffect(() => {
      onReady(anchor)
    }, [anchor, onReady])
    return (
      <div
        data-testid="scroller"
        ref={(node) => {
          anchor.scrollRef.current = node
          if (!node || Object.hasOwn(node, 'scrollHeight')) return
          let top = 0
          // Room reserved on the content element is real scrollable height,
          // exactly as the padding this hook writes would be in a browser.
          const reserved = () => {
            const content = node.lastElementChild as HTMLElement | null
            return parseFloat(content?.style.paddingBottom || '0') || 0
          }
          Object.defineProperty(node, 'scrollTop', {
            configurable: true,
            get: () => top,
            set: (v: number) => {
              const max = Math.max(0, scrollHeight + reserved() - clientHeight)
              top = Math.max(0, Math.min(v, max))
            },
          })
          Object.defineProperty(node, 'scrollHeight', {
            configurable: true,
            get: () => scrollHeight + reserved(),
          })
          Object.defineProperty(node, 'clientHeight', {
            configurable: true,
            get: () => clientHeight,
          })
          node.getBoundingClientRect = () => ({ top: 0 }) as DOMRect
        }}
        onScroll={anchor.onScroll}
      >
        <div data-testid="content">
          <div
            data-testid="pin"
            ref={(node) => {
              if (!node) return
              // The prompt sits `pinTop` down the content; its on-screen top
              // is that minus however far the container is scrolled.
              node.getBoundingClientRect = () =>
                ({
                  top: pinTop - (node.parentElement?.parentElement?.scrollTop ?? 0),
                }) as DOMRect
            }}
          />
        </div>
      </div>
    )
  }

  it('lifts the just-sent prompt to the top instead of leaving it mid-viewport', () => {
    let anchor!: TranscriptAnchor
    const { getByTestId } = render(<PinHost onReady={(a) => (anchor = a)} />)
    const scroller = getByTestId('scroller')

    // Before pinning there is nowhere further to scroll: the ceiling is 600,
    // which leaves the prompt (900 down) 300px BELOW the top of the viewport —
    // exactly the "sits in the lower-middle with old context still above it"
    // the recordings showed.
    expect(scroller.scrollTop).toBe(600)
    expect(pinTop - scroller.scrollTop).toBe(300)

    act(() => anchor.pinTurnToTop(getByTestId('pin')))
    vi.advanceTimersByTime(1500)

    // Now the prompt's top edge IS the top of the viewport.
    expect(scroller.scrollTop).toBe(900)
    expect(pinTop - scroller.scrollTop).toBe(0)
  })

  // Regression: chasing this bug through the input-recency heuristic (the
  // composer's Enter keydown, then the Send button's pointerdown/up) fixed
  // two live-reported causes one at a time, with no reason to believe those
  // were the last of them — the send gesture is ALWAYS one of wheel/touch/
  // key/pointer, by definition, so it always sits inside READER_INPUT_MS
  // regardless of which of those it happens to be. `pinTurnToTop` protects
  // its own `stuck = true` directly instead: for PIN_SETTLE_GRACE_MS after a
  // turn starts, NOTHING reads a scroll as the reader's, whatever kind of
  // input just happened — proven here with a plain `wheel`, an event this
  // file has always correctly tracked, to show the grace window is what is
  // actually holding `stuck` here, not the type of the event.
  it('a real, ordinary input event landing right after pinning does not un-stick the pin', () => {
    let anchor!: TranscriptAnchor
    const { getByTestId } = render(<PinHost onReady={(a) => (anchor = a)} />)
    const scroller = getByTestId('scroller')

    act(() => anchor.pinTurnToTop(getByTestId('pin')))

    // A real, unrelated wheel-driven scroll event lands moments later — the
    // same shape as the browser's own resize-driven adjustment landing near
    // any ordinary input, not specifically Enter or Send.
    act(() => {
      fireEvent.wheel(scroller)
      scroller.scrollTop = 500
      fireEvent.scroll(scroller)
    })

    vi.advanceTimersByTime(1500)

    // Still lands the pin — the wheel event did not un-stick it.
    expect(scroller.scrollTop).toBe(900)
  })

  it('holds the prompt at the top while a short reply grows underneath it', () => {
    let anchor!: TranscriptAnchor
    const { getByTestId } = render(<PinHost onReady={(a) => (anchor = a)} />)
    const scroller = getByTestId('scroller')
    act(() => anchor.pinTurnToTop(getByTestId('pin')))
    vi.advanceTimersByTime(1500)

    // 200px of reply lands — still short of the 400px viewport, so the prompt
    // must not move at all: the reply fills the reserved space instead.
    scrollHeight = 1200
    fire()
    vi.advanceTimersByTime(1500)
    expect(scroller.scrollTop).toBe(900)
  })

  it('hands over to ordinary bottom-following once the reply outgrows the space', () => {
    let anchor!: TranscriptAnchor
    const { getByTestId } = render(<PinHost onReady={(a) => (anchor = a)} />)
    const scroller = getByTestId('scroller')
    const content = getByTestId('content')
    act(() => anchor.pinTurnToTop(getByTestId('pin')))
    vi.advanceTimersByTime(1500)
    expect(content.style.paddingBottom).toBe('300px')

    // The reply grows past the viewport: 700px now sits below the prompt.
    scrollHeight = 1600
    fire()
    vi.advanceTimersByTime(1500)

    // Nothing is reserved any more, and the transcript is following the true
    // bottom again — 1600 - 400.
    expect(content.style.paddingBottom).toBe('')
    expect(scroller.scrollTop).toBe(1200)
  })

  it('reserves nothing at all when the reply already fills the viewport', () => {
    let anchor!: TranscriptAnchor
    pinTop = 200
    const { getByTestId } = render(<PinHost onReady={(a) => (anchor = a)} />)
    const content = getByTestId('content')

    act(() => anchor.pinTurnToTop(getByTestId('pin')))
    vi.advanceTimersByTime(1500)

    expect(content.style.paddingBottom).toBe('')
  })
})

/**
 * Who moved the scrollbar?
 *
 * Following stops the moment the reader scrolls up — that is this hook's whole
 * contract, and it is the right one. But `scrollTop` changing is NOT evidence
 * that the reader did anything: the browser moves it too, on its own, to keep
 * the view stable when content around it changes size (scroll anchoring). A
 * transcript is content changing size continuously, so this is not an edge
 * case.
 *
 * Measured live during a 35-item numbered list, sampled every 200ms: the view
 * sat correctly 20-80px from the bottom for 15 seconds, then jumped BACKWARD
 * 213px in a single sample while `scrollHeight` moved by only 3px — far too
 * small a content change to have clamped it, and no gesture anywhere near it.
 * Following then stopped dead for 15.6 seconds, the gap frozen at 242px, and
 * only recovered when something unrelated forced a resync. Read as "scroll
 * bouncing, not stable".
 *
 * Real input is the discriminator, and the browser hands it to us: a gesture
 * arrives as `wheel`, `touchmove`, `keydown` or a scrollbar `pointerdown`
 * moments before the scroll event it causes. An anchoring adjustment arrives
 * with none of them.
 */
describe('useTranscriptAnchor: telling the reader apart from the browser', () => {
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

  const grow = (next: number) => {
    scrollHeight = next
    act(() => {
      for (const cb of [...observerCallbacks]) cb()
    })
  }

  function Host() {
    const anchor = useTranscriptAnchor()
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

  it('keeps following when the browser moves the view with no input from the reader', () => {
    const { getByTestId } = render(<Host />)
    const scroller = getByTestId('scroller')
    expect(scroller.scrollTop).toBe(600) // pinned to the bottom

    // Scroll anchoring pulls the view up as content around it resizes. No
    // wheel, no key, no pointer — nobody touched anything.
    act(() => {
      scroller.scrollTop = 350
      fireEvent.scroll(scroller)
    })

    grow(1400)
    vi.advanceTimersByTime(1500)

    // It has to recover on its own. Before this, following stopped here for
    // the rest of the turn.
    expect(scroller.scrollTop).toBe(1000)
  })

  it('still stops following the moment the reader really does scroll up', () => {
    const { getByTestId } = render(<Host />)
    const scroller = getByTestId('scroller')

    // The same movement, this time caused by a real gesture.
    act(() => {
      fireEvent.wheel(scroller)
      scroller.scrollTop = 350
      fireEvent.scroll(scroller)
    })

    grow(1400)
    vi.advanceTimersByTime(1500)

    // Left exactly where they put it — yanking a reader back to the bottom is
    // worse than never following at all.
    expect(scroller.scrollTop).toBe(350)
  })

  it('treats a keyboard scroll as the reader too', () => {
    const { getByTestId } = render(<Host />)
    const scroller = getByTestId('scroller')

    act(() => {
      fireEvent.keyDown(scroller, { key: 'PageUp' })
      scroller.scrollTop = 200
      fireEvent.scroll(scroller)
    })

    grow(1400)
    vi.advanceTimersByTime(1500)

    expect(scroller.scrollTop).toBe(200)
  })

  // Regression: the live bug reported as "the space is there, the
  // auto-scroll didn't work" right after sending a message. Pressing Enter
  // to SEND is a keydown too, and it fires on the composer — a
  // contenteditable, nowhere near the transcript — but the window-level
  // listener could not tell that apart from PageUp/PageDown scrolling the
  // transcript itself, so the send keystroke armed `reader` for a full
  // second afterward. Any resize-driven scroll adjustment landing in that
  // window (the browser's own clamp when content shrinks while a queued
  // prompt settles into the ledger, say) then read as the reader grabbing
  // the scrollbar, latched `stuck` false, and following never resumed for
  // the rest of the turn — visible live as the transcript freezing exactly
  // where that one adjustment left it, while the tail-room reservation kept
  // adjusting around it with nothing to show for it.
  it('does not treat a keydown that fires while typing/sending in the composer as the reader scrolling', () => {
    function HostWithComposer() {
      const anchor = useTranscriptAnchor()
      return (
        <div>
          <div data-testid="composer" contentEditable suppressContentEditableWarning />
          <div
            data-testid="scroller"
            ref={(node) => {
              anchor.scrollRef.current = node
              if (!node || Object.hasOwn(node, 'scrollHeight')) return
              let top = 0
              Object.defineProperty(node, 'scrollTop', {
                configurable: true,
                get: () => top,
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
        </div>
      )
    }

    const { getByTestId } = render(<HostWithComposer />)
    const scroller = getByTestId('scroller')
    const composer = getByTestId('composer')
    expect(scroller.scrollTop).toBe(600) // pinned to the bottom

    act(() => {
      // Pressing Enter to send.
      fireEvent.keyDown(composer, { key: 'Enter' })
      // A resize-driven scroll adjustment (the browser's own clamp) lands
      // moments later, well within READER_INPUT_MS of that keystroke — with
      // no wheel, touch, or pointer gesture anywhere.
      scroller.scrollTop = 350
      fireEvent.scroll(scroller)
    })

    grow(1400)
    vi.advanceTimersByTime(1500)

    // Recovers and keeps following — the send keystroke must never have
    // been read as the reader scrolling away.
    expect(scroller.scrollTop).toBe(1000)
  })

  // Regression, same bug as the keydown one above, different event: a
  // pointerdown/pointerup pair is exactly what clicking the SEND BUTTON
  // fires too, and pointerdown/up were tracked window-wide with no target
  // check at all — so clicking Send (as opposed to pressing Enter, the
  // other regression here) reproduced the identical freeze through a
  // completely different, still-unfixed path.
  it('does not treat a click on something outside the transcript (e.g. Send) as the reader touching the scrollbar', () => {
    function HostWithButton() {
      const anchor = useTranscriptAnchor()
      return (
        <div>
          <button data-testid="send-button" type="button" />
          <div
            data-testid="scroller"
            ref={(node) => {
              anchor.scrollRef.current = node
              if (!node || Object.hasOwn(node, 'scrollHeight')) return
              let top = 0
              Object.defineProperty(node, 'scrollTop', {
                configurable: true,
                get: () => top,
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
        </div>
      )
    }

    const { getByTestId } = render(<HostWithButton />)
    const scroller = getByTestId('scroller')
    const sendButton = getByTestId('send-button')
    expect(scroller.scrollTop).toBe(600)

    act(() => {
      fireEvent.pointerDown(sendButton)
      fireEvent.pointerUp(sendButton)
      // A resize-driven scroll adjustment lands moments later — same shape
      // as the keydown regression above.
      scroller.scrollTop = 350
      fireEvent.scroll(scroller)
    })

    grow(1400)
    vi.advanceTimersByTime(1500)

    expect(scroller.scrollTop).toBe(1000)
  })

  // The behaviour the scoping above must NOT break: a real scrollbar drag
  // starts with pointerdown ON the scroller itself and can end anywhere —
  // the cursor routinely outruns the scrollbar during a fast drag — so
  // pointerup/pointercancel stay unscoped, gated on `pointerHeld` instead.
  it('still treats a real scrollbar drag as the reader, even when it ends outside the scroller', () => {
    const { getByTestId } = render(<Host />)
    const scroller = getByTestId('scroller')

    act(() => {
      fireEvent.pointerDown(scroller)
      scroller.scrollTop = 200
      fireEvent.scroll(scroller)
      // Released off the scroller — document, not the scroller itself.
      fireEvent.pointerUp(document.body)
    })

    grow(1400)
    vi.advanceTimersByTime(1500)

    // Left exactly where the drag put it.
    expect(scroller.scrollTop).toBe(200)
  })
})
