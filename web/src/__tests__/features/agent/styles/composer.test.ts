import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

/**
 * Regression coverage for the empty chat document's (`.docwrap`/`.doc`,
 * agent-empty-document.tsx) two typographic bugs: no horizontal scrollbar on
 * overflow, and a font-size that no longer matches the transcript's rendered
 * turn output.
 *
 * These assert against the raw stylesheet source rather than jsdom — jsdom
 * does not run layout, so there is nothing to measure a real scrollbar or
 * computed font-size against. Same technique as tailwind-color-tokens.test.ts.
 */

const HERE = dirname(fileURLToPath(import.meta.url))
const WEB_ROOT = join(HERE, '../../../../..')
const read = (p: string) => readFileSync(join(WEB_ROOT, p), 'utf8')

const composerCss = read('src/features/agent/styles/composer.css')
const transcriptCss = read('src/features/agent/styles/transcript.css')

/**
 * The declarations a selector ends up with once every same-specificity rule
 * matching it has applied, in source order — i.e. plain CSS cascade, last
 * declaration for a given property wins. `composer.css` declares
 * `.agent-chat .doc {}` TWICE (a live rule and a retired `.sheet`-era one
 * further down); this is what caught that a fix landed only in the first one
 * never actually reached the screen.
 */
function cascadedDeclarations(css: string, selector: string): Record<string, string> {
  const escaped = selector.replace(/[.]/g, '\\.')
  const pattern = new RegExp(`${escaped}\\s*\\{([^}]*)\\}`, 'g')
  const declarations: Record<string, string> = {}
  for (const match of css.matchAll(pattern)) {
    for (const decl of match[1].matchAll(/([a-z-]+)\s*:\s*([^;]+);/g)) {
      const [, prop, value] = decl
      if (prop === 'overflow') {
        // Shorthand — expands to both longhands, whichever order they were
        // set in by an earlier declaration.
        declarations['overflow-x'] = value.trim()
        declarations['overflow-y'] = value.trim()
      } else {
        declarations[prop] = value.trim()
      }
    }
  }
  return declarations
}

describe('empty chat document — overflow-x', () => {
  it('scrolls content wider than the pane instead of clipping it', () => {
    const doc = cascadedDeclarations(composerCss, '.agent-chat .doc')
    expect(doc['overflow-x']).toBe('auto')
  })

  // REGRESSION: a retired `.sheet`-placeholder rule shares the exact same
  // `.agent-chat .doc` selector and sits LATER in the file. Its `overflow:
  // hidden` had equal specificity and later source order, so it silently
  // beat the live rule's own `overflow-x` for the one real `.doc` element on
  // screen — adding `overflow-x: auto` to the live rule alone did nothing.
  it('is not clobbered by the retired .sheet-era .doc rule further down the file', () => {
    const occurrences = [...composerCss.matchAll(/\.agent-chat \.doc\s*\{/g)]
    expect(occurrences.length).toBeGreaterThan(1)
    expect(composerCss).not.toMatch(/\.agent-chat \.doc\s*\{[^}]*overflow\s*:\s*hidden/)
  })
})

describe('empty chat document — font size', () => {
  it('matches the transcript turn output size exactly', () => {
    const doc = cascadedDeclarations(composerCss, '.agent-chat .doc')
    const agentProse = cascadedDeclarations(transcriptCss, '.agent-chat .agent-prose')

    expect(doc['font-size']).toBe(agentProse['font-size'])
  })

  // REGRESSION: `.doc` used to sit at 16px, a deliberate "drafting vs.
  // reading" size difference from the transcript — that call is now
  // overridden, so 16px must not come back.
  it('is no longer the old 16px drafting size', () => {
    const doc = cascadedDeclarations(composerCss, '.agent-chat .doc')
    expect(doc['font-size']).not.toBe('16px')
  })
})
