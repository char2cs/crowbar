export interface FlickerStrip {
  markup: string
  frames: number
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

const num = (n: number) => String(Number(n.toFixed(4)))

const escapeAttr = (value: string) =>
  value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/"/g, '&quot;')

const serialize = (attrs: Attrs) =>
  attrs.map(([name, value]) => ` ${name}="${escapeAttr(value)}"`).join('')

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

    const a = attrsOf(animate[1])
    if (get(a, 'attributeName') !== 'fill-opacity') return null
    if (get(a, 'calcMode') !== 'discrete') return null
    if (get(a, 'repeatCount') !== 'indefinite') return null

    const dur = get(a, 'dur')
    const values = get(a, 'values')?.split(';')
    if (!dur || !values || values.length === 0) return null

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
  const stripAttrs = withAttr(
    withAttr(rootAttrs, 'viewBox', stripViewBox),
    'class',
    'size-full',
  ).filter(([name]) => name !== 'width' && name !== 'height')

  return { markup: `<svg${serialize(stripAttrs)}>${body}</svg>`, frames, duration }
}
