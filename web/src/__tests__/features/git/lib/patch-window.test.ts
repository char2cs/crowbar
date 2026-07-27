import { describe, expect, it } from 'vitest'
import {
  planWindow,
  LOOKAHEAD_FILES,
  EVICT_BEYOND_FILES,
  MAX_MATERIALIZED_FILES,
  MAX_MATERIALIZED_LINES,
} from '@/features/git/lib/patch-window'
import type { WindowInput } from '@/features/git/lib/patch-window'

// planWindow is the one part of the windowed renderer that decides, on every
// scroll frame, which file patches exist in memory. It is deliberately pure —
// no React, no fetch, no timers — so these tests are the exhaustive proof that
// it converges instead of thrashing.

function makePaths(n: number, prefix = 'f'): string[] {
  return Array.from({ length: n }, (_, i) => `${prefix}${i}`)
}

function uniformCounts(paths: readonly string[], lines: number): Record<string, number> {
  return Object.fromEntries(paths.map((p) => [p, lines]))
}

/** Build a WindowInput with sane defaults so each test states only what it exercises. */
function input(overrides: Partial<WindowInput> & { paths: readonly string[] }): WindowInput {
  const paths = overrides.paths
  return {
    visible: { first: 0, last: 0 },
    total: paths.length,
    materialized: [],
    lineCounts: uniformCounts(paths, 100),
    ...overrides,
  }
}

/** The set of paths still materialised once a plan is applied. */
function applyPlan(materialized: readonly string[], plan: { fetch: string[]; evict: string[] }) {
  const next = new Set(materialized)
  for (const p of plan.evict) next.delete(p)
  for (const p of plan.fetch) next.add(p)
  return [...next]
}

describe('planWindow constants', () => {
  // The hysteresis gap IS the anti-thrash mechanism: a file that leaves the
  // fetch band must sit in a no-man's-land before it is allowed to be evicted.
  it('the eviction band is strictly wider than the lookahead band', () => {
    expect(EVICT_BEYOND_FILES).toBeGreaterThan(LOOKAHEAD_FILES)
  })

  it('exposes the materialisation budgets', () => {
    expect(MAX_MATERIALIZED_FILES).toBe(40)
    expect(MAX_MATERIALIZED_LINES).toBe(60_000)
  })
})

describe('planWindow — empty input', () => {
  it('is a no-op with no files at all', () => {
    expect(
      planWindow({
        visible: { first: 0, last: 0 },
        total: 0,
        materialized: [],
        paths: [],
        lineCounts: {},
      }),
    ).toEqual({ fetch: [], evict: [] })
  })

  it('is a no-op when the file list is empty but something is still materialised', () => {
    expect(
      planWindow({
        visible: { first: 0, last: 0 },
        total: 0,
        materialized: ['stale.ts'],
        paths: [],
        lineCounts: {},
      }),
    ).toEqual({ fetch: [], evict: [] })
  })
})

