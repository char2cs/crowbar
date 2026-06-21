import { describe, it, expect, beforeEach } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import {
  createBranchReviewSlice,
  INITIAL_BRANCH_REVIEW_STATE,
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

  it('starts with null activeFileKey and nonce 0', () => {
    const { activeFileKey, activeFileNonce } = store.getState().branchReview
    expect(activeFileKey).toBeNull()
    expect(activeFileNonce).toBe(0)
  })

  it('setBranchReviewActiveFile sets the key and increments the nonce', () => {
    store.getState().setBranchReviewActiveFile('src/foo.ts:0')
    const state = store.getState().branchReview
    expect(state.activeFileKey).toBe('src/foo.ts:0')
    expect(state.activeFileNonce).toBe(1)
  })

  it('calling setBranchReviewActiveFile twice with the same key bumps the nonce each time', () => {
    store.getState().setBranchReviewActiveFile('src/foo.ts:0')
    store.getState().setBranchReviewActiveFile('src/foo.ts:0')
    const state = store.getState().branchReview
    expect(state.activeFileKey).toBe('src/foo.ts:0')
    expect(state.activeFileNonce).toBe(2)
  })

  it('setBranchReviewActiveFile also switches activeSubtab to diff', () => {
    expect(store.getState().branchReview.activeSubtab).toBe(
      INITIAL_BRANCH_REVIEW_STATE.activeSubtab,
    )
    store.getState().setBranchReviewActiveFile('src/bar.ts:1')
    expect(store.getState().branchReview.activeSubtab).toBe('diff')
  })

  it('setBranchReviewActiveFile accepts null to clear the active file', () => {
    store.getState().setBranchReviewActiveFile('src/foo.ts:0')
    store.getState().setBranchReviewActiveFile(null)
    const state = store.getState().branchReview
    expect(state.activeFileKey).toBeNull()
    expect(state.activeFileNonce).toBe(2)
  })
})
