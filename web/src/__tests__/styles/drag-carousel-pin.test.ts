/**
 * Guards the compositing hint on the drag-time carousel pin in src/index.css.
 *
 * `html[data-row-dragging] [data-sidebar-carousel]` drops the carousel from
 * `overflow-x: scroll` to `hidden` so a row carried out of the sidebar cannot
 * scroll a whole panel away. The catch is that `overflow-x: scroll` is ALSO what
 * earns the carousel its composited scrolling layer: pinning it takes the
 * promotion away, the 315x976 sidebar and its ~295 descendants fall back onto
 * the page layer, and every per-frame write the drag makes (drop line, row
 * classes, ghost) repaints it on the main thread.
 *
 * Measured on the live app, one pointermove per frame over 125 frames:
 *
 *   idle .......................................  8.3 ms/frame
 *   dragging, pin as written ................... 28-39 ms/frame
 *   dragging, pin rule deleted .................  8.3 ms/frame
 *   dragging, pin KEPT + will-change ...........  8.3 ms/frame
 *
 * `will-change` reads like a stray optimisation hint and is exactly the kind of
 * line a cleanup deletes, so this pins it. It is not a permanent hint: the same
 * attribute scopes it to the drag, and the layer exists anyway whenever the
 * carousel is its normal scrollable self.
 *
 * Source-level on purpose — jsdom has no layout, no compositing and no frames,
 * so nothing about this is observable by rendering.
 */
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const HERE = dirname(fileURLToPath(import.meta.url))
const css = readFileSync(resolve(HERE, '..', '..', 'index.css'), 'utf-8')

/** The rule's body, so assertions cannot be satisfied by an unrelated block. */
function pinRuleBody(): string {
  const match = css.match(/html\[data-row-dragging\]\s+\[data-sidebar-carousel\]\s*\{([^}]*)\}/)
  expect(match, 'the drag-time carousel pin rule must exist').not.toBeNull()
  return match![1]
}

describe('drag-time carousel pin', () => {
  it('still pins the carousel while a row is in the air', () => {
    expect(pinRuleBody()).toMatch(/overflow-x:\s*hidden/)
  })

  it('keeps the carousel on its own layer, or the pin costs 4x the frame budget', () => {
    expect(pinRuleBody()).toMatch(/will-change:\s*transform/)
  })

  it('pins with `hidden` rather than `clip`, which would break programmatic scroll', () => {
    // `clip` is not a scroll container at all, so the carousel's own
    // ResizeObserver re-align and its activeTab scrollTo would stop working
    // mid-drag. Verified live: scrollLeft is still settable through a drag.
    expect(pinRuleBody()).not.toMatch(/overflow-x:\s*clip/)
  })
})