describe('planWindow — the fetch band', () => {
  it('fetches the visible band plus LOOKAHEAD_FILES on each side when nothing is materialised', () => {
    const paths = makePaths(100)
    const plan = planWindow(input({ paths, visible: { first: 20, last: 24 } }))

    // 20-6 .. 24+6 inclusive
    expect(new Set(plan.fetch)).toEqual(new Set(paths.slice(14, 31)))
    expect(plan.fetch).toHaveLength(17)
    expect(plan.evict).toEqual([])
  })

  it('clamps the band at the head of the list', () => {
    const paths = makePaths(100)
    const plan = planWindow(input({ paths, visible: { first: 0, last: 1 } }))

    expect(new Set(plan.fetch)).toEqual(new Set(paths.slice(0, 8)))
  })

  it('clamps the band at the tail of the list', () => {
    const paths = makePaths(10)
    const plan = planWindow(input({ paths, visible: { first: 8, last: 9 } }))

    expect(new Set(plan.fetch)).toEqual(new Set(paths.slice(2, 10)))
  })

  it('does not refetch what is already materialised', () => {
    const paths = makePaths(100)
    const visible = { first: 20, last: 24 }
    const materialized = paths.slice(14, 31)

    const plan = planWindow(input({ paths, visible, materialized }))

    expect(plan.fetch).toEqual([])
    expect(plan.evict).toEqual([])
  })

  it('orders the fetch nearest-to-viewport first so on-screen files land first', () => {
    const paths = makePaths(100)
    const plan = planWindow(input({ paths, visible: { first: 20, last: 20 } }))

    expect(plan.fetch[0]).toBe('f20')
    // f19/f21 are distance 1, f18/f22 distance 2, ... — never a far file before a near one.
    expect(plan.fetch.slice(0, 3)).toEqual(['f20', 'f19', 'f21'])
    expect(plan.fetch.at(-1)).toBe('f26')
  })

  it('treats `total` as the authoritative item count when it is shorter than paths', () => {
    const paths = makePaths(10)
    const plan = planWindow(input({ paths, total: 3, visible: { first: 0, last: 0 } }))

    expect(new Set(plan.fetch)).toEqual(new Set(['f0', 'f1', 'f2']))
  })

  it('clamps an out-of-range viewport instead of producing garbage', () => {
    const paths = makePaths(5)
    const plan = planWindow(input({ paths, visible: { first: 99, last: 200 } }))

    expect(new Set(plan.fetch)).toEqual(new Set(paths))
  })
})

describe('planWindow — eviction band', () => {
  it('evicts materialised files further than EVICT_BEYOND_FILES from the viewport', () => {
    const paths = makePaths(100)
    // distances from a viewport of exactly f50: f0=50, f29=21, f30=20, f70=20, f71=21, f99=49
    const materialized = ['f0', 'f29', 'f30', 'f70', 'f71', 'f99']

    const plan = planWindow(input({ paths, visible: { first: 50, last: 50 }, materialized }))

    expect(new Set(plan.evict)).toEqual(new Set(['f0', 'f29', 'f71', 'f99']))
  })

  it('evicts a materialised path that is no longer in the file list at all', () => {
    const paths = makePaths(10)
    const plan = planWindow(
      input({ paths, visible: { first: 0, last: 0 }, materialized: ['f0', 'gone.ts'] }),
    )

    expect(plan.evict).toEqual(['gone.ts'])
  })

  // The property the whole hysteresis design exists for: a single-row scroll
  // that pushes a file out of the lookahead band must NOT evict it, or the very
  // next frame would refetch it — a network request per frame, forever.
  it('does not evict-then-refetch a file oscillating at the lookahead boundary', () => {
    const paths = makePaths(100)

    // Frame 1: viewport at rows 30..35 — the band reaches back to f24.
    const first = planWindow(input({ paths, visible: { first: 30, last: 35 } }))
    expect(first.fetch).toContain('f24')
    const materialized = applyPlan([], first)

    // Frame 2: one row down. f24 is now distance 7 — outside the lookahead band
    // but well inside the eviction band, so it stays.
    const second = planWindow(input({ paths, visible: { first: 31, last: 36 }, materialized }))
    expect(second.evict).toEqual([])
    expect(second.fetch).toEqual(['f42']) // only the newly-revealed lookahead row

    // Frame 3: back where we started. Nothing is refetched, because nothing was dropped.
    const third = planWindow(
      input({
        paths,
        visible: { first: 30, last: 35 },
        materialized: applyPlan(materialized, second),
      }),
    )
    expect(third.fetch).toEqual([])
    expect(third.evict).toEqual([])
  })
})

