import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useWorkspaceStoreById } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'
import { getWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { useSidebarStore } from '@/lib/store/sidebar'
import { getPostDeleteNavigationTarget } from '@/lib/store/repo-tree'
import { setMergeStrategy as patchMergeStrategy, mergeIntoParent } from '../api/review-api'
import type { MergeStrategy } from '@/features/workspace/stores/slices/branch-review-slice'

/** Strategy choice and the merge call, shared by the sidebar popover and the Conversation merge box. */
export function useBranchMerge(wsId: string) {
  const [mergeError, setMergeError] = useState<string | null>(null)
  const [strategyError, setStrategyError] = useState<string | null>(null)
  const [merging, setMerging] = useState(false)
  const navigate = useNavigate()
  const strategy = useWorkspaceStoreById(wsId, (s) => s.branchReview.mergeStrategy)

  // Persist the chosen strategy so it becomes the default next time (optimistic).
  const selectStrategy = async (next: MergeStrategy) => {
    if (next === strategy) return
    const previous = strategy
    setStrategyError(null)
    getWorkspaceStore(wsId)?.getState().setBranchReviewMergeStrategy(next)
    try {
      await patchMergeStrategy(wsId, next)
    } catch {
      getWorkspaceStore(wsId)?.getState().setBranchReviewMergeStrategy(previous)
      setStrategyError('Failed to save strategy — try again')
    }
  }

  /** Resolves true when the daemon accepted the merge. */
  const merge = async (deleteAfterMerge: boolean): Promise<boolean> => {
    setMergeError(null)
    setMerging(true)
    let redirect: { projectId: string; repoId: string; wsId: string } | null = null
    if (deleteAfterMerge) {
      const repos = useSidebarStore.getState().repos
      const targetWsId = getPostDeleteNavigationTarget(repos, wsId)
      const repo = targetWsId
        ? repos.find((r) => r.workspaces.some((w) => w.id === targetWsId))
        : undefined
      if (targetWsId && repo) {
        redirect = { projectId: repo.projectId ?? '', repoId: repo.id, wsId: targetWsId }
      }
    }
    try {
      await mergeIntoParent(wsId, strategy, deleteAfterMerge)
      if (redirect) void navigate({ to: '/ide/$projectId/$repoId/$wsId', params: redirect })
      return true
    } catch {
      setMergeError('Merge failed — check the logs for details')
      return false
    } finally {
      setMerging(false)
    }
  }

  return { strategy, selectStrategy, strategyError, merge, merging, mergeError }
}
