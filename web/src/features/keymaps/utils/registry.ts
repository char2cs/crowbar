/**
 * Legacy command-registry shim.
 *
 * The real, typed command registry now lives in `@/features/keymaps/registry`.
 * This module remains only to satisfy the command palette's optional
 * `keymapRegistry.getKeybinding(id)?.key` lookup, which currently has no live
 * keybinding to surface there. It is intentionally a no-op shim.
 */
import type { Command } from '@/features/keymaps/types'

interface LegacyKeybinding {
  command: string
  key: string
}

export const commandRegistry = {
  register: (_cmd: unknown) => {},
  unregister: (_id: string) => {},
  execute: async (_id: string) => {},
  executeCommand: async (_id: string, ..._args: unknown[]) => {},
  getAll: () => [] as Command[],
  getAllCommands: () => [] as Command[],
  getAllKeybindings: () => [] as LegacyKeybinding[],
  getKeybinding: (_commandId: string): LegacyKeybinding | undefined => undefined,
  get: (_id: string) => null as Command | null,
}

/** Athas alias for commandRegistry */
export const keymapRegistry = commandRegistry
