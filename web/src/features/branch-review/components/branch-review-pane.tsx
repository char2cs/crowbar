import { useEffect } from 'react'
import { useWorkspaceStoreContext, useWorkspaceStore } from '@/features/workspace/stores/workspace-context'
import { getRefDiff } from '@/features/git/api/git-diff-api'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { ReviewThread, ReviewMessage } from '@/features/branch-review/types/review-types'
import { BranchReviewHeader } from './branch-review-header'
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

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <BranchReviewHeader
        branchName={branchName}
        parentBranch="main"
        isLocked={false}
        hasConflicts={false}
        aheadCount={diffCache?.totalFiles ?? 0}
        additions={diffCache?.totalAdditions ?? 0}
        deletions={diffCache?.totalDeletions ?? 0}
        mergeStrategy={mergeStrategy}
        onMerge={() => {}}
        onStrategyChange={s => store.getState().setBranchReviewMergeStrategy(s)}
      />

      <div className="flex shrink-0 gap-1 border-b border-border px-4">
        {(['about', 'commits', 'diff'] as const).map(tab => (
          <button
            key={tab}
            onClick={() => store.getState().setBranchReviewSubtab(tab)}
            className={
              'px-2 py-1.5 text-xs font-medium capitalize transition-colors ' +
              (activeSubtab === tab
                ? 'border-b-2 border-foreground text-foreground'
                : 'text-muted-foreground hover:text-foreground')
            }
          >
            {tab}
          </button>
        ))}
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {activeSubtab === 'about' && (
          <AboutTab
            description={description}
            onDescriptionChange={v => store.getState().setBranchReviewDescription(v)}
            onOpenConversation={handleOpenConversation}
          />
        )}
        {activeSubtab === 'commits' && <CommitsTab repoPath={wsId} />}
        {activeSubtab === 'diff' && (
          diffStatus === 'loading' ? (
            <p className="p-4 text-xs text-muted-foreground/50">Loading diff…</p>
          ) : diffStatus === 'error' ? (
            <p className="p-4 text-xs text-destructive/70">Failed to load diff.</p>
          ) : diffCache ? (
            <BranchReviewDiffViewer
              multiDiff={diffCache}
              threads={threads}
              onAddThread={handleAddThread}
              onReply={handleReply}
              onResolve={handleResolve}
            />
          ) : (
            <p className="p-4 text-xs text-muted-foreground/50">No diff available.</p>
          )
        )}
      </div>
    </div>
  )
}
