import { lazy, Suspense, useCallback, useRef, useState } from 'react'
import { FileDashed, MagnifyingGlass } from '@phosphor-icons/react'
import { LoadingSpinner } from '@/components/ui/loading-spinner'
import { Button } from '@/components/ui/button'
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { useReviewFilesSummary } from '@/features/git/hooks/use-review-files-summary'
import { useReviewOutline } from '@/features/git/hooks/use-review-outline'
import type { SearchHit } from '@/features/git/api/review-window-api'
import type { ReviewCodeViewHandle } from './diff/review-code-view'

const ReviewCodeViewLazy = lazy(() =>
  import('./diff/review-code-view').then((m) => ({ default: m.ReviewCodeView })),
)
const ReviewSearchBarLazy = lazy(() =>
  import('./diff/review-search-bar').then((m) => ({ default: m.ReviewSearchBar })),
)

interface ReviewDiffTabProps {
  onRetry: () => void
  /** Branch-review header data (branch name + base) for the shared diff header. */
  branchHeader?: { title: string; baseBranch?: string }
  isActivePane?: boolean
}

function CenteredState({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 text-center text-muted-foreground">
      {children}
    </div>
  )
}

/**
 * The branch-review surface: a child workspace's whole branch against its
 * parent, the GitHub-like review step.
 *
 * Fed by the files summary (one row per changed file, O(files)) and the outline
 * (hunk geometry, O(hunks)) — never by the old `/review` composite, which
 * carried one JSON object per diff line. On a 406-file / 1M-line branch that
 * composite was 158MB and 1,441,452 objects, cost 1,162MB of webview memory,
 * and still retained 544MB after the tab was closed. Patches are now fetched
 * per file as the viewport reaches them, so what this holds is bounded by the
 * window rather than by the size of the branch.
 */
export function ReviewDiffTab({ onRetry, branchHeader, isActivePane }: ReviewDiffTabProps) {
  const wsId = useWorkspaceStoreContext((s) => s.workspaceId)
  const { files, loaded: filesLoaded } = useReviewFilesSummary(wsId ?? null)
  const { outline } = useReviewOutline(wsId ?? null)
  const [searchOpen, setSearchOpen] = useState(false)
  const surfaceRef = useRef<ReviewCodeViewHandle | null>(null)

  const handleSelectHit = useCallback((hit: SearchHit) => {
    surfaceRef.current?.revealLine(hit.path, hit.lineNumber, hit.side)
  }, [])

  // The summary gates first paint, not the outline. The file list is renderable
  // without geometry — heights are estimated from ± counts until the outline
  // lands — so blocking on it would hold the pane for no visual gain.
  if (!filesLoaded) {
    return (
      <CenteredState>
        <LoadingSpinner label="Loading branch diff" showLabel />
      </CenteredState>
    )
  }

  if (files.length === 0) {
    return (
      <CenteredState>
        <FileDashed className="size-6" />
        <span className="ui-text-sm">No changes between this branch and its parent.</span>
        <button
          type="button"
          onClick={onRetry}
          className="ui-text-sm text-secondary underline-offset-4 hover:underline"
        >
          Refresh
        </button>
      </CenteredState>
    )
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-background">
      <div className="flex shrink-0 items-center gap-2 border-border border-b px-3 py-1.5">
        <span className="ui-text-sm min-w-0 truncate font-medium">
          {branchHeader?.title ?? 'Branch Review'}
        </span>
        {branchHeader?.baseBranch && (
          <span className="ui-text-sm shrink-0 text-muted-foreground">
            → {branchHeader.baseBranch}
          </span>
        )}
        <span className="ui-text-sm ml-auto shrink-0 text-muted-foreground">
          {files.length} changed {files.length === 1 ? 'file' : 'files'}
        </span>
        <Button
          variant="ghost"
          size="icon-sm"
          className="shrink-0"
          onClick={() => setSearchOpen((v) => !v)}
          aria-label="Find in diff"
          aria-pressed={searchOpen}
          title="Find in diff"
        >
          <MagnifyingGlass />
        </Button>
      </div>

      {searchOpen && wsId && (
        <Suspense fallback={null}>
          <ReviewSearchBarLazy
            wsId={wsId}
            onSelectHit={handleSelectHit}
            onClose={() => setSearchOpen(false)}
          />
        </Suspense>
      )}

      <div className="min-h-0 flex-1">
        <Suspense fallback={<CenteredState>{<LoadingSpinner />}</CenteredState>}>
          <ReviewCodeViewLazy
            wsId={wsId ?? ''}
            files={files}
            outline={outline}
            isActivePane={isActivePane}
            surfaceRef={surfaceRef}
          />
        </Suspense>
      </div>
    </div>
  )
}
