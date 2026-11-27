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

export type ReviewAnnotation = DiffLineAnnotation<ReviewThread>

const DRAFT_THREAD_ID = '__crowbar-draft-thread__'

function isDraftThread(thread: ReviewThread): boolean {
  return thread.id === DRAFT_THREAD_ID
}

export function toAnnotationSide(side: ReviewThread['side']): AnnotationSide {
  return side === 'old' ? 'deletions' : 'additions'
}

function toThreadSide(side: AnnotationSide): ReviewThread['side'] {
  return side === 'deletions' ? 'old' : 'new'
}

function threadToAnnotation(thread: ReviewThread): ReviewAnnotation {
  return { side: toAnnotationSide(thread.side), lineNumber: thread.lineNumber, metadata: thread }
}

function annotationToThread(annotation: ReviewAnnotation): ReviewThread {
  return annotation.metadata
}

export function isGeneralThread(thread: ReviewThread): boolean {
  return thread.filePath === ''
}

function groupAnnotationsByPath(threads: readonly ReviewThread[]): Map<string, ReviewAnnotation[]> {
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

function countThreadsByPath(threads: readonly ReviewThread[]): Record<string, number> {
  const counts: Record<string, number> = {}
  for (const thread of threads) {
    if (isGeneralThread(thread)) continue
    counts[thread.filePath] = (counts[thread.filePath] ?? 0) + 1
  }
  return counts
}

type Draft = ReviewThread & { id: typeof DRAFT_THREAD_ID }

export interface ReviewAnnotationLayer {
  annotationsByPath: ReadonlyMap<string, ReviewAnnotation[]>
  threadCounts: Readonly<Record<string, number>>
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
      const side = range.side ?? 'additions'
      const crossesSides = range.endSide != null && range.endSide !== side
      const startLine = crossesSides ? range.start : Math.min(range.start, range.end)
      const endLine = crossesSides ? range.start : Math.max(range.start, range.end)
      setDraft({
        id: DRAFT_THREAD_ID,
        filePath: path,
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
      <Button
        variant="default"
        size="icon-xs"
        className="z-10"
        aria-label={`Comment on a line in ${item.id}`}
        onClick={() => {
          const hovered = getHoveredLine()
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
