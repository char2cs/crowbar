import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const WEB_ROOT = join(HERE, '../../..')
const SRC = join(WEB_ROOT, 'src')

const read = (p: string) => readFileSync(join(WEB_ROOT, p), 'utf8')

function colorTokens(css: string): string[] {
  return [...css.matchAll(/--color-([a-z0-9-]+)\s*:/g)].map((m) => m[1])
}

function sizedScales(css: string): string[] {
  return [
    ...css.matchAll(/--(?:text|shadow|radius|blur|drop-shadow|leading|tracking)-([a-z0-9-]+)\s*:/g),
  ].map((m) => m[1])
}

const indexCss = read('src/index.css')
const localSheets = [...indexCss.matchAll(/@import\s+'\.\/([^']+\.css)'/g)].map(
  (m) => `src/${m[1]}`,
)
const appCss = [indexCss, ...localSheets.map(read)].join('\n')
const tailwindCss = read('node_modules/tailwindcss/theme.css')

const KNOWN = new Set([
  ...colorTokens(appCss),
  ...colorTokens(tailwindCss),
  ...sizedScales(tailwindCss),
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

    expect(
      Object.fromEntries([...unresolved].map(([token, files]) => [token, files.sort()])),
    ).toEqual({})
  })

  it('gives --brand a value in both themes', () => {
    const theme = read('src/styles/theme.css')
    const darkStart = theme.search(/^\.dark\s*\{/m)
    expect(darkStart).toBeGreaterThan(-1)
    expect(theme.slice(0, darkStart)).toMatch(/^\s*--brand:\s*[^;\s][^;]*;/m)
    expect(theme.slice(darkStart)).toMatch(/^\s*--brand:\s*[^;\s][^;]*;/m)
  })
})
