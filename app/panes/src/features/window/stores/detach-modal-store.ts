import { create } from 'zustand'

interface DetachTarget {
  wsId: string
  branch: string
  heldByPath: string
}

interface DetachModalState {
  target: DetachTarget | null
  open: (target: DetachTarget) => void
  close: () => void
}

export const useDetachModalStore = create<DetachModalState>()((set) => ({
  target: null,
  open: (target) => set({ target }),
  close: () => set({ target: null }),
}))
