import { create } from 'zustand'

interface SidebarPeekState {
  occupied: boolean
  setOccupied: (occupied: boolean) => void
}

export const useSidebarPeekStore = create<SidebarPeekState>()((set) => ({
  occupied: false,
  setOccupied: (occupied) => set({ occupied }),
}))
