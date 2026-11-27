import { useEffect } from 'react'
import { useThemeVersion } from '@/features/editor/theme/use-theme-version'
import { useSettingsStore } from '@/features/settings/store'

const opaque = (css: string) =>
  css.replace(/^rgba\(([^,]+,[^,]+,[^,]+),[^)]*\)$/, 'rgb($1)').replace(/\s*\/\s*[\d.]+%?\)$/, ')')

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
    meta.content = opaque(getComputedStyle(document.body).backgroundColor)
  }, [themeVersion, theme])
}
