// Stub

/**
 * Strongly-typed map of all CSS design tokens.
 * camelCase keys map to kebab-case CSS variable names:
 *   syntaxKeyword → --syntax-keyword
 *   appScrollbarThumb → --app-scrollbar-thumb
 */
export interface ThemeTokens {
  // shadcn base tokens
  background: string
  foreground: string
  card: string
  cardForeground: string
  popover: string
  popoverForeground: string
  primary: string
  primaryForeground: string
  secondary: string
  secondaryForeground: string
  muted: string
  mutedForeground: string
  accent: string
  accentForeground: string
  destructive: string
  border: string
  input: string
  ring: string

  // chrome tokens
  chromeBg?: string

  // custom tokens — no shadcn equivalent
  warning: string
  info: string
  editorFontFamily: string
  uiTextXs: string
  uiTextSm: string
  appScrollbarSize: string
  appScrollbarThumb: string
  appScrollbarThumbBorder: string
  appScrollbarThumbHover: string
  appScrollbarTrack: string
  appScrollbarRadius: string

  // syntax highlighting tokens (27)
  syntaxKeyword: string
  syntaxString: string
  syntaxNumber: string
  syntaxConstant: string
  syntaxComment: string
  syntaxVariable: string
  syntaxProperty: string
  syntaxType: string
  syntaxFunction: string
  syntaxOperator: string
  syntaxPunctuation: string
  syntaxTag: string
  syntaxAttribute: string
  syntaxBoolean: string
  syntaxNull: string
  syntaxRegex: string
  syntaxJsx: string
  syntaxJsxAttribute: string
  syntaxMarkdownHeading: string
  syntaxMarkdownBold: string
  syntaxMarkdownItalic: string
  syntaxMarkdownStrikethrough: string
  syntaxMarkdownLink: string
  syntaxMarkdownLinkText: string
  syntaxMarkdownCode: string
  syntaxMarkdownList: string
  syntaxMarkdownQuote: string
}

export interface ThemeDefinition {
  id: string
  name: string
  type?: 'light' | 'dark'
  isDark: boolean
  description?: string
  category?: 'System' | 'Light' | 'Dark' | 'Colorful'
  icon?: React.ReactNode
  colors?: Record<string, string>
  variables?: Record<string, string>
  /** @deprecated Use `tokens` instead */
  cssVariables?: Record<string, string>
  syntaxTokens?: Record<string, unknown>
  /** Strongly-typed token map — preferred over cssVariables */
  tokens?: ThemeTokens
}
