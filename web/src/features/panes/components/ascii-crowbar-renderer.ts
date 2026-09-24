/**
 * The tumbling-ASCII-crowbar render engine — the donut.c-style projection
 * math, canvas painting, and resize/visibility bookkeeping that
 * `ascii-crowbar.tsx` drives — split into its own pure canvas/DOM module
 * (no React) because none of it touches component props reactively: it
 * reads its inputs once, at attach time, and runs its own imperative loop
 * from there. See `ascii-crowbar.tsx`'s own doc comment for the full
 * pipeline description (rotate → project → z-buffer → shade) and the
 * `<canvas>`-over-`<pre>` performance rationale.
 */

// ── Look / projection constants (tunable) ───────────────────────────────────
// Fallback grid when the container hasn't been measured yet (tests / SSR).
const DEFAULT_WIDTH = 76
const DEFAULT_HEIGHT = 34
export const DEFAULT_FONT_SIZE = 9
export const DEFAULT_RAMP = '.,:;irsXA253hMHGS#9B&@'

/** Cell aspect: typical mono advance ≈ 0.6em wide; matching the line-height
 *  makes cells near-square, so the crowbar isn't vertically stretched. */
const MONO_ADVANCE_RATIO = 0.6
const LINE_HEIGHT_RATIO = 0.6
const ASPECT = MONO_ADVANCE_RATIO / LINE_HEIGHT_RATIO // 1 when cells are square

const TARGET_FPS = 30
const FRAME_INTERVAL = 1000 / TARGET_FPS
/** How long the tumble plays after mount before settling on a static frame.
 *  An empty pane is idle by definition: a backdrop that animates forever was
 *  measured at ~97% of the idle webview's CPU (half a core per empty pane). */
export const INTRO_MS = 1500

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

export interface AsciiCrowbarRendererOptions {
  /** Point cloud from `decodeCrowbarCloud()`: position + surface normal per point. */
  geo: Float32Array
  count: number
  rampCodes: Uint16Array
  /** Force a fixed character grid. Omit to fill `wrap`. */
  width?: number
  height?: number
  /** Rotation-speed multiplier (1 = default tumble). */
  speed: number
  /** Glyph size in px (fixed). */
  fontSize: number
  /** Seed for the STARTING orientation. Omit to start at the fixed default pose. */
  seed?: string | number
}

/**
 * Attaches the whole render/animation loop to `canvas` and returns its
 * teardown. `wrap` (the canvas's sizing container) may be null (SSR/tests
 * without a measured container) — sizing then falls back to `width`/`height`
 * or the module defaults.
 */
