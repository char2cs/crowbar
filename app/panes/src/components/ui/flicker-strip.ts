// Bakes a flip-dot spinner's discrete SMIL timeline into a sprite strip. WebKit's main-thread SMIL
// rebuilds the WHOLE layer tree per step, so one spinner slows the window; a stepped transform is
// composited. EXACT or null: every dot in every frame (layering double-composites edges).

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
  // width/height would fight the CSS box. `class="size-full"` opts out of the hosts'
  // `[&_svg:not([class*='size-'])]:size-4` rule, which would crush the N-frame strip to 16px.
  const stripAttrs = withAttr(
    withAttr(rootAttrs, 'viewBox', stripViewBox),
    'class',
    'size-full',
  ).filter(([name]) => name !== 'width' && name !== 'height')

  return { markup: `<svg${serialize(stripAttrs)}>${body}</svg>`, frames, duration }
}
