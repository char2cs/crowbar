import { useCallback, useEffect } from 'react'
import { GitPullRequest } from '@phosphor-icons/react'
import { Tabs, TabsList, TabsTab, TabsPanel } from '@/components/ui/tabs'
import {
  useWorkspaceStore,
  useWorkspaceStoreContext,
} from '@/features/workspace/stores/workspace-context'
import type { BranchReviewState } from '@/features/workspace/stores/slices/branch-review-slice'
import { getReview } from '../api/review-api'
import { ReviewAboutTab } from './review-about-tab'
import { ReviewDiffTab } from './review-diff-tab'
import { ReviewThreadPanel } from './review-thread-panel'

interface BranchReviewPaneProps {
  wsId: string
}

type Subtab = BranchReviewState['activeSubtab']

const SUBTABS: { value: Subtab; label: string }[] = [
  { value: 'about', label: 'About' },
  { value: 'git', label: 'Git' },
  { value: 'diff', label: 'Diff' },
]

export function BranchReviewPane({ wsId }: BranchReviewPaneProps) {
  const activeSubtab = useWorkspaceStoreContext((s) => s.branchReview.activeSubtab)
  const diffStatus = useWorkspaceStoreContext((s) => s.branchReview.diffStatus)
  const store = useWorkspaceStore()

  // Load the composite review read model + branch diff on mount. The backend
  // folds the branch-vs-parent diff into the same /review payload, so one fetch
  // hydrates description, merge strategy, threads, conversations, and the diff.
  const load = useCallback(async () => {
    const actions = store.getState()
    actions.setBranchReviewDiffStatus('loading')
    try {
      const review = await getReview(wsId)
      const a = store.getState()
      a.setBranchReviewDescription(review.description)
      a.setBranchReviewMergeStrategy(review.mergeStrategy)
      a.setBranchReviewConversations(review.conversations)
      // Replace threads wholesale with the server set.
      for (const t of a.branchReview.threads) a.removeReviewThread(t.id)
      for (const t of review.threads) a.addReviewThread(t)
      a.setBranchReviewDiff(review.diff)
    } catch {
      store.getState().setBranchReviewDiffStatus('error')
    }
  }, [store, wsId])

  useEffect(() => {
    void load()
  }, [load])

  const isLoading = diffStatus === 'loading' || diffStatus === 'idle'

  return (
    <div className="flex h-full flex-col overflow-hidden bg-background">
      <div className="flex shrink-0 items-center gap-2 border-border border-b px-3 py-2">
        <GitPullRequest className="size-4 text-muted-foreground" />
        <span className="ui-text-sm font-medium text-foreground">Branch Review</span>
      </div>

      <Tabs
        value={activeSubtab}
        onValueChange={(value) => store.getState().setBranchReviewSubtab(value as Subtab)}
        className="flex flex-1 flex-col overflow-hidden"
      >
        <div className="flex shrink-0 items-center px-2 py-1.5">
          <TabsList variant="default" className="w-full">
            {SUBTABS.map((tab) => (
              <TabsTab key={tab.value} value={tab.value} className="flex-1 justify-center">
                {tab.label}
              </TabsTab>
            ))}
          </TabsList>
        </div>

        <TabsPanel value="about" className="flex-1 overflow-hidden">
          <ReviewAboutTab wsId={wsId} isLoading={isLoading} />
        </TabsPanel>

        <TabsPanel value="git" className="flex-1 overflow-hidden">
          <ReviewThreadPanel wsId={wsId} />
        </TabsPanel>

        <TabsPanel value="diff" className="flex-1 overflow-hidden">
          <ReviewDiffTab onRetry={() => void load()} />
        </TabsPanel>
      </Tabs>
    </div>
  )
}
