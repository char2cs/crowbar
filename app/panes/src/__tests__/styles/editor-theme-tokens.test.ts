import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const css = readFileSync(
  join(dirname(fileURLToPath(import.meta.url)), '../../styles/editor-theme.css'),
  'utf8',
)

function rootValue(name: string): string | null {
  const matches = [...css.matchAll(new RegExp(`${name}:\\s*([^;]+);`, 'g'))]
  return matches.length === 0 ? null : matches[matches.length - 1][1].trim()
}

describe('editor-theme.css font tokens', () => {
  it('defaults --editor-font-family to the bundled Geist Mono Variable', () => {
    expect(rootValue('--editor-font-family')).toContain('Geist Mono Variable')
  })

  it('aliases the branch-review diff font to the editor font', () => {
    expect(rootValue('--diffs-font-family')).toBe('var(--editor-font-family)')
  })

  it('defines a header font for the diff renderer', () => {
    expect(rootValue('--diffs-header-font-family')).toBeTruthy()
  })

  it('never references the dropped JetBrains Mono font', () => {
    expect(css).not.toMatch(/jetbrains/i)
  })
})
