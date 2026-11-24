import { beforeEach, describe, expect, it } from 'vitest'
import { followShellSettings } from '@/entries/follow-shell-settings'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'
import { useKeymapStore } from '@/features/keymaps/stores/store'
import { useSettingsStore } from '@/features/settings/store'
import type { SettingValues } from '@/sdk/setting-values'

function replay(frames: SettingValues[], listener: (values: SettingValues) => void) {
  frames.forEach(listener)
  return () => {}
}

function shellThatSends(...frames: SettingValues[]) {
  return {
    setSetting: () => {},
    onSettings: (listener: (values: SettingValues) => void) => replay(frames, listener),
  }
}

const defaults = getDefaultSettingsSnapshot()

describe('followShellSettings', () => {
  beforeEach(() => {
    useSettingsStore.setState({ settings: getDefaultSettingsSnapshot() })
  })

  it.each([
    [
      'makes the shell values the pane settings, so an editor option reaches the pane',
      [{ wordWrap: true, tabSize: 4, maxOpenTabs: 7 }],
      [true, 4, 7],
    ],
    [
      'takes the whole set each time: a reset key returns to its default',
      [{ wordWrap: true, tabSize: 4 }, {}],
      [defaults.wordWrap, defaults.tabSize, defaults.maxOpenTabs],
    ],
  ])('%s', (_name, frames, expected) => {
    followShellSettings(shellThatSends(...frames))
    const { settings } = useSettingsStore.getState()
    expect([settings.wordWrap, settings.tabSize, settings.maxOpenTabs]).toEqual(expected)
  })

  it('sends the pane writes to the shell and keeps no copy of them', () => {
    const written: unknown[][] = []
    const stop = followShellSettings({
      ...shellThatSends(),
      setSetting: (key, value) => written.push([key, value]),
    })
    void useSettingsStore.getState().updateSetting('renderIndentGuides', false)
    void useSettingsStore.getState().updateSetting('coreFeatures', { breadcrumbs: false })
    expect(written).toEqual([['renderIndentGuides', false]])
    const { settings } = useSettingsStore.getState()
    expect([settings.renderIndentGuides, settings.coreFeatures.breadcrumbs]).toEqual([true, false])
    stop()
    void useSettingsStore.getState().updateSetting('renderIndentGuides', false)
    expect(useSettingsStore.getState().settings.renderIndentGuides).toBe(false)
  })

  it('keeps the font variables in step with the shell', () => {
    followShellSettings(shellThatSends({ uiFontSize: 20 }))
    expect(document.documentElement.style.getPropertyValue('--app-ui-font-size')).toBe('20px')
  })

  it.each([
    [
      'takes the keybinding overrides the shell owns',
      [{ keybindingUserOverrides: { 'agent.zoomIn': 'mod+k' } }],
      { 'agent.zoomIn': 'mod+k' },
    ],
    [
      'drops them when the shell sends none',
      [{ keybindingUserOverrides: { 'agent.zoomIn': 'mod+k' } }, {}],
      {},
    ],
  ])('%s', (_name, frames, expected) => {
    followShellSettings(shellThatSends(...frames))
    expect(useKeymapStore.getState().overrides).toEqual(expected)
  })
})
