import { useCallback, useEffect } from 'react'
import { useStore } from 'zustand'
import type { GitDiff } from '@/features/git/types/git-types'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'

function fileSignature(file: GitDiff): string {
  const kind = file.is_new ? 'A' : file.is_deleted ? 'D' : file.is_renamed ? 'R' : 'M'
  return `${kind}|${file.old_path ?? ''}|${file.additions ?? 0}|${file.deletions ?? 0}`
}

export function isFileViewed(viewed: Readonly<Record<string, string>>, file: GitDiff): boolean {
  return viewed[file.file_path] === fileSignature(file)
}

export interface ReviewViewed {
  viewed: Readonly<Record<string, string>>
  loaded: boolean
  setFilesViewed: (files: readonly GitDiff[], value: boolean) => void
}

function useViewedOf(store: WorkspaceStore, enabled: boolean): ReviewViewed {
  const viewed = useStore(store, (s) => s.viewedFiles)
  const loaded = useStore(store, (s) => s.viewedLoaded)

  useEffect(() => {
    if (enabled) void store.getState().loadBranchReviewViewed()
  }, [store, enabled])

  const setFilesViewed = useCallback(
    (files: readonly GitDiff[], value: boolean) => {
      void store.getState().setFilesViewed(
        files.map((file) => ({ path: file.file_path, signature: fileSignature(file) })),
        value,
      )
    },
    [store],
  )

  return { viewed, loaded, setFilesViewed }
}

export function useReviewViewed(): ReviewViewed {
  return useViewedOf(useWorkspaceStore(), true)
}
