import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it, vi } from 'vitest'
import { cssColorToHex, readSyntaxPalette } from '@/features/editor/theme/resolve-css-color'

const css = readFileSync(
  join(dirname(fileURLToPath(import.meta.url)), '../../styles/theme.css'),
  'utf8',
)

const SYNTAX_TOKEN_KEYS = (() => {
  const style = { getPropertyValue: () => '#ffffff' } as unknown as CSSStyleDeclaration
  const spy = vi.spyOn(window, 'getComputedStyle').mockReturnValue(style)
  try {
    return Object.keys(readSyntaxPalette())
  } finally {
    spy.mockRestore()
  }
})()

function relLuminance(hex: string): number {
  const h = hex.replace('#', '').slice(0, 6)
  const chan = [0, 2, 4].map((i) => {
    const c = Number.parseInt(h.slice(i, i + 2), 16) / 255
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * chan[0] + 0.7152 * chan[1] + 0.0722 * chan[2]
}

function contrast(a: string, b: string): number {
  const la = relLuminance(a)
  const lb = relLuminance(b)
  const [hi, lo] = la > lb ? [la, lb] : [lb, la]
  return (hi + 0.05) / (lo + 0.05)
}

const DARK_BLOCK_START = css.search(/^\.dark\s*\{/m)

function darkValue(name: string): string | null {
  const darkBlock = css.slice(DARK_BLOCK_START)
  const matches = [...darkBlock.matchAll(new RegExp(`${name}:\\s*([^;]+);`, 'g'))]
  if (matches.length === 0) {
    return null
  }
  const raw = matches[matches.length - 1][1].trim()
  const aliasMatch = raw.match(/^var\((--syntax-[\w-]+)\)$/)
  if (aliasMatch) {
    return darkValue(aliasMatch[1])
  }
  return raw
}

function lightValue(name: string): string | null {
  const rootBlock = css.slice(0, DARK_BLOCK_START)
  const matches = [...rootBlock.matchAll(new RegExp(`${name}:\\s*([^;]+);`, 'g'))]
  if (matches.length === 0) {
    return null
  }
  return matches[matches.length - 1][1].trim()
}

const BOUND = new Set([
  'comment',
  'variable',
  'punctuation',
  'operator',
  'error',
  'markdown-strikethrough',
  'markdown-quote',
])
const dedicated = SYNTAX_TOKEN_KEYS.filter((key) => !BOUND.has(key))

describe('theme.css syntax tokens', () => {
  it('declares every syntax token the renderers reference', () => {
    for (const key of SYNTAX_TOKEN_KEYS) {
      expect(css.includes(`--syntax-${key}:`)).toBe(true)
    }
  })

  it('dark dedicated syntax hues clear WCAG AA against --background', () => {
    const bgRaw = darkValue('--background')
    expect(bgRaw).toBeTruthy()
    const bg = cssColorToHex(bgRaw as string)
    expect(bg).toBeTruthy()

    for (const key of dedicated) {
      const raw = darkValue(`--syntax-${key}`)
      expect(raw, `--syntax-${key} missing in .dark`).toBeTruthy()
      const hex = cssColorToHex(raw as string)
      expect(hex, `--syntax-${key} unparseable`).toBeTruthy()
      const ratio = contrast(hex as string, bg as string)
      expect(ratio, `--syntax-${key} contrast ${ratio.toFixed(2)} < 4.5`).toBeGreaterThanOrEqual(
        4.5,
      )
    }
  })

  it('light dedicated syntax hues clear WCAG AA against light --background', () => {
    const bg = '#ffffff'
    for (const key of dedicated) {
      const raw = lightValue(`--syntax-${key}`)
      expect(raw, `--syntax-${key} missing in :root`).toBeTruthy()
      const hex = cssColorToHex(raw as string)
      expect(hex, `--syntax-${key} unparseable`).toBeTruthy()
      const ratio = contrast(hex as string, bg)
      expect(ratio, `--syntax-${key} contrast ${ratio.toFixed(2)} < 4.5`).toBeGreaterThanOrEqual(
        4.5,
      )
    }
  })
})

describe('theme.css destructive fill', () => {
  it('carries its button text at WCAG AA in both themes', () => {
    const light = contrast(
      '#ffffff',
      cssColorToHex(lightValue('--destructive') as string) as string,
    )
    const dark = contrast(
      cssColorToHex(darkValue('--destructive-foreground') as string) as string,
      cssColorToHex(darkValue('--destructive') as string) as string,
    )

    expect(light, `light white on --destructive ${light.toFixed(2)} < 4.5`).toBeGreaterThanOrEqual(
      4.5,
    )
    expect(dark, `dark ink on --destructive ${dark.toFixed(2)} < 4.5`).toBeGreaterThanOrEqual(4.5)
  })
})
