import type { IconThemeDefinition, IconThemeSource } from './types'

class IconThemeRegistry {
  private themes: Map<string, IconThemeDefinition> = new Map()
  private themeSources: Map<string, IconThemeSource> = new Map()
  private listeners: Set<() => void> = new Set()

  registerTheme(theme: IconThemeDefinition, source?: IconThemeSource) {
    this.themes.set(theme.id, theme)
    if (source) {
      this.themeSources.set(theme.id, source)
    } else {
      this.themeSources.delete(theme.id)
    }
    this.notifyListeners()
  }

  unregisterTheme(id: string) {
    this.themes.delete(id)
    this.themeSources.delete(id)
    this.notifyListeners()
  }

  unregisterThemesByExtension(extensionId: string) {
    // react-doctor-disable-next-line js-combine-iterations -- themeSources is bounded by installed extensions' registered icon themes (a handful, total); called only on extension unregister, not a hot path.
    const themeIds = Array.from(this.themeSources.entries())
      .filter(([, source]) => source.extensionId === extensionId)
      .map(([themeId]) => themeId)

    for (const themeId of themeIds) {
      this.themes.delete(themeId)
      this.themeSources.delete(themeId)
    }

    if (themeIds.length > 0) {
      this.notifyListeners()
    }
  }

  getThemeSource(id: string): IconThemeSource | undefined {
    return this.themeSources.get(id)
  }

  getThemesByExtension(extensionId: string): IconThemeDefinition[] {
    // react-doctor-disable-next-line js-combine-iterations -- themeSources is bounded by installed extensions' registered icon themes (a handful, total); not a hot path.
    return Array.from(this.themeSources.entries())
      .filter(([, source]) => source.extensionId === extensionId)
      .map(([themeId]) => this.themes.get(themeId))
      .filter((theme): theme is IconThemeDefinition => Boolean(theme))
  }

  clearExtension(extensionId: string) {
    this.unregisterThemesByExtension(extensionId)
  }

  markBundledTheme(id: string) {
    const theme = this.themes.get(id)
    if (!theme) return
    this.themeSources.set(id, { extensionId: 'builtin', isBundled: true })
    this.notifyListeners()
  }

  getTheme(id: string): IconThemeDefinition | undefined {
    return this.themes.get(id)
  }

  getAllThemes(): IconThemeDefinition[] {
    return Array.from(this.themes.values())
  }

  // Alias kept for backward compat with existing callers
  getIconTheme(id: string): IconThemeDefinition | undefined {
    return this.getTheme(id)
  }

  // Alias kept for backward compat
  getAllIconThemes(): IconThemeDefinition[] {
    return this.getAllThemes()
  }

  onRegistryChange(callback: () => void): () => void {
    this.listeners.add(callback)
    return () => this.listeners.delete(callback)
  }

  private notifyListeners() {
    for (const listener of this.listeners) {
      listener()
    }
  }
}

export const iconThemeRegistry = new IconThemeRegistry()
