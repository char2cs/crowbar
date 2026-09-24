import { describe, expect, it } from 'vitest'
import { buildMonacoThemeData } from '@/features/editor/monaco/define-theme'

const SYNTAX = {
  keyword: '#d97757',
  string: '#a3c585',
  function: '#6fb0e0',
  comment: '#999999',
}

const UI = {
  background: '#00000000',
  widgetBackground: '#1f1f1f',
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

  it('sets editor background to transparent (CSS handles it) and foreground from UI tokens', () => {
    const { colors } = buildMonacoThemeData({ isDark: true, syntax: SYNTAX, ui: UI })
    expect(colors['editor.background']).toBe('#00000000')
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

  it('makes the current-line highlight a subtle wash, not a solid block', () => {
    // Live-reported: the selected-line background was "really weird...
    // shouldn't be that much grey" — `ui.border` is an opaque hairline-border
    // color, so using it at full strength painted a solid grey bar.
    const { colors } = buildMonacoThemeData({ isDark: true, syntax: SYNTAX, ui: UI })
    const lineHighlight = colors['editor.lineHighlightBackground']
    expect(lineHighlight.slice(0, 7)).toBe(UI.border.slice(0, 7))
    expect(lineHighlight).toHaveLength(9) // #rrggbb + alpha byte
    expect(lineHighlight.slice(7)).not.toBe('ff')
  })

  it('gives sticky scroll its own opaque background so scrolled text cannot show through it', () => {
    // Live-reported: a function's sticky header sometimes showed the text
    // scrolled underneath leaking through it. Root cause: Monaco defaults
    // editorStickyScroll(Gutter).background to editor.background, which this
    // theme deliberately sets transparent for the CSS pane background — sticky
    // scroll floats OVER scrolled content, so it needs its own opaque color.
    const { colors } = buildMonacoThemeData({ isDark: true, syntax: SYNTAX, ui: UI })
    expect(colors['editorStickyScroll.background']).toBe(UI.widgetBackground)
    expect(colors['editorStickyScrollGutter.background']).toBe(UI.widgetBackground)
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
    expect(fg('method')).toBe('6fb0e0') // server types share their category's color
    expect(fg('decorator')).toBe('d6a95c')
    expect(fg('attribute')).toBe('d6a95c')
  })

  it('colors readonly variables (and shiki constants) as constants', () => {
    const { rules } = buildMonacoThemeData({
      isDark: true,
      syntax: { variable: '#aaaaaa', constant: '#bb5555' },
      ui: UI,
    })
    const fg = (t: string) => rules.find((r) => r.token === t)?.foreground
    expect(fg('variable')).toBe('aaaaaa')
    expect(fg('variable.readonly')).toBe('bb5555')
    expect(fg('enumMember')).toBe('bb5555')
  })
})
