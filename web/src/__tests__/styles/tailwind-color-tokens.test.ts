import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

/**
 * Guards the failure mode that made Cmd+A look broken in the rich markdown
 * editor: a component styled with `bg-brand/[.13]` against a `--color-brand`
 * theme entry the app never declared.
 *
 * Tailwind v4 resolves a colour utility from a `--color-*` entry in `@theme`.
 * When the entry is missing it does not warn or fall back — it emits NO RULE AT
 * ALL, so the class lands on the element and paints nothing. `BlockSelection`
 * (components/ui/block-selection.tsx) is the whole visual of a block selection,
 * so Cmd+A selected every block and highlighted none of them. Three more
 * registry components copied from platejs.org had the same dead token: the
 * table drop line, the selected table cell, and a selected equation.
 *
 * The check keys off the OPACITY MODIFIER. `bg-x/50` only parses if `x` is a
 * colour — no non-colour utility takes a `/`, which is what keeps this free of
 * the false positives a bare `bg-*`/`text-*` scan drowns in (`border-b`,
 * `text-left`, `ring-offset-2` …). Sized scales are still excluded because
 * `text-sm/6` is the font-size + line-height shorthand, not a colour.
 */

// See theme-tokens.test.ts for why the directory is derived from import.meta.url
// rather than `new URL(...)`: vite asset-rewrites the literal form under jsdom.
const HERE = dirname(fileURLToPath(import.meta.url))
const WEB_ROOT = join(HERE, '../../..')
const SRC = join(WEB_ROOT, 'src')

const read = (p: string) => readFileSync(join(WEB_ROOT, p), 'utf8')

/** `--color-<name>:` entries declared by a stylesheet. */
function colorTokens(css: string): string[] {
  return [...css.matchAll(/--color-([a-z0-9-]+)\s*:/g)].map((m) => m[1])
}

/**
 * Namespaces whose values share the utility prefixes below but are NOT colours
 * — `text-sm`, `shadow-lg`, `leading-none`. They reach this scan through the
 * slash shorthands (`text-sm/6`, `shadow-md/20`), so they have to be spared.
 */
function sizedScales(css: string): string[] {
  return [
    ...css.matchAll(/--(?:text|shadow|radius|blur|drop-shadow|leading|tracking)-([a-z0-9-]+)\s*:/g),
  ].map((m) => m[1])
}

const appCss = [read('src/index.css'), read('src/styles/theme.css')].join('\n')
const tailwindCss = read('node_modules/tailwindcss/theme.css')

const KNOWN = new Set([
  ...colorTokens(appCss),
  ...colorTokens(tailwindCss),
  ...sizedScales(tailwindCss),
  // CSS-wide colour keywords Tailwind maps directly, with no theme entry.
  'inherit',
  'current',
  'transparent',
  'white',
  'black',
])

const COLOR_PREFIXES = [
  'bg',
  'text',
  'border',
  'ring',
  'fill',
  'stroke',
  'outline',
  'caret',
  'accent',
  'decoration',
  'divide',
  'shadow',
  'from',
  'via',
  'to',
].join('|')

/**
 * `<prefix>-<token>/` — the trailing slash is the discriminator. The leading
 * boundary keeps an import path (`from '@/components/…'`) from reading as a
 * `from-components/…` utility.
 */
const UTILITY = new RegExp(
  String.raw`(?:^|[\s"'\`:[({])(?:${COLOR_PREFIXES})-([a-z][a-z0-9]*(?:-[a-z0-9]+)*)/`,
  'gm',
)

function sourceFiles(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) {
      if (entry === '__tests__' || entry === 'node_modules') continue
      sourceFiles(full, out)
    } else if (/\.tsx?$/.test(entry)) {
      out.push(full)
    }
  }
  return out
}

describe('tailwind colour tokens', () => {
  it('declares every colour token a source file styles against', () => {
    const unresolved = new Map<string, string[]>()

    for (const file of sourceFiles(SRC)) {
      for (const match of readFileSync(file, 'utf8').matchAll(UTILITY)) {
        const token = match[1]
        if (KNOWN.has(token)) continue
        const where = unresolved.get(token) ?? []
        where.push(relative(WEB_ROOT, file))
        unresolved.set(token, where)
      }
    }

    // Named individually so a failure reads as "which token, used where" rather
    // than as a bare count.
    expect(
      Object.fromEntries([...unresolved].map(([token, files]) => [token, files.sort()])),
    ).toEqual({})
  })

  it('gives --brand a value in both themes', () => {
    // The registry components apply their own alpha (`/[.13]`, `/5`, `/15`,
    // `/50`), so the token itself is opaque and each theme needs its own
    // lightness — a single value legible on white is not legible on the dark
    // surface at 13%.
    const theme = read('src/styles/theme.css')
    const darkStart = theme.search(/^\.dark\s*\{/m)
    expect(darkStart).toBeGreaterThan(-1)
    expect(theme.slice(0, darkStart)).toMatch(/^\s*--brand:\s*[^;\s][^;]*;/m)
    expect(theme.slice(darkStart)).toMatch(/^\s*--brand:\s*[^;\s][^;]*;/m)
  })
})
