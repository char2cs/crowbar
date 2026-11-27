import { useEffect, useState, useSyncExternalStore } from 'react'
import { getReviewFiles } from '@/features/git/api/review-api'
import { reviewFilesSummaryToChangedFiles } from '@/features/git/utils/review-file-summary-to-git-diff'
import { useHostResolved } from '@/lib/host-resolve'
import { report } from '@/lib/errors/report'
import { getOwningChatId, subscribeToWorkspaceScope } from '@/lib/workspace-scope'
import type { GitDiff } from '@/features/git/types/git-types'
import { useWorkspaceStoreById } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'

export interface UseReviewFilesSummaryResult {
  files: GitDiff[]
  loaded: boolean
}

const EMPTY_FILES: GitDiff[] = []

export function useReviewFilesSummary(
  wsId: string | null,
  commit?: string,
): UseReviewFilesSummaryResult {
  const storeFiles = useWorkspaceStoreById(wsId ?? '', (s) => s.reviewFiles)
  const commitSummary = useCommitFilesSummary(commit ? wsId : null, commit)
  if (commit) return commitSummary
  return { files: storeFiles ?? EMPTY_FILES, loaded: storeFiles !== null }
}

function useCommitFilesSummary(
  wsId: string | null,
  commit: string | undefined,
): UseReviewFilesSummaryResult {
  const [files, setFiles] = useState<GitDiff[]>(EMPTY_FILES)
  const [loaded, setLoaded] = useState(false)

  const scopeKey = `${wsId ?? ''}\u0000${commit ?? ''}`
  const [prevScope, setPrevScope] = useState(scopeKey)
  if (prevScope !== scopeKey) {
    setPrevScope(scopeKey)
    setLoaded(false)
    setFiles(EMPTY_FILES)
  }

  const owningChatId = useSyncExternalStore(
    (onChange) => (wsId ? subscribeToWorkspaceScope(wsId, onChange) : () => {}),
    () => (wsId ? getOwningChatId(wsId) : null),
  )

  const hostResolved = useHostResolved({ wsId: wsId ?? '' })

  useEffect(() => {
    if (!wsId || owningChatId === null || !hostResolved) return
    let cancelled = false
    getReviewFiles({ wsId, commit })
      .then((summary) => {
        if (cancelled) return
        setFiles(reviewFilesSummaryToChangedFiles(summary))
        setLoaded(true)
      })
      .catch((err: unknown) => report(err, { action: 'Load the commit’s files' }))
    return () => {
      cancelled = true
    }
  }, [wsId, commit, owningChatId, hostResolved])

  return { files, loaded }
}
