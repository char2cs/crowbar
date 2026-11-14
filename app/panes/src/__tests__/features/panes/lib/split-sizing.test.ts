import { describe, expect, it } from 'vitest'
import { clampFirstPx, percentsToPx, pxToPercents } from '@/features/panes/lib/split-sizing'

describe('pxToPercents', () => {
  it('produces a 50/50 split for a centered divider', () => {
    expect(pxToPercents(1000, 500, 100)).toEqual([50, 50])
  })

  it('sums to exactly 100', () => {
    const [a, b] = pxToPercents(1000, 333, 100)
    expect(a + b).toBe(100)
  })

  it('clamps the first pane to the minimum when dragged too small', () => {
    // min 100px of 1000px container = 10%
    const [a, b] = pxToPercents(1000, 0, 100)
    expect(a).toBe(10)
    expect(b).toBe(90)
  })

  it('clamps the second pane to the minimum when first dragged too large', () => {
    const [a, b] = pxToPercents(1000, 1000, 100)
    expect(a).toBe(90)
    expect(b).toBe(10)
    expect(a + b).toBe(100)
  })

  it('handles a degenerate (zero) container without NaN', () => {
    const [a, b] = pxToPercents(0, 0, 100)
    expect(a + b).toBe(100)
    expect(Number.isFinite(a)).toBe(true)
    expect(Number.isFinite(b)).toBe(true)
  })

  it('handles a container smaller than 2x min without producing negatives', () => {
    const [a, b] = pxToPercents(100, 50, 100)
    expect(a + b).toBe(100)
    expect(a).toBeGreaterThanOrEqual(0)
    expect(b).toBeGreaterThanOrEqual(0)
  })
})

describe('percentsToPx', () => {
  it('converts a 50/50 split to half the container each', () => {
    expect(percentsToPx(1000, [50, 50])).toEqual([500, 500])
  })

  it('sums to the container size (no lost pixels from rounding)', () => {
    const [a, b] = percentsToPx(1000, [33.3333, 66.6667])
    expect(a + b).toBe(1000)
  })

  it('handles a 70/30 split', () => {
    expect(percentsToPx(1000, [70, 30])).toEqual([700, 300])
  })
})

describe('clampFirstPx', () => {
  it('keeps a value within [minPx, containerPx - minPx]', () => {
    expect(clampFirstPx(500, 1000, 100)).toBe(500)
  })

  it('clamps below the minimum up to minPx', () => {
    expect(clampFirstPx(10, 1000, 100)).toBe(100)
  })

  it('clamps above the maximum down to containerPx - minPx', () => {
    expect(clampFirstPx(990, 1000, 100)).toBe(900)
  })

  it('collapses to the midpoint when container is smaller than 2x min', () => {
    expect(clampFirstPx(10, 100, 100)).toBe(50)
  })
})
