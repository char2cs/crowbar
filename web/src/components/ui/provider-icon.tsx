import DOMPurify, { type DOMPurify as Purifier } from 'dompurify'
import { cn } from '@/utils/cn'

interface ProviderIconProps {
  /** Raw SVG markup from a provider descriptor — untrusted: any paired client can upload one. */
  svg: string
  className?: string
}

const SVG_NS = 'http://www.w3.org/2000/svg'

// Static artwork only: nothing that runs, animates, loads or embeds.
const ALLOWED_TAGS = [
  'svg',
  'g',
  'path',
  'circle',
  'ellipse',
  'line',
  'polyline',
  'polygon',
  'rect',
  'text',
  'tspan',
  'defs',
  'linearGradient',
  'radialGradient',
  'stop',
  'clipPath',
  'mask',
  'title',
  'desc',
]

const FORBID_TAGS = [
  'script',
  'foreignObject',
  'use',
  'animate',
  'set',
  'animateMotion',
  'animateTransform',
  'image',
  'iframe',
  'object',
  'style',
  'a',
  'switch',
]

// No `class`/`style`: the app's own CSS would let an icon restyle the page around it.
const ALLOWED_ATTR = [
  'xmlns',
  'xmlns:xlink',
  'version',
  'id',
  'viewBox',
  'preserveAspectRatio',
  'width',
  'height',
  'x',
  'y',
  'x1',
  'y1',
  'x2',
  'y2',
  'cx',
  'cy',
  'r',
  'rx',
  'ry',
  'fx',
  'fy',
  'd',
  'points',
  'pathLength',
  'transform',
  'opacity',
  'color',
  'fill',
  'fill-opacity',
  'fill-rule',
  'clip-rule',
  'clip-path',
  'mask',
  'stroke',
  'stroke-width',
  'stroke-linecap',
  'stroke-linejoin',
  'stroke-miterlimit',
  'stroke-dasharray',
  'stroke-dashoffset',
  'stroke-opacity',
  'vector-effect',
  'shape-rendering',
  'offset',
  'stop-color',
  'stop-opacity',
  'gradientUnits',
  'gradientTransform',
  'spreadMethod',
  'clipPathUnits',
  'maskUnits',
  'maskContentUnits',
  'href',
  'xlink:href',
  'font-family',
  'font-size',
  'font-weight',
  'text-anchor',
  'dx',
  'dy',
  'role',
  'aria-hidden',
  'aria-label',
  'focusable',
]

/** A `url(...)` that points anywhere but this document. */
const EXTERNAL_URL_REFERENCE = /url\(\s*['"]?\s*(?!#)/i

let purifier: Purifier | null = null

// Its own instance: hooks on the shared DOMPurify singleton would also run for
// markdown, mermaid and the excalidraw preview.
function iconPurifier(): Purifier {
  if (purifier) return purifier
  purifier = DOMPurify(window)
  purifier.addHook('afterSanitizeAttributes', (node) => {
    for (const attribute of Array.from(node.attributes)) {
      const name = attribute.localName.toLowerCase()
      const value = attribute.value.trim()
      const offDocument =
        name === 'href' ? !value.startsWith('#') : EXTERNAL_URL_REFERENCE.test(value)
      if (name.startsWith('on') || offDocument) node.removeAttributeNode(attribute)
    }
  })
  return purifier
}

function sanitizeIcon(svg: string): string {
  const fragment = iconPurifier().sanitize(svg, {
    ALLOWED_TAGS,
    ALLOWED_ATTR,
    FORBID_TAGS,
    FORBID_ATTR: ['style', 'class'],
    RETURN_DOM_FRAGMENT: true,
  })
  const root = Array.from(fragment.children).find(
    (element) => element.localName === 'svg' && element.namespaceURI === SVG_NS,
  )
  return root?.outerHTML ?? ''
}

// Few distinct icons exist, but a hostile daemon could serve endless variants.
const CACHE_LIMIT = 64
const sanitized = new Map<string, string>()

function sanitizedIcon(svg: string): string {
  const cached = sanitized.get(svg)
  if (cached !== undefined) return cached
  const clean = svg ? sanitizeIcon(svg) : ''
  if (sanitized.size >= CACHE_LIMIT) sanitized.delete(sanitized.keys().next().value ?? '')
  sanitized.set(svg, clean)
  return clean
}

/**
 * Inline the SVG so its `currentColor` paths inherit the host row/tab's text
 * token — an <img> can't. Only the sanitised artwork is ever inlined.
 */
export function ProviderIcon({ svg, className }: ProviderIconProps) {
  return (
    <span
      aria-hidden="true"
      data-provider-icon=""
      className={cn(
        'inline-flex size-3.5 shrink-0 items-center justify-center [&>svg]:size-full',
        className,
      )}
      dangerouslySetInnerHTML={{ __html: sanitizedIcon(svg) }}
    />
  )
}
