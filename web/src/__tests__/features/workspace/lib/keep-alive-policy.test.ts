import { describe, it, expect } from 'vitest'
import { planRetention, RETENTION_CAP } from '@/features/workspace/lib/keep-alive-policy'

const MIN = 60_000

describe('planRetention', () => {
  it('retains a lone active workspace and schedules nothing', () => {
    const plan = planRetention([{ wsId: 'a', lastActiveAt: 1000 }], 1000, 10 * MIN, RETENTION_CAP)
    expect(plan.retain).toEqual(['a'])
    expect(plan.evict).toEqual([])
    expect(plan.nextExpiryAt).toBeNull()
  })

  it('returns empty plan for no entries', () => {
    const plan = planRetention([], 5000, 10 * MIN, RETENTION_CAP)
    expect(plan).toEqual({ retain: [], evict: [], nextExpiryAt: null })
  })

  it('retains entries whose age is within the window', () => {
    const now = 100_000
    const plan = planRetention(
      [
        { wsId: 'active', lastActiveAt: now },
        { wsId: 'recent', lastActiveAt: now - 2 * MIN }, // 2 min old, window 10 min
      ],
      now,
      10 * MIN,
      RETENTION_CAP,
    )
    expect(plan.retain).toEqual(['active', 'recent'])
    expect(plan.evict).toEqual([])
    // The earliest non-active retained expiry: recent.lastActiveAt + window.
    expect(plan.nextExpiryAt).toBe(now - 2 * MIN + 10 * MIN)
  })

  it('evicts entries whose age exceeds the window, keeping the active one', () => {
    const now = 100_000
    const plan = planRetention(
      [
        { wsId: 'active', lastActiveAt: now },
        { wsId: 'stale', lastActiveAt: now - 11 * MIN }, // 11 min old, beyond 10 min window
      ],
      now,
      10 * MIN,
      RETENTION_CAP,
    )
    expect(plan.retain).toEqual(['active'])
    expect(plan.evict).toEqual(['stale'])
    expect(plan.nextExpiryAt).toBeNull()
  })

  it('treats an entry exactly at the window boundary as expired (avoids a timer loop)', () => {
    const now = 100_000
    const plan = planRetention(
      [
        { wsId: 'active', lastActiveAt: now },
        { wsId: 'edge', lastActiveAt: now - 10 * MIN }, // exactly at the window
      ],
      now,
      10 * MIN,
      RETENTION_CAP,
    )
    expect(plan.retain).toEqual(['active'])
    expect(plan.evict).toEqual(['edge'])
  })

  it('never evicts the most-recently-active workspace even when it is idle past the window', () => {
    // Simulates the timer path: the active workspace has sat idle past the
    // window but must still be retained.
    const now = 100_000
    const plan = planRetention(
      [{ wsId: 'active', lastActiveAt: now - 30 * MIN }],
      now,
      10 * MIN,
      RETENTION_CAP,
    )
    expect(plan.retain).toEqual(['active'])
    expect(plan.evict).toEqual([])
    expect(plan.nextExpiryAt).toBeNull()
  })

  it('windowMs === 0 retains only the active workspace (destroy-on-switch)', () => {
    const now = 100_000
    const plan = planRetention(
      [
        { wsId: 'active', lastActiveAt: now },
        { wsId: 'b', lastActiveAt: now - 1 },
        { wsId: 'c', lastActiveAt: now - 2 },
      ],
      now,
      0,
      RETENTION_CAP,
    )
    expect(plan.retain).toEqual(['active'])
    expect(plan.evict).toEqual(['b', 'c'])
    expect(plan.nextExpiryAt).toBeNull()
  })

  it('caps the retained set at the hard cap, evicting the oldest despite the window', () => {
    const now = 1_000_000
    // 7 workspaces all within the window; cap is 6, so the oldest is evicted.
    const entries = Array.from({ length: 7 }, (_, i) => ({
      wsId: `ws${i}`,
      // ws0 oldest ... ws6 newest (active)
      lastActiveAt: now - (6 - i) * 1000,
    }))
    const plan = planRetention(entries, now, 60 * MIN, 6)
    expect(plan.retain).toEqual(['ws1', 'ws2', 'ws3', 'ws4', 'ws5', 'ws6'])
    expect(plan.evict).toEqual(['ws0'])
  })

  it('preserves input order in retain and evict arrays', () => {
    const now = 1000
    const plan = planRetention(
      [
        { wsId: 'x', lastActiveAt: now - 20 * MIN }, // stale
        { wsId: 'y', lastActiveAt: now }, // active
        { wsId: 'z', lastActiveAt: now - 1 * MIN }, // recent
      ],
      now,
      10 * MIN,
      RETENTION_CAP,
    )
    // retain follows input order: y then z (x is stale)
    expect(plan.retain).toEqual(['y', 'z'])
    expect(plan.evict).toEqual(['x'])
  })

  it('nextExpiryAt is the earliest retained non-active expiry', () => {
    const now = 500_000
    const plan = planRetention(
      [
        { wsId: 'active', lastActiveAt: now },
        { wsId: 'older', lastActiveAt: now - 5 * MIN },
        { wsId: 'newer', lastActiveAt: now - 1 * MIN },
      ],
      now,
      10 * MIN,
      RETENTION_CAP,
    )
    // older expires first: (now - 5min) + 10min = now + 5min
    expect(plan.nextExpiryAt).toBe(now - 5 * MIN + 10 * MIN)
  })

  it('exposes a hard cap of 6', () => {
    expect(RETENTION_CAP).toBe(6)
  })
})
