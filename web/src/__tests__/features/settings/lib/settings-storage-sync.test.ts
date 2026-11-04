import { afterEach, describe, expect, it } from 'vitest'
import { startSettingsStorageSync } from '@/features/settings/lib/settings-storage-sync'
import { getDefaultSetting, useSettingsStore } from '@/features/settings/store'

// Another webview (the phone's settings page) wrote a setting to the shared localStorage.
const written = (key: string, value: unknown) =>
  window.dispatchEvent(
    new StorageEvent('storage', {
      key,
      newValue: value === null ? null : JSON.stringify(value),
    }),
  )

let stop: (() => void) | undefined
afterEach(() => {
  stop?.()
  useSettingsStore.setState((s) => {
    s.settings.compactGitStatusBadges = getDefaultSetting('compactGitStatusBadges')
  })
})

describe('startSettingsStorageSync', () => {
  it('takes a setting another webview saved, and its default once removed there', () => {
    stop = startSettingsStorageSync()
    const flipped = !getDefaultSetting('compactGitStatusBadges')
    written('crowbar:settings:compactGitStatusBadges', flipped)
    expect(useSettingsStore.getState().settings.compactGitStatusBadges).toBe(flipped)

    written('crowbar:settings:compactGitStatusBadges', null)
    expect(useSettingsStore.getState().settings.compactGitStatusBadges).toBe(
      getDefaultSetting('compactGitStatusBadges'),
    )
  })

  it('ignores keys that are not settings, and stops when stopped', () => {
    stop = startSettingsStorageSync()
    written('crowbar:settings:notASetting', true)
    written('crowbar.hosts', { hosts: [] })
    stop()
    written('crowbar:settings:compactGitStatusBadges', !getDefaultSetting('compactGitStatusBadges'))
    expect(useSettingsStore.getState().settings.compactGitStatusBadges).toBe(
      getDefaultSetting('compactGitStatusBadges'),
    )
  })
})
