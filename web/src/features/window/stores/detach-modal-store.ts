import { create } from 'zustand'

/** The placeholder whose holder the user is about to detach. */
export interface DetachTarget {
  wsId: string
  branch: string
  heldByPath: string
}

interface DetachModalState {
  target: DetachTarget | null
  open: (target: DetachTarget) => void
  close: () => void
}

// Global UI store (features/window/stores): both the placeholder row's Detach…
// button and the placeholder toast's Fix… action open the single detach modal,
// which is rendered once at the shell level and reads this target.
export const useDetachModalStore = create<DetachModalState>()((set) => ({
  target: null,
  open: (target) => set({ target }),
  close: () => set({ target: null }),
}))