describe('planWindow — MAX_MATERIALIZED_FILES', () => {
  it('caps the materialised file count, evicting furthest-from-viewport first', () => {
    const paths = makePaths(60)
    const plan = planWindow(
      input({
        paths,
        visible: { first: 30, last: 31 },
        materialized: paths,
        lineCounts: uniformCounts(paths, 100), // 6,000 lines total — the LINE budget is not what bites
      }),
    )

    const survivors = applyPlan(paths, plan)
    expect(survivors).toHaveLength(MAX_MATERIALIZED_FILES)
    // The 40 survivors are the 40 nearest the viewport: f11..f50.
    expect(new Set(survivors)).toEqual(new Set(paths.slice(11, 51)))
    // Furthest goes first.
    expect(plan.evict[0]).toBe('f0')
  })
})

describe('planWindow — MAX_MATERIALIZED_LINES', () => {
  it('bites even when the file count is far under MAX_MATERIALIZED_FILES', () => {
    const paths = ['big.ts', 'other.ts']
    const plan = planWindow({
      visible: { first: 0, last: 0 },
      total: 2,
      materialized: paths,
      paths,
      lineCounts: { 'big.ts': 50_000, 'other.ts': 20_000 },
    })

    expect(paths.length).toBeLessThan(MAX_MATERIALIZED_FILES)
    expect(plan.evict).toEqual(['other.ts'])
  })

  // Evicting under the line budget is only correct if the very next plan does
  // not immediately propose fetching the same file back.
  it('does not re-propose a file the line budget cannot afford', () => {
    const paths = ['big.ts', 'other.ts']
    const lineCounts = { 'big.ts': 50_000, 'other.ts': 20_000 }

    const plan = planWindow({
      visible: { first: 0, last: 0 },
      total: 2,
      materialized: ['big.ts'],
      paths,
      lineCounts,
    })

    expect(plan).toEqual({ fetch: [], evict: [] })
  })

  it('counts a not-yet-fetched file against the budget before fetching it', () => {
    const paths = ['a.ts', 'b.ts', 'c.ts']
    const plan = planWindow({
      visible: { first: 0, last: 0 },
      total: 3,
      materialized: [],
      paths,
      lineCounts: { 'a.ts': 40_000, 'b.ts': 30_000, 'c.ts': 30_000 },
    })

    // a.ts is visible and always fetched; only one of the lookahead files fits.
    expect(plan.fetch).toEqual(['a.ts'])
    expect(plan.evict).toEqual([])
  })
})

describe('planWindow — a visible file is never evicted', () => {
  it('keeps a single visible file that alone blows the line budget', () => {
    const paths = ['monster.js']
    const plan = planWindow({
      visible: { first: 0, last: 0 },
      total: 1,
      materialized: paths,
      paths,
      lineCounts: { 'monster.js': 500_000 },
    })

    expect(plan.evict).toEqual([])
  })

  it('keeps every visible file even when they collectively blow the budget', () => {
    const paths = ['a.ts', 'b.ts']
    const plan = planWindow({
      visible: { first: 0, last: 1 },
      total: 2,
      materialized: paths,
      paths,
      lineCounts: { 'a.ts': 50_000, 'b.ts': 50_000 },
    })

    expect(plan.evict).toEqual([])
  })

  it('evicts every off-screen file before touching a visible one', () => {
    const paths = makePaths(30)
    const plan = planWindow(
      input({
        paths,
        visible: { first: 0, last: 0 },
        materialized: paths,
        lineCounts: { ...uniformCounts(paths, 10_000) },
      }),
    )

    const survivors = applyPlan(paths, plan)
    expect(survivors).toContain('f0')
    expect(plan.evict).not.toContain('f0')
  })
})

describe('planWindow — purity', () => {
  it('does not mutate its input', () => {
    const paths = makePaths(60)
    const materialized = [...paths]
    const lineCounts = uniformCounts(paths, 100)
    const arg: WindowInput = {
      visible: { first: 30, last: 31 },
      total: paths.length,
      materialized,
      paths,
      lineCounts,
    }

    planWindow(arg)

    expect(materialized).toEqual(paths)
    expect(paths).toEqual(makePaths(60))
    expect(lineCounts).toEqual(uniformCounts(makePaths(60), 100))
  })
})
