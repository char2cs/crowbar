import { describe, expect, it } from 'vitest'
import { planWindow } from '@/features/git/lib/patch-window'
import type { WindowInput } from '@/features/git/lib/patch-window'

const MAX_MATERIALIZED_FILES = 40

function makePaths(n: number, prefix = 'f'): string[] {
  return Array.from({ length: n }, (_, i) => `${prefix}${i}`)
}

function uniformCounts(paths: readonly string[], lines: number): Record<string, number> {
  return Object.fromEntries(paths.map((p) => [p, lines]))
}

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

function applyPlan(materialized: readonly string[], plan: { fetch: string[]; evict: string[] }) {
  const next = new Set(materialized)
  for (const p of plan.evict) next.delete(p)
  for (const p of plan.fetch) next.add(p)
  return [...next]
}

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

  it('does not evict-then-refetch a file oscillating at the lookahead boundary', () => {
    const paths = makePaths(100)

    const first = planWindow(input({ paths, visible: { first: 30, last: 35 } }))
    expect(first.fetch).toContain('f24')
    const materialized = applyPlan([], first)

    const second = planWindow(input({ paths, visible: { first: 31, last: 36 }, materialized }))
    expect(second.evict).toEqual([])
    expect(second.fetch).toEqual(['f42'])

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
        lineCounts: uniformCounts(paths, 100),
      }),
    )

    const survivors = applyPlan(paths, plan)
    expect(survivors).toHaveLength(MAX_MATERIALIZED_FILES)
    expect(new Set(survivors)).toEqual(new Set(paths.slice(11, 51)))
    expect(plan.evict[0]).toBe('f0')
  })
})

describe('planWindow — MAX_MATERIALIZED_LINES', () => {
  it('bites even when the file count is far under MAX_MATERIALIZED_FILES', () => {
    const paths = ['a.ts', 'b.ts', 'c.ts', 'd.ts']
    const plan = planWindow({
      visible: { first: 0, last: 0 },
      total: 4,
      materialized: paths,
      paths,
      lineCounts: uniformCounts(paths, 20_000),
    })

    expect(paths.length).toBeLessThan(MAX_MATERIALIZED_FILES)
    expect(plan.evict).toEqual(['d.ts'])
  })

  it('does not re-propose a file the line budget cannot afford', () => {
    const paths = ['a.ts', 'b.ts', 'c.ts', 'd.ts']
    const lineCounts = uniformCounts(paths, 20_000)

    const plan = planWindow({
      visible: { first: 0, last: 0 },
      total: paths.length,
      materialized: ['a.ts', 'b.ts', 'c.ts'],
      paths,
      lineCounts,
    })

    expect(plan.fetch).toEqual([])
  })

  it('counts a not-yet-fetched file against the budget before fetching it', () => {
    const paths = ['a.ts', 'b.ts', 'c.ts', 'd.ts', 'e.ts']
    const plan = planWindow({
      visible: { first: 0, last: 0 },
      total: paths.length,
      materialized: [],
      paths,
      lineCounts: uniformCounts(paths, 20_000),
    })

    expect(plan.fetch[0]).toBe('a.ts')
    expect(plan.fetch.length).toBeLessThan(paths.length)
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

describe('planWindow — a file larger than the entire budget', () => {
  it('still fetches a visible file whose ± count dwarfs MAX_MATERIALIZED_LINES', () => {
    const paths = ['monster.ts', 'next.ts']
    const plan = planWindow({
      visible: { first: 0, last: 0 },
      total: paths.length,
      materialized: [],
      paths,
      lineCounts: { 'monster.ts': 420_000, 'next.ts': 1_000 },
    })

    expect(plan.fetch).toContain('monster.ts')
  })

  it('never evicts that file while it is on screen', () => {
    const paths = ['monster.ts']
    const plan = planWindow({
      visible: { first: 0, last: 0 },
      total: 1,
      materialized: ['monster.ts'],
      paths,
      lineCounts: { 'monster.ts': 420_000 },
    })

    expect(plan.evict).toEqual([])
  })

  it('budgets it at the capped cost, so it does not starve its neighbours', () => {
    const paths = ['m1.ts', 'm2.ts', 's1.ts', 's2.ts']
    const plan = planWindow({
      visible: { first: 0, last: 1 },
      total: paths.length,
      materialized: [],
      paths,
      lineCounts: { 'm1.ts': 420_000, 'm2.ts': 420_000, 's1.ts': 500, 's2.ts': 500 },
    })

    expect(plan.fetch).toEqual(expect.arrayContaining(['m1.ts', 'm2.ts', 's1.ts', 's2.ts']))
  })
})

describe('planWindow with viewed files', () => {
  it('never fetches a viewed file, even one on screen, and still fetches its neighbours', () => {
    const paths = makePaths(5)
    const plan = planWindow(
      input({ paths, visible: { first: 0, last: 2 }, viewed: new Set(['f1']) }),
    )

    expect(plan.fetch).toEqual(['f0', 'f2', 'f3', 'f4'])
  })

  it('evicts a viewed file that is held, wherever it is', () => {
    const paths = makePaths(5)
    const plan = planWindow(
      input({
        paths,
        visible: { first: 0, last: 0 },
        materialized: ['f0', 'f1'],
        viewed: new Set(['f0']),
      }),
    )

    expect(plan.evict).toEqual(['f0'])
    expect(plan.fetch).not.toContain('f0')
  })

  it('converges: applying the plan and re-planning changes nothing', () => {
    const paths = makePaths(30)
    const viewed = new Set(['f2', 'f9'])
    const first = planWindow(input({ paths, materialized: ['f2'], viewed }))
    const held = applyPlan(['f2'], first)
    expect(held).not.toContain('f2')
    expect(planWindow(input({ paths, materialized: held, viewed }))).toEqual({
      fetch: [],
      evict: [],
    })
  })
})
