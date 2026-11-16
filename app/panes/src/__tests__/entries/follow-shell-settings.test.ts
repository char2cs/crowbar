import { beforeEach, describe, expect, it } from 'vitest'
import { followShellSettings } from '@/entries/follow-shell-settings'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'
import { useSettingsStore } from '@/features/settings/store'
import type { SettingValues } from '@/sdk/setting-values'

function shellThatSends(...frames: SettingValues[]) {
  return {
    setSetting: () => {},
    onSettings: (listener: (values: SettingValues) => void) => {
      frames.forEach(listener)
      return () => {}
    },
  }
}

describe('followShellSettings', () => {
  beforeEach(() => {
    useSettingsStore.setState({ settings: getDefaultSettingsSnapshot() })
  })

  it('makes the shell values the pane settings, so an editor option reaches the pane', () => {
    followShellSettings(shellThatSends({ wordWrap: true, tabSize: 4, maxOpenTabs: 7 }))
    const { settings } = useSettingsStore.getState()
    expect([settings.wordWrap, settings.tabSize, settings.maxOpenTabs]).toEqual([true, 4, 7])
  })

  it('takes the whole set each time: a reset key returns to its default', () => {
    followShellSettings(shellThatSends({ wordWrap: true }, {}))
    expect(useSettingsStore.getState().settings.wordWrap).toBe(false)
  })

  it('sends the pane writes to the shell and keeps no copy of them', () => {
    const written: unknown[][] = []
    const stop = followShellSettings({
      ...shellThatSends(),
      setSetting: (key, value) => written.push([key, value]),
    })
    void useSettingsStore.getState().updateSetting('showMinimap', true)
    void useSettingsStore.getState().updateSetting('coreFeatures', { breadcrumbs: false })
    expect(written).toEqual([['showMinimap', true]])
    const { settings } = useSettingsStore.getState()
    expect([settings.showMinimap, settings.coreFeatures.breadcrumbs]).toEqual([false, false])
    stop()
    void useSettingsStore.getState().updateSetting('showMinimap', true)
    expect(useSettingsStore.getState().settings.showMinimap).toBe(true)
  })

  it('keeps the font variables in step with the shell', () => {
    followShellSettings(shellThatSends({ uiFontSize: 20 }))
    expect(document.documentElement.style.getPropertyValue('--app-ui-font-size')).toBe('20px')
  })
})