export function attachAsciiCrowbarRenderer(
  canvas: HTMLCanvasElement,
  wrap: HTMLDivElement | null,
  { geo, count, rampCodes, width, height, speed, fontSize, seed }: AsciiCrowbarRendererOptions,
): () => void {
  const ctx = canvas.getContext('2d')
  if (!ctx) return () => {}

  const rampLen = rampCodes.length
  // Glyph size is FIXED; the grid grows to cover the pane. Cells are square
  // (advance ≈ line-height ≈ 0.6em), so one cell is `cell` px on both axes.
  const cell = fontSize * MONO_ADVANCE_RATIO
  const lineHeight = fontSize * LINE_HEIGHT_RATIO

  // Grid + buffers are (re)allocated by measure() and read by renderFrame().
  let W = 0
  let H = 0
  let screen = new Uint16Array(0)
  let zbuf = new Float32Array(0)
  let rows: string[] = []
  let cx = 0
  let cy = 0
  let K1 = 0

  // `color`/`fontFamily` resolved from CSS classes on the (invisible,
  // canvas-drawn-nothing-itself) canvas element rather than hardcoded, so
  // this still "tracks light/dark for free" the way the old `<pre>` did —
  // canvas ignores `color`/`font-family` for its own box, but they still
  // resolve correctly via `getComputedStyle`. Re-read on a theme flip
  // (watched via the same `class`/`data-theme` attributes settings-effects.ts
  // writes onto `<html>`), not every frame.
  let color = '#000'
  let fontFamily = 'monospace'
  const refreshStyle = () => {
    const computed = getComputedStyle(canvas)
    color = computed.color
    fontFamily = computed.fontFamily
    ctx.font = `${fontSize}px ${fontFamily}`
  }

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
    // Row strings, one `fillText` each — never per-cell draw calls, and
    // (unlike the old `pre.textContent = rows.join('\n')`) never anything
    // that touches layout: canvas painting is compositor-only.
    ctx.clearRect(0, 0, W * cell, H * lineHeight)
    ctx.fillStyle = color
    for (let r = 0; r < H; r++) {
      const s = r * W
      // Spread one row (W codes) into fromCharCode, then join — array-join
      // style, never per-cell string concatenation.
      rows[r] = String.fromCharCode(...screen.subarray(s, s + W))
      ctx.fillText(rows[r], 0, r * lineHeight)
    }
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

    // Backing store at devicePixelRatio, CSS-sized down — the standard
    // crisp-canvas recipe. `ctx.font` survives a `canvas.width` write on
    // some engines but not reliably on all, so it's reset in `refreshStyle`
    // right after, not left to chance.
    const dpr = window.devicePixelRatio || 1
    const pixelW = W * cell
    const pixelH = H * lineHeight
    canvas.width = Math.max(1, Math.round(pixelW * dpr))
    canvas.height = Math.max(1, Math.round(pixelH * dpr))
    canvas.style.width = `${pixelW}px`
    canvas.style.height = `${pixelH}px`
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    ctx.textBaseline = 'top'
    refreshStyle()

    renderFrame()
  }

  const reduced =
    typeof window !== 'undefined' && typeof window.matchMedia === 'function'
      ? window.matchMedia('(prefers-reduced-motion: reduce)').matches
      : false

  measure()

  // A re-grid reallocates both buffers, rewrites `canvas.width` (dropping the
  // backing store) and re-renders all 15000 points. The glyph cell is 5.4px,
  // so a sash drag — which rewrites the pane's flex-basis on every raw
  // pointermove — genuinely changes the grid on nearly every one: measured
  // live, this observer alone burned 216ms across a ~2s drag, the largest
  // single cost in it. It also wrote `canvas.style` INSIDE the observer
  // callback, re-dirtying layout into another observation pass (the
  // "ResizeObserver loop completed with undelivered notifications" storm).
  // Hence: one re-grid per frame at most, and none for the span of a drag —
  // the `data-pane-resizing` / `pane-resize-end` pair the Monaco and
  // EdgeDissolve pauses already key off.
  let measureFrame = 0
  const scheduleMeasure = () => {
    if (measureFrame) return
    measureFrame = requestAnimationFrame(() => {
      measureFrame = 0
      if (document.documentElement.hasAttribute('data-pane-resizing')) return
      measure()
    })
  }
  const onPaneResizeEnd = () => scheduleMeasure()
  window.addEventListener('pane-resize-end', onPaneResizeEnd)

  let ro: ResizeObserver | undefined
  if (typeof ResizeObserver !== 'undefined' && wrap) {
    ro = new ResizeObserver(scheduleMeasure)
    ro.observe(wrap)
  }

  // A theme flip changes `color`'s resolved value — re-paint the CURRENT
  // pose immediately rather than waiting for the tumble's next natural
  // frame (imperceptible while animating, but a visible stale tint for a
  // whole beat under `prefers-reduced-motion`, which never renders again
  // on its own).
  let themeObserver: MutationObserver | undefined
  if (typeof MutationObserver !== 'undefined' && typeof document !== 'undefined') {
    themeObserver = new MutationObserver(() => {
      refreshStyle()
      renderFrame()
    })
    themeObserver.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class', 'data-theme'],
    })
  }

  if (reduced) {
    // Static frame only; keep the observers so it re-grids/re-tints.
    return () => {
      if (measureFrame) cancelAnimationFrame(measureFrame)
      window.removeEventListener('pane-resize-end', onPaneResizeEnd)
      ro?.disconnect()
      themeObserver?.disconnect()
    }
  }

  let rafId = 0
  let running = false
  let settled = false
  let lastT: number | null = null
  let lastRender = 0
  let played = 0
  let onscreen = true
  let tabVisible = typeof document !== 'undefined' ? document.visibilityState !== 'hidden' : true

  const loop = (t: number) => {
    if (lastT === null) lastT = t
    const dtMs = t - lastT
    lastT = t
    played += dtMs
    // Advance by real elapsed time so speed is fps-independent.
    angA += RATE_A * speed * (dtMs / 1000)
    angB += RATE_B * speed * (dtMs / 1000)
    if (played >= INTRO_MS) {
      // The intro is over: paint the pose it ended on and schedule nothing,
      // ever again. Only resize/theme observers remain, and they fire on
      // change, not on time.
      renderFrame()
      settle()
      return
    }
    rafId = requestAnimationFrame(loop)
    if (t - lastRender < FRAME_INTERVAL) return
    lastRender = t
    renderFrame()
  }
  const start = () => {
    if (running || settled) return
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
  function settle() {
    settled = true
    stop()
    io?.disconnect()
    if (typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }

  sync()

  return () => {
    // `stop()`'s body, inlined: the loop self-schedules, so teardown must
    // cancel the pending frame or it keeps running past unmount. Inlined
    // rather than calling stop() so the cancel is visible right here.
    running = false
    if (rafId) cancelAnimationFrame(rafId)
    rafId = 0
    if (measureFrame) cancelAnimationFrame(measureFrame)
    window.removeEventListener('pane-resize-end', onPaneResizeEnd)
    io?.disconnect()
    ro?.disconnect()
    themeObserver?.disconnect()
    if (typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }
}
