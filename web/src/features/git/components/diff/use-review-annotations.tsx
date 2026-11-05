import { useCallback, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { Plus } from '@phosphor-icons/react'
import type {
  AnnotationSide,
  CodeViewItem,
  CodeViewLineSelection,
  DiffLineAnnotation,
  GetHoveredLineResult,
  SelectedLineRange,
} from '@pierre/diffs'
import { Button } from '@/components/ui/button'
import { ReviewThreadItem } from '@/features/git/components/review-thread-item'
import { useReviewThreadActions } from '@/features/git/hooks/use-review-thread-actions'
import { useReviewThreads } from '@/features/git/hooks/use-review-threads'
import { CommentComposer } from '@/features/panes/components/comment-composer'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'

// Review threads (live from the entity store, independent of the diff) as @pierre/diffs line
// annotations. An annotation needs a rendered line, and placeholder files have none, so a per-file
// thread count lets the header advertise what the body can't show yet.

/** One review thread, positioned on one side of one line of one file. */
export type ReviewAnnotation = DiffLineAnnotation<ReviewThread>

/** The unposted draft's id. The composer is an annotation, so it sits where the comment will,
 *  and annotations need metadata: the draft is a message-less ReviewThread no server id matches. */
export const DRAFT_THREAD_ID = '__crowbar-draft-thread__'

export function isDraftThread(thread: ReviewThread): boolean {
  return thread.id === DRAFT_THREAD_ID
}

// ── The side map ────────────────────────────────────────────────────
// Threads say old/new, the renderer deletions/additions. Written out, not derived: an inversion
// survives every round trip and puts every comment on the wrong half.

export function toAnnotationSide(side: ReviewThread['side']): AnnotationSide {
  return side === 'old' ? 'deletions' : 'additions'
}

export function toThreadSide(side: AnnotationSide): ReviewThread['side'] {
  return side === 'deletions' ? 'old' : 'new'
}

export function threadToAnnotation(thread: ReviewThread): ReviewAnnotation {
  return { side: toAnnotationSide(thread.side), lineNumber: thread.lineNumber, metadata: thread }
}

export function annotationToThread(annotation: ReviewAnnotation): ReviewThread {
  return annotation.metadata
}

/** A Conversation-only comment: it belongs to the branch, not to a line. */
export function isGeneralThread(thread: ReviewThread): boolean {
  return thread.filePath === ''
}

/** Annotations for every file that has any, ordered by line within each file. */
export function groupAnnotationsByPath(
  threads: readonly ReviewThread[],
): Map<string, ReviewAnnotation[]> {
  const byPath = new Map<string, ReviewThread[]>()
  for (const thread of threads) {
    if (isGeneralThread(thread)) continue
    const list = byPath.get(thread.filePath)
    if (list == null) byPath.set(thread.filePath, [thread])
    else list.push(thread)
  }

  const grouped = new Map<string, ReviewAnnotation[]>()
  for (const [path, list] of byPath) {
    grouped.set(path, [...list].sort((a, b) => a.lineNumber - b.lineNumber).map(threadToAnnotation))
  }
  return grouped
}

/** Threads per file, from the threads alone, so it is right for files the diff hasn't loaded. */
export function countThreadsByPath(threads: readonly ReviewThread[]): Record<string, number> {
  const counts: Record<string, number> = {}
  for (const thread of threads) {
    if (isGeneralThread(thread)) continue
    counts[thread.filePath] = (counts[thread.filePath] ?? 0) + 1
  }
  return counts
}

// ── The layer ───────────────────────────────────────────────────────

/** Where a comment is being written, before it is a thread. */
type Draft = ReviewThread & { id: typeof DRAFT_THREAD_ID }

export interface ReviewAnnotationLayer {
  /** Every file's annotations, including the draft composer's. */
  annotationsByPath: ReadonlyMap<string, ReviewAnnotation[]>
  /** Thread count per file, for headers whose file has no rendered lines. */
  threadCounts: Readonly<Record<string, number>>
  /** This file's threads, ordered by line. */
  threadsFor: (path: string) => ReviewThread[]
  draft: ReviewThread | null
  error: { threadId?: string; msg: string } | null
  startDraft: (path: string, range: SelectedLineRange) => void
  cancelDraft: () => void
  submitDraft: (body: string) => Promise<void>
  onSelectedLinesChange: (selection: CodeViewLineSelection | null) => void
  renderAnnotation: (annotation: ReviewAnnotation) => ReactNode
  renderGutterUtility: (
    getHoveredLine: (() => GetHoveredLineResult<'diff'> | undefined) | (() => unknown),
    item: CodeViewItem<ReviewThread>,
  ) => ReactNode
}

export function useReviewAnnotations({ wsId }: { wsId: string }): ReviewAnnotationLayer {
  const threads = useReviewThreads(wsId)
  const [draft, setDraft] = useState<Draft | null>(null)
  const {
    identity,
    error,
    clearError,
    postThread,
    reply: handleReply,
    resolve: handleResolve,
    reopen: handleReopen,
    edit: handleEditMessage,
    removeMessage: handleDeleteMessage,
    removeThread: handleDeleteThread,
  } = useReviewThreadActions(wsId)

  const threadAnnotations = useMemo(() => groupAnnotationsByPath(threads), [threads])
  const threadCounts = useMemo(() => countThreadsByPath(threads), [threads])

  const annotationsByPath = useMemo(() => {
    if (draft == null) return threadAnnotations
    const merged = new Map(threadAnnotations)
    const existing = merged.get(draft.filePath) ?? []
    merged.set(draft.filePath, [...existing, threadToAnnotation(draft)])
    return merged
  }, [draft, threadAnnotations])

  const threadsFor = useCallback(
    (path: string) => threadAnnotations.get(path)?.map(annotationToThread) ?? [],
    [threadAnnotations],
  )

  const startDraft = useCallback(
    (path: string, range: SelectedLineRange) => {
      clearError()
      // A thread has ONE side, so a selection dragged across both halves is
      // anchored to the side it started on and collapsed to that line.
      const side = range.side ?? 'additions'
      const crossesSides = range.endSide != null && range.endSide !== side
      const startLine = crossesSides ? range.start : Math.min(range.start, range.end)
      const endLine = crossesSides ? range.start : Math.max(range.start, range.end)
      setDraft({
        id: DRAFT_THREAD_ID,
        filePath: path,
        // The comment hangs off the LAST line of the range, as it does on GitHub.
        lineNumber: endLine,
        startLine,
        endLine,
        side: toThreadSide(side),
        messages: [],
        isResolved: false,
      })
    },
    [clearError],
  )

  const cancelDraft = useCallback(() => setDraft(null), [])

  const submitDraft = useCallback(
    async (body: string) => {
      if (draft == null) return
      const posted = await postThread({
        filePath: draft.filePath,
        line: draft.lineNumber,
        startLine: draft.startLine,
        endLine: draft.endLine,
        side: draft.side,
        body,
      })
      // A failed post keeps the draft: dropping it would throw away the text
      // the user just wrote because of a failure they cannot see.
      if (posted) setDraft(null)
    },
    [draft, postThread],
  )

  const onSelectedLinesChange = useCallback(
    (selection: CodeViewLineSelection | null) => {
      if (selection == null) {
        setDraft(null)
        return
      }
      startDraft(selection.id, selection.range)
    },
    [startDraft],
  )

  const renderAnnotation = useCallback(
    (annotation: ReviewAnnotation): ReactNode => {
      const thread = annotationToThread(annotation)
      if (isDraftThread(thread)) {
        return (
          // The code-font opt-out this slot needs lives on CommentComposer itself.
          <div className="max-w-xl py-2">
            <CommentComposer
              title={composerTitle(thread)}
              submitLabel="Comment"
              onSubmit={(body) => void submitDraft(body)}
              onCancel={cancelDraft}
            />
          </div>
        )
      }
      return (
        <ReviewThreadItem
          thread={thread}
          wsId={wsId}
          currentIdentity={identity}
          onReply={handleReply}
          onResolve={handleResolve}
          onReopen={handleReopen}
          onEditMessage={handleEditMessage}
          onDeleteMessage={handleDeleteMessage}
          onDeleteThread={handleDeleteThread}
        />
      )
    },
    [
      cancelDraft,
      handleDeleteMessage,
      handleDeleteThread,
      handleEditMessage,
      handleReopen,
      handleReply,
      handleResolve,
      identity,
      submitDraft,
      wsId,
    ],
  )

  const renderGutterUtility = useCallback(
    (
      getHoveredLine: (() => GetHoveredLineResult<'diff'> | undefined) | (() => unknown),
      item: CodeViewItem<ReviewThread>,
    ): ReactNode => (
      // The button sits over the sticky gutter (z 3) in an unstacked wrapper: z-10 lifts it above
      // the line numbers and annotation row (2) within the file's stacking context, and the
      // filled chip reads on both added green and removed red.
      <Button
        variant="default"
        size="icon-xs"
        className="z-10"
        aria-label={`Comment on a line in ${item.id}`}
        onClick={() => {
          const hovered = getHoveredLine()
          // A file (non-diff) row has no side, and a review comment needs one.
          if (hovered == null || typeof hovered !== 'object' || !('side' in hovered)) return
          const { lineNumber, side } = hovered as GetHoveredLineResult<'diff'>
          startDraft(item.id, { start: lineNumber, end: lineNumber, side })
        }}
      >
        <Plus />
      </Button>
    ),
    [startDraft],
  )

  return {
    annotationsByPath,
    threadCounts,
    threadsFor,
    draft,
    error,
    startDraft,
    cancelDraft,
    submitDraft,
    onSelectedLinesChange,
    renderAnnotation,
    renderGutterUtility,
  }
}

function composerTitle(draft: ReviewThread): string {
  const half = draft.side === 'old' ? 'the original' : 'the new'
  if (draft.startLine === draft.endLine) return `Comment on line ${draft.endLine} of ${half} file`
  return `Comment on lines ${draft.startLine}–${draft.endLine} of ${half} file`
}
