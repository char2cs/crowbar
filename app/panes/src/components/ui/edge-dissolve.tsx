import type { CSSProperties } from 'react'
import { cn } from '@/lib/utils'

interface DissolveLayerSpec {
  /** px blur radius for THIS layer's backdrop-filter. */
  blur: number
  /** Mask stops as fractions of the zone (0 = far edge, 1 = near): transparent→black, then
   *  →black→ transparent for mid layers; the last two run to the near edge. From composer-dock.css's
   *  `.dissolve-layer`: stacked masked layers fake a ramping blur one backdrop-filter can't. */
  stops: number[]
}

// composer-dock.css's 7-layer `.dissolve-layer` recipe, as data rather than `:nth-child` rules tied
// to one zone height, so it runs at whatever height the caller implies.
const LAYERS: DissolveLayerSpec[] = [
  { blur: 1, stops: [0, 0.1, 0.3, 0.4] },
  { blur: 2, stops: [0.1, 0.2, 0.4, 0.5] },
  { blur: 4, stops: [0.15, 0.3, 0.5, 0.6] },
  { blur: 8, stops: [0.2, 0.4, 0.6, 0.7] },
  { blur: 16, stops: [0.4, 0.6, 0.8, 0.9] },
  { blur: 32, stops: [0.6, 0.8] },
  { blur: 64, stops: [0.7, 1] },
]

function layerStyle(layer: DissolveLayerSpec, height: number): CSSProperties {
  const firstStop = layer.stops[0]!
  const lastStop = layer.stops[layer.stops.length - 1]!
  const top = Math.max(0, height * firstStop - layer.blur)
  // A 2-stop layer runs to the near edge (bottom: 0 as authored; edge="top" flips it); a 4-stop
  // layer's fade-out stops short of it.
  const bottom = layer.stops.length === 4 ? Math.max(0, height * (1 - lastStop) - layer.blur) : 0
  const colors =
    layer.stops.length === 4
      ? ['transparent', 'black', 'black', 'transparent']
      : ['transparent', 'black']
  const stopsCss = layer.stops
    .map((s, i) => `${colors[i]} ${(height * s - top).toFixed(2)}px`)
    .join(', ')
  const mask = `linear-gradient(to bottom, ${stopsCss})`
  return {
    position: 'absolute',
    left: 0,
    right: 0,
    top,
    bottom,
    backdropFilter: `blur(${layer.blur}px)`,
    WebkitBackdropFilter: `blur(${layer.blur}px)`,
    maskImage: mask,
    WebkitMaskImage: mask,
  }
}

interface EdgeDissolveProps {
  /** Which edge of its positioned ancestor this dissolves FROM — content
   *  passing under that edge is what blurs. */
  edge: 'top' | 'bottom'
  /** Zone height in px; taller ramps the blur more gradually (the composer's is ~2-4x its dock). */
  height: number
  className?: string
}

// The composer's "glass": content behind blurs and fades instead of clipping at a hard edge.
// Authored once for edge="bottom"; "top" flips the stack with scaleY(-1) (blur is isotropic).
// Decorative, no pointer events; the caller positions it inside its own relative box.
export function EdgeDissolve({ edge, height, className }: EdgeDissolveProps) {
  return (
    <div
      aria-hidden="true"
      data-testid="edge-dissolve"
      data-edge={edge}
      className={cn('absolute inset-x-0 [contain:paint]', className)}
      style={{
        height,
        top: edge === 'top' ? 0 : undefined,
        bottom: edge === 'bottom' ? 0 : undefined,
        transform: edge === 'top' ? 'scaleY(-1)' : undefined,
        pointerEvents: 'none',
      }}
    >
      {LAYERS.map((layer, i) => (
        <div key={i} style={layerStyle(layer, height)} />
      ))}
    </div>
  )
}
