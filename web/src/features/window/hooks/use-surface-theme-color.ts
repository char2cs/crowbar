import { useEffect } from 'react'
import { useThemeVersion } from '@/features/editor/theme/use-theme-version'
import { useSettingsStore } from '@/features/settings/store'

/** Keeps the page's theme-color on its body's surface colour, through every theme change: the
 *  phone's native chrome paints the strip around an extra webview from it. */
export function useSurfaceThemeColor() {
  const themeVersion = useThemeVersion()
  const theme = useSettingsStore((s) => s.settings.theme)
  useEffect(() => {
    let meta = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')
    if (!meta) {
      meta = document.createElement('meta')
      meta.name = 'theme-color'
      document.head.append(meta)
    }
    meta.content = getComputedStyle(document.body).backgroundColor
  }, [themeVersion, theme])
}
