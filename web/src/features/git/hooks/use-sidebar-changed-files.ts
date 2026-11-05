import { useMemo, useRef } from 'react'
import deepEqual from 'fast-deep-equal'
import { useWorkspaceStoreById } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'
import { gitStatusToChangedFiles } from '@/features/git/utils/git-status-to-changed-files'
import type { GitDiff } from '@/features/git/types/git-types'

export interface UseSidebarChangedFilesResult {
  files: GitDiff[]
  uncommittedCount: number
}

const EMPTY_FILES: GitDiff[] = []

/** The sidebar changed-files tree: the files summary (committed and working tree, with counts,
 *  current per git tick) once loaded; until then the working-tree status projection, which is
 *  already in the store but lacks committed-only files and counts. */
export function useSidebarChangedFiles(wsId: string | null): UseSidebarChangedFilesResult {
  // Null until it has loaded.
  const summaryFiles = useWorkspaceStoreById(wsId ?? '', (s) => s.reviewFiles)

  const statusFiles = useWorkspaceStoreById(wsId ?? '', (s) => s.gitStatus?.files)

  // Hold the previous projection when an identical git reload reprojects the same content,
  // so the memoized tree doesn't churn.
  const rawProjected = useMemo(() => gitStatusToChangedFiles(statusFiles), [statusFiles])
  const projectedRef = useRef<GitDiff[]>(rawProjected)
  if (rawProjected !== projectedRef.current && !deepEqual(rawProjected, projectedRef.current)) {
    projectedRef.current = rawProjected
  }
  const projected = projectedRef.current

  const files = summaryFiles ?? projected
  const uncommittedCount = summaryFiles ? countUncommitted(summaryFiles) : projected.length

  return { files: files ?? EMPTY_FILES, uncommittedCount }
}

function countUncommitted(files: GitDiff[]): number {
  return files.reduce((n, f) => n + (f.uncommitted ? 1 : 0), 0)
}
