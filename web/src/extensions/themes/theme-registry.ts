// Stub: theme registry is out of scope for this session.
import type { ThemeDefinition } from "./types"

export class ThemeRegistry {
  getTheme(_name: string): ThemeDefinition | undefined { return undefined }
  getAllThemes(): ThemeDefinition[] { return [] }
  getActiveTheme(): ThemeDefinition | null { return null }
  onThemeChange(_callback: () => void): () => void { return () => {} }
  onRegistryChange(_callback: () => void): () => void { return () => {} }
  applyTheme(_themeId: string): void {}
  registerTheme(_theme: ThemeDefinition): void {}
  isRegistryReady(): boolean { return true }
  onReady(cb: () => void): void { cb() }
}

export const themeRegistry = new ThemeRegistry()
