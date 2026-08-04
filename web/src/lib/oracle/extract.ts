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
   *
   * A supplied `theme` is additionally checked **against the document being
   * measured**, and a contradiction throws — see {@link oracleNormalizeState}.
   * `content` and `flags` are not properties of the document and are taken on
   * trust; `width` is the *viewport* width, which nothing here measures.
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
   * (ANCHORS.md §3). Only valid when the pseudo is `position:absolute` — its
   * box is read from its own resolved inset ({@link oraclePseudoBoxRect}), not
   * assumed to be `inset:0` (P3.34).
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
 * Does anything actually hide this element's horizontal overflow — its own box,
 * or an ancestor's? ANCHORS.md v1.12, and the term {@link oracleIsClipped} was
 * missing.
 *
 * ## Which `overflow` values hide
 *
 * **Every one except `visible`**, which is the same test {@link oracleIsVisible}
 * already applies one function below, and it is correct by construction rather
 * than by enumeration: CSS defines `visible` as the *only* value that does not
 * establish a clip. `hidden` and `clip` clip outright; `scroll` and `auto` clip
 * the painted content to the padding box and make the rest reachable only by
 * scrolling, which is still not painted at this scroll offset; `overlay` is a
 * legacy alias that computes to `auto` in this engine and would be covered even
 * if it did not. An unrecognised future value therefore reads as *hiding*, which
 * is the direction that cannot silently drop a truncation.
 *
 * The empty string is the one exception, and it means "the style object does not
 * carry this longhand" rather than any CSS value — it is read as **not** hiding,
 * so a partial style stub cannot manufacture a truncation.
 *
 * ## Only `overflow-x`
 *
 * `text_width` and `clipped` are about the *inline* direction. A vertical-only
 * clip does not truncate a line, and it cannot hide behind this: CSS computes
 * `overflow-x: visible` to `auto` whenever the other axis is not `visible`, so
 * `overflow-y: hidden` arrives here as `overflow-x: auto` and is caught.
 *
 * ## `text-overflow` is deliberately not consulted
 *
 * It is **inert** without a hiding `overflow` — CSS UI applies it only to a block
 * container whose overflow is other than `visible` — so it can never be the thing
 * that hides a glyph. v1.12's own motivating element proves the converse
 * direction too: it computes `text-overflow: clip` and is not truncated at all.
 * Reading it would add a condition that is either redundant or wrong.
 *
 * ## Ancestors are walked, and **intersected** — not merely tested
 *
 * v1.12 says "the element's own box, **or an ancestor**". A CSS-only ancestor
 * test would be a tautology in this app and the ruling would do nothing:
 * `index.css` puts `overflow: hidden` on `html`, `body` **and** `#root`, so every
 * anchor in every live capture has three clipping ancestors. So an ancestor
 * counts only where its rect **actually cuts** this element's box, exactly as
 * {@link oracleIsVisible} intersects rather than asking whether a clip exists.
 * The viewport does not cut a row that is on screen; a narrow scrollport does.
 *
 * The element's **own** box is not treated geometrically, because v1.3 ruling 1
 * already answers that question more precisely than a rect can: `clientWidth` is
 * an integer and `text_width` against the fractional content box is not.
 *
 * **Known imprecision, stated rather than hidden:** the ancestor probe is this
 * element's border box, so an ancestor edge that cuts only text spilling *past*
 * that box — `justify-center` on a box narrower than its glyph — is not seen. The
 * exact probe is the text's own laid-out rect, which the contract does not carry
 * and which would mean a second `Range` pass over every text anchor.
 */
export function oracleOverflowHidesText(
  el: Element,
  box: { left: number; top: number; width: number; height: number },
  epsilon?: number,
): boolean {
  function hides(value: string | null | undefined, fallback: string | null | undefined): boolean {
    let v = String(value === null || value === undefined ? '' : value)
      .trim()
      .toLowerCase()
    if (v === '') {
      v = String(fallback === null || fallback === undefined ? '' : fallback)
        .trim()
        .toLowerCase()
    }
    return v !== '' && v !== 'visible'
  }

  const eps = typeof epsilon === 'number' ? epsilon : 0.5
  const doc = el.ownerDocument
  const win = doc ? doc.defaultView : null
  if (!win || typeof win.getComputedStyle !== 'function') return false

  const own = win.getComputedStyle(el)
  if (hides(own.overflowX, own.overflow)) return true

  const left = box.left
  const right = box.left + box.width
  let node: Element | null = el.parentElement
  while (node) {
    const s = win.getComputedStyle(node)
    if (hides(s.overflowX, s.overflow)) {
      const nr = node.getBoundingClientRect()
      if (nr.left - left > eps || right - (nr.left + nr.width) > eps) return true
    }
    node = node.parentElement
  }
  return false
}

/**
 * Is the text visually truncated in this box?
 *
 * `scrollWidth`/`clientWidth` are the contract's stated signal but both are
 * **rounded to integers**, so a 100.4px string in a 100px box reads as a 1px
 * overflow that never paints an ellipsis. When the unclipped advance width and
 * the content-box width are available — they are for every text anchor, both
 * fractional — they decide instead, with an epsilon absorbing the sub-pixel case.
 *
 * ## `overflowHidden` — ANCHORS.md v1.12, and why it is required rather than
 * defaulted
 *
 * Overflowing a box and being *truncated* are two different facts, and v1.3
 * ruling 1 conflated them when it improved the precision: it replaced
 * `scrollWidth > clientWidth`, which is `overflow`-aware, with a fractional
 * content-box test that is true for **any** overflowing text, hidden or not. §3
 * defines the field as "whether the text is **visually truncated** in this box",
 * and a glyph painted in full is not truncated — P3.10's callout emoji spills
 * 2.5px past each edge of a 24px box under `overflow-x: visible` and nothing
 * hides it.
 *
 * So the caller must say whether anything hides the overflow — see
 * {@link oracleOverflowHidesText}, which is what answers it from the DOM. It is a
 * required field and not a defaulted one on purpose: a default of `true` would
 * leave the old behaviour one forgotten argument away, on a field whose whole
 * job is to say something about text.
 *
 * The term gates the `scrollWidth`/`clientWidth` fallback as well as the
 * fractional test. That path is only meaningful for a scroll container anyway,
 * and it is the same question either way.
 */
