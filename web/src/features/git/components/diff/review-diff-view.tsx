import { useVirtualizer } from '@tanstack/react-virtual'
import {
  CaretDown as ChevronDown,
  CaretRight as ChevronRight,
  FileText,
  FileDashed,
  Eye,
  EyeSlash,
} from '@phosphor-icons/react'
import {
  memo,
  useCallback,
  useMemo,
  useRef,
  useState,
  useEffect,
} from 'react'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/utils/cn'
import type { FileDiffSummary, MultiFileDiff } from '../../types/git-diff-types'
import type { GitDiff } from '../../types/git-types'
import { getFileStatus } from '../../utils/git-diff-helpers'
import ImageDiffViewer from './git-diff-image'
import TextDiffViewer from './git-diff-text'
import {
  useWorkspaceStoreContext,
} from '@/features/workspace/stores/workspace-context'

const LARGE_DIFF_THRESHOLD = 500

interface FileDiffRowProps {
  diff: GitDiff
  summary: FileDiffSummary
  viewMode: 'unified' | 'split'
  showWhitespace: boolean
  forceExpand?: boolean
}

const FileDiffRow = memo(
  ({
    diff,
    summary,
    viewMode,
    showWhitespace,
    forceExpand,
  }: FileDiffRowProps) => {
    const [isViewed, setIsViewed] = useState(false)
    const [isExpanded, setIsExpanded] = useState(!summary.shouldAutoCollapse)

    useEffect(() => {
      if (forceExpand) {
        setIsExpanded(true)
        setIsViewed(false)
      }
    }, [forceExpand])

    const statusColors: Record<string, string> = {
      added: 'text-git-added',
      deleted: 'text-git-deleted',
      modified: 'text-git-modified',
      renamed: 'text-git-renamed',
    }

    return (
      <div className="border-border border-b last:border-b-0">
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
              <TextDiffViewer
                diff={diff}
                isStaged={false}
                viewMode={viewMode}
                showWhitespace={showWhitespace}
                isInMultiFileView
                isEmbeddedInScrollView
              />
            )}
          </div>
        )}
      </div>
    )
  },
)

FileDiffRow.displayName = 'FileDiffRow'

export interface ReviewDiffViewProps {
  multiDiff: MultiFileDiff
}

export const ReviewDiffView = memo(({ multiDiff }: ReviewDiffViewProps) => {
  const files = multiDiff?.files ?? []

  const [viewMode, setViewMode] = useState<'unified' | 'split'>('unified')
  const [showWhitespace, setShowWhitespace] = useState(false)

  const activeFileKey = useWorkspaceStoreContext((s) => s.branchReview.activeFileKey)
  const activeFileNonce = useWorkspaceStoreContext((s) => s.branchReview.activeFileNonce)

  const fileSummaries: FileDiffSummary[] = useMemo(() => {
    return files.map((diff, index) => {
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
  }, [multiDiff.fileKeys, files])

  const [expandedByScrollKey, setExpandedByScrollKey] = useState<string | null>(null)

  const scrollRef = useRef<HTMLDivElement>(null)

  const virtualizer = useVirtualizer({
    count: files.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: (index) => {
      const summary = fileSummaries[index]
      if (!summary) return 36
      const lineCount = files[index].lines.length
      return 36 + lineCount * 22
    },
    overscan: 3,
    measureElement: (el) => el.getBoundingClientRect().height,
  })

  const handleScrollToFile = useCallback(() => {
    if (!activeFileKey || activeFileNonce === 0) return
    const index = fileSummaries.findIndex((s) => s.key === activeFileKey)
    if (index === -1) return
    setExpandedByScrollKey(activeFileKey)
    virtualizer.scrollToIndex(index, { align: 'start' })
  }, [activeFileKey, activeFileNonce, fileSummaries, virtualizer])

  useEffect(() => {
    handleScrollToFile()
  }, [activeFileNonce]) // eslint-disable-line react-hooks/exhaustive-deps

  if (files.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 text-center text-muted-foreground">
        <FileDashed className="size-6" />
        <span className="ui-text-sm">No changes to show.</span>
      </div>
    )
  }

  const totalAdditions = fileSummaries.reduce((acc, s) => acc + s.additions, 0)
  const totalDeletions = fileSummaries.reduce((acc, s) => acc + s.deletions, 0)

  return (
    <div className="flex h-full flex-col overflow-hidden bg-background">
      <div className="flex items-center justify-between border-border border-b bg-background px-3 py-1.5">
        <div className="flex items-center gap-2">
          <span className="ui-text-sm font-medium text-foreground">
            {multiDiff.title ?? 'Review'}
          </span>
          <span className="ui-text-xs text-muted-foreground">
            {files.length} {files.length === 1 ? 'file' : 'files'}
          </span>
          <span className="ui-text-xs text-git-added">+{totalAdditions}</span>
          <span className="ui-text-xs text-git-deleted">-{totalDeletions}</span>
        </div>
        <div className="flex items-center gap-1">
          <button
            className={cn(
              'rounded px-2 py-0.5 ui-text-xs transition-colors',
              showWhitespace
                ? 'bg-accent text-accent-foreground'
                : 'text-muted-foreground hover:text-foreground',
            )}
            onClick={() => setShowWhitespace((v) => !v)}
          >
            Whitespace
          </button>
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

      <div ref={scrollRef} className="flex-1 overflow-y-auto overflow-x-hidden">
        <div
          style={{
            height: `${virtualizer.getTotalSize()}px`,
            position: 'relative',
          }}
        >
          {virtualizer.getVirtualItems().map((virtualItem) => {
            const diff = files[virtualItem.index]
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
                  showWhitespace={showWhitespace}
                  forceExpand={expandedByScrollKey === summary.key}
                />
              </div>
            )
          })}
        </div>
      </div>

      <div className="flex items-center justify-between border-border border-t bg-background px-3 py-1 ui-text-xs text-muted-foreground">
        <span>{files.length} {files.length !== 1 ? 'files' : 'file'} changed</span>
        <div className="flex items-center gap-2">
          <span className="text-git-added">+{totalAdditions}</span>
          <span className="text-git-deleted">-{totalDeletions}</span>
        </div>
      </div>
    </div>
  )
})

ReviewDiffView.displayName = 'ReviewDiffView'

export default ReviewDiffView
