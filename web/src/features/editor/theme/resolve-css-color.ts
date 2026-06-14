/**
 * CSS-first color resolution. The canonical color values live in theme.css as
 * CSS custom properties (OKLCH / rgb / hex). Monaco and xterm cannot read CSS
 * variables, so we resolve them off the DOM and convert to #hex here.
 */

function clamp255(n: number): number {
  return Math.max(0, Math.min(255, Math.round(n)))
}

function toHexByte(n: number): string {
  return clamp255(n).toString(16).padStart(2, '0')
}

function expandShortHex(hex: string): string {
  if (hex.length === 4) {
    const [, r, g, b] = hex
    return `#${r}${r}${g}${g}${b}${b}`
  }
  return hex
}

function gammaEncode(c: number): number {
  const v = c <= 0.0031308 ? 12.92 * c : 1.055 * c ** (1 / 2.4) - 0.055
  return v * 255
}

/** OKLCH → sRGB hex. Math per Björn Ottosson's OKLab reference. */
function oklchToHex(l: number, c: number, hDeg: number, alpha: number): string {
  const h = (hDeg * Math.PI) / 180
  const a = c * Math.cos(h)
  const b = c * Math.sin(h)

  const l_ = l + 0.3963377774 * a + 0.2158037573 * b
  const m_ = l - 0.1055613458 * a - 0.0638541728 * b
  const s_ = l - 0.0894841775 * a - 1.291485548 * b

  const lc = l_ ** 3
  const mc = m_ ** 3
  const sc = s_ ** 3

  const r = 4.0767416621 * lc - 3.3077115913 * mc + 0.2309699292 * sc
  const g = -1.2684380046 * lc + 2.6097574011 * mc - 0.3413193965 * sc
  const bl = -0.0041960863 * lc - 0.7034186147 * mc + 1.707614701 * sc

  const hex = `#${toHexByte(gammaEncode(r))}${toHexByte(gammaEncode(g))}${toHexByte(gammaEncode(bl))}`
  return alpha >= 1 ? hex : `${hex}${toHexByte(alpha * 255)}`
}

function parseAlpha(raw: string | undefined): number {
  if (!raw) return 1
  const t = raw.trim()
  if (t.endsWith('%')) return Number.parseFloat(t) / 100
  return Number.parseFloat(t)
}

/**
 * Convert a CSS color string to #rrggbb (or #rrggbbaa when alpha < 1).
 * Supports the formats used in theme.css: #hex/#rgb, rgb()/rgba(), oklch().
 * Returns null if the value is empty or unrecognized.
 */
export function cssColorToHex(value: string): string | null {
  const v = value.trim().toLowerCase()
  if (!v) return null

  if (/^#[0-9a-f]{3}$/.test(v)) return expandShortHex(v)
  if (/^#[0-9a-f]{6}([0-9a-f]{2})?$/.test(v)) return v

  const rgb = v.match(
    /^rgba?\(\s*([\d.]+)[\s,]+([\d.]+)[\s,]+([\d.]+)(?:[\s,/]+([\d.]+%?))?\s*\)$/,
  )
  if (rgb) {
    const [, r, g, b, a] = rgb
    const alpha = parseAlpha(a)
    const hex = `#${toHexByte(Number(r))}${toHexByte(Number(g))}${toHexByte(Number(b))}`
    return alpha >= 1 ? hex : `${hex}${toHexByte(alpha * 255)}`
  }

  const oklch = v.match(
    /^oklch\(\s*([\d.]+%?)\s+([\d.]+)\s+([\d.]+)(?:\s*\/\s*([\d.]+%?))?\s*\)$/,
  )
  if (oklch) {
    const [, lRaw, cRaw, hRaw, aRaw] = oklch
    const l = lRaw.endsWith('%') ? Number.parseFloat(lRaw) / 100 : Number.parseFloat(lRaw)
    return oklchToHex(l, Number(cRaw), Number(hRaw), parseAlpha(aRaw))
  }

  return null
}

/** Syntax token keys → their CSS variable is `--syntax-<key>`. */
export const SYNTAX_TOKEN_KEYS = [
  'keyword', 'string', 'number', 'constant', 'comment', 'variable', 'property',
  'type', 'function', 'operator', 'punctuation', 'tag', 'attribute', 'boolean',
  'null', 'regex', 'jsx', 'jsx-attribute', 'error',
  'markdown-heading', 'markdown-bold', 'markdown-italic', 'markdown-strikethrough',
  'markdown-link', 'markdown-link-text', 'markdown-code', 'markdown-list', 'markdown-quote',
] as const
export type SyntaxTokenKey = (typeof SYNTAX_TOKEN_KEYS)[number]

/** ANSI keys → their CSS variable is `--terminal-<key>`. */
export const TERMINAL_ANSI_KEYS = [
  'black', 'red', 'green', 'yellow', 'blue', 'magenta', 'cyan', 'white',
  'bright-black', 'bright-red', 'bright-green', 'bright-yellow',
  'bright-blue', 'bright-magenta', 'bright-cyan', 'bright-white',
] as const
export type TerminalAnsiKey = (typeof TERMINAL_ANSI_KEYS)[number]

/** Resolve a single CSS variable on <html> to #hex, or null if unset/unparseable. */
export function resolveCssVar(name: string, el: Element = document.documentElement): string | null {
  const raw = getComputedStyle(el).getPropertyValue(name)
  return cssColorToHex(raw)
}

function readPalette<K extends string>(keys: readonly K[], prefix: string): Partial<Record<K, string>> {
  const out: Partial<Record<K, string>> = {}
  for (const key of keys) {
    const hex = resolveCssVar(`${prefix}${key}`)
    if (hex) out[key] = hex
  }
  return out
}

export function readSyntaxPalette(): Partial<Record<SyntaxTokenKey, string>> {
  return readPalette(SYNTAX_TOKEN_KEYS, '--syntax-')
}

export function readTerminalPalette(): Partial<Record<TerminalAnsiKey, string>> {
  return readPalette(TERMINAL_ANSI_KEYS, '--terminal-')
}
