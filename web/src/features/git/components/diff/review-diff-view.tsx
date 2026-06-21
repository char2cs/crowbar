import { useVirtualizer } from '@tanstack/react-virtual'
import {
  CaretDown as ChevronDown,
  CaretRight as ChevronRight,
  FileText,
  Eye,
  EyeSlash,
  FileDashed,
} from '@phosphor-icons/react'
import {
  memo,
  useCallback,
  useMemo,
  useRef,
  useState,
  useEffect,
  type ReactNode,
} from 'react'
import { Diff, Hunk } from 'react-diff-view'
import 'react-diff-view/style/index.css'
import type { HunkTokens, ChangeData } from 'react-diff-view'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/utils/cn'
import type { FileDiffSummary, MultiFileDiff } from '../../types/git-diff-types'
import type { GitDiff } from '../../types/git-types'
import { getFileStatus } from '../../utils/git-diff-helpers'
import { gitDiffToHunks } from '../../lib/to-diff-view-hunks'
import { buildDiffTokens, renderTreeSitterToken } from '../../lib/render-tree-sitter-token'
import { changeKeyToAnchor, anchorToChangeKey } from '../../lib/diff-change-key'
import { CommentComposer } from '@/features/panes/components/comment-composer'
import { ReviewThreadItem } from '../review-thread-item'
import ImageDiffViewer from './git-diff-image'
import {
  useWorkspaceStoreContext,
  useWorkspaceStore,
} from '@/features/workspace/stores/workspace-context'
import { addThreadOptimistic } from '@/features/workspace/stores/slices/branch-review-slice'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import { openThread, replyToThread, setThreadResolved } from '../../api/review-api'
import { useCurrentIdentity } from '../../hooks/use-current-identity'

const LARGE_DIFF_THRESHOLD = 500

// ─────────────────────────────────────────────────────────────────
// Pending anchor (open composer state)
// ─────────────────────────────────────────────────────────────────

interface PendingAnchor {
  side: 'old' | 'new'
  line: number
  startLine: number
  endLine: number
}

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
  wsId: string
  threads: ReviewThread[]
}

