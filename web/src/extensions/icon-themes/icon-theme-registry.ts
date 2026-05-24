// Stub
export class IconThemeRegistry {
  getIconTheme(_name: string): null { return null }
  getTheme(_name: string): null { return null }
  getAllIconThemes(): unknown[] { return [] }
  getAllThemes(): unknown[] { return [] }
  getActiveTheme(): null { return null }
  onThemeChange(_callback: () => void): () => void { return () => {} }
  onRegistryChange(_callback: () => void): () => void { return () => {} }
}
export const iconThemeRegistry = new IconThemeRegistry()
