import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  SYNTAX_TOKEN_KEYS,
  TERMINAL_ANSI_KEYS,
  cssColorToHex,
  readSyntaxPalette,
  readTerminalPalette,
  resolveCssVar,
} from '@/features/editor/theme/resolve-css-color'

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
    expect(hex).not.toBeNull()
    const r = Number.parseInt((hex as string).slice(1, 3), 16)
    const g = Number.parseInt((hex as string).slice(3, 5), 16)
    const b = Number.parseInt((hex as string).slice(5, 7), 16)
    expect(r).toBeGreaterThan(250) // strong red channel
    expect(g).toBeLessThan(20) // green/blue near zero
    expect(b).toBeLessThan(20)
  })

  it('passes through 8-digit hex (rrggbbaa)', () => {
    expect(cssColorToHex('#aabbccdd')).toBe('#aabbccdd')
  })

  it('converts space-separated rgb', () => {
    expect(cssColorToHex('rgb(255 0 0)')).toBe('#ff0000')
  })

  it('returns null for unparseable input', () => {
    expect(cssColorToHex('')).toBeNull()
    expect(cssColorToHex('not-a-color')).toBeNull()
  })
})

describe('DOM resolver', () => {
  beforeEach(() => {
    // Map every CSS var this suite asks for to a known oklch value.
    vi.spyOn(window, 'getComputedStyle').mockReturnValue({
      getPropertyValue: (name: string) => (name.startsWith('--') ? 'oklch(1 0 0)' : ''),
    } as unknown as CSSStyleDeclaration)
  })
  afterEach(() => vi.restoreAllMocks())

  it('resolveCssVar converts the computed value to hex', () => {
    expect(resolveCssVar('--syntax-keyword')).toBe('#ffffff')
  })

  it('resolveCssVar returns null for an unset var', () => {
    vi.spyOn(window, 'getComputedStyle').mockReturnValue({
      getPropertyValue: () => '',
    } as unknown as CSSStyleDeclaration)
    expect(resolveCssVar('--nope')).toBeNull()
  })

  it('resolveCssVar resolves var() chains via a temporary element', () => {
    vi.spyOn(window, 'getComputedStyle').mockImplementation((el) => {
      if (el === document.documentElement) {
        return {
          getPropertyValue: (name: string) =>
            name === '--syntax-comment' ? 'var(--muted-foreground)' : '',
        } as unknown as CSSStyleDeclaration
      }
      // Called on the temporary span — simulate browser resolving the chain.
      return { color: 'rgb(105, 105, 105)', getPropertyValue: () => '' } as unknown as CSSStyleDeclaration
    })
    const appendSpy = vi.spyOn(document.body, 'appendChild').mockImplementation((n) => n as never)
    const removeSpy = vi.spyOn(document.body, 'removeChild').mockImplementation((n) => n as never)
    expect(resolveCssVar('--syntax-comment')).toBe('#696969')
    expect(appendSpy).toHaveBeenCalledOnce()
    expect(removeSpy).toHaveBeenCalledOnce()
  })

  it('readSyntaxPalette returns a hex for every syntax key', () => {
    const palette = readSyntaxPalette()
    for (const key of SYNTAX_TOKEN_KEYS) {
      expect(palette[key]).toBe('#ffffff')
    }
  })

  it('readTerminalPalette returns a hex for every ANSI key', () => {
    const palette = readTerminalPalette()
    for (const key of TERMINAL_ANSI_KEYS) {
      expect(palette[key]).toBe('#ffffff')
    }
  })
})
