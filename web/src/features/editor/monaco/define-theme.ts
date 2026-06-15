/**
 * Builds + registers a Monaco theme from the live CSS token layer (theme.css).
 * CSS-first: every color is resolved off <html> at call time, so the Monaco
 * theme always matches whatever .dark / [data-theme] is currently applied.
 */

import { editor as monacoEditor } from 'monaco-editor'
import type * as Monaco from 'monaco-editor'
import {
  readSyntaxPalette,
  resolveCssVar,
  type SyntaxTokenKey,
} from '@/features/editor/theme/resolve-css-color'

/** Monaco token scope → our syntax palette key. */
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

export interface MonacoUiTokens {
  background: string
  foreground: string
  selection: string
  border: string
  subtle: string
  ring: string
  error: string
}

export interface MonacoThemeInput {
  isDark: boolean
  syntax: Partial<Record<SyntaxTokenKey, string>>
  ui: MonacoUiTokens
}

export interface MonacoThemeData {
  base: 'vs' | 'vs-dark'
  inherit: true
  rules: Monaco.editor.ITokenThemeRule[]
  colors: Record<string, string>
}

/** Pure: turn resolved palettes into Monaco theme data. Unit-tested. */
export function buildMonacoThemeData(input: MonacoThemeInput): MonacoThemeData {
  const { isDark, syntax, ui } = input

  const rules: Monaco.editor.ITokenThemeRule[] = TOKEN_MAP.flatMap(([token, key]) => {
    const color = syntax[key]
    return color ? [{ token, foreground: stripHash(color) }] : []
  })

  return {
    base: isDark ? 'vs-dark' : 'vs',
    inherit: true,
    rules,
    colors: {
      'editor.background': ui.background,
      'editor.foreground': ui.foreground,
      'editorCursor.foreground': ui.foreground,
      'editor.selectionBackground': ui.selection,
      'editor.inactiveSelectionBackground': ui.border,
      'editor.findMatchBackground': ui.selection,
      'editor.findMatchHighlightBackground': ui.border,
      focusBorder: ui.ring,
      'editor.lineHighlightBackground': ui.border,
      'editorLineNumber.foreground': ui.subtle,
      'editorLineNumber.activeForeground': ui.foreground,
      'editorIndentGuide.background1': ui.border,
      'editorIndentGuide.activeBackground1': ui.subtle,
      'editorWhitespace.foreground': ui.subtle,
      'editorWidget.background': ui.background,
      'editorWidget.foreground': ui.foreground,
      'editorWidget.border': ui.border,
      'editorSuggestWidget.background': ui.background,
      'editorSuggestWidget.foreground': ui.foreground,
      'editorSuggestWidget.border': ui.border,
      'editorSuggestWidget.selectedBackground': ui.border,
      'input.background': ui.background,
      'input.foreground': ui.foreground,
      'input.border': ui.border,
      // Bracket pair colorization is on by default in Monaco; without these it
      // falls back to a loud stock gold/orchid/blue rainbow that fights the
      // palette. Calm it to the muted punctuation tone so brackets read like the
      // rest of the delimiters; only genuinely unmatched brackets get flagged.
      'editorBracketHighlight.foreground1': ui.subtle,
      'editorBracketHighlight.foreground2': ui.subtle,
      'editorBracketHighlight.foreground3': ui.subtle,
      'editorBracketHighlight.foreground4': ui.subtle,
      'editorBracketHighlight.foreground5': ui.subtle,
      'editorBracketHighlight.foreground6': ui.subtle,
      'editorBracketHighlight.unexpectedBracket.foreground': ui.error,
    },
  }
}

function readUiTokens(isDark: boolean): MonacoUiTokens {
  return {
    background: resolveCssVar('--background') ?? (isDark ? '#1f1f1f' : '#ffffff'),
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

/**
 * Resolve the live token layer, (re)define the Monaco theme, and return its id.
 * Signature is unchanged from the previous implementation; `themeId` only seeds
 * the dark/light decision as a fallback when no .dark class is present.
 */
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
