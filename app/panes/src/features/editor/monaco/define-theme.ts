import { editor as monacoEditor } from 'monaco-editor/esm/vs/editor/editor.api.js'
import type * as Monaco from 'monaco-editor'
import {
  readSyntaxPalette,
  resolveCssVar,
  type SyntaxTokenKey,
  withAlpha,
} from '@/features/editor/theme/resolve-css-color'
import { SEMANTIC_TOKEN_RULES } from './semantic-tokens-legend'

const TOKEN_MAP: Array<[monacoToken: string, syntaxKey: SyntaxTokenKey]> = [
  ['comment', 'comment'],
  ['keyword', 'keyword'],
  ['string', 'string'],
  ['number', 'number'],
  ['regexp', 'regex'],
  ['function', 'function'],
  ['variable', 'variable'],
  ['constant', 'constant'],
  ['type', 'type'],
  ['class', 'type'],
  ['interface', 'type'],
  ['namespace', 'type'],
  ['tag', 'tag'],
  ['attribute.name', 'attribute'],
  ['delimiter', 'punctuation'],
  ['delimiter.bracket', 'punctuation'],
  ['operator', 'operator'],
  ['keyword.operator', 'operator'],
  ['keyword.json', 'property'],
  ['string.key.json', 'property'],
]

function stripHash(value: string): string {
  return value.startsWith('#') ? value.slice(1) : value
}

interface MonacoUiTokens {
  background: string
  widgetBackground: string
  foreground: string
  selection: string
  border: string
  subtle: string
  ring: string
  error: string
}

interface MonacoThemeInput {
  isDark: boolean
  syntax: Partial<Record<SyntaxTokenKey, string>>
  ui: MonacoUiTokens
}

interface MonacoThemeData {
  base: 'vs' | 'vs-dark'
  inherit: true
  rules: Monaco.editor.ITokenThemeRule[]
  colors: Record<string, string>
}

function buildMonacoThemeData(input: MonacoThemeInput): MonacoThemeData {
  const { isDark, syntax, ui } = input

  const rules: Monaco.editor.ITokenThemeRule[] = TOKEN_MAP.flatMap(([token, key]) => {
    const color = syntax[key]
    return color ? [{ token, foreground: stripHash(color) }] : []
  })

  for (const [token, key] of SEMANTIC_TOKEN_RULES) {
    const color = syntax[key]
    if (color) rules.push({ token, foreground: stripHash(color) })
  }

  return {
    base: isDark ? 'vs-dark' : 'vs',
    inherit: true,
    rules,
    colors: {
      'editor.background': '#00000000',
      'editor.foreground': ui.foreground,
      'editorCursor.foreground': ui.foreground,
      'editor.selectionBackground': ui.selection,
      'editor.inactiveSelectionBackground': ui.border,
      'editor.findMatchBackground': ui.selection,
      'editor.findMatchHighlightBackground': ui.border,
      focusBorder: ui.ring,
      'editor.lineHighlightBackground': withAlpha(ui.border, 0.08),
      'editorLineNumber.foreground': ui.subtle,
      'editorLineNumber.activeForeground': ui.foreground,
      'editorIndentGuide.background1': ui.border,
      'editorIndentGuide.activeBackground1': ui.subtle,
      'editorWhitespace.foreground': ui.subtle,
      'editorWidget.background': ui.widgetBackground,
      'editorWidget.foreground': ui.foreground,
      'editorWidget.border': ui.border,
      'editorStickyScroll.background': ui.widgetBackground,
      'editorStickyScrollGutter.background': ui.widgetBackground,
      'editorSuggestWidget.background': ui.widgetBackground,
      'editorSuggestWidget.foreground': ui.foreground,
      'editorSuggestWidget.border': ui.border,
      'editorSuggestWidget.selectedBackground': ui.border,
      'input.background': ui.widgetBackground,
      'input.foreground': ui.foreground,
      'input.border': ui.border,
      'editorBracketHighlight.foreground1': ui.subtle,
      'editorBracketHighlight.foreground2': ui.subtle,
      'editorBracketHighlight.foreground3': ui.subtle,
      'editorBracketHighlight.foreground4': ui.subtle,
      'editorBracketHighlight.foreground5': ui.subtle,
      'editorBracketHighlight.foreground6': ui.subtle,
      'editorBracketHighlight.unexpectedBracket.foreground': ui.error,
      'editorGutter.background': '#00000000',
      'minimap.background': '#00000000',
      'minimapSlider.background': ui.border,
      'minimapSlider.hoverBackground': ui.subtle,
      'minimapSlider.activeBackground': ui.subtle,
      'editorOverviewRuler.background': '#00000000',
      'editorOverviewRuler.border': ui.border,
      'scrollbar.shadow': '#00000000',
      'scrollbarSlider.background': ui.border,
      'scrollbarSlider.hoverBackground': ui.subtle,
      'scrollbarSlider.activeBackground': ui.subtle,
    },
  }
}

function readUiTokens(isDark: boolean): MonacoUiTokens {
  return {
    background: '#00000000',
    widgetBackground: resolveCssVar('--background') ?? (isDark ? '#1f1f1f' : '#ffffff'),
    foreground: resolveCssVar('--foreground') ?? (isDark ? '#f5f5f5' : '#1f1f1f'),
    selection:
      resolveCssVar('--editor-selection') ??
      resolveCssVar('--accent') ??
      (isDark ? '#33445566' : '#6a9bcc4d'),
    border: resolveCssVar('--border') ?? (isDark ? '#2a2a2a' : '#e4e7ec'),
    subtle: resolveCssVar('--muted-foreground') ?? (isDark ? '#888888' : '#787d86'),
    ring: resolveCssVar('--ring') ?? (isDark ? '#888888' : '#9aa0a6'),
    error: resolveCssVar('--destructive') ?? (isDark ? '#e5484d' : '#c0392b'),
  }
}

function toMonacoThemeName(isDark: boolean): string {
  return isDark ? 'crowbar-dark' : 'crowbar-light'
}

export function defineMonacoTheme(themeId: string): string {
  const isDark =
    typeof document !== 'undefined'
      ? document.documentElement.classList.contains('dark')
      : !themeId.includes('light')

  const data = buildMonacoThemeData({
    isDark,
    syntax: readSyntaxPalette(),
    ui: readUiTokens(isDark),
  })

  const name = toMonacoThemeName(isDark)
  monacoEditor.defineTheme(name, data)
  return name
}
