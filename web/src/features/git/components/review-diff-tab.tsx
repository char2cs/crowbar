import { lazy, Suspense, useCallback, useEffect, useRef, useState } from 'react'
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
  /** Scopes the whole surface to ONE commit instead of the workspace's branch. */
  commit?: string
  /** Shown when the scoped diff has no files. Defaults to the branch wording. */
  emptyMessage?: string
}

function CenteredState({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 text-center text-muted-foreground">
      {children}
    </div>
  )
}

/**
 * The windowed diff surface.
 *
 * Its default scope is a child workspace's whole branch against its parent —
 * the GitHub-like review step. Passing `commit` narrows every read to one
 * commit, which is how history is read: the same layout, the same per-file
 * fetching, the same find-in-diff, against a different pair of trees. That
 * parameter is the whole reason there is no second diff renderer in this app
 * any more; the Monaco stack that used to serve commit diffs is gone.
 *
 * Fed by the files summary (one row per changed file, O(files)) and the outline
 * (hunk geometry, O(hunks)) — never by the old `/review` composite, which
 * carried one JSON object per diff line. On a 406-file / 1M-line branch that
 * composite was 158MB and 1,441,452 objects, cost 1,162MB of webview memory,
 * and still retained 544MB after the tab was closed. Patches are now fetched
 * per file as the viewport reaches them, so what this holds is bounded by the
 * window rather than by the size of the diff.
 */
export function ReviewDiffTab({
  onRetry,
  branchHeader,
  isActivePane,
  commit,
  emptyMessage,
}: ReviewDiffTabProps) {
  const wsId = useWorkspaceStoreContext((s) => s.workspaceId)
  const { files, loaded: filesLoaded } = useReviewFilesSummary(wsId ?? null, commit)
  const { outline } = useReviewOutline(wsId ?? null, commit)
  const [searchOpen, setSearchOpen] = useState(false)
  const surfaceRef = useRef<ReviewCodeViewHandle | null>(null)
  // Mirrored into state as well as a ref: the reveal below has to run WHEN the
  // handle appears, and a ref assignment does not re-render. The surface is
  // lazy, so on the path that matters most — clicking a file while the review
  // tab is still closed — the request is raised before the handle exists.
  const [surface, setSurface] = useState<ReviewCodeViewHandle | null>(null)
  const attachSurface = useCallback((handle: ReviewCodeViewHandle | null) => {
    surfaceRef.current = handle
    setSurface(handle)
  }, [])

  const handleSelectHit = useCallback((hit: SearchHit) => {
    surfaceRef.current?.revealLine(hit.path, hit.lineNumber, hit.side)
  }, [])

  // The sidebar's changed-files tree asks for a file by path; the surface is
  // the only thing that can honour it (the target may not be materialised, and
  // scrolling to a placeholder lands near the file rather than at it).
  //
  // The request carries a NONCE because clicking the same file twice must
  // reveal it again, and a path compared to itself never re-runs an effect.
  // Gated on the handle rather than fired blind: a click that OPENS this tab
  // raises the request before the lazy surface has mounted, so the reveal has
  // to wait for the handle and then honour whatever is outstanding. The
  // last-consumed nonce is what stops it re-firing on unrelated re-renders.
  //
  // Commit tabs opt out — the request comes from the branch review's own file
  // list and may name a path a commit does not contain.
  const revealPath = useWorkspaceStoreContext((s) => s.branchReview.revealFilePath)
  const revealNonce = useWorkspaceStoreContext((s) => s.branchReview.revealFileNonce)
  const consumedNonce = useRef(0)
  useEffect(() => {
    if (commit || !surface || !revealPath) return
    if (consumedNonce.current === revealNonce) return
    consumedNonce.current = revealNonce
    surface.revealFile(revealPath)
  }, [surface, revealNonce, revealPath, commit])

  // The summary gates first paint, not the outline. The file list is renderable
  // without geometry — heights are estimated from ± counts until the outline
  // lands — so blocking on it would hold the pane for no visual gain.
  if (!filesLoaded) {
    return (
      <CenteredState>
        <LoadingSpinner label={commit ? 'Loading commit diff' : 'Loading branch diff'} showLabel />
      </CenteredState>
    )
  }

  if (files.length === 0) {
    return (
      <CenteredState>
        <FileDashed className="size-6" />
        <span className="ui-text-sm">
          {emptyMessage ?? 'No changes between this branch and its parent.'}
        </span>
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
            commit={commit}
            onSelectHit={handleSelectHit}
            onClose={() => setSearchOpen(false)}
          />
        </Suspense>
      )}

      <div className="min-h-0 flex-1">
        <Suspense fallback={<CenteredState>{<LoadingSpinner />}</CenteredState>}>
          <ReviewCodeViewLazy
            wsId={wsId ?? ''}
            commit={commit}
            files={files}
            outline={outline}
            isActivePane={isActivePane}
            surfaceRef={attachSurface}
          />
        </Suspense>
      </div>
    </div>
  )
}
