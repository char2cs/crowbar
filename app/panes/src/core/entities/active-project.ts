import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface ActiveProjectState {
  activeProjectId: string
  setActiveProject: (id: string) => void
}

/** The project the app last opened onto, kept across launches. */
export const useProjectStore = create<ActiveProjectState>()(
  persist(
    (set) => ({
      activeProjectId: '',
      setActiveProject: (id) => set({ activeProjectId: id }),
    }),
    { name: 'crowbar.activeProject', partialize: (s) => ({ activeProjectId: s.activeProjectId }) },
  ),
)
