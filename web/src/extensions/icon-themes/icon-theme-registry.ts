// Stub
import type { IconThemeDefinition } from "./types"

export class IconThemeRegistry {
  getIconTheme(_name: string): IconThemeDefinition | null { return null }
  getTheme(_name: string): IconThemeDefinition | null { return null }
  getAllIconThemes(): IconThemeDefinition[] { return [] }
  getAllThemes(): IconThemeDefinition[] { return [] }
  getActiveTheme(): IconThemeDefinition | null { return null }
  onThemeChange(_callback: () => void): () => void { return () => {} }
  onRegistryChange(_callback: () => void): () => void { return () => {} }
}
export const iconThemeRegistry = new IconThemeRegistry()
