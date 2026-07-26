import { describe, expect, it } from 'vitest'
import {
  buildTerminalTheme,
  buildTerminalThemePayload,
} from '@/features/terminal/hooks/use-terminal-theme'

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
const UI = {
  foreground: '#f5f5f5',
  cursor: '#f5f5f5',
  scrollbarThumb: '#8080806b',
  scrollbarThumbHover: '#80808094',
}

describe('buildTerminalTheme', () => {
  it('maps ANSI palette keys onto xterm theme fields', () => {
    const theme = buildTerminalTheme(ANSI, UI)
    expect(theme.red).toBe('#d97757')
    expect(theme.brightWhite).toBe('#f5f5f5')
    // Background is intentionally transparent so the CSS --pane-background
    // shows through the xterm canvas (commit beea46e); the ui.background
    // token is no longer consumed.
    expect(theme.background).toBe('#00000000')
    expect(theme.foreground).toBe('#f5f5f5')
  })

  it('derives a translucent selection from the cursor color', () => {
    const theme = buildTerminalTheme(ANSI, UI)
    expect(theme.selectionBackground).toBe('rgba(245, 245, 245, 0.25)')
  })

  it('carries the app scrollbar tokens onto the xterm 6 slider colours', () => {
    // xterm 6 paints the scrollbar from the theme, not from CSS, so these are
    // the only lever keeping the terminal's scrollbar matching the app's.
    const theme = buildTerminalTheme(ANSI, UI)
    expect(theme.scrollbarSliderBackground).toBe('#8080806b')
    expect(theme.scrollbarSliderHoverBackground).toBe('#80808094')
    expect(theme.scrollbarSliderActiveBackground).toBe('#80808094')
  })
})

describe('buildTerminalThemePayload', () => {
  it('reports the resolved --background/--foreground and dark flag to the daemon', () => {
    const resolve = (name: string) =>
      name === '--background' ? '#101014' : name === '--foreground' ? '#e8e8e8' : null
    expect(buildTerminalThemePayload(resolve, true)).toEqual({
      background: '#101014',
      foreground: '#e8e8e8',
      dark: true,
    })
  })

  it('carries the light polarity independently of the resolved colours', () => {
    const resolve = (name: string) => (name === '--background' ? '#ffffff' : '#111111')
    expect(buildTerminalThemePayload(resolve, false)).toEqual({
      background: '#ffffff',
      foreground: '#111111',
      dark: false,
    })
  })

  it('falls back to sane opaque defaults when the vars are unresolved', () => {
    const payload = buildTerminalThemePayload(() => null, false)
    expect(payload.dark).toBe(false)
    expect(payload.background).toMatch(/^#[0-9a-f]{6}$/i)
    expect(payload.foreground).toMatch(/^#[0-9a-f]{6}$/i)
  })
})
