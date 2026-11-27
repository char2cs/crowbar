import { COMMANDS } from '@/features/keymaps/registry'
import { getPreset } from '@/features/keymaps/defaults/keybinding-presets'
import { normalizeChord } from '@/features/keymaps/utils/chord'
import type {
  Command,
  EffectiveBinding,
  KeymapOverrides,
  KeymapPresetId,
} from '@/features/keymaps/types'

export interface ResolveOptions {
  preset: KeymapPresetId
  overrides: KeymapOverrides
  commands?: Command[]
}

function resolveBinding(command: Command, opts: ResolveOptions): EffectiveBinding {
  const userChord = opts.overrides[command.id]
  if (userChord != null) {
    return { commandId: command.id, chord: normalizeChord(userChord), source: 'user' }
  }
  const presetChord = getPreset(opts.preset).bindings[command.id]
  if (presetChord != null) {
    return { commandId: command.id, chord: normalizeChord(presetChord), source: 'preset' }
  }
  return { commandId: command.id, chord: normalizeChord(command.defaultChord), source: 'default' }
}

export function getEffectiveBindings(opts: ResolveOptions): EffectiveBinding[] {
  const commands = opts.commands ?? COMMANDS
  return commands.map((command) => resolveBinding(command, opts))
}

export function getEffectiveChordMap(opts: ResolveOptions): Record<string, string> {
  const map: Record<string, string> = {}
  for (const binding of getEffectiveBindings(opts)) {
    map[binding.commandId] = binding.chord
  }
  return map
}

export function findConflictingCommands(
  commandId: string,
  chord: string,
  opts: ResolveOptions,
): string[] {
  const normalized = normalizeChord(chord)
  if (!normalized) return []
  // react-doctor-disable-next-line js-combine-iterations -- bindings are bounded by the same small, fixed COMMANDS registry (registry.ts) merged with user overrides; called on-demand from the keybinding-editor UI, not a render hot path.
  return getEffectiveBindings(opts)
    .filter((b) => b.commandId !== commandId && b.chord === normalized)
    .map((b) => b.commandId)
}
