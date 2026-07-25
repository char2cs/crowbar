import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

// See theme-tokens.test.ts: derive the directory from import.meta.url first, then
// join — vite rewrites `new URL('<literal>', import.meta.url)` into an asset URL
// that jsdom resolves to http://localhost/..., which fileURLToPath rejects.
const here = dirname(fileURLToPath(import.meta.url))
const themeCss = readFileSync(join(here, '../../styles/theme.css'), 'utf8')
const zenCss = readFileSync(join(here, '../../styles/zen.css'), 'utf8')

/** Custom-property declarations inside the first rule whose selector matches. */
function declarations(css: string, selector: RegExp): Record<string, string> {
  const match = css.match(selector)
  if (!match || match.index === undefined) {
    throw new Error(`selector ${selector} not found`)
  }

  const open = css.indexOf('{', match.index)
  let depth = 0
  let end = open
  for (let i = open; i < css.length; i++) {
    if (css[i] === '{') depth++
    if (css[i] === '}') {
      depth--
      if (depth === 0) {
        end = i
        break
      }
    }
  }

  const body = css.slice(open + 1, end)
  const out: Record<string, string> = {}
  for (const line of body.split('\n')) {
    const decl = line.match(/^\s*(--[a-z0-9-]+)\s*:\s*(.+?);\s*$/)
    if (decl) out[decl[1]] = decl[2].trim()
  }
  return out
}

const baseLight = declarations(themeCss, /^:root\s*\{/m)
const baseDark = declarations(themeCss, /^\.dark\s*\{/m)
const zenLight = declarations(zenCss, /^\[data-theme='zen'\]:not\(\.dark\)\s*\{/m)
const zenDark = declarations(zenCss, /^\[data-theme='zen'\]\.dark\s*\{/m)

/**
 * Zen is imported AFTER theme.css and its selectors out-specify `:root`/`.dark`,
 * so anything it declares wins. The only reason it exists is to restyle the
 * WINDOW SURFACES in isolation; every other token is meant to be the shared
 * palette. Overriding one is how the WCAG-measured --muted-foreground values got
 * stranded at their pre-fix numbers in Zen while theme.css moved on.
 */
const SURFACE_TOKENS = new Set(['--background', '--pane-background', '--chrome-bg'])

describe('zen.css palette', () => {
  it('does not strand a semantic token at a value the base palette moved off', () => {
    const drift: string[] = []
    for (const [mode, base, zen] of [
      ['light', baseLight, zenLight],
      ['dark', baseDark, zenDark],
    ] as const) {
      for (const [token, value] of Object.entries(zen)) {
        if (SURFACE_TOKENS.has(token)) continue
        if (base[token] !== undefined && base[token] !== value) {
          drift.push(`${mode} ${token}: base ${base[token]} vs zen ${value}`)
        }
      }
    }

    expect(drift).toEqual([])
  })

  it('declares only the surface tokens it means to diverge on', () => {
    const redundant = [
      ...Object.keys(zenLight).filter((t) => !SURFACE_TOKENS.has(t)),
      ...Object.keys(zenDark).filter((t) => !SURFACE_TOKENS.has(t)),
    ]

    expect(redundant).toEqual([])
  })

  it('keeps overriding the surfaces that are the point of the theme', () => {
    for (const token of SURFACE_TOKENS) {
      expect(zenLight[token], `${token} missing from Zen light`).toBeTruthy()
      expect(zenDark[token], `${token} missing from Zen dark`).toBeTruthy()
    }
  })
})
