import { useEffect } from 'react'
import { useWorkspaceStoreContext, useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { useSidebarStore } from '@/lib/store/sidebar'
import { WorkspaceBranchIcon } from '@/components/layout/workspace-branch-icon'
import { getMockBranchDiff, getMockBranchReviewThreads, getMockBranchReviewDescription } from '@/lib/mock/branch-diff'
import type { ReviewThread, ReviewMessage } from '@/features/branch-review/types/review-types'
import { Frame, FrameHeader, FramePanel, FrameTitle, FrameDescription } from '@/components/ui/frame'
import { Tabs, TabsList, TabsTab, TabsPanel } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'
import { MergeButton } from './merge-button'
import { AboutTab } from './about-tab'
import { CommitsTab } from './commits-tab'
import { BranchReviewDiffViewer } from './branch-review-diff-viewer'

interface BranchReviewPaneProps {
  wsId: string
  branchName: string
}

export function BranchReviewPane({ wsId, branchName }: BranchReviewPaneProps) {
  const store = useWorkspaceStore()

  const description = useWorkspaceStoreContext(s => s.branchReview.description)
  const mergeStrategy = useWorkspaceStoreContext(s => s.branchReview.mergeStrategy)
  const activeSubtab = useWorkspaceStoreContext(s => s.branchReview.activeSubtab)
  const diffCache = useWorkspaceStoreContext(s => s.branchReview.diffCache)
  const threads = useWorkspaceStoreContext(s => s.branchReview.threads)

  const parentBranch = useSidebarStore(s => {
    const allWs = s.repos.flatMap(r => r.workspaces)
    const ws = allWs.find(w => w.id === wsId)
    if (!ws?.parentId) return null
    return allWs.find(w => w.id === ws.parentId)?.branch ?? null
  })

  const status = useSidebarStore(s => {
    const allWs = s.repos.flatMap(r => r.workspaces)
    return allWs.find(w => w.id === wsId)?.status ?? 'new'
  })

  useEffect(() => {
    const { branchReview, setBranchReviewDiff, setBranchReviewDescription } = store.getState()
    if (branchReview.diffStatus !== 'idle') return

    setBranchReviewDiff(getMockBranchDiff(wsId))

    // Seed mock threads and description if not already persisted
    if (branchReview.threads.length === 0) {
      const mockThreads = getMockBranchReviewThreads(wsId)
      mockThreads.forEach(t => store.getState().addReviewThread(t))
    }
    if (!branchReview.description) {
      setBranchReviewDescription(getMockBranchReviewDescription(wsId))
    }
  }, [wsId, store])

  function handleAddThread(filePath: string, lineNumber: number) {
    const thread: ReviewThread = {
      id: `thread-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      filePath,
      lineNumber,
      side: 'right',
      messages: [],
      isResolved: false,
    }
    store.getState().addReviewThread(thread)
  }

  function handleReply(threadId: string, body: string) {
    const message: ReviewMessage = {
      id: `msg-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      author: null,
      isAgent: false,
      body,
      createdAt: new Date().toISOString(),
    }
    store.getState().addReviewMessage(threadId, message)
  }

  function handleResolve(threadId: string) {
    store.getState().resolveReviewThread(threadId)
  }

  function handleOpenConversation(id: string) {
    store.getState().bufferActions.openContent({ type: 'crowbarChat', wsId: id, name: id })
  }


  return (
    // ONE Frame = the whole window. No horizontal/bottom gap; small top gap.
    <Frame className="h-full overflow-hidden rounded-none px-0 pb-0 pt-2">
      <Tabs
        value={activeSubtab}
        onValueChange={v => store.getState().setBranchReviewSubtab(v as typeof activeSubtab)}
        className="flex h-full flex-col overflow-hidden gap-0"
      >
        {/* FrameHeader — branch identity + stats + tabs navigation */}
        <FrameHeader className="shrink-0 flex-col gap-2 border-b border-border/50 pb-0">
          <div className="flex items-start justify-between gap-4">
            <div className="flex min-w-0 flex-col gap-0.5">
              <FrameTitle className="flex items-center gap-2 text-base">
                <WorkspaceBranchIcon status={status} />
                <span className="truncate">{branchName}</span>
                {parentBranch && (
                  <Badge variant="outline" className="shrink-0 text-xs font-normal text-muted-foreground">
                    → {parentBranch}
                  </Badge>
                )}
              </FrameTitle>
              {diffCache && (
                <FrameDescription className="flex items-center gap-1.5">
                  <span>{diffCache.totalFiles} file{diffCache.totalFiles !== 1 ? 's' : ''}</span>
                  <span className="text-muted-foreground/40">·</span>
                  <span className="text-git-added">+{diffCache.totalAdditions.toLocaleString()}</span>
                  <span className="text-git-deleted">−{diffCache.totalDeletions.toLocaleString()}</span>
                </FrameDescription>
              )}
            </div>
            <div className="shrink-0">
              <MergeButton
                strategy={mergeStrategy}
                isLocked={false}
                hasConflicts={false}
                onMerge={() => {}}
                onStrategyChange={s => store.getState().setBranchReviewMergeStrategy(s)}
              />
            </div>
          </div>
          <TabsList className="w-fit">
            <TabsTab value="about">About</TabsTab>
            <TabsTab value="commits">Commits</TabsTab>
            <TabsTab value="diff">Diff</TabsTab>
          </TabsList>
        </FrameHeader>

        {/* ONE FramePanel — the entire scrollable content body, full-bleed with a top gap */}
        <FramePanel className="mt-2 min-h-0 flex-1 overflow-y-auto rounded-none border-0 before:hidden">
          <TabsPanel value="about">
            <AboutTab
              wsId={wsId}
              description={description}
              onDescriptionChange={v => store.getState().setBranchReviewDescription(v)}
              onOpenConversation={handleOpenConversation}
            />
          </TabsPanel>

          <TabsPanel value="commits">
            <CommitsTab repoPath={wsId} />
          </TabsPanel>

          <TabsPanel value="diff" className="-m-5 overflow-hidden">
            {diffCache ? (
              <BranchReviewDiffViewer
                multiDiff={diffCache}
                threads={threads}
                onAddThread={handleAddThread}
                onReply={handleReply}
                onResolve={handleResolve}
              />
            ) : (
              <p className="p-5 text-xs text-muted-foreground/50">Loading diff…</p>
            )}
          </TabsPanel>
        </FramePanel>
      </Tabs>
    </Frame>
  )
}
