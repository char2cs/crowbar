import { describe, expect, it } from 'vitest'
import { percentsToPx, pxToPercents } from '@/features/panes/lib/split-sizing'

describe('pxToPercents', () => {
  it('produces a 50/50 split for a centered divider', () => {
    expect(pxToPercents(1000, 500, 100)).toEqual([50, 50])
  })

  it('sums to exactly 100', () => {
    const [a, b] = pxToPercents(1000, 333, 100)
    expect(a + b).toBe(100)
  })

  it('clamps the first pane to the minimum when dragged too small', () => {
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

  it('centres the divider when the container is smaller than 2x min', () => {
    expect(pxToPercents(100, 10, 100)).toEqual([50, 50])
  })

  it('keeps an in-range drag where it was dropped', () => {
    expect(pxToPercents(1000, 250, 100)).toEqual([25, 75])
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
