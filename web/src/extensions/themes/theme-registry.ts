import type { ThemeDefinition } from './types'
import {
  CROWBAR_BOOTSTRAP_DEFAULTS,
  applyBootstrapAppearance,
  readAppearanceBootstrapCache,
  writeAppearanceBootstrapCache,
  sanitizeVarMap,
  DEFAULT_APPEARANCE_BOOTSTRAP_CACHE,
} from '@/features/settings/lib/appearance-bootstrap'

// Re-export so callers can still use it from here
export {
  cacheThemeForBootstrap,
  sanitizeVarMap,
} from '@/features/settings/lib/appearance-bootstrap'

function prefixRecord(prefix: string, record: Record<string, string>): Record<string, string> {
  const result: Record<string, string> = {}
  for (const [key, value] of Object.entries(record)) {
    result[`${prefix}${key}`] = value
  }
  return result
}

/** A theme that has both a light and a dark palette. Theme Mode picks which one to apply. */
interface DualModeVariants {
  dark: { cssVariables: Record<string, string>; syntaxTokens: Record<string, string> }
  light: { cssVariables: Record<string, string>; syntaxTokens: Record<string, string> }
}

const CROWBAR_VARIANTS: DualModeVariants = {
  dark: {
    cssVariables: prefixRecord('--', CROWBAR_BOOTSTRAP_DEFAULTS.dark.colors),
    syntaxTokens: prefixRecord('--syntax-', CROWBAR_BOOTSTRAP_DEFAULTS.dark.syntax),
  },
  light: {
    cssVariables: prefixRecord('--', CROWBAR_BOOTSTRAP_DEFAULTS.light.colors),
    syntaxTokens: prefixRecord('--syntax-', CROWBAR_BOOTSTRAP_DEFAULTS.light.syntax),
  },
}

/** Built-in themes exposed in the UI. Each theme handles its own light/dark palette internally. */
const BUILTIN_THEMES: ThemeDefinition[] = [
  {
    id: 'crowbar',
    name: 'Crowbar',
    isDark: true, // default palette is dark; actual type follows current Theme Mode
    type: 'dark',
    category: 'Dark',
    // cssVariables are intentionally left unset here — applyTheme picks the right palette at runtime
    cssVariables: CROWBAR_VARIANTS.dark.cssVariables,
    syntaxTokens: CROWBAR_VARIANTS.dark.syntaxTokens,
  },
]

export class ThemeRegistry {
  private themes: Map<string, ThemeDefinition> = new Map()
  private dualModeVariants: Map<string, DualModeVariants> = new Map()
  private activeThemeId: string | null = null
  private listeners: Set<() => void> = new Set()

  constructor() {
    for (const theme of BUILTIN_THEMES) {
      this.themes.set(theme.id, theme)
    }
    this.dualModeVariants.set('crowbar', CROWBAR_VARIANTS)
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
   * Apply a theme. For dual-mode themes (e.g. "crowbar"), picks the dark or light
   * palette based on whether `<html>` currently has the `.dark` class — which is set
   * by `applyThemeMode` immediately before this is called in the settings side-effects.
   */
  applyTheme(themeId: string): void {
    const theme = this.themes.get(themeId)
    if (!theme) {
      // Unknown theme — fall back to toggling dark class by name convention
      if (typeof document !== 'undefined') {
        document.documentElement.classList.toggle('dark', !themeId.includes('light'))
      }
      return
    }

    this.activeThemeId = themeId

    // Determine the active mode from the DOM (applyThemeMode runs before applyTheme)
    const isDark =
      typeof document !== 'undefined' && document.documentElement.classList.contains('dark')

    // Pick the right palette for dual-mode themes; fall back to theme's own cssVariables
    const variants = this.dualModeVariants.get(themeId)
    const palette = variants
      ? isDark
        ? variants.dark
        : variants.light
      : { cssVariables: theme.cssVariables ?? {}, syntaxTokens: theme.syntaxTokens ?? {} }

    // Update the in-memory theme definition so getTheme() returns the correct isDark flag
    // and syntax tokens for the current mode — Monaco reads these to build its token rules.
    this.themes.set(themeId, {
      ...theme,
      isDark,
      cssVariables: sanitizeVarMap(palette.cssVariables),
      syntaxTokens: sanitizeVarMap(palette.syntaxTokens),
    })

    const existing = readAppearanceBootstrapCache() ?? DEFAULT_APPEARANCE_BOOTSTRAP_CACHE
    const next = {
      ...existing,
      themeId,
      themeType: (isDark ? 'dark' : 'light') as 'dark' | 'light',
      cssVariables: sanitizeVarMap(palette.cssVariables),
      syntaxTokens: sanitizeVarMap(palette.syntaxTokens),
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
