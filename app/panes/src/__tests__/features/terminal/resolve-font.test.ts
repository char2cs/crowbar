import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resolveTerminalFont } from '@/features/terminal/utils/resolve-font'

describe('resolveTerminalFont', () => {
  let available: Set<string>

  beforeEach(() => {
    available = new Set()
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

  it('keeps the configured font first and adds Nerd Font glyph fallbacks', async () => {
    available.add('Geist Mono Variable')
    const fontFamily = await resolveTerminalFont('Geist Mono Variable', 14)

    expect(fontFamily.startsWith('"Geist Mono Variable",')).toBe(true)
    expect(fontFamily).toContain('"Symbols Nerd Font Mono"')
    expect(fontFamily).toContain('"MesloLGS NF"')
    expect(fontFamily).toMatch(/,\s*monospace$/)
  })

  it('deduplicates existing fallback lists without quoting CSS generic families', async () => {
    available.add('Geist Mono Variable')
    const fontFamily = await resolveTerminalFont(
      '"Geist Mono Variable", "Symbols Nerd Font Mono", monospace',
      14,
    )

    expect(fontFamily.match(/"Symbols Nerd Font Mono"/g)).toHaveLength(1)
    expect(fontFamily).toContain('"Geist Mono Variable"')
    expect(fontFamily).toMatch(/,\s*monospace$/)
    expect(fontFamily).not.toContain('"monospace"')
  })

  it('keeps a variable font as-is instead of mapping to its static cut', async () => {
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
    const result = await resolveTerminalFont('Nonexistent Font Variable', 14)

    expect(result).toMatch(/,\s*monospace$/)
  })
})
