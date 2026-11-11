// A chord is one keystroke combination stored platform-neutral, e.g. `mod+shift+t`;
// `mod` resolves to Cmd on macOS and Ctrl elsewhere.

/** Categories used to group commands in the Keybindings settings tab. */
export type CommandCategory = 'Panes' | 'Tabs' | 'Editor' | 'Navigation' | 'Chats'

/** A bindable action from the fixed set the keyboard hooks implement. */
export interface Command {
  /** Stable identifier, e.g. `panes.navigateLeft`. */
  id: string
  /** Human-readable label shown in the settings tab. */
  label: string
  category: CommandCategory
  /** Default chord in normalized string form. */
  defaultChord: string
  /** True when the live binding resolves from the registry; false when a hook hardcodes it. */
  liveEditable: boolean
}

/** The set of selectable presets. Only some are "real" (populated). */
export type KeymapPresetId = 'default' | 'compact'

export interface KeymapPreset {
  id: KeymapPresetId
  label: string
  /** Whether this preset ships a real remap set. */
  populated: boolean
  /** commandId -> chord overrides applied on top of command defaults. */
  bindings: Record<string, string>
}

/** A user override: commandId -> chord (or null to mean "unbound"). */
export type KeymapOverrides = Record<string, string>

/** One resolved binding plus where it came from. */
export interface EffectiveBinding {
  commandId: string
  chord: string
  source: 'default' | 'preset' | 'user'
}
