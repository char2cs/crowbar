import { enableMapSet } from 'immer'
import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'

// Enable Immer's MapSet plugin so Set mutations work inside immer producers
enableMapSet()

export interface FileWatcherActions {
  watchPath(path: string): void
  unwatchPath(path: string): void
  getWatchedPaths(): string[]
  markPendingSave(path: string): void
  clearPendingSave(path: string): void
  isPendingSave(path: string): boolean
}

export interface FileWatcherSlice {
  watchedPaths: Set<string>
  pendingSaves: Set<string>
  fileWatcherActions: FileWatcherActions
}

export const createFileWatcherSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  FileWatcherSlice
> = (set, get) => ({
  watchedPaths: new Set<string>(),
  pendingSaves: new Set<string>(),

  fileWatcherActions: {
    watchPath(path) {
      set(state => { state.watchedPaths.add(path) })
    },

    unwatchPath(path) {
      set(state => { state.watchedPaths.delete(path) })
    },

    getWatchedPaths() {
      return Array.from(get().watchedPaths)
    },

    markPendingSave(path) {
      set(state => { state.pendingSaves.add(path) })
    },

    clearPendingSave(path) {
      set(state => { state.pendingSaves.delete(path) })
    },

    isPendingSave(path) {
      return get().pendingSaves.has(path)
    },
  },
})