export function oracleIsClipped(params: {
  scrollWidth: number
  clientWidth: number
  /** Does the element's own box, or an ancestor's, hide the overflow? (v1.12) */
  overflowHidden: boolean
  textWidth?: number | null
  contentWidth?: number | null
  epsilon?: number
}): boolean {
  if (params.overflowHidden !== true) return false
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
 * The host's padding box — what `position:absolute; inset:0` resolves
 * against, and therefore the box of an `inset:0` pseudo-element backing an
 * anchor (§3). For any other inset, {@link oraclePseudoBoxRect} offsets from
 * this box rather than returning it directly.
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

/**
 * A CSS length that has already been resolved to a definite pixel value, or
 * `null` if it has not.
 *
 * ANCHORS.md §3's pseudo shortcut (below) must never guess: `getComputedStyle`
 * is asked for a *resolved* value, and this is the check that the answer it
 * gave back was actually resolved — a plain `<number>px` — rather than a
 * literal `auto`, an unresolved percentage, or anything else CSSOM permits a
 * UA to hand back when it cannot compute a used value.
 */
export function oracleParsePx(raw: string): number | null {
  if (typeof raw !== 'string') return null
  const trimmed = raw.trim()
  if (!/^-?\d+(\.\d+)?px$/.test(trimmed)) return null
  const value = parseFloat(trimmed)
  return isFinite(value) ? value : null
}

/**
 * The pseudo's *own* box — ANCHORS.md §3, generalised past the `inset:0` case
 * {@link oraclePaddingBoxRect} alone handles. P3.34: `extract.ts` used to
 * return the host's padding box for every pseudo-backed anchor, on the stated
 * assumption that the pseudo is always `inset:0`. `slider`'s track pseudo is
 * `before:inset-x-0.5` — inset 2px on each side, not 0 — and the shortcut
 * reported `{x:0, w:668}` where the true box is `{x:2, w:664}`.
 *
 * ## Mechanism
 *
 * `top`/`right`/`bottom`/`left` and `margin-*` are exactly the properties
 * CSSOM's resolved-value algorithm reports as the **used** value — a definite
 * px length — rather than the literal specified value, for any absolutely
 * positioned, box-generating element, regardless of whether the author wrote
 * a length, a percentage, or left the property `auto`. The abspos placement
 * algorithm (CSS2.1 §10.3.7 / §10.6.4) resolves every one of the four insets
 * to a concrete number before layout is done — that is precisely what makes
 * an otherwise over/under-constrained system solvable — so reading them back
 * is asking the engine for its own answer, not re-deriving one by hand.
 *
 * Offsetting the host's padding box by the resolved left/top, then shrinking
 * it by left+right and top+bottom, handles every shape in one formula without
 * this function ever needing to know which sides the author actually wrote:
 *
 * - `inset: 0` — all four resolve to `0`, so the result is the host's padding
 *   box exactly: adding and subtracting zero is exact in floating point, so
 *   this reproduces {@link oraclePaddingBoxRect}'s answer bit-for-bit, not
 *   merely approximately. `git-row-item` and `file-row-item` are `inset: 0`
 *   (`.file-tree-item::before`) and stay on this identity.
 * - `inset-x-0.5` (the `slider` track) — left/right resolve to the authored
 *   `2px`, so `width = hostWidth - 4` and `left = hostLeft + 2`.
 * - A pseudo positioned by `left` + `width` alone, `right` left `auto` in the
 *   source: the abspos algorithm still solves a concrete *used* `right` that
 *   balances the equation, so subtracting it reproduces the authored width —
 *   this function never has to read `width` itself to get that case right.
 *
 * ## Refusals — loud, not a plausible-looking wrong number
 *
 * - Anything but `position: absolute` is out of scope: ANCHORS.md §3 only
 *   describes this shortcut for an absolutely positioned pseudo, and
 *   `position: relative`'s offset semantics (an author's `right` can be
 *   *present but ignored*) do not fit the same formula.
 * - A non-zero (or unresolved) margin refuses outright. Insetting from the
 *   host's padding box lands on the pseudo's *margin* edge, and folding
 *   margin into the formula too is unexercised by anything in this codebase
 *   today — better to refuse than to add an untested term.
 * - Any of the six lengths not resolving to a plain `<number>px` refuses by
 *   name: a literal `auto` surviving resolution, or a percentage the engine
 *   could not compute against an indefinite containing block. This is the
 *   "percentage insets that don't resolve" case — nothing here decides what
 *   such a value *should* mean, it stops instead.
 * - A box that would resolve to a negative width or height — the insets
 *   exceed the host's own box — refuses rather than emit a nonsensical size.
 */
export function oraclePseudoBoxRect(
  el: Element,
  hostStyle: {
    borderLeftWidth: string
    borderTopWidth: string
    borderRightWidth: string
    borderBottomWidth: string
  },
  pseudoStyle: {
    position: string
    left: string
    top: string
    right: string
    bottom: string
    marginLeft: string
    marginTop: string
    marginRight: string
    marginBottom: string
  },
  id: string,
): { left: number; top: number; width: number; height: number } {
  if (pseudoStyle.position !== 'absolute') {
    throw new Error(
      'oracle: pseudo-backed anchor "' +
        id +
        '" is `position: ' +
        pseudoStyle.position +
        "`, not `absolute` — ANCHORS.md §3's pseudo shortcut only covers an " +
        'absolutely positioned pseudo. Refusing to guess its box.',
    )
  }

  const marginLeft = oracleParsePx(pseudoStyle.marginLeft)
  const marginTop = oracleParsePx(pseudoStyle.marginTop)
  const marginRight = oracleParsePx(pseudoStyle.marginRight)
  const marginBottom = oracleParsePx(pseudoStyle.marginBottom)
  if (marginLeft !== 0 || marginTop !== 0 || marginRight !== 0 || marginBottom !== 0) {
    throw new Error(
      'oracle: pseudo-backed anchor "' +
        id +
        '" has a non-zero or unresolved margin on its pseudo (left=' +
        pseudoStyle.marginLeft +
        ' top=' +
        pseudoStyle.marginTop +
        ' right=' +
        pseudoStyle.marginRight +
        ' bottom=' +
        pseudoStyle.marginBottom +
        ") — this shortcut offsets from the host's padding box by inset alone " +
        'and does not account for margin. Refusing to guess its box.',
    )
  }

  const left = oracleParsePx(pseudoStyle.left)
  const top = oracleParsePx(pseudoStyle.top)
  const right = oracleParsePx(pseudoStyle.right)
  const bottom = oracleParsePx(pseudoStyle.bottom)
  if (left === null || top === null || right === null || bottom === null) {
    throw new Error(
      'oracle: pseudo-backed anchor "' +
        id +
        '" has an inset that did not resolve to a definite pixel length ' +
        '(left=' +
        pseudoStyle.left +
        ' top=' +
        pseudoStyle.top +
        ' right=' +
        pseudoStyle.right +
        ' bottom=' +
        pseudoStyle.bottom +
        '). Refusing to guess its box.',
    )
  }

  const host = oraclePaddingBoxRect(el, hostStyle)
  const width = host.width - left - right
  const height = host.height - top - bottom
  if (width < 0 || height < 0) {
    throw new Error(
      'oracle: pseudo-backed anchor "' +
        id +
        '" resolves to a negative box (' +
        width +
        'x' +
        height +
        ') after its inset is subtracted from the host padding box. Refusing ' +
        'to emit a negative size.',
    )
  }

  return {
    left: host.left + left,
    top: host.top + top,
    width,
    height,
  }
}

/**
 * Does this element generate a box at all? ANCHORS.md v1.11.
 *
 * An element that generates none **is not an anchor**, and the extractor skips
 * it rather than emitting a zero-rect record with `visible: false`. GPUI has no
 * such element in its tree, so the two sides otherwise disagree on *anchor
 * presence* — a structural delta on a resting cell, caused by the contract
 * rather than by the port. P3.9's checkbox tick, which is `display: none` when
 * unchecked, is the case that found it.
 *
 * ## The distinction that must not be blurred
 *
 * `visibility: hidden`, `opacity: 0` and being clipped by an ancestor all **do**
 * generate boxes. Those stay anchors, with `visible: false` — which is exactly
 * how the carousel's snapped-out panels and v1.7's opacity case are caught. Only
 * `display: none` generates nothing, and only it is tested here. This is
 * therefore a strictly narrower predicate than {@link oracleIsVisible}, and
 * deliberately so: the two must not be collapsed into one walk.
 *
 * ## Ancestors
 *
 * A descendant of a `display: none` element generates no box either, and
 * `getComputedStyle` will not say so — it reports the descendant's *own*
 * `display`, which is whatever it was authored as. So the walk is necessary
 * rather than defensive, and it stops where {@link oracleIsVisible}'s does.
 *
 * Without a window to ask, the answer is **yes**: filtering is the one operation
 * that can quietly make a capture smaller than the surface, and a shrinking
 * reference proves less every time it does.
 */
export function oracleGeneratesBox(el: Element): boolean {
  const doc = el.ownerDocument
  const win = doc ? doc.defaultView : null
  if (!win || typeof win.getComputedStyle !== 'function') return true
  let node: Element | null = el
  while (node) {
    if (win.getComputedStyle(node).display === 'none') return false
    node = node.parentElement
  }
  return true
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
 * Refuse a capture whose `visible` column the driver could not have reproduced
 * — ANCHORS.md v1.7, and `corpus/001-opacity-ancestor-visible.md`'s narrowed
 * standing restriction.
 *
 * ## The gap this closes over
 *
 * v1.7 decided that `visible` is false at zero opacity on the element **or any
 * ancestor**. The two sides implement different amounts of that sentence:
 *
 * | | sees zero opacity on |
 * |---|---|
 * | {@link oracleIsVisible} | the element and **every** ancestor — it walks `parentElement` to `<html>` |
 * | `crowbar-driver` | the element and **anchored** ancestors only |
 *
 * The driver folds opacity in at `AnchoredBox` boundaries
 * (`registry.enter(self.style.opacity)`), so a plain `div().opacity(0.)`
 * between two anchors — or an `opacity-0` layer *above* the root anchor, which
 * is exactly what `NavStack` puts on the sidebar carousel — is never entered
 * and never seen. It cannot be closed on that side: gpui accumulates precisely
 * this in `Window::element_opacity`, but it is `pub(crate)` with no accessor
 * and is applied from `Interactivity::paint`, after the `prepaint` where the
 * driver records. Reaching it means patching the pinned `native/vendor/`.
 *
 * So the gap stays, and this side — the one that *can* see the whole chain —
 * makes it unable to produce a wrong answer. A cell driven with such an
 * ancestor reports `visible: false` on every anchor here and `true` on every
 * anchor there: a delta on every anchor whose cause is the contract rather than
 * the port, and one that reads exactly like a port bug. That capture must not
 * exist, in the same way and for the same reason a mislabelled theme must not
 * (see {@link oracleNormalizeState}).
 *
 * ## The region searched
 *
 * Every ancestor of every anchor, up to and including `<html>` — which is the
 * region {@link oracleIsVisible} reads, so the guard trips on exactly the
 * elements that can move this side's answer. That is deliberately *both*
 * directions the corpus entry names: between two anchors, and **above the root
 * anchor**, the latter being the motivating `NavStack` case.
 *
 * ## Anchored ancestors are exempt, and that matters
 *
 * An ancestor carrying `data-oracle-id` is one the driver enters, so both sides
 * fold it in and the capture *is* comparable. The corpus entry's advice for
 * testing opacity — "put it on an anchor, the root anchor will do" — depends on
 * this exemption, so a guard that tripped on it would block the one legitimate
 * way to exercise the field.
 *
 * ## Exactly zero, not merely translucent
 *
 * `opacity: 0.5` on an unanchored ancestor changes `visible` on **neither**
 * side: this side tests `parseFloat(style.opacity) === 0` and the driver tests
 * `declared == 0.0`, both per level rather than as a product. There is no
 * disagreement to prevent, so flagging it would be a false alarm — and a false
 * alarm here throws away a legitimate capture, on a tree where `opacity-90`
 * wrappers are ordinary. The predicate is therefore the same `=== 0` the two
 * `visible` implementations already use, so the guard trips if and only if an
 * ancestor the driver cannot see is one this side would have called invisible.
 */
export function oracleAssertComparableOpacity(elements: Element[]): void {
  function describe(el: Element): string {
    let out = '<' + String(el.tagName || 'element').toLowerCase()
    const id = el.getAttribute('id')
    if (id) out += ' id="' + id + '"'
    const cls = el.getAttribute('class')
    if (cls) out += ' class="' + cls + '"'
    return out + '>'
  }

  const seen: Element[] = []
  for (let i = 0; i < elements.length; i++) {
    const el = elements[i]
    const doc = el.ownerDocument
    const win = doc ? doc.defaultView : null
    if (!win || typeof win.getComputedStyle !== 'function') continue
    let node: Element | null = el.parentElement
    while (node) {
      // Already walked, so everything above it was walked too — this loop only
      // ever ends by reaching the top or by throwing.
      if (seen.indexOf(node) >= 0) break
      seen.push(node)
      if (
        node.getAttribute('data-oracle-id') === null &&
        parseFloat(win.getComputedStyle(node).opacity) === 0
      ) {
        throw new Error(
          'oracle: ' +
            describe(node) +
            ' is an ancestor of anchor "' +
            (el.getAttribute('data-oracle-id') || '') +
            '" at opacity 0, and carries no data-oracle-id. crowbar-driver folds ' +
            'opacity in at anchor boundaries only, so it cannot see this element: ' +
            'this side would report visible=false on every anchor beneath it and ' +
            'the native side visible=true. The capture would not be comparable — ' +
            'a delta on every anchor, caused by the contract rather than by the ' +
            'port. Refusing to emit it. Put the opacity on an anchor instead (the ' +
            'root anchor will do — opacity moves no geometry and every bound is ' +
            'root-relative), or drive the cell with that layer opaque. See ' +
            'native/oracle/corpus/001-opacity-ancestor-visible.md. There is no ' +
            'override.',
        )
      }
      node = node.parentElement
    }
  }
}

/**
 * The anchors a surface owns, and the root it owns them from — declared **per
 * surface**, never per capture.
 *
 * ## The defect this exists for
 *
 * The walk below collects every `data-oracle-id` beneath the root anchor. For a
 * leaf surface that is the surface. For a surface whose root *contains* other
 * anchored subtrees it is not: `resizable`'s root is `resize-group`, the IDE
 * shell's own group, so it swallowed the sidebar — the carousel, the file rows
 * and ten git rows, **81 anchors with repeated ids** against the native
 * surface's four. The differ matches by id and refused it outright, correctly.
 * `sidebar-carousel` hit the same thing and was worked around by hand-filtering
 * to the `carousel-*` ids, which only worked because those ids happen to be
 * unique. Hand-filtering is not a fix; this is.
 *
 * ## Why the surface declares it, and not the call site
 *
 * A free-form `include: string[]` per call is the same shape of mistake this
 * contract exists to prevent. Two captures of one surface could then disagree
 * about which anchors belong to it, and the differ — which sees only the anchor
 * lists — would read the disagreement as a UI delta, or as an anchor "missing"
 * from a port that renders it perfectly well. The set is a property of the
 * component, and the native side already treats it that way: each
 * `crates/crowbar-ui/src/components/*.rs` names its anchors in `ID_*` constants
 * and each `crates/crowbar-app/src/surfaces/*.rs` pins `Surface { name, root }`.
 * The table below is that same declaration on this side, keyed by the same
 * `surface` string §2 puts in the snapshot, so the two are readable against each
 * other by a human — which is all three implementations ever had.
 *
 * ## Why only these eight surfaces are here
 *
 * A surface may declare a set only when the set is a property of the *surface*
 * rather than of the *cell*, and all eight of these are:
 *
 * | surface | its set |
 * |---|---|
 * | `resizable` | the group, the two panels `ide-shell.tsx` names, and the separator. The grip is **not** here: it renders only under `withHandle`, which no live call site passes — `resizable.rs` records the same fact |
 * | `sidebar-carousel` | the scrollport and its four panels, all four rendered unconditionally |
 * | `popover` | the popup and its viewport — the two boxes `PopoverPopup` builds on every render, whatever the call site nests inside |
 * | `select` | the popup, the panel and the list — the three boxes `SelectPopup` builds on every render, whatever items the call site passes |
 * | `dialog` | the popup alone — `DialogPopup`'s one unconditional box. `dialog-header`/`dialog-title`/`dialog-description`/`dialog-footer` are all call-site-optional (P3.21's `import-project-modal` reference has a header and a footer; `add-repository-modal` — same primitive — has no description; a bare `<DialogContent>` has none of the four), so none of them is "a property of the surface" in the sense this table requires — see the point right below about `popover-title`, which the same reasoning already covers |
 * | `sheet` | the popup alone, for the same reason `dialog`'s is just the popup — and doubly so here: `sheet.tsx` has no live call site at all (`crowbar_ui::components::sheet`'s module docs), so there is no reference to derive a fuller set from even if the primitive's own shape allowed one |
 * | `search` | (P3.37) ten of `search.tsx`'s own ids: the root, the leading replace toggle, the input control and field, the close button, the three always-live option toggles and both nav chevrons — every box `SearchPopover` and its `leadingControl` build on the one reachable call site, `find-bar.tsx`. See the dedicated subsection below; this row alone needed a ruling long enough to earn one |
 * | `command` (P3.32) | the popup, the panel, the footer, and every box `autocomplete.tsx`'s `AutocompleteInput`/`AutocompleteList` build unconditionally (the input group, the start addon `CommandInput` always passes, the field, the always-mounted empty node, and the scroll area) — `autocomplete-item` is **not** here, `select-item`'s own reason (see below) |
 *
 * Each set is read off the component that builds the boxes — `popover.tsx`,
 * `select.tsx`, `sidebar-carousel.tsx`, `resizable.tsx` + `ide-shell.tsx`,
 * `dialog.tsx`, `sheet.tsx`, `search.tsx`, `command.tsx` + `autocomplete.tsx` —
 * and never off a capture. A capture shows what one call site produced, which
 * is the very thing being filtered out; deriving the set from it would encode
 * the call site into the surface and the filter would then be a no-op by
 * construction.
 *
 * ### The two overlays, and what each of them had to drop
 *
 * `popover` and `select` are both floated by Base UI, so a capture rooted at the
 * popup contains the call site's whole sub-UI. On the one live popover a parity
 * run can reach that is an avatar, its fallback and three buttons — a
 * **duplicate-id document**, which ANCHORS.md v1.8 refuses outright. Both were
 * reduced by hand instead; the two rows above are that reduction, moved to where
 * it can be read and re-derived.
 *
 * Three ids each are deliberately **not** declared, and the reasons differ:
 *
 * * **`popover-title`** renders only where a call site places `<PopoverTitle>`.
 *   `surfaces/popover.rs` defaults `--title` to `None` for the same reason —
 *   *"the one popover a parity run can drive renders no title, and an anchor the
 *   reference cannot produce is a `FieldPresence` delta that forgives nothing"* —
 *   and declaring it here would make the loud-missing rule below refuse that
 *   popover on every capture. Exactly `resizable`'s grip, one component over.
 * * **`select-item`, `select-item-indicator`, `select-item-text`** exist once per
 *   item the call site passes. Their *count* is a property of the cell, so
 *   declaring them could not be satisfied: the repeated-id rule below would
 *   refuse every list of more than one. v1.8's "each at most once" is the same
 *   sentence read from the other end.
 * * **`select-trigger`, `select-value`, `select-icon`** are real anchors on
 *   `select.tsx` and are unreachable from this root — `SelectPopup` renders
 *   inside a `SelectPrimitive.Portal`, so the popup is not a descendant of the
 *   trigger and no single root spans both. The surface is the popup, which is
 *   also where `dropdown-menu`'s native `Surface` roots (`ID_POPUP`); a trigger
 *   capture is a different surface and does not have one yet.
 * * **`dialog-header`, `dialog-title`, `dialog-description`, `dialog-footer`**
 *   (and `sheet`'s equivalents) are the same shape as `popover-title`, four
 *   times over: each renders only where a call site nests the matching
 *   component, so declaring any of them would refuse every capture where the
 *   call site left it out — which both `dialog` references do, in different
 *   fields (`import-project-modal` has no description; `add-repository-modal`,
 *   same primitive, has neither a description nor is distinguishable from it by
 *   this table). P3.21's own reference JSON therefore was not produced through
 *   this function at all: `crowbar-app/native/mapping/dialog.md` §6 records
 *   that it was captured undeclared (root `dialog-popup`, whatever
 *   `import-project-modal` produced under it) and reduced by hand to the
 *   `dialog-*` ids — the same manoeuvre `popover`'s own reference needed before
 *   this table existed, two rows up.
 *
 * `popover` and `select` are inert only historically — both were captured
 * before this table carried them and had to be reduced by hand once, the
 * account two rows up. Once `data-oracle-id` landed on the two files (P3.18)
 * the declarations here started doing real work on every capture since.
 * `dialog` and `sheet` carry their `data-oracle-id`s from the same change that
 * added these two rows, so — `dialog-header`/`dialog-title`/
 * `dialog-description`/`dialog-footer` aside, which no declaration here could
 * ever safely cover — the two entries below are live from the start.
 *
 * ### `search` (P3.37), and the one call it needed that the others didn't
 *
 * The defect this closes: `/tmp/p3-ref-search.json`'s anchor list nested
 * `search-toggle-icon` inside each of the three always-live option toggles
 * (`search.tsx`'s `options` prop takes `SearchToggleOption.icon: ReactNode`,
 * and every live call site fills it from `SEARCH_TOGGLE_ICONS`, which is
 * `search-toggle-icons.tsx` — a **different, already-verified surface**
 * (P3.8), reused unmodified here). Walking undeclared therefore recorded the
 * same id three times under one root, and the differ refuses a repeated id
 * for the reason `ANCHORS.md` v1.8 states — it cannot say which of the three
 * it compared. Declaring `search`'s own ids and leaving `search-toggle-icon`
 * off this list is what removes it: an anchor the surface does not declare is
 * dropped without comment (see the "loud-missing" walk below), and dropping
 * all three of a duplicate is exactly as sound as dropping none of a unique
 * one — the rule does not know or care how many instances it is discarding.
 *
 * The other two things left off, and why each is a *cell* property rather
 * than a *surface* one:
 *
 * * **`search-toggle-preserve-case`** exists only while `--replace` is
 *   passed (`find-bar.tsx`'s single `isReplaceVisible` gates it, exactly as
 *   `SearchPopover::replace: Option<SearchReplaceRow>` does on the native
 *   side). The reference this surface is validated against on every ordinary
 *   run — `/tmp/p3-ref-search.json`, replace collapsed — does not have it, so
 *   declaring it would refuse that capture by the loud-missing rule below.
 *   Exactly `popover-title`'s shape, one surface over.
 * * **`search-replace-row` and everything under it** (`search-replace-icon`,
 *   the row's own second `input-control`/`input`, `search-replace-confirm`,
 *   `search-replace-all`) are excluded for the identical cell-conditional
 *   reason, **and** for a second, structural one that would still apply even
 *   if `--replace` were somehow a surface-level constant: expanding the
 *   replace row mounts a *second* `<Input>` while `SearchPopover`'s own is
 *   still mounted, and `input.tsx` hard-codes `data-oracle-id="input-control"`/
 *   `"input"` with no override prop (unlike `button.tsx`'s own
 *   `'data-oracle-id': 'button', ...props`, which a call site can shadow). A
 *   document spanning both mounted `Input`s carries the id twice regardless
 *   of what any scope declares — declaring is a *filter*, and a duplicate
 *   *within* the declared set still trips `oracleSelectDeclaredAnchors`' own
 *   "two elements carry one declared id" rule. So `search-replace-row` is
 *   its own registered surface instead (`crowbar-app/src/surfaces/
 *   search_replace_row.rs`, P3.37) — reachable only as a second, sibling-
 *   rooted capture, `/tmp/p3-ref-search-replace-row.json`, never nested
 *   inside this one. See that surface's module docs for the driver-side half
 *   of the identical argument: `AnchorRegistry::record` *replaces* rather
 *   than refuses a repeated id, so `--surface search --replace` does not
 *   crash, but its `"input-control"` record silently becomes whichever
 *   `Input` prepainted last, and `Snapshot::build` copies every recorded
 *   anchor into a snapshot regardless of which id `root` names — so even a
 *   snapshot asked for that same run with `root: "search-replace-row"` would
 *   still carry `search-popover` and the rest, not the six-anchor shape the
 *   reference has. Two structurally independent proofs, converging on one
 *   answer: this row cannot be reached by widening `search`'s own scope, on
 *   either side of the contract.
 *
 * **Why `search-replace-toggle` *is* declared, unlike the two above, even
 * though it too arrives through a generic prop (`leadingControl`).** The
 * v1.8 test is "a property of the surface", and the operative question a
 * reader should check before trusting either answer in this file is whether
 * declaring costs the one real, reachable capture anything. `popover-title`
 * and `dialog-header` fail that test — the one live popover and one of the
 * two live dialogs render *without* the optional slot, so declaring it would
 * refuse a capture that exists today. `search-replace-toggle` does not fail
 * it: `find-bar.tsx` — the only call site a `search` capture is ever taken
 * from, because it is the only one `crowbar_ui::components::search::
 * SearchPopover`'s Rust struct models (its own doc comment: *"the struct
 * models the **reachable** configuration: a leading `SearchReplaceToggle`…"*,
 * with no field to omit it) — passes `leadingControl` unconditionally, and
 * `/tmp/p3-ref-search.json` already has it. `terminal-search.tsx` is real and
 * does not pass one, but no `search` capture will ever be taken from it: the
 * native side has no parameter that renders that shape, so such a capture
 * would already be incomparable regardless of this declaration. Leaving
 * `search-replace-toggle` off, by contrast, would cost something concrete —
 * the native side renders it on *every* cell this surface can produce, so an
 * undeclared-but-always-present anchor would either leak in through the
 * undeclared path (fine, but then the declared/undeclared paths disagree on
 * what "the surface" is) or, declared-elsewhere-but-missing-here, manufacture
 * a permanent, un-fixable "present on one side only" delta on every
 * comparison this surface will ever run. `SearchReplaceToggle` is also, unlike
 * `PopoverTitle` or a dialog's header slot, authored in `search.tsx` itself —
 * a sibling export of the same file, not a call site's own unrelated markup —
 * which is the same-file test `sidebar-header`'s exclusion of its own
 * call-site `<Input>`/`<Button>` does *not* get to lean on (those really are
 * foreign to `sidebar.tsx`). Both facts point the same way; neither alone
 * would have been enough to overturn the general rule.
 *
 * `git-status-row`, `file-tree-row` and `dropdown-menu` are deliberately absent,
 * and not for want of getting to them. Their anchor sets are functions of the
 * cell — `git-row-guide-{n}` exists once per depth level, `git-row-dir` and
 * `git-row-deleted` only when the fixture has them, a `--tick` row takes its
 * primitive's own id — so any list written here would be a lie in most cells,
 * and the loud-missing rule below would then reject honest captures. They also
 * need no scope: their roots contain their own anchors and nothing else. An
 * undeclared surface therefore captures its whole subtree exactly as before,
 * which is what keeps the 63 archived `git-status-row` / `file-tree-row` pairs
 * in `native/oracle/runs/` comparing byte-for-byte.
 *
 * Returns `null` for an undeclared surface — "no declaration" and "an empty
 * declaration" are different facts and only the first is expressible here.
 */
export function oracleSurfaceScope(
  surface: string | undefined | null,
): { root: string; anchors: string[] } | null {
  // Inlined rather than lifted to a module constant, per the injectability rules
  // at the top of this file: this function is serialised by `toString()` into a
  // page with no module scope, where a captured binding is a `ReferenceError`
  // there and nowhere else.
  const declared: Record<string, { root: string; anchors: string[] }> = {
    resizable: {
      root: 'resize-group',
      anchors: ['resize-group', 'resize-panel-sidebar', 'resize-handle', 'resize-panel-content'],
    },
    'sidebar-carousel': {
      root: 'carousel-scrollport',
      anchors: [
        'carousel-scrollport',
        'carousel-panel-workspaces',
        'carousel-panel-chats',
        'carousel-panel-files',
        'carousel-panel-git',
      ],
    },
    // `popover.tsx`: `PopoverPopup` renders `PopoverPrimitive.Popup` wrapping
    // exactly one `PopoverPrimitive.Viewport`, unconditionally. `children` — the
    // call site's sub-UI — goes inside the viewport and is not the surface.
    popover: {
      root: 'popover-popup',
      anchors: ['popover-popup', 'popover-viewport'],
    },
    // `select.tsx`: `SelectPopup` renders `Popup` → the panel `div` → `List`,
    // unconditionally. `children` — the call site's `SelectItem`s — go inside
    // the list, one `select-item` each, and are not the surface.
    select: {
      root: 'select-popup',
      anchors: ['select-popup', 'select-panel', 'select-list'],
    },
    // `search.tsx` (P3.37): `SearchPopover` renders its own root, a leading
    // `SearchReplaceToggle` (via `leadingControl`), an `Input`, a close
    // button, three always-live option toggles and two nav chevrons —
    // unconditionally, on the one reachable call site (`find-bar.tsx`).
    // `search-toggle-icon` (the option toggles' own icon content) belongs to
    // `search-toggle-icons`, a different, already-verified surface, and is
    // deliberately not here; neither is `search-toggle-preserve-case`
    // (`--replace`-only) nor `search-replace-row` and its own children
    // (`--replace`-only, and its own registered surface besides) — see the
    // module docs above for the full ruling on all three.
    search: {
      root: 'search-popover',
      anchors: [
        'search-popover',
        'search-replace-toggle',
        'input-control',
        'input',
        'search-close',
        'search-toggle-case-sensitive',
        'search-toggle-whole-word',
        'search-toggle-regex',
        'search-nav-previous',
        'search-nav-next',
      ],
    },
    // `scroll-area.tsx`: `ScrollArea` renders `Root` wrapping exactly one
    // `Viewport`, unconditionally. `children` — a whole file tree, a diff list,
    // a command list — go inside the viewport and are **not** the surface; a
    // capture of `workspace-tree`'s instance without this entry swallows every
    // `file-row-item` under it, which is the `resizable` failure v1.8 was
    // written for.
    //
    // **The two scrollbars and the corner are deliberately not here**, and that
    // is the half of this entry worth reading. `ScrollAreaScrollbar` is
    // `shouldRender = keepMounted || !isHidden` and returns `null` otherwise;
    // `scroll-area.tsx` never passes `keepMounted`, so a track exists only on an
    // axis that actually overflows. Presence is therefore a property of the
    // **cell**, not of the surface, and v1.8 permits a declaration only for the
    // latter — declaring them would turn every honest no-overflow capture into a
    // refusal for a missing declared anchor.
    'scroll-area': {
      root: 'scroll-area-root',
      anchors: ['scroll-area-root', 'scroll-area-viewport'],
    },
    // `dialog.tsx`: `DialogPopup` renders `DialogPrimitive.Popup` and nothing
    // else unconditionally. `DialogHeader`/`DialogTitle`/`DialogDescription`/
    // `DialogFooter` are all call-site slots — see the module docs above for
    // why none of the four can be declared — and `children` (the call site's
    // own body) is not the surface either.
    dialog: {
      root: 'dialog-popup',
      anchors: ['dialog-popup'],
    },
    // `alert-dialog.tsx` (P3.28): `AlertDialogPopup` renders
    // `AlertDialogPrimitive.Popup` and nothing else unconditionally, the same
    // shape `dialog`'s is — `AlertDialogHeader`/`AlertDialogTitle`/
    // `AlertDialogDescription`/`AlertDialogFooter` are all call-site slots.
    // Sharper reason to declare it than `dialog` had: the one reachable call
    // site (`review-thread-item.tsx`'s delete confirmation) nests two real
    // `<Button>`s inside `AlertDialogFooter`, each carrying the primitive's
    // own `data-oracle-id="button"` — an undeclared capture rooted at
    // `alert-dialog-popup` would pull both in.
    'alert-dialog': {
      root: 'alert-dialog-popup',
      anchors: ['alert-dialog-popup'],
    },
    // `sheet.tsx`: same shape as `dialog`'s, for the same reason — see the
    // module docs. No live call site mounts one at all
    // (`crowbar_ui::components::sheet`'s module docs), so this entry exists
    // for the day one does rather than for any capture taken so far.
    sheet: {
      root: 'sheet-popup',
      anchors: ['sheet-popup'],
    },
    // `sidebar.tsx`: `SidebarHeader` is **one** `<div>` and nothing else of its
    // own — everything inside it belongs to the call site. The one live call
    // site (`file-explorer-tree.tsx`) puts an `<Input>` and a `<Button>` in
    // there, and both carry their own primitive's ids, so an undeclared capture
    // comes back with `input-control`, `input` and `button` under this root.
    // Measured on the running app before this entry was written; the set below
    // is read off the component, not off that capture.
    'sidebar-header': {
      root: 'sidebar-header',
      anchors: ['sidebar-header'],
    },
    // `sidebar.tsx`: `SidebarEmptyActionState` renders its own `<div>` and a
    // message `<div>` unconditionally. The icon `<span>`, the description
    // `<div>` and the action `<Button>` are each behind a prop, so none of them
    // is declared — the call `popover` makes about `PopoverTitle`, for the same
    // reason: a declared anchor that is not in the document throws.
    'sidebar-empty': {
      root: 'sidebar-empty',
      anchors: ['sidebar-empty', 'sidebar-empty-message'],
    },
    // `sidebar-project-header.tsx` (P3.54): the wrapper `<div>` plus its four
    // `<Button>`s — the toggle, back, forward and settings — all unconditional
    // on this one call site (no prop gates whether any of the five renders).
    // `crowbar_ui::components::sidebar_project_header::SidebarProjectHeader`
    // gives each button its own authored id and matches this set exactly.
    //
    // **Not declared: `sidebar-toggle-icon`.** The toggle `<Button>` always
    // nests a `<SidebarToggleIcon />`, which carries its own unconditional
    // `data-oracle-id="sidebar-toggle-icon"` — a real anchor, but one belonging
    // to a *different*, already-separately-ported surface
    // (`sidebar_toggle_icon.rs`, its own `Surface`). This composition's own
    // Rust port deliberately does not compose or paint that glyph (its own
    // module docs: "this file does not reach into another component's
    // anchoring... the toggle's glyph is left unpainted"), so an undeclared
    // capture rooted at `sidebar-project-header` would carry an anchor with no
    // native counterpart in this surface's own render — exactly the class of
    // delta v1.8 exists to prevent, `sidebar-header`'s own foreign-content
    // reasoning one level down.
    'sidebar-project-header': {
      root: 'sidebar-project-header',
      anchors: [
        'sidebar-project-header',
        'sidebar-project-header-toggle',
        'sidebar-project-header-back',
        'sidebar-project-header-forward',
        'sidebar-project-header-settings',
      ],
    },
    // `command.tsx` (P3.32): `CommandDialogPopup` renders
    // `CommandDialogPrimitive.Popup` and nothing else unconditionally —
    // `dialog`'s own shape, one file over (`command` hand-rolls its own
    // dialog chrome from raw base-ui `Dialog` primitives rather than
    // composing `dialog.tsx`'s `DialogPopup`; see
    // `crowbar_ui::components::command`'s module docs). Inside it,
    // `Command`/`CommandInput`/`CommandPanel`/`CommandList`/`CommandFooter`
    // render `autocomplete.tsx`'s own `AutocompleteInput`/`AutocompleteList`
    // boxes, restyled through `className`, not reimplemented — the reused
    // ids (`autocomplete-input-group`, `autocomplete-start-addon`,
    // `autocomplete-input`, `autocomplete-empty`, `autocomplete-list`, and
    // the already-declared `scroll-area-root`/`scroll-area-viewport` this
    // surface nests) are declared here rather than a second time under
    // `command-*` names, because there is only one set of boxes and
    // `command.tsx` never overrides their `data-slot` (confirmed live —
    // `CommandInput` passes none, so the field still reports
    // `data-slot="autocomplete-input"`).
    //
    // `autocomplete-item` is **not** declared, for `select-item`'s own
    // reason: its count is a property of the cell (the call site's own
    // workspace list), and this dev environment's one-workspace fixture
    // happening to keep it at exactly one does not make it a property of
    // the *surface*. Its footer nests several `<Kbd>`s — `kbd.tsx`'s own
    // id, repeated — which is the sharper reason a declaration is needed
    // here at all: an undeclared capture rooted at `command-dialog-popup`
    // pulls every one of them in, and `ANCHORS.md` v1.8 refuses a document
    // with a repeated id outright.
    command: {
      root: 'command-dialog-popup',
      anchors: [
        'command-dialog-popup',
        'command-panel',
        'command-footer',
        'autocomplete-input-group',
        'autocomplete-start-addon',
        'autocomplete-input',
        'autocomplete-empty',
        'scroll-area-root',
        'scroll-area-viewport',
        'autocomplete-list',
      ],
    },
    // `workspace-tree-item.tsx` (P3.61, `layout-denominator.md` §8's Cluster
    // 8): the row itself. **This declared scope is only sound for a LEAF
    // cell — no children, no pending creates.** The component recurses (it
    // renders itself for `children`), and both `workspace-tree-item` and the
    // composed `workspace-branch-icon` are the SAME literal id at every
    // depth — `select-item`'s own "count is a cell property" reasoning means
    // neither the recursive family nor `pending-create-row` (its own
    // registered surface) is declared, but unlike `repo-section`/`workspace-
    // tree` below, the repeated id here is not confined to an excluded
    // child: it is this surface's OWN root and its OWN icon. A capture taken
    // with any child present would carry two elements under `workspace-tree-
    // item` (and, if either row is working/placeholder, two under
    // `workspace-branch-icon` too) and `oracleSelectDeclaredAnchors`' "two
    // elements carry one declared id" rule refuses it outright — the
    // identical shape `search --replace`'s two `Input`s hit, documented
    // above. So the one reachable reference this scope can validate is a
    // childless row; a deeper cell is a future worker's problem to solve the
    // way `search-replace-row` did (its own registered surface, captured
    // separately), not something this declaration can paper over.
    //
    // The row's own conditional slots — `workspace-tree-item-added`/`-deleted`
    // (active + has changes only), `workspace-tree-item-expand` (has children
    // only), `-placeholder-details` (placeholder + active only) and
    // `-create-input` (creating a child only, mutually exclusive with
    // `-new-button`) — are left undeclared, `dialog-header`'s own reason:
    // each is real on some reachable cell and absent on others, and declaring
    // one would refuse every capture where that particular cell does not
    // happen to show it. `workspace-tree-item-add-child` is this row's own
    // button when no children are present — one per row, not a repeated child
    // element.
    'workspace-tree-item': {
      root: 'workspace-tree-item',
      anchors: ['workspace-tree-item', 'workspace-branch-icon', 'workspace-tree-item-label', 'workspace-tree-item-add-child'],
    },
    // `repo-section.tsx` (P3.61, Cluster 8): the repo header row's own
    // unconditional chrome. `repo-icon-popover-trigger` is real and painted
    // by this composition (not foreign — `repo-icon-popover.tsx`'s own
    // finding, reused here), so it is declared rather than excluded.
    // `repo-section-add-child` is this row's own conditional button (shown
    // when `repo.defaultWorkspaceId` exists) — one per row, not a repeated
    // child element like the `select-item` precedent.
    //
    // Not declared, `workspace-tree-item`'s own two reasons: `workspace-tree-
    // item` (recursive, one per root workspace — count is a cell property)
    // and `pending-create-row` (one per in-flight create at this repo's root
    // — same). `repo-section-label` is declared (present on every capture
    // that is not mid-rename, which is the overwhelmingly common and only
    // currently-reachable shape).
    'repo-section': {
      root: 'repo-section',
      anchors: [
        'repo-section',
        'repo-icon-popover-trigger',
        'repo-section-label',
        'repo-section-import',
        'repo-section-add-child',
        'repo-section-collapse',
      ],
    },
    // `workspace-tree.tsx` (P3.61, Cluster 8): the outer scaffold only —
    // `ProjectHomeRow` (already declared under its own five ids, reused
    // rather than repeated here — `command`'s own `scroll-area-*` reuse) and
    // the `ScrollArea` shell it wraps around the repo list. `repo-section`
    // is excluded for `select-item`'s reason one level up: the repo count is
    // a property of the workspace list, not of this surface. Whatever a
    // `repo-section` capture nests (in turn excluding its own `workspace-
    // tree-item`/`pending-create-row` families) is verified through that
    // surface's own root, never through this one — `resizable`'s own
    // precedent for a container whose repeated content is somebody else's
    // surface.
    'workspace-tree': {
      root: 'workspace-tree',
      anchors: [
        'workspace-tree',
        'project-home-row',
        'project-home-row-icon',
        'project-home-row-label',
        'project-home-row-import',
        'project-home-row-switch',
        'scroll-area-root',
        'scroll-area-viewport',
      ],
    },
  }
  const key = String(surface === null || surface === undefined ? '' : surface)
  // Own properties only: `declared['constructor']` would otherwise hand back a
  // function and be read as a declaration.
  if (!Object.prototype.hasOwnProperty.call(declared, key)) return null
  return declared[key]
}

/**
 * Keep exactly the surface's own anchors, in document order, or refuse.
 *
 * Three ways this ends, and only one of them emits a snapshot:
 *
 * 1. **A declared anchor is not in the document → throws.** Filtering is the one
 *    operation that can make a capture quietly *smaller* than the surface, and a
 *    reference that shrinks proves less every time it does — silently, because
 *    the differ compares the anchors it is given. A rename on one side, a
 *    surface driven into a state it does not have, a typo in the table above:
 *    all three land here rather than in an archive.
 * 2. **Two elements carry one declared id → throws.** The differ matches by id
 *    and could not say which of the two it compared. Taking the first would make
 *    that choice invisible, which is worse than the ambiguity.
 * 3. Otherwise the kept elements come back **in the order the walk found them**,
 *    so a declared capture is a subsequence of the undeclared one — the same
 *    elements, measured by the same code, in the same order.
 *
 * An anchor found under the root that the surface does not declare is dropped
 * without comment: on `resizable` that is most of the sidebar, so it cannot be
 * an error. The reciprocal risk — a **new** anchor added to a declared
 * component and not added here — is the one thing this cannot see, and it does
 * not vanish silently either: the native side renders it, the reference does
 * not, and the differ reports an anchor present on one side only.
 */
export function oracleSelectDeclaredAnchors(
  elements: Element[],
  surface: string,
  declared: string[],
): Element[] {
  const counts: number[] = []
  for (let i = 0; i < declared.length; i++) {
    if (declared.indexOf(declared[i]) !== i) {
      throw new Error(
        'oracle: surface "' +
          surface +
          '" declares the anchor "' +
          declared[i] +
          '" twice. A repeated declaration cannot be satisfied — the second copy ' +
          'would report as missing however the document is built.',
      )
    }
    counts.push(0)
  }

  const kept: Element[] = []
  for (let i = 0; i < elements.length; i++) {
    const id = elements[i].getAttribute('data-oracle-id') || ''
    const at = declared.indexOf(id)
    if (at < 0) continue
    counts[at]++
    kept.push(elements[i])
  }

  const missing: string[] = []
  const repeated: string[] = []
  for (let i = 0; i < declared.length; i++) {
    if (counts[i] === 0) missing.push(declared[i])
    else if (counts[i] > 1) repeated.push(declared[i] + ' ×' + counts[i])
  }

  if (missing.length > 0) {
    throw new Error(
      'oracle: surface "' +
        surface +
        '" declares the anchor(s) ' +
        missing.join(', ') +
        ', and the document has none. Either the surface was driven into a state ' +
        'it does not render them in, or the id was renamed on one side only. ' +
        'Emitting the rest would archive a capture smaller than the surface, and a ' +
        'comparison against a shrinking anchor set stops proving anything without ' +
        'ever saying so. Refusing to emit it. There is no override.',
    )
  }
  if (repeated.length > 0) {
    throw new Error(
      'oracle: surface "' +
        surface +
        '" has more than one element carrying the declared anchor id(s) ' +
        repeated.join(', ') +
        '. The differ matches by id and would have no way to say which of them it ' +
        'compared; picking the first would make that choice invisible. Give them ' +
        'distinct ids at the call site — ide-shell.tsx names its two panels ' +
        'resize-panel-sidebar and resize-panel-content for exactly this reason — ' +
        'or narrow `scope` to one of them. Refusing to emit it.',
    )
  }

  return kept
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
 *
 * ## `derivedTheme` is ground truth, not a default
 *
 * It used to be only the fallback for a caller who supplied nothing, and a
 * supplied `theme` overrode it **unchecked**. That silently produced the one
 * artefact this whole contract exists to prevent: a reference captured with
 * `state: { theme: 'dark' }` while a page reload had quietly put the app back
 * into **light**, emitted as a snapshot *labelled* `dark` carrying light-theme
 * values. Compared against the native `--theme dark` cell it reported a colour
 * delta on every anchor whose real cause was the label. It was caught only
 * because a reader recognised `border.color: #00000014` as a light token —
 * nothing in the pipeline would have caught it, and a mislabelled reference
 * that gets archived is worse than a failing run, because it becomes evidence.
 *
 * So a supplied `theme` that contradicts {@link oracleDetectTheme} **throws**.
 * It does not warn: this runs inside an `execute_js` bridge where a warning
 * goes to a console nobody is reading, so the only way to make the mislabel
 * impossible is to make the snapshot impossible.
 *
 * **There is deliberately no override.** The emitted value is unchanged either
 * way — after this check the declaration and the document agree by
 * construction — so the check costs a correct caller nothing, and an escape
 * hatch would only ever be reached for the case it exists to catch.
 *
 * `width`, `content` and `flags` are *not* checked against the document. See
 * the notes at each below: the reason differs per field and is not "we did not
 * get to it".
 */
export function oracleNormalizeState(
  requested: Partial<OracleState> | undefined | null,
  derivedWidth: number,
  derivedTheme: 'light' | 'dark',
): OracleState {
  const wanted = requested || {}

  // NOT checked against the document, and that is a decision.
  //
  // `state.width` is the **viewport** width (ANCHORS.md §8.3, and the ruling
  // recorded in QUEUE.md after a 569px webview silently rendered the narrow
  // badge variant). `derivedWidth` is the *root anchor's* width — a different
  // quantity, and knowably so: the archived `ref-600.json` pair declares
  // `width: 600` around a `git-row-item` measuring 294. Comparing the two would
  // false-alarm on every honest capture.
  //
  // The document does expose viewport-ish numbers, but not one that is
  // unambiguously this field: `window.innerWidth` includes a classic
  // scrollbar's gutter and `documentElement.clientWidth` does not, and they
  // disagree precisely when a breakpoint is near — which is the only reason
  // this field is load-bearing at all. Picking one and asserting equality would
  // be a second guess layered on a field ANCHORS.md itself still records as
  // open, and a false alarm here blocks a capture outright. Left unchecked,
  // deliberately.
  //
  // What *is* enforced: a supplied width has to be a number. `Math.round(NaN)`
  // used to land on `0`, so `width: '600px'` emitted a snapshot labelled with a
  // cell no run ever produced — the same silent mislabel as the theme case, and
  // this one has no false-alarm question hanging over it.
  let width: number
  if (wanted.width === undefined || wanted.width === null) {
    width = Math.round(Number(derivedWidth))
    if (!isFinite(width)) width = 0
  } else {
    width = Math.round(Number(wanted.width))
    if (!isFinite(width)) {
      throw new Error(
        'oracle: state.width must be a number of logical px, got ' + JSON.stringify(wanted.width),
      )
    }
  }

  let theme = derivedTheme
  if (wanted.theme !== undefined && wanted.theme !== null) {
    const t = String(wanted.theme).trim().toLowerCase()
    if (t !== 'light' && t !== 'dark') {
      throw new Error(
        'oracle: state.theme must be "light" or "dark", got ' + JSON.stringify(wanted.theme),
      )
    }
    if (t !== derivedTheme) {
      throw new Error(
        'oracle: state.theme declares "' +
          t +
          '" but the document being measured is "' +
          derivedTheme +
          '". Refusing to emit a snapshot labelled with a cell it was not ' +
          'captured in — an archived mislabel is worse than a failed run. Put ' +
          'the app into "' +
          t +
          '" and capture again, or declare "' +
          derivedTheme +
          '". There is no override.',
      )
    }
    theme = t as 'light' | 'dark'
  }

  // `content` and `flags` are the caller's *intent*, not properties of the
  // document, and there is deliberately no attempt to verify them.
  //
  // `content` names which fixture string the surface was driven with — the
  // extractor sees one string and cannot know whether the author considers it
  // short, normal or overflow; `clipped` is a consequence of the box, not of
  // the cell. `flags` names how the surface was *driven* (`hover` is a pointer
  // the harness is holding, `loading` a prop). A DOM guess — `:hover` matching,
  // `aria-selected` — would be a heuristic that disagrees with the harness on
  // exactly the surfaces that do not use the attribute it happened to look at,
  // and a wrong guess here throws away a legitimate capture. Unverifiable is
  // the honest answer; inventing a check would only look like coverage.

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
 * v1 snapshot — narrowed to the surface's own anchors where the surface declares
 * a set (see {@link oracleSurfaceScope}), because a root that contains other
 * anchored subtrees would otherwise capture them too.
 *
 * Everything is read from `getComputedStyle` on the live element —
 * never inferred from class names, because the tree CSS overrides the component's
 * Tailwind classes (`rounded-md` → `border-radius: 2px !important`,
 * `hover:bg-muted` killed outright) and a class list would lie.
 */
export function extractSnapshot(options: ExtractOptions): OracleSnapshot {
  const opts = options || ({} as ExtractOptions)
  // The surface's own declaration, where it has one (v1.7 + P2.11). It supplies
  // the root as well as the anchor set, because the root *is* one of the
  // surface's anchors: a capture rooted somewhere else reports every bound
  // against an origin the other side never used, and the differ sees only the
  // `root` string that came with it.
  const declared = oracleSurfaceScope(opts.surface)
  const rootId = opts.root || (declared ? declared.root : '') || 'git-row-item'
  if (declared && rootId !== declared.root) {
    throw new Error(
      'oracle: surface "' +
        opts.surface +
        '" is rooted on "' +
        declared.root +
        '", not on "' +
        rootId +
        '". Every bound in a snapshot is relative to its root (ANCHORS.md §4) ' +
        'and the native surface pins that root, so a capture taken from another ' +
        'one is not comparable with anything. Refusing to emit it.',
    )
  }
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

  // P2.11: everything beneath the root is not the surface. A declared surface
  // keeps its own anchors and refuses rather than shrink; an undeclared one is
  // handed the identical array, so the path the archived `git-status-row` and
  // `file-tree-row` pairs were captured on is untouched — same elements, same
  // order, same code below.
  const scoped = declared
    ? oracleSelectDeclaredAnchors(elements, String(opts.surface), declared.anchors)
    : elements

  // v1.7 precondition, before anything is measured: an unanchored ancestor at
  // zero opacity makes this side's `visible` column unreproducible by the
  // driver. Nothing below changes as a result — the guard either throws or is
  // invisible, so the archived pairs in `native/oracle/runs/` compare unchanged.
  //
  // Run on the *scoped* set, which is the set that gets measured: an `opacity-0`
  // layer inside the sidebar is not a reason to refuse a `resizable` capture
  // that does not report a single anchor from under it — and NavStack puts
  // exactly such a layer there.
  oracleAssertComparableOpacity(scoped)

  // v1.11: an element that generates no box is not an anchor. Emitting it as a
  // zero-rect record with `visible: false` put a delta on *anchor presence* —
  // GPUI has no such element at all — and neither repair on that side is honest:
  // anchoring nothing is impossible there, and synthesising a zero-rect record
  // would be writing this extractor's output into the port.
  //
  // Applied after the walk and after the v1.7 guard rather than instead of
  // either, so nothing above changes: the scoped set, the surface's declaration
  // and the opacity precondition all still see every anchored element. Only the
  // *measured* set narrows, and only by elements that have no box to measure.
  const boxed: Element[] = []
  for (let i = 0; i < scoped.length; i++) {
    if (oracleGeneratesBox(scoped[i])) boxed.push(scoped[i])
  }
  // The root is the origin every other bound is expressed against (§4). If it
  // generates no box there is no origin, and the §4 check at the bottom would
  // report that as "not at the origin" — true, but it names the wrong cause and
  // costs an hour. `scoped[0]` is the root anchor on both paths: the walk seeds
  // `elements` with it, and a declared surface is refused unless it is rooted on
  // one of its own declared anchors.
  if (boxed.length === 0 || boxed[0] !== scoped[0]) {
    throw new Error(
      'oracle: root anchor "' +
        rootId +
        '" generates no box — it or an ancestor computes display:none. Every ' +
        'bound in a snapshot is relative to this element (ANCHORS.md §4), so ' +
        'there is nothing to measure against. The surface was driven into a ' +
        'state it does not render. Refusing to emit it.',
    )
  }

  const normalLineHeights: Record<string, number> = {}
  const anchors: OracleAnchor[] = []
  let rootBox: OracleBox = { left: 0, top: 0, width: 0, height: 0 }

  for (let i = 0; i < boxed.length; i++) {
    const el = boxed[i] as HTMLElement
    const id = el.getAttribute('data-oracle-id') || ''
    const style = win.getComputedStyle(el)

    // Pseudo-backed anchors (§3): the paint lives on a pseudo-element that has
    // no DOM node and cannot carry the attribute. Only valid while the pseudo
    // is `position:absolute` — {@link oraclePseudoBoxRect} reads its actual
    // resolved inset rather than assuming `inset:0` (P3.34).
    let pseudoStyle: CSSStyleDeclaration | null = null
    const pseudoSelector = pseudoMap[id]
    if (pseudoSelector) {
      const candidate = win.getComputedStyle(el, pseudoSelector)
      if (candidate && candidate.content !== 'none') pseudoStyle = candidate
    }

    let box: OracleBox
    if (pseudoStyle) {
      box = oraclePseudoBoxRect(el, style, pseudoStyle, id)
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
        // v1.12. Overflowing the content box is not truncation on its own —
        // something has to hide it. Measured on the same `box` the bounds and
        // `visible` come from, so the three answers cannot be about different
        // rects.
        overflowHidden: oracleOverflowHidesText(el, box),
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
  oracleOverflowHidesText,
  oracleIsClipped,
  oracleContentSized,
  oracleLineSized,
  oracleOwnTextNodes,
  oracleOwnText,
  oracleTextAdvanceWidth,
  oraclePaddingBoxRect,
  oracleParsePx,
  oraclePseudoBoxRect,
  oracleGeneratesBox,
  oracleIsVisible,
  oracleAssertComparableOpacity,
  oracleSurfaceScope,
  oracleSelectDeclaredAnchors,
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
