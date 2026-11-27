import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import {
  readTerminalThemePayload,
  useTerminalTheme,
} from '@/features/terminal/hooks/use-terminal-theme'

const css = vi.hoisted(() => ({
  vars: {} as Record<string, string>,
  ansi: {} as Record<string, string>,
}))

vi.mock('@/features/editor/theme/resolve-css-color', () => ({
  readTerminalPalette: () => css.ansi,
  resolveCssVar: (name: string) => css.vars[name] ?? null,
}))

const ANSI = {
  black: '#1f1f1f',
  red: '#d97757',
  green: '#a3c585',
  yellow: '#d6a95c',
  blue: '#6fb0e0',
  magenta: '#c4a6dd',
  cyan: '#5fbcc4',
  white: '#999999',
  'bright-black': '#999999',
  'bright-red': '#d97757',
  'bright-green': '#a3c585',
  'bright-yellow': '#d6a95c',
  'bright-blue': '#6fb0e0',
  'bright-magenta': '#c4a6dd',
  'bright-cyan': '#5fbcc4',
  'bright-white': '#f5f5f5',
}

css.ansi = ANSI

const UI_VARS = {
  '--foreground': '#f5f5f5',
  '--app-scrollbar-thumb': '#8080806b',
  '--app-scrollbar-thumb-hover': '#80808094',
}

function terminalTheme() {
  return renderHook(() => useTerminalTheme()).result.current.getTerminalTheme()
}

afterEach(() => {
  css.vars = {}
  document.documentElement.classList.remove('dark')
})

describe('useTerminalTheme', () => {
  it('maps ANSI palette keys onto xterm theme fields', () => {
    css.vars = UI_VARS
    const theme = terminalTheme()
    expect(theme.red).toBe('#d97757')
    expect(theme.brightWhite).toBe('#f5f5f5')
    expect(theme.background).toBe('#00000000')
    expect(theme.foreground).toBe('#f5f5f5')
  })

  it('derives a translucent selection from the cursor color', () => {
    css.vars = UI_VARS
    expect(terminalTheme().selectionBackground).toBe('rgba(245, 245, 245, 0.25)')
  })

  it('carries the app scrollbar tokens onto the xterm 6 slider colours', () => {
    css.vars = UI_VARS
    const theme = terminalTheme()
    expect(theme.scrollbarSliderBackground).toBe('#8080806b')
    expect(theme.scrollbarSliderHoverBackground).toBe('#80808094')
    expect(theme.scrollbarSliderActiveBackground).toBe('#80808094')
  })
})

describe('readTerminalThemePayload', () => {
  it('reports the resolved --background/--foreground and dark flag to the daemon', () => {
    css.vars = { '--background': '#101014', '--foreground': '#e8e8e8' }
    document.documentElement.classList.add('dark')
    expect(readTerminalThemePayload()).toEqual({
      background: '#101014',
      foreground: '#e8e8e8',
      dark: true,
    })
  })

  it('carries the light polarity independently of the resolved colours', () => {
    css.vars = { '--background': '#ffffff', '--foreground': '#111111' }
    expect(readTerminalThemePayload()).toEqual({
      background: '#ffffff',
      foreground: '#111111',
      dark: false,
    })
  })

  it('falls back to sane opaque defaults when the vars are unresolved', () => {
    const payload = readTerminalThemePayload()
    expect(payload.dark).toBe(false)
    expect(payload.background).toMatch(/^#[0-9a-f]{6}$/i)
    expect(payload.foreground).toMatch(/^#[0-9a-f]{6}$/i)
  })
})
