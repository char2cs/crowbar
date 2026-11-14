import { renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { formatClock, useTextClock } from '@/features/agent/activity/lib/text-clock'

afterEach(() => vi.useRealTimers())

/** Mounts a clock from `since` on a fresh element; returns it and its unmount. */
function mount(since: string) {
  const el = document.createElement('span')
  const ref = renderHook(() => useTextClock(since)).result.current
  const stop = ref(el)
  return { el, stop: () => stop?.() }
}

describe('formatClock', () => {
  it('writes seconds, then minutes with padded seconds, then hours', () => {
    expect(formatClock(58)).toBe('58s')
    expect(formatClock(102)).toBe('1m 42s')
    expect(formatClock(363)).toBe('6m 03s')
    expect(formatClock(3900)).toBe('1h 05m')
    expect(formatClock(-5)).toBe('0s')
  })
})

describe('text clock', () => {
  it('writes elapsed time into each element without a React render', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-01T12:00:05Z'))
    const a = mount('2026-10-01T12:00:00Z')
    const b = mount('2026-10-01T11:59:00Z')
    expect([a.el.textContent, b.el.textContent]).toEqual(['5s', '1m 05s'])
    vi.advanceTimersByTime(2000)
    expect([a.el.textContent, b.el.textContent]).toEqual(['7s', '1m 07s'])
    a.stop()
    vi.advanceTimersByTime(1000)
    expect([a.el.textContent, b.el.textContent]).toEqual(['7s', '1m 08s'])
    b.stop()
  })

  it('stops its interval when the last element unregisters', () => {
    vi.useFakeTimers()
    const clocks = [mount('2026-10-01T12:00:00Z'), mount('2026-10-01T12:00:00Z')]
    expect(vi.getTimerCount()).toBe(1)
    clocks[0]!.stop()
    expect(vi.getTimerCount()).toBe(1)
    clocks[1]!.stop()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('runs no clock without a start time', () => {
    vi.useFakeTimers()
    const el = document.createElement('span')
    const ref = renderHook(() => useTextClock(undefined)).result.current
    expect(ref(el)).toBeUndefined()
    expect(vi.getTimerCount()).toBe(0)
  })
})
