import { TERMINAL_NERD_FONT_FALLBACKS } from './terminal-fonts'

const WINDOWS_FALLBACK = 'Consolas'
const MAC_FALLBACK = 'Menlo'
const LINUX_FALLBACK = '"Liberation Mono"'
const CSS_GENERIC_FONT_FAMILIES = new Set([
  'cursive',
  'emoji',
  'fangsong',
  'fantasy',
  'math',
  'monospace',
  'sans-serif',
  'serif',
  'system-ui',
  'ui-monospace',
  'ui-rounded',
  'ui-sans-serif',
  'ui-serif',
])

function getPlatform(): 'windows' | 'mac' | 'linux' {
  if (typeof navigator === 'undefined') return 'linux'
  const ua = navigator.userAgent
  if (/Windows/i.test(ua)) return 'windows'
  if (/Mac/i.test(ua)) return 'mac'
  return 'linux'
}

function getPlatformFallback(): string {
  const platform = getPlatform()
  if (platform === 'windows') return WINDOWS_FALLBACK
  if (platform === 'mac') return MAC_FALLBACK
  return LINUX_FALLBACK
}

function stripWrappingQuotes(name: string): string {
  return name.trim().replace(/^['"]+|['"]+$/g, '')
}

function quoteFontName(name: string): string {
  const normalized = stripWrappingQuotes(name)
  if (!normalized) return ''
  if (CSS_GENERIC_FONT_FAMILIES.has(normalized.toLowerCase())) return normalized
  return `"${normalized.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`
}

function splitFontFamilyList(fontFamily: string): string[] {
  const families: string[] = []
  let current = ''
  let quote: string | null = null

  for (const char of fontFamily) {
    if ((char === '"' || char === "'") && !quote) {
      quote = char
      current += char
      continue
    }

    if (char === quote) {
      quote = null
      current += char
      continue
    }

    if (char === ',' && !quote) {
      const family = stripWrappingQuotes(current)
      if (family) families.push(family)
      current = ''
      continue
    }

    current += char
  }

  const family = stripWrappingQuotes(current)
  if (family) families.push(family)
  return families
}

function uniqueFontFamilies(families: string[]): string[] {
  const seen = new Set<string>()
  const result: string[] = []

  for (const family of families) {
    const normalized = stripWrappingQuotes(family)
    const key = normalized.toLowerCase()
    if (!normalized || seen.has(key)) continue
    seen.add(key)
    result.push(normalized)
  }

  return result
}

/**
 * Build the terminal font-family string with platform-aware fallbacks.
 *
 * xterm.js measures character width from the *first* font it can resolve,
 * so the order matters: primary -> Nerd Font glyph fallbacks -> platform native -> generic monospace.
 */
export function buildTerminalFontFamily(primaryFont: string): string {
  const requestedFonts = splitFontFamilyList(primaryFont)
  const concreteRequestedFonts = requestedFonts.filter(
    (font) => !CSS_GENERIC_FONT_FAMILIES.has(stripWrappingQuotes(font).toLowerCase()),
  )
  const platformFallback = getPlatformFallback()
  return uniqueFontFamilies([
    ...concreteRequestedFonts,
    ...TERMINAL_NERD_FONT_FALLBACKS,
    platformFallback,
    'monospace',
  ])
    .flatMap((font) => quoteFontName(font) || [])
    .join(', ')
}

/**
 * Load a font and verify it's available for canvas rendering.
 * Returns `true` if the font is ready, `false` if it failed/timed out.
 */
export async function loadAndVerifyFont(fontFamily: string, fontSize: number): Promise<boolean> {
  const testString = `${fontSize}px "${fontFamily}"`

  try {
    await document.fonts.load(testString)
  } catch {
    return false
  }

  // `check()` returns true only if every glyph in the test string can be
  // rendered with the requested font (i.e. the font actually loaded).
  return document.fonts.check(testString)
}

/**
 * Resolve the terminal font family — attempts to load the requested font,
 * verifies it, and falls back to a platform-native monospace font if needed.
 *
 * Always returns a usable CSS font-family string for xterm.js.
 */
/**
 * Variable fonts break xterm.js's WebGL glyph atlas (texture misalignment), so
 * xterm silently falls back to its DOM renderer for them — which rebuilds the
 * whole cell grid every frame and stalls the main thread ~130ms on full-screen
 * TUIs (cmatrix, htop, vim). Map a variable family to its static cut (e.g.
 * "JetBrains Mono Variable" -> "JetBrains Mono") so the WebGL renderer stays on.
 *
 * Returns `null` when the font has no distinct static equivalent.
 */
export function deriveStaticFontEquivalent(font: string): string | null {
  const base = stripWrappingQuotes(font)
  const stripped = base.replace(/\s*variable\s*$/i, '').trim()
  if (!stripped) return null
  return stripped.toLowerCase() === base.toLowerCase() ? null : stripped
}

/**
 * The bundled Nerd Font symbol fallback family (see the @font-face in theme.css
 * and the fallback list in terminal-fonts.ts). TUIs (Claude Code, lazygit, etc.)
 * render private-use icon glyphs that no text mono font carries.
 */
const SYMBOL_FALLBACK_FONT = 'Symbols Nerd Font Mono'

/**
 * Warm the bundled symbol fallback font so the terminal's WebGL glyph atlas
 * rasterizes icon glyphs correctly on first paint instead of caching a
 * missing-glyph box (the atlas does not re-rasterize already-drawn codepoints
 * until it is cleared). Best-effort: never throws, never blocks terminal init.
 */
async function ensureSymbolFallbackFontLoaded(fontSize: number): Promise<void> {
  if (typeof document === 'undefined' || !document.fonts) return
  try {
    await document.fonts.load(`${fontSize}px "${SYMBOL_FALLBACK_FONT}"`)
  } catch {
    // Font missing/unavailable: the chain still degrades to a box, same as before.
  }
}

/**
 * Render the terminal through xterm's DOM renderer rather than its WebGL one.
 *
 * WebGL rasterizes each glyph once into a texture atlas at whole-device-pixel
 * dimensions (`floor(cssCharWidth * dpr)`) and blits that bitmap into every
 * cell. On a 1x display the rounding error is ~8x larger than at 2x: measured at
 * 14px, cells came out 6.6% narrow and only ~20% of glyph ink landed at full
 * opacity, so stems never resolved and text read as unantialiased. The DOM
 * renderer emits real text and lets the platform stack rasterize it — hinting,
 * gamma-corrected blending, subpixel positioning — matching Monaco.
 *
 * KNOWN COST: the WebGL path also *synthesizes* ~169 glyphs as exact vectors
 * sized to the cell (box drawing, block elements, shading, Powerline) via
 * `tryDrawCustomChar`, independent of font. The DOM path has no equivalent and
 * delegates them to the font, so quadrant/half blocks (U+2588-259F) fall short
 * of the cell box and show seams — most visibly in block-element ASCII art.
 *
 * NOT a throughput cost. The previous default was justified by a claim of
 * "130-180ms main-thread stalls on full-screen TUI apps like htop" under the DOM
 * renderer. That does not reproduce. Measured in-app on a 113x59 grid, 60
 * full-screen SGR-heavy repaints, timing each write to its render callback plus
 * a following rAF (so a frame counts only once actually committed):
 *
 *     renderer   p50    p90    p99    max    mean
 *     DOM        8ms    9ms    10ms   10ms   8.33ms
 *     WebGL      8ms    9ms    13ms   13ms   8.30ms
 *
 * Identical at the median and DOM's tail is tighter. Both sit inside a 16ms
 * frame budget. Measure before reverting this on performance grounds — and not
 * with the FPS overlay, which is itself a rAF loop and reports its own cadence.
 */
const USE_DOM_RENDERER = true

export async function resolveTerminalFont(
  requestedFont: string,
  fontSize: number,
): Promise<{ fontFamily: string; skipWebGL: boolean }> {
  // Make sure the icon-glyph fallback is loaded before the terminal renders so
  // Nerd Font symbols don't get cached as tofu in the WebGL atlas.
  await ensureSymbolFallbackFontLoaded(fontSize)

  if (USE_DOM_RENDERER) {
    // No variable -> static mapping here: that existed solely to keep the WebGL
    // glyph atlas alive, and the DOM renderer handles variable cuts natively.
    const preferred = (await loadAndVerifyFont(requestedFont, fontSize))
      ? requestedFont
      : getPlatformFallback()
    return { fontFamily: buildTerminalFontFamily(preferred), skipWebGL: true }
  }

  // Prefer the static cut of a variable font so xterm can keep its WebGL
  // renderer. Only taken when the static cut is actually available.
  const staticEquivalent = deriveStaticFontEquivalent(requestedFont)
  if (staticEquivalent && (await loadAndVerifyFont(staticEquivalent, fontSize))) {
    return {
      fontFamily: buildTerminalFontFamily(staticEquivalent),
      skipWebGL: false,
    }
  }

  const loaded = await loadAndVerifyFont(requestedFont, fontSize)

  if (loaded) {
    return {
      fontFamily: buildTerminalFontFamily(requestedFont),
      // No static cut available; a variable font still forces the DOM renderer.
      skipWebGL: /\bvariable\b/i.test(requestedFont),
    }
  }

  // Font didn't load — use platform native monospace (WebGL-friendly)
  const fallback = getPlatformFallback()
  return {
    fontFamily: buildTerminalFontFamily(fallback),
    skipWebGL: false,
  }
}
