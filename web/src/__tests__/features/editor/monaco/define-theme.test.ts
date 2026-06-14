import { describe, expect, it } from 'vitest'
import { buildMonacoThemeData } from '@/features/editor/monaco/define-theme'

const SYNTAX = {
  keyword: '#d97757',
  string: '#a3c585',
  function: '#6fb0e0',
  comment: '#999999',
}

const UI = {
  background: '#1f1f1f',
  foreground: '#f5f5f5',
  selection: '#33445566',
  border: '#2a2a2a',
  subtle: '#888888',
  ring: '#778899',
}

describe('buildMonacoThemeData', () => {
  it('uses vs-dark base in dark mode and vs in light', () => {
    expect(buildMonacoThemeData({ isDark: true, syntax: SYNTAX, ui: UI }).base).toBe('vs-dark')
    expect(buildMonacoThemeData({ isDark: false, syntax: SYNTAX, ui: UI }).base).toBe('vs')
  })

  it('maps syntax tokens to rules without the leading #', () => {
    const { rules } = buildMonacoThemeData({ isDark: true, syntax: SYNTAX, ui: UI })
    const keyword = rules.find((r) => r.token === 'keyword')
    expect(keyword?.foreground).toBe('d97757')
  })

  it('skips tokens that are missing from the palette (no crash)', () => {
    const { rules } = buildMonacoThemeData({ isDark: true, syntax: { keyword: '#d97757' }, ui: UI })
    expect(rules.some((r) => r.token === 'string')).toBe(false)
    expect(rules.some((r) => r.token === 'keyword')).toBe(true)
  })

  it('sets editor background/foreground from UI tokens', () => {
    const { colors } = buildMonacoThemeData({ isDark: true, syntax: SYNTAX, ui: UI })
    expect(colors['editor.background']).toBe('#1f1f1f')
    expect(colors['editor.foreground']).toBe('#f5f5f5')
  })

  it('sets find-match and focus colors from selection/ring', () => {
    const { colors } = buildMonacoThemeData({ isDark: true, syntax: SYNTAX, ui: UI })
    expect(colors['editor.findMatchBackground']).toBe(UI.selection)
    expect(colors['focusBorder']).toBe(UI.ring)
  })
})
