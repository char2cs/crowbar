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

function buildTerminalFontFamily(primaryFont: string): string {
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

async function loadAndVerifyFont(fontFamily: string, fontSize: number): Promise<boolean> {
  const testString = `${fontSize}px "${fontFamily}"`

  try {
    await document.fonts.load(testString)
  } catch {
    return false
  }

  return document.fonts.check(testString)
}

const SYMBOL_FALLBACK_FONT = 'Symbols Nerd Font Mono'

async function ensureSymbolFallbackFontLoaded(fontSize: number): Promise<void> {
  if (typeof document === 'undefined' || !document.fonts) return
  try {
    await document.fonts.load(`${fontSize}px "${SYMBOL_FALLBACK_FONT}"`)
  } catch {
    return
  }
}

export async function resolveTerminalFont(
  requestedFont: string,
  fontSize: number,
): Promise<string> {
  await ensureSymbolFallbackFontLoaded(fontSize)
  const preferred = (await loadAndVerifyFont(requestedFont, fontSize))
    ? requestedFont
    : getPlatformFallback()
  return buildTerminalFontFamily(preferred)
}
