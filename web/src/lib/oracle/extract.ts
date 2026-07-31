/**
 * The React half of the anchor-snapshot oracle — see `native/oracle/ANCHORS.md`,
 * which is the contract this file implements. **Read it before changing anything
 * here**: a field that exists on one side and not the other is invisible to the
 * differ and is therefore worse than no coverage at all.
 *
 * ## Injectability
 *
 * This module is written so the whole runtime can be serialised into a single
 * self-contained IIFE and pasted into a page over an `execute_js` bridge, where
 * there is no module resolution. That imposes two rules on everything below:
 *
 * 1. Every runtime helper is a **named `function` declaration** at module scope
 *    and appears in {@link ORACLE_RUNTIME}. `extractSnapshotSource` emits each
 *    one's `Function.prototype.toString()` into one scope, so cross-references
 *    resolve by name (this survives minification, which renames consistently).
 * 2. **No runtime helper may close over a module-level binding** — not an
 *    import, not a `const`. Constants are inlined at their use site. Types are
 *    fine: they are erased before `toString()` ever sees the source.
 */

// ─── the snapshot shape, ANCHORS.md v1.1 §2/§3 (`schema` stays 1) ────────────
//
// Unknown fields are a hard failure in the differ, at document and anchor level.
// Nothing below may grow a field that is not in §3's table — a new one goes into
// ANCHORS.md first, with a version bump, and all three implementations follow.

export interface OracleBounds {
  x: number
  y: number
  w: number
  h: number
}

export interface OracleFont {
  size: number
  weight: number
  family: string
  line_height: number
}

export interface OracleBorder {
  w: number
  color: string
}

export interface OracleAnchor {
  id: string
  bounds: OracleBounds
  bg: string
  visible: boolean
  radius?: number
  border?: OracleBorder
  fg?: string
  text?: string
  text_width?: number
  clipped?: boolean
  font?: OracleFont
  /**
   * ANCHORS.md v1.5 — this anchor's box sizes to its own text.
   *
   * **Declared by the component, never detected here.** GPUI `ceil()`s a text
   * run's max-content width where this engine keeps the fraction, so the differ
   * compares such a box against `ceil(reference)`. Working the flag out from
   * `width: auto` and not-a-stretched-flex-item is a heuristic that flex-grow
   * falsifies, and the GPUI side's equivalent guess is falsifiable the same way
   * — two extractors each guessing is the silent divergence this contract
   * exists to prevent, and a wrong guess announces nothing. So it comes off
   * `data-oracle-content-sized`, authored next to `data-oracle-id`.
   *
   * Emitted **only when true**: v1.5 defines the absent key and an explicit
   * `false` as the same fact, and a `false` on every anchor would be bytes that
   * say nothing.
   */
  content_sized?: boolean
  /**
   * ANCHORS.md v1.6 — this anchor's box height **is** its own line box.
   *
   * The differ then compares `bounds.h` against this side's `font.line_height`
   * — the fractional 18.9 — rather than against this side's `bounds.h`. It has
   * to, because the two engines quantise that one number differently and
   * neither can produce the other's: WebKit floors a line box to a whole
   * logical pixel (14px × 1.35 = 18.9 → 18) where GPUI snaps it to the device
   * grid (→ 19.0 at DPR 2).
   *
   * **Declared, never detected**, like `content_sized` and for a sharper
   * reason. "One line of text and no explicit height" is exactly wrong for this
   * row's own `<Badge>`: `sm:h-4` pins its border box at 16px around a 13.33px
   * line box, so a detector would compare 16 against 13.33 and manufacture a
   * 2.67px delta on an anchor where both engines already agree. So it comes off
   * `data-oracle-line-sized`, authored next to `data-oracle-id`.
   *
   * An anchor that declares it **must** paint text: the differ refuses, by
   * anchor name, a snapshot carrying `line_sized` without a `font`. This
   * extractor does not soften that — a mis-authored attribute on a box has to
   * be loud, not quietly dropped.
   *
   * Emitted **only when true**, exactly as `content_sized` is.
   */
  line_sized?: boolean
}

export type OracleTheme = 'light' | 'dark'
export type OracleContent = 'short' | 'normal' | 'overflow'
export type OracleFlag = 'empty' | 'loading' | 'error' | 'hover' | 'focus' | 'selected'

/**
 * The §8.3 matrix cell that produced a snapshot. Data, not decoration — and
 * since v1.1 a **fixed vocabulary**, lowercase, no synonyms. The differ treats a
 * mismatched cell as a refusal rather than a delta, so `"Dark"` against `"dark"`
 * silently refuses every comparison in the run.
 */
export interface OracleState {
  /** Integer logical px. */
  width: number
  theme: OracleTheme
  content: OracleContent
  /** A set: sorted, no duplicates. */
  flags: OracleFlag[]
}

