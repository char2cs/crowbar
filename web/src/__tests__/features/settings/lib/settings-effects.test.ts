import { beforeEach, describe, expect, it, vi } from 'vitest'

const bridge = vi.hoisted(() => ({
  setMacOSWindowAppearance: vi.fn(async (_themeType: string, _transparency: boolean) => {}),
  setWindowTransparency: vi.fn(async (_enabled: boolean) => {}),
}))

vi.mock('@/lib/crowbar-bridge', () => ({
  setMacOSWindowAppearance: bridge.setMacOSWindowAppearance,
  setWindowTransparency: bridge.setWindowTransparency,
}))

import { applyTheme } from '@/features/settings/lib/settings-effects'

describe('applyTheme with a persisted theme id the registry no longer knows', () => {
  beforeEach(() => {
    bridge.setMacOSWindowAppearance.mockClear()
    // 'terra' is the only alternative theme this project ever shipped; the branch
    // renamed it to 'zen' and deleted its stylesheet, so a real install can still
    // hold it. Theme Mode = Light has already toggled .dark off by this point.
    document.documentElement.classList.remove('dark')
    localStorage.clear()
  })

  it('leaves light mode alone instead of forcing .dark', async () => {
    await applyTheme('terra')

    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })

  it('reports a light macOS window appearance', async () => {
    await applyTheme('terra')

    expect(bridge.setMacOSWindowAppearance).toHaveBeenCalled()
    expect(bridge.setMacOSWindowAppearance.mock.calls.at(-1)?.[0]).toBe('light')
  })

  it('caches the resolved theme (not the dead id) for the next launch', async () => {
    await applyTheme('terra')

    const cached = JSON.parse(localStorage.getItem('crowbar.bootstrap.appearance.v2') ?? '{}')
    expect(cached.themeId).toBe('crowbar')
    expect(cached.themeType).toBe('light')
  })

  it('still tracks dark mode for a known theme', async () => {
    document.documentElement.classList.add('dark')

    await applyTheme('zen')

    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(bridge.setMacOSWindowAppearance.mock.calls.at(-1)?.[0]).toBe('dark')
  })
})
