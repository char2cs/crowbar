// Stub
import { create } from "zustand"
import { createSelectors } from "@/utils/zustand-selectors"

export type VimMode = "normal" | "insert" | "visual" | "command" | "replace"

interface VimState {
  mode: string
  isEnabled: boolean
  visualSelection: null | { start: { line: number; character: number }; end: { line: number; character: number } }
  actions: {
    enable: () => void
    disable: () => void
    setMode: (mode: string) => void
  }
}

const useVimStoreBase = create<VimState>((set) => ({
  mode: "normal",
  isEnabled: false,
  visualSelection: null,
  actions: {
    enable: () => set({ isEnabled: true }),
    disable: () => set({ isEnabled: false }),
    setMode: (mode) => set({ mode }),
  },
}))
export const useVimStore = createSelectors(useVimStoreBase)
