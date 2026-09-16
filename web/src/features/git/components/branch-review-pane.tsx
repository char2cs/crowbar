import { useCallback, useEffect, useSyncExternalStore } from 'react'
import { useShallow } from 'zustand/react/shallow'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { useSidebarStore } from '@/lib/store/sidebar'
import { getOwningChatId, subscribeToWorkspaceScope } from '@/lib/workspace-scope'
import { getReview } from '../api/review-api'
import { ReviewDiffTab } from './review-diff-tab'

interface BranchReviewPaneProps {
  wsId: string
  isActivePane?: boolean
}

export function BranchReviewPane({ wsId, isActivePane }: BranchReviewPaneProps) {
  const store = useWorkspaceStore()
  // reviewBaseForWorkspace(wsId) — which getReview below resolves through —
  // throws without a recorded owning chat id. The sidebar's chat-list fetch
  // that records one races WorkspaceView's own (often faster) hydration, so on
  // a workspace whose review tab auto-restores on activation this can still be
  // null. Subscribing makes the id a piece of React state so `load` below can
  // wait for it instead of firing early and having its catch set a permanent
  // 'error' status — same fix as useWorkspaceEffects' useOwningChatId.
  const owningChatId = useSyncExternalStore(
    (onChange) => subscribeToWorkspaceScope(wsId, onChange),
    () => getOwningChatId(wsId),
  )

  // Branch + base for the shared diff header: title = branch name, meta = → base.
  // Sourced from the sidebar workspace record (same data the merge section uses).
  const branchHeader = useSidebarStore(
    useShallow((s): { title: string; baseBranch?: string } => {
      for (const repo of s.repos) {
        const ws = repo.workspaces.find((w) => w.id === wsId)
        if (ws) return { title: ws.branch || 'Branch Review', baseBranch: ws.parentBranch }
      }
      return { title: 'Branch Review' }
    }),
  )

  // Load the composite review read model on mount: description, merge strategy
  // and conversations. The DIFF no longer comes from here — the surface reads
  // /review/files + /review/outline and fetches patches per file.
  // Threads are intentionally NOT sourced here — they are seeded and kept live
  // by useWorkspaceThreadsStream (mounted in useWorkspaceEffects) so optimistic
  // writes and WS pushes are not clobbered on every pane remount.
  const load = useCallback(async () => {
    const actions = store.getState()
    actions.setBranchReviewDiffStatus('loading')
    try {
      const review = await getReview(wsId)
      const a = store.getState()
      a.setBranchReviewDescription(review.description)
      a.setBranchReviewMergeStrategy(review.mergeStrategy)
      a.setBranchReviewConversations(review.conversations)
      // Deliberately NOT storing review.diff. The pane renders from the
      // files summary + outline now; keeping the composite's line-level diff
      // would re-import the very payload this phase removed.
    } catch {
      store.getState().setBranchReviewDiffStatus('error')
    }
  }, [store, wsId])

  useEffect(() => {
    // Nothing to fetch yet — wait for owningChatId (dependency below) rather
    // than firing a call that can only fail and land on the error status.
    if (owningChatId === null) return
    void load()
  }, [load, owningChatId])

  return (
    <div className="flex h-full flex-col overflow-hidden bg-background">
      <div className="flex flex-1 flex-col overflow-hidden">
        <ReviewDiffTab
          onRetry={() => void load()}
          wsId={wsId}
          branchHeader={branchHeader}
          isActivePane={isActivePane}
        />
      </div>
    </div>
  )
}
