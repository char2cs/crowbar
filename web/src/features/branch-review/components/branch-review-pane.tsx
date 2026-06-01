import { useEffect } from 'react'
import { GitBranch } from '@phosphor-icons/react'
import { useWorkspaceStoreContext, useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { getRefDiff } from '@/features/git/api/git-diff-api'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
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
  const diffStatus = useWorkspaceStoreContext(s => s.branchReview.diffStatus)
  const threads = useWorkspaceStoreContext(s => s.branchReview.threads)

  useEffect(() => {
    if (diffStatus !== 'idle') return
    const { setBranchReviewDiff, setBranchReviewDiffStatus } = store.getState()
    setBranchReviewDiffStatus('loading')
    getRefDiff(wsId, 'HEAD~1', 'HEAD')
      .then(diffs => {
        if (!diffs) { setBranchReviewDiffStatus('error'); return }
        const multiDiff: MultiFileDiff = {
          commitHash: branchName,
          files: diffs,
          totalFiles: diffs.length,
          totalAdditions: diffs.reduce((n, d) => n + (d.additions ?? 0), 0),
          totalDeletions: diffs.reduce((n, d) => n + (d.deletions ?? 0), 0),
        }
        setBranchReviewDiff(multiDiff)
      })
      .catch(() => setBranchReviewDiffStatus('error'))
  }, [diffStatus, wsId, branchName, store])

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

  const stats = [
    diffCache && diffCache.totalFiles > 0 && `${diffCache.totalFiles} file${diffCache.totalFiles !== 1 ? 's' : ''}`,
    diffCache && diffCache.totalAdditions > 0 && `+${diffCache.totalAdditions.toLocaleString()}`,
    diffCache && diffCache.totalDeletions > 0 && `-${diffCache.totalDeletions.toLocaleString()}`,
  ].filter(Boolean).join(' · ')

  return (
    <Frame className="h-full overflow-hidden rounded-none p-2">
      <Tabs
        value={activeSubtab}
        onValueChange={v => store.getState().setBranchReviewSubtab(v as typeof activeSubtab)}
        className="flex h-full flex-col overflow-hidden gap-0"
      >
        <FrameHeader className="shrink-0 gap-3 border-b border-border/60 pb-0">
          <div className="flex items-center justify-between">
            <div>
              <div className="flex items-center gap-2 text-sm font-semibold text-foreground">
                <GitBranch size={14} className="text-muted-foreground" />
                {branchName}
                <Badge variant="outline" className="text-xs font-normal text-muted-foreground">
                  → main
                </Badge>
              </div>
              {stats && <p className="mt-0.5 text-xs text-muted-foreground">{stats}</p>}
            </div>
            <MergeButton
              strategy={mergeStrategy}
              isLocked={false}
              hasConflicts={false}
              onMerge={() => {}}
              onStrategyChange={s => store.getState().setBranchReviewMergeStrategy(s)}
            />
          </div>

          <TabsList variant="underline" className="w-full justify-start gap-0 rounded-none border-0 bg-transparent px-0">
            <TabsTab value="about">About</TabsTab>
            <TabsTab value="commits">Commits</TabsTab>
            <TabsTab value="diff">Diff</TabsTab>
          </TabsList>
        </FrameHeader>

        <FramePanel className="min-h-0 flex-1 overflow-y-auto p-0">
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
            {diffStatus === 'loading' && (
              <p className="p-4 text-xs text-muted-foreground/50">Loading diff…</p>
            )}
            {diffStatus === 'error' && (
              <p className="p-4 text-xs text-destructive/70">Failed to load diff.</p>
            )}
            {diffStatus === 'loaded' && diffCache && (
              <BranchReviewDiffViewer
                multiDiff={diffCache}
                threads={threads}
                onAddThread={handleAddThread}
                onReply={handleReply}
                onResolve={handleResolve}
              />
            )}
            {diffStatus === 'loaded' && !diffCache && (
              <p className="p-4 text-xs text-muted-foreground/50">No diff available.</p>
            )}
          </TabsPanel>
        </FramePanel>
      </Tabs>
    </Frame>
  )
}
