import { lazy, Suspense, useCallback, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Columns, FileDashed, MagnifyingGlass, Rows } from '@phosphor-icons/react'
import { LoadingSpinner } from '@/components/ui/loading-spinner'
import { Button } from '@/components/ui/button'
import { Toggle, ToggleGroup } from '@/components/ui/toggle-group'
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { useReviewFilesSummary } from '@/features/git/hooks/use-review-files-summary'
import { useReviewOutline } from '@/features/git/hooks/use-review-outline'
import { useSettingsStore } from '@/features/settings/store'
import type { SearchHit } from '@/features/git/api/review-window-api'
import type { ReviewCodeViewHandle } from './diff/review-code-view'

const ReviewCodeViewLazy = lazy(() =>
  import('./diff/review-code-view').then((m) => ({ default: m.ReviewCodeView })),
)
const ReviewSearchBarLazy = lazy(() =>
  import('./diff/review-search-bar').then((m) => ({ default: m.ReviewSearchBar })),
)

export interface LineRequest {
  path: string
  lineNumber: number
  side: 'old' | 'new'
  nonce: number
}

interface ReviewDiffTabProps {
  onRetry: () => void
  /** This diff's workspace (from its buffer), never the ambient context: every retained
   *  WorkspaceView renders the pane tree under its own context, so the ambient one would fetch
   *  another workspace's diff. */
  wsId: string
  /** Heading for a surface the Branch Review shell does not wrap (a commit tab). */
  title?: string
  /** A thread's line to scroll to; the nonce makes asking for the same line twice work. */
  lineRequest?: LineRequest
  isActivePane?: boolean
  /** Scopes the whole surface to ONE commit instead of the workspace's branch. */
  commit?: string
  /** Shown when the scoped diff has no files. Defaults to the branch wording. */
  emptyMessage?: string
  /** Renders the view toggle and find button here (a tab row) instead of in a toolbar row of its own. */
  toolbarHost?: HTMLElement | null
}

function CenteredState({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 text-center text-muted-foreground">
      {children}
    </div>
  )
}

/** The windowed diff surface: a branch against its parent, or with `commit` one commit — so the
 *  app has one diff renderer. Fed by the files summary (O(files)) and outline (O(hunks)), patches
 *  fetched per file, so memory is bounded by the window, not the diff. */
export function ReviewDiffTab({
  onRetry,
  wsId,
  title,
  lineRequest,
  isActivePane,
  commit,
  emptyMessage,
  toolbarHost,
}: ReviewDiffTabProps) {
  const { files, loaded: filesLoaded } = useReviewFilesSummary(wsId, commit)
  const { outline } = useReviewOutline(wsId, commit)
  const [searchOpen, setSearchOpen] = useState(false)
  // A display preference, not per-tab state, so it lives in settings and outlives the tab.
  const diffStyle = useSettingsStore((s) => s.settings.diffViewMode)
  const updateSetting = useSettingsStore((s) => s.updateSetting)
  const surfaceRef = useRef<ReviewCodeViewHandle | null>(null)
  // Also state, not just a ref: the lazy surface's handle can appear after a reveal is
  // requested (a file click that opens this tab), and the reveal must run when it does.
  const [surface, setSurface] = useState<ReviewCodeViewHandle | null>(null)
  const attachSurface = useCallback((handle: ReviewCodeViewHandle | null) => {
    surfaceRef.current = handle
    setSurface(handle)
  }, [])

  const handleSelectHit = useCallback((hit: SearchHit) => {
    surfaceRef.current?.revealLine(hit.path, hit.lineNumber, hit.side)
  }, [])

  // The sidebar's file reveal, honoured once the handle exists. The nonce lets the same file be
  // revealed twice; the consumed nonce stops re-firing on re-renders. Commit tabs skip it: the
  // path comes from the branch's file list.
  const revealPath = useWorkspaceStoreContext((s) => s.branchReview.revealFilePath)
  const revealNonce = useWorkspaceStoreContext((s) => s.branchReview.revealFileNonce)
  const consumedNonce = useRef(0)
  useEffect(() => {
    if (commit || !surface || !revealPath) return
    if (consumedNonce.current === revealNonce) return
    consumedNonce.current = revealNonce
    surface.revealFile(revealPath)
  }, [surface, revealNonce, revealPath, commit])

  // Same wait-for-the-handle rule as the file reveal above: the Conversation's
  // "Jump to line" mounts this surface and asks in the same breath.
  const consumedLineNonce = useRef(0)
  useEffect(() => {
    if (!surface || !lineRequest || consumedLineNonce.current === lineRequest.nonce) return
    consumedLineNonce.current = lineRequest.nonce
    surface.revealLine(lineRequest.path, lineRequest.lineNumber, lineRequest.side)
  }, [surface, lineRequest])

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

  const controls = (
    <div className="flex shrink-0 items-center gap-1">
      <ToggleGroup
        value={[diffStyle]}
        onValueChange={(next) => {
          const mode = next[0]
          if (mode === 'split' || mode === 'unified') updateSetting('diffViewMode', mode)
        }}
        aria-label="Diff layout"
      >
        <Toggle value="split" aria-label="Side-by-side diff" title="Side-by-side">
          <Columns />
        </Toggle>
        <Toggle value="unified" aria-label="Inline diff" title="Inline">
          <Rows />
        </Toggle>
      </ToggleGroup>
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={() => setSearchOpen((v) => !v)}
        aria-label="Find in diff"
        aria-pressed={searchOpen}
        title="Find in diff"
      >
        <MagnifyingGlass />
      </Button>
    </div>
  )

  return (
    <div
      className="flex h-full min-h-0 flex-col bg-background"
      onKeyDown={(e) => {
        if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'f') {
          e.preventDefault()
          setSearchOpen(true)
        }
      }}
    >
      {toolbarHost !== undefined ? (
        toolbarHost && createPortal(controls, toolbarHost)
      ) : (
        <div className="flex shrink-0 items-center gap-2 border-border border-b px-3 py-1.5">
          {title && <span className="ui-text-sm min-w-0 truncate font-medium">{title}</span>}
          {commit && (
            <span className="ui-text-sm shrink-0 text-muted-foreground">
              {files.length} changed {files.length === 1 ? 'file' : 'files'}
            </span>
          )}
          <div className="ml-auto">{controls}</div>
        </div>
      )}

      {searchOpen && (
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
            wsId={wsId}
            commit={commit}
            files={files}
            outline={outline}
            isActivePane={isActivePane}
            surfaceRef={attachSurface}
            diffStyle={diffStyle}
          />
        </Suspense>
      </div>
    </div>
  )
}