const FileDiffRow = memo(
  ({
    diff,
    summary,
    viewMode,
    tokenCache,
    onTokensResolved,
    forceExpand,
    wsId,
    threads,
  }: FileDiffRowProps) => {
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

    // ── Inline comment state ────────────────────────────────────────
    const [pendingAnchor, setPendingAnchor] = useState<PendingAnchor | null>(null)
    const [rangeStart, setRangeStart] = useState<{ side: 'old' | 'new'; line: number } | null>(
      null,
    )

    const store = useWorkspaceStore()
    const identity = useCurrentIdentity(wsId)

    // Filter threads to this file
    const fileThreads = useMemo(
      () => threads.filter((t) => t.filePath === summary.filePath),
      [threads, summary.filePath],
    )

    // Build widgets record: changeKey → ReactNode
    const widgets = useMemo((): Record<string, ReactNode> => {
      const result: Record<string, ReactNode> = {}

      // Place thread widgets
      for (const thread of fileThreads) {
        const key = anchorToChangeKey(hunks, { side: thread.side, line: thread.lineNumber })
        const isOutdated = key === null

        // For outdated threads we place them at a synthetic key; they render "Outdated"
        const widgetKey = isOutdated ? `outdated-${thread.id}` : key!

        const handleReply = async (threadId: string, body: string) => {
          const updated = await replyToThread(wsId, threadId, { body })
          store.getState().upsertReviewThread(updated)
        }

        const handleResolve = async (threadId: string) => {
          const updated = await setThreadResolved(wsId, threadId, true)
          store.getState().upsertReviewThread(updated)
        }

        const handleReopen = async (threadId: string) => {
          const updated = await setThreadResolved(wsId, threadId, false)
          store.getState().upsertReviewThread(updated)
        }

        result[widgetKey] = (
          <ReviewThreadItem
            key={thread.id}
            thread={thread}
            wsId={wsId}
            isOutdated={isOutdated}
            onReply={handleReply}
            onResolve={handleResolve}
            onReopen={handleReopen}
          />
        )
      }

      // Place the open composer at the pending anchor
      if (pendingAnchor) {
        const composerKey = anchorToChangeKey(hunks, {
          side: pendingAnchor.side,
          line: pendingAnchor.line,
        })
        if (composerKey) {
          const lineLabel = `${pendingAnchor.side === 'new' ? 'R' : 'L'}${pendingAnchor.line}`
          result[composerKey] = (
            <div className="px-2 py-1.5">
              <CommentComposer
                title={`Add a comment on line ${lineLabel}`}
                autoFocus
                onSubmit={(body) => {
                  void handleOpenThread(body)
                }}
                onCancel={() => {
                  setPendingAnchor(null)
                  setRangeStart(null)
                }}
              />
            </div>
          )
        }
      }

      return result
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [fileThreads, hunks, pendingAnchor, wsId])

    const handleOpenThread = async (body: string) => {
      if (!pendingAnchor) return
      const tempId = `temp-${Date.now()}`
      const newThread: ReviewThread = {
        id: tempId,
        filePath: summary.filePath,
        lineNumber: pendingAnchor.line,
        startLine: pendingAnchor.startLine,
        endLine: pendingAnchor.endLine,
        side: pendingAnchor.side,
        messages: [
          {
            id: `msg-${tempId}`,
            author: identity?.login ?? identity?.displayName ?? null,
            isAgent: false,
            body,
            createdAt: new Date().toISOString(),
          },
        ],
        isResolved: false,
      }

      await addThreadOptimistic({
        apply: () => store.getState().upsertReviewThread(newThread),
        rollback: () => store.getState().removeReviewThread(tempId),
        commit: () =>
          openThread(wsId, {
            filePath: summary.filePath,
            line: pendingAnchor.line,
            startLine: pendingAnchor.startLine,
            endLine: pendingAnchor.endLine,
            side: pendingAnchor.side,
            author: identity?.login ?? identity?.displayName,
            isAgent: false,
            body,
          }).then((t) => store.getState().upsertReviewThread(t)),
      })

      setPendingAnchor(null)
      setRangeStart(null)
    }

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
                widgets={widgets}
                renderGutter={({ change, side, inHoverState }: {
                  change: ChangeData
                  side: 'old' | 'new'
                  inHoverState: boolean
                  renderDefault: () => ReactNode
                  wrapInAnchor: (element: ReactNode) => ReactNode
                }) => {
                  const anchor = changeKeyToAnchor(change)
                  // For normal changes, override side with the gutter's actual side
                  const effectiveSide =
                    change.type === 'normal' ? side : anchor.side
                  const effectiveLine =
                    change.type === 'normal'
                      ? side === 'old'
                        ? (change as { oldLineNumber: number }).oldLineNumber
                        : (change as { newLineNumber: number }).newLineNumber
                      : anchor.line

                  return (
                    <button
                      className={cn(
                        'diff-comment-gutter-btn',
                        'w-full h-full flex items-center justify-center',
                        'text-muted-foreground/40 hover:text-muted-foreground',
                        'opacity-0 transition-opacity',
                        inHoverState && 'opacity-100',
                      )}
                      title="Add comment"
                      onClick={(e) => {
                        e.stopPropagation()
                        // Shift-click extends a range
                        if (
                          e.shiftKey &&
                          rangeStart &&
                          rangeStart.side === effectiveSide
                        ) {
                          const startLine = Math.min(rangeStart.line, effectiveLine)
                          const endLine = Math.max(rangeStart.line, effectiveLine)
                          setPendingAnchor({
                            side: effectiveSide,
                            line: effectiveLine,
                            startLine,
                            endLine,
                          })
                          setRangeStart(null)
                        } else {
                          setRangeStart({ side: effectiveSide, line: effectiveLine })
                          setPendingAnchor({
                            side: effectiveSide,
                            line: effectiveLine,
                            startLine: effectiveLine,
                            endLine: effectiveLine,
                          })
                        }
                      }}
                    >
                      +
                    </button>
                  )
                }}
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
  wsId?: string
}

export const ReviewDiffView = memo(({ multiDiff, wsId: wsIdProp }: ReviewDiffViewProps) => {
  // Derive files once — guards against null/undefined from the backend serializing empty arrays as null
  const files = multiDiff?.files ?? []

  const [viewMode, setViewMode] = useState<'unified' | 'split'>('unified')

  // Scroll-to-file: subscribe to activeFileKey + activeFileNonce from the workspace store.
  // This component renders inside the branch-review pane's provider, so useWorkspaceStoreContext is correct.
  const activeFileKey = useWorkspaceStoreContext((s) => s.branchReview.activeFileKey)
  const activeFileNonce = useWorkspaceStoreContext((s) => s.branchReview.activeFileNonce)
  const workspaceId = useWorkspaceStoreContext((s) => s.workspaceId)
  const threads = useWorkspaceStoreContext((s) => s.branchReview.threads)

  const wsId = wsIdProp ?? workspaceId

  // Token cache: fileKey → HunkTokens | null | 'pending'
  // We use a ref (not state) to avoid re-renders on every cache write.
  // setResolvedCount is just a counter to trigger re-render when tokens arrive.
  const tokenCache = useRef<Map<string, HunkTokens | null | 'pending'>>(new Map())
  const [, setResolvedCount] = useState(0)

  const handleTokensResolved = useCallback(() => {
    setResolvedCount((c) => c + 1)
  }, [])

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

  // Track which file key should be force-expanded (set when scrolling to a file)
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

  // Scroll-to-file effect: when activeFileNonce changes (including re-clicks of the same key),
  // find the file by key, force-expand it, and scroll the virtualizer to it.
  useEffect(() => {
    if (!activeFileKey || activeFileNonce === 0) return
    const index = fileSummaries.findIndex((s) => s.key === activeFileKey)
    if (index === -1) return
    setExpandedByScrollKey(activeFileKey)
    virtualizer.scrollToIndex(index, { align: 'start' })
  }, [activeFileNonce]) // eslint-disable-line react-hooks/exhaustive-deps

  // Empty state — shown when the backend serializes files as null or an empty array
  if (files.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 text-center text-muted-foreground">
        <FileDashed className="size-6" />
        <span className="ui-text-sm">No changes to show.</span>
      </div>
    )
  }

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
                  tokenCache={tokenCache}
                  onTokensResolved={handleTokensResolved}
                  forceExpand={expandedByScrollKey === summary.key}
                  wsId={wsId}
                  threads={threads}
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
