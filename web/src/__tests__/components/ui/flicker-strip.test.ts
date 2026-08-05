import { describe, expect, it } from 'vitest'
import { buildFlickerStrip } from '@/components/ui/flicker-strip'

/**
 * The strip builder bakes a flip-dot spinner's discrete SMIL timeline into a
 * static N-frame sprite so the animation can run as ONE compositor-driven
 * `transform: translateX()` instead of 25 main-thread SMIL timers.
 *
 * The correctness bar is exact visual equality, so the assertions here are
 * frame-by-frame reconstructions rather than shape checks: for every frame the
 * strip must carry the same dots, at the same radius, at the same opacity the
 * SMIL animation would be showing at that step.
 *
 * The parser under test is regex-based; this suite reads its output with
 * DOMParser instead, so a shared misreading of the markup can't make both
 * sides agree.
 */

const svg = (body: string, viewBox = '0 0 30 30') =>
  `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${viewBox}" fill="currentColor">${body}</svg>`

const dot = (cx: number, cy: number, values: string | null, dur = '0.72s', opacity = '0.15') =>
  values === null
    ? `<circle cx="${cx}" cy="${cy}" r="2" fill="currentColor" fill-opacity="${opacity}"/>`
    : `<circle cx="${cx}" cy="${cy}" r="2" fill="currentColor" fill-opacity="${opacity}">` +
      `<animate attributeName="fill-opacity" values="${values}" dur="${dur}" calcMode="discrete" repeatCount="indefinite"/>` +
      `</circle>`

const parse = (markup: string) => {
  const doc = new DOMParser().parseFromString(markup, 'image/svg+xml')
  const root = doc.documentElement
  expect(root.querySelector('parsererror')).toBeNull()
  return root
}

/** Every circle in `markup`, as comparable plain records. */
const dotsOf = (root: Element) =>
  Array.from(root.querySelectorAll('circle')).map((c) => ({
    cx: Number(c.getAttribute('cx')),
    cy: Number(c.getAttribute('cy')),
    r: Number(c.getAttribute('r')),
    fill: c.getAttribute('fill'),
    opacity: c.getAttribute('fill-opacity'),
  }))

