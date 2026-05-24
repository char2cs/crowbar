// Stub: theme registry is out of scope for this session.
import type { ThemeDefinition } from "./types"

export class ThemeRegistry {
  getTheme(_name: string): ThemeDefinition | null { return null }
  getAllThemes(): ThemeDefinition[] { return [] }
  getActiveTheme(): ThemeDefinition | null { return null }
  onThemeChange(_callback: () => void): () => void { return () => {} }
  onRegistryChange(_callback: () => void): () => void { return () => {} }
  applyTheme(_themeId: string): void {}
  registerTheme(_theme: ThemeDefinition): void {}
}

export const themeRegistry = new ThemeRegistry()
