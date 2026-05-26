// Stub: file-system file-watcher is out of scope for this session.
import { create } from 'zustand'
import { createSelectors } from '@/utils/zustand-selectors'

export interface FileWatcherState {
  watchedPaths: string[]
  pendingSaves: Set<string>
  markPendingSave: (path: string) => void
  clearPendingSave: (path: string) => void
}

const useFileWatcherStoreBase = create<FileWatcherState>((set, get) => ({
  watchedPaths: [],
  pendingSaves: new Set(),
  markPendingSave: (path) => set({ pendingSaves: new Set([...get().pendingSaves, path]) }),
  clearPendingSave: (path) => {
    const next = new Set(get().pendingSaves)
    next.delete(path)
    set({ pendingSaves: next })
  },
}))

export const useFileWatcherStore = createSelectors(useFileWatcherStoreBase)
