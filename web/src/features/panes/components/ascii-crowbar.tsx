import { useEffect, useMemo, useRef } from 'react'
import { decodeCrowbarCloud } from './ascii-crowbar-cloud'

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
 * PROJECTION (per frame, `renderFrame`), the donut.c pipeline:
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
 * PERFORMANCE: one string into one <pre> via `textContent` — never a node per
 * cell, never React state per frame. The char + z buffers are allocated once and
 * reused every frame (zero allocation in the hot loop besides the unavoidable
 * output string). Capped at ~30fps; paused only when the work is INVISIBLE —
 * offscreen, or the tab hidden. It deliberately keeps tumbling while the app is
 * merely not the key window: a background window is still on screen, and a
 * backdrop that freezes the moment you click away is a visible defect, not a
 * saving. A single static frame under `prefers-reduced-motion`.
 *
 * THEMING: colour and font come from the app's tokens (`text-muted-foreground`,
 * `font-mono`), so it tracks light/dark for free. There is no HEV-orange token
 * in this codebase, so — per the brief — the muted foreground is used rather
 * than an invented colour; DIM_OPACITY keeps it a background, not a focal point.
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

// ── Look / projection constants (tunable) ───────────────────────────────────
// Fallback grid when the container hasn't been measured yet (tests / SSR).
const DEFAULT_WIDTH = 76
const DEFAULT_HEIGHT = 34
const DEFAULT_FONT_SIZE = 9
const DEFAULT_RAMP = '.,:;irsXA253hMHGS#9B&@'

/** Cell aspect: typical mono advance ≈ 0.6em wide; matching the line-height
 *  makes cells near-square, so the crowbar isn't vertically stretched. */
const MONO_ADVANCE_RATIO = 0.6
const LINE_HEIGHT_RATIO = 0.6
const ASPECT = MONO_ADVANCE_RATIO / LINE_HEIGHT_RATIO // 1 when cells are square

/** How dim the art sits behind the New Tab cluster. */
const DIM_OPACITY = 0.5

const TARGET_FPS = 30
const FRAME_INTERVAL = 1000 / TARGET_FPS

const K2 = 4.6 // camera distance
const FILL = 0.98 // fraction of the shorter grid axis the unit sphere fills

// Rotation rates (rad/s) — deliberately incommensurate so the tumble never
// settles into a flat spin.
const RATE_A = 0.5
const RATE_B = 0.29
// A pleasant static 3/4 view (also the prefers-reduced-motion frame).
const INIT_A = -0.35
const INIT_B = 0.7

// Fixed light direction in VIEW space (upper-front-left, toward the camera at
// −z), normalised. Front faces (normal.z < 0) catch it.
const LIGHT = (() => {
  const x = -0.35
  const y = 0.55
  const z = -0.75
  const inv = 1 / Math.hypot(x, y, z)
  return { x: x * inv, y: y * inv, z: z * inv }
})()

const SPACE = 32

/** Deterministic 32-bit hash, for turning a seed into a start pose. FNV-1a plus
 *  an xorshift-multiply finalizer, so seeds differing by one trailing character
 *  (e.g. "ws-1" vs "ws-2") still avalanche to far-apart poses. */
function hash32(s: string): number {
  let h = 2166136261
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  h ^= h >>> 16
  h = Math.imul(h, 2246822507)
  h ^= h >>> 13
  h = Math.imul(h, 3266489909)
  h ^= h >>> 16
  return h >>> 0
}

/** Map a seed to a starting (angA, angB) spread across the full tumble range,
 *  so two surfaces with different seeds never begin at the same orientation. */
function seededAngles(seed: string | number): { a: number; b: number } {
  const key = String(seed)
  const TAU = Math.PI * 2
  return {
    a: (hash32('a:' + key) / 0x100000000) * TAU,
    b: (hash32('b:' + key) / 0x100000000) * TAU,
  }
}

