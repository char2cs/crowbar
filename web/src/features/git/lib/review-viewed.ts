import { useCallback, useEffect } from 'react'
import { useStore } from 'zustand'
import type { GitDiff } from '@/features/git/types/git-types'
import { useRegisteredWorkspaceStore } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'

/**
 * A fingerprint of a file as the files summary describes it: change kind, old
 * path and ± counts. A file marked viewed stays viewed only while this holds.
 *
 * The summary carries no content hash, so an edit that leaves the counts alone
 * (one line rewritten in place) is NOT detected; the cost of a stronger check
 * would be fetching every patch.
 */
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

/**
 * Viewed state for the review surface, from the ambient workspace store.
 * Calling it loads the persisted map, so mount it only where the review is
 * shown.
 */
export function useReviewViewed(): ReviewViewed {
  return useViewedOf(useWorkspaceStore(), true)
}

/**
 * The same state for a surface outside the workspace provider (the sidebar),
 * addressed by an explicit `wsId`. Loads only while `enabled`, and never for a
 * workspace that is not mounted (the registry then hands back a detached store).
 */
export function useReviewViewedById(wsId: string, enabled: boolean): ReviewViewed {
  const store = useRegisteredWorkspaceStore(wsId)
  return useViewedOf(store, enabled && wsId !== '' && store.getState().workspaceId === wsId)
}
