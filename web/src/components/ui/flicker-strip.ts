/**
 * Bakes a flip-dot spinner's discrete SMIL timeline into a static sprite strip.
 *
 * WHY THIS EXISTS. The captured spinners animate 25 dots with 25 SMIL
 * `<animate calcMode="discrete">` timers apiece. WebKit runs SMIL on the main
 * thread, and every discrete step re-serialises the animated attribute
 * (`Style::Extractor::propertyValueSerialization`) and dirties the render tree,
 * which forces `PlatformCALayerRemote::recursiveBuildTransaction` to rebuild
 * the WHOLE layer tree for the CoreAnimation commit. The cost of one 16px
 * spinner is therefore proportional to the size of the entire app — which is
 * why a single spinner visibly slows the whole window.
 *
 * There is very little information in these animations: each dot is one of two
 * opacities, the step rate is a uniform 11.1/s, and a loop is 4-18 frames. So
 * the timeline can be flattened into a horizontal strip of pre-composed frames
 * and played by translating that strip one frame-width at a time. A stepped
 * `transform` animation is compositable, so playback costs no main-thread work
 * at all, no style recalc, and no layer-tree rebuild — and it keeps running
 * when the window isn't focused, because compositor animations are driven
 * independently of rAF and of focus.
 *
 * EXACTNESS IS THE CONTRACT. Every dot is emitted in every frame at its own
 * opacity. The tempting optimisation — paint the dim grid once underneath and
 * only put lit dots in the strip — is wrong: an opaque dot layered over a
 * translucent one composites its antialiased edge twice, which at a ~1px dot
 * radius visibly thickens the dot. Anything this builder cannot reproduce
 * exactly it refuses to bake (returns null), and the caller falls back to
 * inlining the original SMIL asset. A slower spinner, never a wrong one.
 */

export interface FlickerStrip {
  /** Static `<svg>`: `frames` copies of the source laid out left to right. */
  markup: string
  /** Frames in one loop — the strip is this many source-widths wide. */
  frames: number
  /** Loop duration, verbatim from the source (e.g. `0.72s`). */
  duration: string
}

const ROOT = /^\s*<svg\b([^>]*)>([\s\S]*)<\/svg>\s*$/
const CIRCLE = /<circle\b([^>]*?)(?:\/>|>([\s\S]*?)<\/circle>)/g
const ANIMATE = /<animate\b([^>]*?)\/?>/
const ATTR = /([a-zA-Z:-]+)\s*=\s*"([^"]*)"/g

type Attrs = [name: string, value: string][]

const attrsOf = (source: string): Attrs => {
  const out: Attrs = []
  for (const [, name, value] of source.matchAll(ATTR)) out.push([name, value])
  return out
}

const get = (attrs: Attrs, name: string) => attrs.find(([n]) => n === name)?.[1]

/** Plain integers stay plain; anything else is rounded off float noise. */
const num = (n: number) => String(Number(n.toFixed(4)))

const escapeAttr = (value: string) =>
  value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/"/g, '&quot;')

const serialize = (attrs: Attrs) =>
  attrs.map(([name, value]) => ` ${name}="${escapeAttr(value)}"`).join('')

/** Replace an attribute in place, preserving source order; append if absent. */
const withAttr = (attrs: Attrs, name: string, value: string): Attrs => {
  const index = attrs.findIndex(([n]) => n === name)
  if (index === -1) return [...attrs, [name, value]]
  const next = attrs.slice()
  next[index] = [name, value]
  return next
}

interface Dot {
  attrs: Attrs
  cx: number
  /** One opacity per frame, or null for a dot that never changes. */
  values: string[] | null
}

export function buildFlickerStrip(source: string): FlickerStrip | null {
  const root = ROOT.exec(source)
  if (!root) return null

  const rootAttrs = attrsOf(root[1])
  const viewBox = get(rootAttrs, 'viewBox')
    ?.trim()
    .split(/[\s,]+/)
    .map(Number)
  if (!viewBox || viewBox.length !== 4 || viewBox.some((n) => !Number.isFinite(n))) return null
  const [minX, minY, width, height] = viewBox
  if (width <= 0 || height <= 0) return null

  const dots: Dot[] = []
  let frames = 0
  let duration = ''

  for (const [, rawAttrs, inner] of root[2].matchAll(CIRCLE)) {
    const attrs = attrsOf(rawAttrs)
    const cx = Number(get(attrs, 'cx'))
    if (!Number.isFinite(cx)) return null

    const animate = inner ? ANIMATE.exec(inner) : null
    if (!animate) {
      dots.push({ attrs, cx, values: null })
      continue
    }

    // Only a discrete, indefinitely repeating fill-opacity timeline can be
    // flattened into frames. Interpolated or finite animation, or any other
    // animated attribute, is not something this builder can reproduce.
    const a = attrsOf(animate[1])
    if (get(a, 'attributeName') !== 'fill-opacity') return null
    if (get(a, 'calcMode') !== 'discrete') return null
    if (get(a, 'repeatCount') !== 'indefinite') return null

    const dur = get(a, 'dur')
    const values = get(a, 'values')?.split(';')
    if (!dur || !values || values.length === 0) return null

    // A strip has ONE length and ONE period; mixed timelines would need a
    // least-common-multiple strip, which no asset calls for.
    if (frames === 0) {
      frames = values.length
      duration = dur
    } else if (values.length !== frames || dur !== duration) {
      return null
    }

    dots.push({ attrs, cx, values })
  }

  if (dots.length === 0 || frames === 0) return null

  let body = ''
  for (let frame = 0; frame < frames; frame++) {
    for (const dot of dots) {
      let attrs = withAttr(dot.attrs, 'cx', num(dot.cx + frame * width))
      if (dot.values) attrs = withAttr(attrs, 'fill-opacity', dot.values[frame])
      body += `<circle${serialize(attrs)}/>`
    }
  }

  const stripViewBox = `${num(minX)} ${num(minY)} ${num(width * frames)} ${num(height)}`
  // width/height would fight the CSS box the strip is stretched into.
  const stripAttrs = withAttr(rootAttrs, 'viewBox', stripViewBox).filter(
    ([name]) => name !== 'width' && name !== 'height',
  )

  return { markup: `<svg${serialize(stripAttrs)}>${body}</svg>`, frames, duration }
}
