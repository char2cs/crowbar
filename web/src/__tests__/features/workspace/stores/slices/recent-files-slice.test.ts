import { describe, it, expect, beforeEach } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { createRecentFilesSlice, type RecentFilesSlice } from '@/features/workspace/stores/slices/recent-files-slice'

function makeStore() {
  return createStore<RecentFilesSlice>()(immer((set, get) => createRecentFilesSlice(set as any, get as any, {} as any)))
}

describe('recent-files-slice', () => {
  let store: ReturnType<typeof makeStore>
  beforeEach(() => { store = makeStore() })

  it('starts with no recent files', () => {
    expect(store.getState().recentFilesActions.getRecentFiles()).toHaveLength(0)
  })

  it('addRecentFile adds an entry', () => {
    store.getState().recentFilesActions.addRecentFile('/src/index.ts', 'index.ts')
    const files = store.getState().recentFilesActions.getRecentFiles()
    expect(files).toHaveLength(1)
    expect(files[0].path).toBe('/src/index.ts')
    expect(files[0].name).toBe('index.ts')
  })

  it('addRecentFile moves existing entry to front', () => {
    const actions = store.getState().recentFilesActions
    actions.addRecentFile('/a.ts', 'a.ts')
    actions.addRecentFile('/b.ts', 'b.ts')
    actions.addRecentFile('/a.ts', 'a.ts')
    const files = actions.getRecentFiles()
    expect(files).toHaveLength(2)
    expect(files[0].path).toBe('/a.ts')
  })

  it('caps at 50 entries', () => {
    const actions = store.getState().recentFilesActions
    for (let i = 0; i < 60; i++) {
      actions.addRecentFile(`/file${i}.ts`, `file${i}.ts`)
    }
    expect(actions.getRecentFiles()).toHaveLength(50)
  })

  it('removeRecentFile removes the entry', () => {
    store.getState().recentFilesActions.addRecentFile('/a.ts', 'a.ts')
    store.getState().recentFilesActions.removeRecentFile('/a.ts')
    expect(store.getState().recentFilesActions.getRecentFiles()).toHaveLength(0)
  })

  it('clearRecentFiles empties the list', () => {
    store.getState().recentFilesActions.addRecentFile('/a.ts', 'a.ts')
    store.getState().recentFilesActions.clearRecentFiles()
    expect(store.getState().recentFilesActions.getRecentFiles()).toHaveLength(0)
  })
})
