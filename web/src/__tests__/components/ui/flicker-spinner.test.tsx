import { render } from '@testing-library/react'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { buildFlickerStrip } from '@/components/ui/flicker-strip'

// Not `new URL(rel, import.meta.url)`: Vite statically rewrites that pattern
// into an asset-glob lookup, which yields undefined for source files.
const HERE = dirname(fileURLToPath(import.meta.url))
const read = (relativeToSrc: string) =>
  readFileSync(resolve(HERE, '..', '..', '..', relativeToSrc), 'utf-8')

const ASSETS = import.meta.glob('/src/components/ui/spinners/*.svg', {
  eager: true,
  query: '?raw',
  import: 'default',
}) as Record<string, string>

describe('FlickerSpinner', () => {
  it('inlines a sprite strip whose dots inherit currentColor', () => {
    const { container } = render(<FlickerSpinner />)
    const svg = container.querySelector('svg')
    expect(svg).not.toBeNull()
    expect(svg!.querySelector('[fill="currentColor"]')).not.toBeNull()
    // The timeline is baked into frames, so no SMIL survives into the DOM —
    // that is the entire point of the strip.
    expect(svg!.querySelector('animate')).toBeNull()
  })

  it('plays the strip with a stepped transform, one step per frame', () => {
    const { container } = render(<FlickerSpinner />)
    const strip = container.querySelector<HTMLElement>('[style*="flicker-strip"]')
    expect(strip).not.toBeNull()

    // Recover which asset was picked so the numbers can be checked against it.
    const frames = Number(strip!.style.width.replace('%', '')) / 100
    expect(Number.isInteger(frames)).toBe(true)
    expect(frames).toBeGreaterThan(1)

    const baked = Object.values(ASSETS)
      .map((source) => buildFlickerStrip(source))
      .find((s) => s && s.frames === frames && strip!.style.animation.includes(s.duration))
    expect(baked, 'the rendered strip must correspond to a real asset').toBeTruthy()

    // steps(N) over a -100% translate of an N-frame-wide strip = exactly one
    // frame per step. Any other pairing shears the animation off its frames.
    expect(strip!.style.animation).toContain(`steps(${frames})`)
    expect(strip!.style.animation).toContain('infinite')
    expect(strip!.style.willChange).toBe('transform')
  })

  it('clips the strip to a single frame', () => {
    const { getByRole } = render(<FlickerSpinner />)
    // Without overflow-hidden every frame of the strip would be on screen at
    // once, which reads as a smear rather than a spinner.
    expect(getByRole('status').className).toContain('overflow-hidden')
  })

  // The FlickerSpinner picks its markup at RANDOM per mount, so any assertion
  // on a rendered spinner only samples ONE of the assets. These two guards turn
  // "someone drops a differently-shaped .svg into the folder" from a flake on
  // ~1 mount in N into a deterministic failure that names the offending file.
  it('EVERY spinner asset animates and uses currentColor', () => {
    const paths = Object.keys(ASSETS)
    expect(paths.length).toBeGreaterThan(0)
    for (const path of paths) {
      expect(ASSETS[path], `${path} must contain an <animate> element`).toContain('<animate')
      expect(ASSETS[path], `${path} must paint with currentColor`).toContain('currentColor')
    }
  })

  it('EVERY spinner asset can be baked into a strip', () => {
    // An asset the baker refuses still renders — it falls back to its own SMIL
    // — but it silently reintroduces the main-thread cost the strip exists to
    // remove, so it must fail loudly here instead.
    for (const path of Object.keys(ASSETS)) {
      expect(buildFlickerStrip(ASSETS[path]), `${path} must be bakeable`).not.toBeNull()
    }
  })

  it('exposes a status role and honours className sizing', () => {
    const { getByRole } = render(<FlickerSpinner className="size-3.5" />)
    expect(getByRole('status').className).toContain('size-3.5')
  })

  it('has an accessible loading label', () => {
    const { getByRole } = render(<FlickerSpinner />)
    expect(getByRole('status')).toHaveAttribute('aria-label', 'Loading')
  })

  it('marks itself for call sites to assert on', () => {
    const { getByRole } = render(<FlickerSpinner />)
    expect(getByRole('status')).toHaveAttribute('data-flicker-spinner')
  })

  it('renders the frames of a real captured spinner, in order', () => {
    const { container } = render(<FlickerSpinner />)
    const rendered = container.querySelector('svg')!.outerHTML

    const knownOuterHtml = new Set(
      Object.values(ASSETS).map((source) => {
        // Through the same jsdom innerHTML pipeline the component uses, so
        // serialization differences (attribute order, self-closing tags) can't
        // cause a false negative.
        const probe = document.createElement('div')
        probe.innerHTML = buildFlickerStrip(source)!.markup
        return probe.querySelector('svg')!.outerHTML
      }),
    )

    expect(knownOuterHtml.has(rendered)).toBe(true)
  })

  describe('the @keyframes it depends on', () => {
    const css = read('index.css')

    it('is declared in index.css under the name the component composes', () => {
      expect(css).toContain('@keyframes flicker-strip')
      expect(read('components/ui/flicker-spinner.tsx')).toContain('`flicker-strip ${')
    })

    it('is declared unlayered, where Tailwind cannot tree-shake it', () => {
      // Tailwind v4 emits @theme keyframes only when the matching --animate-*
      // utility is used. This animation is composed inline (duration and step
      // count differ per asset), so declared inside @theme it would be dropped
      // from the production stylesheet and the spinner would sit motionless in
      // packaged builds ONLY — invisible to every test and to `bun dev`.
      const withoutComments = css.replace(/\/\*[\s\S]*?\*\//g, '')
      const at = withoutComments.indexOf('@keyframes flicker-strip')
      expect(at).toBeGreaterThan(-1)
      const before = withoutComments.slice(0, at)
      const depth = (before.match(/{/g) ?? []).length - (before.match(/}/g) ?? []).length
      expect(depth, '@keyframes flicker-strip must not be nested in a block').toBe(0)
    })
  })
})
