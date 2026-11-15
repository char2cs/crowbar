import { beforeEach, describe, expect, it, vi } from 'vitest'

const bridge = vi.hoisted(() => ({
  setMacOSWindowAppearance: vi.fn(async (_themeType: string) => {}),
}))

vi.mock('@/lib/crowbar-bridge', () => ({
  setMacOSWindowAppearance: bridge.setMacOSWindowAppearance,
}))

import { applySettingSideEffect } from '@/features/settings/lib/settings-effects'
import { defaultSettings } from '@/features/settings/config/default-settings'
import type { Settings } from '@/features/settings/types/settings'

// Applying a theme is fire-and-forget behind the settings side effect, so a test waits on its result.
function applyTheme(theme: Settings['theme'], themeMode: Settings['themeMode'] = 'light'): void {
  const settings: Settings = { ...defaultSettings, theme, themeMode }
  applySettingSideEffect('theme', theme, () => settings)
}

describe('applyTheme with a persisted theme id the registry no longer knows', () => {
  beforeEach(() => {
    bridge.setMacOSWindowAppearance.mockClear()
    // A real install can still hold a removed theme id ('terra', 'zen').
    // Theme Mode = Light has already toggled .dark off by this point.
    document.documentElement.classList.remove('dark')
    localStorage.clear()
  })

  it('leaves light mode alone instead of forcing .dark', async () => {
    applyTheme('terra')

    await vi.waitFor(() => expect(bridge.setMacOSWindowAppearance).toHaveBeenCalled())
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })

  it('reports a light macOS window appearance', async () => {
    applyTheme('terra')

    await vi.waitFor(() => expect(bridge.setMacOSWindowAppearance).toHaveBeenCalled())
    expect(bridge.setMacOSWindowAppearance.mock.calls.at(-1)?.[0]).toBe('light')
  })

  it('caches the resolved theme (not the dead id) for the next launch', async () => {
    applyTheme('terra')

    await vi.waitFor(() => expect(bridge.setMacOSWindowAppearance).toHaveBeenCalled())
    const cached = JSON.parse(localStorage.getItem('crowbar.bootstrap.appearance.v2') ?? '{}')
    expect(cached.themeId).toBe('crowbar')
    expect(cached.themeType).toBe('light')
  })

  it('resolves the removed zen theme to the default theme', async () => {
    applyTheme('zen')

    await vi.waitFor(() => expect(bridge.setMacOSWindowAppearance).toHaveBeenCalled())
    const cached = JSON.parse(localStorage.getItem('crowbar.bootstrap.appearance.v2') ?? '{}')
    expect(cached.themeId).toBe('crowbar')
  })

  it('still tracks dark mode for a known theme', async () => {
    applyTheme('frosted', 'dark')

    await vi.waitFor(() => expect(bridge.setMacOSWindowAppearance).toHaveBeenCalled())
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(bridge.setMacOSWindowAppearance.mock.calls.at(-1)?.[0]).toBe('dark')
  })
})
