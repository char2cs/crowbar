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
import {
  deleteMessage,
  deleteThread,
  editMessage,
  openThread,
  replyToThread,
  setThreadResolved,
} from '@/features/git/api/review-api'
import { ReviewThreadItem } from '@/features/git/components/review-thread-item'
import { resolveIdentity, useCurrentIdentity } from '@/features/git/hooks/use-current-identity'
import { CommentComposer } from '@/features/panes/components/comment-composer'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'

/**
 * Review threads as `@pierre/diffs` line annotations.
 *
 * This replaces the HOST, not the behaviour. `use-review-comment-layer.tsx` hung
 * the same threads off Monaco view zones anchored to a unified model line; here
 * they are `DiffLineAnnotation`s the renderer portals into LIGHT dom beside the
 * line they belong to. Everything either side of that is unchanged: the threads
 * still come from `branchReview.threads` (kept live by the WS stream, and
 * therefore INDEPENDENT of whether the diff payload has loaded), and every write
 * still goes through `review-api.ts`.
 *
 * The one genuinely new problem the annotation host creates: an annotation needs
 * a rendered LINE to attach to, and the windowed surface deliberately leaves
 * most files as content-free placeholders. A thread on such a file is not
 * missing — it simply has nowhere to hang. That is why the layer publishes a
 * per-file thread COUNT alongside the annotations: the header can advertise what
 * the body cannot yet show. (Reaching one is the surface's job — it has to
 * materialise the file before it can scroll to a line inside it.)
 */

/** One review thread, positioned on one side of one line of one file. */
export type ReviewAnnotation = DiffLineAnnotation<ReviewThread>

/**
 * The id of the thread being composed but not yet posted.
 *
 * The composer is an annotation like any other so that it appears exactly where
 * the finished comment will, and `metadata` on a `DiffLineAnnotation<T>` is
 * required — so the draft is carried as a real, message-less `ReviewThread`.
 * Nothing the server ever returns can collide with this id.
 */
export const DRAFT_THREAD_ID = '__crowbar-draft-thread__'

export function isDraftThread(thread: ReviewThread): boolean {
  return thread.id === DRAFT_THREAD_ID
}

// ── The side map ────────────────────────────────────────────────────
//
// A thread records `old`/`new`; the renderer speaks `deletions`/`additions`.
// The two mappings are written out rather than derived because an inversion
// here is invisible in every round trip and puts every comment on the wrong
// half of the diff.

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

/** Annotations for every file that has any, ordered by line within each file. */
export function groupAnnotationsByPath(
  threads: readonly ReviewThread[],
): Map<string, ReviewAnnotation[]> {
  const byPath = new Map<string, ReviewThread[]>()
  for (const thread of threads) {
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

/**
 * How many threads each file carries.
 *
 * Counted from the threads store alone, so it is right for files the diff has
 * not loaded — which is the entire reason it exists.
 */
export function countThreadsByPath(threads: readonly ReviewThread[]): Record<string, number> {
  const counts: Record<string, number> = {}
  for (const thread of threads) counts[thread.filePath] = (counts[thread.filePath] ?? 0) + 1
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
  const threads = useWorkspaceStoreContext((s) => s.branchReview.threads)
  const identity = useCurrentIdentity(wsId)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [error, setError] = useState<{ threadId?: string; msg: string } | null>(null)

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

  // Always stamp the real provider login. The identity hook returns null until
  // its fetch resolves, so eagerly await it here — posting before it loaded used
  // to store an empty author (the "U / You" bug).
  const resolveAuthor = useCallback(async () => {
    const id = identity ?? (await resolveIdentity(wsId))
    return id?.login || id?.displayName || undefined
  }, [identity, wsId])

  const startDraft = useCallback((path: string, range: SelectedLineRange) => {
    setError(null)
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
  }, [])

  const cancelDraft = useCallback(() => setDraft(null), [])

  const submitDraft = useCallback(
    async (body: string) => {
      if (draft == null) return
      setError(null)
      try {
        const author = await resolveAuthor()
        await openThread(wsId, {
          filePath: draft.filePath,
          line: draft.lineNumber,
          startLine: draft.startLine,
          endLine: draft.endLine,
          side: draft.side,
          author,
          isAgent: false,
          body,
        })
        setDraft(null)
      } catch {
        // The draft survives on purpose: dropping it would throw away the text
        // the user just wrote because of a failure they cannot see.
        setError({ msg: 'Failed to post comment' })
      }
    },
    [draft, resolveAuthor, wsId],
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

  const handleReply = useCallback(
    async (threadId: string, body: string) => {
      setError(null)
      try {
        const author = await resolveAuthor()
        await replyToThread(wsId, threadId, { author, isAgent: false, body })
      } catch {
        setError({ threadId, msg: 'Failed to post reply' })
      }
    },
    [resolveAuthor, wsId],
  )
  const handleResolve = useCallback(
    async (threadId: string) => {
      await setThreadResolved(wsId, threadId, true)
    },
    [wsId],
  )
  const handleReopen = useCallback(
    async (threadId: string) => {
      await setThreadResolved(wsId, threadId, false)
    },
    [wsId],
  )
  const handleEditMessage = useCallback(
    async (threadId: string, messageId: string, body: string) => {
      setError(null)
      try {
        await editMessage(wsId, threadId, messageId, body)
      } catch (err) {
        setError({ threadId, msg: 'Failed to edit comment' })
        // Re-throw so the inline editor stays open and the user keeps their text.
        throw err
      }
    },
    [wsId],
  )
  const handleDeleteMessage = useCallback(
    async (threadId: string, messageId: string) => {
      setError(null)
      try {
        await deleteMessage(wsId, threadId, messageId)
      } catch {
        setError({ threadId, msg: 'Failed to delete comment' })
      }
    },
    [wsId],
  )
  const handleDeleteThread = useCallback(
    async (threadId: string) => {
      setError(null)
      try {
        await deleteThread(wsId, threadId)
      } catch {
        setError({ threadId, msg: 'Failed to delete thread' })
      }
    },
    [wsId],
  )

  const renderAnnotation = useCallback(
    (annotation: ReviewAnnotation): ReactNode => {
      const thread = annotationToThread(annotation)
      if (isDraftThread(thread)) {
        return (
          // The font opt-out this slot needs (an annotation is parented by the
          // diff's <pre>, so it inherits the CODE font) lives on CommentComposer
          // itself — it is a property of the composer, not of this one call
          // site, and the other two call sites need it just as much.
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
      // Filled rather than ghost, and lifted above the gutter.
      //
      // The library parks this button over the gutter column, which is
      // `position: sticky; z-index: 3` — and the wrapper it slots the button
      // into carries no z-index of its own. So the button painted UNDER the
      // line numbers: the number read straight through the "+", the two glyphs
      // tangled, and the control was neither legible nor recognisable as one.
      // Opacity alone could not fix that; what is on top has to change.
      //
      // z-10 clears the gutter's 3 and the annotation row's 2 while staying
      // inside the file's own stacking context, and the filled primary chip is
      // legible against both the added-line green and the removed-line red.
      <Button
        variant="default"
        size="icon-xs"
        className="z-10"
        aria-label={`Comment on a line in ${item.id}`}
        onClick={() => {
          const hovered = getHoveredLine()
          // A file (non-diff) row has no side, and there is no such thing as a
          // one-sided review comment — so there is nothing to open.
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
