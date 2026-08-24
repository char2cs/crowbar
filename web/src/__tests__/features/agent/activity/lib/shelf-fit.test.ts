import { describe, expect, it } from 'vitest'

import {
  DENSE_THRESHOLD,
  fitShelf,
  formatElapsed,
  type ShelfToken,
} from '@/features/agent/activity/lib/shelf-fit'

const tokens = (n: number): ShelfToken[] =>
  Array.from({ length: n }, (_, i) => ({ id: `s${i}`, agentType: 'Explore', elapsed: i * 10 }))

/** Every token the same width, so the arithmetic is the thing under test. */
const fixed = (width: number) => () => width

describe('fitShelf', () => {
  it('shows everything at three in flight, names and all', () => {
    const layout = fitShelf(tokens(3), 600, fixed(60))
    expect(layout.shown).toHaveLength(3)
    expect(layout.overflow).toBe(0)
    expect(layout.dense).toBe(false)
  })

  // Names go first, at five.
  it('drops the names past the dense threshold', () => {
    expect(fitShelf(tokens(DENSE_THRESHOLD), 600, fixed(60)).dense).toBe(false)
    expect(fitShelf(tokens(DENSE_THRESHOLD + 1), 600, fixed(60)).dense).toBe(true)
  })

  it('spills whole tokens into a counter once the row is full', () => {
    // 200px of room, 30 reserved for the counter → 170 / 64 = 2 tokens fit.
    const layout = fitShelf(tokens(9), 200, fixed(60))
    expect(layout.shown).toHaveLength(2)
    expect(layout.overflow).toBe(7)
    expect(layout.shown.length + layout.overflow).toBe(9)
  })

  // The count and the clocks are all Crowbar is told, so they never shed.
  it('never renders a bare counter with nothing beside it', () => {
    const layout = fitShelf(tokens(9), 40, fixed(400))
    expect(layout.shown).toHaveLength(1)
    expect(layout.overflow).toBe(8)
  })

  it('shows everything before layout rather than collapsing for a frame', () => {
    const layout = fitShelf(tokens(9), 0, fixed(60))
    expect(layout.shown).toHaveLength(9)
    expect(layout.overflow).toBe(0)
  })

  it('is empty for no subagents', () => {
    expect(fitShelf([], 600, fixed(60))).toEqual({ shown: [], overflow: 0, dense: false })
  })
})

describe('formatElapsed', () => {
  it('reads like a stopwatch, because subagents run past a minute', () => {
    expect(formatElapsed(0)).toBe('0:00')
    expect(formatElapsed(9)).toBe('0:09')
    expect(formatElapsed(75)).toBe('1:15')
    expect(formatElapsed(3600)).toBe('60:00')
  })

  it('never renders a negative clock from a skewed timestamp', () => {
    expect(formatElapsed(-5)).toBe('0:00')
  })
})
