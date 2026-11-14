import { useCallback, useEffect } from 'react'
import { useStore } from 'zustand'
import type { GitDiff } from '@/features/git/types/git-types'
import { useRegisteredWorkspaceStore } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'

/** A viewed file stays viewed while this holds: kind, old path, ± counts. With no content hash, an
 *  edit that keeps the counts goes unseen; a stronger check would mean fetching every patch. */
function fileSignature(file: GitDiff): string {
  const kind = file.is_new ? 'A' : file.is_deleted ? 'D' : file.is_renamed ? 'R' : 'M'
  return `${kind}|${file.old_path ?? ''}|${file.additions ?? 0}|${file.deletions ?? 0}`
}

export function isFileViewed(viewed: Readonly<Record<string, string>>, file: GitDiff): boolean {
  return viewed[file.file_path] === fileSignature(file)
}

export interface ReviewViewed {
  viewed: Readonly<Record<string, string>>
  /** False until the persisted map is read; nothing is viewed before then. */
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

/** Viewed state from the ambient workspace store. It loads the persisted map, so mount it only
 *  where the review is shown. */
export function useReviewViewed(): ReviewViewed {
  return useViewedOf(useWorkspaceStore(), true)
}

/** The same for a surface outside the provider (the sidebar), by `wsId`. Loads only while
 *  `enabled` and the workspace is mounted (otherwise the registry returns a detached store). */
export function useReviewViewedById(wsId: string, enabled: boolean): ReviewViewed {
  const store = useRegisteredWorkspaceStore(wsId)
  return useViewedOf(store, enabled && wsId !== '' && store.getState().workspaceId === wsId)
}
