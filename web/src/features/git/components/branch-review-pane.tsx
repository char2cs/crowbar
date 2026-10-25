import { useCallback, useEffect, useMemo, useState, useSyncExternalStore } from 'react'
import { useShallow } from 'zustand/react/shallow'
import {
  useWorkspaceStore,
  useWorkspaceStoreContext,
} from '@/features/workspace/stores/workspace-context'
import { useHostResolved } from '@/lib/host-resolve'
import { useSidebarStore } from '@/lib/store/sidebar'
import { getOwningChatId, subscribeToWorkspaceScope } from '@/lib/workspace-scope'
import { Badge } from '@/components/ui/badge'
import { SegmentedControl, SegmentedControlItem } from '@/components/ui/segmented-control'
import { Tabs as TabsPrimitive } from '@base-ui/react/tabs'
import { Tabs } from '@/components/ui/tabs'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { BranchReviewView } from '@/features/panes/types/pane-content'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import { getReview } from '../api/review-api'
import { useReviewFilesSummary } from '../hooks/use-review-files-summary'
import { BranchReviewHeader } from './branch-review-header'
import { ReviewConversationView } from './review-conversation-view'
import { ReviewDiffTab, type LineRequest } from './review-diff-tab'

interface BranchReviewPaneProps {
  /** The buffer this pane renders; the chosen tab is persisted on it. */
  bufferId: string
  view?: BranchReviewView
  wsId: string
  isActivePane?: boolean
}

export function BranchReviewPane({
  bufferId,
  view = 'conversation',
  wsId,
  isActivePane,
}: BranchReviewPaneProps) {
  const store = useWorkspaceStore()
  // The Files changed controls render into the tab row; the diff tab keeps their state.
  const [toolbarHost, setToolbarHost] = useState<HTMLElement | null>(null)
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

  // The title is the owning chat's, the same one the sidebar row shows.
  const branchHeader = useSidebarStore(
    useShallow((s): { title: string; branch?: string; baseBranch?: string } => {
      for (const repo of s.repos) {
        const ws = repo.workspaces.find((w) => w.id === wsId)
        if (!ws) continue
        const chatTitle = repo.chats?.find((c) => c.id === ws.owningChatId)?.title
        return {
          title: chatTitle || ws.branch || 'Branch Review',
          branch: ws.branch,
          baseBranch: ws.parentBranch,
        }
      }
      return { title: 'Branch Review' }
    }),
  )
  const threadCount = useWorkspaceStoreContext((s) => s.branchReview.threads.length)
  const { files, loaded: filesLoaded } = useReviewFilesSummary(wsId)
  // Branch totals from the summary already in the store; -1 marks a binary file.
  const stats = useMemo(() => {
    let additions = 0
    let deletions = 0
    for (const f of files) {
      additions += Math.max(f.additions ?? 0, 0)
      deletions += Math.max(f.deletions ?? 0, 0)
    }
    return { files: files.length, additions, deletions }
  }, [files])
  const [lineRequest, setLineRequest] = useState<LineRequest | undefined>()

  const setView = (next: BranchReviewView) => {
    // A pending line request belongs to the surface it was raised for; keeping
    // it would replay the scroll the next time Files changed mounts.
    if (next === 'conversation') setLineRequest(undefined)
    windowPaneStore.getState().bufferActions.setBranchReviewView(bufferId, next)
  }

  const jumpToLine = (thread: ReviewThread) => {
    setLineRequest((prev) => ({
      path: thread.filePath,
      lineNumber: thread.lineNumber,
      side: thread.side,
      nonce: (prev?.nonce ?? 0) + 1,
    }))
    setView('files')
  }

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

  // On a paired install the project list says whose daemon holds the review.
  const hostResolved = useHostResolved({ wsId })

  useEffect(() => {
    // Nothing to fetch yet — wait for owningChatId and the host (dependencies below)
    // rather than firing a call that can only fail and land on the error status.
    if (owningChatId === null || !hostResolved) return
    void load()
  }, [load, owningChatId, hostResolved])

  return (
    <div className="@container flex h-full flex-col overflow-hidden bg-background">
      <div className="mx-auto w-full max-w-[960px] shrink-0 px-4 @lg:px-8">
        <BranchReviewHeader {...branchHeader} stats={filesLoaded ? stats : undefined} />
        <Tabs
          value={view}
          onValueChange={(next) => setView(next as BranchReviewView)}
          className="gap-0 pb-3"
        >
          <div className="flex items-center justify-between gap-2">
            <SegmentedControl render={<TabsPrimitive.List activateOnFocus />}>
              <SegmentedControlItem render={<TabsPrimitive.Tab value="conversation" />}>
                Conversation
                <Badge variant="secondary" size="sm" className="@max-[420px]:hidden">
                  {threadCount}
                </Badge>
              </SegmentedControlItem>
              <SegmentedControlItem render={<TabsPrimitive.Tab value="files" />}>
                Files changed
                {filesLoaded && (
                  <Badge variant="secondary" size="sm" className="@max-[420px]:hidden">
                    {files.length}
                  </Badge>
                )}
              </SegmentedControlItem>
            </SegmentedControl>
            <div ref={setToolbarHost} className="flex items-center" />
          </div>
        </Tabs>
      </div>
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        {view === 'conversation' ? (
          <div className="min-h-0 flex-1 overflow-y-auto">
            <ReviewConversationView wsId={wsId} onJumpToLine={jumpToLine} />
          </div>
        ) : (
          <ReviewDiffTab
            onRetry={() => void load()}
            wsId={wsId}
            lineRequest={lineRequest}
            isActivePane={isActivePane}
            toolbarHost={toolbarHost}
          />
        )}
      </div>
    </div>
  )
}
