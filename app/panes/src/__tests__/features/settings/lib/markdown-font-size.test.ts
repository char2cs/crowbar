import { describe, expect, it } from 'vitest'
import {
  MARKDOWN_FONT_SIZE_DEFAULT,
  MARKDOWN_FONT_SIZE_MAX,
  MARKDOWN_FONT_SIZE_MIN,
  normalizeMarkdownFontSize,
} from '@/features/settings/lib/markdown-font-size'

describe('normalizeMarkdownFontSize', () => {
  it('keeps an in-range whole pixel value untouched', () => {
    expect(normalizeMarkdownFontSize(18)).toBe(18)
  })

  it('defaults to the size the stylesheet used to hardcode', () => {
    // `1rem` with no root font-size override — the default must not change what
    // an existing user sees.
    expect(MARKDOWN_FONT_SIZE_DEFAULT).toBe(16)
  })

  it('clamps below the minimum and above the maximum', () => {
    expect(normalizeMarkdownFontSize(2)).toBe(MARKDOWN_FONT_SIZE_MIN)
    expect(normalizeMarkdownFontSize(999)).toBe(MARKDOWN_FONT_SIZE_MAX)
  })

  it('snaps fractional input to whole pixels', () => {
    expect(normalizeMarkdownFontSize(17.4)).toBe(17)
    expect(normalizeMarkdownFontSize(17.6)).toBe(18)
  })

  it('parses a numeric string, as a persisted or typed value can be', () => {
    expect(normalizeMarkdownFontSize('20')).toBe(20)
  })

  it('falls back to the default for anything non-finite', () => {
    for (const bad of [NaN, Infinity, null, undefined, 'huge', {}, []]) {
      expect(normalizeMarkdownFontSize(bad)).toBe(MARKDOWN_FONT_SIZE_DEFAULT)
    }
  })
})
