// Stub
import type { Command, Keybinding } from "@/features/keymaps/types"

export const commandRegistry = {
  register: (_cmd: unknown) => {},
  unregister: (_id: string) => {},
  execute: async (_id: string) => {},
  executeCommand: async (_id: string, ..._args: unknown[]) => {},
  getAll: () => [] as Command[],
  getAllCommands: () => [] as Command[],
  getAllKeybindings: () => [] as Keybinding[],
  getKeybinding: (_commandId: string): Keybinding | undefined => undefined,
  get: (_id: string) => null as Command | null,
}

/** Athas alias for commandRegistry */
export const keymapRegistry = commandRegistry
