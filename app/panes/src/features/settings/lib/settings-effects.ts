import {
  cacheFontsForBootstrap,
  cacheThemeForBootstrap,
} from '@/features/settings/lib/appearance-bootstrap'
import { setMacOSWindowAppearance } from '@/lib/crowbar-bridge'
import type { Settings, Theme } from '@/features/settings/types/settings'

const ALL_THEME_CLASSES = [
  'force-crowbar-light',
  'force-crowbar-dark',
  'force-vitesse-light',
  'force-vitesse-dark',
]

function applyFallbackTheme(theme: Theme) {
  ALL_THEME_CLASSES.forEach((cls) => document.documentElement.classList.remove(cls))
  document.documentElement.classList.add(`force-${theme}`)
  // Keep .dark in sync so Tailwind dark: variants still apply in error cases.
  document.documentElement.classList.toggle('dark', !theme.includes('light'))
}

type SystemThemePreference = 'light' | 'dark'

let removeThemeSyncListener: (() => void) | null = null

function applyWindowTransparency(enabled: boolean) {
  if (typeof document === 'undefined') return

  document.documentElement.setAttribute(
    'data-window-transparency',
    enabled ? 'enabled' : 'disabled',
  )
}

function getSystemThemePreference(): SystemThemePreference {
  if (typeof window !== 'undefined' && window.matchMedia) {
    try {
      return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
    } catch (error) {
      console.warn('matchMedia not available:', error)
    }
  }

  return 'dark'
}

function applyThemeMode(themeMode: 'light' | 'dark' | 'system') {
  if (typeof document === 'undefined') return
  if (themeMode === 'system') {
    const systemIsDark = getSystemThemePreference() === 'dark'
    document.documentElement.classList.toggle('dark', systemIsDark)
  } else {
    document.documentElement.classList.toggle('dark', themeMode === 'dark')
  }
}

function stopSystemThemeSync() {
  removeThemeSyncListener?.()
  removeThemeSyncListener = null
}

function syncThemeWithSystem(themeMode: 'light' | 'dark' | 'system', onModeChanged?: () => void) {
  if (typeof window === 'undefined' || !window.matchMedia) return
  if (themeMode !== 'system') {
    stopSystemThemeSync()
    return
  }

  const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
  stopSystemThemeSync()

  const handleChange = () => {
    applyThemeMode('system')
    onModeChanged?.()
  }

  mediaQuery.addEventListener('change', handleChange)
  removeThemeSyncListener = () => mediaQuery.removeEventListener('change', handleChange)
}

export async function applyTheme(theme: Theme) {
  if (typeof window === 'undefined') return

  try {
    const { themeRegistry } = await import('@/extensions/themes/theme-registry')

    const syncAppliedTheme = () => {
      themeRegistry.applyTheme(theme)
      // The theme the registry resolved to, not a lookup of the raw id: applyTheme() degrades an
      // unknown id to the default, while getTheme(id) would be undefined and hit the name-guess
      // fallback below, pinning the window dark whatever Theme Mode says.
      const appliedTheme = themeRegistry.getActiveTheme()
      if (appliedTheme) {
        cacheThemeForBootstrap(appliedTheme)
        syncMacOSWindowAppearance(appliedTheme.isDark ? 'dark' : 'light')
        return
      }

      // Genuine registry failure: a stub that resolved to no theme at all.
      // Derive dark/light from the theme name so Tailwind dark: variants apply.
      const isDark = !theme.includes('light')
      document.documentElement.classList.toggle('dark', isDark)
      syncMacOSWindowAppearance(isDark ? 'dark' : 'light')
    }

    if (!themeRegistry.isRegistryReady()) {
      themeRegistry.onReady(syncAppliedTheme)
      return
    }

    syncAppliedTheme()
  } catch (error) {
    console.error('Failed to apply theme via registry:', error)
    applyFallbackTheme(theme)
  }
}

function syncMacOSWindowAppearance(themeType: 'light' | 'dark') {
  void setMacOSWindowAppearance(themeType).catch((error) => {
    console.warn('Failed to sync macOS window appearance', error)
  })
}

function cacheFontSettings(
  settings: Pick<
    Settings,
    'fontFamily' | 'uiFontFamily' | 'uiFontSize' | 'markdownFontSize' | 'headingFontFamily'
  >,
) {
  cacheFontsForBootstrap({
    editorFontFamily: settings.fontFamily,
    uiFontFamily: settings.uiFontFamily,
    uiFontSize: settings.uiFontSize,
    markdownFontSize: settings.markdownFontSize,
    headingFontFamily: settings.headingFontFamily,
  })
}

export function applySettingsSideEffects(settings: Settings) {
  cacheFontSettings(settings)
  applyWindowTransparency(settings.windowTransparency)
  applyThemeMode(settings.themeMode)
  void applyTheme(settings.theme)
  syncThemeWithSystem(settings.themeMode, () => void applyTheme(settings.theme))
}

export function applySettingSideEffect<K extends keyof Settings>(
  key: K,
  value: Settings[K],
  getSettings: () => Settings,
) {
  if (key === 'theme') {
    const settings = getSettings()
    void applyTheme(settings.theme)
    applyThemeMode(settings.themeMode)
  }

  if (key === 'themeMode') {
    const settings = getSettings()
    applyThemeMode(settings.themeMode)
    syncThemeWithSystem(settings.themeMode, () => void applyTheme(settings.theme))
    // Re-apply the theme's color palette for the new light/dark mode
    void applyTheme(settings.theme)
  }

  if (
    key === 'fontFamily' ||
    key === 'uiFontFamily' ||
    key === 'uiFontSize' ||
    key === 'markdownFontSize' ||
    key === 'headingFontFamily'
  ) {
    cacheFontSettings(getSettings())
  }

  if (key === 'windowTransparency') {
    applyWindowTransparency(value as boolean)
  }
}
