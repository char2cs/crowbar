import { describe, expect, it } from 'vitest'
import { buildTerminalTheme } from '@/features/terminal/hooks/use-terminal-theme'

const ANSI = {
  black: '#1f1f1f', red: '#d97757', green: '#a3c585', yellow: '#d6a95c',
  blue: '#6fb0e0', magenta: '#c4a6dd', cyan: '#5fbcc4', white: '#999999',
  'bright-black': '#999999', 'bright-red': '#d97757', 'bright-green': '#a3c585',
  'bright-yellow': '#d6a95c', 'bright-blue': '#6fb0e0', 'bright-magenta': '#c4a6dd',
  'bright-cyan': '#5fbcc4', 'bright-white': '#f5f5f5',
}
const UI = { foreground: '#f5f5f5', cursor: '#f5f5f5' }

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
})
