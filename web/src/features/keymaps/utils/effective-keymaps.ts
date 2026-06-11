/**
 * Effective keymap resolution.
 *
 * The effective binding for each command is computed by merging, in order of
 * increasing precedence:
 *   1. command default chord (from the registry)
 *   2. active preset overrides
 *   3. user overrides
 *
 * All chords are normalized so conflict detection compares canonical forms.
 */
import { COMMANDS } from '@/features/keymaps/registry'
import { getPreset } from '@/features/keymaps/defaults/keybinding-presets'
import { normalizeChord } from '@/features/keymaps/utils/chord'
import type {
  Command,
  EffectiveBinding,
  KeymapConflict,
  KeymapOverrides,
  KeymapPresetId,
} from '@/features/keymaps/types'

export interface ResolveOptions {
  preset: KeymapPresetId
  overrides: KeymapOverrides
  commands?: Command[]
}

/** Resolve the effective binding for a single command. */
export function resolveBinding(command: Command, opts: ResolveOptions): EffectiveBinding {
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

/** Resolve every command into an effective binding. */
export function getEffectiveBindings(opts: ResolveOptions): EffectiveBinding[] {
  const commands = opts.commands ?? COMMANDS
  return commands.map((command) => resolveBinding(command, opts))
}

/** Map of commandId -> effective chord. */
export function getEffectiveChordMap(opts: ResolveOptions): Record<string, string> {
  const map: Record<string, string> = {}
  for (const binding of getEffectiveBindings(opts)) {
    map[binding.commandId] = binding.chord
  }
  return map
}

/**
 * Detect conflicts: chords bound to more than one command. Only non-empty
 * chords are considered.
 */
export function detectConflicts(opts: ResolveOptions): KeymapConflict[] {
  const byChord = new Map<string, string[]>()
  for (const binding of getEffectiveBindings(opts)) {
    if (!binding.chord) continue
    const list = byChord.get(binding.chord) ?? []
    list.push(binding.commandId)
    byChord.set(binding.chord, list)
  }
  const conflicts: KeymapConflict[] = []
  for (const [chord, commandIds] of byChord) {
    if (commandIds.length > 1) conflicts.push({ chord, commandIds })
  }
  return conflicts
}

/**
 * Command ids that would conflict if `chord` were assigned to `commandId`,
 * given the current options (excluding commandId itself).
 */
export function findConflictingCommands(
  commandId: string,
  chord: string,
  opts: ResolveOptions,
): string[] {
  const normalized = normalizeChord(chord)
  if (!normalized) return []
  return getEffectiveBindings(opts)
    .filter((b) => b.commandId !== commandId && b.chord === normalized)
    .map((b) => b.commandId)
}
