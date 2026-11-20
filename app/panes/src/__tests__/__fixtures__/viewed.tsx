import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { expect } from 'vitest'
import { useReviewViewed } from '@/features/git/lib/review-viewed'
import type { GitDiff } from '@/features/git/types/git-types'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

/** Marks `files` viewed in `store` through the public hook, as the UI does. */
export async function markViewed(store: WorkspaceStore, files: readonly GitDiff[]): Promise<void> {
  const wrapper = ({ children }: { children: ReactNode }) => (
    <WorkspaceStoreContext.Provider value={store}>{children}</WorkspaceStoreContext.Provider>
  )
  const { result, unmount } = renderHook(() => useReviewViewed(), { wrapper })
  await act(async () => {
    await store.getState().loadBranchReviewViewed()
    result.current.setFilesViewed(files, true)
  })
  // The owner answers asynchronously; the mark is done once the store holds it.
  await waitFor(() =>
    expect(Object.keys(store.getState().viewedFiles)).toEqual(
      expect.arrayContaining(files.map((file) => file.file_path)),
    ),
  )
  unmount()
}

/** The persisted viewed map (path to signature) after viewing `files`. */
export async function viewedMapOf(files: readonly GitDiff[]): Promise<Record<string, string>> {
  const store = createWorkspaceStore('viewed-map-fixture')
  await markViewed(store, files)
  return { ...store.getState().viewedFiles }
}
