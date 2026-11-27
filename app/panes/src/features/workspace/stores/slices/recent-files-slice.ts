import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'

const MAX_RECENT_FILES = 50

interface RecentFileEntry {
  path: string
  name: string
  lastOpenedAt: number
}

interface RecentFilesActions {
  addRecentFile(path: string, name: string): void
  removeRecentFile(path: string): void
  clearRecentFiles(): void
  getRecentFiles(): RecentFileEntry[]
}

export interface RecentFilesSlice {
  recentFiles: RecentFileEntry[]
  recentFilesActions: RecentFilesActions
}

export const createRecentFilesSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  RecentFilesSlice
> = (set, get) => ({
  recentFiles: [],

  recentFilesActions: {
    addRecentFile(path, name) {
      set((state) => {
        state.recentFiles = state.recentFiles.filter((f) => f.path !== path)
        state.recentFiles.unshift({ path, name, lastOpenedAt: Date.now() })
        if (state.recentFiles.length > MAX_RECENT_FILES) {
          state.recentFiles = state.recentFiles.slice(0, MAX_RECENT_FILES)
        }
      })
    },

    removeRecentFile(path) {
      set((state) => {
        state.recentFiles = state.recentFiles.filter((f) => f.path !== path)
      })
    },

    clearRecentFiles() {
      set((state) => {
        state.recentFiles = []
      })
    },

    getRecentFiles() {
      return get().recentFiles
    },
  },
})
