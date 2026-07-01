import type { ThemeDefinition } from './types'
import {
  applyBootstrapAppearance,
  readAppearanceBootstrapCache,
  writeAppearanceBootstrapCache,
  DEFAULT_APPEARANCE_BOOTSTRAP_CACHE,
} from '@/features/settings/lib/appearance-bootstrap'

export { cacheThemeForBootstrap } from '@/features/settings/lib/appearance-bootstrap'

/**
 * Built-in themes exposed in the UI. Colors are CSS-first (theme.css); a theme
 * is metadata + a `dataTheme` selector. A future installable theme adds a CSS
 * block for `[data-theme="<id>"]` (+ .dark) and one entry here — nothing else.
 */
const BUILTIN_THEMES: ThemeDefinition[] = [
  {
    id: 'crowbar',
    name: 'Crowbar',
    isDark: true, // default mode; actual type follows the current Theme Mode
    type: 'dark',
    category: 'Dark',
  },
  {
    id: 'terra',
    name: 'Terra',
    isDark: true,
    type: 'dark',
    category: 'Dark',
  },
]

export class ThemeRegistry {
  private themes: Map<string, ThemeDefinition> = new Map()
  private activeThemeId: string | null = null
  private listeners: Set<() => void> = new Set()

  constructor() {
    for (const theme of BUILTIN_THEMES) {
      this.themes.set(theme.id, theme)
    }
  }

  getTheme(id: string): ThemeDefinition | undefined {
    return this.themes.get(id)
  }

  getAllThemes(): ThemeDefinition[] {
    return Array.from(this.themes.values())
  }

  getActiveTheme(): ThemeDefinition | null {
    if (!this.activeThemeId) return null
    return this.themes.get(this.activeThemeId) ?? null
  }

  registerTheme(theme: ThemeDefinition): void {
    this.themes.set(theme.id, theme)
    this.notifyListeners()
  }

  /**
   * Apply a theme. Colors come from CSS keyed on `data-theme` + `.dark`
   * (applyThemeMode toggles `.dark` immediately before this runs). We only set
   * the selector attributes, refresh the bootstrap cache, and notify so the
   * Monaco/xterm subscribers rebuild from the now-current CSS vars.
   */
  applyTheme(themeId: string): void {
    const known = this.themes.get(themeId)
    const resolvedId = known ? themeId : 'crowbar'

    this.activeThemeId = resolvedId

    const isDark =
      typeof document !== 'undefined'
        ? document.documentElement.classList.contains('dark')
        : !themeId.includes('light')

    if (known) {
      this.themes.set(resolvedId, { ...known, isDark })
    }

    const existing = readAppearanceBootstrapCache() ?? DEFAULT_APPEARANCE_BOOTSTRAP_CACHE
    const next = {
      ...existing,
      themeId: resolvedId,
      themeType: (isDark ? 'dark' : 'light') as 'dark' | 'light',
    }
    writeAppearanceBootstrapCache(next)
    applyBootstrapAppearance(next)
    this.notifyListeners()
  }

  onThemeChange(callback: () => void): () => void {
    this.listeners.add(callback)
    return () => this.listeners.delete(callback)
  }

  onRegistryChange(callback: () => void): () => void {
    return this.onThemeChange(callback)
  }

  isRegistryReady(): boolean {
    return true
  }

  onReady(cb: () => void): void {
    cb()
  }

  private notifyListeners(): void {
    for (const listener of this.listeners) {
      listener()
    }
  }
}

export const themeRegistry = new ThemeRegistry()
