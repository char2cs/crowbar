import type { ThemeDefinition } from './types'
import {
  applyBootstrapAppearance,
  readAppearanceBootstrapCache,
  writeAppearanceBootstrapCache,
  DEFAULT_APPEARANCE_BOOTSTRAP_CACHE,
} from '@/features/settings/lib/appearance-bootstrap'

export { cacheThemeForBootstrap } from '@/features/settings/lib/appearance-bootstrap'

// Colours are CSS-first (theme.css): a theme is metadata plus a `dataTheme` selector, so adding
// one is a `[data-theme="<id>"]` (+ .dark) block and an entry here.
const BUILTIN_THEMES: ThemeDefinition[] = [
  {
    id: 'crowbar',
    name: 'Crowbar',
    isDark: true, // default mode; actual type follows the current Theme Mode
    type: 'dark',
    category: 'Dark',
  },
  {
    id: 'zen',
    name: 'Zen',
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

  /** Colours come from CSS (`data-theme` + `.dark`, toggled just before by applyThemeMode); this
   *  sets the selector and notifies Monaco/xterm to rebuild from the current CSS vars. */
  applyTheme(themeId: string): void {
    const known = this.themes.get(themeId)
    const resolvedId = known ? themeId : 'crowbar'

    this.activeThemeId = resolvedId

    const isDark =
      typeof document !== 'undefined'
        ? document.documentElement.classList.contains('dark')
        : !themeId.includes('light')

    // The resolved theme's flag: a fallback for an unknown id must report the mode on screen, not
    // BUILTIN_THEMES' stale seed value.
    const resolved = known ?? this.themes.get(resolvedId)
    if (resolved) {
      this.themes.set(resolvedId, { ...resolved, isDark })
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
