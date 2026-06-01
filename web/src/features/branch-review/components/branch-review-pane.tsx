import { useEffect } from 'react'
import { GitBranch } from '@phosphor-icons/react'
import { useWorkspaceStoreContext, useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { getMockBranchDiff } from '@/lib/mock/branch-diff'
import type { ReviewThread, ReviewMessage } from '@/features/branch-review/types/review-types'
import { Frame, FrameHeader, FramePanel } from '@/components/ui/frame'
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

  useEffect(() => {
    const { branchReview, setBranchReviewDiff } = store.getState()
    if (branchReview.diffStatus !== 'idle') return
    setBranchReviewDiff(getMockBranchDiff(wsId))
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
    <Frame className="h-full overflow-hidden rounded-none p-2">
      <FramePanel className="flex h-full flex-col overflow-hidden p-0">
        <Tabs
          value={activeSubtab}
          onValueChange={v => store.getState().setBranchReviewSubtab(v as typeof activeSubtab)}
          className="flex h-full flex-col overflow-hidden gap-0"
        >
          <FrameHeader className="shrink-0 gap-3 border-b border-border/50">
            <div className="flex items-center justify-between">
              <div>
                <div className="flex items-center gap-2 text-sm font-semibold text-foreground">
                  <GitBranch size={14} className="text-muted-foreground" />
                  {branchName}
                  <Badge variant="outline" className="text-xs font-normal text-muted-foreground">
                    → develop
                  </Badge>
                </div>
                {diffCache && (
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    {diffCache.totalFiles} file{diffCache.totalFiles !== 1 ? 's' : ''} · +{diffCache.totalAdditions} −{diffCache.totalDeletions}
                  </p>
                )}
              </div>
              <MergeButton
                strategy={mergeStrategy}
                isLocked={false}
                hasConflicts={false}
                onMerge={() => {}}
                onStrategyChange={s => store.getState().setBranchReviewMergeStrategy(s)}
              />
            </div>

            <TabsList className="w-fit">
              <TabsTab value="about">About</TabsTab>
              <TabsTab value="commits">Commits</TabsTab>
              <TabsTab value="diff">Diff</TabsTab>
            </TabsList>
          </FrameHeader>

          <div className="min-h-0 flex-1 overflow-y-auto">
            <TabsPanel value="about">
              <AboutTab
                description={description}
                onDescriptionChange={v => store.getState().setBranchReviewDescription(v)}
                onOpenConversation={handleOpenConversation}
              />
            </TabsPanel>

            <TabsPanel value="commits">
              <CommitsTab repoPath={wsId} />
            </TabsPanel>

            <TabsPanel value="diff">
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
          </div>
        </Tabs>
      </FramePanel>
    </Frame>
  )
}
