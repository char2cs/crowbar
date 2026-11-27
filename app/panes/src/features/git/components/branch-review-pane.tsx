import { useCallback, useEffect, useMemo, useState, useSyncExternalStore } from 'react'
import { useShallow } from 'zustand/react/shallow'
import { useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { selectAgentChat, selectWorkspace, useSelect } from '@/core/entities/select'
import { useHostResolved } from '@/lib/host-resolve'
import { useReviewThreads } from '../hooks/use-review-threads'
import { getOwningChatId, subscribeToWorkspaceScope } from '@/lib/workspace-scope'
import { Badge } from '@/components/ui/badge'
import { SegmentedControl, SegmentedControlItem } from '@/components/ui/segmented-control'
import { Tabs as TabsPrimitive } from '@base-ui/react/tabs'
import { Tabs } from '@/components/ui/tabs'
import type { BranchReviewView } from '@/features/panes/types/pane-content'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import { getReview } from '../api/review-api'
import { useReviewFilesSummary } from '../hooks/use-review-files-summary'
import { BranchReviewHeader } from './branch-review-header'
import { ReviewConversationView } from './review-conversation-view'
import { ReviewDiffTab, type LineRequest } from './review-diff-tab'

interface BranchReviewPaneProps {
  view?: BranchReviewView
  onViewChange: (view: BranchReviewView) => void
  wsId: string
  isActivePane?: boolean
}

export function BranchReviewPane({
  view = 'conversation',
  onViewChange,
  wsId,
  isActivePane,
}: BranchReviewPaneProps) {
  const store = useWorkspaceStore()
  const [toolbarHost, setToolbarHost] = useState<HTMLElement | null>(null)
  const owningChatId = useSyncExternalStore(
    (onChange) => subscribeToWorkspaceScope(wsId, onChange),
    () => getOwningChatId(wsId),
  )

  const branchHeader = useSelect(
    useShallow((s): { title: string; branch?: string; baseBranch?: string } => {
      const ws = selectWorkspace(s, wsId)
      if (ws) {
        const chatTitle = ws.owningChatId ? selectAgentChat(s, ws.owningChatId)?.title : ''
        return {
          title: chatTitle || ws.branch || 'Branch Review',
          branch: ws.branch,
          baseBranch: ws.parentBranch,
        }
      }
      return { title: 'Branch Review' }
    }),
  )
  const threadCount = useReviewThreads(wsId).length
  const { files, loaded: filesLoaded } = useReviewFilesSummary(wsId)
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
    if (next === 'conversation') setLineRequest(undefined)
    onViewChange(next)
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

  const load = useCallback(async () => {
    const actions = store.getState()
    actions.setBranchReviewDiffStatus('loading')
    try {
      const review = await getReview(wsId)
      const a = store.getState()
      a.setBranchReviewDescription(review.description)
      a.setBranchReviewMergeStrategy(review.mergeStrategy)
      a.setBranchReviewConversations(review.conversations)
    } catch {
      store.getState().setBranchReviewDiffStatus('error')
    }
  }, [store, wsId])

  const hostResolved = useHostResolved({ wsId })

  useEffect(() => {
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
