import { useCallback } from 'react'
import {
  readTerminalPalette,
  resolveCssVar,
  type TerminalAnsiKey,
} from '@/features/editor/theme/resolve-css-color'

export interface TerminalTheme {
  background: string
  foreground: string
  cursor: string
  cursorAccent: string
  selectionBackground: string
  selectionForeground: string
  black: string
  red: string
  green: string
  yellow: string
  blue: string
  magenta: string
  cyan: string
  white: string
  brightBlack: string
  brightRed: string
  brightGreen: string
  brightYellow: string
  brightBlue: string
  brightMagenta: string
  brightCyan: string
  brightWhite: string
}

export interface TerminalUiTokens {
  foreground: string
  cursor: string
}

const ANSI_FALLBACK = '#808080'

function camel(key: TerminalAnsiKey): keyof TerminalTheme {
  // 'bright-red' -> 'brightRed', 'black' -> 'black'
  return key.replace(/-([a-z])/g, (_, c) => c.toUpperCase()) as keyof TerminalTheme
}

/** #rrggbb -> rgba() with the given alpha (for the selection wash). */
function withAlpha(hex: string, alpha: number): string {
  const h = hex.replace('#', '').slice(0, 6)
  const r = Number.parseInt(h.slice(0, 2), 16)
  const g = Number.parseInt(h.slice(2, 4), 16)
  const b = Number.parseInt(h.slice(4, 6), 16)
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

/** Pure: turn resolved palettes into an xterm ITheme. Unit-tested. */
export function buildTerminalTheme(
  ansi: Partial<Record<TerminalAnsiKey, string>>,
  ui: TerminalUiTokens,
): TerminalTheme {
  const theme = {
    background: '#00000000',
    foreground: ui.foreground,
    cursor: ui.cursor,
    cursorAccent: '#00000000',
    selectionBackground: withAlpha(ui.cursor, 0.25),
    selectionForeground: ui.foreground,
  } as TerminalTheme

  const keys: TerminalAnsiKey[] = [
    'black', 'red', 'green', 'yellow', 'blue', 'magenta', 'cyan', 'white',
    'bright-black', 'bright-red', 'bright-green', 'bright-yellow',
    'bright-blue', 'bright-magenta', 'bright-cyan', 'bright-white',
  ]
  for (const key of keys) {
    theme[camel(key)] = ansi[key] ?? ANSI_FALLBACK
  }
  return theme
}

function readUiTokens(): TerminalUiTokens {
  const isDark =
    typeof document !== 'undefined' && document.documentElement.classList.contains('dark')
  return {
    foreground: resolveCssVar('--foreground') ?? (isDark ? '#f5f5f5' : '#141413'),
    cursor: resolveCssVar('--foreground') ?? (isDark ? '#f5f5f5' : '#141413'),
  }
}

export function useTerminalTheme() {
  const getTerminalTheme = useCallback(
    (): TerminalTheme => buildTerminalTheme(readTerminalPalette(), readUiTokens()),
    [],
  )
  return { getTerminalTheme }
}
