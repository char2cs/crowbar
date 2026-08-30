import { describe, expect, it, beforeEach } from 'vitest'
import {
  CARD_BOTTOM_INSET_VAR,
  DEFAULT_CARD_HEIGHT_FRACTION,
  MIN_CARD_HEIGHT_FRACTION,
  MAX_CARD_HEIGHT_FRACTION,
  clampCardHeightFraction,
  loadCardHeightFraction,
  saveCardHeightFraction,
} from '@/components/layout/sidebar-card-height'

const STORAGE_KEY = 'sidebar-card-height-fraction'

describe('sidebar-card-height', () => {
  beforeEach(() => {
    localStorage.removeItem(STORAGE_KEY)
  })

  describe('clampCardHeightFraction', () => {
    it('leaves an in-range fraction untouched', () => {
      expect(clampCardHeightFraction(0.5)).toBe(0.5)
    })

    it('clamps below the minimum', () => {
      expect(clampCardHeightFraction(0)).toBe(MIN_CARD_HEIGHT_FRACTION)
      expect(clampCardHeightFraction(-1)).toBe(MIN_CARD_HEIGHT_FRACTION)
    })

    it('clamps above the maximum', () => {
      expect(clampCardHeightFraction(1)).toBe(MAX_CARD_HEIGHT_FRACTION)
      expect(clampCardHeightFraction(5)).toBe(MAX_CARD_HEIGHT_FRACTION)
    })
  })

  describe('loadCardHeightFraction', () => {
    it('returns the spec default (one third) with nothing persisted', () => {
      expect(loadCardHeightFraction()).toBe(DEFAULT_CARD_HEIGHT_FRACTION)
    })

    it('returns a previously saved fraction', () => {
      saveCardHeightFraction(0.5)
      expect(loadCardHeightFraction()).toBe(0.5)
    })

    it('falls back to the default on a corrupt/non-numeric stored value', () => {
      localStorage.setItem(STORAGE_KEY, 'not-a-number')
      expect(loadCardHeightFraction()).toBe(DEFAULT_CARD_HEIGHT_FRACTION)
    })

    it('clamps an out-of-range stored value read back from a previous session', () => {
      localStorage.setItem(STORAGE_KEY, '2')
      expect(loadCardHeightFraction()).toBe(MAX_CARD_HEIGHT_FRACTION)
    })
  })

  describe('saveCardHeightFraction', () => {
    it('persists a clamped value, not the raw input', () => {
      saveCardHeightFraction(10)
      expect(loadCardHeightFraction()).toBe(MAX_CARD_HEIGHT_FRACTION)
    })
  })

  it('exports one shared CSS custom property name for the writer and the reader', () => {
    expect(CARD_BOTTOM_INSET_VAR).toBe('--card-bottom-inset')
  })
})