export interface OracleSnapshot {
  schema: 1
  surface: string
  state: OracleState
  root: string
  anchors: OracleAnchor[]
}

export interface ExtractOptions {
  /** Names the component under test, e.g. `git-status-row`. */
  surface: string
  /**
   * Matrix cell. Missing fields are filled in from the live document: `width`
   * from the root anchor, `theme` by {@link oracleDetectTheme}, `content` as
   * `normal` and `flags` as `[]`. Anything supplied is validated against the
   * v1.1 vocabulary and rejected loudly rather than emitted as a silent refusal.
   */
  state?: Partial<OracleState>
  /** Anchor id that all geometry is relative to (ANCHORS.md §4). */
  root?: string
  /**
   * Where to look for the root anchor. A CSS selector (the only form that
   * survives serialisation into an injected script) or an element.
   */
  scope?: string | Element | Document
  /** Which occurrence of the root anchor inside `scope`. Defaults to the first. */
  index?: number
  /**
   * Anchor id → pseudo-element selector whose paint backs that anchor
   * (ANCHORS.md §3). Only valid when the pseudo is `position:absolute; inset:0`.
   *
   * Defaults to `::before` on **both** gate surfaces' root anchors —
   * `git-row-item` and `file-row-item`. They are the same wrapper class,
   * `.file-tree-item`, and every visible row background on either one is
   * painted by `.file-tree-item::before` while the button is pinned
   * `background-color: transparent !important`. That rule group is *unscoped*,
   * so it reaches the git status panel as well as the file tree.
   *
   * The map is keyed by anchor id, so carrying both entries costs a snapshot of
   * either surface nothing: the other key never matches.
   */
  pseudo?: Record<string, string>
}

interface OracleBox {
  left: number
  top: number
  width: number
  height: number
}

// ─── runtime helpers (see the injectability rules above) ─────────────────────

/** Two decimal places — well inside the ±0.5px tolerance, and readable JSON. */
export function oracleRound(value: number): number {
  if (typeof value !== 'number' || !isFinite(value)) return 0
  return Math.round(value * 100) / 100
}

/**
 * Any CSS colour the engine can hand back → `#rrggbbaa` sRGB.
 *
 * `getComputedStyle` does not return one canonical form: legacy `rgb()`/`rgba()`,
 * modern slash-alpha `rgb(r g b / a)`, `color(srgb …)` (what `color-mix()`
 * resolves to), and `oklch()` (what this app's theme tokens are authored in) all
 * turn up. Anything still unrecognised is measured on a 1x1 canvas, which asks
 * the engine itself rather than guessing.
 */
