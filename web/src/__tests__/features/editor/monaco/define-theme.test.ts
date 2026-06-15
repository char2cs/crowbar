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
  error: '#e5484d',
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

  it('calms bracket pair colorization to the muted tone, flags unexpected brackets', () => {
    const { colors } = buildMonacoThemeData({ isDark: true, syntax: SYNTAX, ui: UI })
    expect(colors['editorBracketHighlight.foreground1']).toBe(UI.subtle)
    expect(colors['editorBracketHighlight.foreground6']).toBe(UI.subtle)
    expect(colors['editorBracketHighlight.unexpectedBracket.foreground']).toBe(UI.error)
  })

  it('emits a rule for each semantic legend type from the syntax palette', () => {
    const syntax = {
      keyword: '#d97757',
      function: '#6fb0e0',
      type: '#c4a6dd',
      property: '#cfc9bd',
      operator: '#999999',
      punctuation: '#999999',
      attribute: '#d6a95c',
    }
    const { rules } = buildMonacoThemeData({ isDark: true, syntax, ui: UI })
    const fg = (t: string) => rules.find((r) => r.token === t)?.foreground
    expect(fg('function')).toBe('6fb0e0')
    expect(fg('type')).toBe('c4a6dd')
    expect(fg('property')).toBe('cfc9bd') // not present in the Monarch TOKEN_MAP
    expect(fg('punctuation')).toBe('999999') // not present in the Monarch TOKEN_MAP
    expect(fg('attribute')).toBe('d6a95c')
  })
})
