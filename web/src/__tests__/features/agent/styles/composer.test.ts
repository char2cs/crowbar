import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

/**
 * The empty chat document (`.docwrap`/`.doc`, agent-empty-document.tsx) never scrolls
 * horizontally on overflow, and its font-size matches the transcript's rendered turns.
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
  // Escapes every regex metacharacter, not just `.` — a selector with a
  // pseudo-class argument (`:not(:disabled)`) has unescaped parens that used
  // to turn into a capture group instead of literal text, silently matching
  // nothing.
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
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

  // The retired `.sheet` rule reuses the `.agent-chat .doc` selector later in the file; its
  // `overflow: hidden` must not beat the live rule's `overflow-x`.
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

  // No separate 16px "drafting" size: the document reads at the transcript's size.
  it('is no longer the old 16px drafting size', () => {
    const doc = cascadedDeclarations(composerCss, '.agent-chat .doc')
    expect(doc['font-size']).not.toBe('16px')
  })
})

// `.right` (queued count, gauge, Compact) must stay on the baseline row however narrow the dock;
// `.left` absorbs the pressure, else a wrapped `.right` is pushed through the dock's bottom edge.
describe('underbar — .right never leaves the baseline row', () => {
  it('the row itself does not wrap', () => {
    const underbar = cascadedDeclarations(composerCss, '.agent-chat .underbar')
    expect(underbar['flex-wrap']).toBe('nowrap')
  })

  it('.right holds its natural width — it never shrinks or wraps off the row', () => {
    const right = cascadedDeclarations(composerCss, '.agent-chat .underbar .right')
    expect(right['flex']).toBe('none')
  })

  it('.left clips its own overflow instead of growing the row taller', () => {
    const left = cascadedDeclarations(composerCss, '.agent-chat .underbar .left')
    // The helper expands the `overflow` shorthand into both longhands (see
    // its own doc above) rather than keying it as `overflow`.
    expect(left['overflow-x']).toBe('hidden')
    expect(left['overflow-y']).toBe('hidden')
  })
})

// User call, overruling an earlier "always dimly visible" change: Compact
// must stay fully hidden until the gauge is hovered, and then appear ON TOP
// OF the bar itself (`.gstack` stacks `.gbar` and `.gaction` in the same grid
// cell), not beside `.gpct`'s percentage text and not as a second element
// widening the row.
describe('gauge — Compact stays hidden until hover, then overlays the bar', () => {
  it('.gaction is fully hidden at rest', () => {
    const gaction = cascadedDeclarations(composerCss, '.gauge .gaction')
    expect(gaction.opacity).toBe('0')
  })

  it('.gaction becomes visible on hover', () => {
    const hovered = cascadedDeclarations(composerCss, '.gauge.chip:hover:not(:disabled) .gaction')
    expect(Number(hovered.opacity)).toBe(1)
  })

  it('.gbar and .gaction share the same grid cell — the overlay sits on the bar', () => {
    const stack = cascadedDeclarations(composerCss, '.gauge .gstack')
    const cell = cascadedDeclarations(composerCss, '.gauge .gstack > *')
    expect(stack.display).toBe('grid')
    expect(cell['grid-area']).toBe('1 / 1')
  })
})
