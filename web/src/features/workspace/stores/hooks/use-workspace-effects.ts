import { useEffect } from 'react'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useBufferActions } from './use-buffer-store'
import { useWorkspaceStore } from '../workspace-context'
import { useFileTreeStore } from '@/features/files/stores/file-tree-store'
import { dataOf } from '@/lib/loadable'
import { openFileContent } from '@/features/workspace/lib/open-file-content'
import type { AppFile } from '@/features/file-system/types/app'
import type { BranchReviewContent } from '@/features/panes/types/pane-content'

export function useWorkspaceEffects(wsId: string, label?: string) {
  const bufferActions = useBufferActions()
  const store = useWorkspaceStore()
  const repoPath = `/repos/${wsId}`

  // Seed file system from API
  useEffect(() => {
    void (async () => {
      await useFileTreeStore.getState().fetch(repoPath)
      const files = dataOf(useFileTreeStore.getState().data)
      if (!files) return
      useFileSystemStore.setState({
        rootFolderPath: repoPath,
        files: files as unknown as AppFile[],
        handleFileOpen: async (path: string, revealOrIsDir?: boolean) => {
          if (revealOrIsDir === true) return
          await openFileContent(path, bufferActions, { preview: false })
        },
        handleFileSelect: (path: string, isDir?: boolean) => {
          if (isDir) return
          void openFileContent(path, bufferActions, { preview: true })
        },
      })
    })()
  }, [repoPath]) // eslint-disable-line react-hooks/exhaustive-deps

  // Open the branchReview buffer — the sole default pane for a workspace.
  // (Conversations are surfaced inside the review's About tab; individual
  // chats open on demand as their own crowbarChat buffers.)
  //
  // Only open it when one isn't already present. If the layout was restored
  // with the review already in a pane, re-opening would add the same buffer to
  // the (possibly different) active pane — making it appear in two panes.
  useEffect(() => {
    const alreadyOpen = store.getState().buffers.some(
      b => b.type === 'branchReview' && (b as BranchReviewContent).wsId === wsId,
    )
    if (alreadyOpen) return
    const branchName = label ?? wsId
    bufferActions.openContent({ type: 'branchReview', wsId, branchName, name: branchName })
  }, [wsId]) // eslint-disable-line react-hooks/exhaustive-deps
}