export default function AsciiCrowbar({
  width,
  height,
  speed = 1,
  fontSize = DEFAULT_FONT_SIZE,
  charRamp = DEFAULT_RAMP,
  seed,
}: AsciiCrowbarProps) {
  const wrapRef = useRef<HTMLDivElement>(null)
  const preRef = useRef<HTMLPreElement>(null)

  // Decoded once (module-cached); independent of every prop.
  const { geo, count } = useMemo(() => decodeCrowbarCloud(), [])
  const rampCodes = useMemo(() => {
    const src = charRamp.length > 0 ? charRamp : DEFAULT_RAMP
    const codes = new Uint16Array(src.length)
    for (let i = 0; i < src.length; i++) codes[i] = src.charCodeAt(i)
    return codes
  }, [charRamp])

  useEffect(() => {
    const pre = preRef.current
    const wrap = wrapRef.current
    if (!pre) return

    const rampLen = rampCodes.length
    // Glyph size is FIXED; the grid grows to cover the pane. Cells are square
    // (advance ≈ line-height ≈ 0.6em), so one cell is `cell` px on both axes.
    const cell = fontSize * MONO_ADVANCE_RATIO

    // Grid + buffers are (re)allocated by measure() and read by renderFrame().
    let W = 0
    let H = 0
    let screen = new Uint16Array(0)
    let zbuf = new Float32Array(0)
    let rows: string[] = []
    let cx = 0
    let cy = 0
    let K1 = 0

    // Seeded surfaces start at their own pose; unseeded ones at the default.
    const start0 = seed !== undefined ? seededAngles(seed) : { a: INIT_A, b: INIT_B }
    let angA = start0.a
    let angB = start0.b

    const renderFrame = () => {
      if (W <= 0 || H <= 0) return
      screen.fill(SPACE)
      zbuf.fill(0)
      const ca = Math.cos(angA)
      const sa = Math.sin(angA)
      const cbb = Math.cos(angB)
      const sbb = Math.sin(angB)
      const lx = LIGHT.x
      const ly = LIGHT.y
      const lz = LIGHT.z
      for (let i = 0; i < count; i++) {
        const o = i * 6
        const x = geo[o]
        const y = geo[o + 1]
        const z = geo[o + 2]
        const nx = geo[o + 3]
        const ny = geo[o + 4]
        const nz = geo[o + 5]
        // rotate about X, then Y (position)
        const y1 = y * ca - z * sa
        const z1 = y * sa + z * ca
        const x2 = x * cbb + z1 * sbb
        const z2 = -x * sbb + z1 * cbb
        const zc = z2 + K2
        if (zc <= 0) continue
        const ooz = 1 / zc
        const sx = Math.round(cx + K1 * ooz * x2)
        const sy = Math.round(cy - K1 * ooz * y1 * ASPECT)
        if (sx < 0 || sx >= W || sy < 0 || sy >= H) continue
        const cell2 = sy * W + sx
        if (ooz > zbuf[cell2]) {
          zbuf[cell2] = ooz
          // rotate the normal the same way, shade against the fixed light
          const ny1 = ny * ca - nz * sa
          const nz1 = ny * sa + nz * ca
          const nx2 = nx * cbb + nz1 * sbb
          const nz2 = -nx * sbb + nz1 * cbb
          const lum = nx2 * lx + ny1 * ly + nz2 * lz
          let idx = lum > 0 ? (lum * rampLen) | 0 : 0
          if (idx >= rampLen) idx = rampLen - 1
          screen[cell2] = rampCodes[idx]
        }
      }
      for (let r = 0; r < H; r++) {
        const s = r * W
        // Spread one row (W codes) into fromCharCode, then join — array-join
        // style, never per-cell string concatenation.
        rows[r] = String.fromCharCode(...screen.subarray(s, s + W))
      }
      pre.textContent = rows.join('\n')
    }

    // Size the grid to the container (glyph size fixed). `width`/`height` props,
    // if given, force a fixed grid instead. Reallocates only on a real change.
    const measure = () => {
      const cw = wrap?.clientWidth ?? 0
      const ch = wrap?.clientHeight ?? 0
      let w = width ?? Math.floor(cw / cell)
      let h = height ?? Math.floor(ch / cell)
      if (!Number.isFinite(w) || w < 8) w = width ?? DEFAULT_WIDTH
      if (!Number.isFinite(h) || h < 8) h = height ?? DEFAULT_HEIGHT
      if (w === W && h === H) return
      W = w
      H = h
      screen = new Uint16Array(W * H)
      zbuf = new Float32Array(W * H)
      rows = new Array<string>(H)
      cx = W / 2
      cy = H / 2
      // Unit sphere → FILL of the shorter (tighter) grid axis. Cells are square,
      // so the same scale applies to both axes; the tighter axis avoids clipping.
      K1 = (FILL * Math.min(W, H) * K2) / 2
      renderFrame()
    }

    const reduced =
      typeof window !== 'undefined' && typeof window.matchMedia === 'function'
        ? window.matchMedia('(prefers-reduced-motion: reduce)').matches
        : false

    // Fixed glyph size; draw an immediate frame so there's never a blank flash.
    pre.style.fontSize = `${fontSize}px`
    measure()

    let ro: ResizeObserver | undefined
    if (typeof ResizeObserver !== 'undefined' && wrap) {
      ro = new ResizeObserver(measure)
      ro.observe(wrap)
    }

    if (reduced) {
      // Static frame only; keep the ResizeObserver so it re-grids on resize.
      return () => ro?.disconnect()
    }

    let rafId = 0
    let running = false
    let lastT: number | null = null
    let lastRender = 0
    let onscreen = true
    let tabVisible = typeof document !== 'undefined' ? document.visibilityState !== 'hidden' : true

    const loop = (t: number) => {
      rafId = requestAnimationFrame(loop)
      if (lastT === null) lastT = t
      const dt = (t - lastT) / 1000
      lastT = t
      // Advance by real elapsed time so speed is fps-independent.
      angA += RATE_A * speed * dt
      angB += RATE_B * speed * dt
      if (t - lastRender < FRAME_INTERVAL) return
      lastRender = t
      renderFrame()
    }
    const start = () => {
      if (running) return
      running = true
      lastT = null
      rafId = requestAnimationFrame(loop)
    }
    const stop = () => {
      running = false
      if (rafId) cancelAnimationFrame(rafId)
      rafId = 0
    }
    const sync = () => {
      if (onscreen && tabVisible) start()
      else stop()
    }

    let io: IntersectionObserver | undefined
    if (typeof IntersectionObserver !== 'undefined' && wrap) {
      io = new IntersectionObserver(
        (entries) => {
          onscreen = entries[entries.length - 1]?.isIntersecting ?? true
          sync()
        },
        { threshold: 0 },
      )
      io.observe(wrap)
    }

    const onVisibility = () => {
      tabVisible = document.visibilityState !== 'hidden'
      sync()
    }
    if (typeof document !== 'undefined') {
      document.addEventListener('visibilitychange', onVisibility)
    }

    sync()

    return () => {
      stop()
      io?.disconnect()
      ro?.disconnect()
      if (typeof document !== 'undefined') {
        document.removeEventListener('visibilitychange', onVisibility)
      }
    }
  }, [width, height, speed, fontSize, seed, geo, count, rampCodes])

  return (
    <div
      ref={wrapRef}
      aria-hidden="true"
      className="pointer-events-none absolute inset-0 grid select-none place-items-center overflow-hidden"
    >
      <pre
        ref={preRef}
        className="m-0 whitespace-pre font-mono text-muted-foreground"
        style={{ fontSize, lineHeight: LINE_HEIGHT_RATIO, opacity: DIM_OPACITY }}
      />
    </div>
  )
}
