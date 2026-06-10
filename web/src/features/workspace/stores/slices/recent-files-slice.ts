import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'

const MAX_RECENT_FILES = 50

export interface RecentFileEntry {
  path: string
  name: string
  lastOpenedAt: number
}

export interface RecentFilesActions {
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
        // Remove existing entry with same path (deduplication)
        state.recentFiles = state.recentFiles.filter((f) => f.path !== path)
        // Prepend new entry at front
        state.recentFiles.unshift({ path, name, lastOpenedAt: Date.now() })
        // Cap at max
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
