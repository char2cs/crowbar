import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

// See theme-tokens.test.ts for why the directory is derived from
// import.meta.url before joining rather than via a vite-rewritten URL.
const css = readFileSync(
  join(dirname(fileURLToPath(import.meta.url)), '../../styles/editor-theme.css'),
  'utf8',
)

/** Pull a :root-block value of a CSS var (last definition wins). */
function rootValue(name: string): string | null {
  const matches = [...css.matchAll(new RegExp(`${name}:\\s*([^;]+);`, 'g'))]
  return matches.length === 0 ? null : matches[matches.length - 1][1].trim()
}

describe('editor-theme.css font tokens', () => {
  it('defaults --editor-font-family to the bundled Geist Mono Variable', () => {
    expect(rootValue('--editor-font-family')).toContain('Geist Mono Variable')
  })

  it('aliases the branch-review diff font to the editor font', () => {
    // Regression: @pierre/diffs (the branch-review diff renderer) reads its
    // code font from `--diffs-font-family` on its host element, which the app
    // never set — so the diff view silently fell back to the library's own
    // hardcoded "SF Mono" stack instead of the user's configured font, no
    // matter what Settings said. Aliasing it to `--editor-font-family` (the
    // same variable Monaco consumes) is what makes a font-setting change
    // affect both surfaces.
    expect(rootValue('--diffs-font-family')).toBe('var(--editor-font-family)')
  })

  it('defines a header font for the diff renderer', () => {
    expect(rootValue('--diffs-header-font-family')).toBeTruthy()
  })

  it('never references the dropped JetBrains Mono font', () => {
    expect(css).not.toMatch(/jetbrains/i)
  })
})
