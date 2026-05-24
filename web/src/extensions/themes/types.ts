// Stub
export interface ThemeDefinition {
  id: string
  name: string
  type: "light" | "dark"
  isDark: boolean
  colors?: Record<string, string>
  variables?: Record<string, string>
  /** Athas alias for variables */
  cssVariables?: Record<string, string>
  syntaxTokens?: Record<string, unknown>
}
