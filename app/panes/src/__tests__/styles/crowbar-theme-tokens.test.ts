import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

// See theme-tokens.test.ts: derive the directory from import.meta.url first, then
// join — vite rewrites `new URL('<literal>', import.meta.url)` into an asset URL
// that jsdom resolves to http://localhost/..., which fileURLToPath rejects.
const here = dirname(fileURLToPath(import.meta.url))
const themeCss = readFileSync(join(here, '../../styles/theme.css'), 'utf8')
const crowbarCss = readFileSync(join(here, '../../styles/crowbar.css'), 'utf8')

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
const baseDarkIos = declarations(themeCss, /^\.dark\[data-platform='ios'\]\s*\{/m)
const crowbarControls = declarations(crowbarCss, /^\[data-theme='crowbar'\]\s*\{/m)
const crowbarLightChrome = declarations(
  crowbarCss,
  /^\[data-theme='crowbar'\]:not\(\.dark\):not\(\[data-platform='ios'\]\)\s*\{/m,
)

const crowbarLinuxLight = declarations(
  crowbarCss,
  /^\[data-theme='crowbar'\]\[data-platform='linux'\]:not\(\.dark\)\s*\{/m,
)

const crowbarIosSidebarPage = declarations(
  crowbarCss,
  /^\[data-theme='crowbar'\]\[data-platform='ios'\]\[data-chrome-page\]:not\(\.dark\)\s*\{/m,
)

/** Every selector of the rule that holds the dark palette (`.dark` and its light-mode twins). */
const darkPaletteSelectors = (
  themeCss.replace(/\/\*[\s\S]*?\*\//g, '').match(/([^{}]*)\{\s*--background: #1c1c1a;/)?.[1] ?? ''
)
  .split(',')
  .map((selector) => selector.trim())

describe('crowbar.css theme', () => {
  it('pins the raised controls to the light-variant white surface in both modes', () => {
    expect(crowbarControls['--control-surface']).toBe(baseLight['--background'])
    expect(crowbarControls['--control-foreground']).toBe(baseLight['--foreground'])
  })

  it('paints the light-mode chrome with the dark theme sidebar wash', () => {
    expect(crowbarLightChrome['--sidebar']).toBe(baseDark['--sidebar'])
    expect(crowbarLightChrome['--chrome-bg']).toBe(baseDark['--chrome-bg'])
  })

  it('lets a touch of the wash through the light chat pane only', () => {
    expect(crowbarLightChrome['--pane-chrome-bg']).toBe(
      'color-mix(in oklch, var(--pane-background) 95%, transparent)',
    )
    expect(crowbarLightChrome['--pane-background']).toBeUndefined()
    expect(crowbarControls['--pane-background']).toBeUndefined()
  })

  it('flattens the light chat pane fill on Linux, which has no vibrancy to show through it', () => {
    expect(crowbarLinuxLight['--chrome-bg']).toBeDefined()
    expect(crowbarLinuxLight['--pane-chrome-bg']).toBe('var(--pane-background)')
  })

  it('makes the light active pane a 2px brand ring against a recessive inactive edge', () => {
    expect(baseLight['--pane-accent']).toBe('var(--secondary)')
    expect(baseLight['--pane-accent-width']).toBe('1px')
    expect(baseLight['--pane-border']).toBe('var(--border)')
    expect(baseDark['--pane-accent']).toBeUndefined()
    expect(crowbarLightChrome['--pane-accent']).toBe('var(--brand)')
    expect(crowbarLightChrome['--pane-accent-width']).toBe('2px')
    expect(crowbarLightChrome['--pane-border']).toBe('oklch(0.55 0 0)')
  })

  it('paints the phone sidebars dark in light mode with the desktop sidebar wash, opaque as iOS has no vibrancy', () => {
    expect(crowbarIosSidebarPage['--sidebar']).toBe(baseDark['--sidebar'])
    expect(crowbarIosSidebarPage['--chrome-bg']).toBe(baseDarkIos['--chrome-bg'])
  })

  it('gives chrome-surface regions and the phone sidebar pages the dark palette in light mode, on iOS too', () => {
    expect(darkPaletteSelectors).toContain("[data-theme='crowbar']:not(.dark) .chrome-surface")
    expect(darkPaletteSelectors).toContain("[data-theme='crowbar']:not(.dark)[data-chrome-page]")
    expect(darkPaletteSelectors.some((selector) => selector.includes("'ios'"))).toBe(false)
  })
})
