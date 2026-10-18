import { create } from 'zustand'

interface DaemonHaltState {
  /** Why the daemon refused to open its home; null while it has not. */
  message: string | null
  halt: (message: string) => void
  clear: () => void
}

// Global UI state (features/window/stores): the root route renders the halt in
// place of every route, and the health listener is what learns of it.
export const useDaemonHaltStore = create<DaemonHaltState>()((set) => ({
  message: null,
  halt: (message) => set({ message }),
  clear: () => set({ message: null }),
}))
