import { describe, it, expect, beforeEach } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import {
  createFileWatcherSlice,
  type FileWatcherSlice,
} from '@/features/workspace/stores/slices/file-watcher-slice'

function makeStore() {
  return createStore<FileWatcherSlice>()(
    immer((set, get) =>
      createFileWatcherSlice(
        ...([set, get, {}] as unknown as Parameters<typeof createFileWatcherSlice>),
      ),
    ),
  )
}

describe('file-watcher-slice', () => {
  let store: ReturnType<typeof makeStore>
  beforeEach(() => {
    store = makeStore()
  })

  it('starts with no watched paths and no pending saves', () => {
    expect(store.getState().fileWatcherActions.getWatchedPaths()).toHaveLength(0)
    expect(store.getState().fileWatcherActions.isPendingSave('/a.ts')).toBe(false)
  })

  it('watchPath adds a path', () => {
    store.getState().fileWatcherActions.watchPath('/src/index.ts')
    expect(store.getState().fileWatcherActions.getWatchedPaths()).toContain('/src/index.ts')
  })

  it('unwatchPath removes a path', () => {
    store.getState().fileWatcherActions.watchPath('/src/index.ts')
    store.getState().fileWatcherActions.unwatchPath('/src/index.ts')
    expect(store.getState().fileWatcherActions.getWatchedPaths()).not.toContain('/src/index.ts')
  })

  it('markPendingSave / clearPendingSave roundtrip', () => {
    store.getState().fileWatcherActions.markPendingSave('/src/index.ts')
    expect(store.getState().fileWatcherActions.isPendingSave('/src/index.ts')).toBe(true)
    store.getState().fileWatcherActions.clearPendingSave('/src/index.ts')
    expect(store.getState().fileWatcherActions.isPendingSave('/src/index.ts')).toBe(false)
  })
})
