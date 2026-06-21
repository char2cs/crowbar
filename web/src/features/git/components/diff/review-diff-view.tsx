import { useVirtualizer } from '@tanstack/react-virtual'
import {
  CaretDown as ChevronDown,
  CaretRight as ChevronRight,
  FileText,
  Eye,
  EyeSlash,
} from '@phosphor-icons/react'
import { memo, useCallback, useMemo, useRef, useState, useEffect } from 'react'
import { Diff, Hunk } from 'react-diff-view'
import 'react-diff-view/style/index.css'
import type { HunkTokens } from 'react-diff-view'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/utils/cn'
import type { FileDiffSummary, MultiFileDiff } from '../../types/git-diff-types'
import type { GitDiff } from '../../types/git-types'
import { getFileStatus } from '../../utils/git-diff-helpers'
import { gitDiffToHunks } from '../../lib/to-diff-view-hunks'
import { buildDiffTokens, renderTreeSitterToken } from '../../lib/render-tree-sitter-token'
import ImageDiffViewer from './git-diff-image'
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'

const LARGE_DIFF_THRESHOLD = 500

// ─────────────────────────────────────────────────────────────────
// Per-file row
// ─────────────────────────────────────────────────────────────────

interface FileDiffRowProps {
  diff: GitDiff
  summary: FileDiffSummary
  viewMode: 'unified' | 'split'
  tokenCache: React.MutableRefObject<Map<string, HunkTokens | null | 'pending'>>
  onTokensResolved: (key: string, tokens: HunkTokens | null) => void
  forceExpand?: boolean
}

const FileDiffRow = memo(
  ({ diff, summary, viewMode, tokenCache, onTokensResolved, forceExpand }: FileDiffRowProps) => {
    const [isViewed, setIsViewed] = useState(false)
    const [isExpanded, setIsExpanded] = useState(!summary.shouldAutoCollapse)

    useEffect(() => {
      if (forceExpand) {
        setIsExpanded(true)
        setIsViewed(false)
      }
    }, [forceExpand])
    const [localTokens, setLocalTokens] = useState<HunkTokens | null | undefined>(
      () => {
        const cached = tokenCache.current.get(summary.key)
        if (cached === 'pending' || cached === undefined) return undefined
        return cached
      },
    )

    // Kick off tokenization when the row is expanded and tokens aren't cached yet
    useEffect(() => {
      if (!isExpanded || diff.is_binary || diff.is_image) return
      const existing = tokenCache.current.get(summary.key)
      if (existing !== undefined) {
        // Already resolved or pending
        if (existing !== 'pending') setLocalTokens(existing)
        return
      }
      // Mark as pending so we don't double-tokenize
      tokenCache.current.set(summary.key, 'pending')
      buildDiffTokens(diff).then((tokens) => {
        tokenCache.current.set(summary.key, tokens)
        onTokensResolved(summary.key, tokens)
        setLocalTokens(tokens)
      })
    }, [isExpanded, diff, summary.key, tokenCache, onTokensResolved])

    const hunks = useMemo(() => gitDiffToHunks(diff), [diff])

    const statusColors: Record<string, string> = {
      added: 'text-git-added',
      deleted: 'text-git-deleted',
      modified: 'text-git-modified',
      renamed: 'text-git-renamed',
    }

    return (
      <div className="border-border border-b last:border-b-0">
        {/* File header */}
        <div
          className={cn(
            'group flex cursor-pointer items-center gap-2 px-3 py-1',
            'bg-background ui-text-sm leading-5 hover:bg-muted',
          )}
          onClick={() => setIsExpanded((v) => !v)}
        >
          {isExpanded ? (
            <ChevronDown className="shrink-0 text-muted-foreground" />
          ) : (
            <ChevronRight className="shrink-0 text-muted-foreground" />
          )}

          <FileText className={cn('shrink-0', statusColors[summary.status])} />

          <span className="truncate font-medium text-foreground">{summary.filePath}</span>

          <div className="ml-auto flex shrink-0 items-center gap-2 ui-text-sm leading-none">
            {summary.uncommitted && (
              <Badge variant="warning" size="sm">
                uncommitted
              </Badge>
            )}
            {summary.additions > 0 && (
              <span className="text-git-added">+{summary.additions}</span>
            )}
            {summary.deletions > 0 && (
              <span className="text-git-deleted">-{summary.deletions}</span>
            )}
            {/* Viewed toggle */}
            <button
              className={cn(
                'flex items-center gap-1 rounded px-1.5 py-0.5 transition-colors',
                isViewed
                  ? 'bg-accent text-accent-foreground'
                  : 'text-muted-foreground hover:text-foreground',
              )}
              onClick={(e) => {
                e.stopPropagation()
                setIsViewed((v) => {
                  const next = !v
                  if (next) setIsExpanded(false)
                  return next
                })
              }}
              aria-label={isViewed ? 'Mark as unviewed' : 'Mark as viewed'}
            >
              {isViewed ? (
                <EyeSlash className="size-3.5" />
              ) : (
                <Eye className="size-3.5" />
              )}
              <span className="ui-text-xs">{isViewed ? 'Viewed' : 'View'}</span>
            </button>
          </div>
        </div>

        {/* Diff body — only when expanded and not viewed */}
        {isExpanded && !isViewed && (
          <div
            className="border-border border-t overflow-x-auto"
            style={{
              contentVisibility: 'auto',
              containIntrinsicHeight: `${diff.lines.length * 22}px`,
            }}
          >
            {diff.is_image ? (
              <ImageDiffViewer
                diff={diff}
                fileName={summary.fileName}
                onClose={() => {}}
              />
            ) : (
              <Diff
                viewType={viewMode === 'split' ? 'split' : 'unified'}
                diffType={
                  diff.is_new ? 'add' : diff.is_deleted ? 'delete' : diff.is_renamed ? 'rename' : 'modify'
                }
                hunks={hunks}
                tokens={localTokens ?? undefined}
                renderToken={renderTreeSitterToken}
              >
                {(renderedHunks) =>
                  renderedHunks.map((hunk) => (
                    <Hunk key={hunk.content} hunk={hunk} />
                  ))
                }
              </Diff>
            )}
          </div>
        )}
      </div>
    )
  },
)

