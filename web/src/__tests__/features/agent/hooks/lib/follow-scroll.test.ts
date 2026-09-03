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
