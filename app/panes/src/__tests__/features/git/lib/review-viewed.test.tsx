import { act, renderHook, waitFor } from '@testing-library/react'
import { IDBFactory } from 'fake-indexeddb'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it } from 'vitest'
import {
  isFileViewed,
  useReviewViewed,
  useReviewViewedById,
} from '@/features/git/lib/review-viewed'
import type { GitDiff } from '@/features/git/types/git-types'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import {
  destroyWorkspaceStore,
  detachedWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { viewedMapOf } from '@/__tests__/__fixtures__/viewed'
import { resetDB } from '@/lib/persistence/idb'
import {
  loadBranchReviewViewed,
  saveBranchReviewViewed,
} from '@/lib/persistence/branch-review-viewed'

function viewedCount(viewed: Record<string, string>, files: GitDiff[]): number {
  return files.filter((f) => isFileViewed(viewed, f)).length
}

function file(path: string, over: Partial<GitDiff> = {}): GitDiff {
  return {
    file_path: path,
    is_new: false,
    is_deleted: false,
    is_renamed: false,
    lines: [],
    additions: 3,
    deletions: 1,
    ...over,
  }
}

beforeEach(() => {
  globalThis.indexedDB = new IDBFactory()
  resetDB()
})

describe('isFileViewed', () => {
  it('un-views a file when its change kind, rename source or counts move', async () => {
    const base = file('a.ts')
    const viewed = await viewedMapOf([base])
    expect(isFileViewed(viewed, base)).toBe(true)
    expect(isFileViewed(viewed, file('a.ts', { additions: 4 }))).toBe(false)
    expect(isFileViewed(viewed, file('a.ts', { deletions: 0 }))).toBe(false)
    expect(isFileViewed(viewed, file('a.ts', { is_deleted: true }))).toBe(false)
    expect(isFileViewed(viewed, file('a.ts', { old_path: 'b.ts', is_renamed: true }))).toBe(false)
    expect(isFileViewed(viewed, file('other.ts'))).toBe(false)
  })
})

describe('useReviewViewed', () => {
  function setup() {
    const store = createWorkspaceStore('ws1')
    const wrapper = ({ children }: { children: ReactNode }) => (
      <WorkspaceStoreContext.Provider value={store}>{children}</WorkspaceStoreContext.Provider>
    )
    return renderHook(() => useReviewViewed(), { wrapper })
  }

  it('loads the persisted map on first use and counts against the given files', async () => {
    const a = file('a.ts')
    await saveBranchReviewViewed('ws1', { ...(await viewedMapOf([a])), 'b.ts': 'stale' })
    const { result } = setup()
    expect(result.current.loaded).toBe(false)
    await waitFor(() => expect(result.current.loaded).toBe(true))
    expect(viewedCount(result.current.viewed, [a, file('b.ts'), file('c.ts')])).toBe(1)
  })

  it('marks a batch of files viewed with one persisted write, and unmarks them', async () => {
    const files = [file('a.ts'), file('b.ts')]
    const { result } = setup()
    await waitFor(() => expect(result.current.loaded).toBe(true))

    act(() => result.current.setFilesViewed(files, true))
    await waitFor(() => expect(viewedCount(result.current.viewed, files)).toBe(2))
    const expected = await viewedMapOf(files)
    await waitFor(async () => expect(await loadBranchReviewViewed('ws1')).toEqual(expected))

    act(() => result.current.setFilesViewed([files[0]], false))
    await waitFor(() => expect(viewedCount(result.current.viewed, files)).toBe(1))
  })
})

describe('useReviewViewedById', () => {
  it('reads the registered store with no provider, and stays inert until enabled', async () => {
    const store = getOrCreateWorkspaceStore('ws-side')
    try {
      const a = file('a.ts')
      await saveBranchReviewViewed('ws-side', await viewedMapOf([a]))

      const { result, rerender } = renderHook(
        ({ enabled }) => useReviewViewedById('ws-side', enabled),
        { initialProps: { enabled: false } },
      )
      // Hidden panel: nothing is read from IndexedDB.
      expect(store.getState().viewedLoaded).toBe(false)
      expect(result.current.loaded).toBe(false)

      rerender({ enabled: true })
      await waitFor(() => expect(result.current.loaded).toBe(true))
      expect(viewedCount(result.current.viewed, [a])).toBe(1)

      act(() => result.current.setFilesViewed([a], false))
      await waitFor(() => expect(viewedCount(result.current.viewed, [a])).toBe(0))
      await waitFor(async () => expect(await loadBranchReviewViewed('ws-side')).toEqual({}))
    } finally {
      destroyWorkspaceStore('ws-side')
    }
  })

  it('never loads for a workspace that is not mounted', async () => {
    const { result } = renderHook(() => useReviewViewedById('ws-missing', true))
    await act(async () => {})
    expect(result.current.loaded).toBe(false)
    expect(detachedWorkspaceStore().getState().viewedLoaded).toBe(false)
  })
})
