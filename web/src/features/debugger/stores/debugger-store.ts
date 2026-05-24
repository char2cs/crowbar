// Stub
import { create } from "zustand"
import { createSelectors } from "@/utils/zustand-selectors"

export interface Breakpoint {
  filePath: string
  line: number
  enabled: boolean
  id?: string
}

interface DebuggerState {
  isActive: boolean
  breakpoints: Breakpoint[]
  actions: {
    toggleBreakpoint: (filePath: string, line: number) => void
    addBreakpoint: (filePath: string, line: number) => void
    removeBreakpoint: (filePath: string, line: number) => void
    clearBreakpoints: () => void
  }
}

const useDebuggerStoreBase = create<DebuggerState>((set, get) => ({
  isActive: false,
  breakpoints: [],
  actions: {
    toggleBreakpoint: (filePath, line) => {
      const bps = get().breakpoints
      const existing = bps.findIndex(b => b.filePath === filePath && b.line === line)
      if (existing >= 0) {
        set({ breakpoints: bps.filter((_, i) => i !== existing) })
      } else {
        set({ breakpoints: [...bps, { filePath, line, enabled: true }] })
      }
    },
    addBreakpoint: (filePath, line) => {
      set({ breakpoints: [...get().breakpoints, { filePath, line, enabled: true }] })
    },
    removeBreakpoint: (filePath, line) => {
      set({ breakpoints: get().breakpoints.filter(b => !(b.filePath === filePath && b.line === line)) })
    },
    clearBreakpoints: () => set({ breakpoints: [] }),
  },
}))
export const useDebuggerStore = createSelectors(useDebuggerStoreBase)
