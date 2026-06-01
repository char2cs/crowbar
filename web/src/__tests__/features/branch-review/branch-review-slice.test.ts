import { describe, it, expect } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { createBranchReviewSlice, type BranchReviewSlice } from '@/features/workspace/stores/slices/branch-review-slice'

function makeStore() {
  return createStore<BranchReviewSlice>()(
    immer((set, get, api) => createBranchReviewSlice(set as any, get as any, api as any))
  )
}

describe('branchReviewSlice', () => {
  it('has correct initial state', () => {
    const { branchReview } = makeStore().getState()
    expect(branchReview.description).toBe('')
    expect(branchReview.mergeStrategy).toBe('merge')
    expect(branchReview.activeSubtab).toBe('about')
    expect(branchReview.diffCache).toBeNull()
    expect(branchReview.diffStatus).toBe('idle')
    expect(branchReview.threads).toEqual([])
  })

  it('setBranchReviewDescription updates description', () => {
    const store = makeStore()
    store.getState().setBranchReviewDescription('hello world')
    expect(store.getState().branchReview.description).toBe('hello world')
  })

  it('setBranchReviewMergeStrategy updates strategy', () => {
    const store = makeStore()
    store.getState().setBranchReviewMergeStrategy('squash')
    expect(store.getState().branchReview.mergeStrategy).toBe('squash')
  })

  it('setBranchReviewSubtab updates active subtab', () => {
    const store = makeStore()
    store.getState().setBranchReviewSubtab('diff')
    expect(store.getState().branchReview.activeSubtab).toBe('diff')
  })

  it('addReviewThread adds a thread', () => {
    const store = makeStore()
    store.getState().addReviewThread({
      id: 't1', filePath: 'src/index.ts', lineNumber: 10,
      side: 'right', messages: [], isResolved: false,
    })
    expect(store.getState().branchReview.threads).toHaveLength(1)
    expect(store.getState().branchReview.threads[0].id).toBe('t1')
  })

  it('addReviewMessage appends to the correct thread', () => {
    const store = makeStore()
    store.getState().addReviewThread({ id: 't1', filePath: 'src/index.ts', lineNumber: 10, side: 'right', messages: [], isResolved: false })
    store.getState().addReviewMessage('t1', { id: 'm1', author: null, isAgent: false, body: 'looks good', createdAt: '2026-06-01' })
    expect(store.getState().branchReview.threads[0].messages).toHaveLength(1)
    expect(store.getState().branchReview.threads[0].messages[0].id).toBe('m1')
  })

  it('resolveReviewThread marks thread as resolved', () => {
    const store = makeStore()
    store.getState().addReviewThread({ id: 't1', filePath: 'src/index.ts', lineNumber: 10, side: 'right', messages: [], isResolved: false })
    store.getState().resolveReviewThread('t1')
    expect(store.getState().branchReview.threads[0].isResolved).toBe(true)
  })
})
