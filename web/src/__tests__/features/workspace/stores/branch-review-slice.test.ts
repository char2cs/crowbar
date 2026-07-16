import { describe, it, expect, beforeEach } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import {
  createBranchReviewSlice,
  type BranchReviewSlice,
  type ReviewThread,
} from '@/features/workspace/stores/slices/branch-review-slice'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'

function makeStore() {
  return createStore<BranchReviewSlice>()(
    immer((set, get) =>
      createBranchReviewSlice(
        ...([set, get, {}] as unknown as Parameters<typeof createBranchReviewSlice>),
      ),
    ),
  )
}

function makeDiff(files: string[], overrides: Partial<MultiFileDiff> = {}): MultiFileDiff {
  return {
    commitHash: 'branch-head',
    files: files.map((filePath) => ({
      file_path: filePath,
      is_new: false,
      is_deleted: false,
      is_renamed: false,
      lines: [],
      uncommitted: false,
    })),
    totalFiles: files.length,
    totalAdditions: 0,
    totalDeletions: 0,
    ...overrides,
  }
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

  it('setBranchReviewActiveFile accepts null to clear the active file', () => {
    store.getState().setBranchReviewActiveFile('src/foo.ts:0')
    store.getState().setBranchReviewActiveFile(null)
    const state = store.getState().branchReview
    expect(state.activeFileKey).toBeNull()
    expect(state.activeFileNonce).toBe(2)
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

  // ── setBranchReviewDiff ───────────────────────────────────────────────

  it('setBranchReviewDiff writes the diff and flips diffStatus to loaded on the first load', () => {
    expect(store.getState().branchReview.diffStatus).toBe('idle')
    const diff = makeDiff(['src/a.ts'])

    store.getState().setBranchReviewDiff(diff)

    expect(store.getState().branchReview.diffCache).toBe(diff)
    expect(store.getState().branchReview.diffStatus).toBe('loaded')
  })

  it('setBranchReviewDiff writes a new reference when the incoming diff differs from the cached one', () => {
    store.getState().setBranchReviewDiff(makeDiff(['src/a.ts']))
    const second = makeDiff(['src/a.ts', 'src/b.ts'])

    store.getState().setBranchReviewDiff(second)

    expect(store.getState().branchReview.diffCache).toBe(second)
  })

  it('setBranchReviewDiff skips the write — same top-level state reference AND same diffCache reference — when the incoming diff deep-equals the cached one and status is already loaded', () => {
    const first = makeDiff(['src/a.ts'])
    store.getState().setBranchReviewDiff(first)
    const stateAfterFirst = store.getState()

    // A structurally-identical but distinct object instance, as a second
    // fetch of an unchanged branch diff would produce.
    const second = makeDiff(['src/a.ts'])
    store.getState().setBranchReviewDiff(second)

    // Immer no-op: returning from the recipe without mutating the draft must
    // yield the exact same top-level state reference, not just an
    // equivalent one.
    expect(store.getState()).toBe(stateAfterFirst)
    expect(store.getState().branchReview.diffCache).toBe(first)
    expect(store.getState().branchReview.diffCache).not.toBe(second)
  })

  it('setBranchReviewDiff WRITES (does not skip) when the diff deep-equals the cached value but diffStatus is not yet loaded', () => {
    const seed = makeDiff(['src/a.ts'])
    // Seed the cache directly so diffCache already deep-equals the incoming
    // diff, but leave diffStatus at something other than 'loaded' — the
    // guard must only skip once a load has actually completed.
    store.setState((s) => {
      s.branchReview.diffCache = seed
      s.branchReview.diffStatus = 'error'
    })

    const equivalent = makeDiff(['src/a.ts'])
    store.getState().setBranchReviewDiff(equivalent)

    expect(store.getState().branchReview.diffCache).toBe(equivalent)
    expect(store.getState().branchReview.diffStatus).toBe('loaded')
  })
})
