import type { ReactNode } from 'react'
import { create } from 'zustand'

export interface SidebarScreen {
  id: string
  title: string
  component: ReactNode
}

interface SidebarNavStore {
  stack: SidebarScreen[]
  push: (screen: SidebarScreen) => void
  pop: () => void
  reset: () => void
}

export const useSidebarNavStore = create<SidebarNavStore>((set, get) => ({
  stack: [],
  push: (screen) => {
    if (get().stack.some((s) => s.id === screen.id)) return
    set((state) => ({ stack: [...state.stack, screen] }))
  },
  pop: () => {
    set((state) => ({ stack: state.stack.slice(0, -1) }))
  },
  reset: () => set({ stack: [] }),
}))
