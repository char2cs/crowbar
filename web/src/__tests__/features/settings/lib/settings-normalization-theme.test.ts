import { describe, expect, it } from 'vitest'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'
import {
  normalizeSettings,
  normalizeSettingValue,
} from '@/features/settings/lib/settings-normalization'

/**
 * `Theme` is a bare `string`, so a persisted id can outlive the theme it names —
 * 'terra' was renamed to 'zen' and its stylesheet deleted. Nothing coerced the
 * dead id, so every consumer had to guess: the Color Theme picker showed a value
 * absent from its own menu, and the appearance effect force-added `.dark`.
 * Coerce at READ time (graceful fallback), never with a migration pass.
 */
describe('normalizeSettings theme coercion', () => {
  it('coerces a theme id the registry does not know to the default theme', () => {
    const normalized = normalizeSettings({ ...getDefaultSettingsSnapshot(), theme: 'terra' })

    expect(normalized.theme).toBe('crowbar')
  })

  it('keeps a registered theme id untouched', () => {
    expect(normalizeSettings({ ...getDefaultSettingsSnapshot(), theme: 'zen' }).theme).toBe('zen')
    expect(normalizeSettings({ ...getDefaultSettingsSnapshot(), theme: 'crowbar' }).theme).toBe(
      'crowbar',
    )
  })

  it('coerces a non-string persisted theme', () => {
    const normalized = normalizeSettings({
      ...getDefaultSettingsSnapshot(),
      theme: undefined as unknown as string,
    })

    expect(normalized.theme).toBe('crowbar')
  })

  it('coerces an unknown theme on write too', () => {
    expect(normalizeSettingValue('theme', 'terra')).toBe('crowbar')
    expect(normalizeSettingValue('theme', 'zen')).toBe('zen')
  })
})