FileDiffRow.displayName = 'FileDiffRow'

// ─────────────────────────────────────────────────────────────────
// ReviewDiffView
// ─────────────────────────────────────────────────────────────────

export interface ReviewDiffViewProps {
  multiDiff: MultiFileDiff
}

export const ReviewDiffView = memo(({ multiDiff }: ReviewDiffViewProps) => {
  const [viewMode, setViewMode] = useState<'unified' | 'split'>('unified')

  // Scroll-to-file: subscribe to activeFileKey + activeFileNonce from the workspace store.
  // This component renders inside the branch-review pane's provider, so useWorkspaceStoreContext is correct.
  const activeFileKey = useWorkspaceStoreContext((s) => s.branchReview.activeFileKey)
  const activeFileNonce = useWorkspaceStoreContext((s) => s.branchReview.activeFileNonce)

  // Token cache: fileKey → HunkTokens | null | 'pending'
  // We use a ref (not state) to avoid re-renders on every cache write.
  // setResolvedCount is just a counter to trigger re-render when tokens arrive.
  const tokenCache = useRef<Map<string, HunkTokens | null | 'pending'>>(new Map())
  const [, setResolvedCount] = useState(0)

  const handleTokensResolved = useCallback(() => {
    setResolvedCount((c) => c + 1)
  }, [])

  const fileSummaries: FileDiffSummary[] = useMemo(() => {
    return multiDiff.files.map((diff, index) => {
      let additions = 0
      let deletions = 0
      for (const line of diff.lines) {
        if (line.line_type === 'added') additions++
        else if (line.line_type === 'removed') deletions++
      }

      return {
        key: multiDiff.fileKeys?.[index] ?? `${diff.file_path}:${index}`,
        fileName: diff.file_path.split('/').pop() || diff.file_path,
        filePath: diff.file_path,
        status: getFileStatus(diff) as 'added' | 'deleted' | 'modified' | 'renamed',
        additions,
        deletions,
        shouldAutoCollapse: additions + deletions > LARGE_DIFF_THRESHOLD,
        uncommitted: diff.uncommitted ?? false,
      }
    })
  }, [multiDiff.fileKeys, multiDiff.files])

  // Track which file key should be force-expanded (set when scrolling to a file)
  const [expandedByScrollKey, setExpandedByScrollKey] = useState<string | null>(null)

  const scrollRef = useRef<HTMLDivElement>(null)

  const virtualizer = useVirtualizer({
    count: multiDiff.files.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: (index) => {
      const summary = fileSummaries[index]
      if (!summary) return 36
      const lineCount = multiDiff.files[index].lines.length
      return 36 + lineCount * 22
    },
    overscan: 3,
    measureElement: (el) => el.getBoundingClientRect().height,
  })

  // Scroll-to-file effect: when activeFileNonce changes (including re-clicks of the same key),
  // find the file by key, force-expand it, and scroll the virtualizer to it.
  useEffect(() => {
    if (!activeFileKey || activeFileNonce === 0) return
    const index = fileSummaries.findIndex((s) => s.key === activeFileKey)
    if (index === -1) return
    setExpandedByScrollKey(activeFileKey)
    virtualizer.scrollToIndex(index, { align: 'start' })
  }, [activeFileNonce]) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="flex h-full flex-col overflow-hidden bg-background">
      {/* Toolbar */}
      <div className="flex items-center justify-between border-border border-b bg-background px-3 py-1.5">
        <span className="ui-text-sm font-medium text-foreground">
          {multiDiff.title ?? 'Review'}
        </span>
        <div className="flex items-center gap-1">
          <button
            className={cn(
              'rounded px-2 py-0.5 ui-text-xs transition-colors',
              viewMode === 'unified'
                ? 'bg-accent text-accent-foreground'
                : 'text-muted-foreground hover:text-foreground',
            )}
            onClick={() => setViewMode('unified')}
          >
            Unified
          </button>
          <button
            className={cn(
              'rounded px-2 py-0.5 ui-text-xs transition-colors',
              viewMode === 'split'
                ? 'bg-accent text-accent-foreground'
                : 'text-muted-foreground hover:text-foreground',
            )}
            onClick={() => setViewMode('split')}
          >
            Split
          </button>
        </div>
      </div>

      {/* Virtualized file list */}
      <div ref={scrollRef} className="flex-1 overflow-y-auto overflow-x-hidden">
        <div
          style={{
            height: `${virtualizer.getTotalSize()}px`,
            position: 'relative',
          }}
        >
          {virtualizer.getVirtualItems().map((virtualItem) => {
            const diff = multiDiff.files[virtualItem.index]
            const summary = fileSummaries[virtualItem.index]
            if (!diff || !summary) return null
            return (
              <div
                key={summary.key}
                ref={virtualizer.measureElement}
                data-index={virtualItem.index}
                style={{
                  position: 'absolute',
                  top: 0,
                  left: 0,
                  width: '100%',
                  transform: `translateY(${virtualItem.start}px)`,
                }}
              >
                <FileDiffRow
                  diff={diff}
                  summary={summary}
                  viewMode={viewMode}
                  tokenCache={tokenCache}
                  onTokensResolved={handleTokensResolved}
                  forceExpand={expandedByScrollKey === summary.key}
                />
              </div>
            )
          })}
        </div>
      </div>

      {/* Footer */}
      <div className="flex items-center justify-between border-border border-t bg-background px-3 py-1 ui-text-xs text-muted-foreground">
        <span>{multiDiff.totalFiles} file{multiDiff.totalFiles !== 1 ? 's' : ''} changed</span>
        <div className="flex items-center gap-2">
          <span className="text-git-added">+{multiDiff.totalAdditions}</span>
          <span className="text-git-deleted">-{multiDiff.totalDeletions}</span>
        </div>
      </div>
    </div>
  )
})

ReviewDiffView.displayName = 'ReviewDiffView'

export default ReviewDiffView
