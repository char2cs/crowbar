import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'

describe('FlickerSpinner', () => {
  it('inlines a flip-dot SVG so currentColor + <animate> work', () => {
    const { container } = render(<FlickerSpinner />)
    const svg = container.querySelector('svg')
    expect(svg).not.toBeNull()
    // Inlined (not an <img>): currentColor dots + an <animate> element are present.
    expect(svg!.querySelector('animate')).not.toBeNull()
    expect(svg!.querySelector('[fill="currentColor"]')).not.toBeNull()
  })

  it('exposes a status role and honours className sizing', () => {
    const { getByRole } = render(<FlickerSpinner className="size-3.5" />)
    const el = getByRole('status')
    expect(el.className).toContain('size-3.5')
  })

  it('has an accessible loading label', () => {
    const { getByRole } = render(<FlickerSpinner />)
    expect(getByRole('status')).toHaveAttribute('aria-label', 'Loading')
  })

  it('picks the inlined markup from the real captured spinner set', () => {
    const spinners = import.meta.glob('/src/components/ui/spinners/*.svg', {
      eager: true,
      query: '?raw',
      import: 'default',
    }) as Record<string, string>

    // Parse every source .svg through the same jsdom innerHTML pipeline the
    // component uses, so serialization differences (attribute order, self
    // closing tags) can't cause a false negative when comparing outerHTML.
    const knownOuterHtml = new Set(
      Object.values(spinners).map((markup) => {
        const probe = document.createElement('div')
        probe.innerHTML = markup
        return probe.querySelector('svg')!.outerHTML
      }),
    )

    const { container } = render(<FlickerSpinner />)
    const svgOuterHtml = container.querySelector('svg')!.outerHTML
    expect(knownOuterHtml.has(svgOuterHtml)).toBe(true)
  })
})
