/**
 * The Color Theme picker must never display a value that is absent from its own
 * menu. `Theme` is a bare `string`, so a persisted id can name a theme that no
 * longer exists ('terra' was renamed to 'zen'); the picker's "ensure the current
 * theme appears" fallback bails out for an id the registry doesn't know, leaving
 * the trigger showing a dead id while the app really paints Crowbar.
 *
 * The guarantee is upheld at the store's normalisation seam (a read-time
 * fallback, never a migration), so this drives the REAL `updateSetting` path.
 *
 * The menu items live in a portal that is only mounted while the popup is open,
 * so base-ui's Select.Value has no label to resolve and renders the raw value —
 * which is exactly the string this asserts against the registry.
 */
import { act, cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { AppearanceSettings } from '@/features/settings/components/tabs/appearance-settings'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'
import { useSettingsStore } from '@/features/settings/store'
import { themeRegistry } from '@/extensions/themes/theme-registry'

function colorThemeTriggerValue(): string {
  const trigger = document.querySelectorAll('[data-slot="select-trigger"]')[0]
  return trigger?.textContent?.trim() ?? ''
}

describe('AppearanceSettings color theme picker', () => {
  beforeEach(() => {
    act(() => {
      useSettingsStore.setState({ settings: getDefaultSettingsSnapshot() })
    })
  })

  afterEach(() => {
    cleanup()
  })

  it('never displays a theme that is absent from its own menu', async () => {
    await act(async () => {
      await useSettingsStore.getState().updateSetting('theme', 'terra')
    })

    render(<AppearanceSettings />)

    const shown = colorThemeTriggerValue()
    expect(shown).not.toBe('terra')
    expect(
      themeRegistry.getTheme(shown),
      `Color Theme shows "${shown}", which is in no menu option`,
    ).toBeTruthy()
  })

  it('leaves a live theme selected', async () => {
    await act(async () => {
      await useSettingsStore.getState().updateSetting('theme', 'zen')
    })

    render(<AppearanceSettings />)

    expect(colorThemeTriggerValue()).toBe('zen')
  })
})
