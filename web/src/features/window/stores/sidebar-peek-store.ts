import { create } from 'zustand'

interface SidebarPeekState {
  /** The peek card occupies the screen: summoned, or still sliding out. Owned by SidebarPeek. */
  occupied: boolean
  setOccupied: (occupied: boolean) => void
}

export const useSidebarPeekStore = create<SidebarPeekState>()((set) => ({
  occupied: false,
  setOccupied: (occupied) => set({ occupied }),
}))