export function oracleNormalizeColor(input: string | null | undefined): string {
  function hex2(n: number): string {
    let v = Math.round(n)
    if (!(v >= 0)) v = 0
    if (v > 255) v = 255
    return (v < 16 ? '0' : '') + v.toString(16)
  }
  function pack(r: number, g: number, b: number, a: number): string {
    let alpha = a
    if (!(alpha >= 0)) alpha = 0
    if (alpha > 1) alpha = 1
    return '#' + hex2(r) + hex2(g) + hex2(b) + hex2(alpha * 255)
  }
  function channel(token: string | undefined, scale: number): number {
    if (token === undefined) return 0
    const t = String(token).trim()
    if (t === '' || t === 'none') return 0
    if (t.charAt(t.length - 1) === '%') return (parseFloat(t) / 100) * scale
    const n = parseFloat(t)
    return isFinite(n) ? n : 0
  }
  function angle(token: string | undefined): number {
    if (token === undefined) return 0
    const t = String(token).trim()
    const n = parseFloat(t)
    if (!isFinite(n)) return 0
    if (t.indexOf('turn') >= 0) return n * 360
    if (t.indexOf('grad') >= 0) return n * 0.9
    if (t.indexOf('rad') >= 0) return (n * 180) / Math.PI
    return n
  }
  function gamma(c: number): number {
    const v = c <= 0.0031308 ? 12.92 * c : 1.055 * Math.pow(c, 1 / 2.4) - 0.055
    return Math.max(0, Math.min(1, v)) * 255
  }
  function oklabToRgb(L: number, a: number, b: number): number[] {
    const l_ = L + 0.3963377774 * a + 0.2158037573 * b
    const m_ = L - 0.1055613458 * a - 0.0638541728 * b
    const s_ = L - 0.0894841775 * a - 1.291485548 * b
    const l = l_ * l_ * l_
    const m = m_ * m_ * m_
    const s = s_ * s_ * s_
    return [
      gamma(4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s),
      gamma(-1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s),
      gamma(-0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s),
    ]
  }
  function hslToRgb(h: number, s: number, l: number): number[] {
    const hh = ((h % 360) + 360) % 360
    const c = (1 - Math.abs(2 * l - 1)) * s
    const x = c * (1 - Math.abs(((hh / 60) % 2) - 1))
    const m = l - c / 2
    let r = 0
    let g = 0
    let b = 0
    if (hh < 60) {
      r = c
      g = x
    } else if (hh < 120) {
      r = x
      g = c
    } else if (hh < 180) {
      g = c
      b = x
    } else if (hh < 240) {
      g = x
      b = c
    } else if (hh < 300) {
      r = x
      b = c
    } else {
      r = c
      b = x
    }
    return [(r + m) * 255, (g + m) * 255, (b + m) * 255]
  }

  if (input === null || input === undefined) return '#00000000'
  const value = String(input).trim()
  const lower = value.toLowerCase()
  if (lower === '' || lower === 'transparent' || lower === 'none') return '#00000000'

  if (lower.charAt(0) === '#') {
    const h = lower.slice(1)
    if (h.length === 3 || h.length === 4) {
      const a = h.length === 4 ? parseInt(h.charAt(3) + h.charAt(3), 16) / 255 : 1
      return pack(
        parseInt(h.charAt(0) + h.charAt(0), 16),
        parseInt(h.charAt(1) + h.charAt(1), 16),
        parseInt(h.charAt(2) + h.charAt(2), 16),
        a,
      )
    }
    if (h.length === 6 || h.length === 8) {
      const a = h.length === 8 ? parseInt(h.slice(6, 8), 16) / 255 : 1
      return pack(
        parseInt(h.slice(0, 2), 16),
        parseInt(h.slice(2, 4), 16),
        parseInt(h.slice(4, 6), 16),
        a,
      )
    }
  }

  const open = lower.indexOf('(')
  if (open > 0 && lower.charAt(lower.length - 1) === ')') {
    const fn = lower.slice(0, open).trim()
    const body = lower.slice(open + 1, lower.length - 1)
    const slash = body.indexOf('/')
    const head = slash >= 0 ? body.slice(0, slash) : body
    const tail = slash >= 0 ? body.slice(slash + 1) : ''
    const parts = head.split(/[\s,]+/)
    const args: string[] = []
    for (let i = 0; i < parts.length; i++) {
      if (parts[i] !== '') args.push(parts[i])
    }
    const alphaToken = slash >= 0 ? tail.trim() : undefined

    if (fn === 'rgb' || fn === 'rgba') {
      const a =
        alphaToken !== undefined
          ? channel(alphaToken, 1)
          : args[3] !== undefined
            ? channel(args[3], 1)
            : 1
      return pack(channel(args[0], 255), channel(args[1], 255), channel(args[2], 255), a)
    }
    if (fn === 'hsl' || fn === 'hsla') {
      const a =
        alphaToken !== undefined
          ? channel(alphaToken, 1)
          : args[3] !== undefined
            ? channel(args[3], 1)
            : 1
      const rgb = hslToRgb(angle(args[0]), channel(args[1], 1), channel(args[2], 1))
      return pack(rgb[0], rgb[1], rgb[2], a)
    }
    if (fn === 'oklch') {
      const a =
        alphaToken !== undefined
          ? channel(alphaToken, 1)
          : args[3] !== undefined
            ? channel(args[3], 1)
            : 1
      const L = channel(args[0], 1)
      const C = channel(args[1], 0.4)
      const h = (angle(args[2]) * Math.PI) / 180
      const rgb = oklabToRgb(L, C * Math.cos(h), C * Math.sin(h))
      return pack(rgb[0], rgb[1], rgb[2], a)
    }
    if (fn === 'oklab') {
      const a =
        alphaToken !== undefined
          ? channel(alphaToken, 1)
          : args[3] !== undefined
            ? channel(args[3], 1)
            : 1
      const rgb = oklabToRgb(channel(args[0], 1), channel(args[1], 0.4), channel(args[2], 0.4))
      return pack(rgb[0], rgb[1], rgb[2], a)
    }
    if (fn === 'color' && (args[0] === 'srgb' || args[0] === 'srgb-linear')) {
      const a =
        alphaToken !== undefined
          ? channel(alphaToken, 1)
          : args[4] !== undefined
            ? channel(args[4], 1)
            : 1
      let r = channel(args[1], 1)
      let g = channel(args[2], 1)
      let b = channel(args[3], 1)
      if (args[0] === 'srgb-linear') {
        return pack(gamma(r), gamma(g), gamma(b), a)
      }
      r = Math.max(0, Math.min(1, r))
      g = Math.max(0, Math.min(1, g))
      b = Math.max(0, Math.min(1, b))
      return pack(r * 255, g * 255, b * 255, a)
    }
  }

  // Last resort: let the engine resolve it. `copy` compositing writes the
  // source unblended, so getImageData round-trips the exact colour (8-bit,
  // which is the precision the compositor works in anyway).
  try {
    const doc = typeof document !== 'undefined' ? document : null
    if (doc) {
      const canvas = doc.createElement('canvas')
      canvas.width = 1
      canvas.height = 1
      const ctx = canvas.getContext('2d')
      if (ctx) {
        ctx.globalCompositeOperation = 'copy'
        ctx.fillStyle = '#000000'
        ctx.fillStyle = value
        ctx.fillRect(0, 0, 1, 1)
        const d = ctx.getImageData(0, 0, 1, 1).data
        return pack(d[0], d[1], d[2], d[3] / 255)
      }
    }
  } catch {
    /* no canvas (jsdom, or a hardened context) — fall through */
  }
  return '#00000000'
}

