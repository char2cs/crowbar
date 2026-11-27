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
  wsId: string
  title?: string
  lineRequest?: LineRequest
  isActivePane?: boolean
  commit?: string
  emptyMessage?: string
  toolbarHost?: HTMLElement | null
}

function CenteredState({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 text-center text-muted-foreground">
      {children}
    </div>
  )
}

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
  const diffStyle = useSettingsStore((s) => s.settings.diffViewMode)
  const updateSetting = useSettingsStore((s) => s.updateSetting)
  const surfaceRef = useRef<ReviewCodeViewHandle | null>(null)
  const [surface, setSurface] = useState<ReviewCodeViewHandle | null>(null)
  const attachSurface = useCallback((handle: ReviewCodeViewHandle | null) => {
    surfaceRef.current = handle
    setSurface(handle)
  }, [])

  const handleSelectHit = useCallback((hit: SearchHit) => {
    surfaceRef.current?.revealLine(hit.path, hit.lineNumber, hit.side)
  }, [])

  const revealPath = useWorkspaceStoreContext((s) => s.branchReview.revealFilePath)
  const revealNonce = useWorkspaceStoreContext((s) => s.branchReview.revealFileNonce)
  const consumedNonce = useRef(0)
  useEffect(() => {
    if (commit || !surface || !revealPath) return
    if (consumedNonce.current === revealNonce) return
    consumedNonce.current = revealNonce
    surface.revealFile(revealPath)
  }, [surface, revealNonce, revealPath, commit])

  const consumedLineNonce = useRef(0)
  useEffect(() => {
    if (!surface || !lineRequest || consumedLineNonce.current === lineRequest.nonce) return
    consumedLineNonce.current = lineRequest.nonce
    surface.revealLine(lineRequest.path, lineRequest.lineNumber, lineRequest.side)
  }, [surface, lineRequest])

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
