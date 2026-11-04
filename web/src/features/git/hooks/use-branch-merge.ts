import { useEffect, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useWorkspaceStoreById } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'
import {
  getActiveWorkspaceId,
  getWorkspaceStore,
  subscribeActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'
import { entityStore, holdsEntity } from '@/core/entities/entity-store'
import { entityKey } from '@/core/stream/host-scope'
import { resolvedHostOf } from '@/lib/host-resolve'
import { selectEntityBusy, useOperations } from '@/core/entities/operations'
import { readSelect, selectRepos } from '@/core/entities/select'
import { getPostDeleteNavigationTarget, holdsWorkspace } from '@/core/entities/repo-queries'
import { runOperation, whenSettled } from '@/core/entities/run-operation'
import { setMergeStrategy as patchMergeStrategy, mergeIntoParent } from '../api/review-api'
import type { MergeStrategy } from '@/features/workspace/stores/slices/branch-review-slice'

/** Calls `go` once `wsId` has left the sidebar, whichever frame lands first, while
 *  the user still shows it; leaving it (or the returned stop) ends the wait. A
 *  merge that kept the child never sees it leave, so it never navigates. */
function whenChildLeaves(wsId: string, go: () => void): () => void {
  const stops: (() => void)[] = []
  const stop = () => stops.splice(0).forEach((s) => s())
  // Named while the child is still held: once it leaves, nothing resolves its host.
  const hostId = resolvedHostOf({ wsId })
  if (!hostId) return stop
  const child = { kind: 'workspace' as const, id: entityKey(hostId, wsId) }
  const check = () => {
    if (getActiveWorkspaceId() !== wsId) return stop()
    if (holdsEntity(entityStore.getState(), child)) return
    stop()
    go()
  }
  stops.push(entityStore.subscribe(check), subscribeActiveWorkspaceId(check))
  check()
  return stop
}

/** Where to land once `wsId` is deleted by its merge, if anywhere. */
function postMergeTarget(wsId: string) {
  const repos = readSelect(selectRepos)
  const targetWsId = getPostDeleteNavigationTarget(repos, wsId)
  const repo = targetWsId ? repos.find((r) => holdsWorkspace(r, targetWsId)) : undefined
  if (!targetWsId || !repo) return null
  return { projectId: repo.projectId ?? '', repoId: repo.id, wsId: targetWsId }
}

/** The merge strategy choice and the merge into the parent, run as an operation. */
export function useBranchMerge(wsId: string, parentBranch: string | undefined) {
  const [strategyError, setStrategyError] = useState<string | null>(null)
  const navigate = useNavigate()
  // One hierarchy verb at a time per workspace; the daemon refuses a second too.
  const busy = useOperations((s) => selectEntityBusy(s, wsId))
  const strategy = useWorkspaceStoreById(wsId, (s) => s.branchReview.mergeStrategy)
  const stopWaiting = useRef<(() => void) | null>(null)
  useEffect(() => () => stopWaiting.current?.(), [])

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

  // A failure is the operations watcher's to show; the redirect waits for the fold.
  const merge = (deleteAfterMerge: boolean) => {
    if (busy) return
    const target = deleteAfterMerge ? postMergeTarget(wsId) : null
    const op = runOperation(
      {
        kind: 'workspace.merge_into_parent',
        subject: { kind: 'workspace', id: wsId },
        projectId: target?.projectId ?? '',
        label: `Merge into ${parentBranch ?? 'parent'}`,
      },
      (opId) => mergeIntoParent(wsId, strategy, deleteAfterMerge, opId),
    )
    if (!target) return
    // Waits from the click: the fold's frame can land before the terminal one,
    // and the route guard would move off the vanished child first.
    stopWaiting.current?.()
    const stop = whenChildLeaves(wsId, () => {
      void navigate({ to: '/ide/$projectId/$repoId/$wsId', params: target })
    })
    stopWaiting.current = stop
    void whenSettled(op.id).catch(stop)
  }

  return { strategy, selectStrategy, strategyError, merge, merging: busy }
}
