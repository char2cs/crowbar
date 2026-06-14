import { describe, expect, it } from 'vitest'
import { cssColorToHex } from '@/features/editor/theme/resolve-css-color'

describe('cssColorToHex', () => {
  it('passes through and expands hex', () => {
    expect(cssColorToHex('#aabbcc')).toBe('#aabbcc')
    expect(cssColorToHex('#abc')).toBe('#aabbcc')
    expect(cssColorToHex('  #ABCDEF ')).toBe('#abcdef')
  })

  it('converts rgb/rgba', () => {
    expect(cssColorToHex('rgb(255, 0, 0)')).toBe('#ff0000')
    expect(cssColorToHex('rgba(0, 0, 0, 0.5)')).toBe('#00000080')
  })

  it('converts oklch endpoints exactly', () => {
    expect(cssColorToHex('oklch(1 0 0)')).toBe('#ffffff')
    expect(cssColorToHex('oklch(0 0 0)')).toBe('#000000')
  })

  it('handles oklch alpha', () => {
    expect(cssColorToHex('oklch(1 0 0 / 50%)')).toBe('#ffffff80')
  })

  it('converts a known chromatic oklch within tolerance of sRGB red', () => {
    // oklch for #ff0000 ≈ L0.6279 C0.2577 H29.23
    const hex = cssColorToHex('oklch(0.6279 0.2577 29.23)')
    expect(hex.slice(0, 1)).toBe('#')
    const r = Number.parseInt(hex.slice(1, 3), 16)
    expect(r).toBeGreaterThan(250) // strong red channel
  })

  it('returns null for unparseable input', () => {
    expect(cssColorToHex('')).toBeNull()
    expect(cssColorToHex('not-a-color')).toBeNull()
  })
})
