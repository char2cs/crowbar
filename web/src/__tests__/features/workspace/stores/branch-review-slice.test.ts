import { describe, it, expect, beforeEach } from 'vitest'
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
