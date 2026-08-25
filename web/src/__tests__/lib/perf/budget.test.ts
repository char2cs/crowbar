import { describe, expect, it } from 'vitest'
import { checkBudgets, summarize, type PerfMeasure } from '@/lib/perf/budget'
import baseline from '@/lib/perf/perf-baseline.json'

describe('summarize', () => {
  it('reports count, max, and p95 per distinct measure name', () => {
    const measures: PerfMeasure[] = [
      { name: 'chat.scroll.frame', duration: 8 },
      { name: 'chat.scroll.frame', duration: 9 },
      { name: 'chat.scroll.frame', duration: 40 },
      { name: 'chat.open', duration: 120 },
    ]
    const out = summarize(measures)
    expect(out.get('chat.scroll.frame')).toEqual({ count: 3, maxMs: 40, p95Ms: 40 })
    expect(out.get('chat.open')).toEqual({ count: 1, maxMs: 120, p95Ms: 120 })
  })

  it('returns an empty map for no measures', () => {
    expect(summarize([]).size).toBe(0)
  })
})

describe('checkBudgets', () => {
  const budgets = { 'chat.open': 100, 'chat.scroll.frame': 10 }

  it('reports no violations when every measure is within its budget times the tolerance', () => {
    const measures: PerfMeasure[] = [
      { name: 'chat.open', duration: 95 },
      { name: 'chat.scroll.frame', duration: 9 },
    ]
    expect(checkBudgets(measures, budgets)).toEqual([])
  })

  it('flags a measure whose p95 exceeds its budget beyond the tolerance', () => {
    const measures: PerfMeasure[] = [
      { name: 'chat.open', duration: 200 },
    ]
    const violations = checkBudgets(measures, budgets)
    expect(violations).toHaveLength(1)
    expect(violations[0]).toMatchObject({ name: 'chat.open', observedMs: 200, budgetMs: 100 })
    expect(violations[0].overBy).toBeCloseTo(100, 0)
  })

  it('ignores a measure with no budget entry', () => {
    const measures: PerfMeasure[] = [{ name: 'chat.stream.token', duration: 999 }]
    expect(checkBudgets(measures, budgets)).toEqual([])
  })

  it('allows a default 15% tolerance before flagging, and respects a custom tolerance', () => {
    const measures: PerfMeasure[] = [{ name: 'chat.open', duration: 114 }] // +14%, within default 15%
    expect(checkBudgets(measures, budgets)).toEqual([])
    expect(checkBudgets(measures, budgets, 0)).toHaveLength(1) // zero tolerance: any excess flags
  })
})

describe('perf-baseline.json', () => {
  it('has a positive budget for every span this phase instruments', () => {
    for (const span of ['chat.open', 'chat.scroll.frame', 'chat.stream.token']) {
      expect(baseline).toHaveProperty(span)
      expect((baseline as Record<string, number>)[span]).toBeGreaterThan(0)
    }
  })
})
