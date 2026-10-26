import { defaultSettings, getDefaultSetting } from '@/features/settings/config/default-settings'
import { applySettingSideEffect } from '@/features/settings/lib/settings-effects'
import { normalizeSettingValue } from '@/features/settings/lib/settings-normalization'
import { useSettingsStore } from '@/features/settings/store'
import type { Settings } from '@/features/settings/types/settings'

const PREFIX = 'crowbar:settings:'

function parse(raw: string | null): unknown {
  if (raw === null) return null
  try {
    return JSON.parse(raw)
  } catch {
    return null
  }
}

/** Keeps this webview's settings in step with another's (the phone's settings page shares
 *  localStorage): a setting saved there is taken here, never saved back. Returns the teardown. */
export function startSettingsStorageSync(win: Window = window): () => void {
  const onStorage = (event: StorageEvent) => {
    if (!event.key?.startsWith(PREFIX)) return
    const key = event.key.slice(PREFIX.length) as keyof Settings
    if (!(key in defaultSettings)) return
    const stored = parse(event.newValue)
    const fallback = getDefaultSetting(key)
    const merged =
      stored === null
        ? fallback
        : typeof fallback === 'object' && fallback !== null && !Array.isArray(fallback)
          ? { ...fallback, ...(stored as object) }
          : stored
    const value = normalizeSettingValue(key, merged as Settings[typeof key])
    useSettingsStore.setState((s) => {
      ;(s.settings as Record<keyof Settings, unknown>)[key] = value
    })
    applySettingSideEffect(key, value, () => useSettingsStore.getState().settings)
  }
  win.addEventListener('storage', onStorage)
  return () => win.removeEventListener('storage', onStorage)
}
