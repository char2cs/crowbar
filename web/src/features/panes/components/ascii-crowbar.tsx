import { useEffect, useMemo, useRef } from 'react'
import { decodeCrowbarCloud } from './ascii-crowbar-cloud'
import {
  attachAsciiCrowbarRenderer,
  DEFAULT_FONT_SIZE,
  DEFAULT_RAMP,
} from './ascii-crowbar-renderer'

/**
 * <AsciiCrowbar /> — a slowly tumbling 3D crowbar rendered as ASCII art, in the
 * spirit of the classic donut.c demo. It is a decorative background for the New
 * Tab surface, so it is `pointer-events-none` and `aria-hidden`.
 *
 * ── How it works ────────────────────────────────────────────────────────────
 * GEOMETRY: a *point cloud* of a real crowbar — position + surface normal per
 * point — decoded once from `ascii-crowbar-cloud.ts`. That data is BAKED offline
 * (scripts/bake-crowbar-cloud.mjs) by area-weighted surface-sampling a CC0
 * crowbar mesh, so the runtime stays pure-Math with no 3D library and no mesh
 * parsing on the client. The cloud is already recentred and scaled to a unit
 * bounding radius, so it tumbles in place. (An earlier version approximated the
 * shape parametrically; the real mesh reads correctly from every angle.)
 *
 * PROJECTION (per frame, `renderFrame` in `ascii-crowbar-renderer.ts`), the
 * donut.c pipeline:
 *   1. rotate every point about X (angle a) and Y (angle b) at DIFFERENT rates,
 *      so it tumbles rather than spinning flat. The normal is rotated too.
 *   2. push the cloud away from the camera (z += K2) and perspective-divide:
 *      ooz = 1/z; sx = cx + K1·ooz·x; sy = cy − K1·ooz·y·ASPECT.
 *      K1 is the projection scale (chosen so the unit sphere fills FILL of the
 *      grid), K2 the camera distance. ASPECT corrects for non-square cells.
 *   3. z-buffer with `ooz` (larger ooz = nearer) so near points occlude far.
 *   4. luminance = rotated-normal · fixed light; higher luminance → denser glyph
 *      from the ramp. The nearest point always writes (at least the dimmest
 *      glyph) so the silhouette stays solid rather than showing through.
 *
 * PERFORMANCE: rendered onto a `<canvas>` — one `fillText` call per row, never
 * a DOM node per cell and never React state per frame. Canvas painting is
 * compositor-only: it cannot trigger layout or invalidate hit-testing, unlike
 * the `<pre>`+`textContent` this used to be. That distinction is not
 * theoretical — live-measured in the dev app (rAF-delta sampling), the old
 * `<pre>` cratered the WHOLE WINDOW to ~20fps merely by being on screen (no
 * mouse involved), and `display:none`-ing just that node — leaving the exact
 * same per-frame JS math running underneath, untouched — instantly restored
 * 120fps. The cost was never the projection math; it was the browser laying
 * out a many-thousand-character text block 30 times a second. The char + z
 * buffers are allocated once and reused every frame (zero allocation in the
 * hot loop besides the row strings `fillText` needs). Capped at ~30fps; paused
 * only when the work is INVISIBLE — offscreen, or the tab hidden. It
 * deliberately keeps tumbling while the app is merely not the key window: a
 * background window is still on screen, and a backdrop that freezes the
 * moment you click away is a visible defect, not a saving. A single static
 * frame under `prefers-reduced-motion`.
 *
 * THEMING: colour and font come from the app's tokens (`text-muted-foreground`,
 * `font-mono`), so it tracks light/dark for free. There is no HEV-orange token
 * in this codebase, so — per the brief — the muted foreground is used rather
 * than an invented colour; DIM_OPACITY keeps it a background, not a focal point.
 *
 * The render/animation loop itself (projection math, canvas sizing, resize/
 * visibility bookkeeping, the rAF loop) lives in `ascii-crowbar-renderer.ts` —
 * split out because none of it is React: it reads its inputs once, at attach
 * time, and runs its own imperative loop from there. This component's own job
 * is just owning the props, decoding the point cloud, and wiring the render
 * engine to its canvas/wrap refs for the effect's lifetime.
 */

interface AsciiCrowbarProps {
  /** Force a fixed character grid. Omit (the default) to fill the container:
   *  the glyph size is fixed and the grid grows/shrinks to cover the pane. */
  width?: number
  height?: number
  /** Rotation-speed multiplier (1 = default tumble). */
  speed?: number
  /** Glyph size in px (fixed). Smaller = denser art / more cells per pane. */
  fontSize?: number
  /** Sparse→dense luminance ramp. Later chars = brighter/denser. */
  charRamp?: string
  /** Seed for the STARTING orientation. Two surfaces with different seeds begin
   *  the tumble at different poses (e.g. per workspace), instead of in lockstep.
   *  Omit to start at the fixed default pose. */
  seed?: string | number
}

/** How dim the art sits behind the New Tab cluster. */
const DIM_OPACITY = 0.5

export default function AsciiCrowbar({
  width,
  height,
  speed = 1,
  fontSize = DEFAULT_FONT_SIZE,
  charRamp = DEFAULT_RAMP,
  seed,
}: AsciiCrowbarProps) {
  const wrapRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)

  // Decoded once (module-cached); independent of every prop.
  const { geo, count } = useMemo(() => decodeCrowbarCloud(), [])
  const rampCodes = useMemo(() => {
    const src = charRamp.length > 0 ? charRamp : DEFAULT_RAMP
    const codes = new Uint16Array(src.length)
    for (let i = 0; i < src.length; i++) codes[i] = src.charCodeAt(i)
    return codes
  }, [charRamp])

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    return attachAsciiCrowbarRenderer(canvas, wrapRef.current, {
      geo,
      count,
      rampCodes,
      width,
      height,
      speed,
      fontSize,
      seed,
    })
  }, [width, height, speed, fontSize, seed, geo, count, rampCodes])

  return (
    <div
      ref={wrapRef}
      aria-hidden="true"
      className="pointer-events-none absolute inset-0 grid select-none place-items-center overflow-hidden"
    >
      <canvas
        ref={canvasRef}
        className="m-0 block font-mono text-muted-foreground"
        style={{ opacity: DIM_OPACITY }}
      />
    </div>
  )
}
