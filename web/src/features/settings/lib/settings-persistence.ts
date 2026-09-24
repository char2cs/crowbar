import {
  defaultSettings,
  getDefaultSettingsSnapshot,
} from '@/features/settings/config/default-settings'
import type { Settings } from '@/features/settings/types/settings'

/**
 * The one persistence path for settings: one localStorage key per setting,
 * namespaced under "crowbar:settings:". Keys absent from storage take their
 * default; object-valued settings merge stored fields over the defaults so a
 * newly added field gets its default.
 */
const STORAGE_PREFIX = 'crowbar:settings:'

function readKey(key: string): unknown {
  try {
    const raw = localStorage.getItem(STORAGE_PREFIX + key)
    return raw === null ? null : JSON.parse(raw)
  } catch {
    return null
  }
}

export function loadPersistedSettings(): Settings {
  const loaded = getDefaultSettingsSnapshot()
  for (const key of Object.keys(defaultSettings) as Array<keyof Settings>) {
    const stored = readKey(key)
    if (stored === null || stored === undefined) continue
    const fallback = defaultSettings[key]
    const value =
      typeof fallback === 'object' && fallback !== null && !Array.isArray(fallback)
        ? { ...fallback, ...(stored as object) }
        : stored
    ;(loaded as Record<keyof Settings, unknown>)[key] = value
  }
  return loaded
}

export function persistSettings(settings: Partial<Settings>): void {
  for (const [key, value] of Object.entries(settings)) {
    try {
      localStorage.setItem(STORAGE_PREFIX + key, JSON.stringify(value))
    } catch (error) {
      console.warn('Failed to persist setting:', key, error)
    }
  }
}

let saveTimeout: ReturnType<typeof setTimeout> | null = null
let pendingSettings: Partial<Settings> = {}

/** Coalesce rapid single-setting edits (a slider, a text field) into one write. */
export function persistSettingsDebounced(settings: Partial<Settings>): void {
  pendingSettings = { ...pendingSettings, ...settings }
  if (saveTimeout) clearTimeout(saveTimeout)
  saveTimeout = setTimeout(() => {
    const toSave = pendingSettings
    pendingSettings = {}
    saveTimeout = null
    persistSettings(toSave)
  }, 300)
}
