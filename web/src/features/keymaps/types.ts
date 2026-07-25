/**
 * Keymap domain types.
 *
 * A "chord" is a single keystroke combination (modifiers + a key), stored in a
 * normalized, platform-neutral string form such as `mod+\`, `mod+shift+t`,
 * `mod+alt+arrowleft`. `mod` resolves to Cmd on macOS and Ctrl elsewhere.
 */

/** Categories used to group commands in the Keybindings settings tab. */
export type CommandCategory = 'Panes' | 'Tabs' | 'Editor' | 'Navigation' | 'Chats'

/**
 * A command is a finite, knowable action that can be bound to a key chord.
 * Commands are extracted from the keyboard hooks that already exist in the
 * codebase — this is not an open-ended command system.
 */
export interface Command {
  /** Stable identifier, e.g. `panes.splitRight`. */
  id: string
  /** Human-readable label shown in the settings tab. */
  label: string
  category: CommandCategory
  /** Default chord in normalized string form. */
  defaultChord: string
  /**
   * Whether the live binding is resolved from the registry (live-editable) or
   * still hardcoded in a hook and only represented here for display.
   */
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
