interface ThemeTokens {
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

  chromeBg?: string

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
  cssVariables?: Record<string, string>
  syntaxTokens?: Record<string, unknown>
  tokens?: ThemeTokens
}
