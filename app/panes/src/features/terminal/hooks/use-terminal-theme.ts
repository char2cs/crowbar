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
  scrollbarSliderBackground: string
  scrollbarSliderHoverBackground: string
  scrollbarSliderActiveBackground: string
}

export interface TerminalUiTokens {
  foreground: string
  cursor: string
  scrollbarThumb: string
  scrollbarThumbHover: string
}

const ANSI_FALLBACK = '#808080'

function camel(key: TerminalAnsiKey): keyof TerminalTheme {
  return key.replace(/-([a-z])/g, (_, c) => c.toUpperCase()) as keyof TerminalTheme
}

function withAlpha(hex: string, alpha: number): string {
  const h = hex.replace('#', '').slice(0, 6)
  const r = Number.parseInt(h.slice(0, 2), 16)
  const g = Number.parseInt(h.slice(2, 4), 16)
  const b = Number.parseInt(h.slice(4, 6), 16)
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

function buildTerminalTheme(
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
    scrollbarSliderBackground: ui.scrollbarThumb,
    scrollbarSliderHoverBackground: ui.scrollbarThumbHover,
    scrollbarSliderActiveBackground: ui.scrollbarThumbHover,
  } as TerminalTheme

  const keys: TerminalAnsiKey[] = [
    'black',
    'red',
    'green',
    'yellow',
    'blue',
    'magenta',
    'cyan',
    'white',
    'bright-black',
    'bright-red',
    'bright-green',
    'bright-yellow',
    'bright-blue',
    'bright-magenta',
    'bright-cyan',
    'bright-white',
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
    scrollbarThumb: resolveCssVar('--app-scrollbar-thumb') ?? '#8080806b',
    scrollbarThumbHover: resolveCssVar('--app-scrollbar-thumb-hover') ?? '#80808094',
  }
}

export interface TerminalThemePayload {
  background: string
  foreground: string
  dark: boolean
}

function buildTerminalThemePayload(
  resolve: (name: string) => string | null,
  isDark: boolean,
): TerminalThemePayload {
  return {
    background: resolve('--background') ?? (isDark ? '#141413' : '#faf9f5'),
    foreground: resolve('--foreground') ?? (isDark ? '#f5f5f5' : '#141413'),
    dark: isDark,
  }
}

export function readTerminalThemePayload(): TerminalThemePayload {
  const isDark =
    typeof document !== 'undefined' && document.documentElement.classList.contains('dark')
  return buildTerminalThemePayload((name) => resolveCssVar(name), isDark)
}

export function useTerminalTheme() {
  const getTerminalTheme = useCallback(
    (): TerminalTheme => buildTerminalTheme(readTerminalPalette(), readUiTokens()),
    [],
  )
  return { getTerminalTheme }
}
