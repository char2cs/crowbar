import type { CSSProperties } from 'react'
import { cn } from '@/lib/utils'

interface DissolveLayerSpec {
  /** px blur radius for THIS layer's backdrop-filter. */
  blur: number
  /** Fractions of the zone height (0 = the zone's own "far" edge, 1 = its
   *  "near" edge) where this layer's mask ramps transparent -> black (->
   *  black -> transparent for a mid-zone layer; the last two layers run all
   *  the way to the near edge, so they only need the transparent -> black
   *  ramp). Ported verbatim from composer.css's `.dissolve-layer`
   *  `:nth-child` rules — see that file's own doc for why the recipe looks
   *  like this (stacked masked layers faking a blur that ramps, since a
   *  single backdrop-filter can't). */
  stops: number[]
}

// The exact 7-layer recipe from composer.css's `.dissolve-layer`, written
// generically instead of as `:nth-child` rules tied to one hard-coded zone
// height — this needs to run at whatever height a caller's own row implies,
// not just the composer's own measured dock height.
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
  // A 2-stop layer runs to the zone's own near edge (bottom: 0 in this
  // component's own "as authored" frame — see the edge="top" doc below for
  // how that becomes the visual TOP once flipped); a 4-stop layer's own
  // fade-out means it stops short of it.
  const bottom = layer.stops.length === 4 ? Math.max(0, height * (1 - lastStop) - layer.blur) : 0
  const colors = layer.stops.length === 4 ? ['transparent', 'black', 'black', 'transparent'] : ['transparent', 'black']
  const stopsCss = layer.stops.map((s, i) => `${colors[i]} ${(height * s - top).toFixed(2)}px`).join(', ')
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
  /** The dissolve zone's own height in px. Bigger gives the blur more room
   *  to ramp gradually; the composer's own zone is roughly 2-4x its dock's
   *  height (composer.css's `--dissolve-h`) — same proportions are a
   *  reasonable start for a caller sizing this off its own header. */
  height: number
  className?: string
}

/**
 * The chat composer's own "glass" — content passing behind this blurs and
 * fades rather than being clipped by a hard edge (composer.css's
 * `.dissolve`/`.dissolve-layer`, "the layered-mask recipe" it cites). Pulled
 * out generically so it can anchor to either edge at any height, rather than
 * staying wired to the composer's own measured dock height and scrollbar
 * inset.
 *
 * The recipe is authored ONCE, as if for edge="bottom" (matching the
 * composer's own orientation: sharp far edge, heaviest blur at the near
 * edge). edge="top" reuses the IDENTICAL layer math and just flips the
 * whole stack vertically (`scaleY(-1)`) rather than re-deriving mirrored
 * percentages — blur is isotropic, so only the mask's distribution needs to
 * flip, and a transform does that for free.
 *
 * Purely decorative: `pointer-events: none` and `aria-hidden`, same as the
 * composer's own dissolve — a caller positions this absolutely within its
 * own `position: relative` box (it does not position itself beyond pinning
 * to its own edge) and stacks real content both before it (what should
 * blur) and after it (what should stay sharp — a header's own label, say).
 */
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
