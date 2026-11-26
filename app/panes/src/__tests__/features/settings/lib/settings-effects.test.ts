import { beforeEach, describe, expect, it, vi } from 'vitest'

import { applySettingSideEffect } from '@/features/settings/lib/settings-effects'
import { defaultSettings } from '@/features/settings/config/default-settings'
import type { Settings } from '@/features/settings/types/settings'

const BOOTSTRAP_KEY = 'crowbar.bootstrap.appearance.v2'

// Applying a theme is fire-and-forget behind the settings side effect: a test waits for the
// resolved theme to be cached for the next launch, its last step.
async function applyTheme(
  theme: Settings['theme'],
  themeMode: Settings['themeMode'] = 'light',
): Promise<{ themeId?: string; themeType?: string }> {
  const settings: Settings = { ...defaultSettings, theme, themeMode }
  applySettingSideEffect('theme', theme, () => settings)
  await vi.waitFor(() => expect(localStorage.getItem(BOOTSTRAP_KEY)).not.toBeNull())
  return JSON.parse(localStorage.getItem(BOOTSTRAP_KEY) ?? '{}')
}

describe('applyTheme with a persisted theme id the registry no longer knows', () => {
  beforeEach(() => {
    // A real install can still hold a removed theme id ('terra', 'zen').
    // Theme Mode = Light has already toggled .dark off by this point.
    document.documentElement.classList.remove('dark')
    localStorage.clear()
  })

  it('leaves light mode alone instead of forcing .dark', async () => {
    await applyTheme('terra')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })

  it('caches the resolved theme (not the dead id) for the next launch', async () => {
    const cached = await applyTheme('terra')
    expect(cached.themeId).toBe('crowbar')
    expect(cached.themeType).toBe('light')
  })

  it('resolves the removed zen theme to the default theme', async () => {
    const cached = await applyTheme('zen')
    expect(cached.themeId).toBe('crowbar')
  })

  it('still tracks dark mode for a known theme', async () => {
    const cached = await applyTheme('frosted', 'dark')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(cached.themeType).toBe('dark')
  })
})
