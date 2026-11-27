import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Button } from '@/components/ui/button'
import { buttonVariants } from '@/components/ui/button-variants'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { buildFlickerStrip } from '@/components/ui/flicker-strip'

const HOST_SIZING_OPT_OUT = '[class*="size-"]'

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
    const { container } = render(<FlickerSpinner />)
    const svg = container.querySelector('[data-flicker-spinner] svg')!
    const [, , w, h] = svg.getAttribute('viewBox')!.split(/\s+/).map(Number)
    expect(w).toBeGreaterThan(h)
    expect(svg.getAttribute('width')).toBeNull()
    expect(svg.getAttribute('height')).toBeNull()
  })

  it('cancels the unconditional horizontal margin those hosts also apply', () => {
    expect(buttonVariants()).toContain('[&_svg]:-mx-0.5')
    const { container } = render(<FlickerSpinner />)
    const strip = container.querySelector('[data-flicker-spinner] > span')!
    expect(strip.className).toContain('[&>svg]:mx-0!')
  })
})
