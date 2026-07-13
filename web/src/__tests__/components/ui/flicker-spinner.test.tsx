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

  // The FlickerSpinner picks its markup at RANDOM per mount, so the assertions above (and
  // the identical ones in workspace-branch-icon / project-home-row / context-pill /
  // workspace-tree-repo-home) only sample ONE of the assets. They pass today because every
  // asset happens to animate and use currentColor — an invariant NOTHING enforces. Drop one
  // static or hard-coded-colour spinner into the folder and those five files start failing
  // on ~1 mount in N, with no code change to blame. This guard turns that latent flake into
  // a deterministic failure that names the offending asset.
  it('EVERY spinner asset animates and uses currentColor (the sampled tests depend on it)', () => {
    const assets = import.meta.glob('../../../components/ui/spinners/*.svg', {
      eager: true,
      query: '?raw',
      import: 'default',
    }) as Record<string, string>
    const paths = Object.keys(assets)
    expect(paths.length).toBeGreaterThan(0)
    for (const path of paths) {
      const markup = assets[path]
      expect(markup, `${path} must contain an <animate> element`).toContain('<animate')
      expect(markup, `${path} must paint with currentColor`).toContain('currentColor')
    }
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
