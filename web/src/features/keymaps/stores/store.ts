/**
 * Keymap store.
 *
 * Owns the active preset and per-command user overrides, both persisted to
 * localStorage under the existing `crowbar:settings:` namespace:
 *   - `crowbar:settings:keybindingPreset`         → KeymapPresetId
 *   - `crowbar:settings:keybindingUserOverrides`  → Record<commandId, chord>
 *
 * The store is the single source of truth that the keyboard hooks and the
 * Keybindings settings tab both read from.
 */
import { create } from 'zustand'
import { createSelectors } from '@/utils/zustand-selectors'
import { isKeymapPresetId } from '@/features/keymaps/defaults/keybinding-presets'
import { normalizeChord } from '@/features/keymaps/utils/chord'
import type { KeymapOverrides, KeymapPresetId } from '@/features/keymaps/types'

const STORAGE_PREFIX = 'crowbar:settings:'
const PRESET_KEY = STORAGE_PREFIX + 'keybindingPreset'
const OVERRIDES_KEY = STORAGE_PREFIX + 'keybindingUserOverrides'

function readPreset(): KeymapPresetId {
  try {
    const raw = localStorage.getItem(PRESET_KEY)
    if (raw === null) return 'default'
    const parsed = JSON.parse(raw)
    return isKeymapPresetId(parsed) ? parsed : 'default'
  } catch {
    return 'default'
  }
}

function readOverrides(): KeymapOverrides {
  try {
    const raw = localStorage.getItem(OVERRIDES_KEY)
    if (raw === null) return {}
    const parsed = JSON.parse(raw)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as KeymapOverrides
    }
    return {}
  } catch {
    return {}
  }
}

function persist(key: string, value: unknown): void {
  try {
    localStorage.setItem(key, JSON.stringify(value))
  } catch (error) {
    console.warn('Failed to persist keymap state:', key, error)
  }
}

interface KeymapStoreState {
  preset: KeymapPresetId
  overrides: KeymapOverrides
  actions: {
    setPreset: (preset: KeymapPresetId) => void
    /** Set (or replace) a user override for a command. */
    setOverride: (commandId: string, chord: string) => void
    /** Remove a user override, falling back to preset/default. */
    clearOverride: (commandId: string) => void
    /** Clear every user override. */
    resetOverrides: () => void
  }
}

const useKeymapStoreBase = create<KeymapStoreState>((set) => ({
  preset: readPreset(),
  overrides: readOverrides(),
  actions: {
    setPreset: (preset) => {
      persist(PRESET_KEY, preset)
      set({ preset })
    },
    setOverride: (commandId, chord) => {
      set((state) => {
        const next = { ...state.overrides, [commandId]: normalizeChord(chord) }
        persist(OVERRIDES_KEY, next)
        return { overrides: next }
      })
    },
    clearOverride: (commandId) => {
      set((state) => {
        if (!(commandId in state.overrides)) return state
        const next = { ...state.overrides }
        delete next[commandId]
        persist(OVERRIDES_KEY, next)
        return { overrides: next }
      })
    },
    resetOverrides: () => {
      persist(OVERRIDES_KEY, {})
      set({ overrides: {} })
    },
  },
}))

export const useKeymapStore = createSelectors(useKeymapStoreBase)
