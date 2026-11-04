import { describe, it, expect, beforeEach } from 'vitest'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'
import {
  loadBranchReviewViewed,
  saveBranchReviewViewed,
} from '@/lib/persistence/branch-review-viewed'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import {
  createBranchReviewSlice,
  type BranchReviewSlice,
} from '@/features/workspace/stores/slices/branch-review-slice'

function makeStore() {
  return createStore<BranchReviewSlice>()(
    immer((set, get) =>
      createBranchReviewSlice(
        ...([set, get, {}] as unknown as Parameters<typeof createBranchReviewSlice>),
      ),
    ),
  )
}

describe('branch-review-slice', () => {
  let store: ReturnType<typeof makeStore>

  beforeEach(() => {
    store = makeStore()
  })

  it('starts with no reveal request', () => {
    const { revealFilePath, revealFileNonce } = store.getState().branchReview
    expect(revealFilePath).toBeNull()
    expect(revealFileNonce).toBe(0)
  })

  it('revealBranchReviewFile records the PATH and bumps the nonce', () => {
    // A path, not the `path:index` composite this replaced — that one was
    // resolved against a whole-diff cache the surface no longer loads, so every
    // click in the sidebar's changed-files tree resolved to nothing.
    store.getState().revealBranchReviewFile('src/foo.ts')
    const state = store.getState().branchReview
    expect(state.revealFilePath).toBe('src/foo.ts')
    expect(state.revealFileNonce).toBe(1)
  })

  it('bumps the nonce again for the SAME path', () => {
    // The nonce is what makes a repeat click re-reveal: the path compares equal
    // to itself, so an effect keyed on it alone would never re-run.
    store.getState().revealBranchReviewFile('src/foo.ts')
    store.getState().revealBranchReviewFile('src/foo.ts')
    const state = store.getState().branchReview
    expect(state.revealFilePath).toBe('src/foo.ts')
    expect(state.revealFileNonce).toBe(2)
  })
})

describe('branch-review-slice viewed files', () => {
  function makeViewedStore() {
    return createStore<BranchReviewSlice & { workspaceId: string }>()(
      immer((set, get) => ({
        workspaceId: 'ws1',
        ...createBranchReviewSlice(
          ...([set, get, {}] as unknown as Parameters<typeof createBranchReviewSlice>),
        ),
      })),
    )
  }

  beforeEach(() => {
    globalThis.indexedDB = new IDBFactory()
    resetDB()
  })

  it('starts unloaded and empty, and reads nothing until asked', async () => {
    await saveBranchReviewViewed('ws1', { 'a.ts': 's1' })
    const store = makeViewedStore()
    expect(store.getState().viewedLoaded).toBe(false)
    expect(store.getState().viewedFiles).toEqual({})

    await store.getState().loadBranchReviewViewed()
    expect(store.getState().viewedLoaded).toBe(true)
    expect(store.getState().viewedFiles).toEqual({ 'a.ts': 's1' })
  })

  it('marks and unmarks a batch with one write-through, keeping earlier entries', async () => {
    await saveBranchReviewViewed('ws1', { 'old.ts': 'o' })
    const store = makeViewedStore()
    await store.getState().setFilesViewed(
      [
        { path: 'a.ts', signature: 's1' },
        { path: 'b.ts', signature: 's2' },
      ],
      true,
    )
    expect(store.getState().viewedFiles).toEqual({ 'old.ts': 'o', 'a.ts': 's1', 'b.ts': 's2' })
    expect(await loadBranchReviewViewed('ws1')).toEqual(store.getState().viewedFiles)

    await store.getState().setFilesViewed([{ path: 'a.ts', signature: 's1' }], false)
    expect(store.getState().viewedFiles).toEqual({ 'old.ts': 'o', 'b.ts': 's2' })
    expect(await loadBranchReviewViewed('ws1')).toEqual({ 'old.ts': 'o', 'b.ts': 's2' })
  })

  it('does not let a slow concurrent load undo a toggle', async () => {
    await saveBranchReviewViewed('ws1', { 'a.ts': 's1' })
    const store = makeViewedStore()
    const loads = [
      store.getState().loadBranchReviewViewed(),
      store.getState().loadBranchReviewViewed(),
    ]
    await store.getState().setFilesViewed([{ path: 'a.ts', signature: 's1' }], false)
    await Promise.all(loads)
    expect(store.getState().viewedFiles).toEqual({})
  })
})
