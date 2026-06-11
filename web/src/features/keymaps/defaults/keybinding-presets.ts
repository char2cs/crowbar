/**
 * Keybinding presets.
 *
 * `default` is the baseline — it adds no overrides, so every command uses its
 * registry default chord.
 *
 * `compact` is a real, populated alternate preset (proof that switching the
 * preset re-resolves live bindings). It remaps the editor-split commands onto
 * single-modifier chords so they don't collide with `\`.
 *
 * Additional IDE presets (vscode/jetbrains/sublime/xcode/atom/emacs/zed) are
 * intentionally NOT authored here — see SCOPE in the settings tab.
 */
import { PANE_SPLIT_DOWN, PANE_SPLIT_RIGHT, TAB_REOPEN_CLOSED } from '@/features/keymaps/registry'
import type { KeymapPreset, KeymapPresetId } from '@/features/keymaps/types'

export const KEYMAP_PRESETS: Record<KeymapPresetId, KeymapPreset> = {
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
      // Remap splits onto bracket keys, free up backslash.
      [PANE_SPLIT_RIGHT]: 'mod+]',
      [PANE_SPLIT_DOWN]: 'mod+shift+]',
      // Reopen closed tab on a single modifier.
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

/**
 * Validates the legacy `settings.keybindingPreset` value (a separate, dead
 * setting whose allowed values are the IDE preset names). Kept here so
 * settings normalization continues to validate that field.
 */
const LEGACY_SETTINGS_PRESETS = new Set([
  'none',
  'vscode',
  'jetbrains',
  'sublime',
  'xcode',
  'atom',
  'emacs',
  'zed',
])

export function isKeybindingPreset(value: unknown): boolean {
  return typeof value === 'string' && LEGACY_SETTINGS_PRESETS.has(value)
}
