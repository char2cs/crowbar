import { create } from 'zustand'

interface DaemonHaltState {
  message: string | null
  halt: (message: string) => void
  clear: () => void
}

export const useDaemonHaltStore = create<DaemonHaltState>()((set) => ({
  message: null,
  halt: (message) => set({ message }),
  clear: () => set({ message: null }),
}))
