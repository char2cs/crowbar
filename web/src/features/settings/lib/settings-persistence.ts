import {
  defaultSettings,
  getDefaultSettingsSnapshot,
} from '@/features/settings/config/default-settings'
import type { Settings } from '@/features/settings/types/settings'

type Store = {
  get: <T>(key: string) => Promise<T | null>
  set: (key: string, value: unknown) => Promise<void>
  save: () => Promise<void>
  onKeyChange: (key: string, cb: (value: unknown) => void) => () => void
}

// localStorage-backed store. Keys are namespaced under "crowbar:settings:" to
// avoid collisions with other things in the same origin.
const STORAGE_PREFIX = 'crowbar:settings:'

const load = async (_path: string, _opts?: unknown): Promise<Store> => {
  return {
    get: async <T>(key: string): Promise<T | null> => {
      try {
        const raw = localStorage.getItem(STORAGE_PREFIX + key)
        if (raw === null) return null
        return JSON.parse(raw) as T
      } catch {
        return null
      }
    },
    set: async (key: string, value: unknown): Promise<void> => {
      try {
        localStorage.setItem(STORAGE_PREFIX + key, JSON.stringify(value))
      } catch (error) {
        console.warn('Failed to persist setting:', key, error)
      }
    },
    // localStorage writes are synchronous — no explicit flush needed.
    save: async (): Promise<void> => {},
    onKeyChange: (key: string, cb: (value: unknown) => void) => {
      const handler = (e: StorageEvent) => {
        if (e.key === STORAGE_PREFIX + key) {
          try {
            cb(e.newValue !== null ? JSON.parse(e.newValue) : null)
          } catch {
            cb(null)
          }
        }
      }
      window.addEventListener('storage', handler)
      return () => window.removeEventListener('storage', handler)
    },
  }
}

let storeInstance: Store

async function initializeStoreDefaults(store: Store) {
  // Every key is independent (its own get -> maybe-set), so run them
  // concurrently instead of one setting at a time.
  await Promise.all(
    Object.entries(defaultSettings).map(async ([key, value]) => {
      const current = await store.get(key)
      if (current === null || current === undefined) {
        await store.set(key, value)
      } else if (typeof value === 'object' && value !== null && !Array.isArray(value)) {
        const merged = { ...value, ...(current as object) }
        await store.set(key, merged)
      }
    }),
  )
  await store.save()
}

export async function getSettingsStore() {
  if (!storeInstance) {
    storeInstance = await load('settings.json')
    await initializeStoreDefaults(storeInstance)
  }

  return storeInstance
}

export async function loadSettingsFromStore(): Promise<Settings> {
  const store = await getSettingsStore()
  const loadedSettings = getDefaultSettingsSnapshot()

  // Every key is read independently and written to its own distinct property
  // of `loadedSettings`, so run the reads concurrently.
  await Promise.all(
    (Object.keys(defaultSettings) as Array<keyof Settings>).map(async (key) => {
      const value = await store.get(key)
      if (value !== null && value !== undefined) {
        ;(loadedSettings as Record<keyof Settings, Settings[keyof Settings]>)[key] =
          value as Settings[typeof key]
      }
    }),
  )

  return loadedSettings
}

export async function saveSettingsToStore(settings: Partial<Settings>) {
  try {
    const store = await getSettingsStore()

    // Every key is written independently, so run them concurrently.
    await Promise.all(Object.entries(settings).map(([key, value]) => store.set(key, value)))

    await store.save()
  } catch (error) {
    console.error('Failed to save settings to store:', error)
  }
}

let saveTimeout: ReturnType<typeof setTimeout> | null = null
let pendingSettings: Partial<Settings> = {}

export function debouncedSaveSettingsToStore(settings: Partial<Settings>) {
  pendingSettings = { ...pendingSettings, ...settings }

  if (saveTimeout) {
    clearTimeout(saveTimeout)
  }

  saveTimeout = setTimeout(() => {
    const settingsToSave = pendingSettings
    pendingSettings = {}
    saveTimeout = null
    void saveSettingsToStore(settingsToSave)
  }, 300)
}
