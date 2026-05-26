// Stub
import { create } from "zustand"
import { createSelectors } from "@/utils/zustand-selectors"
import type { Keybinding, KeybindingPreset } from "@/features/keymaps/types"

interface KeymapStoreState {
  keybindings: Keybinding[]
  preset: KeybindingPreset
  contexts: Record<string, unknown>
  setPreset: (preset: KeybindingPreset) => void
  executeCommand: (commandId: string) => Promise<void>
  actions: {
    setPreset: (preset: KeybindingPreset) => void
    resetToDefaults: () => void
    importKeybindings: (bindings: unknown[]) => void
    addKeybinding: (binding: unknown) => void
    removeKeybinding: (commandId: string) => void
    updateKeybinding: (commandId: string, key: string) => void
  }
}

const useKeymapStoreBase = create<KeymapStoreState>((set) => ({
  keybindings: [],
  preset: "default",
  contexts: {},
  setPreset: (preset) => set({ preset }),
  executeCommand: async () => {},
  actions: {
    setPreset: (preset) => set({ preset }),
    resetToDefaults: () => {},
    importKeybindings: () => {},
    addKeybinding: () => {},
    removeKeybinding: () => {},
    updateKeybinding: () => {},
  },
}))

export const useKeymapStore = createSelectors(useKeymapStoreBase)
