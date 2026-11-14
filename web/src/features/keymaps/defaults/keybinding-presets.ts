// `default` adds no overrides (registry chords). `compact` is a populated alternate that moves
// Reopen closed tab, proving a preset switch re-resolves live bindings.
import { TAB_REOPEN_CLOSED } from '@/features/keymaps/registry'
import type { KeymapPreset, KeymapPresetId } from '@/features/keymaps/types'

const KEYMAP_PRESETS: Record<KeymapPresetId, KeymapPreset> = {
  default: {
    id: 'default',
    label: 'Default',
    populated: true,
    bindings: {},
  },
  compact: {
    id: 'compact',
    label: 'Compact',
    populated: true,
    bindings: {
      [TAB_REOPEN_CLOSED]: 'mod+shift+r',
    },
  },
}

export const KEYMAP_PRESET_OPTIONS: Array<{ value: KeymapPresetId; label: string }> = [
  { value: 'default', label: 'Default' },
  { value: 'compact', label: 'Compact (alternate)' },
]

export function getPreset(id: KeymapPresetId): KeymapPreset {
  return KEYMAP_PRESETS[id] ?? KEYMAP_PRESETS.default
}

export function isKeymapPresetId(value: unknown): value is KeymapPresetId {
  return value === 'default' || value === 'compact'
}
