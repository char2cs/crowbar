import { act, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
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
  unmount()
}

/** The persisted viewed map (path to signature) after viewing `files`. */
export async function viewedMapOf(files: readonly GitDiff[]): Promise<Record<string, string>> {
  const store = createWorkspaceStore('viewed-map-fixture')
  await markViewed(store, files)
  return { ...store.getState().viewedFiles }
}
