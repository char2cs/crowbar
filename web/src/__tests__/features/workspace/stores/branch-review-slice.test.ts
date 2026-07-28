import { describe, it, expect, beforeEach } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import {
  createBranchReviewSlice,
  type BranchReviewSlice,
  type ReviewThread,
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

function makeThread(overrides: Partial<ReviewThread> = {}): ReviewThread {
  return {
    id: 'thread-1',
    filePath: 'src/foo.ts',
    lineNumber: 10,
    startLine: 8,
    endLine: 12,
    side: 'new',
    messages: [],
    isResolved: false,
    ...overrides,
  }
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

  // ── upsertReviewThread ──────────────────────────────────────────────

  it('upsertReviewThread inserts a new thread when id is absent', () => {
    const t = makeThread({ id: 't-a' })
    store.getState().upsertReviewThread(t)
    const threads = store.getState().branchReview.threads
    expect(threads).toHaveLength(1)
    expect(threads[0].id).toBe('t-a')
    expect(threads[0].startLine).toBe(8)
    expect(threads[0].endLine).toBe(12)
    expect(threads[0].side).toBe('new')
  })

  it('upsertReviewThread merges (replaces) by id when thread already exists', () => {
    const original = makeThread({ id: 't-a', isResolved: false, lineNumber: 10 })
    const updated = makeThread({
      id: 't-a',
      isResolved: true,
      lineNumber: 99,
      startLine: 99,
      endLine: 99,
    })

    store.getState().upsertReviewThread(original)
    store.getState().upsertReviewThread(updated)

    const threads = store.getState().branchReview.threads
    expect(threads).toHaveLength(1)
    expect(threads[0].lineNumber).toBe(99)
    expect(threads[0].isResolved).toBe(true)
  })

  it('upsertReviewThread preserves insertion order for different ids', () => {
    store.getState().upsertReviewThread(makeThread({ id: 't-1' }))
    store.getState().upsertReviewThread(makeThread({ id: 't-2' }))
    store.getState().upsertReviewThread(makeThread({ id: 't-3' }))

    const ids = store.getState().branchReview.threads.map((t) => t.id)
    expect(ids).toEqual(['t-1', 't-2', 't-3'])
  })

  // ── setReviewThreadResolved ─────────────────────────────────────────

  it('setReviewThreadResolved(id, true) marks thread resolved', () => {
    store.getState().addReviewThread(makeThread({ id: 'r-1', isResolved: false }))
    store.getState().setReviewThreadResolved('r-1', true)
    expect(store.getState().branchReview.threads[0].isResolved).toBe(true)
  })

  it('setReviewThreadResolved(id, false) reopens a resolved thread', () => {
    store.getState().addReviewThread(makeThread({ id: 'r-2', isResolved: true }))
    store.getState().setReviewThreadResolved('r-2', false)
    expect(store.getState().branchReview.threads[0].isResolved).toBe(false)
  })

  it('setReviewThreadResolved is a no-op for unknown ids', () => {
    store.getState().addReviewThread(makeThread({ id: 'r-3', isResolved: false }))
    store.getState().setReviewThreadResolved('no-such-id', true)
    expect(store.getState().branchReview.threads[0].isResolved).toBe(false)
  })

  // ── legacy resolveReviewThread backward compat ──────────────────────

  it('resolveReviewThread (legacy) still sets isResolved to true', () => {
    store.getState().addReviewThread(makeThread({ id: 'legacy-1', isResolved: false }))
    store.getState().resolveReviewThread('legacy-1')
    expect(store.getState().branchReview.threads[0].isResolved).toBe(true)
  })

  // ── ReviewThread shape ──────────────────────────────────────────────

  it('ReviewThread accepts side "old" and "new"', () => {
    const tOld = makeThread({ id: 'side-old', side: 'old' })
    const tNew = makeThread({ id: 'side-new', side: 'new' })
    store.getState().addReviewThread(tOld)
    store.getState().addReviewThread(tNew)
    const threads = store.getState().branchReview.threads
    expect(threads[0].side).toBe('old')
    expect(threads[1].side).toBe('new')
  })
})
