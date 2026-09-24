import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  buildTerminalFontFamily,
  resolveTerminalFont,
} from '@/features/terminal/utils/resolve-font'

describe('terminal font resolution', () => {
  it('keeps the configured font first and adds Nerd Font glyph fallbacks', () => {
    const fontFamily = buildTerminalFontFamily('Geist Mono Variable')

    expect(fontFamily.startsWith('"Geist Mono Variable",')).toBe(true)
    expect(fontFamily).toContain('"Symbols Nerd Font Mono"')
    expect(fontFamily).toContain('"MesloLGS NF"')
    expect(fontFamily).toMatch(/,\s*monospace$/)
  })

  it('deduplicates existing fallback lists without quoting CSS generic families', () => {
    const fontFamily = buildTerminalFontFamily(
      '"Geist Mono Variable", "Symbols Nerd Font Mono", monospace',
    )

    expect(fontFamily.match(/"Symbols Nerd Font Mono"/g)).toHaveLength(1)
    expect(fontFamily).toContain('"Geist Mono Variable"')
    expect(fontFamily).toMatch(/,\s*monospace$/)
    expect(fontFamily).not.toContain('"monospace"')
  })
})

describe('resolveTerminalFont', () => {
  // available = set of family names whose load()+check() succeed.
  let available: Set<string>

  beforeEach(() => {
    available = new Set()
    // jsdom lacks FontFaceSet; provide a minimal document.fonts.
    Object.defineProperty(document, 'fonts', {
      configurable: true,
      value: {
        load: vi.fn(async () => {}),
        check: (spec: string) => {
          const m = spec.match(/"([^"]+)"/)
          return m ? available.has(m[1]) : false
        },
      },
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('keeps a variable font as-is instead of mapping to its static cut', async () => {
    // Both cuts available: the DOM renderer takes a variable font as-is.
    available.add('Geist Mono')
    available.add('Geist Mono Variable')
    const result = await resolveTerminalFont('Geist Mono Variable', 14)

    expect(result.startsWith('"Geist Mono Variable",')).toBe(true)
  })

  it('resolves a variable font that loads', async () => {
    available.add('Geist Mono Variable')
    const result = await resolveTerminalFont('Geist Mono Variable', 14)

    expect(result.startsWith('"Geist Mono Variable",')).toBe(true)
  })

  it('resolves a non-variable font that loads', async () => {
    available.add('Geist Mono')
    const result = await resolveTerminalFont('Geist Mono', 14)

    expect(result.startsWith('"Geist Mono",')).toBe(true)
  })

  it('uses the platform fallback when nothing loads', async () => {
    // available stays empty
    const result = await resolveTerminalFont('Nonexistent Font Variable', 14)

    expect(result).toMatch(/,\s*monospace$/)
  })
})