/**
 * `font-weight` keyword or number → numeric. Deliberately **unclamped**: v1.1
 * accepts the full CSS 1–1000 range, so a variable font's legitimate 850 must
 * survive rather than be rounded onto the 100s our own tokens happen to use.
 */
export function oracleFontWeight(input: string | number | null | undefined): number {
  if (typeof input === 'number') return isFinite(input) ? input : 400
  const v = String(input === null || input === undefined ? '' : input)
    .trim()
    .toLowerCase()
  if (v === '' || v === 'normal') return 400
  if (v === 'bold') return 700
  if (v === 'lighter') return 100
  if (v === 'bolder') return 700
  const n = parseFloat(v)
  return isFinite(n) ? n : 400
}

/** `font-family` list → the resolved *first* family, unquoted. */
export function oracleFirstFontFamily(input: string | null | undefined): string {
  const raw = String(input === null || input === undefined ? '' : input)
  const first = raw.split(',')[0] || ''
  return first.trim().replace(/^["']/, '').replace(/["']$/, '')
}

/**
 * `line-height` → px. `normal` is not a number, so the contract cannot carry it;
 * `normalPx` is the measured used value (see {@link oracleMeasureNormalLineHeight}).
 */
export function oracleResolveLineHeight(
  input: string | null | undefined,
  fontSize: number,
  normalPx: number,
): number {
  const size = isFinite(fontSize) ? fontSize : 0
  const v = String(input === null || input === undefined ? '' : input)
    .trim()
    .toLowerCase()
  if (v === '' || v === 'normal') {
    return normalPx > 0 ? normalPx : size * 1.2
  }
  if (v.slice(-2) === 'px') {
    const n = parseFloat(v)
    return isFinite(n) ? n : size * 1.2
  }
  if (v.charAt(v.length - 1) === '%') {
    const n = parseFloat(v)
    return isFinite(n) ? (n / 100) * size : size * 1.2
  }
  const n = parseFloat(v)
  return isFinite(n) ? n * size : size * 1.2
}

/**
 * Measure what `line-height: normal` actually resolves to for a given font, by
 * laying one line out in an off-screen probe. `getComputedStyle` returns the
 * literal string `normal`, and the contract requires px.
 */
export function oracleMeasureNormalLineHeight(
  el: Element,
  style: {
    fontFamily: string
    fontSize: string
    fontWeight: string
    fontStyle: string
    fontStretch?: string
  },
): number {
  const doc = el.ownerDocument
  const body = doc ? doc.body : null
  if (!doc || !body) return 0
  const probe = doc.createElement('div')
  probe.textContent = 'Hxg'
  const s = probe.style
  s.position = 'absolute'
  s.top = '-99999px'
  s.left = '-99999px'
  s.visibility = 'hidden'
  s.whiteSpace = 'nowrap'
  s.margin = '0'
  s.padding = '0'
  s.border = '0'
  s.lineHeight = 'normal'
  s.fontFamily = style.fontFamily
  s.fontSize = style.fontSize
  s.fontWeight = style.fontWeight
  s.fontStyle = style.fontStyle
  if (style.fontStretch) s.fontStretch = style.fontStretch
  body.appendChild(probe)
  // `finally` so a throwing measurement still takes the probe back out — an
  // orphaned probe would sit in the document under test for every later anchor.
  try {
    return probe.getBoundingClientRect().height
  } finally {
    body.removeChild(probe)
  }
}

/** Border-box rect minus the root's origin — ANCHORS.md §4. */
export function oracleRelativeBounds(
  box: { left: number; top: number; width: number; height: number },
  rootBox: { left: number; top: number; width: number; height: number },
): { x: number; y: number; w: number; h: number } {
  return {
    x: oracleRound(box.left - rootBox.left),
    y: oracleRound(box.top - rootBox.top),
    w: oracleRound(box.width),
    h: oracleRound(box.height),
  }
}

/**
 * Is the text visually truncated in this box?
 *
 * `scrollWidth`/`clientWidth` are the contract's stated signal but both are
 * **rounded to integers**, so a 100.4px string in a 100px box reads as a 1px
 * overflow that never paints an ellipsis. When the unclipped advance width and
 * the content-box width are available — they are for every text anchor, both
 * fractional — they decide instead, with an epsilon absorbing the sub-pixel case.
 */
export function oracleIsClipped(params: {
  scrollWidth: number
  clientWidth: number
  textWidth?: number | null
  contentWidth?: number | null
  epsilon?: number
}): boolean {
  const epsilon = typeof params.epsilon === 'number' ? params.epsilon : 0.5
  const textWidth = params.textWidth
  const contentWidth = params.contentWidth
  if (
    typeof textWidth === 'number' &&
    isFinite(textWidth) &&
    typeof contentWidth === 'number' &&
    isFinite(contentWidth) &&
    contentWidth > 0
  ) {
    return textWidth - contentWidth > epsilon
  }
  const scrollWidth = typeof params.scrollWidth === 'number' ? params.scrollWidth : 0
  const clientWidth = typeof params.clientWidth === 'number' ? params.clientWidth : 0
  return scrollWidth - clientWidth > epsilon
}

/**
 * Whether the element declares itself content-sized (ANCHORS.md v1.5).
 *
 * `data-oracle-content-sized` present and not literally `"false"`. React
 * renders `data-x={true}` as `="true"` and `data-x={false}` as `="false"`, and
 * a bare attribute in hand-written markup arrives as `""` — all three read the
 * way an author would expect, and anything unrecognised is *false*, so a
 * typo opens no blind spot on this side that the differ cannot see: the other
 * extractor's `true` then shows up as a `FieldPresence` delta.
 */
export function oracleContentSized(el: Element): boolean {
  const raw = el.getAttribute('data-oracle-content-sized')
  if (raw === null) return false
  return String(raw).trim().toLowerCase() !== 'false'
}

/**
 * Whether the element declares its box height to be its own line box
 * (ANCHORS.md v1.6).
 *
 * `data-oracle-line-sized` present and not literally `"false"` — the same three
 * spellings {@link oracleContentSized} accepts, for the same reason: React
 * renders `data-x={true}` as `="true"`, `data-x={false}` as `="false"`, and a
 * bare attribute in hand-written markup arrives as `""`.
 *
 * A separate reader rather than a shared one taking the attribute name: these
 * are two independent claims about an anchor, an element may make either, both
 * or neither, and a helper parameterised by a string is one typo away from
 * reading the wrong flag with nothing to catch it.
 */
export function oracleLineSized(el: Element): boolean {
  const raw = el.getAttribute('data-oracle-line-sized')
  if (raw === null) return false
  return String(raw).trim().toLowerCase() !== 'false'
}

/** The element's *own* text nodes — never a descendant's. */
export function oracleOwnTextNodes(el: Element): Text[] {
  const out: Text[] = []
  const nodes = el.childNodes
  for (let i = 0; i < nodes.length; i++) {
    const node = nodes[i]
    if (node.nodeType === 3 && node.nodeValue !== null && node.nodeValue !== '') {
      out.push(node as Text)
    }
  }
  return out
}

/** Full own text, before any visual truncation. */
export function oracleOwnText(el: Element): string {
  const nodes = oracleOwnTextNodes(el)
  let out = ''
  for (let i = 0; i < nodes.length; i++) out += nodes[i].nodeValue
  return out
}

/**
 * Rendered advance width of the element's own text, **unclipped**.
 *
 * A `Range` reports layout, which is not clipped by `overflow: hidden` and is
 * unaffected by `text-overflow: ellipsis` (a paint-time effect). `offsetWidth`
 * would report the clipped box instead and is deliberately not used.
 */
export function oracleTextAdvanceWidth(el: Element): number {
  const nodes = oracleOwnTextNodes(el)
  if (nodes.length === 0) return 0
  const doc = el.ownerDocument
  if (!doc || typeof doc.createRange !== 'function') return 0
  let total = 0
  for (let i = 0; i < nodes.length; i++) {
    const range = doc.createRange()
    range.selectNodeContents(nodes[i])
    let width = 0
    const rects = typeof range.getClientRects === 'function' ? range.getClientRects() : null
    if (rects && rects.length > 0) {
      for (let j = 0; j < rects.length; j++) width += rects[j].width
    } else if (typeof range.getBoundingClientRect === 'function') {
      width = range.getBoundingClientRect().width
    }
    total += width
  }
  return total
}

/**
 * The host's padding box, which is what `position:absolute; inset:0` resolves
 * against — so it is the box of a pseudo-element backing an anchor (§3).
 */
export function oraclePaddingBoxRect(
  el: Element,
  style: {
    borderLeftWidth: string
    borderTopWidth: string
    borderRightWidth: string
    borderBottomWidth: string
  },
): { left: number; top: number; width: number; height: number } {
  const r = el.getBoundingClientRect()
  const bl = parseFloat(style.borderLeftWidth) || 0
  const bt = parseFloat(style.borderTopWidth) || 0
  const br = parseFloat(style.borderRightWidth) || 0
  const bb = parseFloat(style.borderBottomWidth) || 0
  return {
    left: r.left + bl,
    top: r.top + bt,
    width: r.width - bl - br,
    height: r.height - bt - bb,
  }
}

/** Actually painted: ANCHORS.md §3's `visible`. */
export function oracleIsVisible(
  el: Element,
  box: { left: number; top: number; width: number; height: number },
): boolean {
  const doc = el.ownerDocument
  const win = doc ? doc.defaultView : null
  if (!win || typeof win.getComputedStyle !== 'function') return false
  const style = win.getComputedStyle(el)
  if (style.display === 'none') return false
  if (style.visibility === 'hidden' || style.visibility === 'collapse') return false
  if (parseFloat(style.opacity) === 0) return false
  if (!(box.width > 0 && box.height > 0)) return false

  let left = box.left
  let top = box.top
  let right = box.left + box.width
  let bottom = box.top + box.height
  let node: Element | null = el.parentElement
  while (node) {
    const s = win.getComputedStyle(node)
    if (s.display === 'none') return false
    if (parseFloat(s.opacity) === 0) return false
    if (s.overflowX !== 'visible' || s.overflowY !== 'visible') {
      const nr = node.getBoundingClientRect()
      const l = Math.max(left, nr.left)
      const t = Math.max(top, nr.top)
      const r = Math.min(right, nr.left + nr.width)
      const b = Math.min(bottom, nr.top + nr.height)
      if (!(r - l > 0 && b - t > 0)) return false
      left = l
      top = t
      right = r
      bottom = b
    }
    node = node.parentElement
  }
  return true
}

/**
 * `light` or `dark` — the only two values §2's `state.theme` permits.
 *
 * This app names its themes (`data-theme="crowbar"`), so the attribute is only
 * usable when it happens to *be* one of the two words. The luminance fallback is
 * what actually answers for a named theme: dark chrome is dark whatever it calls
 * itself, and guessing `light` because the name was unfamiliar would refuse
 * every comparison in the run.
 */
export function oracleDetectTheme(doc: Document): 'light' | 'dark' {
  const el = doc.documentElement
  const win = doc.defaultView
  if (el && el.classList) {
    if (el.classList.contains('dark')) return 'dark'
    if (el.classList.contains('light')) return 'light'
  }
  if (el) {
    const attr = (el.getAttribute('data-theme') || '').trim().toLowerCase()
    if (attr === 'dark' || attr === 'light') return attr
  }
  if (win && typeof win.getComputedStyle === 'function' && el) {
    const scheme = String(win.getComputedStyle(el).colorScheme || '')
      .trim()
      .toLowerCase()
    if (scheme === 'dark') return 'dark'
    if (scheme === 'light') return 'light'
    const surface = doc.body || el
    const hex = oracleNormalizeColor(win.getComputedStyle(surface).backgroundColor)
    const alpha = parseInt(hex.slice(7, 9), 16)
    if (alpha > 0) {
      const r = parseInt(hex.slice(1, 3), 16) / 255
      const g = parseInt(hex.slice(3, 5), 16) / 255
      const b = parseInt(hex.slice(5, 7), 16) / 255
      return 0.2126 * r + 0.7152 * g + 0.0722 * b < 0.5 ? 'dark' : 'light'
    }
  }
  if (win && typeof win.matchMedia === 'function') {
    if (win.matchMedia('(prefers-color-scheme: dark)').matches) return 'dark'
  }
  return 'light'
}

/**
 * The v1.1 `state` schema: exactly four keys, fixed vocabulary, `flags` as a
 * sorted duplicate-free set.
 *
 * A bad value throws here rather than being emitted. The differ's response to an
 * unknown value is to refuse the comparison, and a refusal that shows up as
 * "0 deltas" three steps later is far harder to trace than a failure at the
 * point the wrong string was written.
 */
export function oracleNormalizeState(
  requested: Partial<OracleState> | undefined | null,
  derivedWidth: number,
  derivedTheme: 'light' | 'dark',
): OracleState {
  const wanted = requested || {}

  let width = wanted.width === undefined || wanted.width === null ? derivedWidth : wanted.width
  width = Math.round(Number(width))
  if (!isFinite(width)) width = 0

  let theme = derivedTheme
  if (wanted.theme !== undefined && wanted.theme !== null) {
    const t = String(wanted.theme).trim().toLowerCase()
    if (t !== 'light' && t !== 'dark') {
      throw new Error(
        'oracle: state.theme must be "light" or "dark", got ' + JSON.stringify(wanted.theme),
      )
    }
    theme = t as 'light' | 'dark'
  }

  let content: OracleContent = 'normal'
  if (wanted.content !== undefined && wanted.content !== null) {
    const c = String(wanted.content).trim().toLowerCase()
    if (c !== 'short' && c !== 'normal' && c !== 'overflow') {
      throw new Error(
        'oracle: state.content must be "short", "normal" or "overflow", got ' +
          JSON.stringify(wanted.content),
      )
    }
    content = c as OracleContent
  }

  const allowed = ['empty', 'loading', 'error', 'hover', 'focus', 'selected']
  const flags: string[] = []
  const raw = wanted.flags === undefined || wanted.flags === null ? [] : wanted.flags
  if (!(raw instanceof Array)) {
    throw new Error('oracle: state.flags must be an array')
  }
  for (let i = 0; i < raw.length; i++) {
    const f = String(raw[i]).trim().toLowerCase()
    if (allowed.indexOf(f) < 0) {
      throw new Error(
        'oracle: state.flags may only contain ' +
          allowed.join(', ') +
          ', got ' +
          JSON.stringify(raw[i]),
      )
    }
    if (flags.indexOf(f) < 0) flags.push(f)
  }
  flags.sort()

  return { width: width, theme: theme, content: content, flags: flags as OracleFlag[] }
}

/**
 * Walk every `data-oracle-id` under (and including) the root anchor and emit a
 * v1 snapshot. Everything is read from `getComputedStyle` on the live element —
 * never inferred from class names, because the tree CSS overrides the component's
 * Tailwind classes (`rounded-md` → `border-radius: 2px !important`,
 * `hover:bg-muted` killed outright) and a class list would lie.
 */
export function extractSnapshot(options: ExtractOptions): OracleSnapshot {
  const opts = options || ({} as ExtractOptions)
  const rootId = opts.root || 'git-row-item'
  const index = opts.index || 0
  const pseudoMap = opts.pseudo || {
    'git-row-item': '::before',
    'file-row-item': '::before',
  }

  let scope: Document | Element = document
  if (typeof opts.scope === 'string') {
    const found = document.querySelector(opts.scope)
    if (!found) throw new Error('oracle: scope not found for selector ' + opts.scope)
    scope = found
  } else if (opts.scope) {
    scope = opts.scope as Element
  }

  const roots = scope.querySelectorAll('[data-oracle-id="' + rootId + '"]')
  const rootEl = roots[index] as HTMLElement | undefined
  if (!rootEl) {
    throw new Error(
      'oracle: root anchor "' +
        rootId +
        '" #' +
        index +
        ' not found (' +
        roots.length +
        ' present)',
    )
  }

  const doc = rootEl.ownerDocument
  const win = doc.defaultView
  if (!win) throw new Error('oracle: root anchor has no window')

  const elements: Element[] = [rootEl]
  const nested = rootEl.querySelectorAll('[data-oracle-id]')
  for (let i = 0; i < nested.length; i++) elements.push(nested[i])

  const normalLineHeights: Record<string, number> = {}
  const anchors: OracleAnchor[] = []
  let rootBox: OracleBox = { left: 0, top: 0, width: 0, height: 0 }

  for (let i = 0; i < elements.length; i++) {
    const el = elements[i] as HTMLElement
    const id = el.getAttribute('data-oracle-id') || ''
    const style = win.getComputedStyle(el)

    // Pseudo-backed anchors (§3): the paint lives on a pseudo-element that has
    // no DOM node and cannot carry the attribute. Only valid while the pseudo
    // is `position:absolute; inset:0`, which resolves to the host's padding box.
    let pseudoStyle: CSSStyleDeclaration | null = null
    const pseudoSelector = pseudoMap[id]
    if (pseudoSelector) {
      const candidate = win.getComputedStyle(el, pseudoSelector)
      if (candidate && candidate.content !== 'none') pseudoStyle = candidate
    }

    let box: OracleBox
    if (pseudoStyle) {
      box = oraclePaddingBoxRect(el, style)
    } else {
      const r = el.getBoundingClientRect()
      box = { left: r.left, top: r.top, width: r.width, height: r.height }
    }
    if (i === 0) rootBox = box

    const paint = pseudoStyle || style
    let radius = parseFloat(paint.borderTopLeftRadius) || 0
    if (String(paint.borderTopLeftRadius).indexOf('%') >= 0) {
      radius = (parseFloat(paint.borderTopLeftRadius) / 100) * box.width
    }

    const record: OracleAnchor = {
      id: id,
      bounds: oracleRelativeBounds(box, rootBox),
      bg: oracleNormalizeColor(paint.backgroundColor),
      visible: oracleIsVisible(el, box),
      radius: oracleRound(radius),
      border: {
        w: oracleRound(parseFloat(paint.borderTopWidth) || 0),
        color: oracleNormalizeColor(paint.borderTopColor),
      },
    }

    // v1.5: emitted only when true. Absent *is* false in the contract, so a
    // `false` here would be a key the GPUI side does not write and this one
    // does — a difference in the wire shape that says nothing about the UI.
    if (oracleContentSized(el)) {
      record.content_sized = true
    }

    // v1.6, and emitted the same way and for the same reason. Written here
    // rather than inside the text branch below on purpose: an author who puts
    // `data-oracle-line-sized` on a box that paints no text has made a mistake,
    // and the differ's job is to refuse that document by name. Emitting it only
    // where a font happens to exist would swallow the mistake instead.
    if (oracleLineSized(el)) {
      record.line_sized = true
    }

    const text = oracleOwnText(el)
    if (text.length > 0 && text.replace(/\s/g, '') !== '') {
      const fontSize = parseFloat(style.fontSize) || 0
      let normalPx = 0
      if (String(style.lineHeight).trim().toLowerCase() === 'normal') {
        const key =
          style.fontFamily + '|' + style.fontSize + '|' + style.fontWeight + '|' + style.fontStyle
        if (normalLineHeights[key] === undefined) {
          normalLineHeights[key] = oracleMeasureNormalLineHeight(el, style)
        }
        normalPx = normalLineHeights[key]
      }
      const textWidth = oracleTextAdvanceWidth(el)
      const contentWidth =
        box.width -
        (parseFloat(style.paddingLeft) || 0) -
        (parseFloat(style.paddingRight) || 0) -
        (parseFloat(style.borderLeftWidth) || 0) -
        (parseFloat(style.borderRightWidth) || 0)

      record.fg = oracleNormalizeColor(style.color)
      record.text = text
      record.text_width = oracleRound(textWidth)
      record.clipped = oracleIsClipped({
        scrollWidth: el.scrollWidth,
        clientWidth: el.clientWidth,
        textWidth: textWidth,
        contentWidth: contentWidth,
      })
      record.font = {
        size: oracleRound(fontSize),
        weight: oracleFontWeight(style.fontWeight),
        family: oracleFirstFontFamily(style.fontFamily),
        line_height: oracleRound(oracleResolveLineHeight(style.lineHeight, fontSize, normalPx)),
      }
    }

    anchors.push(record)
  }

  // §4, made a load error in v1.1: the root must be present and at the origin.
  // The arithmetic above already guarantees it; this catches the refactor that
  // stops guaranteeing it, which would otherwise ship window-absolute
  // coordinates and offset every anchor by a constant.
  const first = anchors[0]
  if (!first || first.id !== rootId || first.bounds.x !== 0 || first.bounds.y !== 0) {
    throw new Error('oracle: root anchor "' + rootId + '" is not at the origin of its own snapshot')
  }

  return {
    schema: 1,
    surface: opts.surface || 'unknown',
    state: oracleNormalizeState(opts.state, rootBox.width, oracleDetectTheme(doc)),
    root: rootId,
    anchors: anchors,
  }
}

// ─── injection ───────────────────────────────────────────────────────────────

/**
 * Every function the injected script needs, in declaration order. Adding a
 * runtime helper without adding it here is a `ReferenceError` in the page and
 * nowhere else — the module path keeps working.
 */
const ORACLE_RUNTIME = [
  oracleRound,
  oracleNormalizeColor,
  oracleFontWeight,
  oracleFirstFontFamily,
  oracleResolveLineHeight,
  oracleMeasureNormalLineHeight,
  oracleRelativeBounds,
  oracleIsClipped,
  oracleContentSized,
  oracleLineSized,
  oracleOwnTextNodes,
  oracleOwnText,
  oracleTextAdvanceWidth,
  oraclePaddingBoxRect,
  oracleIsVisible,
  oracleDetectTheme,
  oracleNormalizeState,
  extractSnapshot,
]

/**
 * A self-contained IIFE that evaluates to the snapshot **as a JSON string**,
 * for pasting into a page over an `execute_js` bridge where nothing can be
 * imported. Depends only on the DOM.
 *
 * `scope` must be a selector string here: an `Element` cannot survive
 * serialisation into source, and silently dropping it would snapshot the wrong
 * row, so it is rejected instead.
 */
export function extractSnapshotSource(options: ExtractOptions): string {
  if (options && options.scope !== undefined && typeof options.scope !== 'string') {
    throw new Error('oracle: extractSnapshotSource needs `scope` as a CSS selector string')
  }
  const runtime = ORACLE_RUNTIME.map((fn) => fn.toString()).join('\n\n')
  return (
    '(function () {\n' +
    runtime +
    '\n\nreturn JSON.stringify(extractSnapshot(' +
    JSON.stringify(options || {}) +
    '), null, 2)\n})()'
  )
}
