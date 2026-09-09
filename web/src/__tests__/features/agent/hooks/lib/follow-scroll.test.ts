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

    vi.advanceTimersByTime(1000) // several time constants — fully settled
    expect(el.scrollTop).toBe(1000)
  })

  it('does nothing before setTarget is called', () => {
    el.scrollTop = 42
    createFollowScroll(el)
    vi.advanceTimersByTime(500)
    expect(el.scrollTop).toBe(42)
  })

  // The bug this whole redesign exists to fix: a fixed-duration tween,
  // cancelled and restarted from scratch on every retarget, resets to the
  // steep part of a fresh ease curve each time — and a target that moves
  // faster than the tween's own duration means every restart gets cut short
  // just a few pixels in, reading as near-instant, choppy motion instead of
  // one glide. Retargeting here must never cause a visible step backward or
  // a restart, only a smooth continuation toward the new value.
  it('retargeting mid-flight (faster than settling) continues smoothly, with no step back or restart', () => {
    const follow = createFollowScroll(el)
    el.scrollTop = 0
    follow.setTarget(200) // simulates one new line landing

    vi.advanceTimersByTime(20)
    const beforeRetarget = el.scrollTop
    expect(beforeRetarget).toBeGreaterThan(0)

    follow.setTarget(220) // another line lands before the first has settled
    vi.advanceTimersByTime(20)

    // Kept moving forward from where it already was — no snap-back to 0, no
    // pause while a "new" animation spins up from scratch.
    expect(el.scrollTop).toBeGreaterThan(beforeRetarget)
  })

  it('a burst of many retargets in quick succession still ends up exactly at the final target', () => {
    const follow = createFollowScroll(el)
    el.scrollTop = 0
    for (let i = 1; i <= 20; i++) {
      follow.setTarget(i * 20) // 20 "new lines", one every 15ms
      vi.advanceTimersByTime(15)
    }
    vi.advanceTimersByTime(1000)
    expect(el.scrollTop).toBe(400)
  })

  it('calls onTick with the value it writes each frame', () => {
    const onTick = vi.fn()
    createFollowScroll(el, onTick).setTarget(500)

    vi.advanceTimersByTime(1000) // several time constants — fully settled

    expect(onTick).toHaveBeenCalled()
    expect(onTick.mock.calls.at(-1)?.[0]).toBe(500)
  })

  it('pauses when something else changes scrollTop between frames, and re-arms cleanly on the next setTarget', () => {
    const follow = createFollowScroll(el)
    el.scrollTop = 0
    follow.setTarget(1000)

    vi.advanceTimersByTime(20)
    const interrupted = el.scrollTop
    el.scrollTop = 5 // a real scroll gesture

    vi.advanceTimersByTime(500) // would otherwise have reached the target by now
    expect(el.scrollTop).toBe(5)
    expect(el.scrollTop).not.toBe(interrupted)

    // Re-arms from wherever scrollTop actually is now, not from the stale
    // pre-interruption position.
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

/**
 * What this loop does when the main thread is BUSY — which, during a long
 * streamed reply, is most of the time.
 *
 * Reported live as "scroll bouncing, not stable", alongside an fps HUD in the
 * red for the whole generation. Both of these are jank-triggered: each is
 * harmless at a steady 60fps and visible the moment frames start dropping.
 */
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

  /** rAF, driven by hand: the browser stops INVOKING callbacks under load,
   *  it does not stop accepting them, so a resumed frame arrives with a
   *  timestamp far later than the loop expected. */
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

  // A frame that arrives 250ms late must not swallow the whole remaining
  // distance in one write. Exponential smoothing over real elapsed time says
  // it should — 1 - e^(-250/100) is 92% of what is left — but the frames in
  // between were never painted, so the reader does not see a fast glide,
  // they see the transcript teleport. Bounded per-frame motion is what makes
  // a recovering scroll read as catching up rather than jumping.
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
    vi.advanceTimersByTime(250) // a burst of dropped frames
    frame()
    const jump = el.scrollTop - afterNormalFrame

    // Before this was bounded, one late frame closed ~92% of the gap.
    expect(jump).toBeLessThan(remaining * 0.4)
    expect(jump).toBeGreaterThan(0) // still making real progress
  })

  it('still converges on the target after a burst, just over more frames', () => {
    const frame = manualRaf()
    el.scrollTop = 0
    const follow = createFollowScroll(el)
    follow.setTarget(1000)
    for (let i = 0; i < 60; i++) {
      vi.advanceTimersByTime(120) // sustained jank throughout
      frame()
    }
    expect(el.scrollTop).toBe(1000)
  })
})

/**
 * A container CLAMPS scrollTop to its own scrollable ceiling, so what it
 * takes is not always what it was handed — and the ceiling drops whenever
 * the transcript's content shrinks (a turn releasing the room reserved to
 * pin it to the top is exactly that, and so is a re-render that collapses
 * leaves).
 *
 * This matters because the value reported through `onTick` is what
 * `use-transcript-anchor` compares the next scroll EVENT against to tell its
 * own writes apart from the reader's gesture. Reporting a position the
 * container never took makes the very next scroll event look like the reader
 * grabbing the scrollbar — the transcript decides it is no longer stuck and
 * stops following the reply mid-generation.
 */
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
    // Every reported tick has to be a position the element genuinely holds,
    // or the anchor's "was that me or the reader?" check is comparing against
    // a number that never existed.
    expect(seen.at(-1)).toBe(el.scrollTop)
  })

  it('keeps following after the content shrinks under it', () => {
    let ceiling = 1000
    clamped(() => ceiling)
    const seen: number[] = []
    const follow = createFollowScroll(el, (v) => seen.push(v))
    follow.setTarget(1000)
    vi.advanceTimersByTime(120)

    // The reply released the room reserved above it: the reachable bottom
    // drops sharply, with no retarget in between.
    ceiling = 200
    vi.advanceTimersByTime(500)

    expect(el.scrollTop).toBe(200)
    expect(seen.at(-1)).toBe(el.scrollTop)
  })
})
