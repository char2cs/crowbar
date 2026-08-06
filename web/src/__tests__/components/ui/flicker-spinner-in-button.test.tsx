import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Button } from '@/components/ui/button'
import { buttonVariants } from '@/components/ui/button-variants'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { buildFlickerStrip } from '@/components/ui/flicker-strip'

// The predicate the themed hosts in components/ui actually write. Button, Tabs,
// Select, Badge, Toolbar and the menus all size descendant icons with
// `[&_svg:not([class*='size-'])]:size-4`, so an svg that satisfies this selector
// is one those rules leave alone. Asserting `.matches()` on it tests the real
// cascade hook rather than a class string we chose.
const HOST_SIZING_OPT_OUT = '[class*="size-"]'

/**
 * Regression: the context pill renders the agent spinner inside a <Button>, and
 * the Button's descendant-icon sizing crushed the N-frames-wide sprite strip
 * into a 16px square. The animation then translated the strip clean out of its
 * frame window, so the pill showed no glyph at all while an agent was working —
 * everywhere else (plain spans) the spinner was fine.
 */
describe('FlickerSpinner inside a themed host', () => {
  it('carries the size opt-out the host sizing rules key off', () => {
    const { container } = render(
      <Button>
        <FlickerSpinner />
      </Button>,
    )
    const svg = container.querySelector('[data-flicker-spinner] svg')
    expect(svg).not.toBeNull()
    expect(svg!.matches(HOST_SIZING_OPT_OUT)).toBe(true)
  })

  it('bakes the opt-out into the strip markup itself, not the call site', () => {
    // The strip is injected as raw markup, so the class has to come off the
    // baker — a wrapper class cannot reach inside dangerouslySetInnerHTML.
    const strip = buildFlickerStrip(
      '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 30 30" fill="currentColor">' +
        '<circle cx="15" cy="15" r="3" fill-opacity="0.15">' +
        '<animate attributeName="fill-opacity" values="0.15;1;0.15" dur="0.18s" ' +
        'calcMode="discrete" repeatCount="indefinite"/>' +
        '</circle></svg>',
    )
    expect(strip).not.toBeNull()
    const el = document.createElement('div')
    el.innerHTML = strip!.markup
    expect(el.firstElementChild!.matches(HOST_SIZING_OPT_OUT)).toBe(true)
  })

  it('keeps the strip stretched rather than square', () => {
    // What the Button was overriding. A strip is `frames` viewBox-widths wide,
    // so a square svg is the bug's signature.
    const { container } = render(<FlickerSpinner />)
    const svg = container.querySelector('[data-flicker-spinner] svg')!
    const [, , w, h] = svg.getAttribute('viewBox')!.split(/\s+/).map(Number)
    expect(w).toBeGreaterThan(h)
    // and it must not re-introduce the intrinsic sizing the CSS box replaces
    expect(svg.getAttribute('width')).toBeNull()
    expect(svg.getAttribute('height')).toBeNull()
  })

  it('cancels the unconditional horizontal margin those hosts also apply', () => {
    // Control: this half of the host rule has no :not() escape hatch, which is
    // why the strip overrides it instead of opting out. If Button ever drops
    // the pull, this assertion is what says the override went stale.
    expect(buttonVariants()).toContain('[&_svg]:-mx-0.5')
    const { container } = render(<FlickerSpinner />)
    const strip = container.querySelector('[data-flicker-spinner] > span')!
    expect(strip.className).toContain('[&>svg]:mx-0!')
  })
})
