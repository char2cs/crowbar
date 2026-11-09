import { describe, expect, it } from 'vitest'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'
import { themeRegistry } from '@/extensions/themes/theme-registry'

describe('themeRegistry built-in themes', () => {
  it('offers Crowbar and Frosted, and no Zen', () => {
    const themes = themeRegistry.getAllThemes()

    expect(themes.map((theme) => [theme.id, theme.name])).toEqual([
      ['crowbar', 'Crowbar'],
      ['frosted', 'Frosted'],
    ])
    expect(themeRegistry.getTheme('zen')).toBeUndefined()
  })

  it('makes Crowbar the default theme', () => {
    expect(getDefaultSettingsSnapshot().theme).toBe('crowbar')
  })
})
