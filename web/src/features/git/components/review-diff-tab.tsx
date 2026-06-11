import { lazy, Suspense } from 'react'
import { FileDashed, Warning } from '@phosphor-icons/react'
import { LoadingSpinner } from '@/components/ui/loading-spinner'
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'

const MultiFileDiffViewer = lazy(() => import('./diff/git-diff-multi-file'))

interface ReviewDiffTabProps {
  onRetry: () => void
}

function CenteredState({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 text-center text-muted-foreground">
      {children}
    </div>
  )
}

export function ReviewDiffTab({ onRetry }: ReviewDiffTabProps) {
  const diff = useWorkspaceStoreContext((s) => s.branchReview.diffCache)
  const status = useWorkspaceStoreContext((s) => s.branchReview.diffStatus)

  if (status === 'loading' || status === 'idle') {
    return (
      <CenteredState>
        <LoadingSpinner label="Loading branch diff" showLabel />
      </CenteredState>
    )
  }

  if (status === 'error') {
    return (
      <CenteredState>
        <Warning className="size-6" />
        <span className="ui-text-sm">Failed to load the branch diff.</span>
        <button
          type="button"
          onClick={onRetry}
          className="ui-text-sm text-accent underline-offset-4 hover:underline"
        >
          Retry
        </button>
      </CenteredState>
    )
  }

  if (!diff || diff.files.length === 0) {
    return (
      <CenteredState>
        <FileDashed className="size-6" />
        <span className="ui-text-sm">No changes between this branch and its parent.</span>
      </CenteredState>
    )
  }

  return (
    <Suspense fallback={<CenteredState>{<LoadingSpinner />}</CenteredState>}>
      <MultiFileDiffViewer multiDiff={diff} onClose={() => {}} />
    </Suspense>
  )
}
