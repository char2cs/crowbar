import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { cssColorToHex, SYNTAX_TOKEN_KEYS } from '@/features/editor/theme/resolve-css-color'

// NOTE: vite statically rewrites `new URL('<literal>', import.meta.url)` into its
// asset-URL helper, which under jsdom resolves to a `http://localhost/...` URL and
// makes fileURLToPath throw "The URL must be of scheme file". Derive the directory
// from import.meta.url first (not asset-rewritten), then join — same on-disk read.
const css = readFileSync(
  join(dirname(fileURLToPath(import.meta.url)), '../../styles/theme.css'),
  'utf8',
)

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

/** Pull the dark-block value of a CSS var (last definition wins for .dark). */
function darkValue(name: string): string | null {
  const darkBlock = css.slice(css.indexOf('.dark'))
  const matches = [...darkBlock.matchAll(new RegExp(`${name}:\\s*([^;]+);`, 'g'))]
  if (matches.length === 0) {
    // bound aliases live only in :root — resolve one hop for known aliases below
    return null
  }
  return matches[matches.length - 1][1].trim()
}

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

    // Bound/aliased tokens intentionally inherit shadcn semantics (muted-foreground /
    // destructive) and are defined only in :root, so they have no dark-block hex to test.
    // Everything else is a dedicated hue and MUST clear AA — derive from the shared key
    // list so a newly added hue is covered automatically (the whole point of this guard).
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
    for (const key of dedicated) {
      const raw = darkValue(`--syntax-${key}`)
      expect(raw, `--syntax-${key} missing in .dark`).toBeTruthy()
      const hex = cssColorToHex(raw as string)
      expect(hex, `--syntax-${key} unparseable`).toBeTruthy()
      const ratio = contrast(hex as string, bg as string)
      expect(ratio, `--syntax-${key} contrast ${ratio.toFixed(2)} < 4.5`).toBeGreaterThanOrEqual(4.5)
    }
  })
})
