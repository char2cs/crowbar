import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  buildTerminalFontFamily,
  deriveStaticFontEquivalent,
  resolveTerminalFont,
} from '@/features/terminal/utils/resolve-font'

describe('terminal font resolution', () => {
  it('keeps the configured font first and adds Nerd Font glyph fallbacks', () => {
    const fontFamily = buildTerminalFontFamily('JetBrains Mono Variable')

    expect(fontFamily.startsWith('"JetBrains Mono Variable",')).toBe(true)
    expect(fontFamily).toContain('"Symbols Nerd Font Mono"')
    expect(fontFamily).toContain('"MesloLGS NF"')
    expect(fontFamily).toMatch(/,\s*monospace$/)
  })

  it('deduplicates existing fallback lists without quoting CSS generic families', () => {
    const fontFamily = buildTerminalFontFamily(
      '"JetBrains Mono Variable", "Symbols Nerd Font Mono", monospace',
    )

    expect(fontFamily.match(/"Symbols Nerd Font Mono"/g)).toHaveLength(1)
    expect(fontFamily).toContain('"JetBrains Mono Variable"')
    expect(fontFamily).toMatch(/,\s*monospace$/)
    expect(fontFamily).not.toContain('"monospace"')
  })
})

describe('deriveStaticFontEquivalent', () => {
  it('strips a trailing "Variable" to get the static cut', () => {
    expect(deriveStaticFontEquivalent('JetBrains Mono Variable')).toBe('JetBrains Mono')
    expect(deriveStaticFontEquivalent('IBM Plex Mono Variable')).toBe('IBM Plex Mono')
  })

  it('handles wrapping quotes', () => {
    expect(deriveStaticFontEquivalent('"JetBrains Mono Variable"')).toBe('JetBrains Mono')
  })

  it('is case-insensitive on the "Variable" suffix', () => {
    expect(deriveStaticFontEquivalent('Fira Code variable')).toBe('Fira Code')
  })

  it('returns null when there is no distinct static equivalent', () => {
    expect(deriveStaticFontEquivalent('JetBrains Mono')).toBeNull()
    expect(deriveStaticFontEquivalent('Menlo')).toBeNull()
    expect(deriveStaticFontEquivalent('Variable')).toBeNull()
    expect(deriveStaticFontEquivalent('')).toBeNull()
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

  it('keeps WebGL by resolving a variable font to its available static cut', async () => {
    available.add('JetBrains Mono') // static cut is available
    const result = await resolveTerminalFont('JetBrains Mono Variable', 14)

    expect(result.skipWebGL).toBe(false)
    expect(result.fontFamily.startsWith('"JetBrains Mono",')).toBe(true)
  })

  it('falls back to the variable font (DOM renderer) when no static cut exists', async () => {
    available.add('JetBrains Mono Variable') // only the variable cut is available
    const result = await resolveTerminalFont('JetBrains Mono Variable', 14)

    expect(result.skipWebGL).toBe(true)
    expect(result.fontFamily.startsWith('"JetBrains Mono Variable",')).toBe(true)
  })

  it('keeps WebGL for a non-variable font that loads', async () => {
    available.add('JetBrains Mono')
    const result = await resolveTerminalFont('JetBrains Mono', 14)

    expect(result.skipWebGL).toBe(false)
    expect(result.fontFamily.startsWith('"JetBrains Mono",')).toBe(true)
  })

  it('uses a WebGL-friendly platform fallback when nothing loads', async () => {
    // available stays empty
    const result = await resolveTerminalFont('Nonexistent Font Variable', 14)

    expect(result.skipWebGL).toBe(false)
    expect(result.fontFamily).toMatch(/,\s*monospace$/)
  })
})