describe('buildFlickerStrip', () => {
  it('lays the frames out horizontally and drops the SMIL entirely', () => {
    const strip = buildFlickerStrip(svg(dot(3, 3, '1;0.15;1;0.15')))

    expect(strip).not.toBeNull()
    expect(strip!.frames).toBe(4)
    expect(strip!.duration).toBe('0.72s')
    expect(strip!.markup).not.toContain('<animate')

    const root = parse(strip!.markup)
    // Four frames of a 30-wide source, one row tall.
    expect(root.getAttribute('viewBox')).toBe('0 0 120 30')
    expect(dotsOf(root)).toEqual([
      { cx: 3, cy: 3, r: 2, fill: 'currentColor', opacity: '1' },
      { cx: 33, cy: 3, r: 2, fill: 'currentColor', opacity: '0.15' },
      { cx: 63, cy: 3, r: 2, fill: 'currentColor', opacity: '1' },
      { cx: 93, cy: 3, r: 2, fill: 'currentColor', opacity: '0.15' },
    ])
  })

  it('emits every dot in every frame, so no dot is ever painted twice', () => {
    // Two dots, one of which never changes. Both must appear in all 2 frames:
    // layering an opaque dot over a translucent one would darken its
    // antialiased edge, which is a visible difference at this size.
    const strip = buildFlickerStrip(svg(dot(3, 3, '1;0.15') + dot(9, 3, null)))

    const dots = dotsOf(parse(strip!.markup))
    expect(dots).toHaveLength(4)
    expect(dots).toEqual([
      { cx: 3, cy: 3, r: 2, fill: 'currentColor', opacity: '1' },
      { cx: 9, cy: 3, r: 2, fill: 'currentColor', opacity: '0.15' },
      { cx: 33, cy: 3, r: 2, fill: 'currentColor', opacity: '0.15' },
      { cx: 39, cy: 3, r: 2, fill: 'currentColor', opacity: '0.15' },
    ])
  })

  it('carries the source root attributes over so currentColor still resolves', () => {
    const root = parse(buildFlickerStrip(svg(dot(3, 3, '1;0.15')))!.markup)
    expect(root.getAttribute('fill')).toBe('currentColor')
  })

  it('is deterministic', () => {
    const source = svg(dot(3, 3, '1;0.15;1') + dot(9, 9, '0.15;1;0.15'))
    expect(buildFlickerStrip(source)).toEqual(buildFlickerStrip(source))
  })

  describe('refuses to bake anything it cannot reproduce exactly', () => {
    // Each of these returns null so the caller falls back to inlining the
    // original SMIL asset — a slower spinner, never a wrong one.
    it('rejects interpolated (non-discrete) animation', () => {
      const source = svg(dot(3, 3, '1;0.15')).replace('calcMode="discrete"', 'calcMode="linear"')
      expect(buildFlickerStrip(source)).toBeNull()
    })

    it('rejects an animated attribute other than fill-opacity', () => {
      const source = svg(dot(3, 3, '1;0.15')).replace(
        'attributeName="fill-opacity"',
        'attributeName="r"',
      )
      expect(buildFlickerStrip(source)).toBeNull()
    })

    it('rejects a finite repeatCount', () => {
      const source = svg(dot(3, 3, '1;0.15')).replace('repeatCount="indefinite"', 'repeatCount="3"')
      expect(buildFlickerStrip(source)).toBeNull()
    })

    it('rejects mixed durations', () => {
      expect(buildFlickerStrip(svg(dot(3, 3, '1;0.15') + dot(9, 3, '1;0.15', '1.08s')))).toBeNull()
    })

    it('rejects mixed frame counts', () => {
      expect(buildFlickerStrip(svg(dot(3, 3, '1;0.15') + dot(9, 3, '1;0.15;1')))).toBeNull()
    })

    it('rejects markup with nothing animated', () => {
      expect(buildFlickerStrip(svg(dot(3, 3, null)))).toBeNull()
    })

    it('rejects an unreadable viewBox', () => {
      expect(buildFlickerStrip(svg(dot(3, 3, '1;0.15'), 'nonsense'))).toBeNull()
    })
  })

  /**
   * The exhaustive check. The spinner set is small and fully enumerable — 31
   * assets, 4-18 frames each — so this proves equality for every frame the user
   * can ever see, rather than sampling.
   */
  describe('over the real spinner assets', () => {
    const assets = import.meta.glob('/src/components/ui/spinners/*.svg', {
      eager: true,
      query: '?raw',
      import: 'default',
    }) as Record<string, string>

    const paths = Object.keys(assets).sort()

    it('finds the asset set', () => {
      expect(paths.length).toBeGreaterThan(0)
    })

    it.each(paths)('%s reproduces every frame exactly', (path) => {
      const source = parse(assets[path])
      const sourceDots = Array.from(source.querySelectorAll('circle'))
      const width = Number(source.getAttribute('viewBox')!.split(/\s+/)[2])

      const strip = buildFlickerStrip(assets[path])
      expect(strip, `${path} must be bakeable`).not.toBeNull()

      // What SMIL shows at step f, derived straight from the source document.
      const expected: ReturnType<typeof dotsOf> = []
      for (let f = 0; f < strip!.frames; f++) {
        for (const c of sourceDots) {
          const animate = c.querySelector('animate')
          const values = animate?.getAttribute('values')?.split(';')
          expected.push({
            cx: Number(c.getAttribute('cx')) + f * width,
            cy: Number(c.getAttribute('cy')),
            r: Number(c.getAttribute('r')),
            fill: c.getAttribute('fill'),
            opacity: values ? values[f] : c.getAttribute('fill-opacity'),
          })
        }
      }

      expect(dotsOf(parse(strip!.markup))).toEqual(expected)
    })

    it.each(paths)('%s keeps its loop duration', (path) => {
      const dur = assets[path].match(/dur="([^"]*)"/)![1]
      expect(buildFlickerStrip(assets[path])!.duration).toBe(dur)
    })
  })
})
